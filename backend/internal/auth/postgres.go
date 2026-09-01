package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"homeflow/backend/internal/platform/id"
	"homeflow/backend/internal/shopping"
)

const (
	accessLifetime     = 15 * time.Minute
	refreshLifetime    = 30 * 24 * time.Hour
	defaultHouseholdID = "00000000-0000-4000-8000-000000000010"
)

type BootstrapUser struct {
	Username    string
	DisplayName string
	Password    string
	Role        string
}

type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type TokenPair struct {
	AccessToken      string    `json:"accessToken"`
	AccessExpiresAt  time.Time `json:"accessExpiresAt"`
	RefreshToken     string    `json:"refreshToken"`
	RefreshExpiresAt time.Time `json:"refreshExpiresAt"`
	TokenType        string    `json:"tokenType"`
}

// passwordConcurrency bounds how many Argon2id verifications may run at once.
// It is not a throughput knob: each one allocates argonMemory of anonymous
// memory that nothing can reclaim under pressure, so this number multiplied by
// argonMemory is the floor the container limit has to clear.
//
// It was 2. Two concurrent logins drove the pod's cgroup to 191.8 MiB of its
// 192 MiB limit and the kernel killed it - measured, with the OOM trace, in
// HF-0086. Serialising costs the second login the duration of the first hash,
// roughly 100 to 200 ms, and nothing else.
const passwordConcurrency = 1

type PostgresService struct {
	pool          *pgxpool.Pool
	now           func() time.Time
	passwordSlots chan struct{}
}

func NewPostgresService(pool *pgxpool.Pool) *PostgresService {
	return &PostgresService{pool: pool, now: func() time.Time { return time.Now().UTC() },
		passwordSlots: make(chan struct{}, passwordConcurrency)}
}

func Bootstrap(ctx context.Context, pool *pgxpool.Pool, users [2]BootstrapUser) error {
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if count == 2 {
		if err := ensureSharedList(ctx, pool); err != nil {
			return err
		}
		return ensureHouseholdMemberships(ctx, pool)
	}
	if count != 0 {
		return fmt.Errorf("database contains %d users; HomeFlow requires zero or exactly two", count)
	}
	for i := range users {
		if users[i].Username == "" || users[i].DisplayName == "" || users[i].Password == "" {
			return fmt.Errorf("bootstrap user %d is incomplete", i+1)
		}
		if users[i].Role == "" {
			users[i].Role = "member"
		}
	}
	if users[0].Username == users[1].Username {
		return errors.New("bootstrap usernames must be different")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, user := range users {
		hash, err := HashPassword(user.Password)
		if err != nil {
			return fmt.Errorf("hash password for %s: %w", user.Username, err)
		}
		userID, err := id.New()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO users(id,username,display_name,password_hash,role)
            VALUES($1,$2,$3,$4,$5)`, userID, user.Username, user.DisplayName, hash, user.Role); err != nil {
			return fmt.Errorf("create bootstrap user %s: %w", user.Username, err)
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO households(id,name) VALUES($1,'Zuhause') ON CONFLICT DO NOTHING`, defaultHouseholdID); err != nil {
		return fmt.Errorf("create default household: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO household_members(household_id,user_id,role)
		SELECT $1,id,CASE WHEN role='admin' THEN 'owner' ELSE 'member' END FROM users
		ON CONFLICT (user_id) DO NOTHING`, defaultHouseholdID); err != nil {
		return fmt.Errorf("assign bootstrap users to household: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO shopping_lists(id,household_id,name,visibility)
		VALUES($1,$2,'Gemeinsame Einkaufsliste','shared') ON CONFLICT DO NOTHING`, shopping.SharedListID, defaultHouseholdID); err != nil {
		return fmt.Errorf("create shared shopping list: %w", err)
	}
	return tx.Commit(ctx)
}

func ensureHouseholdMemberships(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `INSERT INTO household_members(household_id,user_id,role)
		SELECT $1,id,CASE WHEN role='admin' THEN 'owner' ELSE 'member' END FROM users
		ON CONFLICT (user_id) DO NOTHING`, defaultHouseholdID)
	return err
}

func ensureSharedList(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `INSERT INTO shopping_lists(id,household_id,name,visibility)
		VALUES($1,$2,'Gemeinsame Einkaufsliste','shared') ON CONFLICT DO NOTHING`, shopping.SharedListID, defaultHouseholdID)
	return err
}

// withPasswordSlot runs work while holding one of the passwordConcurrency
// slots, and blocks until a slot is free or the caller gives up.
//
// The slot exists to bound memory, and only Argon2 allocates any, so it is
// taken as late as possible and given back immediately. It used to wrap all of
// Login - database query, audit write and token minting included - which was
// invisible at a capacity of two and serialised the whole login at a capacity
// of one.
func (s *PostgresService) withPasswordSlot(ctx context.Context, work func()) error {
	select {
	case s.passwordSlots <- struct{}{}:
		defer func() { <-s.passwordSlots }()
	case <-ctx.Done():
		return ctx.Err()
	}
	work()
	return nil
}

func (s *PostgresService) Login(ctx context.Context, credentials Credentials, sourceIP string) (TokenPair, error) {
	var principal Principal
	var passwordHash string
	var disabled bool
	err := s.pool.QueryRow(ctx, `SELECT id::text,username,role,password_hash,disabled
        FROM users WHERE username=$1`, credentials.Username).Scan(
		&principal.UserID, &principal.Username, &principal.Role, &passwordHash, &disabled)
	if errors.Is(err, pgx.ErrNoRows) {
		// The dummy verification keeps the response time of an unknown user
		// indistinguishable from a wrong password, so it costs the same Argon2
		// memory and takes the same slot. Skipping it here would turn the
		// timing defence into the way around the memory bound.
		if err := s.withPasswordSlot(ctx, func() { consumePasswordWork(credentials.Password) }); err != nil {
			return TokenPair{}, err
		}
		_ = s.audit(ctx, "auth.login.failed", "", sourceIP, `{"reason":"invalid_credentials"}`)
		return TokenPair{}, ErrUnauthenticated
	}
	if err != nil {
		return TokenPair{}, err
	}
	var valid bool
	var verifyErr error
	if err := s.withPasswordSlot(ctx, func() {
		valid, verifyErr = VerifyPassword(credentials.Password, passwordHash)
	}); err != nil {
		return TokenPair{}, err
	}
	err = verifyErr
	if err != nil || !valid || disabled {
		_ = s.audit(ctx, "auth.login.failed", principal.UserID, sourceIP, `{"reason":"invalid_credentials"}`)
		return TokenPair{}, ErrUnauthenticated
	}
	pair, err := s.createSession(ctx, principal.UserID, "")
	if err == nil {
		_ = s.audit(ctx, "auth.login.succeeded", principal.UserID, sourceIP, `{}`)
	}
	return pair, err
}

func (s *PostgresService) Authenticate(ctx context.Context, token string) (Principal, error) {
	hash := sha256.Sum256([]byte(token))
	var principal Principal
	err := s.pool.QueryRow(ctx, `SELECT u.id::text,u.username,u.role
        FROM auth_sessions s JOIN users u ON u.id=s.user_id
        WHERE s.access_token_hash=$1 AND s.revoked_at IS NULL
          AND s.access_expires_at > $2 AND NOT u.disabled`, hash[:], s.now()).Scan(
		&principal.UserID, &principal.Username, &principal.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrUnauthenticated
	}
	return principal, err
}

func (s *PostgresService) Refresh(ctx context.Context, refreshToken, sourceIP string) (TokenPair, error) {
	hash := sha256.Sum256([]byte(refreshToken))
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return TokenPair{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var sessionID, userID string
	var expiresAt time.Time
	var revokedAt *time.Time
	err = tx.QueryRow(ctx, `SELECT id::text,user_id::text,refresh_expires_at,revoked_at
        FROM auth_sessions WHERE refresh_token_hash=$1 FOR UPDATE`, hash[:]).Scan(
		&sessionID, &userID, &expiresAt, &revokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return TokenPair{}, ErrUnauthenticated
	}
	if err != nil {
		return TokenPair{}, err
	}
	now := s.now()
	if revokedAt != nil {
		_, _ = tx.Exec(ctx, `UPDATE auth_sessions SET revoked_at=COALESCE(revoked_at,$2) WHERE user_id=$1`, userID, now)
		_ = tx.Commit(ctx)
		_ = s.audit(ctx, "auth.refresh.reuse_detected", userID, sourceIP, `{}`)
		return TokenPair{}, ErrUnauthenticated
	}
	if !expiresAt.After(now) {
		return TokenPair{}, ErrUnauthenticated
	}
	if _, err := tx.Exec(ctx, `UPDATE auth_sessions SET revoked_at=$2,last_used_at=$2 WHERE id=$1`, sessionID, now); err != nil {
		return TokenPair{}, err
	}
	pair, err := createSessionWith(ctx, tx, userID, sessionID, now)
	if err != nil {
		return TokenPair{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return TokenPair{}, err
	}
	_ = s.audit(ctx, "auth.refresh.rotated", userID, sourceIP, `{}`)
	return pair, nil
}

func (s *PostgresService) Logout(ctx context.Context, refreshToken, sourceIP string) error {
	hash := sha256.Sum256([]byte(refreshToken))
	var userID string
	err := s.pool.QueryRow(ctx, `UPDATE auth_sessions SET revoked_at=COALESCE(revoked_at,$2)
        WHERE refresh_token_hash=$1 RETURNING user_id::text`, hash[:], s.now()).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err == nil {
		_ = s.audit(ctx, "auth.logout", userID, sourceIP, `{}`)
	}
	return err
}

func (s *PostgresService) createSession(ctx context.Context, userID, rotatedFrom string) (TokenPair, error) {
	return createSessionWith(ctx, s.pool, userID, rotatedFrom, s.now())
}

type queryExecer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func createSessionWith(ctx context.Context, db queryExecer, userID, rotatedFrom string, now time.Time) (TokenPair, error) {
	accessToken, err := randomToken()
	if err != nil {
		return TokenPair{}, err
	}
	refreshToken, err := randomToken()
	if err != nil {
		return TokenPair{}, err
	}
	sessionID, err := id.New()
	if err != nil {
		return TokenPair{}, err
	}
	accessHash := sha256.Sum256([]byte(accessToken))
	refreshHash := sha256.Sum256([]byte(refreshToken))
	accessExpires := now.Add(accessLifetime)
	refreshExpires := now.Add(refreshLifetime)
	var rotated any
	if rotatedFrom != "" {
		rotated = rotatedFrom
	}
	_, err = db.Exec(ctx, `INSERT INTO auth_sessions
        (id,user_id,access_token_hash,refresh_token_hash,access_expires_at,refresh_expires_at,rotated_from_id)
        VALUES($1,$2,$3,$4,$5,$6,$7)`, sessionID, userID, accessHash[:], refreshHash[:], accessExpires, refreshExpires, rotated)
	if err != nil {
		return TokenPair{}, err
	}
	return TokenPair{AccessToken: accessToken, AccessExpiresAt: accessExpires,
		RefreshToken: refreshToken, RefreshExpiresAt: refreshExpires, TokenType: "Bearer"}, nil
}

func randomToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func (s *PostgresService) audit(ctx context.Context, eventType, userID, sourceIP, metadata string) error {
	auditID, err := id.New()
	if err != nil {
		return err
	}
	var actor any
	if userID != "" {
		actor = userID
	}
	var ip any
	if sourceIP != "" {
		ip = sourceIP
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO audit_log(id,actor_user_id,event_type,source_ip,metadata)
        VALUES($1,$2,$3,$4,$5::jsonb)`, auditID, actor, eventType, ip, metadata)
	return err
}

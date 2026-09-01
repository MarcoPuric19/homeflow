package integration

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"homeflow/backend/internal/auth"
	"homeflow/backend/internal/calendar"
	"homeflow/backend/internal/documents"
	"homeflow/backend/internal/expenses"
	"homeflow/backend/internal/links"
	"homeflow/backend/internal/pets"
	"homeflow/backend/internal/postgres"
	"homeflow/backend/internal/receipts"
	"homeflow/backend/internal/reminders"
	"homeflow/backend/internal/shopping"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("HOMEFLOW_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("HOMEFLOW_TEST_DATABASE_URL is not set")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(config.ConnConfig.Database, "_test") {
		t.Fatalf("refusing destructive integration test against database %q: name must end in _test", config.ConnConfig.Database)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := postgres.Migrate(ctx, pool); err != nil {
		pool.Close()
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `TRUNCATE event_outbox,audit_log,expense_appointments,expense_medical_records,document_appointments,document_medical_records,receipts,documents,pet_budgets,expenses,calendar_events,lab_results,weight_entries,
		shopping_items,shopping_lists,pet_medications,medications,vaccinations,medical_records,pets,household_members,auth_sessions,users,households CASCADE`)
	if err != nil {
		pool.Close()
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestPetHouseholdIsolationAndPagination(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	if err := auth.Bootstrap(ctx, pool, [2]auth.BootstrapUser{
		{Username: "alice", DisplayName: "Alice", Password: "correct horse battery staple", Role: "admin"},
		{Username: "bob", DisplayName: "Bob", Password: "another secure household password", Role: "member"},
	}); err != nil {
		t.Fatal(err)
	}
	var alice, bob string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM users WHERE username='alice'`).Scan(&alice); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM users WHERE username='bob'`).Scan(&bob); err != nil {
		t.Fatal(err)
	}
	const secondHousehold = "00000000-0000-4000-8000-000000000020"
	if _, err := pool.Exec(ctx, `INSERT INTO households(id,name) VALUES($1,'Other')`, secondHousehold); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE household_members SET household_id=$1 WHERE user_id=$2`, secondHousehold, bob); err != nil {
		t.Fatal(err)
	}
	service := pets.NewService(pets.NewPostgresRepository(pool))
	birthDate := "2021-04-01"
	created, err := service.Create(ctx, alice, pets.Input{Name: "Momo", Species: "Katze", BirthDate: &birthDate, ApproximateBirthDate: true, Color: "Getigert", Insurance: "TierSchutz", InsuranceNumber: "TS-42", Veterinarian: "Praxis am Park"})
	if err != nil {
		t.Fatal(err)
	}
	if !created.ApproximateBirthDate || created.Color != "Getigert" || created.InsuranceNumber != "TS-42" || created.Veterinarian != "Praxis am Park" {
		t.Fatalf("pet profile details=%#v", created)
	}
	updated, err := service.Update(ctx, alice, created.ID, pets.Input{Name: "Momo", Species: "Katze", BirthDate: &birthDate, Color: "Schwarz", Insurance: "TierSchutz", InsuranceNumber: "TS-43", Veterinarian: "Neue Praxis"})
	if err != nil || updated.Color != "Schwarz" || updated.InsuranceNumber != "TS-43" || updated.ApproximateBirthDate {
		t.Fatalf("updated pet profile=%#v err=%v", updated, err)
	}
	if _, err := service.Get(ctx, bob, created.ID); !errors.Is(err, pets.ErrNotFound) {
		t.Fatalf("cross-household read err=%v", err)
	}
	weight, err := service.CreateWeight(ctx, alice, created.ID, pets.WeightInput{MeasuredOn: "2026-08-31", WeightKg: "4.250"})
	if err != nil || weight.WeightKg != "4.250" {
		t.Fatalf("weight=%#v err=%v", weight, err)
	}
	if _, err := service.ListWeights(ctx, bob, created.ID, 100); !errors.Is(err, pets.ErrNotFound) {
		t.Fatalf("cross-household weights err=%v", err)
	}
	low, high := "4.0000", "8.0000"
	lab, err := service.CreateLabResult(ctx, alice, created.ID, pets.LabResultInput{MeasuredOn: "2026-08-31", TestName: "Glukose", Value: "5.5000", Unit: "mmol/L", ReferenceLow: &low, ReferenceHigh: &high})
	if err != nil || lab.Value != "5.5000" {
		t.Fatalf("lab=%#v err=%v", lab, err)
	}
	if _, err := service.ListLabResults(ctx, bob, created.ID, 100); !errors.Is(err, pets.ErrNotFound) {
		t.Fatalf("cross-household labs err=%v", err)
	}
	documentStorage, err := receipts.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	documentService := documents.NewService(documents.NewPostgresRepository(pool), documentStorage)
	documentFile, err := os.CreateTemp(t.TempDir(), "document")
	if err != nil {
		t.Fatal(err)
	}
	defer documentFile.Close()
	if _, err := documentFile.Write([]byte("%PDF-1.7\npet report")); err != nil {
		t.Fatal(err)
	}
	if _, err := documentFile.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	documentDate := "2026-08-31"
	document, err := documentService.Upload(ctx, alice, created.ID, documentFile, "../Befund.pdf", documents.Metadata{Title: "Befund", DocumentType: "report", OccurredOn: &documentDate})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := documentService.List(ctx, bob, created.ID, 100); !errors.Is(err, documents.ErrNotFound) {
		t.Fatalf("cross-household documents err=%v", err)
	}
	if _, _, err := documentService.Content(ctx, bob, created.ID, document.ID); !errors.Is(err, documents.ErrNotFound) {
		t.Fatalf("cross-household document content err=%v", err)
	}
	expenseService := expenses.NewService(expenses.NewPostgresRepository(pool))
	petExpense, err := expenseService.Create(ctx, expenses.CreateExpense{Description: "Tierarzt", PlannedAmountCents: 7500, DueDate: "2026-09-15", PetID: &created.ID, BudgetCategory: "Tierarzt"}, alice)
	if err != nil {
		t.Fatal(err)
	}
	otherExpenses, err := expenseService.List(ctx, bob, "2026-09", "", 100)
	if err != nil || len(otherExpenses) != 0 {
		t.Fatalf("cross-household expenses=%#v err=%v", otherExpenses, err)
	}
	if _, err := expenseService.UpsertBudget(ctx, bob, created.ID, expenses.BudgetInput{Month: "2026-09", Category: "Tierarzt", AmountCents: 10000}); !errors.Is(err, expenses.ErrNotFound) {
		t.Fatalf("cross-household budget err=%v", err)
	}
	if _, err := expenseService.UpsertBudget(ctx, alice, created.ID, expenses.BudgetInput{Month: "2026-09", Category: "Tierarzt", AmountCents: 10000}); err != nil {
		t.Fatal(err)
	}
	petSummary, err := expenseService.PetSummary(ctx, alice, created.ID, "2026-09")
	if err != nil || len(petSummary.Categories) != 1 || petSummary.Categories[0].PlannedAmountCents != petExpense.PlannedAmountCents || petSummary.Categories[0].BudgetAmountCents != 10000 {
		t.Fatalf("pet summary=%#v err=%v", petSummary, err)
	}
	page, err := service.List(ctx, alice, false, 1, "")
	if err != nil || len(page.Pets) != 1 || page.Pets[0].ID != created.ID {
		t.Fatalf("page=%#v err=%v", page, err)
	}
	first, err := service.CreateMedicalRecord(ctx, alice, created.ID, pets.MedicalRecordInput{RecordType: pets.RecordExamination, OccurredOn: "2026-08-30", Title: "Kontrolle", Details: map[string]string{"temperature": "38.2 C"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateMedicalRecord(ctx, alice, created.ID, pets.MedicalRecordInput{RecordType: pets.RecordNote, OccurredOn: "2026-08-31", Title: "Verlauf"}); err != nil {
		t.Fatal(err)
	}
	medical, err := service.ListMedicalRecords(ctx, alice, created.ID, 1, "")
	if err != nil || len(medical.Records) != 1 || medical.NextCursor == "" {
		t.Fatalf("medical=%#v err=%v", medical, err)
	}
	next, err := service.ListMedicalRecords(ctx, alice, created.ID, 1, medical.NextCursor)
	if err != nil || len(next.Records) != 1 || next.Records[0].ID != first.ID {
		t.Fatalf("next=%#v err=%v", next, err)
	}
	if _, err := service.ListMedicalRecords(ctx, bob, created.ID, 30, ""); !errors.Is(err, pets.ErrNotFound) {
		t.Fatalf("cross-household medical read err=%v", err)
	}
	due := "2027-08-30"
	validUntil := "2027-08-30"
	firstVaccination, err := service.CreateVaccination(ctx, alice, created.ID, pets.VaccinationInput{VaccineName: "Tollwut", Manufacturer: "Beispiel", Diseases: []string{"Tollwut"}, AdministeredOn: "2026-08-30", ValidUntil: &validUntil, NextDueDate: &due, DocumentID: &document.ID})
	if err != nil {
		t.Fatal(err)
	}
	if firstVaccination.DocumentID == nil || *firstVaccination.DocumentID != document.ID || len(firstVaccination.Diseases) != 1 {
		t.Fatalf("vaccination details=%#v", firstVaccination)
	}
	otherPet, err := service.Create(ctx, alice, pets.Input{Name: "Luna", Species: "Katze"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateVaccination(ctx, alice, otherPet.ID, pets.VaccinationInput{VaccineName: "Falsch", AdministeredOn: "2026-08-30", DocumentID: &document.ID}); !errors.Is(err, pets.ErrNotFound) {
		t.Fatalf("cross-pet document link err=%v", err)
	}
	if _, err := service.CreateVaccination(ctx, alice, created.ID, pets.VaccinationInput{VaccineName: "Katzenschnupfen", AdministeredOn: "2026-08-31"}); err != nil {
		t.Fatal(err)
	}
	vaccinations, err := service.ListVaccinations(ctx, alice, created.ID, 1, "")
	if err != nil || len(vaccinations.Vaccinations) != 1 || vaccinations.NextCursor == "" {
		t.Fatalf("vaccinations=%#v err=%v", vaccinations, err)
	}
	nextVaccinations, err := service.ListVaccinations(ctx, alice, created.ID, 1, vaccinations.NextCursor)
	if err != nil || len(nextVaccinations.Vaccinations) != 1 || nextVaccinations.Vaccinations[0].ID != firstVaccination.ID {
		t.Fatalf("next vaccinations=%#v err=%v", nextVaccinations, err)
	}
	if _, err := service.ListVaccinations(ctx, bob, created.ID, 30, ""); !errors.Is(err, pets.ErrNotFound) {
		t.Fatalf("cross-household vaccination read err=%v", err)
	}
	calendarService := calendar.NewService(calendar.NewPostgresRepository(pool))
	petID := created.ID
	appointmentType := calendar.AppointmentVeterinarian
	appointment, err := calendarService.Create(ctx, calendar.EventInput{Title: "Tierarzt", StartsAt: time.Date(2026, 9, 10, 9, 0, 0, 0, time.UTC), EndsAt: time.Date(2026, 9, 10, 10, 0, 0, 0, time.UTC), PetID: &petID, AppointmentType: &appointmentType}, alice)
	if err != nil || appointment.PetID == nil || *appointment.PetID != created.ID {
		t.Fatalf("appointment=%#v err=%v", appointment, err)
	}
	reminderService := reminders.NewService(reminders.NewPostgresRepository(pool))
	upcoming, err := reminderService.List(ctx, alice, "2026-09-30", 100)
	if err != nil || len(upcoming) != 1 || upcoming[0].Kind != "appointment" {
		t.Fatalf("reminders=%#v err=%v", upcoming, err)
	}
	otherUpcoming, err := reminderService.List(ctx, bob, "2026-09-30", 100)
	if err != nil || len(otherUpcoming) != 0 {
		t.Fatalf("cross-household reminders=%#v err=%v", otherUpcoming, err)
	}
	dashboard, err := service.Dashboard(ctx, alice, created.ID, "2026-09")
	if err != nil || dashboard.PetID != created.ID || dashboard.LatestWeightKg == nil || *dashboard.LatestWeightKg != "4.250" || dashboard.PlannedAmountCents != 7500 || dashboard.BudgetAmountCents != 10000 || dashboard.NextAppointmentTitle == nil {
		t.Fatalf("dashboard=%#v err=%v", dashboard, err)
	}
	if _, err := service.Dashboard(ctx, bob, created.ID, "2026-09"); !errors.Is(err, pets.ErrNotFound) {
		t.Fatalf("cross-household dashboard err=%v", err)
	}
	window := calendar.Range{From: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), PetID: created.ID}
	appointments, err := calendarService.List(ctx, alice, window)
	if err != nil || len(appointments) != 1 {
		t.Fatalf("appointments=%#v err=%v", appointments, err)
	}
	foreignAppointments, err := calendarService.List(ctx, bob, window)
	if err != nil || len(foreignAppointments) != 0 {
		t.Fatalf("foreign appointments=%#v err=%v", foreignAppointments, err)
	}
	linkService := links.NewService(links.NewPostgresRepository(pool))
	documentLinks, err := linkService.ReplaceDocument(ctx, alice, created.ID, document.ID, links.Set{MedicalRecordIDs: []string{first.ID}, AppointmentIDs: []string{appointment.ID}})
	if err != nil || len(documentLinks.MedicalRecordIDs) != 1 || len(documentLinks.AppointmentIDs) != 1 {
		t.Fatalf("document links=%#v err=%v", documentLinks, err)
	}
	expenseLinks, err := linkService.ReplaceExpense(ctx, alice, petExpense.ID, links.Set{MedicalRecordIDs: []string{first.ID}, AppointmentIDs: []string{appointment.ID}})
	if err != nil || len(expenseLinks.MedicalRecordIDs) != 1 || len(expenseLinks.AppointmentIDs) != 1 {
		t.Fatalf("expense links=%#v err=%v", expenseLinks, err)
	}
	if _, err := linkService.GetDocument(ctx, bob, created.ID, document.ID); !errors.Is(err, links.ErrNotFound) {
		t.Fatalf("cross-household document links err=%v", err)
	}
	if _, err := linkService.ReplaceDocument(ctx, alice, created.ID, document.ID, links.Set{MedicalRecordIDs: []string{first.ID}, AppointmentIDs: []string{"00000000-0000-4000-8000-000000000099"}}); !errors.Is(err, links.ErrNotFound) {
		t.Fatalf("unknown appointment link err=%v", err)
	}
	medication, err := service.CreatePetMedication(ctx, alice, created.ID, pets.PetMedicationInput{Name: "Mittel", DoseText: "laut Tierarzt", FrequencyText: "morgens", StartsOn: "2026-09-01", StockQuantity: "12.500", StockUnit: "Tabletten"})
	if err != nil || medication.StockQuantity != "12.500" {
		t.Fatalf("medication=%#v err=%v", medication, err)
	}
	medications, err := service.ListPetMedications(ctx, alice, created.ID, false, 100)
	if err != nil || len(medications.Medications) != 1 {
		t.Fatalf("medications=%#v err=%v", medications, err)
	}
	if _, err := service.ListPetMedications(ctx, bob, created.ID, false, 100); !errors.Is(err, pets.ErrNotFound) {
		t.Fatalf("cross-household medication read err=%v", err)
	}
	shoppingService := shopping.NewService(shopping.NewPostgresRepository(pool))
	lists, err := shoppingService.Lists(ctx, alice)
	if err != nil || len(lists) != 1 || lists[0].ID != shopping.SharedListID {
		t.Fatalf("lists=%#v err=%v", lists, err)
	}
	petItem, err := shoppingService.Create(ctx, shopping.CreateItem{Name: "Katzenfutter", Quantity: "2", PetIDs: []string{created.ID}}, alice)
	if err != nil || len(petItem.PetIDs) != 1 {
		t.Fatalf("pet item=%#v err=%v", petItem, err)
	}
	filtered, err := shoppingService.List(ctx, alice, "shared", false, created.ID, 100)
	if err != nil || len(filtered) != 1 {
		t.Fatalf("filtered=%#v err=%v", filtered, err)
	}
	foreignItems, err := shoppingService.List(ctx, bob, "shared", true, "", 100)
	if err != nil || len(foreignItems) != 0 {
		t.Fatalf("foreign items=%#v err=%v", foreignItems, err)
	}
}

func TestAuthenticationAndShoppingPersistence(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	users := [2]auth.BootstrapUser{
		{Username: "alice", DisplayName: "Alice", Password: "correct horse battery staple", Role: "admin"},
		{Username: "bob", DisplayName: "Bob", Password: "another secure household password", Role: "member"},
	}
	if err := auth.Bootstrap(ctx, pool, users); err != nil {
		t.Fatal(err)
	}
	var userCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&userCount); err != nil || userCount != 2 {
		t.Fatalf("user count=%d err=%v", userCount, err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users(id,username,display_name,password_hash,role)
        VALUES('00000000-0000-4000-8000-000000000099','third','Third','not-used','member')`); err == nil {
		t.Fatal("database accepted a third user")
	}

	authService := auth.NewPostgresService(pool)
	pair, err := authService.Login(ctx, auth.Credentials{Username: "alice", Password: users[0].Password}, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	principal, err := authService.Authenticate(ctx, pair.AccessToken)
	if err != nil || principal.Username != "alice" {
		t.Fatalf("principal=%#v err=%v", principal, err)
	}
	rotated, err := authService.Refresh(ctx, pair.RefreshToken, "127.0.0.1")
	if err != nil || rotated.RefreshToken == pair.RefreshToken {
		t.Fatalf("refresh did not rotate: err=%v", err)
	}
	if _, err := authService.Authenticate(ctx, pair.AccessToken); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("old access token remained valid: %v", err)
	}
	if _, err := authService.Refresh(ctx, pair.RefreshToken, "127.0.0.1"); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("refresh token reuse accepted: %v", err)
	}
	if _, err := authService.Authenticate(ctx, rotated.AccessToken); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("reuse detection did not revoke token family: %v", err)
	}

	service := shopping.NewService(shopping.NewPostgresRepository(pool))
	item, err := service.Create(ctx, shopping.CreateItem{Name: "Milk", Quantity: "2", Unit: "litres"}, principal.UserID)
	if err != nil {
		t.Fatal(err)
	}
	items, err := service.List(ctx, principal.UserID, "shared", false, "", 100)
	if err != nil || len(items) != 1 || items[0].ID != item.ID {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	if _, err := service.Complete(ctx, item.ID, principal.UserID); err != nil {
		t.Fatal(err)
	}

	expenseService := expenses.NewService(expenses.NewPostgresRepository(pool))
	expense, err := expenseService.Create(ctx, expenses.CreateExpense{Description: "Groceries", PlannedAmountCents: 12000, DueDate: "2026-08-31", Status: expenses.StatusOpen}, principal.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := expenseService.MarkPaid(ctx, expense.ID, principal.UserID, expenses.MarkPaid{ActualAmountCents: 11500}); err != nil {
		t.Fatal(err)
	}
	summary, err := expenseService.MonthlySummary(ctx, principal.UserID, "2026-08")
	if err != nil || summary.PlannedAmountCents != 12000 || summary.ActualAmountCents != 11500 {
		t.Fatalf("summary=%#v err=%v", summary, err)
	}

	storageRoot := t.TempDir()
	storage, err := receipts.NewLocalStorage(storageRoot)
	if err != nil {
		t.Fatal(err)
	}
	receiptService := receipts.NewService(receipts.NewPostgresRepository(pool), expenseService, storage)
	upload, err := os.CreateTemp(t.TempDir(), "receipt")
	if err != nil {
		t.Fatal(err)
	}
	defer upload.Close()
	if _, err := upload.Write([]byte("%PDF-1.7\nlocal test receipt")); err != nil {
		t.Fatal(err)
	}
	if _, err := upload.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	total := int64(11500)
	receipt, err := receiptService.Upload(ctx, expense.ID, principal.UserID, upload, "../../receipt.pdf", receipts.Metadata{TotalAmountCents: &total})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.OriginalFilename != "receipt.pdf" || receipt.SHA256 == "" {
		t.Fatalf("receipt=%#v", receipt)
	}
	if _, err := os.Stat(filepath.Join(storageRoot, receipt.StorageKey)); err != nil {
		t.Fatal(err)
	}
	listed, err := receiptService.List(ctx, principal.UserID, expense.ID)
	if err != nil || len(listed) != 1 {
		t.Fatalf("receipts=%#v err=%v", listed, err)
	}

	metadata, body, err := receiptService.Content(ctx, principal.UserID, expense.ID, receipt.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	if metadata.MIMEType != "application/pdf" || metadata.SHA256 != receipt.SHA256 {
		t.Fatalf("metadata=%#v", metadata)
	}
	content, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "%PDF-1.7\nlocal test receipt" {
		t.Fatalf("content=%q", content)
	}

	// A receipt must not be readable through a different expense, and that
	// isolation has to hold in the SQL, not only in the service.
	other, err := expenseService.Create(ctx, expenses.CreateExpense{Description: "Water", PlannedAmountCents: 4200, DueDate: "2026-08-31", Status: expenses.StatusOpen}, principal.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := receiptService.Content(ctx, principal.UserID, other.ID, receipt.ID); !errors.Is(err, receipts.ErrNotFound) {
		t.Fatalf("cross-expense read err=%v, want ErrNotFound", err)
	}
	if _, _, err := receiptService.Content(ctx, principal.UserID, expense.ID, "00000000-0000-4000-8000-0000000000ff"); !errors.Is(err, receipts.ErrNotFound) {
		t.Fatalf("unknown receipt err=%v, want ErrNotFound", err)
	}
}

// TestCalendarPersistence exercises the calendar SQL against a real schema:
// the visibility rule lives in the WHERE clauses, so a fake repository cannot
// prove it.
func TestCalendarPersistence(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	if err := auth.Bootstrap(ctx, pool, [2]auth.BootstrapUser{
		{Username: "alice", DisplayName: "Alice", Password: "correct horse battery staple", Role: "admin"},
		{Username: "bob", DisplayName: "Bob", Password: "another secure household password", Role: "member"},
	}); err != nil {
		t.Fatal(err)
	}
	userID := func(username string) string {
		var value string
		if err := pool.QueryRow(ctx, `SELECT id::text FROM users WHERE username=$1`, username).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	alice, bob := userID("alice"), userID("bob")

	service := calendar.NewService(calendar.NewPostgresRepository(pool))
	instant := func(day, hour int) time.Time { return time.Date(2026, 9, day, hour, 0, 0, 0, time.UTC) }
	september := calendar.Range{From: instant(1, 0), To: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}

	reminder := 30
	shared, err := service.Create(ctx, calendar.EventInput{
		Title: "Müllabfuhr", Description: "Biotonne", StartsAt: instant(10, 7), EndsAt: instant(10, 8),
		Category: "Haushalt", RecurrenceRule: "FREQ=WEEKLY;BYDAY=MO", ReminderMinutes: &reminder,
	}, alice)
	if err != nil {
		t.Fatal(err)
	}
	if shared.RecurrenceRule != "FREQ=WEEKLY;BYDAY=MO" || shared.ReminderMinutes == nil || *shared.ReminderMinutes != 30 {
		t.Fatalf("event=%#v", shared)
	}
	// An event without a rule must round-trip as an empty string, not as a
	// scan error on a NULL column.
	plain, err := service.Create(ctx, calendar.EventInput{Title: "Kino", StartsAt: instant(12, 20), EndsAt: instant(12, 22)}, bob)
	if err != nil {
		t.Fatal(err)
	}
	if plain.RecurrenceRule != "" || plain.ReminderMinutes != nil {
		t.Fatalf("event=%#v", plain)
	}
	private, err := service.Create(ctx, calendar.EventInput{
		Title: "Überraschung", StartsAt: instant(20, 18), EndsAt: instant(20, 19), Visibility: calendar.VisibilityPrivate,
	}, alice)
	if err != nil {
		t.Fatal(err)
	}

	fromAlice, err := service.List(ctx, alice, september)
	if err != nil || len(fromAlice) != 3 {
		t.Fatalf("alice sees %d events, want 3 (err=%v)", len(fromAlice), err)
	}
	fromBob, err := service.List(ctx, bob, september)
	if err != nil || len(fromBob) != 2 {
		t.Fatalf("bob sees %d events, want 2 (err=%v)", len(fromBob), err)
	}
	for _, event := range fromBob {
		if event.ID == private.ID {
			t.Fatal("private event of the other member appeared in the list")
		}
	}
	if fromAlice[0].StartsAt.After(fromAlice[1].StartsAt) {
		t.Fatalf("list is not ordered by start: %v", fromAlice)
	}

	if _, err := service.Get(ctx, bob, private.ID); !errors.Is(err, calendar.ErrNotFound) {
		t.Fatalf("get err=%v, want ErrNotFound", err)
	}
	if _, err := service.Update(ctx, bob, private.ID, calendar.EventInput{
		Title: "Geklaut", StartsAt: instant(20, 18), EndsAt: instant(20, 19),
	}); !errors.Is(err, calendar.ErrNotFound) {
		t.Fatalf("update err=%v, want ErrNotFound", err)
	}
	if err := service.Delete(ctx, bob, private.ID); !errors.Is(err, calendar.ErrNotFound) {
		t.Fatalf("delete err=%v, want ErrNotFound", err)
	}

	updated, err := service.Update(ctx, bob, shared.ID, calendar.EventInput{
		Title: "Müllabfuhr verschoben", StartsAt: instant(11, 7), EndsAt: instant(11, 8), Status: calendar.StatusCompleted,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.OwnerID != alice {
		t.Fatalf("ownerId=%q, an update must not transfer ownership", updated.OwnerID)
	}
	if !updated.CreatedAt.Equal(shared.CreatedAt) || !updated.UpdatedAt.After(shared.UpdatedAt) {
		t.Fatalf("timestamps created=%v/%v updated=%v/%v", shared.CreatedAt, updated.CreatedAt, shared.UpdatedAt, updated.UpdatedAt)
	}
	if updated.RecurrenceRule != "" || updated.ReminderMinutes != nil {
		t.Fatalf("update did not clear the omitted fields: %#v", updated)
	}

	// An event that only overlaps the window still belongs in the result.
	crossing, err := service.Create(ctx, calendar.EventInput{
		Title: "Urlaub", StartsAt: time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC), EndsAt: instant(5, 12),
	}, alice)
	if err != nil {
		t.Fatal(err)
	}
	overlapping, err := service.List(ctx, bob, september)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range overlapping {
		if event.ID == crossing.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("event crossing the window start was not listed")
	}

	if err := service.Delete(ctx, alice, private.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(ctx, alice, private.ID); !errors.Is(err, calendar.ErrNotFound) {
		t.Fatalf("event survived deletion: %v", err)
	}
}

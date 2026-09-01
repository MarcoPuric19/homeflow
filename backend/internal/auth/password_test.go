package auth

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "correct horse battery staple" {
		t.Fatal("password was stored in plaintext")
	}
	ok, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil || !ok {
		t.Fatalf("valid password rejected: ok=%v err=%v", ok, err)
	}
	ok, err = VerifyPassword("wrong password", hash)
	if err != nil || ok {
		t.Fatalf("invalid password accepted: ok=%v err=%v", ok, err)
	}
}

func TestPasswordMinimumLength(t *testing.T) {
	if _, err := HashPassword("too-short"); err == nil {
		t.Fatal("short password accepted")
	}
}

// The password work has to fit in the container alongside everything else the
// process needs. Two concurrent verifications did not: they drove the pod to
// 191.8 MiB of its 192 MiB limit and the kernel killed it (HF-0086, HF-0087).
//
// This guards the product, not either factor on its own - raising the
// concurrency or the Argon2 memory without moving the limit reintroduces the
// same kill, and neither change looks dangerous in isolation.
func TestPasswordWorkFitsInTheContainerLimit(t *testing.T) {
	const containerLimitMiB = 192 // kubernetes/base/api-deployment.yaml
	// Measured at the kill: 190 MiB anonymous, of which 128 MiB was Argon2.
	// The rest is the Go heap, the pool and the HTTP machinery.
	const processOverheadMiB = 64

	peakMiB := passwordConcurrency * (argonMemory / 1024)
	if peakMiB+processOverheadMiB >= containerLimitMiB {
		t.Fatalf("password work peaks at %d MiB, plus %d MiB overhead, against a %d MiB limit: "+
			"lower passwordConcurrency or argonMemory, or raise the limit and contracts.md 2.1 together",
			peakMiB, processOverheadMiB, containerLimitMiB)
	}
}

// The slot has to bound how much Argon2 memory exists at one moment.
//
// What this does NOT prove is that the slot sits in the right place. It would
// pass just as well for the old code, which held the slot across the database
// query too. Proving the placement needs a fake pool, and the service takes a
// concrete *pgxpool.Pool - so that stays a reading exercise for now.
func TestPasswordSlotBoundsConcurrentWork(t *testing.T) {
	service := &PostgresService{passwordSlots: make(chan struct{}, passwordConcurrency)}
	var inFlight, peak atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := service.withPasswordSlot(context.Background(), func() {
				now := inFlight.Add(1)
				for {
					high := peak.Load()
					if now <= high || peak.CompareAndSwap(high, now) {
						break
					}
				}
				time.Sleep(2 * time.Millisecond)
				inFlight.Add(-1)
			})
			if err != nil {
				t.Errorf("withPasswordSlot: %v", err)
			}
		}()
	}
	wg.Wait()
	if got := peak.Load(); got > passwordConcurrency {
		t.Fatalf("%d Argon2 runs overlapped, passwordConcurrency is %d", got, passwordConcurrency)
	}
}

// Waiting for a slot must not outlive the request. Without this the queue in
// front of a busy hasher turns every timeout into a leaked goroutine.
func TestPasswordSlotHonoursContextCancellation(t *testing.T) {
	service := &PostgresService{passwordSlots: make(chan struct{}, passwordConcurrency)}
	occupied := make(chan struct{})
	released := make(chan struct{})
	go func() {
		_ = service.withPasswordSlot(context.Background(), func() {
			close(occupied)
			<-released
		})
	}()
	<-occupied
	defer close(released)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var ran bool
	err := service.withPasswordSlot(ctx, func() { ran = true })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if ran {
		t.Fatal("work ran without a slot")
	}
}

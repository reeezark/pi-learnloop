package daemon

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/reeezark/pi-learnloop/internal/evidence"
)

func TestContinuationStoreIsSingleUseAndOwnsRetainedResult(t *testing.T) {
	store := testContinuationStore()
	result := continuationTestResult("original")
	descriptor, err := store.retain(result)
	if err != nil {
		t.Fatalf("retain(): %v", err)
	}
	if !descriptor.Available || !validContinuationID(descriptor.ID) {
		t.Fatalf("descriptor = %#v, want an available pc1 continuation", descriptor)
	}
	result.Files[0].Declarations[0].Excerpt = "mutated"

	retained, ok := store.consume(descriptor.ID)
	if !ok || retained.result.Files[0].Declarations[0].Excerpt != "original" || retained.piSessionID != "" {
		t.Fatalf("consume() = (%#v, %t), want the retained immutable value", retained, ok)
	}
	if _, ok := store.consume(descriptor.ID); ok {
		t.Fatal("second consume succeeded, want single use")
	}
}

func TestContinuationStoreOwnsPiSessionProvenanceSeparately(t *testing.T) {
	store := testContinuationStore()
	result := continuationTestResult("original")
	descriptor, err := store.retainWithPiSession(result, "session-123")
	if err != nil {
		t.Fatalf("retainWithPiSession(): %v", err)
	}
	result.Files[0].Declarations[0].Excerpt = "mutated"

	retained, ok := store.consume(descriptor.ID)
	if !ok || retained.result.Files[0].Declarations[0].Excerpt != "original" || retained.piSessionID != "session-123" {
		t.Fatalf("consume() = (%#v, %t), want owned evidence and separate Session provenance", retained, ok)
	}
}

func TestContinuationStoreRejectsInvalidPiSessionWithoutEcho(t *testing.T) {
	store := testContinuationStore()
	invalid := "private/session"
	descriptor, err := store.retainWithPiSession(continuationTestResult("evidence"), invalid)
	if err == nil || strings.Contains(err.Error(), invalid) {
		t.Fatalf("retainWithPiSession(invalid) = (%#v, %v), want safe error", descriptor, err)
	}
	if len(store.entries) != 0 || store.retainedBytes != 0 {
		t.Fatalf("invalid Session provenance changed store state: %#v", store)
	}
}

func TestContinuationStoreExpiresAndReleasesCapacity(t *testing.T) {
	store := testContinuationStore()
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	descriptor, err := store.retain(continuationTestResult("evidence"))
	if err != nil {
		t.Fatalf("retain(): %v", err)
	}
	now = now.Add(continuationLifetime)
	if _, ok := store.consume(descriptor.ID); ok {
		t.Fatal("consume at expiry succeeded, want expiration")
	}
	if len(store.entries) != 0 || store.retainedBytes != 0 {
		t.Fatalf("expired store state = (%d entries, %d bytes), want empty", len(store.entries), store.retainedBytes)
	}

	store = testContinuationStore()
	now = time.Date(2026, 9, 1, 12, 0, 0, 500_000_000, time.UTC)
	store.now = func() time.Time { return now }
	descriptor, err = store.retain(continuationTestResult("evidence"))
	if err != nil {
		t.Fatalf("retain(fractional clock): %v", err)
	}
	declaredExpiry, err := time.Parse(time.RFC3339Nano, descriptor.ExpiresAt)
	if err != nil {
		t.Fatalf("Parse(expires_at): %v", err)
	}
	now = declaredExpiry
	if _, ok := store.consume(descriptor.ID); ok {
		t.Fatal("consume at the declared expires_at succeeded, want expiration")
	}
}

func TestContinuationStoreRejectsLiveCapacityWithoutEviction(t *testing.T) {
	store := testContinuationStore()
	ids := make([]string, 0, maxLiveContinuations)
	for index := 0; index < maxLiveContinuations; index++ {
		descriptor, err := store.retain(continuationTestResult(fmt.Sprintf("evidence-%d", index)))
		if err != nil {
			t.Fatalf("retain(%d): %v", index, err)
		}
		ids = append(ids, descriptor.ID)
	}
	descriptor, err := store.retain(continuationTestResult("over capacity"))
	if err != nil {
		t.Fatalf("retain(over capacity): %v", err)
	}
	if descriptor.Available || descriptor.Reason != "capacity" {
		t.Fatalf("descriptor = %#v, want unavailable capacity", descriptor)
	}
	for _, id := range ids {
		if _, ok := store.consume(id); !ok {
			t.Fatalf("live continuation %q was evicted", id)
		}
	}

	store = testContinuationStore()
	descriptor, err = store.retain(continuationTestResult(strings.Repeat("x", maxRetainedExcerptBytes+1)))
	if err != nil {
		t.Fatalf("retain(oversized): %v", err)
	}
	if descriptor.Available || descriptor.Reason != "capacity" || len(store.entries) != 0 {
		t.Fatalf("byte-cap descriptor/state = (%#v, %d), want capacity without retention", descriptor, len(store.entries))
	}
}

func TestContinuationStoreConcurrentConsumeHasOneWinner(t *testing.T) {
	store := testContinuationStore()
	descriptor, err := store.retain(continuationTestResult("evidence"))
	if err != nil {
		t.Fatalf("retain(): %v", err)
	}
	var winners atomic.Int32
	var wait sync.WaitGroup
	for index := 0; index < 16; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if _, ok := store.consume(descriptor.ID); ok {
				winners.Add(1)
			}
		}()
	}
	wait.Wait()
	if winners.Load() != 1 {
		t.Fatalf("consume winners = %d, want 1", winners.Load())
	}
}

func TestContinuationStoreReportsInsufficientEvidenceAndClears(t *testing.T) {
	store := testContinuationStore()
	descriptor, err := store.retain(evidence.Result{})
	if err != nil {
		t.Fatalf("retain(empty): %v", err)
	}
	if descriptor.Available || descriptor.Reason != "insufficient_evidence" {
		t.Fatalf("descriptor = %#v, want insufficient_evidence", descriptor)
	}
	available, err := store.retain(continuationTestResult("evidence"))
	if err != nil {
		t.Fatalf("retain(): %v", err)
	}
	store.clear()
	if _, ok := store.consume(available.ID); ok || store.retainedBytes != 0 {
		t.Fatal("cleared continuation remained available")
	}
}

func testContinuationStore() *continuationStore {
	store := newContinuationStore()
	var sequence atomic.Int64
	store.newID = func() (string, error) {
		value := fmt.Sprintf("%043d", sequence.Add(1))
		return "pc1-" + value, nil
	}
	return store
}

func continuationTestResult(excerpt string) evidence.Result {
	return evidence.Result{
		Files: []evidence.File{{
			Declarations: []evidence.Declaration{{Excerpt: excerpt}},
		}},
	}
}

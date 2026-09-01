package daemon

import (
	"fmt"
	"sync"
	"time"

	"github.com/reeezark/pi-learnloop/internal/evidence"
)

const (
	continuationLifetime        = 5 * time.Minute
	maxLiveContinuations        = 8
	maxRetainedExcerptBytes     = 1024 * 1024
	continuationIdentifierBytes = 32
)

type continuationDescriptor struct {
	Available bool   `json:"available"`
	ID        string `json:"id,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type continuationEntry struct {
	result       evidence.Result
	expiresAt    time.Time
	excerptBytes int
}

type continuationStore struct {
	mu            sync.Mutex
	entries       map[string]continuationEntry
	retainedBytes int
	now           func() time.Time
	newID         func() (string, error)
}

func newContinuationStore() *continuationStore {
	return &continuationStore{
		entries: make(map[string]continuationEntry),
		now:     time.Now,
		newID: func() (string, error) {
			value, err := randomID(continuationIdentifierBytes)
			if err != nil {
				return "", err
			}
			return "pc1-" + value, nil
		},
	}
}

func (store *continuationStore) retain(result evidence.Result) (continuationDescriptor, error) {
	bytes := resultExcerptBytes(result)
	if bytes == 0 {
		return continuationDescriptor{Available: false, Reason: "insufficient_evidence"}, nil
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	now := store.now().UTC()
	store.purgeExpired(now)
	if len(store.entries) >= maxLiveContinuations || store.retainedBytes+bytes > maxRetainedExcerptBytes {
		return continuationDescriptor{Available: false, Reason: "capacity"}, nil
	}

	var id string
	for attempts := 0; attempts < 3; attempts++ {
		candidate, err := store.newID()
		if err != nil {
			return continuationDescriptor{}, fmt.Errorf("generate continuation ID: %w", err)
		}
		if _, exists := store.entries[candidate]; !exists {
			id = candidate
			break
		}
	}
	if id == "" {
		return continuationDescriptor{}, fmt.Errorf("generate unique continuation ID")
	}

	expiresAt := now.Add(continuationLifetime)
	store.entries[id] = continuationEntry{
		result:       cloneEvidenceResult(result),
		expiresAt:    expiresAt,
		excerptBytes: bytes,
	}
	store.retainedBytes += bytes
	return continuationDescriptor{
		Available: true,
		ID:        id,
		ExpiresAt: expiresAt.Format(time.RFC3339Nano),
	}, nil
}

func (store *continuationStore) consume(id string) (evidence.Result, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.purgeExpired(store.now().UTC())
	entry, exists := store.entries[id]
	if !exists {
		return evidence.Result{}, false
	}
	delete(store.entries, id)
	store.retainedBytes -= entry.excerptBytes
	return entry.result, true
}

func (store *continuationStore) clear() {
	store.mu.Lock()
	defer store.mu.Unlock()
	clear(store.entries)
	store.retainedBytes = 0
}

func (store *continuationStore) purgeExpired(now time.Time) {
	for id, entry := range store.entries {
		if now.Before(entry.expiresAt) {
			continue
		}
		delete(store.entries, id)
		store.retainedBytes -= entry.excerptBytes
	}
}

func resultExcerptBytes(result evidence.Result) int {
	total := 0
	for _, file := range result.Files {
		for _, declaration := range file.Declarations {
			total += len(declaration.Excerpt)
		}
	}
	return total
}

func cloneEvidenceResult(value evidence.Result) evidence.Result {
	result := value
	result.Files = make([]evidence.File, len(value.Files))
	for fileIndex, file := range value.Files {
		result.Files[fileIndex] = file
		result.Files[fileIndex].ChangedLines = append([]evidence.LineRange(nil), file.ChangedLines...)
		result.Files[fileIndex].Omissions = append([]evidence.Omission(nil), file.Omissions...)
		result.Files[fileIndex].Declarations = make([]evidence.Declaration, len(file.Declarations))
		for declarationIndex, declaration := range file.Declarations {
			result.Files[fileIndex].Declarations[declarationIndex] = declaration
			result.Files[fileIndex].Declarations[declarationIndex].ChangedLines = append([]evidence.LineRange(nil), declaration.ChangedLines...)
		}
	}
	return result
}

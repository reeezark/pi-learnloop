package daemon

import (
	"fmt"
	"sync"
	"time"

	"github.com/reeezark/pi-learnloop/internal/evidence"
	"github.com/reeezark/pi-learnloop/internal/history"
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
	value        continuationValue
	expiresAt    time.Time
	excerptBytes int
}

type continuationValue struct {
	result      evidence.Result
	piSessionID string
	contract    evidenceContract
}

type evidenceContract uint8

const (
	evidenceContractV1 evidenceContract = iota + 1
	evidenceContractV2
)

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
	return store.retainValue(result, "", evidenceContractV1)
}

func (store *continuationStore) retainWithPiSession(result evidence.Result, piSessionID string) (continuationDescriptor, error) {
	if !history.ValidPiSessionID(piSessionID) {
		return continuationDescriptor{}, fmt.Errorf("retain Pi Session provenance: invalid identity")
	}
	return store.retainValue(result, piSessionID, evidenceContractV1)
}

func (store *continuationStore) retainGoContext(result evidence.Result) (continuationDescriptor, error) {
	return store.retainGoContextValue(result, "")
}

func (store *continuationStore) retainGoContextWithPiSession(result evidence.Result, piSessionID string) (continuationDescriptor, error) {
	if !history.ValidPiSessionID(piSessionID) {
		return continuationDescriptor{}, fmt.Errorf("retain Pi Session provenance: invalid identity")
	}
	return store.retainGoContextValue(result, piSessionID)
}

func (store *continuationStore) retainGoContextValue(result evidence.Result, piSessionID string) (continuationDescriptor, error) {
	if result.GoContext == nil {
		return continuationDescriptor{}, fmt.Errorf("retain Go context: missing enriched evidence")
	}
	return store.retainValue(result, piSessionID, evidenceContractV2)
}

func (store *continuationStore) retainValue(result evidence.Result, piSessionID string, contract evidenceContract) (continuationDescriptor, error) {
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
		value: continuationValue{
			result:      cloneEvidenceResult(result),
			piSessionID: piSessionID,
			contract:    contract,
		},
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

func (store *continuationStore) consume(id string) (continuationValue, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.purgeExpired(store.now().UTC())
	entry, exists := store.entries[id]
	if !exists {
		return continuationValue{}, false
	}
	delete(store.entries, id)
	store.retainedBytes -= entry.excerptBytes
	return entry.value, true
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
	if result.GoContext != nil {
		total += result.GoContext.ApproximateBytes
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
	if value.GoContext != nil {
		contextValue := *value.GoContext
		contextValue.Build.BuildTags = cloneContinuationSlice(value.GoContext.Build.BuildTags)
		contextValue.Build.ToolTags = cloneContinuationSlice(value.GoContext.Build.ToolTags)
		contextValue.Build.ReleaseTags = cloneContinuationSlice(value.GoContext.Build.ReleaseTags)
		contextValue.Build.Modules = cloneContinuationSlice(value.GoContext.Build.Modules)
		contextValue.Build.Workspaces = cloneContinuationSlice(value.GoContext.Build.Workspaces)
		contextValue.Build.Replacements = cloneContinuationSlice(value.GoContext.Build.Replacements)
		contextValue.Items = cloneContinuationSlice(value.GoContext.Items)
		contextValue.Relations = cloneContinuationSlice(value.GoContext.Relations)
		contextValue.Omissions = cloneContinuationSlice(value.GoContext.Omissions)
		result.GoContext = &contextValue
	}
	return result
}

func cloneContinuationSlice[T any](value []T) []T {
	if value == nil {
		return nil
	}
	result := make([]T, len(value))
	copy(result, value)
	return result
}

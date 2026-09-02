package history

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	recordIDPrefix         = "lr1-"
	recordIDRandomBytes    = 32
	maxRepositoryRootLen   = 4096
	maxRevisionLen         = 256
	maxPromptIdentityLen   = 128
	maxPromptVersionLen    = 64
	maxProviderLen         = 128
	maxModelIDLen          = 256
	maxPiSessionIDBytes    = 128
	maxPiSessionCandidates = 20
)

func ValidRecordID(value string) bool {
	if !strings.HasPrefix(value, recordIDPrefix) {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, recordIDPrefix))
	return err == nil && len(decoded) == recordIDRandomBytes
}

func ValidPiSessionID(value string) bool {
	if len(value) == 0 || len(value) > maxPiSessionIDBytes ||
		!asciiAlphaNumeric(value[0]) || !asciiAlphaNumeric(value[len(value)-1]) {
		return false
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if !asciiAlphaNumeric(character) && character != '.' && character != '_' && character != '-' {
			return false
		}
	}
	return true
}

func asciiAlphaNumeric(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
}

func validateStart(value Start) error {
	if !validCanonicalRoot(value.CanonicalRoot) {
		return fmt.Errorf("%w: canonical repository root is invalid", ErrInvalid)
	}
	if err := validateTimestamp("start", value.StartedAt); err != nil {
		return err
	}
	if !validBoundedValue(value.BaseRevision, maxRevisionLen) || !validBoundedValue(value.HeadRevision, maxRevisionLen) {
		return fmt.Errorf("%w: revision is invalid", ErrInvalid)
	}
	if !validLowerSHA256(value.EvidenceManifestSHA256) {
		return fmt.Errorf("%w: evidence manifest hash is invalid", ErrInvalid)
	}
	if value.QuestionSchemaVersion <= 0 || value.AssessmentSchemaVersion <= 0 {
		return fmt.Errorf("%w: schema version is invalid", ErrInvalid)
	}
	if err := validatePrompt("question", value.QuestionPrompt); err != nil {
		return err
	}
	if err := validatePrompt("assessment", value.AssessmentPrompt); err != nil {
		return err
	}
	if value.PiVersion != "0.84.3" {
		return fmt.Errorf("%w: Pi version is unsupported", ErrInvalid)
	}
	if !validArgumentValue(value.Provider, maxProviderLen) || !validArgumentValue(value.ModelID, maxModelIDLen) {
		return fmt.Errorf("%w: model selection is invalid", ErrInvalid)
	}
	switch value.ThinkingLevel {
	case "off", "minimal", "low", "medium", "high", "xhigh", "max":
	default:
		return fmt.Errorf("%w: thinking level is invalid", ErrInvalid)
	}
	return nil
}

func validCanonicalRoot(value string) bool {
	return value != "" && len(value) <= maxRepositoryRootLen && utf8.ValidString(value) &&
		filepath.IsAbs(value) && filepath.Clean(value) == value
}

func validateCompletion(startedAt time.Time, value Completion) error {
	if err := validateTerminalTimestamp(startedAt, value.FinishedAt); err != nil {
		return err
	}
	switch value.Label {
	case LabelUnderstood, LabelPartial, LabelReviewNeeded:
	default:
		return fmt.Errorf("%w: completion label is invalid", ErrInvalid)
	}
	if len(value.Outcomes) != 3 {
		return fmt.Errorf("%w: completion must contain exactly Q1, Q2, and Q3", ErrInvalid)
	}
	wantIDs := []string{"Q1", "Q2", "Q3"}
	wantKinds := []QuestionKind{QuestionKindCodeSpecific, QuestionKindCodeSpecific, QuestionKindGoBackend}
	for index, outcome := range value.Outcomes {
		if outcome.QuestionID != wantIDs[index] || outcome.QuestionKind != wantKinds[index] {
			return fmt.Errorf("%w: question outcome %d is invalid", ErrInvalid, index+1)
		}
		switch outcome.Verdict {
		case VerdictDemonstrated, VerdictPartial, VerdictNotDemonstrated:
		default:
			return fmt.Errorf("%w: question verdict is invalid", ErrInvalid)
		}
	}
	return nil
}

func validateFailure(startedAt time.Time, value Failure) error {
	if err := validateTerminalTimestamp(startedAt, value.FinishedAt); err != nil {
		return err
	}
	switch value.Code {
	case FailureEvaluatorFailed, FailureEvaluatorInvalidOutput, FailureEvaluatorTimeout:
		return nil
	default:
		return fmt.Errorf("%w: failure code is invalid", ErrInvalid)
	}
}

func validateRecord(record Record) error {
	if !ValidRecordID(record.RecordID) {
		return fmt.Errorf("%w: stored record identity is invalid", ErrInvalid)
	}
	if err := validateStart(record.Start); err != nil {
		return err
	}
	switch record.Status {
	case StatusRunning:
		if record.FinishedAt != nil || record.FailureCode != "" || record.Label != "" || len(record.Outcomes) != 0 {
			return fmt.Errorf("%w: running record has terminal data", ErrInvalid)
		}
	case StatusComplete:
		if record.FinishedAt == nil || record.FailureCode != "" {
			return fmt.Errorf("%w: complete record has invalid terminal data", ErrInvalid)
		}
		if err := validateCompletion(record.Start.StartedAt, Completion{
			FinishedAt: *record.FinishedAt,
			Label:      record.Label,
			Outcomes:   record.Outcomes,
		}); err != nil {
			return err
		}
	case StatusFailed:
		if record.FinishedAt == nil || record.Label != "" || len(record.Outcomes) != 0 {
			return fmt.Errorf("%w: failed record has invalid terminal data", ErrInvalid)
		}
		if err := validateFailure(record.Start.StartedAt, Failure{FinishedAt: *record.FinishedAt, Code: record.FailureCode}); err != nil {
			return err
		}
	case StatusInterrupted:
		if record.FinishedAt == nil || record.FailureCode != "" || record.Label != "" || len(record.Outcomes) != 0 {
			return fmt.Errorf("%w: interrupted record has invalid terminal data", ErrInvalid)
		}
		if err := validateTerminalTimestamp(record.Start.StartedAt, *record.FinishedAt); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%w: stored status is invalid", ErrInvalid)
	}
	return nil
}

func validatePrompt(name string, value PromptProvenance) error {
	if !validBoundedValue(value.ID, maxPromptIdentityLen) || !validBoundedValue(value.Version, maxPromptVersionLen) || !validLowerSHA256(value.SHA256) {
		return fmt.Errorf("%w: %s prompt provenance is invalid", ErrInvalid, name)
	}
	return nil
}

func validateTimestamp(name string, value time.Time) error {
	if value.IsZero() || value.UnixMilli() <= 0 || value.Nanosecond()%int(time.Millisecond) != 0 {
		return fmt.Errorf("%w: %s timestamp is invalid", ErrInvalid, name)
	}
	return nil
}

func validateTerminalTimestamp(startedAt, finishedAt time.Time) error {
	if err := validateTimestamp("finish", finishedAt); err != nil {
		return err
	}
	if finishedAt.Before(startedAt) {
		return fmt.Errorf("%w: finish timestamp precedes start", ErrInvalid)
	}
	return nil
}

func validBoundedValue(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && utf8.ValidString(value) && strings.TrimSpace(value) == value && !containsControl(value)
}

func validArgumentValue(value string, maximum int) bool {
	return validBoundedValue(value, maximum) && !strings.HasPrefix(value, "-")
}

func validLowerSHA256(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func containsControl(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}

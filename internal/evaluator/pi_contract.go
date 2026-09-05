package evaluator

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	SupportedPiVersion   = "0.84.3"
	MaxSystemPromptBytes = 64 * 1024
)

type ModelSelection struct {
	PiVersion     string
	Provider      string
	ModelID       string
	ThinkingLevel string
}

func validateSystemPrompt(systemPrompt string) error {
	if strings.TrimSpace(systemPrompt) == "" || !utf8.ValidString(systemPrompt) {
		return invalidInput(errors.New("system prompt must be non-empty valid UTF-8"))
	}
	if len(systemPrompt) > MaxSystemPromptBytes {
		return invalidInput(fmt.Errorf("system prompt exceeds %d bytes", MaxSystemPromptBytes))
	}
	return nil
}

// ValidateModelSelection applies the same non-secret Pi 0.84.3 model contract
// used by every evaluator adapter.
func ValidateModelSelection(selection ModelSelection) error {
	if selection.PiVersion != SupportedPiVersion {
		return invalidInput(fmt.Errorf("Pi version must be %s", SupportedPiVersion))
	}
	if err := validateArgumentValue("model provider", selection.Provider, 128); err != nil {
		return invalidInput(err)
	}
	if err := validateArgumentValue("model id", selection.ModelID, 256); err != nil {
		return invalidInput(err)
	}
	if !validThinkingLevel(selection.ThinkingLevel) {
		return invalidInput(errors.New("thinking level is unsupported"))
	}
	return nil
}

func validateArgumentValue(name, value string, maximum int) error {
	if strings.TrimSpace(value) == "" || !utf8.ValidString(value) {
		return fmt.Errorf("%s must be non-empty valid UTF-8", name)
	}
	if len(value) > maximum {
		return fmt.Errorf("%s exceeds %d bytes", name, maximum)
	}
	if strings.HasPrefix(value, "-") {
		return fmt.Errorf("%s must not begin with '-'", name)
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return fmt.Errorf("%s contains a control character", name)
		}
	}
	return nil
}

func validThinkingLevel(value string) bool {
	switch value {
	case "off", "minimal", "low", "medium", "high", "xhigh", "max":
		return true
	default:
		return false
	}
}

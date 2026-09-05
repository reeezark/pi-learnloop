package evaluator_test

import (
	"strings"
	"testing"

	"github.com/reeezark/pi-learnloop/internal/evaluator"
)

func TestValidateModelSelection(t *testing.T) {
	selection := evaluator.ModelSelection{
		PiVersion: evaluator.SupportedPiVersion, Provider: "openai", ModelID: "gpt-synthetic", ThinkingLevel: "off",
	}
	for _, level := range []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"} {
		t.Run("accepts thinking level "+level, func(t *testing.T) {
			current := selection
			current.ThinkingLevel = level
			if err := evaluator.ValidateModelSelection(current); err != nil {
				t.Fatalf("ValidateModelSelection() error = %v", err)
			}
		})
	}

	tests := []struct {
		name   string
		change func(*evaluator.ModelSelection)
	}{
		{name: "wrong Pi version", change: func(value *evaluator.ModelSelection) { value.PiVersion = "0.84.4" }},
		{name: "empty provider", change: func(value *evaluator.ModelSelection) { value.Provider = "" }},
		{name: "provider option injection", change: func(value *evaluator.ModelSelection) { value.Provider = "--mode" }},
		{name: "provider control character", change: func(value *evaluator.ModelSelection) { value.Provider = "openai\n--model" }},
		{name: "provider too long", change: func(value *evaluator.ModelSelection) { value.Provider = strings.Repeat("a", 129) }},
		{name: "empty model id", change: func(value *evaluator.ModelSelection) { value.ModelID = " " }},
		{name: "model option injection", change: func(value *evaluator.ModelSelection) { value.ModelID = "--no-tools" }},
		{name: "model too long", change: func(value *evaluator.ModelSelection) { value.ModelID = strings.Repeat("a", 257) }},
		{name: "unknown thinking level", change: func(value *evaluator.ModelSelection) { value.ThinkingLevel = "ultra" }},
	}
	for _, test := range tests {
		t.Run("rejects "+test.name, func(t *testing.T) {
			current := selection
			test.change(&current)
			err := evaluator.ValidateModelSelection(current)
			if code := evaluator.ContractErrorCodeOf(err); code != evaluator.ContractErrorInvalidInput {
				t.Fatalf("ContractErrorCodeOf(error) = %q, want %q (error = %v)", code, evaluator.ContractErrorInvalidInput, err)
			}
		})
	}
}

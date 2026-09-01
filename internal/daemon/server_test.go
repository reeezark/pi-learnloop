package daemon

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/reeezark/pi-learnloop/internal/evaluator"
	"github.com/reeezark/pi-learnloop/internal/evidence"
)

func TestQuestionSetConsumesBeforeBundleFailure(t *testing.T) {
	store := newContinuationStore()
	descriptor, err := store.retain(evidence.Result{
		Files: []evidence.File{{
			Declarations: []evidence.Declaration{{Excerpt: "retained but structurally invalid"}},
		}},
	})
	if err != nil {
		t.Fatalf("retain(): %v", err)
	}
	handler := newHandler("instance", "127.0.0.1:43210", "token", serverServices{
		continuations:     store,
		questionEvaluator: evaluator.DeterministicEvaluator{},
	})
	body, err := json.Marshal(map[string]any{
		"continuation_id": descriptor.ID,
		"pi_version":      "0.84.3",
		"model": map[string]string{
			"provider":       "provider",
			"id":             "model",
			"thinking_level": "off",
		},
	})
	if err != nil {
		t.Fatalf("Marshal(request): %v", err)
	}

	first := serveQuestionRequest(handler, body)
	if first.Code != http.StatusBadGateway {
		t.Fatalf("first status = %d, want %d; body = %s", first.Code, http.StatusBadGateway, first.Body.String())
	}
	var firstError struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &firstError); err != nil || firstError.Error.Code != "evaluator_failed" {
		t.Fatalf("first error = (%#v, %v), want evaluator_failed", firstError, err)
	}

	second := serveQuestionRequest(handler, body)
	if second.Code != http.StatusConflict {
		t.Fatalf("second status = %d, want %d; body = %s", second.Code, http.StatusConflict, second.Body.String())
	}
}

func serveQuestionRequest(handler http.Handler, body []byte) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:43210/v1/question-sets", bytes.NewReader(body))
	request.Host = "127.0.0.1:43210"
	request.RemoteAddr = "127.0.0.1:54321"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "PiLearnLoop token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

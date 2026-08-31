package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunRejectsUnsupportedInvocation(t *testing.T) {
	for _, arguments := range [][]string{nil, {"unknown"}, {"daemon", "--port", "8080"}} {
		var stderr bytes.Buffer
		if code := run(context.Background(), arguments, &stderr); code != 2 {
			t.Errorf("run(%q) code = %d, want 2", arguments, code)
		}
		if output := stderr.String(); !strings.Contains(output, "usage: pi-learnloop daemon") {
			t.Errorf("run(%q) stderr = %q, want usage", arguments, output)
		}
	}
}

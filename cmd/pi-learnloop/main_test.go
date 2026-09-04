package main

import (
	"bytes"
	"context"
	"testing"
)

func TestRunRejectsUnsupportedInvocation(t *testing.T) {
	for _, arguments := range [][]string{nil, {"unknown"}, {"daemon", "--port", "8080"}, {"--version"}, {"version", "extra"}} {
		var stdout, stderr bytes.Buffer
		if code := run(context.Background(), arguments, &stdout, &stderr); code != 2 {
			t.Errorf("run(%q) code = %d, want 2", arguments, code)
		}
		if output := stderr.String(); output != "usage: pi-learnloop daemon | version\n" {
			t.Errorf("run(%q) stderr = %q, want usage", arguments, output)
		}
		if stdout.Len() != 0 {
			t.Errorf("run(%q) stdout = %q, want empty", arguments, stdout.String())
		}
	}
}

func TestRunDaemonFailure(t *testing.T) {
	var stdout, stderr bytes.Buffer
	// A nil context fails before touching runtime files or starting Pi.
	if code := run(nil, []string{"daemon"}, &stdout, &stderr); code != 1 {
		t.Fatalf("daemon exit code = %d, want 1", code)
	}
	if got := stderr.String(); got != "pi-learnloop daemon: daemon: nil context\n" {
		t.Errorf("stderr = %q, want unchanged daemon error", got)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}

func TestRunVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("version exit code = %d, want 0", code)
	}
	if got := stdout.String(); got != "pi-learnloop dev\n" {
		t.Errorf("stdout = %q, want development version", got)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

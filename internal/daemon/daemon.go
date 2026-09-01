// Package daemon exposes the evidence preview through the versioned local
// protocol accepted in ADR-0002.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/reeezark/pi-learnloop/agent/prompts"
	"github.com/reeezark/pi-learnloop/internal/assessment"
	"github.com/reeezark/pi-learnloop/internal/evaluator"
)

const (
	protocolVersion = 1
	shutdownTimeout = 5 * time.Second
)

// ErrAlreadyRunning reports that another daemon owns the runtime state lock.
var ErrAlreadyRunning = errors.New("pi-learnloop daemon is already running")

// Config supplies process-local daemon configuration. StateDir is empty in
// production and may be set by integration tests to isolate runtime files.
type Config struct {
	StateDir string
}

// Run serves until ctx is cancelled or the HTTP server fails.
func Run(ctx context.Context, config Config) error {
	if ctx == nil {
		return errors.New("daemon: nil context")
	}
	stateDir, err := resolveStateDir(config.StateDir)
	if err != nil {
		return err
	}
	if err := prepareStateDir(stateDir); err != nil {
		return err
	}
	lock, err := acquireRuntimeLock(stateDir)
	if err != nil {
		return err
	}
	defer lock.release()
	var questionEvaluator evaluator.QuestionEvaluator
	if piEvaluator, err := evaluator.NewPiRPCEvaluator(ctx, prompts.EvaluatorQuestionGenerationV1()); err == nil {
		questionEvaluator = piEvaluator
	}

	instanceID, err := randomID(16)
	if err != nil {
		return fmt.Errorf("daemon: generate instance ID: %w", err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("daemon: listen on IPv4 loopback: %w", err)
	}
	defer listener.Close()

	descriptor := runtimeDescriptor{
		SchemaVersion:   1,
		ProtocolVersion: protocolVersion,
		InstanceID:      instanceID,
		PID:             os.Getpid(),
		BaseURL:         "http://" + listener.Addr().String(),
		StartedAt:       time.Now().UTC().Format(time.RFC3339),
	}
	token, err := randomID(32)
	if err != nil {
		return fmt.Errorf("daemon: generate instance token: %w", err)
	}
	if err := publishToken(stateDir, token); err != nil {
		return err
	}
	if err := publishDescriptor(stateDir, descriptor); err != nil {
		removeRuntimeFiles(stateDir, instanceID)
		_ = os.Remove(filepath.Join(stateDir, "daemon.token"))
		return err
	}
	defer removeRuntimeFiles(stateDir, instanceID)
	continuations := newContinuationStore()
	defer continuations.clear()
	assessments := assessment.New(nil)
	defer assessments.Close()

	server := &http.Server{
		Handler: newHandler(instanceID, listener.Addr().String(), token, serverServices{
			continuations:     continuations,
			questionEvaluator: questionEvaluator,
			assessments:       assessments,
		}),
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      125 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    8 * 1024,
	}
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("daemon: serve: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return fmt.Errorf("daemon: shutdown: %w", err)
		}
		if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("daemon: serve during shutdown: %w", err)
		}
		return nil
	}
}

func resolveStateDir(configured string) (string, error) {
	if configured != "" {
		if !filepath.IsAbs(configured) {
			return "", errors.New("daemon: configured state directory must be absolute")
		}
		return filepath.Clean(configured), nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("daemon: resolve user config directory: %w", err)
	}
	return filepath.Join(configDir, "pi-learnloop", "runtime"), nil
}

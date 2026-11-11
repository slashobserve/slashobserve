package observeagent

import (
	"context"
	"crypto/subtle"
	"errors"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/opaquetoken"
)

const (
	binaryName     = "observe-agent"
	initialBackoff = time.Second
	maxBackoff     = 30 * time.Second
	stableRun      = 60 * time.Second
	stopGrace      = 10 * time.Second
	onExitTimeout  = 5 * time.Second
	pipeDrainDelay = 2 * time.Second
)

var (
	errBinaryNotFound = errors.New("observe-agent is not on PATH; the server image ships it at /usr/local/bin/observe-agent, a source build gets it from make agent, or set SLASHOBSERVE_AGENT_RUNTIME=go")
	errExitedCleanly  = errors.New("observe-agent exited with status 0")
)

type SupervisorConfig struct {
	Path      string
	ServerURL string
	Slots     int
	LogLevel  slog.Level
	Logger    *slog.Logger
	OnExit    func(ctx context.Context)
}

type Supervisor struct {
	cfg SupervisorConfig

	args  []string
	grace time.Duration
	now   func() time.Time
	sleep func(ctx context.Context, d time.Duration)

	mu    sync.Mutex
	token string
}

func FindBinary() (string, error) {
	path, err := exec.LookPath(binaryName)
	if err != nil {
		return "", errBinaryNotFound
	}
	return path, nil
}

func NewSupervisor(c SupervisorConfig) *Supervisor {
	return &Supervisor{cfg: c, grace: stopGrace, now: time.Now, sleep: sleepContext}
}

func (s *Supervisor) Run(ctx context.Context) {
	delay := initialBackoff
	for ctx.Err() == nil {
		ran := s.runChild(ctx)
		if ctx.Err() != nil {
			return
		}
		if ran >= stableRun {
			delay = initialBackoff
		}
		s.sleep(ctx, delay)
		delay = min(delay*2, maxBackoff)
	}
}

func (s *Supervisor) Authenticate(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(s.token)) == 1
}

func (s *Supervisor) setToken(token string) {
	s.mu.Lock()
	s.token = token
	s.mu.Unlock()
}

func (s *Supervisor) runChild(ctx context.Context) time.Duration {
	token, _, err := opaquetoken.New(TokenPrefix)
	if err != nil {
		s.logStartFailure(err)
		return 0
	}
	outR, outW := io.Pipe()
	errR, errW := io.Pipe()
	cmd := exec.Command(s.cfg.Path, s.args...) //nolint:gosec // the path comes from FindBinary, and args exist only for tests
	cmd.Env = childEnv(s.cfg.ServerURL, token, s.cfg.Slots, s.cfg.LogLevel)
	cmd.Stdout, cmd.Stderr = outW, errW
	cmd.WaitDelay = pipeDrainDelay

	s.setToken(token)
	if err := cmd.Start(); err != nil {
		s.setToken("")
		s.logStartFailure(err)
		return 0
	}
	started := s.now()

	var relays sync.WaitGroup
	for _, r := range []io.Reader{outR, errR} {
		relays.Go(func() { relay(r, s.cfg.Logger) })
	}
	waitErr := s.wait(ctx, cmd)
	ran := s.now().Sub(started)

	s.setToken("")
	_ = outW.Close()
	_ = errW.Close()
	relays.Wait()
	s.releaseRuns(ctx)
	s.logExit(ctx, cmd, waitErr, ran)
	return ran
}

func (s *Supervisor) wait(ctx context.Context, cmd *exec.Cmd) error {
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	select {
	case err := <-exited:
		return err
	case <-ctx.Done():
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	grace := time.NewTimer(s.grace)
	defer grace.Stop()
	select {
	case err := <-exited:
		return err
	case <-grace.C:
		_ = cmd.Process.Kill()
		return <-exited
	}
}

func (s *Supervisor) releaseRuns(ctx context.Context) {
	if s.cfg.OnExit == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), onExitTimeout)
	defer cancel()
	s.cfg.OnExit(ctx)
}

func (s *Supervisor) logExit(ctx context.Context, cmd *exec.Cmd, waitErr error, ran time.Duration) {
	attrs := []any{slog.Int("exit_code", cmd.ProcessState.ExitCode()), slog.Duration("ran_for", ran)}
	if ctx.Err() != nil {
		s.cfg.Logger.Info("observe-agent exited", attrs...)
		return
	}
	if waitErr == nil {
		waitErr = errExitedCleanly
	}
	s.cfg.Logger.Warn("observe-agent exited", logattrs.Failure("observe_agent_exited", waitErr, attrs...)...)
}

func (s *Supervisor) logStartFailure(err error) {
	s.cfg.Logger.Error("observe-agent could not be started", logattrs.Failure("observe_agent_start_failed", err)...)
}

func sleepContext(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}

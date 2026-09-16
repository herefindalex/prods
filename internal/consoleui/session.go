package consoleui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
)

type Session struct {
	input       io.Reader
	output      io.Writer
	interactive bool

	mu       sync.Mutex
	guide    Guide
	logs     []string
	pending  string
	prestart bytes.Buffer
	program  *tea.Program
	started  bool
	starting bool
	closing  bool
	done     chan struct{}
	runErr   error
	stop     chan struct{}
	stopOnce sync.Once
}

func New(input, output *os.File) *Session {
	interactive := terminal(input) && terminal(output)
	return newSession(input, output, interactive)
}

func newSession(input io.Reader, output io.Writer, interactive bool) *Session {
	return &Session{input: input, output: output, interactive: interactive, done: make(chan struct{}), stop: make(chan struct{})}
}

func terminal(file *os.File) bool {
	if file == nil {
		return false
	}
	return isatty.IsTerminal(file.Fd()) || isatty.IsCygwinTerminal(file.Fd())
}

func (s *Session) Interactive() bool { return s != nil && s.interactive }

func (s *Session) Interrupt() <-chan struct{} {
	if s == nil {
		return nil
	}
	return s.stop
}

func (s *Session) Start(ctx context.Context, guide Guide) error {
	if s == nil || !s.interactive {
		return nil
	}
	s.mu.Lock()
	if s.started || s.starting {
		s.mu.Unlock()
		s.SetGuide(guide)
		return nil
	}
	s.guide = guide
	s.starting = true
	ready := make(chan struct{})
	program := tea.NewProgram(
		newModel(guide, s.logs, s.requestStop, ready),
		tea.WithInput(s.input), tea.WithOutput(s.output), tea.WithAltScreen(),
	)
	s.program = program
	s.mu.Unlock()

	go func() {
		_, err := program.Run()
		s.mu.Lock()
		s.runErr = err
		s.mu.Unlock()
		close(s.done)
		s.requestStop()
	}()

	select {
	case <-ready:
	case <-s.done:
		return s.programError()
	case <-ctx.Done():
		program.Quit()
		return ctx.Err()
	case <-time.After(2 * time.Second):
		program.Quit()
		return errors.New("console UI did not initialize")
	}
	s.mu.Lock()
	s.starting = false
	s.started = true
	s.prestart.Reset()
	state := replaceStateMsg{guide: s.guide, logs: append([]string(nil), s.logs...)}
	s.mu.Unlock()
	program.Send(state)
	go func() {
		<-ctx.Done()
		program.Quit()
	}()
	return nil
}

func (s *Session) SetGuide(guide Guide) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.guide = guide
	program, started := s.program, s.started
	s.mu.Unlock()
	if started {
		program.Send(guideMsg(guide))
	}
}

func (s *Session) Write(body []byte) (int, error) {
	if s == nil {
		return len(body), nil
	}
	s.mu.Lock()
	started := s.started
	if !s.interactive {
		if _, err := s.output.Write(body); err != nil {
			s.mu.Unlock()
			return 0, err
		}
		s.mu.Unlock()
		return len(body), nil
	}
	if !started {
		_, _ = s.prestart.Write(body)
	}
	s.pending += string(body)
	parts := strings.Split(s.pending, "\n")
	s.pending = parts[len(parts)-1]
	lines := make([]string, 0, len(parts)-1)
	for _, line := range parts[:len(parts)-1] {
		lines = append(lines, strings.TrimSuffix(line, "\r"))
	}
	if len(lines) > 0 {
		s.logs = append(s.logs, lines...)
		if len(s.logs) > maxLogEntries {
			s.logs = append([]string(nil), s.logs[len(s.logs)-maxLogEntries:]...)
		}
	}
	program := s.program
	s.mu.Unlock()
	if started && len(lines) > 0 {
		program.Send(logLinesMsg(lines))
	}
	return len(body), nil
}

func (s *Session) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return nil
	}
	s.closing = true
	program, started := s.program, s.started || s.starting
	prestart := append([]byte(nil), s.prestart.Bytes()...)
	s.mu.Unlock()
	if !started {
		if len(prestart) > 0 {
			_, err := s.output.Write(prestart)
			return err
		}
		return nil
	}
	program.Quit()
	select {
	case <-s.done:
		return s.programError()
	case <-time.After(3 * time.Second):
		return errors.New("console UI did not stop")
	}
}

func (s *Session) programError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runErr
}

func (s *Session) requestStop() {
	s.stopOnce.Do(func() { close(s.stop) })
}

func HostLabel() string {
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		hostname = "unknown"
	}
	return hostname + " (" + runtime.GOOS + "/" + runtime.GOARCH + ")"
}

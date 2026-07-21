package ui

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/Gelmezon/grok-switch/internal/ui/theme"
)

// Braille spinner frames.
var frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner is a single-line progress indicator for CLI mode.
type Spinner struct {
	msg    string
	w      io.Writer
	stop   chan struct{}
	done   chan struct{}
	mu     sync.Mutex
	active bool
}

// NewSpinner creates a spinner writing to stderr (so stdout stays clean for pipes).
func NewSpinner(msg string) *Spinner {
	return &Spinner{
		msg:  msg,
		w:    os.Stderr,
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
}

// Start begins animation if stderr is a TTY; otherwise no-op.
func (s *Spinner) Start() {
	if !termIsStderr() || !ColorEnabled() {
		return
	}
	s.mu.Lock()
	if s.active {
		s.mu.Unlock()
		return
	}
	s.active = true
	s.mu.Unlock()

	go func() {
		defer close(s.done)
		i := 0
		t := time.NewTicker(80 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-s.stop:
				// clear line
				fmt.Fprint(s.w, "\r\033[K")
				return
			case <-t.C:
				frame := frames[i%len(frames)]
				line := frame + "  " + s.msg
				if theme.Enabled() {
					line = theme.Accent.Render(frame) + "  " + s.msg
				}
				fmt.Fprint(s.w, "\r\033[K"+line)
				i++
			}
		}
	}()
}

// Update changes the spinner message.
func (s *Spinner) Update(msg string) {
	s.mu.Lock()
	s.msg = msg
	s.mu.Unlock()
}

// Stop ends the spinner.
func (s *Spinner) Stop() {
	s.mu.Lock()
	if !s.active {
		s.mu.Unlock()
		return
	}
	s.active = false
	s.mu.Unlock()
	close(s.stop)
	<-s.done
	s.stop = make(chan struct{})
	s.done = make(chan struct{})
}

// WithSpinner runs fn while showing a spinner (TTY only).
func WithSpinner(msg string, fn func() error) error {
	sp := NewSpinner(msg)
	sp.Start()
	err := fn()
	sp.Stop()
	return err
}

func termIsStderr() bool {
	// local import avoided cycle; duplicate thin check
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

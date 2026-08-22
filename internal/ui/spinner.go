package ui

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/term"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner is an animated spinner that runs in its own goroutine.
type Spinner struct {
	message string
	stop    chan struct{}
	wg      sync.WaitGroup
	active  atomic.Bool
	tty     bool
}

// StartSpinner begins an animated spinner with the given message. When
// stdout is not a terminal (piped/CI), the animation is suppressed and the
// message is printed once, so logs don't fill with spinner frames.
func StartSpinner(message string) *Spinner {
	tty := term.IsTerminal(int(os.Stdout.Fd()))
	s := &Spinner{message: message, stop: make(chan struct{}), tty: tty}
	s.active.Store(true)
	if !tty {
		// Non-interactive: print the label once and return a no-op spinner.
		fmt.Printf("  %s  %s\n", PrimaryStyle.Render(SymWork), message)
		return s
	}
	s.wg.Add(1)
	go s.run()
	return s
}

func (s *Spinner) run() {
	defer s.wg.Done()
	t := time.NewTicker(90 * time.Millisecond)
	defer t.Stop()
	for i := 0; ; i++ {
		select {
		case <-s.stop:
			fmt.Print("\r\033[2K")
			return
		case <-t.C:
			fmt.Printf("\r  %s  %s", PrimaryStyle.Render(spinnerFrames[i%len(spinnerFrames)]), s.message)
		}
	}
}

// Stop halts the spinner and clears its line. Safe to call on a non-TTY
// spinner (no-op beyond flipping the active flag).
func (s *Spinner) Stop() {
	if s.active.CompareAndSwap(true, false) {
		if s.tty {
			close(s.stop)
			s.wg.Wait()
		}
	}
}

// SuccessLine formats a success line.
func SuccessLine(message string) string {
	return fmt.Sprintf("  %s  %s", SuccessStyle.Render("✓"), message)
}

// ErrorLine formats an error line.
func ErrorLine(message string) string {
	return fmt.Sprintf("  %s  %s", ErrorStyle.Render("✗"), message)
}

// WarningLine formats a warning line.
func WarningLine(message string) string {
	return fmt.Sprintf("  %s  %s", WarningStyle.Render("⚠"), message)
}

// InfoLine formats an info line.
func InfoLine(message string) string {
	return fmt.Sprintf("  %s  %s", PrimaryStyle.Render("ℹ"), message)
}

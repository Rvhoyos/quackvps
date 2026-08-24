// Package ui is the tool's output voice: section headers, status lines, and a
// spinner for long steps. It only writes output; it never prompts. Color
// degrades automatically when stdout isn't a terminal, and the spinner animates
// only on a TTY so piped/CI output stays clean.
package ui

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// warnColor is the one color a warning ever uses: on the lines this package
// prints and inside the prompts' own descriptions, so a caution looks the same
// wherever it turns up. Orange rather than the usual yellow because it has to
// stand out from the grey a prompt description is rendered in, which is exactly
// where a warning gets skipped.
const warnColor = lipgloss.Color("214")

var (
	headerStyle  = lipgloss.NewStyle().Bold(true)
	warnStyle    = lipgloss.NewStyle().Foreground(warnColor)
	cautionStyle = lipgloss.NewStyle().Foreground(warnColor).Bold(true)
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)

// Step prints a section header for a distinct phase of the run.
func Step(title string) {
	fmt.Println()
	fmt.Println(headerStyle.Render("==> " + title))
}

// Info prints a neutral status line.
func Info(format string, args ...any) { fmt.Println(msg(format, args...)) }

// Warn prints a non-fatal warning.
func Warn(format string, args ...any) { fmt.Println(warnStyle.Render("  ! " + msg(format, args...))) }

// Caution returns a warning line to put at the end of a prompt's description,
// the one place this package doesn't print for itself. The prompt library renders
// a description in grey; this keeps its color through the form's own styling, so
// the part the user must not miss doesn't read like the rest of the hint.
func Caution(format string, args ...any) string {
	return cautionStyle.Render("! " + msg(format, args...))
}

// Success prints a completed-step line.
func Success(format string, args ...any) {
	fmt.Println(successStyle.Render("  ✓ " + msg(format, args...)))
}

// Error prints a failure line to stderr.
func Error(format string, args ...any) {
	fmt.Fprintln(os.Stderr, errorStyle.Render("  ✗ "+msg(format, args...)))
}

// Bullet prints an indented list.
func Bullet(items ...string) {
	for _, it := range items {
		fmt.Println("  • " + it)
	}
}

// Spinner runs fn while showing an animated indicator on a TTY. When output
// isn't a terminal it just prints start and result lines. The fn's error is
// returned unchanged.
func Spinner(title string, fn func() error) error {
	if !isTTY() {
		fmt.Println("  " + title + "...")
		return finish(title, fn())
	}

	done := make(chan struct{})
	var once sync.Once
	stop := func() { once.Do(func() { close(done) }) }

	go func() {
		frames := []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}
		t := time.NewTicker(90 * time.Millisecond)
		defer t.Stop()
		i := 0
		for {
			select {
			case <-done:
				return
			case <-t.C:
				fmt.Printf("\r  %c %s", frames[i%len(frames)], title)
				i++
			}
		}
	}()

	err := fn()
	stop()
	fmt.Print("\r\033[K") // clear the spinner line
	return finish(title, err)
}

func finish(title string, err error) error {
	if err != nil {
		Error("%s: %v", title, err)
		return err
	}
	Success("%s", title)
	return nil
}

func msg(format string, args ...any) string {
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}

func isTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

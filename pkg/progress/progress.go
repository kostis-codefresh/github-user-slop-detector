// Package progress prints one numbered line per pipeline step so a run shows
// what it is doing. The CLI points it at stdout; the markdown report itself is
// written to a file, so the two never interleave.
package progress

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
)

// nameWidth pads step names into a column so the "done" results line up.
// "Repo History Merge Rate" (23 characters) is the longest name in the
// pipeline; shorter names get more dots, longer ones get the minimum three.
const nameWidth = 26

// The phase attribute tells the handler which part of a step line a record is,
// so it can leave the line open while the work runs.
const (
	keyPhase   = "phase"
	phaseStart = "start"
	phaseEnd   = "end"
)

// Stepper numbers and prints the pipeline's steps. A nil *Stepper is a silent
// no-op, which is how the metrics tests run without progress output.
type Stepper struct {
	log   *slog.Logger
	total int
	n     int
}

// New returns a Stepper that writes total numbered steps to w.
func New(w io.Writer, total int) *Stepper {
	return &Stepper{log: slog.New(&handler{w: w}), total: total}
}

// Step announces the next step and leaves its line open, so the step is on
// screen while the work runs rather than only once it has finished.
func (s *Stepper) Step(name string) {
	if s == nil {
		return
	}
	s.n++
	dots := strings.Repeat(".", max(nameWidth-len(name), 3))
	width := len(strconv.Itoa(s.total))
	s.log.Info(fmt.Sprintf("step %*d/%d  %s %s ", width, s.n, s.total, name, dots),
		slog.String(keyPhase, phaseStart))
}

// Done closes the open step line. detail is an optional short parenthetical
// summarising what the step produced.
func (s *Stepper) Done(detail string) {
	s.finish("done", detail)
}

// Skip closes the open step line for work that did not apply — an inactive
// metric is neither a pass nor a failure.
func (s *Stepper) Skip(reason string) {
	s.finish("skipped", reason)
}

func (s *Stepper) finish(status, detail string) {
	if s == nil {
		return
	}
	if detail != "" {
		status += " (" + detail + ")"
	}
	s.log.Info(status, slog.String(keyPhase, phaseEnd))
}

// Write lets the standard library's log package share the handler, so a retry
// warning or a fatal error never lands in the middle of an open step line.
func (s *Stepper) Write(p []byte) (int, error) {
	if s == nil {
		return len(p), nil
	}
	s.log.Info(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

// handler renders records as plain terminal lines, with no timestamp and no
// level prefix: the point is a short trail a human reads while waiting, not a
// parseable log. Step lines are written in two halves, so it holds the open
// prefix to be able to repeat it if something interrupts.
type handler struct {
	mu     sync.Mutex
	w      io.Writer
	prefix string //the open "step N/T  name ..." half, empty when no step is open
	broken bool   //something else printed while the step was open
}

func (h *handler) Enabled(context.Context, slog.Level) bool { return true }

func (h *handler) Handle(_ context.Context, r slog.Record) error {
	phase := ""
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == keyPhase {
			phase = a.Value.String()
			return false
		}
		return true
	})

	h.mu.Lock()
	defer h.mu.Unlock()

	switch phase {
	case phaseStart:
		h.prefix, h.broken = r.Message, false
		_, err := io.WriteString(h.w, r.Message)
		return err
	case phaseEnd:
		line := r.Message + "\n"
		if h.broken {
			line = h.prefix + line
		}
		h.prefix, h.broken = "", false
		_, err := io.WriteString(h.w, line)
		return err
	}

	// A warning or an error: end the open step's half-written line first, and
	// mark it so its "done" half repeats the prefix and stays readable.
	prefix := ""
	if h.prefix != "" && !h.broken {
		h.broken = true
		prefix = "\n"
	}
	_, err := io.WriteString(h.w, prefix+r.Message+"\n")
	return err
}

// WithAttrs and WithGroup are unused: Stepper is the only producer of records
// and it passes every attribute on the record itself.
func (h *handler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *handler) WithGroup(string) slog.Handler      { return h }

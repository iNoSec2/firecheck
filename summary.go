package main

import (
	"fmt"
	"io"
	"time"
)

type runSummary struct{ allowed, denied, failed int }

func (s *runSummary) add(r result) {
	switch r.State {
	case stateAllowed:
		s.allowed++
	case stateDenied:
		s.denied++
	default:
		s.failed++
	}
}

func (s runSummary) write(out io.Writer, elapsed time.Duration, code int) error {
	_, err := fmt.Fprintf(out, "firecheck: summary: %d completed, %d allowed, %d denied, %d failed; elapsed %s; exit %d\n", s.allowed+s.denied+s.failed, s.allowed, s.denied, s.failed, elapsed.Round(time.Millisecond), code)
	return err
}

package main

import (
	"fmt"
	"io"
)

// A single consumer owns the reporter so output lines cannot overlap.
type reporter struct {
	out    io.Writer
	saved  io.Writer
	simple bool
}

func (p *reporter) write(r result) error {
	if r.State == stateAllowed && p.saved != nil {
		if _, err := fmt.Fprintln(p.saved, r.URL); err != nil {
			return err
		}
	}
	if p.simple {
		if r.State != stateAllowed {
			return nil
		}
		_, err := fmt.Fprintln(p.out, r.URL)
		return err
	}
	_, err := fmt.Fprintf(p.out, "%s => R: %s | W: not-run | D: not-run\n", r.URL, r.State)
	return err
}

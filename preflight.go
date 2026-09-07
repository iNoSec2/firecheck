package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func checkConfig(cfg config, out, diagnostics io.Writer) int {
	if err := checkOutputPath(cfg.Output); err != nil {
		fmt.Fprintf(diagnostics, "firecheck: configuration error: %v\n", err)
		return 2
	}
	message := "Configuration OK. No requests or file writes were made."
	if cfg.URL == "" {
		message += " No URL supplied; stdin was not checked."
	}
	if _, err := fmt.Fprintln(out, message); err != nil {
		fmt.Fprintf(diagnostics, "firecheck: cannot write configuration result: %v\n", err)
		return 1
	}
	return 0
}

func checkOutputPath(path string) error {
	if path == "" {
		return nil
	}
	info, err := os.Stat(path)
	if err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("--output must name a regular file, not a directory or device")
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("cannot inspect --output: %w", err)
	}
	parent, err := os.Stat(filepath.Dir(path))
	if err != nil || !parent.IsDir() {
		return errors.New("the --output parent directory must exist")
	}
	return nil
}

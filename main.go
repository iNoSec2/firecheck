// firecheck was created by @bp0lr in 2020.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		_ = os.Stdin.Close() // Unblock input on Ctrl+C.
	}()
	info, err := os.Stdin.Stat()
	interactive := err == nil && info.Mode()&os.ModeCharDevice != 0
	os.Exit(run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, interactive))
}

func run(ctx context.Context, args []string, in io.Reader, out, diagnostics io.Writer, interactive bool) (code int) {
	cfg, err := parseConfig(args, out)
	if errors.Is(err, errHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintf(diagnostics, "firecheck: %v\nTry 'firecheck --help' for examples.\n", err)
		return 2
	}
	if cfg.URL == "" && interactive {
		fmt.Fprintln(diagnostics, "firecheck: provide --url or pipe one URL per line. Try 'firecheck --help'.")
		return 2
	}
	var file *os.File
	if cfg.Output != "" {
		file, err = os.OpenFile(cfg.Output, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			fmt.Fprintf(diagnostics, "firecheck: cannot open output: %v\n", err)
			return 1
		}
		defer func() {
			if err := file.Close(); err != nil {
				fmt.Fprintf(diagnostics, "firecheck: cannot close output: %v\n", err)
				code = 1
			}
		}()
	}
	var saved io.Writer
	if file != nil {
		saved = file
	}
	report := reporter{out: out, saved: saved, simple: cfg.Simple}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	client := newClient(cfg)
	defer client.CloseIdleConnections()
	jobs := make(chan string)
	results := make(chan result)
	inputErr := make(chan error, 1)
	go func() {
		defer close(jobs)
		if cfg.URL != "" {
			select {
			case jobs <- cfg.URL:
			case <-ctx.Done():
			}
			inputErr <- nil
			return
		}
		scanner := bufio.NewScanner(in)
		for scanner.Scan() {
			raw := strings.TrimSpace(scanner.Text())
			if raw == "" {
				continue
			}
			select {
			case jobs <- raw:
			case <-ctx.Done():
				inputErr <- ctx.Err()
				return
			}
		}
		inputErr <- scanner.Err()
	}()
	var workers sync.WaitGroup
	for i := 0; i < cfg.Workers; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for {
				var raw string
				var ok bool
				select {
				case raw, ok = <-jobs:
					if !ok {
						return
					}
				case <-ctx.Done():
					return
				}
				r := checkRead(ctx, raw, cfg, client)
				select {
				case results <- r:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		workers.Wait()
		close(results)
	}()
	count := 0
	for r := range results {
		count++
		if r.State == stateError {
			code = 1
			fmt.Fprintf(diagnostics, "firecheck: %s: %s\n", r.URL, r.Detail)
		} else if cfg.Verbose {
			fmt.Fprintf(diagnostics, "firecheck: %s: read %s (HTTP %d)\n", r.URL, r.State, r.Status)
		}
		if err := report.write(r); err != nil {
			fmt.Fprintf(diagnostics, "firecheck: cannot write results: %v\n", err)
			cancel()
			return 1
		}
	}
	if ctx.Err() != nil {
		fmt.Fprintln(diagnostics, "firecheck: interrupted; output may be incomplete.")
		return 130
	}
	if err := <-inputErr; err != nil {
		fmt.Fprintf(diagnostics, "firecheck: cannot read input: %v\n", err)
		return 1
	}
	if count == 0 {
		fmt.Fprintln(diagnostics, "firecheck: no URLs received. Provide --url or pipe one URL per line.")
		return 2
	}
	return code
}

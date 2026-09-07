package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSimpleOutputAlsoSaved(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/.json" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		fmt.Fprint(w, "null")
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "results.txt")
	if err := os.WriteFile(path, []byte("previous\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	code := run(context.Background(), []string{"-u", server.URL, "-s", "-o", path, "-v"}, strings.NewReader(""), &out, &diagnostics, false)
	if code != 0 || out.String() != server.URL+"\n" || calls.Load() != 1 {
		t.Fatalf("code=%d stdout=%q calls=%d stderr=%q", code, out.String(), calls.Load(), diagnostics.String())
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(saved) != "previous\n"+out.String() {
		t.Fatalf("saved=%q", saved)
	}
	if !strings.Contains(diagnostics.String(), "HTTP 200") {
		t.Fatal("missing verbose diagnostics")
	}
}

func TestReadStates(t *testing.T) {
	for _, tc := range []struct {
		status int
		state  string
		exit   int
	}{
		{200, stateAllowed, 0}, {401, stateDenied, 0}, {403, stateDenied, 0},
		{404, stateError, 1}, {429, stateError, 1}, {500, stateError, 1},
	} {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status) }))
			defer server.Close()
			var out, diagnostics bytes.Buffer
			code := run(context.Background(), []string{"-u", server.URL}, strings.NewReader(""), &out, &diagnostics, false)
			if code != tc.exit || !strings.Contains(out.String(), "R: "+tc.state+" | W: not-run | D: not-run") {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), diagnostics.String())
			}
		})
	}
}

func TestConfigErrorsAndHelp(t *testing.T) {
	for _, args := range [][]string{
		{"-w", "0"}, {"-w", "100"}, {"-p", "socks5://localhost:9000"},
		{"-H", "broken"}, {"-H", "Bad Name: value"}, {"-H", "Name: value\r\nx: y"},
		{"-u", "example.invalid"}, {"-u", "https://example.invalid/nested"},
		{"-u", "https://user:secret@example.invalid"}, {"-u", "https://example.invalid/?auth=secret"},
		{"-m", "old-probe"}, {"extra"}, {"--unknown"},
	} {
		var out, diagnostics bytes.Buffer
		if code := run(context.Background(), args, strings.NewReader(""), &out, &diagnostics, false); code != 2 {
			t.Fatalf("args=%q code=%d", args, code)
		}
		if out.Len() != 0 || diagnostics.Len() == 0 || strings.Contains(diagnostics.String(), "secret") {
			t.Fatalf("args=%q stdout=%q stderr=%q", args, out.String(), diagnostics.String())
		}
	}
	var help bytes.Buffer
	if code := run(context.Background(), []string{"--help"}, nil, &help, io.Discard, true); code != 0 || !strings.Contains(help.String(), "Local emulator example") {
		t.Fatalf("help: code=%d output=%q", code, help.String())
	}
}

func TestHeadersAndEmulatorNamespace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ns") != "demo-firecheck" || r.Header.Get("X-Test") != "value: suffix" {
			t.Errorf("namespace=%q header=%q", r.URL.RawQuery, r.Header.Get("X-Test"))
		}
		fmt.Fprint(w, "null")
	}))
	defer server.Close()
	cfg, err := parseConfig([]string{"-H", " X-Test : value: suffix "}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	client := newClient(cfg)
	defer client.CloseIdleConnections()
	if r := checkRead(context.Background(), server.URL+"/?ns=demo-firecheck", cfg, client); r.State != stateAllowed {
		t.Fatalf("result=%+v", r)
	}
}

func TestTLSVerificationAndRedirects(t *testing.T) {
	var calls atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer destination.Close()
	tlsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("untrusted TLS server received a request") }))
	defer tlsServer.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, 302) }))
	defer redirect.Close()
	client := newClient(config{})
	defer client.CloseIdleConnections()
	if r := checkRead(context.Background(), tlsServer.URL, config{}, client); r.State != stateError {
		t.Fatal("untrusted certificate accepted")
	}
	if r := checkRead(context.Background(), redirect.URL, config{}, client); r.Status != 302 || r.State != stateError {
		t.Fatalf("redirect result=%+v", r)
	}
	if calls.Load() != 0 {
		t.Fatal("redirect followed")
	}
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errors.New("input failed") }

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("output failed") }

func TestInputErrors(t *testing.T) {
	for _, tc := range []struct {
		in          io.Reader
		interactive bool
		exit        int
	}{
		{strings.NewReader("\n \n"), false, 2}, {strings.NewReader(""), true, 2},
		{brokenReader{}, false, 1}, {strings.NewReader(strings.Repeat("x", 70*1024)), false, 1},
		{strings.NewReader("https://user:secret@example.invalid\n"), false, 1},
	} {
		var out, diagnostics bytes.Buffer
		if code := run(context.Background(), nil, tc.in, &out, &diagnostics, tc.interactive); code != tc.exit {
			t.Fatalf("code=%d want=%d", code, tc.exit)
		}
		if strings.Contains(out.String()+diagnostics.String(), "secret") {
			t.Fatal("credentials leaked")
		}
	}
}

func TestReporterErrors(t *testing.T) {
	for _, p := range []reporter{
		{out: brokenWriter{}}, {out: io.Discard, saved: brokenWriter{}, simple: true},
	} {
		if err := p.write(result{URL: "http://127.0.0.1", State: stateAllowed}); err == nil {
			t.Fatal("write error lost")
		}
	}
	var out bytes.Buffer
	p := reporter{out: &out, simple: true}
	if err := p.write(result{State: stateDenied}); err != nil || out.Len() != 0 {
		t.Fatal("denied result in simple output")
	}
}

func TestCancellationWhileWaitingForInput(t *testing.T) {
	in, writer := io.Pipe()
	defer in.Close()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() { done <- run(ctx, nil, in, io.Discard, io.Discard, false) }()
	cancel()
	select {
	case code := <-done:
		if code != 130 {
			t.Fatalf("code=%d", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation blocked on stdin")
	}
}

func TestConcurrentOutputIsComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "null") }))
	defer server.Close()
	var out, diagnostics bytes.Buffer
	input := strings.Repeat("  "+server.URL+"  \r\n\n", 40)
	if code := run(context.Background(), []string{"-s", "-w", "8"}, strings.NewReader(input), &out, &diagnostics, false); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, diagnostics.String())
	}
	if out.String() != strings.Repeat(server.URL+"\n", 40) {
		t.Fatal("missing or overlapping output lines")
	}
}

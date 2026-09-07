package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSummaryKeepsStdoutClean(t *testing.T) {
	allowed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "null") }))
	defer allowed.Close()
	denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) }))
	defer denied.Close()
	var out, diagnostics bytes.Buffer
	input := allowed.URL + "\n" + denied.URL + "\ninvalid\n"
	code := run(context.Background(), []string{"-s", "--summary"}, strings.NewReader(input), &out, &diagnostics, false)
	if code != 1 || out.String() != allowed.URL+"\n" {
		t.Fatalf("code=%d stdout=%q", code, out.String())
	}
	if !strings.Contains(diagnostics.String(), "3 completed, 1 allowed, 1 denied, 1 failed; elapsed ") || !strings.HasSuffix(diagnostics.String(), "; exit 1\n") {
		t.Fatalf("stderr=%q", diagnostics.String())
	}
}

func TestSummaryOnInterruptedInput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var diagnostics bytes.Buffer
	code := run(ctx, []string{"--summary"}, strings.NewReader(""), io.Discard, &diagnostics, false)
	if code != 130 || !strings.Contains(diagnostics.String(), "0 completed") || !strings.HasSuffix(diagnostics.String(), "; exit 130\n") {
		t.Fatalf("code=%d stderr=%q", code, diagnostics.String())
	}
}

func TestSummaryOutputFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "null") }))
	defer server.Close()
	if code := run(context.Background(), []string{"-u", server.URL, "--summary"}, nil, io.Discard, brokenWriter{}, false); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

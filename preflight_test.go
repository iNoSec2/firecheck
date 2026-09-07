package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCheckConfigHasNoSideEffects(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "existing.txt")
	if err := os.WriteFile(path, []byte("keep me"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	args := []string{"--check-config", "-u", server.URL, "-o", path, "-H", "Authorization: secret"}
	if code := run(context.Background(), args, brokenReader{}, &out, io.Discard, true); code != 0 {
		t.Fatalf("code=%d", code)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "keep me" || requests.Load() != 0 || strings.Contains(out.String(), "secret") {
		t.Fatalf("side effect or leaked credential: data=%q output=%q requests=%d", data, out.String(), requests.Load())
	}
	newPath := filepath.Join(t.TempDir(), "new.txt")
	if code := run(context.Background(), []string{"--check-config", "-o", newPath}, brokenReader{}, io.Discard, io.Discard, true); code != 0 {
		t.Fatalf("code=%d", code)
	}
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Fatal("preflight created an output file")
	}
}

func TestCheckConfigRejectsBadOutputPaths(t *testing.T) {
	dir := t.TempDir()
	for _, path := range []string{dir, filepath.Join(dir, "missing", "report.txt")} {
		if code := run(context.Background(), []string{"--check-config", "-o", path}, nil, io.Discard, io.Discard, true); code != 2 {
			t.Fatalf("path=%s code=%d", path, code)
		}
	}
	if code := run(context.Background(), []string{"--check-config", "-H", "X-Test: a\x01b"}, nil, io.Discard, io.Discard, true); code != 2 {
		t.Fatalf("control character accepted: code=%d", code)
	}
	if code := run(context.Background(), []string{"--check-config"}, nil, brokenWriter{}, io.Discard, true); code != 1 {
		t.Fatalf("lost output error: code=%d", code)
	}
}

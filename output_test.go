package main

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func BenchmarkReportFile(b *testing.B) {
	for _, buffered := range []bool{false, true} {
		name := "unbuffered"
		if buffered {
			name = "buffered"
		}
		b.Run(name, func(b *testing.B) {
			file, err := os.Create(filepath.Join(b.TempDir(), "report.txt"))
			if err != nil {
				b.Fatal(err)
			}
			defer file.Close()
			var saved io.Writer = file
			var buffer *bufio.Writer
			if buffered {
				buffer = bufio.NewWriterSize(file, 32*1024)
				saved = buffer
			}
			p := reporter{out: io.Discard, saved: saved, simple: true}
			r := result{URL: "http://127.0.0.1:9000/?ns=demo-firecheck", State: stateAllowed}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := p.write(r); err != nil {
					b.Fatal(err)
				}
			}
			if buffer != nil {
				if err := buffer.Flush(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkParseTarget(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := parseTarget("http://127.0.0.1:9000/?ns=demo-firecheck"); err != nil {
			b.Fatal(err)
		}
	}
}

func TestOutputFlushedWhenStdoutFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "null") }))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "report.txt")
	code := run(context.Background(), []string{"-u", server.URL, "-o", path, "-s"}, strings.NewReader(""), brokenWriter{}, io.Discard, false)
	if code != 1 {
		t.Fatalf("code=%d", code)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != server.URL+"\n" {
		t.Fatalf("buffer not flushed: %q", data)
	}
}

func TestOutputFlushError(t *testing.T) {
	if _, err := os.Stat("/dev/full"); err != nil {
		t.Skip("requires /dev/full")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "null") }))
	defer server.Close()
	var diagnostics strings.Builder
	code := run(context.Background(), []string{"-u", server.URL, "-o", "/dev/full", "-s"}, strings.NewReader(""), io.Discard, &diagnostics, false)
	if code != 1 || !strings.Contains(diagnostics.String(), "cannot flush") {
		t.Fatalf("code=%d stderr=%q", code, diagnostics.String())
	}
}

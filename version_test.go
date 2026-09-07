package main

import (
	"bytes"
	"context"
	"io"
	"runtime/debug"
	"strings"
	"testing"
)

func TestVersionDoesNotReadInput(t *testing.T) {
	var out bytes.Buffer
	if code := run(context.Background(), []string{"--version"}, brokenReader{}, &out, io.Discard, true); code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out.String(), "firecheck ") || !strings.Contains(out.String(), "\nCommit: ") || !strings.Contains(out.String(), "\nGo: go") {
		t.Fatalf("version output=%q", out.String())
	}
	if code := run(context.Background(), []string{"--version"}, nil, brokenWriter{}, io.Discard, true); code != 1 {
		t.Fatalf("lost output error: code=%d", code)
	}
}

func TestBuildMetadata(t *testing.T) {
	info := &debug.BuildInfo{
		Main:     debug.Module{Version: "v2.0.0"},
		Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "abc123"}, {Key: "vcs.modified", Value: "true"}},
	}
	if got := describeBuild(info, "go1.27.1"); got != "firecheck v2.0.0\nCommit: abc123 (modified)\nGo: go1.27.1\n" {
		t.Fatal(got)
	}
	if got := describeBuild(nil, "go1.27.1"); got != "firecheck dev\nCommit: unknown\nGo: go1.27.1\n" {
		t.Fatal(got)
	}
}

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readyDoctorTools() doctorTools {
	return doctorTools{
		lookPath: func(name string) (string, error) { return name, nil },
		probe: func(ctx context.Context, name, arg string) (string, error) {
			if name == "node" {
				return "v24.20.0\n", nil
			}
			if name == "java" {
				return "openjdk version \"21.0.10\"\n", nil
			}
			return "", errors.New("unexpected command")
		},
	}
}

func TestDoctorReady(t *testing.T) {
	var out bytes.Buffer
	if code := runDoctor(context.Background(), ".", &out, io.Discard, readyDoctorTools()); code != 0 {
		t.Fatalf("code=%d output=%s", code, out.String())
	}
	for _, want := range []string{"OK  Go:", "OK  Node.js:", "OK  Java:", "127.0.0.1:19000", "No installations"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %s in %s", want, out.String())
		}
	}
}

func TestDoctorMissingOrOldTools(t *testing.T) {
	tools := readyDoctorTools()
	tools.lookPath = func(name string) (string, error) {
		if name == "go" {
			return "", errors.New("missing")
		}
		return name, nil
	}
	tools.probe = func(context.Context, string, string) (string, error) { return "v18.0.0", nil }
	var out bytes.Buffer
	if code := runDoctor(context.Background(), ".", &out, io.Discard, tools); code != 1 {
		t.Fatalf("code=%d", code)
	}
	for _, want := range []string{"FIX  Go:", "found 18; install 24", "FIX  Java:"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %s in %s", want, out.String())
		}
	}
}

func TestDoctorInvalidConfig(t *testing.T) {
	root := t.TempDir()
	if _, err := inspectEmulatorConfig(root); err == nil {
		t.Fatal("missing config accepted")
	}
	for _, content := range []string{
		`not json`,
		`{"emulators":{"database":{"host":"example.invalid","port":19000}}}`,
		`{"database":{"rules":"../outside.json"},"emulators":{"database":{"host":"127.0.0.1","port":19000}}}`,
		`{"database":{"rules":"missing.json"},"emulators":{"database":{"host":"127.0.0.1","port":19000}}}`,
	} {
		if err := os.WriteFile(filepath.Join(root, "firebase.json"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := inspectEmulatorConfig(root); err == nil {
			t.Fatalf("invalid config accepted: %s", content)
		}
	}
}

func TestDoctorCancellationAndOutputErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if code := runDoctor(ctx, ".", io.Discard, io.Discard, readyDoctorTools()); code != 130 {
		t.Fatalf("code=%d", code)
	}
	if code := runDoctor(context.Background(), ".", brokenWriter{}, io.Discard, readyDoctorTools()); code != 1 {
		t.Fatalf("lost write failure: code=%d", code)
	}
}

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type doctorTools struct {
	lookPath func(string) (string, error)
	probe    func(context.Context, string, string) (string, error)
}

func localDoctorTools() doctorTools {
	return doctorTools{
		lookPath: exec.LookPath,
		probe: func(ctx context.Context, name, arg string) (string, error) {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, name, arg)
			var output limitedDiagnostic
			cmd.Stdout, cmd.Stderr = &output, &output
			err := cmd.Run()
			return output.String(), err
		},
	}
}

// Retain enough version output for diagnostics without unbounded buffering.
type limitedDiagnostic struct{ data bytes.Buffer }

func (b *limitedDiagnostic) String() string { return b.data.String() }

func (b *limitedDiagnostic) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := 4096 - b.data.Len(); remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.data.Write(p)
	}
	return n, nil
}

func runDoctor(ctx context.Context, root string, out, diagnostics io.Writer, tools doctorTools) int {
	failed, writeFailed := false, false
	check := func(name, detail string, ok bool) {
		state := "OK"
		if !ok {
			state, failed = "FIX", true
		}
		if _, err := fmt.Fprintf(out, "%s  %s: %s\n", state, name, detail); err != nil {
			writeFailed = true
		}
	}
	_, err := tools.lookPath("go")
	if err != nil {
		check("Go", "install Go 1.27.1 or newer and add it to PATH (https://go.dev/dl/)", false)
	} else {
		check("Go", "launcher found; this executable was built with "+runtime.Version(), true)
	}
	for _, tool := range []struct {
		name, label, arg, pattern string
		minimum                   int
	}{
		{"node", "Node.js", "--version", `^v([0-9]+)\.`, 24},
		{"java", "Java", "-version", `(?m)^(?:openjdk|java)(?: version)? "?([0-9]+)`, 21},
	} {
		path, err := tools.lookPath(tool.name)
		if err != nil {
			check(tool.label, fmt.Sprintf("install version %d or newer and add it to PATH", tool.minimum), false)
			continue
		}
		version, err := tools.probe(ctx, path, tool.arg)
		match := regexp.MustCompile(tool.pattern).FindStringSubmatch(strings.TrimSpace(version))
		if err != nil || len(match) != 2 {
			check(tool.label, "could not read its version; try '"+tool.name+" "+tool.arg+"' in this terminal", false)
			continue
		}
		major, _ := strconv.Atoi(match[1])
		if major < tool.minimum {
			check(tool.label, fmt.Sprintf("found %d; install %d or newer", major, tool.minimum), false)
		} else {
			check(tool.label, fmt.Sprintf("version %d is suitable for the local emulator", major), true)
		}
	}
	if detail, err := inspectEmulatorConfig(root); err != nil {
		check("Emulator configuration", err.Error(), false)
	} else {
		check("Emulator configuration", detail, true)
	}
	if ctx.Err() != nil {
		fmt.Fprintln(diagnostics, "firecheck: doctor interrupted.")
		return 130
	}
	if _, err := fmt.Fprintln(out, "No installations, file writes or network checks were performed. Go toolchain selection and rule behavior are verified by the normal build and integration tests."); err != nil {
		writeFailed = true
	}
	if writeFailed {
		fmt.Fprintln(diagnostics, "firecheck: cannot write doctor results.")
		return 1
	}
	if failed {
		return 1
	}
	return 0
}

func inspectEmulatorConfig(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "firebase.json"))
	if err != nil {
		return "", errors.New("run --doctor from the repository checkout containing firebase.json")
	}
	var cfg struct {
		Database struct {
			Rules string `json:"rules"`
		} `json:"database"`
		Emulators struct {
			Database struct {
				Host string `json:"host"`
				Port int    `json:"port"`
			} `json:"database"`
		} `json:"emulators"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", errors.New("firebase.json must be valid JSON with a database emulator configuration")
	}
	host, port := cfg.Emulators.Database.Host, cfg.Emulators.Database.Port
	if !isLoopback(host) || port < 1 || port > 65535 {
		return "", errors.New("set the database emulator to a literal loopback IP and a port from 1 to 65535")
	}
	path := filepath.Clean(cfg.Database.Rules)
	if cfg.Database.Rules == "" || !filepath.IsLocal(path) {
		return "", errors.New("set database.rules to a relative fixture path inside this checkout")
	}
	data, err = os.ReadFile(filepath.Join(root, path))
	if err != nil {
		return "", errors.New("the configured rule fixture is missing or unreadable")
	}
	var fixture map[string]json.RawMessage
	if err := json.Unmarshal(data, &fixture); err != nil {
		return "", errors.New("the rule fixture must contain valid JSON")
	}
	rules := bytes.TrimSpace(fixture["rules"])
	if len(rules) == 0 || rules[0] != '{' {
		return "", errors.New("the rule fixture must contain a rules object")
	}
	return fmt.Sprintf("%s:%d; rule fixture found and valid JSON", host, port), nil
}

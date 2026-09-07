//go:build integration

package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
)

// Mutation checks exist only in tests and only reach a literal loopback IP.
// The fixed demo namespace and fixture rules never use a production project.
func TestEmulatorRules(t *testing.T) {
	address := os.Getenv("FIREBASE_DATABASE_EMULATOR_HOST")
	if address == "" {
		t.Fatal("start the local emulator with emulators:exec; see README")
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil || !isLoopback(host) {
		t.Fatal("emulator address must use a literal loopback IP and port")
	}
	client := newClient(config{}) // No proxy and no redirects.
	defer client.CloseIdleConnections()
	base := url.URL{Scheme: "http", Host: address, RawQuery: "ns=demo-firecheck-default-rtdb"}
	request := func(method, path, body string) (int, string) {
		t.Helper()
		u := base
		u.Path = path + ".json"
		req, err := http.NewRequestWithContext(context.Background(), method, u.String(), strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, string(data)
	}
	path := "/firecheck_tests/" + rand.Text()
	status, _ := request(http.MethodPut, path, `{"message":"local rule test"}`)
	if status != http.StatusOK {
		t.Fatalf("fixture write: HTTP %d", status)
	}
	t.Cleanup(func() {
		status, _ := request(http.MethodDelete, path, "")
		if status != http.StatusOK {
			t.Errorf("fixture cleanup: HTTP %d", status)
		}
	})
	status, body := request(http.MethodGet, path, "")
	if status != http.StatusOK || !strings.Contains(body, "local rule test") {
		t.Fatalf("fixture read: HTTP %d body=%s", status, body)
	}
	status, _ = request(http.MethodDelete, path, "")
	if status != http.StatusOK {
		t.Fatalf("fixture delete: HTTP %d", status)
	}
	status, body = request(http.MethodGet, path, "")
	if status != http.StatusOK || strings.TrimSpace(body) != "null" {
		t.Fatalf("fixture remains after delete: HTTP %d body=%s", status, body)
	}
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		status, _ := request(method, "/locked/"+rand.Text(), `{"message":"denied fixture"}`)
		if status != http.StatusUnauthorized && status != http.StatusForbidden {
			t.Errorf("%s should be denied: HTTP %d", method, status)
		}
	}
	var output, diagnostics strings.Builder
	code := run(context.Background(), []string{"-u", base.String()}, strings.NewReader(""), &output, &diagnostics, false)
	if code != 0 || !strings.Contains(output.String(), "R: denied") {
		t.Fatal(fmt.Sprintf("root check: code=%d output=%q stderr=%q", code, output.String(), diagnostics.String()))
	}
}

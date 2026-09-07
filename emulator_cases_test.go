//go:build integration

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestEmulatorRulesCases(t *testing.T) {
	data, err := os.ReadFile("testdata/rule-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	cases, err := loadRuleCases(data)
	if err != nil {
		t.Fatal(err)
	}
	base, err := localEmulatorURL(os.Getenv("FIREBASE_DATABASE_EMULATOR_HOST"))
	if err != nil {
		t.Fatal(err)
	}
	client := newClient(config{}) // Direct loopback connection, no proxies or redirects.
	t.Cleanup(client.CloseIdleConnections)
	runID := rand.Text()
	request := func(method, path string, body []byte) (int, error) {
		u := base
		u.Path = path + ".json"
		req, err := http.NewRequestWithContext(context.Background(), method, u.String(), bytes.NewReader(body))
		if err != nil {
			return 0, err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()
		return resp.StatusCode, nil
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			path := strings.ReplaceAll(c.Path, "{run}", runID)
			method := map[string]string{"read": http.MethodGet, "write": http.MethodPut, "delete": http.MethodDelete}[c.Operation]
			status, err := request(method, path, c.Data)
			if err != nil {
				t.Fatal(err)
			}
			if method == http.MethodPut && status == http.StatusOK {
				// Clean up even an unexpectedly allowed write, always within this run's fixture.
				defer func() {
					if status, err := request(http.MethodDelete, path, nil); err != nil || status != http.StatusOK {
						t.Errorf("cleanup of %s failed: HTTP %d, %v", path, status, err)
					}
				}()
			}
			got := stateError
			switch status {
			case http.StatusOK:
				got = stateAllowed
			case http.StatusUnauthorized, http.StatusForbidden:
				got = stateDenied
			}
			if got != c.Expect {
				t.Error(fmt.Sprintf("%s %s: expected %s, got %s (HTTP %d)", c.Operation, c.Path, c.Expect, got, status))
			}
		})
	}
}

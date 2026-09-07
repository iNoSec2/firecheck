package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
)

type ruleCase struct {
	Name      string          `json:"name"`
	Operation string          `json:"operation"`
	Path      string          `json:"path"`
	Data      json.RawMessage `json:"data,omitempty"`
	Expect    string          `json:"expect"`
}

func loadRuleCases(data []byte) ([]ruleCase, error) {
	var suite struct {
		Cases []ruleCase `json:"cases"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&suite); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.New("expected one JSON document")
	}
	if len(suite.Cases) == 0 {
		return nil, errors.New("add at least one rule case")
	}
	seen := map[string]bool{}
	for _, c := range suite.Cases {
		if strings.TrimSpace(c.Name) == "" || seen[c.Name] {
			return nil, errors.New("each case needs a unique nonempty name")
		}
		seen[c.Name] = true
		if c.Expect != stateAllowed && c.Expect != stateDenied {
			return nil, fmt.Errorf("%s: expect must be allowed or denied", c.Name)
		}
		if c.Operation != "read" && c.Operation != "write" && c.Operation != "delete" {
			return nil, fmt.Errorf("%s: operation must be read, write or delete", c.Name)
		}
		if c.Operation == "write" && (len(c.Data) == 0 || string(bytes.TrimSpace(c.Data)) == "null") {
			return nil, fmt.Errorf("%s: writes need non-null data", c.Name)
		}
		if c.Operation != "write" && len(c.Data) != 0 {
			return nil, fmt.Errorf("%s: data is only supported for writes", c.Name)
		}
		if c.Path == "/" && c.Operation == "read" {
			continue
		}
		if !(strings.HasPrefix(c.Path, "/firecheck_tests/{run}/") || strings.HasPrefix(c.Path, "/locked/{run}/")) || strings.Count(c.Path, "{run}") != 1 {
			return nil, fmt.Errorf("%s: paths must be inside /firecheck_tests/{run}/ or /locked/{run}/; only reads may use /", c.Name)
		}
		if strings.ContainsAny(c.Path, "?#\\%") {
			return nil, fmt.Errorf("%s: use a plain fixture path without URL escapes or queries", c.Name)
		}
		for _, part := range strings.Split(strings.TrimPrefix(c.Path, "/"), "/") {
			if part == "" || part == "." || part == ".." {
				return nil, fmt.Errorf("%s: empty or relative path segments are not supported", c.Name)
			}
		}
	}
	return suite.Cases, nil
}

func localEmulatorURL(address string) (url.URL, error) {
	host, portText, err := net.SplitHostPort(address)
	port, portErr := strconv.Atoi(portText)
	if err != nil || portErr != nil || !isLoopback(host) || port < 1 || port > 65535 {
		return url.URL{}, errors.New("FIREBASE_DATABASE_EMULATOR_HOST must use a literal loopback IP and port; run through emulators:exec")
	}
	return url.URL{Scheme: "http", Host: address, RawQuery: "ns=demo-firecheck-default-rtdb"}, nil
}

func TestRuleCaseFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/rule-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loadRuleCases(data); err != nil {
		t.Fatal(err)
	}
}

func TestRuleCaseValidation(t *testing.T) {
	for _, data := range []string{
		`{"cases":[]}`,
		`{"cases":[{"name":"root write","operation":"write","path":"/","data":{},"expect":"allowed"}]}`,
		`{"cases":[{"name":"escape","operation":"delete","path":"/firecheck_tests/{run}/../other","expect":"allowed"}]}`,
		`{"cases":[{"name":"typo","operation":"reed","path":"/","expect":"denied"}]}`,
		`{"cases":[{"name":"missing data","operation":"write","path":"/firecheck_tests/{run}/x","expect":"allowed"}]}`,
		`{"cases":[{"name":"unknown","operation":"read","path":"/","expect":"denied","typo":true}]}`,
		`{"cases":[{"name":"duplicate","operation":"read","path":"/","expect":"denied"},{"name":"duplicate","operation":"read","path":"/","expect":"denied"}]}`,
	} {
		if _, err := loadRuleCases([]byte(data)); err == nil {
			t.Fatalf("invalid cases accepted: %s", data)
		}
	}
}

func TestEmulatorAddressValidation(t *testing.T) {
	for _, address := range []string{"", "example.invalid:19000", "localhost:19000", "127.0.0.1:0", "127.0.0.1:99999", "http://127.0.0.1:19000", "user@127.0.0.1:19000"} {
		if _, err := localEmulatorURL(address); err == nil {
			t.Fatalf("unsafe address accepted: %s", address)
		}
	}
	for _, address := range []string{"127.0.0.1:19000", "[::1]:19000"} {
		if _, err := localEmulatorURL(address); err != nil {
			t.Fatal(err)
		}
	}
}

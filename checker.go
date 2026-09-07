package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	stateAllowed = "allowed"
	stateDenied  = "denied"
	stateError   = "error"
)

type result struct {
	URL    string
	State  string
	Status int
	Detail string
}

func parseTarget(raw string) (*url.URL, error) {
	u, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return nil, errors.New("use a complete HTTP or HTTPS database root URL without embedded credentials")
	}
	if u.Path != "" && u.Path != "/" && u.Path != "/.json" {
		return nil, errors.New("use the database root URL; nested paths are not supported")
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return nil, errors.New("invalid URL query")
	}
	for key := range query {
		if key != "ns" || !isLoopback(u.Hostname()) || len(query[key]) != 1 || query.Get(key) == "" {
			return nil, errors.New("URL queries are only supported for the local emulator namespace (?ns=demo-firecheck)")
		}
	}
	return u, nil
}

func isLoopback(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func newClient(cfg config) *http.Client {
	// Keep the existing transport limits; performance work targets local reporting.
	transport := &http.Transport{
		MaxIdleConns:    30,
		IdleConnTimeout: time.Second,
		DialContext:     (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
	}
	if cfg.Proxy != nil {
		transport.Proxy = http.ProxyURL(cfg.Proxy)
	}
	return &http.Client{
		Transport:     transport,
		Timeout:       5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func checkRead(ctx context.Context, raw string, cfg config, client *http.Client) result {
	u, err := parseTarget(raw)
	if err != nil {
		// Invalid input can contain credentials. Do not echo it into logs or reports.
		return result{URL: "<invalid URL>", State: stateError, Detail: err.Error()}
	}
	r := result{URL: u.String(), State: stateError}
	u.Path = "/.json"
	u.RawPath = ""
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		r.Detail = "could not create the read request"
		return r
	}
	req.Header.Set("User-Agent", "firecheck")
	for name, values := range cfg.Headers {
		req.Header[name] = append([]string(nil), values...)
	}
	resp, err := client.Do(req)
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			r.Detail = "request canceled"
		case errors.Is(err, context.DeadlineExceeded):
			r.Detail = "request timed out after 5 seconds"
		default:
			r.Detail = "request failed; check the connection, proxy and TLS certificate"
		}
		return r
	}
	defer resp.Body.Close()
	r.Status = resp.StatusCode
	switch resp.StatusCode {
	case http.StatusOK:
		r.State = stateAllowed
	case http.StatusUnauthorized, http.StatusForbidden:
		r.State = stateDenied
	default:
		r.Detail = "unexpected HTTP status: " + resp.Status
	}
	return r
}

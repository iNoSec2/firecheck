package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	flag "github.com/spf13/pflag"
)

var errHelp = errors.New("help requested")

type config struct {
	URL, Output string
	Proxy       *url.URL
	Headers     http.Header
	Workers     int
	Simple      bool
	Verbose     bool
}

func parseConfig(args []string, out io.Writer) (config, error) {
	var cfg config
	var headers []string
	var proxy, user string
	var help, randomAgent bool
	fs := flag.NewFlagSet("firecheck", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVarP(&cfg.URL, "url", "u", "", "Database root URL; otherwise read one URL per line from stdin")
	fs.StringVarP(&cfg.Output, "output", "o", "", "Append URLs with allowed reads to this file (also with --simple)")
	fs.StringArrayVarP(&headers, "header", "H", nil, "Request header in 'Name: value' format; may be repeated")
	fs.StringVarP(&proxy, "proxy", "p", "", "HTTP or HTTPS proxy URL")
	fs.IntVarP(&cfg.Workers, "workers", "w", 50, "Concurrent workers, from 1 to 99")
	fs.BoolVarP(&cfg.Simple, "simple", "s", false, "Print only URLs with allowed reads")
	fs.BoolVarP(&cfg.Verbose, "verbose", "v", false, "Print HTTP status details to stderr")
	fs.BoolVarP(&help, "help", "h", false, "Show help and examples")
	fs.StringVarP(&user, "user", "m", "", "Removed: remote write and delete probes are no longer performed")
	fs.BoolVarP(&randomAgent, "random-agent", "r", false, "Deprecated compatibility option; uses the firecheck user agent")
	_ = fs.MarkHidden("user")
	_ = fs.MarkHidden("random-agent")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if help {
		_, err := fmt.Fprint(out, "firecheck: Firebase Realtime Database read checks\n\nUsage:\n  firecheck --url URL [options]\n  firecheck [options] < urls.txt\n\nLocal emulator example:\n  firecheck -u 'http://127.0.0.1:9000/?ns=demo-firecheck'\n\nReads only. Write/delete rule tests run separately in the local emulator.\nTLS certificates are verified. Request timeout: 5 seconds.\n\nOptions:\n"+fs.FlagUsages()+"\nStates: allowed, denied, error, not-run.\nExit codes: 0 completed, 1 operational error, 2 usage error, 130 interrupted.\n")
		if err != nil {
			return cfg, err
		}
		return cfg, errHelp
	}
	if fs.NArg() != 0 {
		return cfg, errors.New("unexpected positional argument; use --url URL")
	}
	if fs.Changed("user") {
		return cfg, errors.New("--user was removed with remote write/delete probes; see README for local rule tests")
	}
	if cfg.Workers < 1 || cfg.Workers > 99 {
		return cfg, errors.New("--workers must be between 1 and 99")
	}
	if proxy != "" {
		parsed, err := url.Parse(proxy)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.Fragment != "" {
			return cfg, errors.New("--proxy must be a valid HTTP or HTTPS URL")
		}
		cfg.Proxy = parsed
	}
	cfg.Headers = make(http.Header)
	for _, header := range headers {
		name, value, ok := strings.Cut(header, ":")
		name, value = strings.TrimSpace(name), strings.TrimSpace(value)
		if !ok || !validHeaderName(name) || strings.ContainsAny(value, "\r\n\x00") {
			return cfg, errors.New("invalid --header; use 'Name: value' without control characters")
		}
		cfg.Headers.Set(name, value)
	}
	if cfg.URL != "" {
		if _, err := parseTarget(cfg.URL); err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}

func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", c)) {
			return false
		}
	}
	return true
}

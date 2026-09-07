# firecheck

A small Go CLI for checking read access to Firebase Realtime Database, created by [@bp0lr](https://github.com/bp0lr).

Give it a database root URL or a file through stdin. It prints a readable result, keeps diagnostics separate, and can save URLs with allowed reads.

**The CLI performs reads only.** Write and delete checks live in the local emulator test suite. Use remote read checks only with databases you own or have permission to assess.

## Install

Requires **Go 1.27.1 or newer**. Get Go from [go.dev](https://go.dev/dl/). The minimum version is recorded in `go.mod`, and CI reads it from the same file.

From this checkout:

```sh
go version
go install .
firecheck --help
firecheck --version
```

Go installs the command into `GOBIN`, or `GOPATH/bin` when `GOBIN` is unset. Add that directory to your `PATH` if your shell cannot find `firecheck`. With automatic toolchain selection enabled, Go can download the required toolchain for this module.

To keep the executable inside the project instead:

```sh
# Linux or macOS
go build -o bin/firecheck .
./bin/firecheck --help
```

```powershell
# Windows PowerShell
go build -o bin/firecheck.exe .
.\bin\firecheck.exe --help
```

These instructions build the current checkout. Use a published version only after the maintenance changes have been released. The old `go get -u` installation command is obsolete; see [Go's installation guidance](https://go.dev/doc/go-get-install-deprecation).

## Try it locally

The repository includes a Firebase Realtime Database emulator configuration. For this example, install **Node.js 24** and **Java 21** alongside Go. The first start downloads the Firebase CLI and emulator.

In one terminal, run:

```sh
npx --yes firebase-tools@15.29.0 emulators:start --only database --project demo-firecheck
```

In another terminal, from the same checkout:

```sh
go run . -u "http://127.0.0.1:19000/?ns=demo-firecheck-default-rtdb"
```

Expected result:

```text
http://127.0.0.1:19000/?ns=demo-firecheck-default-rtdb => R: denied | W: not-run | D: not-run
```

That is a successful check: the fixture denies reads at the root. Its `firecheck_tests` child permits local test data so the integration suite can verify write and delete behavior separately. Stop the emulator with Ctrl+C when finished.

The emulator uses a demo project, binds to `127.0.0.1:19000`, and does not require a Firebase account. See [Firebase's emulator documentation](https://firebase.google.com/docs/emulator-suite/connect_rtdb).

## Everyday usage

Pass a complete root URL with `-u`, or put one URL per line in `urls.txt`. Blank lines and surrounding whitespace are ignored.

```sh
# Linux or macOS
firecheck -s -o results.txt < urls.txt
```

```powershell
# Windows PowerShell
Get-Content urls.txt | firecheck -s -o results.txt
```

`-s` prints only URLs whose read check was allowed. `-o` appends those same URLs to the file, including when `-s` is enabled. A run with no allowed reads leaves no new lines in that file. Existing results are preserved; repeated URLs are not deduplicated.

Use `-v` for HTTP status details on stderr. You can pass a custom header with `-H "Name: value"` and repeat `-H` for additional headers. Treat files containing authenticated results as private.

## Options

| Option | Default | What it does |
| --- | --- | --- |
| `-u`, `--url` | stdin | Check one database root URL. |
| `-o`, `--output` | none | Append URLs with allowed reads to a file. |
| `-s`, `--simple` | off | Print only URLs with allowed reads. |
| `-v`, `--verbose` | off | Send HTTP status details to stderr. |
| `-H`, `--header` | none | Add a header in `Name: value` format; repeat as needed. |
| `-p`, `--proxy` | none | Use an explicit HTTP or HTTPS proxy. |
| `-w`, `--workers` | `50` | Set concurrent workers, from `1` to `99`. |
| `-h`, `--help` | | Show usage and a local example. |
| `--version` | | Show the application version, source commit and Go toolchain, then exit. |

Requests have a five-second timeout. TLS certificates are verified, and redirects are not followed. Environment proxy variables are not used. Ctrl+C cancels pending requests and flushes buffered file output; the report can be incomplete.

## Read the results

When reporting a bug, include `firecheck --version`. Local builds show `dev` until installed from a tagged module version. The commit is `unknown` if build metadata is unavailable; `(modified)` means the build included uncommitted changes. This command does not read stdin or make requests.

| State | Meaning |
| --- | --- |
| `allowed` | The root read returned HTTP 200. |
| `denied` | The root read returned HTTP 401 or 403. |
| `error` | Input, connection, TLS, timeout, or unexpected HTTP status prevented a conclusive check. Details go to stderr. |
| `not-run` | The operation was not attempted. The CLI always reports this for write and delete. |

The default output includes denied and failed checks. Simple output and the output file contain only allowed URLs. With multiple workers, results arrive in completion order.

| Exit code | Meaning |
| --- | --- |
| `0` | Checks completed, including results that were denied. Help also exits with `0`. |
| `1` | An operational error occurred, such as a failed request, input read, or output write. |
| `2` | Invalid options, an invalid `--url`, or no URLs were provided. |
| `130` | The run was interrupted. An output flush or close failure takes precedence and returns `1`. |

## Limits and changes from the 2020 version

- This checks **Realtime Database**, not Cloud Firestore or Cloud Storage.
- A result describes the requested root and supplied headers. It does not prove that every child path has the same permissions, or that access is anonymous when authentication headers were supplied.
- HTTP 200 alone does not prove that the server is Firebase. Use known database endpoints. The command does not validate or save response bodies.
- Root URLs may end in `/` or `/.json`. Nested paths, embedded credentials, and remote query parameters are rejected instead of silently discarded. The emulator accepts a single `ns` query parameter on a literal loopback IP.
- Remote write and delete probes have been removed. The old fixed probe path is no longer touched. `-m` / `--user` now returns an explanatory usage error.
- `-r` / `--random-agent` remains accepted for old scripts but has no effect. The default user agent identifies the tool as `firecheck`.
- TLS validation is now enabled. For a private CA, configure trust in the operating system instead of bypassing validation.
- Input lines use Go's scanner limit of approximately 64 KiB; overlong input is reported as an error.

## Development

Fast checks use temporary files and local HTTP test servers:

```sh
go test ./...
go vet ./...
go build ./...
```

Run the actual Firebase rule integration test with:

```sh
npx --yes firebase-tools@15.29.0 emulators:exec --only database --project demo-firecheck "go test -tags=integration -run TestEmulatorRules -v ./..."
```

This test requires the emulator environment set by `emulators:exec`. It only accepts a literal loopback IP and uses the fixed `demo-firecheck-default-rtdb` namespace. Test writes use unique child keys and are cleaned up. The fixture in `testdata/database.rules.json` is for local tests only.

CI builds and tests on Linux, Windows, and macOS. A separate Linux job runs the race detector and emulator suite. To run the race detector locally, use `go test -race ./...` with a supported C compiler installed.

### Local performance

```sh
go test -run "^$" -bench "BenchmarkReportFile|BenchmarkParseTarget" -benchtime=100000x -count=3
```

The report benchmark compares direct file writes with the 32 KiB buffer used by the CLI. It includes the final buffer flush and makes no network requests.

On Windows amd64 with Go 1.27.1 and a Ryzen 9 3900X, three runs measured about **3,172 ns/result without buffering** and **200 ns/result with buffering** at the median. That is roughly **16x faster for local report writing**, with the same 32 B and two allocations per result. This is not an end-to-end speedup claim; network latency and filesystem behavior will affect real runs.

### Repository layout

| File | Responsibility |
| --- | --- |
| `main.go` | Input, workers, cancellation, and exit codes. |
| `config.go` | Flags, help, and configuration validation. |
| `checker.go` | Read requests, TLS, and result classification. |
| `output.go` | Console and file report formatting. |
| `main_test.go`, `output_test.go` | Regression tests and local benchmarks. |
| `emulator_test.go` | Optional integration tests using the Firebase emulator. |

Generated binaries and historical report files are excluded from version control. Release binaries should be built from a tested commit and attached to a versioned GitHub release.

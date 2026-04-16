### ezBastion — Development Guidelines (project-specific)

#### Build and configuration
- Go toolchain: module `ezBastion`, `go 1.15` as declared in `go.mod`. Newer Go versions generally work, but stick to 1.15–1.20 if you need byte-for-byte identical binaries.
- Microservices layout:
  - Binaries live under `cmd/ezb_*` (e.g., `cmd/ezb_srv`, `cmd/ezb_db`, `cmd/ezb_wks`, …).
  - Shared libraries live under `pkg/*`.
- Windows-centric build pipeline: `Makefile.ps1` provides functions to bump versions and build all/one service. Typical usage (PowerShell):
  - Build all: `do-gobuild`
  - Build one: `do-gobuild ezb_srv`
  - Zip all Windows binaries: `do-gozip`
  - Internals: It updates `cmd/*/versioninfo.json` via `upgrade-semver`, runs `go fmt`, `go generate`, and `go build -o ./bin`.
- GCC requirement for SQLite (only if you build `ezb_db`): you need a C compiler for the SQLite driver. On Windows, use TDM-GCC as referenced in the top-level `README.md`.
- Private dependency: `github.com/chavers/ezb_priv` is referenced in `go.mod`. Some `cmd/*` or `pkg/*` may import it. To build components that need it, configure access:
  - `git config --global url.ssh://git@github.com/.insteadof https://github.com/`
  - `go env -w GOPRIVATE=github.com/chavers/*`
  - Ensure your SSH keys or GH token grants access.
  - If you only work on subsystems that don’t import it, you can build/test those packages in isolation and avoid fetching the private module.
- Platform-specific code: several packages use OS-specific files (e.g., `pkg/logmanager/logmanager_linux.go`, `pkg/logmanager/logmanager_windows.go`). On non-Windows/non-Linux hosts (e.g., macOS/darwin), entire packages may be excluded by build tags. Prefer targeted builds/tests for specific packages instead of `./...` when working on macOS.

#### Testing — configuration and execution
- There are no repo-wide tests by default; add per-package tests where it makes sense.
- Avoid sweeping `go test ./...` on macOS: packages with only `*_linux.go`/`*_windows.go` will fail to build on darwin with “build constraints exclude all the Go files”. Instead, test only the packages you touch (example below).
- Minimal conventions:
  - Name files `something_test.go`, package usually matches the implementation package.
  - Keep tests side-effect free when possible. Many services interact with the OS, so prefer testing pure helpers in `pkg/*`.

##### Example: add and run a simple test
This example targets `pkg/certmanager` because it has pure helpers without platform or external service requirements.

1) Create a test file `pkg/certmanager/certmanager_test.go` with the following content:

```go
package certmanager

import "testing"

// Demonstrates a side-effect-free test in this repo.
func TestNewCertificateRequest(t *testing.T) {
    addrs := []string{"example.com", "127.0.0.1", "::1"}
    csr := NewCertificateRequest("test-cn", addrs)

    if csr == nil {
        t.Fatalf("NewCertificateRequest returned nil")
    }
    if got := csr.Subject.CommonName; got != "test-cn" {
        t.Fatalf("CommonName = %q, want %q", got, "test-cn")
    }
    if len(csr.DNSNames) == 0 {
        t.Fatalf("expected at least one DNS SAN, got 0")
    }
    if len(csr.IPAddresses) == 0 {
        t.Fatalf("expected at least one IP SAN, got 0")
    }
}
```

2) Run just this package’s tests from the project root:

```bash
go test ./pkg/certmanager -run TestNewCertificateRequest -v
```

Expected successful output (times may vary):

```text
=== RUN   TestNewCertificateRequest
--- PASS: TestNewCertificateRequest (0.00s)
PASS
ok  	ezBastion/pkg/certmanager	0.01s
```

3) When adding new tests:
- Keep them constrained to the packages you modify; don’t force repo-wide builds on unsupported OSes.
- If a package is OS-gated and you need tests on macOS, either:
  - add a trivial `*_darwin.go` shim (no-op implementations) and mark with the right build tags; or
  - run tests under a supported GOOS/GOARCH, e.g., `GOOS=linux GOARCH=amd64 go test ./pkg/logmanager` inside a container/VM.
- If tests require the private module, ensure your environment has access as described above.

#### Additional development information
- Code style:
  - Follow gofmt/goimports; the PowerShell build runs `go fmt` automatically for services.
  - Mirror existing naming and file layout; keep functions small and focused as in `pkg/confmanager` and `pkg/certmanager`.
- Versioning/build metadata:
  - Each service under `cmd/ezb_*` has a `main.go` exposing a `VERSION` and a `versioninfo.json` used by `upgrade-semver` to bump build numbers and generate `bin/allver.json`.
  - To locally build a single service without the PowerShell helpers, you can run: `go build -o ./bin ./cmd/ezb_srv`. Prefer the PowerShell helpers on Windows to keep metadata consistent.
- Configuration defaults:
  - `pkg/confmanager.CheckConfig` synthesizes sane defaults when the TOML file is absent, using the host FQDN and standard ports. Understanding these defaults helps when you’re standing up services locally.
- Logging:
  - `pkg/logmanager` provides platform-specific log integrations (e.g., Windows Event Log). Be mindful of build tags when importing it from code that must run cross-platform.
- Binaries runtime hints (from `README.md`):
  - Example lifecycle for `ezb_wks` on Windows PowerShell:
    - `ezb_wks install`
    - `ezb_wks start`

This document focuses on project-specific constraints: per-OS build tags, private deps, PowerShell build pipeline, and package-scoped testing. Use targeted `go test` invocations to avoid unrelated platform breakages.

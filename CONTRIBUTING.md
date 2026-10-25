# Contributing to Bootimus

Thanks for your interest in contributing. This guide covers how the project is built, the conventions we follow, and what to check before opening a PR. If something here doesn't fit your change, mention it in the PR description and we'll work it out.

## Repository layout

Bootimus is a monorepo. Several related but independently-built pieces live in this one tree:

- **[cmd/](cmd/) and [internal/](internal/)** — the main Go server binary (`bootimus`). The bulk of the project. Built with `make build`.
- **[web/](web/)** — the admin UI. Vanilla HTML, CSS, and ES modules; embedded into the Go binary at build time via `go:embed`. No build step.
- **[website/](website/)** — the marketing site at bootimus.com. A separate Node/Next project with its own [package.json](website/package.json) and release pipeline (`make build-website` / `make push-website`). Versioned independently.
- **[appliance/](appliance/)** — scripts and overlay used to produce the Bootimus appliance image (a turnkey ISO/disk image). Built with `make appliance`.
- **[distro-profiles.json](distro-profiles.json) and [tools-profiles.json](tools-profiles.json)** — the source-of-truth profile data, embedded into the Go binary via `make sync-profiles`. Versioned independently from the app.
- **[docs/](docs/)** — documentation.

Most PRs touch only one of these. If yours spans more than one (for example, a new feature that needs a server change *and* an admin-UI change), keep them in a single PR but call out the cross-cutting nature in the description.

## Build and run

```
make build      # current platform
make run        # build and run
make clean
```

Cross-compile targets are in the [Makefile](Makefile). The Go toolchain version is pinned in [go.mod](go.mod) — please match it.

## Go code

- **Format with `gofmt`** and make sure files end with a trailing newline.
- **Prefer deletion over preservation.** If you spot a function with no callers while working nearby, removing it is welcome. New code shouldn't add functions without callers.
- **Hold off on abstractions until they pay for themselves.** Three similar lines is usually fine; pull out a helper once there's a third real caller.
- **Validate at system boundaries**, not between two internal functions. Trust your callers and framework guarantees in internal code paths.
- **No comments.** Function and variable names carry the meaning. The only things that look like comments but stay are compiler directives (`//go:build`, `//go:embed`, `//go:generate`, `//nolint`) and `// Code generated ...; DO NOT EDIT.` markers.
- **Wrap errors with `%w`** in sentence form: `fmt.Errorf("listen UDP/67: %w", err)`. Lowercase, no trailing punctuation.
- **Logging** uses the standard `log` package. Lowercase messages, no trailing punctuation.

## Language

- **British English throughout** — documentation, user-facing strings, log messages, commit messages, and identifiers (`normalise`, `colour`, `behaviour`, `initialise`).
- Spellings fixed by an external standard stay as the standard writes them: the HTTP `Authorization` header, `authorized_keys`, CSS properties, and upstream API and library names (`cobra.OnInitialize`, wimlib's `optimize` subcommand).
- Existing URL paths keep their current spelling (`/api/iso-catalog`) — renaming an endpoint is a breaking change, not a spelling fix.

## Tests

We're building test coverage incrementally — there's no expectation that every PR ships with tests, but contributions that add them are appreciated.

- **Pure logic is the easiest place to start.** Functions that take inputs and return outputs (matching, parsing, formatting, validation) test well. See [internal/profiles/manager_test.go](internal/profiles/manager_test.go) for the pattern: table-driven, real input fixtures, no mocks.
- **Avoid mocking the database.** If a test needs storage, it's usually cleaner to refactor the function under test to take a slice or a small local interface rather than the full `storage.Storage`.
- **Stick to the standard library `testing` package.** `t.Errorf` and `t.Fatalf` cover what we need; no extra runners or assertion libraries.
- **Flakes get fixed or deleted.** A test that passes 99/100 times is worse than no test.
- **Run `go test ./...`** before pushing.
- **Integration tests** that need an external service skip themselves unless it's available. `make test-integration` starts a throwaway OIDC provider ([mock-oauth2-server](https://github.com/navikt/mock-oauth2-server)) in Docker and runs the OIDC sign-in tests against it; CI runs this on every pull request.

If your PR changes behaviour in a tested package, ideally add a test that fails on the old behaviour and passes on the new. If that's hard without an invasive refactor, just call it out in the PR description.

## End-to-end boot test

`make test-e2e` boots real virtual machines against a real bootimus. It builds the binary, starts it with QEMU's user-mode network as the boot network (the guest sees the host as 10.0.2.2), and runs these stages:

| Stage | What it asserts |
|-------|-----------------|
| BIOS menu | SeaBIOS's iPXE ROM fetches `undionly.kpxe`, the embedded script chains `autoexec.ipxe`, the client is auto-discovered with hardware inventory, the menu is requested and its title appears on the serial console |
| UEFI menu | The same through OVMF with `bootimus.efi`. If the installed OVMF has no PXE stack, iPXE is launched from a FAT volume instead and the test says so |
| Alpine next boot | An Alpine ISO is downloaded through the admin API with SHA-256 verification, extracted, given Alpine's netboot initramfs as an initrd override, set as the client's next boot, and booted. The kernel and initrd are served over HTTP, an apkovl overlay fetches the install callback URL from the auto-install endpoint, reports completion and powers the VM off, and the install record ends up completed |
| proxyDHCP | Only with `BOOTIMUS_E2E_PROXYDHCP=1`: a bridge, a tap device and a dnsmasq DHCP server in its own network namespace are created with `sudo`; bootimus answers with proxyDHCP on the bridge and the VM boots from it |

You need `qemu-system-x86_64`, an OVMF firmware (the `ovmf` or `edk2-ovmf` package) and, for the proxyDHCP stage, passwordless `sudo`, `iproute2` and `dnsmasq`. KVM is used when `/dev/kvm` is accessible; without it the VMs run under TCG with longer timeouts. The Alpine files are cached under `~/.cache/bootimus-e2e` (override with `BOOTIMUS_E2E_CACHE`) and only downloaded once.

Bootimus must answer TFTP on UDP/69, because PXE firmware has no port option. If the test binary cannot bind it, the suite falls back to QEMU's built-in TFTP for the loader and `autoexec.ipxe` and logs that the TFTP stage was not exercised. To run the full path locally:

```bash
make e2e-binary
sudo setcap cap_net_bind_service=+ep .e2e/bootimus
BOOTIMUS_E2E_BINARY=$PWD/.e2e/bootimus BOOTIMUS_E2E_REQUIRE_TFTP=1 make test-e2e
```

`BOOTIMUS_E2E_REQUIRE_TFTP=1` turns the fallback into a failure, which is what CI uses. Ports default to 8080 for boot HTTP and 18081 for the admin API; `BOOTIMUS_E2E_HTTP_PORT` and `BOOTIMUS_E2E_ADMIN_PORT` move them if they clash. Every stage writes its serial console and the server log under `.e2e/run/`, which CI uploads as an artefact when the job fails.

## Cross-platform code

Bootimus builds for Linux (primary), macOS, and Windows. When a syscall, path, or API differs by OS:

- Use Go build tags rather than runtime `runtime.GOOS` checks: `//go:build !windows` and `//go:build windows` in separate files named `foo_unix.go` / `foo_windows.go`.
- On Windows, prefer `golang.org/x/sys/windows` over the frozen `syscall` package. One exception: `syscall.Handle` for socket FD conversion has no `x/sys/windows` equivalent.
- If a function has no callers on a given platform, deleting it is preferable to porting it.

## Frontend (admin UI)

The admin UI in [web/static/](web/static/) is intentionally vanilla — plain HTML, CSS, and ES modules with no build step, framework, or bundler. Please keep it that way:

- No npm dependencies, build steps, or frameworks in the admin UI.
- Match the existing CSS variable system and component patterns rather than reskinning sections in isolation.
- Action buttons live in toolbars, not in card titles or section headers.
- View-mode toggles use an icon plus tooltip rather than a one-word label like "Tree".

The marketing site under [website/](website/) is a separate codebase with its own `package.json`. We use **yarn** there, not npm.

## Configuration

There's one example config, [bootimus.example.yaml](bootimus.example.yaml). Rather than adding OS- or scenario-specific copies, please extend the existing example with a comment explaining when to enable any new keys.

## Distro profiles

[distro-profiles.json](distro-profiles.json) and [tools-profiles.json](tools-profiles.json) are the source of truth for boot behaviour. Please bump the `version` field when you change them so caches invalidate cleanly.

## Before opening a PR

- [ ] `gofmt -l .` is clean
- [ ] `go build ./...` passes
- [ ] `go vet ./...` is clean
- [ ] `go test ./...` passes
- [ ] If you touched anything platform-specific: `GOOS=windows go build ./...` and `GOOS=darwin go build ./...` both pass
- [ ] Files end with a trailing newline
- [ ] No new dependencies unless the PR description explains why
- [ ] PR title is short and describes the change

## A few things we'd rather avoid

- **GitHub Actions and release automation.** Releases are cut manually — please don't add workflow files.
- **Drive-by formatting or refactoring** in code unrelated to your PR. It makes review harder.
- **Marketing-style copy** in READMEs or release notes. Plain descriptions of what changed read better.
- **Decorative emoji** in code, comments, commit messages, or docs.

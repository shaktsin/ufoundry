# App-only coding compute plan

Delivery order: complete and verify Milestones 1–6 on macOS first. Then repeat
the platform-specific runtime, integration, and verification work for Linux
(x86_64 and ARM64) as a separate follow-on phase. Linux build work already
explored is retained, but does not gate the macOS track. No user-facing CLI,
browser UI, Docker, or separately installed VM runtime. Platform-specific
runtime pieces stay private behind one UMCode API.

## Milestone 1 — Bundled microVM runtime

Build a private runtime bridge and package all runtime libraries, guest kernel,
and base environment with each app build. Pin stable libkrun/libkrunfw sources;
do not build from libkrun's active 2.x development branch. Keep macOS and Linux
implementations behind the same JSON request protocol, even if their internal
launch mechanisms differ: Linux can use a private `crun` krun handler; macOS
needs an app-private libkrun/HVF bridge because crun's krun handler is a Linux
OCI runtime. Neither helper is a supported user command. Signing and Linux KVM
availability must be checked before enabling compute.

Source baseline: libkrun 1.19.4 and libkrunfw 5.5.0, pinned to full upstream
commits in `sources.lock`. The ABI pairing still needs runtime smoke validation.
Bundle notices and the exact kernel/library source offer with app releases.

**Verify before continuing (macOS phase):** build the macOS desktop package and
run a smoke command in a fresh VM; prove the project mount works; prove the
host home directory is not visible; prove network-off is actually offline;
prove cancellation terminates the guest; record cold-start time and packaged
size. Linux checks belong to the follow-on Linux phase.

## Milestone 2 — Per-task isolated Git workspaces

Create a managed checkout for each coding task, with an explicit starting point
(committed `HEAD` plus staged/unstaged edits and non-ignored untracked files).
Keep it separate from the project checkout and retain it for review. Its Git
metadata must be self-contained: a native `git worktree` `.git` pointer refers
back to the original checkout and would not work in a VM that mounts only the
task workspace.

**Verify before continuing:** concurrent tasks get distinct paths; the original
checkout remains byte-for-byte unchanged; cancellation leaves a recoverable
worktree; invalid/non-Git projects get a clear in-app explanation.

## Milestone 3 — Scope every coding operation to that workspace

Thread the task workspace through file read/list/write, search, shell, tests,
and diff. Project instructions may be read from the original project, but agent
writes and command execution must stay inside the worktree. Never mount the
original project or the user's home directory into the guest.

**Verify before continuing:** end-to-end coding task edits and tests only the
worktree; traversal and symlink escape tests fail closed; project-level file
diff/revert features still behave correctly.

## Milestone 4 — Live progress and cancellation

Stream assistant response, tool lifecycle, command output, and test progress as
events. Add cancellation from chat through the engine to the running process
and VM. Expose concise action/progress summaries and logs, not hidden model
reasoning.

**Verify before continuing:** long-running command output appears before exit;
cancel stops the guest promptly; disconnect/reconnect restores task status.

## Milestone 5 — Resource, network, and secret controls

Apply per-project CPU, memory, wall-time, and writable-workspace limits. Do not
pass provider credentials or ambient host environment variables into the guest.
Make network-off a real deny policy, not a prompt or shell-command heuristic;
dependency downloads should have an explicit, auditable permission path.

**Verify before continuing:** resource exhaustion is bounded; deny-mode cannot
reach the internet or host-only services; allow-mode grants only the intended
network path; secret-canary tests do not expose host credentials.

## Milestone 6 — Review, keep, and discard

Show changed files, diff, and test results from the task workspace. Let the
user apply task changes to the project checkout, or discard the workspace after
an explicit confirmation. Do not commit, push, merge, or open a PR automatically.

**Verify before release:** accept/reject flow is recoverable, never changes the
source checkout before user approval, and passes an end-to-end UI test.

## Execution log — 2026-09-24

Mac phase — Milestone 1 has passed. On this Apple-silicon Mac, the staged libkrun
1.19.4/libkrunfw 5.5.0 bridge boots Alpine 3.24.0, mounts the project at
`/workspace`, cannot see the host home directory, cannot reach the network when
network is disabled, and stops immediately on cancellation. One cold boot took
0.12s; the private runtime stage was 39 MB. These are smoke measurements, not a
performance guarantee.

The macOS runtime has passed fresh guest checks for project mount, host-home
invisibility, and network denial; cancellation previously stopped the guest
promptly. A complete ad-hoc-signed `.app` bundle built at 63 MB. Its bundled
engine reports the expected version, the app and private helper signatures
verify, the helper retains Apple's Hypervisor entitlement, and a VM booted from
the packaged helper/rootfs and printed `PACKAGED_VM_OK`. This closes the Mac M1
gate; M2 is now in progress.

Initial Linux exploration during the Mac phase established that Debian
12/GTK 4.8 is too old for Wails v3; the Linux builder now enforces GTK 4.14+.
The later Linux follow-on results and remaining KVM gate are recorded below.

Linux follow-on (2026-09-25) — ARM64 app packaging was repeated from the
current source with Git metadata available: the 64 MB portable folder builds, the desktop app is an
AArch64 ELF, the bundled engine starts and reports its version, and `ldd`
reports no missing shared libraries. The full Go suite passes in Debian Trixie
ARM64 after supplying the test fixture directory and a test-only Git identity.
The first suite attempt failed only because those container fixtures were
omitted; it passed on rerun. The x86_64 runtime build compiled the pinned
libkrunfw 6.12 kernel under QEMU emulation. The 64 MB x86_64 portable folder
also builds, is an x86-64 ELF, embeds the current
source version, starts its engine, and has no missing shared-library
dependencies in the app or private runtime. Both architectures now pass
runtime build, application packaging, architecture, and shared-library checks.
The ARM64 full Go test suite passes. Both Linux containers lack `/dev/kvm`,
however, so neither architecture can yet pass the required guest boot,
workspace-isolation, network-denial, cancellation, or end-to-end UI smoke gates
here. Linux is not release-verified until those checks run on native Linux/KVM
hosts for ARM64 and x86_64.

The full Go suite also passes in the x86_64 Debian Trixie container. These test
runs exercise the Linux app/backend code and architecture-specific packaging,
but are not substitutes for the KVM guest and UI integration gates above.

Linux readiness follow-up — Compute cannot be enabled for a project unless
`/dev/kvm` exists as an accessible character device; the bridge runner repeats
the check before launch to catch a revoked or unavailable device. Tests in both
Debian ARM64 and x86_64 containers, where KVM is absent, verify project
create/update rejection and the actionable runner error. Both full Go suites
pass after the change, and both 64 MB app folders were rebuilt with this gate
and exported to `/private/tmp/UMCode-linux-{amd64,arm64}-final3`.
Linux frontend validation also passes in Debian ARM64 (`svelte-check`: zero
errors/warnings; Vitest: 3/3), and `go vet ./...` passes in both Linux
architectures. These checks do not cover the KVM-dependent guest/UI smoke test.
The optional per-user desktop installer was also exercised in both containers;
each generated a desktop entry that points to the matching portable-folder
launcher without installing system packages.

Current supporting checks: full `go test ./...` passes with local socket access;
frontend Svelte check has zero errors/warnings and all 3 tests pass; Linux ARM64
package build passes; shell syntax, `gofmt`, and `git diff --check` pass.

Mac phase — Milestone 2 passed. Each task gets a stable private checkout with
self-contained shallow Git metadata, tracked edits, and non-ignored untracked
files; ignored files remain excluded. Tests cover distinct concurrent tasks,
source status/content preservation, persistence across task/source commits,
recovery after cancellation, and clear errors for non-Git/unborn repositories.

Mac phase — Milestone 3 passed. Project coding tools now resolve against the
task checkout, not the registered source folder. Change recording, per-turn
diff, and revert resolve to that same checkout. The end-to-end test confirms
the source file remains unchanged while the task copy is edited and reverted;
server integration tests pass. The guest receives the task root only, so its
Git metadata is available without mounting the original checkout.

Mac phase — Milestone 4 passed. Shell output is sent as item deltas while the
command is running and persisted on the in-progress tool item, so reconnecting
can recover current output. Existing turn/interrupt cancellation reaches the
shell process or kills the bundled bridge/guest. A regression test confirms
output arrives before command exit; server integration tests verify the JSON
RPC event and tool lifecycle path. Packaged microVM cancellation was also
smoke-tested during Milestone 1.

Mac phase — Milestone 5 passed. Per-project limits are configurable (1–8 vCPU,
512–8192 MiB memory, and 1024–16384 MiB workspace); defaults are 4 vCPU,
4096 MiB, and 2048 MiB. The existing shell timeout is capped at 10 minutes.
Guest environment variables are explicitly constructed and contain no host
credentials. With network disabled, shell commands fail closed unless the
project microVM is enabled; the guest has no network device unless explicitly
enabled. Workspace size is checked every 500 ms and cancels the guest on
overage. This disk ceiling is a monitored stop guard, not a kernel-enforced
quota, so a fast write can briefly exceed it before the next check. Go tests
cover resource validation, disk-triggered cancellation, offline fail-closed
behavior, and app-setting validation; macOS libkrun build-tag tests and
frontend checks pass.

Mac phase — Milestone 6 passed. Project chats expose task-root file inspection,
"Keep task changes in project," and a separately confirmed discard action.
Keep stages a patch from the task's synthetic baseline, preflights it against
the current source tree, applies it without committing, and advances the task
baseline so later turns can be kept separately. Conflicts fail closed. Discard
validates the private workspace marker and project identity before removal.
Server end-to-end coverage verifies task-scoped file reads, source isolation,
keep, discard, and no accidental source writes on discard; frontend typecheck
and tests pass. Linux runtime/package verification remains a separate follow-on.

## Verification and live-preview implementation - 2026-09-25

The agent now has a first-party `verification.plan` tool that reads the current
Git changes and configured Node, Go, Rust, and Python manifests without running
project code. It selects project-defined checks, records the reason and required
command for each, and routes configured browser checks separately. The
`verification.run` tool executes the resulting ordered plan, streams command
output, records directory, reason, duration, exit status, and evidence, and
distinguishes passed, failed, blocked, and not-run checks. A server-level
scripted-agent acceptance test exercises plan, intentional failure, diagnostic
fix, rerun, and pass through the real tool loop.

The compute bridge accepts one optional host-to-guest TCP mapping. A managed
`preview.start` tool launches a persistent dev server in the task microVM,
waits for readiness, streams logs, and opens a closeable Preview tab in the
third-column inspector. Reloading or closing the tab does not stop the server;
the UI has a separate Stop preview action. Preview processes are listed after
reconnect and are cancelled on engine shutdown. Because pinned libkrun 1.x
couples TSI port forwarding to its networking device, preview currently
requires the project's explicit network permission; it never silently falls
back to a host process.

The release rootfs builder now installs Git, ripgrep, Node/npm, Go,
Python/pip, compilers, certificates, and Chromium into the pinned Alpine image.
It preserves guest file modes through macOS copy-on-write packaging and each
disposable runtime clone, including the `1777` temporary directories required
by sandboxed browser workers. A fresh ARM64 runtime and application bundle were
built from that image. The packaged guest
served a real HTTP preview on guest port 4173, libkrun mapped it to a different
loopback host port, and the host fetched the expected workspace file. Explicit
stop/cancellation was also exercised. The signed packaged guest additionally
ran Chromium 152 headlessly with network disabled, rendered a workspace HTML
file, and returned a validated PNG screenshot through virtiofs.

The first-party `browser.verify` tool runs configured Playwright, Cypress, or
other browser commands only in compute. It reports missing packages or browser
runtimes as not run without installing them, extracts console/request failure
diagnostics, and inventories newly produced screenshots, traces, videos, JSON,
and HTML reports. Browser results open in a dedicated inspector view;
screenshots use an app-only, size-bounded image endpoint, while active content
and trace archives are listed by path rather than executed.

The rebuilt application passes strict
deep code-signature verification, retains the hypervisor entitlement, and has
no broken resource symlinks. The full Go suite, focused race tests, `go vet`,
Svelte diagnostics, frontend tests/build, shell syntax checks, and
`git diff --check` pass. A final interactive in-app visual walkthrough remains
a release QA step rather than an implementation blocker.

# PRD — GoLab

**Author:** Dhawal Shukla ([GolangForAll](https://golangforall.in))
**Status:** Draft
**Last updated:** September 19, 2026

---

## 1. Summary

GoLab is a lightweight, browser-based playground for writing and running Go code, with full stdin support — programs using `fmt.Scanln`, `bufio.Scanner`, etc. work exactly as they would on a real terminal. It's built as a tool for **GolangForAll**, a community for people who learn, build, teach, and contribute with Go (meetups around NCR/Kanpur/Lucknow, a community WhatsApp group, and a blog).

The frontend is a Next.js app with a Monaco editor; the backend is a single Go HTTP service that compiles and executes submitted code in an isolated temp workspace per request.

## 2. Problem statement

Existing Go playgrounds (e.g. the official Go Playground) are solid but generic — not branded for a specific learning audience, and don't always make stdin an obvious, first-class part of the workflow. GolangForAll wants a playground that:

- Is fast and frictionless — no login, no local setup.
- Makes reading input a visible, explicit step (a dedicated stdin box), not a hidden feature.
- Gives clear, immediate feedback on compile errors, runtime errors, exit code, and runtime — separating stdout from stderr.

## 3. Goals

- Compile and run a Go snippet end-to-end (network + compile + execute) quickly enough to feel instant for short programs.
- Support stdin-consuming programs via an explicit input field.
- Clearly separate and display stdout, stderr, exit code, and runtime.
- Fail predictably and legibly: compile errors, runtime panics, and timeouts should each be distinguishable in the UI.
- Be embeddable in GolangForAll tutorials as a "try it yourself" widget.

## 4. Non-goals (for now)

- Multi-file / multi-package Go projects (backend currently compiles a single `main.go`).
- User accounts, saved snippet history, authentication.
- Support for languages other than Go.
- Long-running, networked, or background processes (5-second run timeout by design).
- Arbitrary `go.mod` dependencies for user-submitted code.

## 5. Target users

- **Primary:** Learners following GolangForAll tutorials, blog posts, and meetup sessions who want to try a snippet immediately, including ones that read input.
- **Secondary:** Anyone wanting a fast, no-setup Go scratchpad — including people finding GoLab through the wider GolangForAll community (WhatsApp group, blog, meetups).

## 6. User stories

1. As a learner, I open GoLab and immediately see a working, runnable example.
2. As a learner, I edit the code and click Run, and see output within a few seconds.
3. As a learner whose program reads input, I type it into a dedicated stdin box before running, so the input path is obvious rather than hidden.
4. As a learner, when my program fails to compile, I see the compiler's stderr output distinctly from normal stdout.
5. As a learner, when my program panics or exits non-zero at runtime, I see that output and the non-zero exit code clearly.
6. As a learner, if my program runs too long, I get a clear "timed out" message instead of a hang or a silent failure.
7. As a learner, if the runner backend itself is unreachable, I get a distinct "no runner" state rather than a confusing blank output.

## 7. Functional requirements

| # | Requirement | Status |
|---|---|---|
| F1 | Editor supports Go syntax highlighting, pre-populated with a runnable example. | ✅ implemented |
| F2 | A stdin field lets the user supply input consumed by `os.Stdin`. | ✅ implemented |
| F3 | Run action POSTs `{ code, input }` to `${NEXT_PUBLIC_RUNNER_URL}/run` and disables itself while in flight. | ✅ implemented |
| F4 | Response `{ stdout, stderr, exitCode, runtimeMs }` renders stdout/stderr in visually distinct blocks. | ✅ implemented |
| F5 | Non-2xx responses and network failures are shown as distinct UI states from program stderr. | ✅ implemented (`error` vs `connection-error` states) |
| F6 | UI reflects run state: idle / running / ok / error / connection-error. | ✅ implemented (`StatusPill`) |
| F7 | Backend enforces size limits on code (10 KB), input (10 KB), and total request body (32 KB), rejecting oversized requests with a clear error. | ✅ implemented |
| F8 | Backend enforces separate compile (90 s) and run (5 s) timeouts, each surfaced as `exitCode 124` with a descriptive stderr message. | ✅ implemented |
| F9 | Backend caps captured stdout/stderr at 1 MB per stream. | ✅ implemented (but truncation isn't surfaced to the user yet — see risks) |
| F10 | `GET /health` endpoint for uptime/monitoring checks. | ✅ implemented |

## 8. Non-functional requirements

- **Latency:** short snippets should return well within the 5-second run timeout, ideally within a couple of seconds end-to-end including compile.
- **Isolation:** each run gets its own temp directory (`os.MkdirTemp`), removed after the request (`defer os.RemoveAll`). Compilation uses `CGO_ENABLED=0` and `GOTOOLCHAIN=local`.
- **CORS:** backend only allows the origin set in `FRONTEND_URL` (defaults to `http://localhost:3000`), not a wildcard.
- **Availability:** single EC2 instance (`golab-runner`) is acceptable at this stage; no HA requirement yet.
- **Portability:** frontend works against any runner URL via `NEXT_PUBLIC_RUNNER_URL`, so local dev doesn't require the EC2 backend.

## 9. System overview

```
Browser (Next.js, frontend/) → POST /run → Go runner (backend/main.go, EC2: golab-runner)
                                                   │
                                                   ├─ write code → <tmp>/main.go
                                                   ├─ go build -trimpath -o <tmp>/program
                                                   ├─ run program with stdin = req.Input
                                                   └─ return stdout/stderr/exitCode/runtimeMs
```

**Deployment flow (current, manual):**
1. Developer commits and pushes from their laptop (`git add . && git commit && git push`).
2. Developer SSHes into `golab-runner` (`ssh -i golab-key.pem ec2-user@<ip>`).
3. Runs `./deploy.sh` from `~/golab/backend`.
4. If SSH is blocked, it's almost always the EC2 security group needing the current IP added to the inbound SSH rule.

## 10. Success metrics

- % of `/run` requests that return a result (not `connection-error`).
- Median end-to-end time from clicking Run to seeing output.
- Ratio of `exitCode 124` (timeout) responses — a proxy for whether the 5s run timeout is too tight or being hit by legitimate use vs. abuse.
- (Once usage exists) runs per session, as an engagement proxy for tutorial embeds.

## 11. Risks & open questions

- **Resource exhaustion:** the run timeout is wall-clock (5s) but there's no CPU/memory cgroup limit — a tight busy-loop can still consume a full CPU core for 5 seconds per request. Worth adding OS-level resource limits (cgroups, `ulimit`, or running in a container/gVisor sandbox) before wider public exposure.
- **No rate limiting:** `/run` has no per-IP or global rate limit today; a single public URL accepting arbitrary Go code is a plausible abuse target (CPU exhaustion via many concurrent requests).
- **Silent output truncation:** stdout/stderr are capped at 1 MB via `LimitedBuffer`, but the response doesn't currently indicate `truncated: true` — a program that legitimately prints a lot will look like it stopped mid-output with no explanation.
- **Single point of failure:** one EC2 instance, manual `deploy.sh`, no documented rollback if a deploy breaks the runner.
- **Open question:** should snippets be shareable via a persistent URL (adds a storage requirement), or stay ephemeral as today?
- **Open question:** is a 5-second run timeout right for all tutorial use cases, or does it need to be configurable per snippet?

## 12. Future considerations

- Sandbox execution more strictly (container per run, or a gVisor/Firecracker-based sandbox) rather than compiling/running directly on the host.
- Add rate limiting and basic abuse protection on `/run`.
- Surface truncation and timeout distinctly in the UI (they currently both show as generic stderr text).
- Snippet sharing via short links.
- Multi-file support for slightly larger examples.
- Move deployment from manual SSH + `deploy.sh` to CI/CD.
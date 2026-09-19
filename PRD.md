# PRD — GoLab

**Author:** Dhawal Shukla ([GolangForAll](https://golangforall.in))

**Status:** Draft

**Last updated:** September 19, 2026

---

## 1. Summary

GoLab is a lightweight, browser-based playground for writing and running Go code, with full stdin support — programs using `fmt.Scanln`, `bufio.Scanner`, etc. can receive input through a dedicated stdin field.

It's built as a tool for **GolangForAll**, a community for people who learn, build, teach, and contribute with Go (meetups around NCR/Kanpur/Lucknow, a community WhatsApp group, and a blog).

The frontend is a Next.js application with a Monaco editor. The backend is a Go HTTP service that compiles and executes submitted code inside Docker-isolated environments on an AWS EC2 runner.

GoLab uses separate Docker images for compilation and runtime execution. Each submission receives a fresh temporary workspace, while a persistent Go build-cache volume is reused to reduce compilation time for subsequent runs.

---

## 2. Problem statement

Existing Go playgrounds (e.g. the official Go Playground) are solid but generic — not branded for a specific learning audience, and don't always make stdin an obvious, first-class part of the workflow.

GolangForAll wants a playground that:

- Is fast and frictionless — no login, no local setup.
- Makes reading input a visible, explicit step through a dedicated stdin box.
- Gives clear, immediate feedback on compile errors, runtime errors, exit code, and timing.
- Provides a sandboxed execution environment rather than executing arbitrary submitted programs directly on the host.
- Can be embedded into GolangForAll tutorials as an interactive learning tool.

---

## 3. Goals

- Compile and run a Go snippet end-to-end quickly enough to feel responsive for short programs.
- Support stdin-consuming programs via an explicit input field.
- Clearly separate and display stdout, stderr, exit code, and runtime information.
- Track compile time, execution time, and total request processing time.
- Fail predictably and legibly: compile errors, runtime failures, and timeouts should each be distinguishable.
- Execute submitted code inside isolated Docker environments with resource and privilege restrictions.
- Reuse a persistent Go compiler build cache to reduce repeated compilation cost.
- Be embeddable in GolangForAll tutorials as a "try it yourself" widget.

---

## 4. Non-goals (for now)

- Multi-file / multi-package Go projects.
- User accounts, saved snippet history, or authentication.
- Support for languages other than Go.
- Long-running, networked, or background processes.
- Arbitrary external package downloads during compilation.
- Persistent snippet storage or shareable URLs.
- High-availability / multi-region execution infrastructure.
- Horizontal scaling across multiple runner instances.

---

## 5. Target users

- **Primary:** Learners following GolangForAll tutorials, blog posts, and meetup sessions who want to try a snippet immediately, including snippets that read input.

- **Secondary:** Anyone wanting a fast, no-setup Go scratchpad — including people discovering GoLab through the wider GolangForAll community.

---

## 6. User stories

1. As a learner, I open GoLab and immediately see a working, runnable example.

2. As a learner, I edit the code and click Run, and see output within a few seconds for a short program.

3. As a learner whose program reads input, I type it into a dedicated stdin box before running, so the input path is obvious rather than hidden.

4. As a learner, when my program fails to compile, I see the compiler's error output distinctly from normal stdout.

5. As a learner, when my program fails or exits non-zero at runtime, I see the runtime output and exit code clearly.

6. As a learner, if my program runs too long, I get a clear timeout instead of a permanent hang.

7. As a learner, if the runner backend itself is unreachable, I get a distinct "no runner" state rather than a confusing blank output.

8. As a learner, I can see how long compilation and execution took.

---

## 7. Functional requirements

| # | Requirement | Status |
|---|---|---|
| F1 | Editor supports Go syntax highlighting and is pre-populated with a runnable example. | ✅ implemented |
| F2 | A dedicated stdin field lets the user supply input consumed through standard input. | ✅ implemented |
| F3 | Run action POSTs `{ code, input }` to `${NEXT_PUBLIC_RUNNER_URL}/run` and disables itself while in flight. | ✅ implemented |
| F4 | Runner returns stdout, stderr, exit code, runtime, compile time, execution time, and total time. | ✅ implemented |
| F5 | Non-2xx responses and network failures are handled as distinct UI states from program stderr. | ✅ implemented |
| F6 | UI reflects run state: idle / running / ok / error / connection-error. | ✅ implemented |
| F7 | Backend enforces request, code, input, and captured-output size limits. | ✅ implemented |
| F8 | Backend enforces separate compile and execution timeouts. | ✅ implemented |
| F9 | Backend executes submitted code inside Docker-isolated compiler/runtime environments. | ✅ implemented |
| F10 | Compiler and runtime containers run with restricted resources and privileges. | ✅ implemented |
| F11 | Runtime containers have no network access. | ✅ implemented |
| F12 | Runtime execution uses a separate lightweight runtime image that does not contain the Go toolchain. | ✅ implemented |
| F13 | A persistent Docker volume is used for the Go build cache. | ✅ implemented |
| F14 | Temporary submission workspaces are deleted after execution. | ✅ implemented |
| F15 | `GET /health` endpoint is available for uptime/monitoring checks. | ✅ implemented |
| F16 | Production runner is managed as a systemd service on AWS EC2. | ✅ implemented |
| F17 | Production deployment can be updated through the repository and `deploy.sh`. | ✅ implemented |

---

## 8. Non-functional requirements

### Latency

Short snippets should return within a few seconds end-to-end under normal load.

GoLab uses a persistent Go build cache to reduce repeated compilation overhead. Cold compilations can still be significantly slower than warm compilations because the compiler cache has to be populated.

### Isolation

Each request receives a fresh temporary workspace.

Compilation takes place in a dedicated compiler Docker container.

Execution takes place in a separate lightweight runtime Docker container.

The runtime container is configured with:

- No network access.
- 128 MB memory limit.
- 0.5 CPU limit.
- PID limit of 32.
- All Linux capabilities dropped.
- `no-new-privileges`.
- Read-only root filesystem.
- Restricted writable `/tmp`.
- File descriptor and file-size limits.
- Non-root execution.
- 5-second execution timeout.

The compiler container is similarly restricted and uses a separate Go toolchain image.

### Build caching

The compiler uses a persistent Docker volume:

```text
golab-go-cache
````

mounted as the Go build cache.

This allows subsequent compilations to reuse previously built Go packages and significantly reduces warm compilation latency.

### CORS

The backend allows only the frontend origin configured through:

```text
FRONTEND_URL
```

For local development this defaults to:

```text
http://localhost:3000
```

Production uses the GoLab frontend origin.

### Availability

A single AWS EC2 runner is acceptable at this stage.

No high-availability or multi-instance requirement exists yet.

### Portability

The frontend connects to the runner through:

```text
NEXT_PUBLIC_RUNNER_URL
```

This allows local development against a local runner while production can point to the public API endpoint.

---

## 9. System overview

```text
Browser
   │
   │ Next.js frontend
   │
   │ POST /run
   ▼
golab.golangforall.in
   │
   ▼
api.golangforall.in
   │
   ▼
Caddy reverse proxy
   │
   ▼
Go runner service
(AWS EC2)
   │
   │
   ├── Create temporary workspace
   │
   ├── Write submitted code → <tmp>/main.go
   │
   ├── Start compiler Docker container
   │       │
   │       ├── Go 1.26 toolchain
   │       ├── No network
   │       ├── Resource limits
   │       └── Persistent GOCACHE volume
   │
   ├── Produce executable → <tmp>/program
   │
   ├── Start runtime Docker container
   │       │
   │       ├── Lightweight Alpine image
   │       ├── No network
   │       ├── Resource limits
   │       ├── Non-root execution
   │       └── 5-second timeout
   │
   ├── Pass stdin to program
   │
   ├── Capture stdout/stderr
   │
   ├── Record compile/execution/total timings
   │
   └── Delete temporary workspace
```

### Production domains

```text
Frontend:
https://golab.golangforall.in

API:
https://api.golangforall.in
```

### Deployment flow (current, manual)

1. Developer commits and pushes changes from their laptop.

2. Developer SSHes into the AWS EC2 runner.

3. Developer runs `./deploy.sh` from `~/golab/backend`.

4. The deployment script pulls the latest `main` branch.

5. The Go backend binary is rebuilt.

6. The persistent `golab-go-cache` Docker volume is created if it does not already exist.

7. Cache permissions are configured for the sandbox user.

8. The `golab` systemd service is restarted.

9. The service status is verified.

Docker compiler and runtime images are currently built separately on the EC2 instance when required, rather than automatically rebuilt by every execution of `deploy.sh`.

---

## 10. Success metrics

* Percentage of `/run` requests that return a result rather than `connection-error`.
* Median end-to-end time from clicking Run to receiving output.
* Median compile time for warm-cache compilations.
* Median runtime execution time.
* Ratio of `exitCode 124` timeout responses.
* Rate of successful stdin-consuming executions.
* Once usage exists: runs per session, as an engagement proxy for tutorial embeds.
* Once usage exists: error rate by category (compile failure, runtime failure, timeout, runner failure).

---

## 11. Risks & open questions

### Resource exhaustion

Docker resource limits now constrain runtime CPU, memory, process count, file size, and execution time.

However, a public arbitrary-code runner remains an abuse target, particularly if many requests are sent repeatedly.

### No rate limiting

`/run` does not currently have per-IP or global rate limiting.

A public deployment should add basic abuse protection before wider adoption.

### Single runner

The deployment currently uses a single EC2 instance.

A runner outage therefore makes GoLab unavailable.

### Single-job execution

The backend currently uses a global execution lock because the initial deployment targets a small EC2 instance.

This intentionally limits concurrency but also means simultaneous submissions are not independently processed.

### Silent output truncation

Captured stdout/stderr is limited to 1 MB per stream.

The backend currently truncates output beyond the limit without explicitly returning a `truncated` flag.

### Docker overhead

Each request starts isolated Docker environments.

This improves isolation but adds container startup overhead.

Persistent compiler caching reduces compilation time, but does not remove the cost of container startup.

### Deployment rollback

Deployment is currently manual and does not yet provide automated rollback if a new runner version fails.

### Open question

Should snippets become shareable through persistent URLs?

This would require a persistence mechanism and additional product/design decisions.

### Open question

Should the 5-second runtime timeout remain fixed, or should individual tutorial snippets be able to request different limits?

---

## 12. Future considerations

* Add rate limiting and abuse protection to `/run`.
* Surface output truncation explicitly in the UI.
* Improve timing display so compile time and execution time are both visible to users.
* Add shareable snippet URLs.
* Add multi-file Go project support.
* Explore pre-warmed compiler/runtime workers to reduce container startup latency.
* Explore stronger sandboxing such as gVisor or Firecracker if GoLab grows into a larger public execution service.
* Move deployment from manual SSH + `deploy.sh` to CI/CD.
* Add automated health checks and rollback support.
* Add horizontal runner scaling if usage requires it.

---

## 13. Current implementation stack

### Frontend

* Next.js
* React
* TypeScript
* Monaco Editor
* styled-jsx

### Backend

* Go
* `net/http`
* Docker
* AWS EC2
* systemd

### Infrastructure

* Vercel — frontend hosting
* AWS EC2 — code execution runner
* Caddy — reverse proxy and HTTPS
* Docker — compiler/runtime isolation
* GitHub — source repository and deployment source

---

## Part of GolangForAll

GoLab is one of the tools built for [GolangForAll](https://golangforall.in) — meetups, a blog, and docs for the Go community around NCR / Kanpur / Lucknow.

See the [community docs](https://www.golangforall.in/docs/intro) or [contribute](https://www.golangforall.in/docs/contribute) if you want to speak, write, or host a meetup.

---

## License

*Add a license (MIT, Apache-2.0, etc.) — none specified yet.*


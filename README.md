# GoLab

**GoLab** is a browser-based Go playground — write Go code, feed it stdin, and run it instantly, with real compile/runtime feedback.

It's built as a tool for [GolangForAll](https://golangforall.in) — a community for people who learn, build, teach, and contribute with Go — by [Dhawal Shukla](https://github.com/DhawalShankar) ([LinkedIn](https://www.linkedin.com/in/dhawalshukl/)).

> Live at: **https://golab.golangforall.in**

---

## What it does

- A Monaco-powered code editor pre-loaded with a working Go snippet
- A dedicated stdin box so programs that read input (`fmt.Scanln`, `bufio.Scanner`, etc.) behave like a real terminal
- One-click compile & run against a remote Go runner, with a shell-prompt-styled run control
- Output pane showing `stdout` / `stderr` separately, plus exit code and runtime
- Separate compile, execution, and total timing information from the runner
- Clear run states: idle → running → succeeded / failed / runner unreachable
- Sandboxed execution using Docker on the backend

---

## Project structure

```text
golab/

├── backend/
│   ├── go.mod
│   ├── main.go                  # HTTP runner service: /health, /run
│   ├── deploy.sh                # EC2 deployment script
│   └── sandbox/
│       ├── compiler.Dockerfile  # Go compiler image
│       └── runtime.Dockerfile   # Lightweight runtime image
│
└── frontend/
    ├── app/
    │   ├── layout.tsx
    │   ├── page.tsx             # main UI
    │   ├── globals.css
    │   └── favicon.ico
    ├── public/
    ├── .env.local               # NEXT_PUBLIC_RUNNER_URL
    ├── next.config.ts
    ├── package.json
    └── ...
````

---

## Architecture

```text
                    HTTPS
                       │
                       ▼
┌─────────────────────────────────────┐
│            Vercel                   │
│                                     │
│  golab.golangforall.in              │
│  Next.js + React + TypeScript       │
│  Monaco Editor                      │
└──────────────────┬──────────────────┘
                   │
                   │ POST /run
                   ▼
┌─────────────────────────────────────┐
│        api.golangforall.in          │
│                                     │
│              Caddy                  │
│         reverse proxy / TLS         │
└──────────────────┬──────────────────┘
                   │
                   ▼
┌─────────────────────────────────────┐
│          AWS EC2 runner             │
│                                     │
│      Go HTTP service :8080          │
│               │                     │
│       ┌───────┴────────┐            │
│       ▼                ▼            │
│  Compiler Docker   Runtime Docker   │
│      image             image        │
│       │                │            │
│    go build          program        │
│       │                │            │
│       └───────┬────────┘            │
│               ▼                     │
│       sandboxed execution           │
└─────────────────────────────────────┘
```

### Frontend

`frontend/` is a Next.js App Router application using React, TypeScript, and `@monaco-editor/react`.

The frontend:

1. Provides the code editor.
2. Accepts program input through the stdin panel.
3. Sends `{ code, input }` to the remote Go runner.
4. Displays stdout, stderr, exit code, and runtime information.
5. Handles running, success, failure, and connection-error states.

### Backend

`backend/main.go` is a lightweight Go HTTP service exposing:

* `GET /health`
* `POST /run`

For every submission, the runner:

1. Creates a fresh temporary workspace.
2. Writes the submitted source as `main.go`.
3. Compiles the code inside the dedicated compiler Docker image.
4. Uses a persistent Go build cache volume to speed up repeated compilations.
5. Runs the compiled binary inside a separate lightweight runtime Docker image.
6. Passes the submitted stdin to the running program.
7. Captures stdout and stderr.
8. Measures compile, execution, and total timings.
9. Returns the result as JSON.
10. Removes the temporary workspace and runtime container.

The user's submitted code is never executed directly on the EC2 host.

---

## Docker sandbox

GoLab uses two separate Docker images.

### Compiler image

The compiler image is based on:

```text
golang:1.26-alpine
```

It contains the Go toolchain required to compile submissions.

Compiler restrictions include:

* No network access
* 512 MB memory limit
* 1 CPU limit
* PID limit of 128
* All Linux capabilities dropped
* `no-new-privileges`
* Read-only root filesystem
* Writable temporary filesystem
* Non-root execution
* Compilation timeout of 90 seconds

The compiler uses a persistent Docker volume:

```text
golab-go-cache
```

mounted as the Go build cache.

This allows subsequent submissions to reuse previously compiled packages instead of rebuilding everything from scratch.

### Runtime image

The runtime image is intentionally much smaller and is based on:

```text
alpine:3.22
```

It does not contain the Go compiler.

Runtime restrictions include:

* No network access
* 128 MB memory limit
* 0.5 CPU limit
* PID limit of 32
* All Linux capabilities dropped
* `no-new-privileges`
* Read-only root filesystem
* `noexec`, `nosuid`, and `nodev` temporary filesystem
* File descriptor limit
* File size limit
* Core dumps disabled
* Non-root execution
* 5-second execution timeout

The compiled program is mounted into the runtime container as read-only.

---

## Backend API

### `GET /health`

```json
{
  "status": "ok"
}
```

### `POST /run`

Request:

```json
{
  "code": "package main\n\nfunc main() { ... }",
  "input": "Dhawal"
}
```

Example response:

```json
{
  "stdout": "Hello Dhawal\n",
  "stderr": "",
  "exitCode": 0,
  "runtimeMs": 337,
  "compileMs": 624,
  "executionMs": 337,
  "totalMs": 997
}
```

### Response fields

| Field         | Description                                             |
| ------------- | ------------------------------------------------------- |
| `stdout`      | Program standard output                                 |
| `stderr`      | Compiler/runtime error output                           |
| `exitCode`    | Process exit code                                       |
| `runtimeMs`   | Execution timing returned for compatibility with the UI |
| `compileMs`   | Time spent compiling                                    |
| `executionMs` | Time spent starting and running the sandboxed program   |
| `totalMs`     | Total backend processing time                           |

---

## Limits enforced by the backend

| Limit                      | Value      |
| -------------------------- | ---------- |
| Request body size          | 32 KB      |
| Max code size              | 10 KB      |
| Max stdin size             | 10 KB      |
| Max stdout/stderr captured | 1 MB       |
| Compile timeout            | 90 seconds |
| Run timeout                | 5 seconds  |
| Compiler memory            | 512 MB     |
| Runtime memory             | 128 MB     |
| Compiler CPU               | 1 CPU      |
| Runtime CPU                | 0.5 CPU    |
| Compiler PID limit         | 128        |
| Runtime PID limit          | 32         |
| Runtime network access     | Disabled   |

Output beyond the configured capture limit is truncated.

CORS is restricted to the configured frontend origin through the `FRONTEND_URL` environment variable.

---

## Getting started

### Prerequisites

You need:

* Go
* Node.js / npm
* Docker

The backend relies on Docker for sandboxed compilation and execution.

### Backend

```bash
cd backend

go run .
```

or:

```bash
go build -o golab-runner .
./golab-runner
```

Environment variables:

```bash
PORT=8080
FRONTEND_URL=http://localhost:3000
GOLAB_COMPILER_IMAGE=golab-compiler:latest
GOLAB_RUNTIME_IMAGE=golab-runtime:latest
```

### Build the sandbox images

Compiler image:

```bash
docker build \
  -t golab-compiler:latest \
  -f sandbox/compiler.Dockerfile .
```

Runtime image:

```bash
docker build \
  -t golab-runtime:latest \
  -f sandbox/runtime.Dockerfile .
```

Create the persistent Go build cache:

```bash
docker volume create golab-go-cache
```

The backend reuses this volume as the Go build cache.

### Frontend

```bash
cd frontend
npm install
```

`.env.local`:

```bash
NEXT_PUBLIC_RUNNER_URL=http://localhost:8080
```

Then:

```bash
npm run dev
```

Open:

```text
http://localhost:3000
```

---

## Production deployment

GoLab currently uses:

* **Vercel** for the frontend
* **AWS EC2** for the Go runner
* **Caddy** as the reverse proxy
* **systemd** to keep the Go runner running
* **Docker** for sandboxed compilation and execution

Production domains:

```text
Frontend:
https://golab.golangforall.in

API:
https://api.golangforall.in
```

### Deployment flow

```text
Laptop
  │
  │ git push
  ▼
GitHub
  │
  │ git pull
  ▼
AWS EC2
  │
  ├── go build
  ├── Docker build cache volume
  └── systemctl restart golab
```

### Ship code changes from your laptop

```bash
git add .
git commit -m "Update runner"
git push
```

### Apply the update on EC2

```bash
ssh -i "$HOME/.ssh/golab-key.pem" ec2-user@54.79.80.188
```

Then:

```bash
cd ~/golab/backend
./deploy.sh
```

The deployment script:

1. Pulls the latest `main` branch.
2. Builds the Go runner binary.
3. Ensures the `golab-go-cache` Docker volume exists.
4. Ensures the cache is owned by the sandbox user.
5. Restarts the `golab` systemd service.
6. Verifies that the service is running.

### One-time EC2 sandbox setup

Before the first deployment, the EC2 instance needs Docker installed and the sandbox images built:

```bash
docker build \
  -t golab-compiler:latest \
  -f sandbox/compiler.Dockerfile .

docker build \
  -t golab-runtime:latest \
  -f sandbox/runtime.Dockerfile .
```

The cache volume is created automatically by `deploy.sh`.

### Verify the service

```bash
sudo systemctl status golab --no-pager
```

Health check:

```bash
curl http://localhost:8080/health
```

Expected:

```json
{
  "status": "ok"
}
```

---

## Known limitations / things to harden next

* GoLab currently supports a single submitted `main.go` file and does not provide general multi-file/module project support.
* External package downloads are not supported because compilation runs without network access.
* The runner currently serializes jobs with a single execution lock because it is designed for a small EC2 instance.
* There is currently no rate limiting or abuse protection on `/run`.
* Output truncation is enforced, but the frontend does not yet explicitly show when output has been truncated.
* Compile and execution still incur Docker/container startup overhead.
* The compiler cache improves warm compilation significantly, but cold compilation can still be slower.
* The production runner is currently managed through manual Git pull + `deploy.sh` deployment rather than CI/CD.

---

## Roadmap

* [ ] Rate limiting / abuse protection on `/run`
* [ ] Surface output-truncation in the UI
* [ ] Shareable snippet links
* [ ] Better compiler/runtime timing display
* [ ] Multi-file Go project support
* [ ] CI-based deployment instead of manual SSH + `deploy.sh`
* [ ] Pre-warmed compiler/runtime workers for lower latency
* [ ] More advanced execution monitoring and isolation

---

## Part of GolangForAll

GoLab is one of the tools built for [GolangForAll](https://golangforall.in) — meetups, a blog, and docs for the Go community around NCR / Kanpur / Lucknow.

See the [community docs](https://www.golangforall.in/docs/intro) or [contribute](https://www.golangforall.in/docs/contribute) if you want to speak, write, or host a meetup.

---

## License

*Add a license (MIT, Apache-2.0, etc.) — none specified yet.*


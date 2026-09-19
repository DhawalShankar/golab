# GoLab

**GoLab** is a browser-based Go playground — write Go code, feed it stdin, and run it instantly, with real compile/runtime feedback.

It's built as a tool for [GolangForAll](https://golangforall.in) — a community for people who learn, build, teach, and contribute with Go — by [Dhawal Shukla](https://github.com/DhawalShankar) ([LinkedIn](https://www.linkedin.com/in/dhawalshukl/)).

> Live at: _add your production URL here_

---

## What it does

- A Monaco-powered code editor pre-loaded with a working Go snippet
- A dedicated stdin box so programs that read input (`fmt.Scanln`, `bufio.Scanner`, etc.) behave like a real terminal
- One-click compile & run against a remote Go runner, with a shell-prompt-styled run control
- Output pane showing `stdout` / `stderr` separately, plus exit code and runtime in milliseconds
- Clear run states: idle → running → succeeded / failed / runner unreachable

## Project structure

```
golab/
├── backend/
│   ├── go.mod
│   └── main.go          # HTTP runner service: /health, /run
└── frontend/
    ├── app/
    │   ├── layout.tsx
    │   ├── page.tsx      # main UI
    │   ├── globals.css
    │   └── favicon.ico
    ├── public/
    ├── .env.local         # NEXT_PUBLIC_RUNNER_URL
    ├── next.config.ts
    ├── package.json
    └── ...
```

## Architecture

```
┌─────────────────────┐        POST /run (JSON)        ┌───────────────────────────┐
│   frontend/ (Next.js) │  ─────────────────────────▶  │   backend/ (Go runner)      │
│   app/page.tsx         │                               │   EC2 instance: golab-runner │
│                        │  ◀─────────────────────────  │                              │
└─────────────────────┘   { stdout, stderr,             │  writes main.go → go build   │
                            exitCode, runtimeMs }        │  → runs binary with stdin    │
                                                          └───────────────────────────┘
```

- **Frontend** (`frontend/`) — Next.js App Router, React, TypeScript, `@monaco-editor/react` for the editor, styled-jsx for the terminal-window UI.
- **Backend** (`backend/`) — a single-file Go HTTP service (`main.go`) that, per request:
  1. Writes the submitted code to a fresh temp directory as `main.go`.
  2. Compiles it with `go build -trimpath` (`GOTOOLCHAIN=local`, `CGO_ENABLED=0`).
  3. Runs the resulting binary with the submitted stdin.
  4. Returns `stdout`, `stderr`, `exitCode`, and `runtimeMs` as JSON.
  5. Deletes the temp directory afterwards.

## Backend API

### `GET /health`
```json
{ "status": "ok" }
```

### `POST /run`

Request:
```json
{ "code": "package main\n\nfunc main() { ... }", "input": "Dhawal" }
```

Response:
```json
{ "stdout": "Hello Dhawal\n", "stderr": "", "exitCode": 0, "runtimeMs": 42 }
```

**Limits enforced by the backend:**

| Limit | Value |
|---|---|
| Request body size | 32 KB |
| Max code size | 10 KB |
| Max stdin size | 10 KB |
| Max stdout/stderr captured | 1 MB (silently truncated beyond that) |
| Compile timeout | 90 s → `exitCode 124`, "Compilation timed out." |
| Run timeout | 5 s → `exitCode 124`, "Execution timed out after 5 seconds." |

CORS is restricted to a single allowed origin via the `FRONTEND_URL` env var (defaults to `http://localhost:3000`).

## Getting started

### Backend

```bash
cd backend
go run main.go
# or: go build -o golab-runner && ./golab-runner
```

Env vars:
```bash
PORT=8080                        # default 8080
FRONTEND_URL=http://localhost:3000   # allowed CORS origin
```

### Frontend

```bash
cd frontend
npm install
```

`.env.local`:
```bash
NEXT_PUBLIC_RUNNER_URL=http://localhost:8080
```

```bash
npm run dev
```

Open [http://localhost:3000](http://localhost:3000).

## Deployment

### 1. Ship code changes — from your laptop

```bash
git add .
git commit -m "Update runner"
git push
```

### 2. Apply the update — on the EC2 instance (`golab-runner`)

```bash
ssh -i "$HOME/.ssh/golab-key.pem" ec2-user@54.79.80.188

cd ~/golab/backend
./deploy.sh
```

> On plain Windows PowerShell/cmd (not Git Bash/WSL), use `$env:USERPROFILE\.ssh\golab-key.pem` instead of `$HOME/.ssh/golab-key.pem`.

### If SSH is refused / times out

Your IP probably changed and the security group is blocking it:

1. **EC2 console → Instances → `golab-runner`**.
2. **Security** tab → click the attached security group.
3. **Edit inbound rules** → SSH (port 22) rule → set source to **My IP**.
4. Save and retry.

## Known limitations / things to harden next

- Each run compiles + executes on the same host with no per-process CPU/memory cap beyond the 5-second wall-clock timeout — a tight loop can still burn CPU for the full 5 seconds.
- No rate limiting on `/run` — a public deployment should add one before wider sharing.
- Output truncation past 1 MB happens silently; the frontend doesn't currently indicate "output was truncated."
- No package/module support beyond the standard library (single `main.go` file, no `go.mod` for user code).

## Roadmap

- [ ] Rate limiting / abuse protection on `/run`
- [ ] Surface output-truncation in the UI
- [ ] Shareable snippet links
- [ ] CI-based deploy instead of manual SSH + `deploy.sh`

## Part of GolangForAll

GoLab is one of the tools built for [GolangForAll](https://golangforall.in) — meetups, a blog, and docs for the Go community around NCR / Kanpur / Lucknow. See the [community docs](https://www.golangforall.in/docs/intro) or [contribute](https://www.golangforall.in/docs/contribute) if you want to speak, write, or host a meetup.

## License

_Add a license (MIT, Apache-2.0, etc.) — none specified yet._
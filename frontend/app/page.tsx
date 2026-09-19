"use client";

import { useState } from "react";
import Editor from "@monaco-editor/react";
import Image from "next/image";

const DEFAULT_CODE = `package main

import "fmt"

func main() {
\tvar name string
\tfmt.Scanln(&name)

\tfmt.Println("Hello", name)
}
`;

type RunResponse = {
  stdout: string;
  stderr: string;
  exitCode: number;
  runtimeMs: number;
};

type RunState = "idle" | "running" | "ok" | "error" | "connection-error";

export default function Home() {
  const [code, setCode] = useState(DEFAULT_CODE);
  const [input, setInput] = useState("Dhawal");
  const [stdout, setStdout] = useState("");
  const [stderr, setStderr] = useState("");
  const [runtimeMs, setRuntimeMs] = useState<number | null>(null);
  const [exitCode, setExitCode] = useState<number | null>(null);
  const [state, setState] = useState<RunState>("idle");

  async function runCode() {
    setState("running");
    setStdout("");
    setStderr("");
    setRuntimeMs(null);
    setExitCode(null);

    try {
      const runnerURL =
        process.env.NEXT_PUBLIC_RUNNER_URL || "http://localhost:8080";

      const response = await fetch(`${runnerURL}/run`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ code, input }),
      });

      const raw = await response.text();

      if (!response.ok) {
        setState("error");
        setStderr(`runner error (${response.status})\n\n${raw}`);
        return;
      }

      let data: RunResponse;
      try {
        data = JSON.parse(raw);
      } catch {
        setState("error");
        setStderr(`runner returned invalid JSON\n\n${raw}`);
        return;
      }

      setStdout(data.stdout ?? "");
      setStderr(data.stderr ?? "");
      setRuntimeMs(data.runtimeMs);
      setExitCode(data.exitCode);
      setState(data.exitCode === 0 ? "ok" : "error");
    } catch (err) {
      console.error(err);
      setState("connection-error");
      setStderr("could not reach the GoLab runner. is it running?");
    } finally {
      // no-op: state already resolved above
    }
  }

  const isRunning = state === "running";
  const hasResult = stdout || stderr;

  return (
    <main className="golab-root">
      <link
        rel="stylesheet"
        href="https://fonts.googleapis.com/css2?family=Space+Grotesk:wght@500;600;700&family=JetBrains+Mono:wght@400;500;600&display=swap"
      />

      {/* Header */}
      <header className="golab-header">
        <div className="golab-brand">
          <Image src="/logo.svg" alt="" width={60} height={60} priority />
          <div className="golab-brand-text">
            <span className="golab-name">GoLab</span>
            <span className="golab-by">by GolangForAll</span>
          </div>
        </div>
        <a
          className="golab-link"
          href="https://go.dev"
          target="_blank"
          rel="noreferrer"
        >
          go.dev/doc
        </a>
      </header>

      <div className="golab-grid">
        {/* Left column */}
        <section className="golab-col">
          <div className="term-window">
            <div className="term-bar">
              <div className="term-dots">
                <span className="dot dot-red" />
                <span className="dot dot-yellow" />
                <span className="dot dot-green" />
              </div>
              <span className="term-path">main.go</span>
              <span className="term-lang">go</span>
            </div>
            <Editor
              height="52vh"
              language="go"
              theme="golab-dark"
              value={code}
              onChange={(value) => setCode(value ?? "")}
              beforeMount={(monaco) => {
                monaco.editor.defineTheme("golab-dark", {
                  base: "vs-dark",
                  inherit: true,
                  rules: [],
                  colors: {
                    "editor.background": "#0d1117",
                  },
                });
              }}
              options={{
                minimap: { enabled: false },
                fontSize: 14.5,
                fontFamily:
                  "'JetBrains Mono', ui-monospace, SFMono-Regular, monospace",
                lineNumbers: "on",
                wordWrap: "on",
                automaticLayout: true,
                tabSize: 4,
                scrollBeyondLastLine: false,
                padding: { top: 14, bottom: 14 },
              }}
            />
          </div>

          <div className="term-window term-window-input">
            <div className="term-bar">
              <span className="term-path">stdin</span>
              <span className="term-hint">fed to fmt.Scan / os.Stdin</span>
            </div>
            <textarea
              value={input}
              onChange={(e) => setInput(e.target.value)}
              placeholder="type program input here..."
              className="stdin-area"
              spellCheck={false}
            />
          </div>

          <button
            onClick={runCode}
            disabled={isRunning}
            className="run-prompt"
            aria-label="Run Go program"
          >
            <span className="run-caret">{isRunning ? "…" : "$"}</span>
            <span className="run-cmd">
              {isRunning ? "compiling and running" : "go run main.go"}
            </span>
            {!isRunning && <span className="run-blink" aria-hidden />}
          </button>
        </section>

        {/* Right column */}
        <section className="term-window golab-output">
          <div className="term-bar">
            <div className="term-dots">
              <span className="dot dot-red" />
              <span className="dot dot-yellow" />
              <span className="dot dot-green" />
            </div>
            <span className="term-path">stdout</span>
            <StatusPill state={state} />
          </div>

          <div className="output-body">
            {!hasResult && state === "idle" && (
              <p className="output-empty">
                run your program to see what it prints. nothing here yet.
              </p>
            )}
            {state === "running" && (
              <p className="output-empty output-empty-running">
                building binary, streaming output…
              </p>
            )}
            {stdout && <pre className="output-stdout">{stdout}</pre>}
            {stderr && <pre className="output-stderr">{stderr}</pre>}
          </div>

          <div className="output-status">
            <span>
              exit code:{" "}
              <b className={exitCode === 0 ? "ok-text" : exitCode ? "err-text" : ""}>
                {exitCode ?? "—"}
              </b>
            </span>
            <span>
              runtime: <b>{runtimeMs != null ? `${runtimeMs} ms` : "—"}</b>
            </span>
          </div>
        </section>
      </div>

      <style jsx global>{`
        :root {
          --bg: #0a0e14;
          --panel: #0d1117;
          --panel-border: #21262d;
          --text: #e6edf3;
          --muted: #6e7681;
          --cyan: #00add8;
          --green: #3fb950;
          --red: #f85149;
          --amber: #d29922;
        }

        html,
        body {
          background: var(--bg);
        }

        .golab-root {
          min-height: 100vh;
          background: var(--bg);
          color: var(--text);
          padding: 28px 32px 40px;
          font-family: "JetBrains Mono", ui-monospace, SFMono-Regular,
            monospace;
        }

        .golab-header {
          display: flex;
          align-items: center;
          justify-content: space-between;
          margin-bottom: 22px;
          padding-bottom: 18px;
          border-bottom: 1px solid var(--panel-border);
        }

        .golab-brand {
          display: flex;
          align-items: center;
          gap: 12px;
        }

        .golab-brand-text {
          display: flex;
          flex-direction: column;
          line-height: 1.15;
        }

        .golab-name {
          font-family: "Space Grotesk", ui-sans-serif, system-ui, sans-serif;
          font-size: 20px;
          font-weight: 700;
          letter-spacing: -0.01em;
        }

        .golab-by {
          font-size: 11.5px;
          color: var(--muted);
          margin-top: 2px;
        }

        .golab-link {
          font-size: 13px;
          color: var(--muted);
          text-decoration: none;
          border: 1px solid var(--panel-border);
          padding: 6px 12px;
          border-radius: 6px;
          transition: color 0.15s ease, border-color 0.15s ease;
        }

        .golab-link:hover {
          color: var(--cyan);
          border-color: var(--cyan);
        }

        .golab-grid {
          display: grid;
          grid-template-columns: 1fr;
          gap: 18px;
          max-width: 1360px;
          margin: 0 auto;
        }

        @media (min-width: 1024px) {
          .golab-grid {
            grid-template-columns: 1fr 420px;
            align-items: start;
          }
        }

        .golab-col {
          display: flex;
          flex-direction: column;
          gap: 16px;
        }

        .term-window {
          border: 1px solid var(--panel-border);
          border-radius: 8px;
          overflow: hidden;
          background: var(--panel);
        }

        .term-bar {
          display: flex;
          align-items: center;
          gap: 10px;
          padding: 9px 14px;
          border-bottom: 1px solid var(--panel-border);
          background: #11161d;
        }

        .term-dots {
          display: flex;
          gap: 6px;
          margin-right: 4px;
        }

        .dot {
          width: 9px;
          height: 9px;
          border-radius: 50%;
          display: inline-block;
        }

        .dot-red {
          background: #ff5f56;
        }
        .dot-yellow {
          background: #ffbd2e;
        }
        .dot-green {
          background: #27c93f;
        }

        .term-path {
          font-size: 12.5px;
          color: var(--text);
          opacity: 0.85;
        }

        .term-lang,
        .term-hint {
          margin-left: auto;
          font-size: 11px;
          color: var(--muted);
        }

        .term-window-input .stdin-area {
          width: 100%;
          height: 96px;
          resize: vertical;
          border: none;
          background: var(--panel);
          color: var(--text);
          font-family: inherit;
          font-size: 13.5px;
          padding: 14px;
          outline: none;
        }

        .stdin-area::placeholder {
          color: #3d444d;
        }

        .run-prompt {
          display: flex;
          align-items: center;
          gap: 10px;
          width: 100%;
          text-align: left;
          background: var(--panel);
          border: 1px solid var(--panel-border);
          border-radius: 8px;
          padding: 13px 16px;
          font-family: inherit;
          font-size: 14px;
          color: var(--text);
          cursor: pointer;
          transition: border-color 0.15s ease, background 0.15s ease;
        }

        .run-prompt:hover:not(:disabled) {
          border-color: var(--cyan);
          background: #0f1720;
        }

        .run-prompt:disabled {
          cursor: not-allowed;
          opacity: 0.7;
        }

        .run-caret {
          color: var(--green);
          font-weight: 600;
        }

        .run-cmd {
          color: var(--cyan);
        }

        .run-blink {
          width: 8px;
          height: 16px;
          background: var(--text);
          margin-left: 2px;
          animation: blink 1.1s steps(1) infinite;
        }

        @keyframes blink {
          0%,
          49% {
            opacity: 1;
          }
          50%,
          100% {
            opacity: 0;
          }
        }

        .golab-output {
          display: flex;
          flex-direction: column;
          min-height: 100%;
        }

        .output-body {
          flex: 1;
          min-height: 44vh;
          padding: 16px;
          overflow: auto;
        }

        .output-empty {
          font-size: 13px;
          color: var(--muted);
        }

        .output-empty-running {
          color: var(--amber);
        }

        .output-stdout {
          white-space: pre-wrap;
          word-break: break-word;
          font-size: 13.5px;
          line-height: 1.6;
          color: var(--text);
          margin: 0 0 12px;
        }

        .output-stderr {
          white-space: pre-wrap;
          word-break: break-word;
          font-size: 13.5px;
          line-height: 1.6;
          color: var(--red);
          margin: 0;
        }

        .output-status {
          display: flex;
          gap: 20px;
          padding: 10px 16px;
          border-top: 1px solid var(--panel-border);
          background: #11161d;
          font-size: 12px;
          color: var(--muted);
        }

        .ok-text {
          color: var(--green);
        }

        .err-text {
          color: var(--red);
        }

        .status-pill {
          margin-left: auto;
          font-size: 11px;
          padding: 3px 9px;
          border-radius: 100px;
          border: 1px solid var(--panel-border);
        }

        .status-pill.idle {
          color: var(--muted);
        }
        .status-pill.running {
          color: var(--amber);
          border-color: var(--amber);
        }
        .status-pill.ok {
          color: var(--green);
          border-color: var(--green);
        }
        .status-pill.error,
        .status-pill.connection-error {
          color: var(--red);
          border-color: var(--red);
        }

        @media (prefers-reduced-motion: reduce) {
          .run-blink {
            animation: none;
            opacity: 1;
          }
        }

        a:focus-visible,
        button:focus-visible,
        textarea:focus-visible {
          outline: 2px solid var(--cyan);
          outline-offset: 2px;
        }
      `}</style>
    </main>
  );
}

function StatusPill({ state }: { state: RunState }) {
  const labels: Record<RunState, string> = {
    idle: "idle",
    running: "running",
    ok: "exit 0",
    error: "failed",
    "connection-error": "no runner",
  };
  return <span className={`status-pill ${state}`}>{labels[state]}</span>;
}
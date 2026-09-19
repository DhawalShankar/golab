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
  compileMs?: number;
  executionMs?: number;
  totalMs?: number;
};

type RunState = "idle" | "running" | "ok" | "error" | "connection-error";

export default function Home() {
  const [code, setCode] = useState(DEFAULT_CODE);
  const [input, setInput] = useState("Dhawal");
  const [stdout, setStdout] = useState("");
  const [stderr, setStderr] = useState("");
  const [runtimeMs, setRuntimeMs] = useState<number | null>(null);
  const [compileMs, setCompileMs] = useState<number | null>(null);
  const [executionMs, setExecutionMs] = useState<number | null>(null);
  const [totalMs, setTotalMs] = useState<number | null>(null);
  const [exitCode, setExitCode] = useState<number | null>(null);
  const [state, setState] = useState<RunState>("idle");

  async function runCode() {
    setState("running");
    setStdout("");
    setStderr("");
    setRuntimeMs(null);
    setCompileMs(null);
    setExecutionMs(null);
    setTotalMs(null);
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
      setCompileMs(data.compileMs ?? null);
      setExecutionMs(data.executionMs ?? null);
      setTotalMs(data.totalMs ?? null);
      setExitCode(data.exitCode);
      setState(data.exitCode === 0 ? "ok" : "error");
    } catch (err) {
      console.error(err);
      setState("connection-error");
      setStderr("could not reach the GoLab runner. is it running?");
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
          <Image src="/logo.svg" alt="" width={30} height={30} priority />
          <div className="golab-brand-text">
            <span className="golab-name">GoLab</span>
            <a
              className="golab-by"
              href="https://golangforall.in"
              target="_blank"
              rel="noreferrer"
            >
              by GolangForAll
            </a>
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
          <div className="term-window editor-window">
            <div className="term-bar">
              <div className="term-dots">
                <span className="dot dot-red" />
                <span className="dot dot-yellow" />
                <span className="dot dot-green" />
              </div>
              <span className="term-path">main.go</span>

              <button
                onClick={runCode}
                disabled={isRunning}
                className="run-btn"
                aria-label="Run Go program"
              >
                <span className="run-caret">{isRunning ? "…" : "$"}</span>
                <span className="run-cmd">
                  {isRunning ? "running" : "go run"}
                </span>
              </button>
            </div>

            <div className="editor-wrap">
              <Editor
                height="100%"
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
                  fontSize: 14,
                  fontFamily:
                    "'JetBrains Mono', ui-monospace, SFMono-Regular, monospace",
                  lineNumbers: "on",
                  wordWrap: "on",
                  automaticLayout: true,
                  tabSize: 4,
                  scrollBeyondLastLine: false,
                  padding: { top: 12, bottom: 12 },
                }}
              />
            </div>
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
              compile: <b>{compileMs != null ? `${compileMs} ms` : "—"}</b>
            </span>
            <span>
              exec: <b>{executionMs != null ? `${executionMs} ms` : "—"}</b>
            </span>
            <span>
              total: <b>{totalMs != null ? `${totalMs} ms` : "—"}</b>
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
          --header-h: 52px;
          --stdin-h: 96px;
          --gap: 12px;
        }

        html,
        body {
          background: var(--bg);
          height: 100%;
          overflow: hidden;
        }

        .golab-root {
          height: 100vh;
          display: flex;
          flex-direction: column;
          background: var(--bg);
          color: var(--text);
          padding: 14px 20px;
          font-family: "JetBrains Mono", ui-monospace, SFMono-Regular,
            monospace;
          box-sizing: border-box;
        }

        .golab-header {
          display: flex;
          align-items: center;
          justify-content: space-between;
          height: var(--header-h);
          flex: 0 0 var(--header-h);
          padding-bottom: 10px;
          border-bottom: 1px solid var(--panel-border);
        }

        .golab-brand {
          display: flex;
          align-items: center;
          gap: 10px;
        }

        .golab-brand-text {
          display: flex;
          flex-direction: column;
          line-height: 1.1;
        }

        .golab-name {
          font-family: "Space Grotesk", ui-sans-serif, system-ui, sans-serif;
          font-size: 17px;
          font-weight: 700;
          letter-spacing: -0.01em;
        }

        .golab-by {
          font-size: 10.5px;
          color: var(--muted);
          margin-top: 1px;
          text-decoration: none;
          transition: color 0.15s ease;
        }

        .golab-by:hover {
          color: var(--cyan);
        }

        .golab-link {
          font-size: 12px;
          color: var(--muted);
          text-decoration: none;
          border: 1px solid var(--panel-border);
          padding: 5px 10px;
          border-radius: 6px;
          transition: color 0.15s ease, border-color 0.15s ease;
        }

        .golab-link:hover {
          color: var(--cyan);
          border-color: var(--cyan);
        }

        .golab-grid {
          flex: 1;
          min-height: 0;
          display: grid;
          grid-template-columns: 1fr;
          gap: var(--gap);
          max-width: 1440px;
          width: 100%;
          margin: 12px auto 0;
        }

        @media (min-width: 1024px) {
          .golab-grid {
            grid-template-columns: 1fr 420px;
          }
        }

        .golab-col {
          min-height: 0;
          display: flex;
          flex-direction: column;
          gap: var(--gap);
        }

        .term-window {
          border: 1px solid var(--panel-border);
          border-radius: 8px;
          overflow: hidden;
          background: var(--panel);
          display: flex;
          flex-direction: column;
        }

        .editor-window {
          flex: 1;
          min-height: 0;
        }

        .editor-wrap {
          flex: 1;
          min-height: 0;
        }

        .term-bar {
          display: flex;
          align-items: center;
          gap: 10px;
          padding: 8px 12px;
          border-bottom: 1px solid var(--panel-border);
          background: #11161d;
          flex: 0 0 auto;
        }

        .term-dots {
          display: flex;
          gap: 6px;
          margin-right: 2px;
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
          font-size: 12px;
          color: var(--text);
          opacity: 0.85;
        }

        .term-hint {
          margin-left: auto;
          font-size: 10.5px;
          color: var(--muted);
        }

        .run-btn {
          margin-left: auto;
          display: flex;
          align-items: center;
          gap: 6px;
          background: transparent;
          border: 1px solid var(--panel-border);
          border-radius: 6px;
          padding: 4px 10px;
          font-family: inherit;
          font-size: 12px;
          color: var(--text);
          cursor: pointer;
          transition: border-color 0.15s ease, background 0.15s ease;
        }

        .run-btn:hover:not(:disabled) {
          border-color: var(--cyan);
          background: #0f1720;
        }

        .run-btn:disabled {
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

        .term-window-input {
          flex: 0 0 var(--stdin-h);
        }

        .term-window-input .stdin-area {
          flex: 1;
          width: 100%;
          resize: none;
          border: none;
          background: var(--panel);
          color: var(--text);
          font-family: inherit;
          font-size: 13px;
          padding: 10px 12px;
          outline: none;
        }

        .stdin-area::placeholder {
          color: #3d444d;
        }

        .golab-output {
          min-height: 0;
        }

        .output-body {
          flex: 1;
          min-height: 0;
          padding: 14px;
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
          font-size: 13px;
          line-height: 1.55;
          color: var(--text);
          margin: 0 0 12px;
        }

        .output-stderr {
          white-space: pre-wrap;
          word-break: break-word;
          font-size: 13px;
          line-height: 1.55;
          color: var(--red);
          margin: 0;
        }

        .output-status {
          flex: 0 0 auto;
          display: flex;
          flex-wrap: wrap;
          gap: 14px 20px;
          padding: 8px 14px;
          border-top: 1px solid var(--panel-border);
          background: #11161d;
          font-size: 11.5px;
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
          font-size: 10.5px;
          padding: 2px 8px;
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

        @media (max-width: 1023px) {
          html,
          body {
            overflow: auto;
          }
          .golab-root {
            height: auto;
            overflow: visible;
          }
          .editor-window .editor-wrap {
            min-height: 320px;
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
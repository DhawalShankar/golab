"use client";

import { useState } from "react";
import Editor from "@monaco-editor/react";

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

export default function Home() {
  const [code, setCode] = useState(DEFAULT_CODE);
  const [input, setInput] = useState("Dhawal");
  const [output, setOutput] = useState("");
  const [running, setRunning] = useState(false);

  async function runCode() {
    setRunning(true);
    setOutput("Running...");

    try {
      const response = await fetch("http://localhost:8080/run", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          code,
          input,
        }),
      });

      const data: RunResponse = await response.json();

      if (data.exitCode === 0) {
        setOutput(
          `${data.stdout}\n✓ Finished in ${data.runtimeMs} ms`
        );
      } else {
        setOutput(
          `${data.stderr}\n✗ Failed`
        );
      }
    } catch (error) {
      console.error(error);
      setOutput("Could not connect to GoLab runner.");
    } finally {
      setRunning(false);
    }
  }

  return (
    <main className="min-h-screen bg-[#0d1117] text-white p-6">
      {/* Header */}
      <div className="mb-5">
        <h1 className="text-2xl font-semibold">GoLab</h1>
        <p className="text-sm text-gray-500 mt-1">
          Write and run Go code instantly.
        </p>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-[1fr_380px] gap-4">
        {/* Left */}
        <section className="flex flex-col gap-4">
          {/* Editor */}
          <div className="overflow-hidden rounded-lg border border-[#30363d]">
            <Editor
              height="55vh"
              language="go"
              theme="vs-dark"
              value={code}
              onChange={(value) => setCode(value ?? "")}
              options={{
                minimap: {
                  enabled: false,
                },
                fontSize: 15,
                lineNumbers: "on",
                wordWrap: "on",
                automaticLayout: true,
                tabSize: 4,
                scrollBeyondLastLine: false,
                padding: {
                  top: 12,
                  bottom: 12,
                },
              }}
            />
          </div>

          {/* Input */}
          <div className="rounded-lg border border-[#30363d] bg-[#161b22] p-4">
            <div className="mb-2 text-sm font-medium text-gray-300">
              Standard Input
            </div>

            <textarea
              value={input}
              onChange={(e) => setInput(e.target.value)}
              placeholder="Enter program input..."
              className="w-full h-24 resize-y rounded-md border border-[#30363d] bg-[#0d1117] p-3 font-mono text-sm text-white outline-none focus:border-gray-500"
            />
          </div>

          {/* Run button */}
          <div>
            <button
              onClick={runCode}
              disabled={running}
              className="rounded-md bg-white px-5 py-2.5 text-sm font-medium text-black transition hover:bg-gray-200 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {running ? "Running..." : "▶ Run"}
            </button>
          </div>
        </section>

        {/* Right */}
        <section className="rounded-lg border border-[#30363d] bg-[#161b22] overflow-hidden">
          <div className="border-b border-[#30363d] px-4 py-3">
            <span className="text-sm font-medium text-gray-300">
              Output
            </span>
          </div>

          <div className="min-h-[55vh] p-4">
            {output ? (
              <pre className="whitespace-pre-wrap break-words font-mono text-sm leading-6 text-gray-200">
                {output}
              </pre>
            ) : (
              <p className="font-mono text-sm text-gray-600">
                Run your Go code to see the output.
              </p>
            )}
          </div>
        </section>
      </div>
    </main>
  );
}
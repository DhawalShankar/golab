package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	maxCodeSize   = 10 * 1024       // 10 KB
	maxInputSize  = 10 * 1024       // 10 KB
	maxOutputSize = 1 * 1024 * 1024 // 1 MB
	runTimeout    = 5 * time.Second
)

type RunRequest struct {
	Code  string `json:"code"`
	Input string `json:"input"`
}

type RunResponse struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exitCode"`
	Runtime  int64  `json:"runtimeMs"`
}

type LimitWriter struct {
	Buffer bytes.Buffer
	Limit  int
}

func (w *LimitWriter) Write(p []byte) (int, error) {
	remaining := w.Limit - w.Buffer.Len()

	if remaining <= 0 {
		return len(p), nil
	}

	if len(p) > remaining {
		p = p[:remaining]
	}

	_, _ = w.Buffer.Write(p)

	// Tell the process that it can continue.
	// We intentionally truncate output instead of failing.
	return len(p), nil
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	_ = json.NewEncoder(w).Encode(data)
}

func enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(
			"Access-Control-Allow-Origin",
			"http://localhost:3000",
		)

		w.Header().Set(
			"Access-Control-Allow-Methods",
			"GET, POST, OPTIONS",
		)

		w.Header().Set(
			"Access-Control-Allow-Headers",
			"Content-Type",
		)

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
			"error": "Method not allowed",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

func runCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
			"error": "Method not allowed",
		})
		return
	}

	var req RunRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Invalid JSON",
		})
		return
	}

	req.Code = strings.TrimSpace(req.Code)

	if req.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Code cannot be empty",
		})
		return
	}

	if len(req.Code) > maxCodeSize {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Code too large. Maximum size is 10 KB.",
		})
		return
	}

	if len(req.Input) > maxInputSize {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Input too large. Maximum size is 10 KB.",
		})
		return
	}

	// Create a temporary workspace.
	dir, err := os.MkdirTemp("", "golab-*")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Could not create workspace",
		})
		return
	}

	defer os.RemoveAll(dir)

	// Write submitted source code.
	source := filepath.Join(dir, "main.go")

	if err := os.WriteFile(source, []byte(req.Code), 0600); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Could not write source file",
		})
		return
	}

	// Windows requires .exe; Linux does not.
	binaryName := "program"

	if runtime.GOOS == "windows" {
		binaryName = "program.exe"
	}

	binary := filepath.Join(dir, binaryName)

	// ---------------------------------------------------------
	// Compile
	// ---------------------------------------------------------

	compileCtx, compileCancel := context.WithTimeout(
		context.Background(),
		runTimeout,
	)
	defer compileCancel()

	compileCmd := exec.CommandContext(
		compileCtx,
		"go",
		"build",
		"-o",
		binary,
		source,
	)

	compileCmd.Dir = dir

	var compileOutput LimitWriter
	compileOutput.Limit = maxOutputSize

	compileCmd.Stdout = &compileOutput.Buffer
	compileCmd.Stderr = &compileOutput.Buffer

	err = compileCmd.Run()

	if compileCtx.Err() == context.DeadlineExceeded {
		writeJSON(w, http.StatusOK, RunResponse{
			Stdout:   "",
			Stderr:   "Compilation timed out.",
			ExitCode: 124,
			Runtime:  0,
		})
		return
	}

	if err != nil {
		writeJSON(w, http.StatusOK, RunResponse{
			Stdout:   "",
			Stderr:   compileOutput.Buffer.String(),
			ExitCode: 1,
			Runtime:  0,
		})
		return
	}

	// ---------------------------------------------------------
	// Execute
	// ---------------------------------------------------------

	ctx, cancel := context.WithTimeout(
		context.Background(),
		runTimeout,
	)
	defer cancel()

	runCmd := exec.CommandContext(ctx, binary)
	runCmd.Dir = dir

	// Send user input to stdin.
	runCmd.Stdin = strings.NewReader(req.Input)

	var output LimitWriter
	output.Limit = maxOutputSize

	runCmd.Stdout = &output.Buffer
	runCmd.Stderr = &output.Buffer

	start := time.Now()

	err = runCmd.Run()

	runtimeMs := time.Since(start).Milliseconds()

	// Timeout.
	if ctx.Err() == context.DeadlineExceeded {
		writeJSON(w, http.StatusOK, RunResponse{
			Stdout:   "",
			Stderr:   "Execution timed out after 5 seconds.",
			ExitCode: 124,
			Runtime:  runtimeMs,
		})
		return
	}

	// Runtime error / panic.
	if err != nil {
		writeJSON(w, http.StatusOK, RunResponse{
			Stdout:   "",
			Stderr:   output.Buffer.String(),
			ExitCode: 1,
			Runtime:  runtimeMs,
		})
		return
	}

	// Successful execution.
	writeJSON(w, http.StatusOK, RunResponse{
		Stdout:   output.Buffer.String(),
		Stderr:   "",
		ExitCode: 0,
		Runtime:  runtimeMs,
	})
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", health)
	mux.HandleFunc("/run", runCode)

	handler := enableCORS(mux)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	println("GoLab runner running on port " + port)

	if err := http.ListenAndServe("0.0.0.0:"+port, handler); err != nil {
		panic(err)
	}
}
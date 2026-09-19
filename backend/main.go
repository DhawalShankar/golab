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
	maxCodeSize     = 10 * 1024        // 10 KB
	maxInputSize    = 10 * 1024        // 10 KB
	maxOutputSize   = 1 * 1024 * 1024  // 1 MB
	compileTimeout  = 30 * time.Second // Compilation
	runTimeout      = 5 * time.Second  // User program
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

type LimitBuffer struct {
	Buffer bytes.Buffer
	Limit  int
}

func (b *LimitBuffer) Write(p []byte) (int, error) {
	remaining := b.Limit - b.Buffer.Len()

	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}

		_, _ = b.Buffer.Write(p)
	}

	// Pretend all bytes were consumed so the process
	// does not fail just because output exceeded our limit.
	return len(p), nil
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	_ = json.NewEncoder(w).Encode(data)
}

func enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// Development frontend
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

	// -----------------------------
	// Validate input
	// -----------------------------

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

	// -----------------------------
	// Temporary workspace
	// -----------------------------

	dir, err := os.MkdirTemp("", "golab-*")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Could not create workspace",
		})
		return
	}

	defer os.RemoveAll(dir)

	source := filepath.Join(dir, "main.go")

	if err := os.WriteFile(source, []byte(req.Code), 0600); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Could not write source file",
		})
		return
	}

	// -----------------------------
	// Binary name
	// -----------------------------

	binaryName := "program"

	if runtime.GOOS == "windows" {
		binaryName = "program.exe"
	}

	binary := filepath.Join(dir, binaryName)

	// -----------------------------
	// Compile
	// -----------------------------

	compileCtx, compileCancel := context.WithTimeout(
		context.Background(),
		compileTimeout,
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

	var compileOutput LimitBuffer
	compileOutput.Limit = maxOutputSize

	compileCmd.Stdout = &compileOutput
	compileCmd.Stderr = &compileOutput

	err = compileCmd.Run()

	// Compilation timeout
	if compileCtx.Err() == context.DeadlineExceeded {
		writeJSON(w, http.StatusOK, RunResponse{
			Stdout:   "",
			Stderr:   "Compilation timed out after 30 seconds.",
			ExitCode: 124,
			Runtime:  compileTimeout.Milliseconds(),
		})
		return
	}

	// Compilation error
	if err != nil {
		writeJSON(w, http.StatusOK, RunResponse{
			Stdout:   "",
			Stderr:   compileOutput.Buffer.String(),
			ExitCode: 1,
			Runtime:  0,
		})
		return
	}

	// -----------------------------
	// Execute
	// -----------------------------

	ctx, cancel := context.WithTimeout(
		context.Background(),
		runTimeout,
	)
	defer cancel()

	runCmd := exec.CommandContext(ctx, binary)

	runCmd.Dir = dir

	// stdin
	runCmd.Stdin = strings.NewReader(req.Input)

	var output LimitBuffer
	output.Limit = maxOutputSize

	runCmd.Stdout = &output
	runCmd.Stderr = &output

	start := time.Now()

	err = runCmd.Run()

	runtimeMs := time.Since(start).Milliseconds()

	// -----------------------------
	// Execution timeout
	// -----------------------------

	if ctx.Err() == context.DeadlineExceeded {
		writeJSON(w, http.StatusOK, RunResponse{
			Stdout:   "",
			Stderr:   "Execution timed out after 5 seconds.",
			ExitCode: 124,
			Runtime:  runtimeMs,
		})
		return
	}

	// -----------------------------
	// Runtime error / panic
	// -----------------------------

	if err != nil {
		writeJSON(w, http.StatusOK, RunResponse{
			Stdout:   "",
			Stderr:   output.Buffer.String(),
			ExitCode: 1,
			Runtime:  runtimeMs,
		})
		return
	}

	// -----------------------------
	// Success
	// -----------------------------

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

	if err := http.ListenAndServe(
		"0.0.0.0:"+port,
		handler,
	); err != nil {
		panic(err)
	}
}
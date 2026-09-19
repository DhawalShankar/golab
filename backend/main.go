package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	maxCodeSize   = 10 * 1024        // 10 KB
	maxInputSize  = 10 * 1024        // 10 KB
	maxOutputSize = 1 * 1024 * 1024  // 1 MB

	compileTimeout = 90 * time.Second
	runTimeout     = 5 * time.Second
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

type LimitedBuffer struct {
	Buffer    bytes.Buffer
	Limit     int
	Truncated bool
}

func (b *LimitedBuffer) Write(p []byte) (int, error) {
	remaining := b.Limit - b.Buffer.Len()

	if remaining <= 0 {
		b.Truncated = true
		return len(p), nil
	}

	if len(p) > remaining {
		_, _ = b.Buffer.Write(p[:remaining])
		b.Truncated = true
		return len(p), nil
	}

	_, _ = b.Buffer.Write(p)

	return len(p), nil
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("JSON encode error: %v", err)
	}
}

func enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		frontendURL := os.Getenv("FRONTEND_URL")

		if frontendURL == "" {
			frontendURL = "http://localhost:3000"
		}

		w.Header().Set(
			"Access-Control-Allow-Origin",
			frontendURL,
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

	// Limit request body.
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)

	var req RunRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
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

	log.Println("========== NEW RUN ==========")
	log.Printf("Code size: %d bytes", len(req.Code))
	log.Printf("Input size: %d bytes", len(req.Input))

	// ---------------------------------------------------------
	// Temporary workspace
	// ---------------------------------------------------------

	dir, err := os.MkdirTemp("", "golab-*")
	if err != nil {
		log.Printf("Workspace error: %v", err)

		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Could not create workspace",
		})
		return
	}

	defer os.RemoveAll(dir)

	source := filepath.Join(dir, "main.go")

	if err := os.WriteFile(source, []byte(req.Code), 0600); err != nil {
		log.Printf("File write error: %v", err)

		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Could not write source file",
		})
		return
	}

	log.Printf("Workspace: %s", dir)

	// ---------------------------------------------------------
	// Detect OS
	// ---------------------------------------------------------

	binaryName := "program"

	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}

	binary := filepath.Join(dir, binaryName)

	// ---------------------------------------------------------
	// Compilation
	// ---------------------------------------------------------

	log.Println("Starting compilation...")

	compileCtx, compileCancel := context.WithTimeout(
		context.Background(),
		compileTimeout,
	)
	defer compileCancel()

	compileCmd := exec.CommandContext(
		compileCtx,
		"go",
		"build",
		"-trimpath",
		"-o",
		binary,
		source,
	)

	compileCmd.Dir = dir

	compileCmd.Env = append(
		os.Environ(),
		"GOTOOLCHAIN=local",
		"CGO_ENABLED=0",
	)

	var compileStdout LimitedBuffer
	var compileStderr LimitedBuffer

	compileStdout.Limit = maxOutputSize
	compileStderr.Limit = maxOutputSize

	compileCmd.Stdout = &compileStdout
	compileCmd.Stderr = &compileStderr

	compileStart := time.Now()

	err = compileCmd.Run()

	compileRuntime := time.Since(compileStart).Milliseconds()

	log.Printf(
		"Compilation finished in %d ms",
		compileRuntime,
	)

	// Compilation timeout.
	if compileCtx.Err() == context.DeadlineExceeded {
		log.Println("COMPILATION TIMEOUT")

		writeJSON(w, http.StatusOK, RunResponse{
			Stdout:   "",
			Stderr:   "Compilation timed out.",
			ExitCode: 124,
			Runtime:  compileRuntime,
		})
		return
	}

	// Compilation error.
	if err != nil {
		log.Printf(
			"Compilation failed: %v",
			err,
		)

		stderr := compileStderr.Buffer.String()

		if stderr == "" {
			stderr = compileStdout.Buffer.String()
		}

		writeJSON(w, http.StatusOK, RunResponse{
			Stdout:   "",
			Stderr:   stderr,
			ExitCode: 1,
			Runtime:  compileRuntime,
		})
		return
	}

	log.Println("Compilation successful")

	// ---------------------------------------------------------
	// Execution
	// ---------------------------------------------------------

	log.Println("Starting execution...")

	ctx, cancel := context.WithTimeout(
		context.Background(),
		runTimeout,
	)
	defer cancel()

	runCmd := exec.CommandContext(
		ctx,
		binary,
	)

	runCmd.Dir = dir

	// stdin
	runCmd.Stdin = strings.NewReader(req.Input)

	var stdout LimitedBuffer
	var stderr LimitedBuffer

	stdout.Limit = maxOutputSize
	stderr.Limit = maxOutputSize

	runCmd.Stdout = &stdout
	runCmd.Stderr = &stderr

	start := time.Now()

	err = runCmd.Run()

	runtimeMs := time.Since(start).Milliseconds()

	log.Printf(
		"Execution finished in %d ms",
		runtimeMs,
	)

	// Timeout.
	if ctx.Err() == context.DeadlineExceeded {
		log.Println("EXECUTION TIMEOUT")

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
		log.Printf(
			"Runtime error: %v",
			err,
		)

		writeJSON(w, http.StatusOK, RunResponse{
			Stdout:   stdout.Buffer.String(),
			Stderr:   stderr.Buffer.String(),
			ExitCode: 1,
			Runtime:  runtimeMs,
		})
		return
	}

	// Success.
	log.Println("EXECUTION SUCCESS")

	writeJSON(w, http.StatusOK, RunResponse{
		Stdout:   stdout.Buffer.String(),
		Stderr:   stderr.Buffer.String(),
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

	address := "0.0.0.0:" + port

	log.Printf("GoLab runner running on %s", address)

	if err := http.ListenAndServe(address, handler); err != nil {
		log.Fatal(err)
	}
}
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
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxCodeSize   = 10 * 1024       // 10 KB
	maxInputSize  = 10 * 1024       // 10 KB
	maxOutputSize = 1 * 1024 * 1024 // 1 MB

	compileTimeout = 90 * time.Second
	runTimeout     = 5 * time.Second

	defaultCompilerImage = "golab-compiler:latest"
	defaultRuntimeImage  = "golab-runtime:latest"
)

// Keep the MVP single-job for the small EC2 instance.
var runLock sync.Mutex

type RunRequest struct {
	Code  string `json:"code"`
	Input string `json:"input"`
}

type RunResponse struct {
	Stdout      string `json:"stdout"`
	Stderr      string `json:"stderr"`
	ExitCode    int    `json:"exitCode"`
	Runtime     int64  `json:"runtimeMs"` // Backward-compatible field for the frontend.
	CompileMs   int64  `json:"compileMs"`
	ExecutionMs int64  `json:"executionMs"`
	TotalMs     int64  `json:"totalMs"`
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

		w.Header().Set("Access-Control-Allow-Origin", frontendURL)
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

func getenv(key, fallback string) string {
	value := os.Getenv(key)

	if value == "" {
		return fallback
	}

	return value
}

func removeContainer(name string) {
	_ = exec.Command("docker", "rm", "-f", name).Run()
}

// ---------------------------------------------------------
// Compile inside Docker compiler container
// ---------------------------------------------------------

func compileInDocker(
	dir string,
	image string,
) (string, int, int64, error) {

	ctx, cancel := context.WithTimeout(
		context.Background(),
		compileTimeout,
	)
	defer cancel()

	containerName := "golab-compile-" + filepath.Base(dir)

	args := []string{
		"run",
		"--rm",
		"--init",

		// No network.
		"--network", "none",

		// Compiler resource limits.
		"--memory", "512m",
		"--memory-swap", "512m",
		"--cpus", "1.0",
		"--pids-limit", "128",

		// Drop capabilities.
		"--cap-drop", "ALL",

		// Prevent privilege escalation.
		"--security-opt", "no-new-privileges:true",

		// Read-only root filesystem.
		"--read-only",

		// Writable temporary storage.
		"--tmpfs",
		"/tmp:rw,nosuid,nodev,size=256m",

		// Non-root user.
		"--user", "10001:10001",

		// Persistent Go build cache across submissions.
		"--mount",
		"type=volume,source=golab-go-cache,target=/gocache",
		"--env",
		"GOCACHE=/gocache",

		"--name",
		containerName,

		// Temporary submission workspace.
		"--mount",
		"type=bind,source=" + dir + ",target=/workspace",

		"--workdir",
		"/workspace",

		image,

		"go",
		"build",
		"-trimpath",
		"-o",
		"/workspace/program",
		"/workspace/main.go",
	}

	cmd := exec.CommandContext(
		ctx,
		"docker",
		args...,
	)

	var stdout LimitedBuffer
	var stderr LimitedBuffer

	stdout.Limit = maxOutputSize
	stderr.Limit = maxOutputSize

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()

	err := cmd.Run()

	runtimeMs := time.Since(start).Milliseconds()

	if ctx.Err() == context.DeadlineExceeded {
		log.Printf(
			"Compilation timeout. Removing %s",
			containerName,
		)

		removeContainer(containerName)

		return "", 124, runtimeMs, ctx.Err()
	}

	exitCode := 0

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	compileOutput := stderr.Buffer.String()

	if compileOutput == "" {
		compileOutput = stdout.Buffer.String()
	}

	return compileOutput, exitCode, runtimeMs, err
}

// ---------------------------------------------------------
// Execute inside tiny runtime container
// ---------------------------------------------------------

func runInDocker(
	dir string,
	image string,
	input string,
) (string, string, int, int64, error) {

	containerName := "golab-run-" + filepath.Base(dir)

	args := []string{
		"run",
		"--init",

		// No network access.
		"--network", "none",

		// Runtime limits.
		"--memory", "128m",
		"--memory-swap", "128m",
		"--cpus", "0.5",
		"--pids-limit", "32",

		// Security restrictions.
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges:true",

		// Read-only root filesystem.
		"--read-only",

		// Writable temporary filesystem.
		"--tmpfs",
		"/tmp:rw,noexec,nosuid,nodev,size=32m",

		// File limits.
		"--ulimit",
		"nofile=64:64",

		"--ulimit",
		"fsize=1048576:1048576",

		"--ulimit",
		"core=0",

		// Non-root.
		"--user", "10001:10001",

		"--name",
		containerName,

		// Read-only access to compiled binary.
		"--mount",
		"type=bind,source="+dir+",target=/workspace,readonly",

		"--workdir",
		"/workspace",

		image,

		"/workspace/program",
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		runTimeout,
	)
	defer cancel()

	cmd := exec.CommandContext(
		ctx,
		"docker",
		args...,
	)

	// IMPORTANT:
	// Attached mode means program receives stdin directly.
	cmd.Stdin = strings.NewReader(input)

	var stdout LimitedBuffer
	var stderr LimitedBuffer

	stdout.Limit = maxOutputSize
	stderr.Limit = maxOutputSize

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()

	err := cmd.Run()

	runtimeMs := time.Since(start).Milliseconds()

	// ---------------------------------------------------------
	// TIMEOUT
	// ---------------------------------------------------------

	if ctx.Err() == context.DeadlineExceeded {
		log.Printf(
			"Execution timeout. Killing %s",
			containerName,
		)

		removeContainer(containerName)

		return "",
			"Execution timed out after 5 seconds.",
			124,
			runtimeMs,
			context.DeadlineExceeded
	}

	// ---------------------------------------------------------
	// Docker / runtime error
	// ---------------------------------------------------------

	if err != nil {
		exitCode := 1

		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}

		// Remove container if it still exists.
		removeContainer(containerName)

		return stdout.Buffer.String(),
			stderr.Buffer.String(),
			exitCode,
			runtimeMs,
			err
	}

	// ---------------------------------------------------------
	// Successful execution
	// ---------------------------------------------------------

	removeContainer(containerName)

	return stdout.Buffer.String(),
		stderr.Buffer.String(),
		0,
		runtimeMs,
		nil
}

// ---------------------------------------------------------
// POST /run
// ---------------------------------------------------------

func runCode(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
			"error": "Method not allowed",
		})
		return
	}

	// One execution at a time on the small server.
	if !runLock.TryLock() {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": "Runner is busy. Please try again in a moment.",
		})
		return
	}

	defer runLock.Unlock()

	// Limit HTTP request body.
	r.Body = http.MaxBytesReader(
		w,
		r.Body,
		32*1024,
	)

	var req RunRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
		})
		return
	}

	req.Code = strings.TrimSpace(req.Code)

	// ---------------------------------------------------------
	// Validation
	// ---------------------------------------------------------

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

	compilerImage := getenv(
		"GOLAB_COMPILER_IMAGE",
		defaultCompilerImage,
	)

	runtimeImage := getenv(
		"GOLAB_RUNTIME_IMAGE",
		defaultRuntimeImage,
	)

	log.Println("========== NEW DOCKER RUN ==========")
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

	// Container user 10001 needs access.
	if err := os.Chmod(dir, 0777); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Could not configure workspace",
		})
		return
	}

	source := filepath.Join(dir, "main.go")

	if err := os.WriteFile(
		source,
		[]byte(req.Code),
		0644,
	); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Could not write source file",
		})
		return
	}

	log.Printf("Workspace: %s", dir)

	// Total runner time starts after validation/workspace setup.
	totalStart := time.Now()

	// ---------------------------------------------------------
	// COMPILE
	// ---------------------------------------------------------

	log.Println("Compiling inside Docker...")

	compileOutput,
		compileExitCode,
		compileRuntime,
		compileErr := compileInDocker(
		dir,
		compilerImage,
	)

	if compileExitCode == 124 {
		writeJSON(w, http.StatusOK, RunResponse{
			Stdout:      "",
			Stderr:      "Compilation timed out after 90 seconds.",
			ExitCode:    124,
			Runtime:     compileRuntime,
			CompileMs:   compileRuntime,
			ExecutionMs: 0,
			TotalMs:     time.Since(totalStart).Milliseconds(),
		})
		return
	}

	if compileErr != nil {
		writeJSON(w, http.StatusOK, RunResponse{
			Stdout:      "",
			Stderr:      compileOutput,
			ExitCode:    compileExitCode,
			Runtime:     compileRuntime,
			CompileMs:   compileRuntime,
			ExecutionMs: 0,
			TotalMs:     time.Since(totalStart).Milliseconds(),
		})
		return
	}

	log.Println("Compilation successful")

	// ---------------------------------------------------------
	// RUN
	// ---------------------------------------------------------

	log.Println("Executing inside Docker...")

	stdout,
		stderr,
		exitCode,
		runtimeMs,
		runErr := runInDocker(
		dir,
		runtimeImage,
		req.Input,
	)

	if exitCode == 124 {
		writeJSON(w, http.StatusOK, RunResponse{
			Stdout:      "",
			Stderr:      "Execution timed out after 5 seconds.",
			ExitCode:    124,
			Runtime:     runtimeMs,
			CompileMs:   compileRuntime,
			ExecutionMs: runtimeMs,
			TotalMs:     time.Since(totalStart).Milliseconds(),
		})
		return
	}

	if runErr != nil {
		writeJSON(w, http.StatusOK, RunResponse{
			Stdout:      stdout,
			Stderr:      stderr,
			ExitCode:    exitCode,
			Runtime:     runtimeMs,
			CompileMs:   compileRuntime,
			ExecutionMs: runtimeMs,
			TotalMs:     time.Since(totalStart).Milliseconds(),
		})
		return
	}

	// ---------------------------------------------------------
	// Result
	// ---------------------------------------------------------

	writeJSON(w, http.StatusOK, RunResponse{
		Stdout:      stdout,
		Stderr:      stderr,
		ExitCode:    exitCode,
		Runtime:     runtimeMs,
		CompileMs:   compileRuntime,
		ExecutionMs: runtimeMs,
		TotalMs:     time.Since(totalStart).Milliseconds(),
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

	log.Printf(
		"GoLab runner running on %s",
		address,
	)

	log.Println("Docker sandbox mode enabled")

	if err := http.ListenAndServe(
		address,
		handler,
	); err != nil {
		log.Fatal(err)
	}
}

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

	if req.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Code cannot be empty",
		})
		return
	}

	if len(req.Code) > maxCodeSize {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Code too large",
		})
		return
	}

	if len(req.Input) > maxInputSize {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Input too large",
		})
		return
	}

	log.Println("========== NEW RUN ==========")
	log.Printf("Code size: %d bytes\n", len(req.Code))
	log.Printf("Input size: %d bytes\n", len(req.Input))

	// Temporary workspace
	dir, err := os.MkdirTemp("", "golab-*")
	if err != nil {
		log.Printf("ERROR creating temp directory: %v\n", err)

		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Could not create workspace",
		})
		return
	}

	defer os.RemoveAll(dir)

	log.Printf("Workspace: %s\n", dir)

	source := filepath.Join(dir, "main.go")

	if err := os.WriteFile(source, []byte(req.Code), 0600); err != nil {
		log.Printf("ERROR writing source: %v\n", err)

		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Could not write source file",
		})
		return
	}

	log.Println("Source written successfully")

	// Check Go installation
	versionCmd := exec.Command("go", "version")

	versionOutput, versionErr := versionCmd.CombinedOutput()

	if versionErr != nil {
		log.Printf("ERROR running 'go version': %v\n", versionErr)
		log.Printf("Output: %s\n", string(versionOutput))
	} else {
		log.Printf("Go: %s\n", strings.TrimSpace(string(versionOutput)))
	}

	// Binary name
	binaryName := "program"

	if runtime.GOOS == "windows" {
		binaryName = "program.exe"
	}

	binary := filepath.Join(dir, binaryName)

	// ---------------------------------------------------------
	// COMPILE
	// ---------------------------------------------------------

	log.Println("Starting compilation...")

	compileCtx, compileCancel := context.WithTimeout(
		context.Background(),
		60*time.Second,
	)
	defer compileCancel()

	compileStart := time.Now()

	compileCmd := exec.CommandContext(
		compileCtx,
		"go",
		"build",
		"-o",
		binary,
		source,
	)

	compileCmd.Dir = dir

	compileOutput, err := compileCmd.CombinedOutput()

	compileDuration := time.Since(compileStart)

	log.Printf(
		"Compilation finished after %d ms\n",
		compileDuration.Milliseconds(),
	)

	if len(compileOutput) > maxOutputSize {
		compileOutput = compileOutput[:maxOutputSize]
	}

	log.Printf("Compile output: %s\n", string(compileOutput))

	if compileCtx.Err() == context.DeadlineExceeded {
		log.Println("COMPILATION TIMEOUT")

		writeJSON(w, http.StatusOK, RunResponse{
			Stdout:   "",
			Stderr:   "Compilation timed out after 60 seconds.",
			ExitCode: 124,
			Runtime:  compileDuration.Milliseconds(),
		})
		return
	}

	if err != nil {
		log.Printf("COMPILATION ERROR: %v\n", err)

		writeJSON(w, http.StatusOK, RunResponse{
			Stdout:   "",
			Stderr:   string(compileOutput),
			ExitCode: 1,
			Runtime:  compileDuration.Milliseconds(),
		})
		return
	}

	log.Println("Compilation successful")
	log.Printf("Binary: %s\n", binary)

	// ---------------------------------------------------------
	// EXECUTE
	// ---------------------------------------------------------

	log.Println("Starting program execution...")

	ctx, cancel := context.WithTimeout(
		context.Background(),
		runTimeout,
	)
	defer cancel()

	runCmd := exec.CommandContext(ctx, binary)
	runCmd.Dir = dir
	runCmd.Stdin = strings.NewReader(req.Input)

	var output bytes.Buffer

	runCmd.Stdout = &output
	runCmd.Stderr = &output

	start := time.Now()

	err = runCmd.Run()

	runtimeMs := time.Since(start).Milliseconds()

	result := output.Bytes()

	if len(result) > maxOutputSize {
		result = result[:maxOutputSize]
	}

	log.Printf("Execution finished after %d ms\n", runtimeMs)

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

	if err != nil {
		log.Printf("RUNTIME ERROR: %v\n", err)
		log.Printf("Output: %s\n", string(result))

		writeJSON(w, http.StatusOK, RunResponse{
			Stdout:   "",
			Stderr:   string(result),
			ExitCode: 1,
			Runtime:  runtimeMs,
		})
		return
	}

	log.Println("EXECUTION SUCCESS")
	log.Printf("Output: %s\n", string(result))

	writeJSON(w, http.StatusOK, RunResponse{
		Stdout:   string(result),
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
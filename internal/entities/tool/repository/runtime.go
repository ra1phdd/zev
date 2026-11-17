package repository

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/ra1phdd/zev/internal/core/childprocess"
)

const (
	executionTimeout = 10 * time.Second
	windowsOS        = "windows"
)

// Execute runs a literal request with a ten-second deadline (or the shorter
// caller deadline). Restricted requests use only in-process builtins and repeat
// CheckReadOnly's checks. Unrestricted execution requires prior authorization
// by the caller; this adapter does not prompt or call a reviewer.
// ExitCode is -1 for denial/start/I/O failures, 124 for timeout, and 130 for
// cancellation. Reason is a stable diagnostic, not raw OS or command text.
// Output combines stdout/stderr, escapes terminal controls, and is at most 32 KiB.
func (e *Executor) Execute(ctx context.Context, request ExecRequest, restricted bool) Result {
	ctx, cancel := context.WithTimeout(ctx, executionTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return failedContext(ctx, Result{ExitCode: -1})
	}
	if !restricted {
		return e.executeApproved(ctx, request)
	}
	file, b, err := e.openBuiltin(request)
	if err != nil {
		return Result{Denied: true, ExitCode: -1, Reason: "request is not an allowed read-only builtin or path"}
	}
	var out boundedOutput
	if request.Command == builtinPWD {
		_, err = out.Write([]byte(e.root + "\n"))
	} else {
		err = readBuiltin(ctx, file, request, b, &out)
	}
	closeErr := file.Close()
	result := out.result()
	if err != nil || closeErr != nil {
		result.ExitCode = -1
		result.Reason = "builtin I/O failed"
	}
	return failedContext(ctx, result)
}

func (e *Executor) executeApproved(ctx context.Context, request ExecRequest) Result {
	if !validProcessRequest(request) {
		return Result{Denied: true, ExitCode: -1, Reason: "invalid executable or argument limits"}
	}
	root, err := e.openRoot()
	if err != nil {
		return Result{Denied: true, ExitCode: -1, Reason: "execution root is unavailable or changed"}
	}
	if err := root.Close(); err != nil {
		return Result{ExitCode: -1, Reason: "execution root close failed"}
	}
	executable, env := processEnvironment(request.Command, e.root)
	// Authorization belongs to the service; argv is literal and no implicit shell is used.
	cmd := exec.CommandContext(ctx, executable, request.Args...) //nolint:gosec // This branch requires explicit caller authorization.
	cmd.Dir = e.root
	cmd.Env = env
	var out boundedOutput
	cmd.Stdout = &out
	cmd.Stderr = &out
	err = childprocess.Run(ctx, cmd)
	result := out.result()
	if err != nil {
		result.ExitCode = -1
		result.Reason = "process execution failed"
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			result.ExitCode = exitErr.ExitCode()
			result.Reason = "process exited unsuccessfully"
		}
	}
	return failedContext(ctx, result)
}

func validProcessRequest(request ExecRequest) bool {
	if request.Command == "" || len(request.Command) > maxPath || strings.ContainsRune(request.Command, 0) || len(request.Args) > 64 || len(request.Reason) > 2048 {
		return false
	}
	total := len(request.Command)
	for _, arg := range request.Args {
		if len(arg) > maxPath || strings.ContainsRune(arg, 0) {
			return false
		}
		total += len(arg)
	}
	return total <= 16*1024
}

func processEnvironment(command, root string) (string, []string) {
	searchPath := "/usr/bin:/bin"
	env := []string{"PATH=" + searchPath, "LANG=C", "LC_ALL=C"}
	if runtime.GOOS == windowsOS {
		systemRoot := os.Getenv("SystemRoot")
		searchPath = filepath.Join(systemRoot, "System32")
		env = []string{"PATH=" + searchPath, "SystemRoot=" + systemRoot, "LANG=C", "LC_ALL=C"}
	}
	if filepath.IsAbs(command) {
		return command, env
	}
	if strings.ContainsAny(command, "/\\") {
		return filepath.Join(root, command), env
	}
	for _, directory := range filepath.SplitList(searchPath) {
		candidate := filepath.Join(directory, command)
		if runtime.GOOS == windowsOS && filepath.Ext(candidate) == "" {
			candidate += ".exe"
		}
		info, err := os.Stat(candidate) //nolint:gosec // Approved execution deliberately permits arbitrary executables; search is limited to the explicit system path.
		if err == nil && info.Mode().IsRegular() && (runtime.GOOS == windowsOS || info.Mode().Perm()&0o111 != 0) {
			return candidate, env
		}
	}
	// An absolute missing path avoids exec.CommandContext's ambient PATH lookup.
	return filepath.Join(filepath.SplitList(searchPath)[0], command), env
}

func failedContext(ctx context.Context, result Result) Result {
	if ctx.Err() == nil {
		return result
	}
	result.ExitCode = 130
	result.Reason = "execution canceled"
	if ctx.Err() == context.DeadlineExceeded {
		result.ExitCode = 124
		result.Reason = "execution deadline exceeded"
	}
	return result
}

type boundedOutput struct {
	mu        sync.Mutex
	data      []byte
	truncated bool
}

func (o *boundedOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	n := len(p)
	remaining := maxOutput - len(o.data)
	if n > remaining {
		p = p[:remaining]
		o.truncated = true
	}
	o.data = append(o.data, p...)
	return n, nil
}

func (o *boundedOutput) result() Result {
	o.mu.Lock()
	defer o.mu.Unlock()
	var output strings.Builder
	for _, r := range string(o.data) {
		text := string(r)
		if (unicode.IsControl(r) && r != '\n' && r != '\t') || unicode.Is(unicode.Cf, r) {
			quoted := strconv.QuoteRuneToASCII(r)
			text = quoted[1 : len(quoted)-1]
		}
		if len(text) > maxOutput-output.Len() {
			o.truncated = true
			break
		}
		output.WriteString(text)
	}
	return Result{Output: output.String(), Truncated: o.truncated}
}

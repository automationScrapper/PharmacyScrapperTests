package runner

import (
    "bytes"
    "context"
    "errors"
    "fmt"
    "io"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "time"
)

type TestIndex struct {
    Groups map[string][]string `json:"groups"`
}

type ExecResult struct {
    OK        bool   `json:"ok"`
    Command   string `json:"command"`
    Args      []string `json:"args"`
    ExitCode  int    `json:"exitCode"`
    DurationMs int64 `json:"durationMs"`
    Stdout    string `json:"stdout"`
    Stderr    string `json:"stderr"`
    Error     string `json:"error,omitempty"`
}

func ListTests(root string) (TestIndex, error) {
    idx := TestIndex{Groups: map[string][]string{}}

    entries, err := os.ReadDir(root)
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return idx, nil
        }
        return idx, err
    }

    for _, e := range entries {
        if !e.IsDir() {
            continue
        }
        group := e.Name()
        groupDir := filepath.Join(root, group)
        tests, _ := os.ReadDir(groupDir)
        for _, t := range tests {
            name := t.Name()
            if t.IsDir() {
                continue
            }
            if strings.HasSuffix(name, ".js") || strings.HasSuffix(name, ".mjs") {
                idx.Groups[group] = append(idx.Groups[group], name)
            }
        }
    }
    return idx, nil
}

func RunAll(playRoot string) ExecResult {
    return runWithEnv(playRoot, []string{"node", "run.js"}, nil)
}

func RunGroup(playRoot, group string) ExecResult {
    path := filepath.Join("tests", group)
    return runWithEnv(playRoot, []string{"node", "run.js", path}, nil)
}

func RunTest(playRoot, group, test string) ExecResult {
    path := filepath.Join("tests", group, test)
    return runWithEnv(playRoot, []string{"node", "run.js", path}, nil)
}

// RunBateoFechaRange sets ERP_* env vars and runs the composed flow test.
func RunBateoFechaRange(playRoot, baseURL, user, pass string) ExecResult {
    args := []string{"node", "run.js", filepath.Join("tests", "bateo", "fecha_rango.js")}
    env := map[string]string{
        "ERP_BASE_URL": baseURL,
        "ERP_USER":     user,
        "ERP_PASS":     pass,
        // default to headless for server mode
        "HEADLESS":     "1",
    }
    return runWithEnv(playRoot, args, env)
}

// RunBateoExportForDate runs the bateo flow for a specific date (YYYY-MM-DD).
// The effective range is: start = first day of that month, end = given date + 1 day.
func RunBateoExportForDate(playRoot, baseURL, user, pass, dateStr string) ExecResult {
    args := []string{"node", "run.js", filepath.Join("tests", "bateo", "fecha_rango.js")}
    env := map[string]string{
        "ERP_BASE_URL": baseURL,
        "ERP_USER":     user,
        "ERP_PASS":     pass,
        "QUERY_DATE":   dateStr,
        // default to headless for server mode
        "HEADLESS":     "1",
    }
    return runWithEnv(playRoot, args, env)
}

func runWithEnv(playRoot string, args []string, extraEnv map[string]string) ExecResult {
    start := time.Now()
    // Resolve playRoot to an absolute directory
    dir := resolvePlayRoot(playRoot)
    // Run node scripts relative to automation project
    cmd := exec.Command(args[0], args[1:]...)
    cmd.Dir = dir
    if len(extraEnv) > 0 {
        // Merge env with current process env
        env := os.Environ()
        for k, v := range extraEnv {
            env = append(env, fmt.Sprintf("%s=%s", k, v))
        }
        cmd.Env = env
    }

    var outBuf, errBuf bytes.Buffer
    // Mirror child output to server stdout/stderr for live visibility,
    // while still capturing buffers for API responses.
    cmd.Stdout = io.MultiWriter(&outBuf, os.Stdout)
    cmd.Stderr = io.MultiWriter(&errBuf, os.Stderr)

    // Add a generous timeout
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
    defer cancel()

    cmd = commandWithContext(ctx, cmd)

    exitCode := 0
    err := cmd.Run()
    if err != nil {
        if exitErr, ok := err.(*exec.ExitError); ok {
            exitCode = exitErr.ExitCode()
        } else {
            exitCode = -1
        }
    }

    res := ExecResult{
        OK:         err == nil,
        Command:    args[0],
        Args:       args[1:],
        ExitCode:   exitCode,
        DurationMs: time.Since(start).Milliseconds(),
        Stdout:     outBuf.String(),
        Stderr:     errBuf.String(),
    }
    if err != nil {
        res.Error = err.Error()
        if errors.Is(err, exec.ErrNotFound) {
            res.Error = fmt.Sprintf("%s (ensure Node.js is installed)", res.Error)
        }
    }
    return res
}

// commandWithContext re-binds an existing exec.Command to respect a context by
// reconstructing it. This avoids duplicating argument wiring.
func commandWithContext(ctx context.Context, base *exec.Cmd) *exec.Cmd {
    c := exec.CommandContext(ctx, base.Path, base.Args[1:]...)
    c.Dir = base.Dir
    c.Env = base.Env
    c.Stdout = base.Stdout
    c.Stderr = base.Stderr
    c.Stdin = base.Stdin
    return c
}

// resolvePlayRoot tries to find the automation directory regardless of where the
// binary is invoked from: first as given (absolute), then relative to CWD, then
// relative to the executable's directory.
func resolvePlayRoot(playRoot string) string {
    if filepath.IsAbs(playRoot) {
        if fi, err := os.Stat(playRoot); err == nil && fi.IsDir() {
            return playRoot
        }
        return playRoot
    }
    if cwd, err := os.Getwd(); err == nil {
        cand := filepath.Join(cwd, playRoot)
        if fi, err := os.Stat(cand); err == nil && fi.IsDir() {
            return cand
        }
    }
    if exe, err := os.Executable(); err == nil {
        exedir := filepath.Dir(exe)
        cand := filepath.Join(exedir, playRoot)
        if fi, err := os.Stat(cand); err == nil && fi.IsDir() {
            return cand
        }
    }
    return playRoot
}

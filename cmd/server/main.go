package main

import (
    "fmt"
    "encoding/json"
    "io"
    "log"
    "net/http"
    "os"
    "path/filepath"
    "strings"
    "time"

    "automation/api/internal/runner"
    "automation/api/internal/ingest"
)

func main() {
    mux := http.NewServeMux()

    mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        writeJSON(w, http.StatusOK, map[string]any{"ok": true})
    })

    mux.HandleFunc("/tests", func(w http.ResponseWriter, r *http.Request) {
        idx, err := runner.ListTests("automation/tests")
        if err != nil {
            writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
            return
        }
        writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": idx})
    })

    // POST /bateo/ventas/fecha-rango -> performs login, sets date range (first of month to tomorrow),
    // triggers export, ingests into SQLite and streams the Excel file
    mux.HandleFunc("/bateo/ventas/fecha-rango", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
            methodNotAllowed(w)
            return
        }
        var body struct {
            BaseURL string `json:"baseUrl"`
            User    string `json:"user"`
            Pass    string `json:"pass"`
        }
        _ = json.NewDecoder(r.Body).Decode(&body)

        baseURL := firstNonEmpty(body.BaseURL, os.Getenv("ERP_BASE_URL"), "http://erpvm.kurigage.com")
        user := firstNonEmpty(body.User, os.Getenv("ERP_USER"), "ricardo.valencia@farmaciasbustillos.com")
        pass := firstNonEmpty(body.Pass, os.Getenv("ERP_PASS"), "P4u1A280325*")

        // Run the full flow (default date = today)
        res := runner.RunBateoExportForDate("automation", baseURL, user, pass, "")
        if !res.OK {
            writeJSON(w, http.StatusBadRequest, res)
            return
        }

        // Determine downloaded file path from stdout
        path := extractDownloadPath(res.Stdout)
        if path == "" {
            writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "failed to determine downloaded file path", "stdout": res.Stdout})
            return
        }

        // Ingest into SQLite
        // date is today by default here
        today := time.Now().Format("2006-01-02")
        rs, re := computeRange(today)
        dbFile := filepath.Join("automation", "data", "erp.sqlite")
        batch, err := ingest.IngestBateoExcel(dbFile, path, rs, re)
        if err != nil {
            log.Printf("ingest error: %v", err)
            w.Header().Set("X-Ingest-OK", "false")
            w.Header().Set("X-Ingest-Error", err.Error())
        } else {
            w.Header().Set("X-Ingest-OK", "true")
            w.Header().Set("X-Ingest-DB", dbFile)
            w.Header().Set("X-Ingest-Batch-Id", fmt.Sprintf("%d", batch.ID))
            w.Header().Set("X-Ingest-Rows", fmt.Sprintf("%d", batch.Rows))
            w.Header().Set("X-Ingest-Range-Start", batch.RangeStart)
            w.Header().Set("X-Ingest-Range-End", batch.RangeEnd)
        }

        // Ensure path is under allowed downloads directory
        abs, _ := filepath.Abs(path)
        dlRoot, _ := filepath.Abs(filepath.Join("automation", "downloads"))
        if !strings.HasPrefix(abs, dlRoot) {
            writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "download path outside allowed directory"})
            return
        }

        f, err := os.Open(abs)
        if err != nil {
            writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
            return
        }
        defer f.Close()

        ctype := contentTypeByExt(filepath.Ext(abs))
        w.Header().Set("Content-Type", ctype)
        w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(abs)+"\"")
        w.WriteHeader(http.StatusOK)
        _, _ = io.Copy(w, f)
    })

    // GET /bateo/ventas/export?date=YYYY-MM-DD
    // Runs the flow for the given date (or today if omitted) and streams the downloaded Excel.
    mux.HandleFunc("/bateo/ventas/export", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodGet {
            methodNotAllowed(w)
            return
        }
        dateStr := strings.TrimSpace(r.URL.Query().Get("date"))

        baseURL := firstNonEmpty(r.URL.Query().Get("baseUrl"), os.Getenv("ERP_BASE_URL"), "http://erpvm.kurigage.com")
        user := firstNonEmpty(r.URL.Query().Get("user"), os.Getenv("ERP_USER"), "ricardo.valencia@farmaciasbustillos.com")
        pass := firstNonEmpty(r.URL.Query().Get("pass"), os.Getenv("ERP_PASS"), "P4u1A280325*")

        // If date is omitted, the Node test defaults to today; we still pass through value (possibly empty)
        res := runner.RunBateoExportForDate("automation", baseURL, user, pass, dateStr)
        if !res.OK {
            writeJSON(w, http.StatusBadRequest, res)
            return
        }

        // Extract download path from stdout
        path := extractDownloadPath(res.Stdout)
        if path == "" {
            writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "failed to determine downloaded file path", "stdout": res.Stdout})
            return
        }

        // Ingest into SQLite before streaming
        // Compute rangeStart = first day of month, rangeEnd = date+1
        rs, re := computeRange(dateStr)
        dbFile := filepath.Join("automation", "data", "erp.sqlite")
        batch, err := ingest.IngestBateoExcel(dbFile, path, rs, re)
        if err != nil {
            log.Printf("ingest error: %v", err)
            // continue to stream file; annotate headers
            w.Header().Set("X-Ingest-OK", "false")
            w.Header().Set("X-Ingest-Error", err.Error())
        } else {
            w.Header().Set("X-Ingest-OK", "true")
            w.Header().Set("X-Ingest-DB", dbFile)
            w.Header().Set("X-Ingest-Batch-Id", fmt.Sprintf("%d", batch.ID))
            w.Header().Set("X-Ingest-Rows", fmt.Sprintf("%d", batch.Rows))
            w.Header().Set("X-Ingest-Range-Start", batch.RangeStart)
            w.Header().Set("X-Ingest-Range-End", batch.RangeEnd)
        }

        // Basic containment: ensure file lives under automation/downloads
        abs, _ := filepath.Abs(path)
        dlRoot, _ := filepath.Abs(filepath.Join("automation", "downloads"))
        if !strings.HasPrefix(abs, dlRoot) {
            writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "download path outside allowed directory"})
            return
        }

        f, err := os.Open(abs)
        if err != nil {
            writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
            return
        }
        defer f.Close()

        // Content-Type by extension
        ctype := contentTypeByExt(filepath.Ext(abs))
        w.Header().Set("Content-Type", ctype)
        w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(abs)+"\"")
        w.WriteHeader(http.StatusOK)
        _, _ = io.Copy(w, f)
    })

    // POST /run/all
    mux.HandleFunc("/run/all", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
            methodNotAllowed(w)
            return
        }
        res := runner.RunAll("automation")
        status := http.StatusOK
        if !res.OK {
            status = http.StatusBadRequest
        }
        writeJSON(w, status, res)
    })

    // Dynamic: /run/{group} or /run/{group}/{test}
    mux.HandleFunc("/run/", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
            methodNotAllowed(w)
            return
        }
        // Expected: /run/<group>[/<test>]
        rest := strings.TrimPrefix(r.URL.Path, "/run/")
        parts := strings.Split(rest, "/")
        if len(parts) == 0 || parts[0] == "" {
            http.NotFound(w, r)
            return
        }
        group := filepath.Clean(parts[0])
        if strings.Contains(group, "..") {
            writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid group"})
            return
        }

        // optional test
        if len(parts) == 1 {
            res := runner.RunGroup("automation", group)
            status := http.StatusOK
            if !res.OK {
                status = http.StatusBadRequest
            }
            writeJSON(w, status, res)
            return
        }

        test := filepath.Clean(parts[1])
        if strings.Contains(test, "..") {
            writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid test"})
            return
        }

        // Accept test with or without extension; default to .js
        if !hasKnownTestExt(test) {
            if _, err := os.Stat(filepath.Join("automation", "tests", group, test+".js")); err == nil {
                test = test + ".js"
            } else if _, err := os.Stat(filepath.Join("automation", "tests", group, test+".mjs")); err == nil {
                test = test + ".mjs"
            }
        }

        res := runner.RunTest("automation", group, test)
        status := http.StatusOK
        if !res.OK {
            status = http.StatusBadRequest
        }
        writeJSON(w, status, res)
    })

    addr := ":8080"
    log.Printf("Starting API server on %s", addr)
    if err := http.ListenAndServe(addr, withCORS(mux)); err != nil {
        log.Fatal(err)
    }
}

func writeJSON(w http.ResponseWriter, code int, v any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    _ = json.NewEncoder(w).Encode(v)
}

func methodNotAllowed(w http.ResponseWriter) {
    writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
}

func withCORS(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
        if r.Method == http.MethodOptions {
            w.WriteHeader(http.StatusNoContent)
            return
        }
        next.ServeHTTP(w, r)
    })
}

func hasKnownTestExt(name string) bool {
    return strings.HasSuffix(name, ".js") || strings.HasSuffix(name, ".mjs")
}

func firstNonEmpty(values ...string) string {
    for _, v := range values {
        if strings.TrimSpace(v) != "" {
            return v
        }
    }
    return ""
}

func extractDownloadPath(stdout string) string {
    // Looks for a line like: [DOWNLOAD] saved to: /abs/path/file.xlsx (12345 bytes)
    lines := strings.Split(stdout, "\n")
    for _, ln := range lines {
        ln = strings.TrimSpace(ln)
        if strings.HasPrefix(ln, "[DOWNLOAD] saved to:") {
            // Split after the prefix
            rest := strings.TrimSpace(strings.TrimPrefix(ln, "[DOWNLOAD] saved to:"))
            // Rest may contain path plus size in parentheses; strip trailing size
            if idx := strings.LastIndex(rest, " ("); idx != -1 {
                rest = strings.TrimSpace(rest[:idx])
            }
            return rest
        }
    }
    return ""
}

func contentTypeByExt(ext string) string {
    switch strings.ToLower(ext) {
    case ".xlsx":
        return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
    case ".xls":
        return "application/vnd.ms-excel"
    case ".csv":
        return "text/csv"
    default:
        return "application/octet-stream"
    }
}

// computeRange returns (start, end) for the given YYYY-MM-DD string where
// start is the first day of the month and end is (date + 1 day), formatted as YYYY-MM-DD.
func computeRange(dateStr string) (string, string) {
    t, err := time.Parse("2006-01-02", dateStr)
    if err != nil {
        // fallback to today
        t = time.Now()
    }
    start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
    // Range end is the day before the consultation day
    end := t.AddDate(0, 0, -1)
    if end.Before(start) {
        end = start
    }
    return start.Format("2006-01-02"), end.Format("2006-01-02")
}

package ingest

import (
    "database/sql"
    "encoding/csv"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "strings"
    "time"
    _ "modernc.org/sqlite"
    "github.com/xuri/excelize/v2"
    xls "github.com/extrame/xls"
)

type BatchInfo struct {
    ID         int64  `json:"id"`
    RangeStart string `json:"rangeStart"`
    RangeEnd   string `json:"rangeEnd"`
    Filename   string `json:"filename"`
    Rows       int    `json:"rows"`
}

func ensureDir(path string) error {
    return os.MkdirAll(path, 0o755)
}

func openDB(dbPath string) (*sql.DB, error) {
    if err := ensureDir(filepath.Dir(dbPath)); err != nil {
        return nil, err
    }
    return sql.Open("sqlite", dbPath)
}

func initSchema(db *sql.DB) error {
    stmts := []string{
        `CREATE TABLE IF NOT EXISTS ingest_batches (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            range_start TEXT NOT NULL,
            range_end   TEXT NOT NULL,
            filename    TEXT NOT NULL,
            created_at  TEXT NOT NULL
        );`,
        `CREATE TABLE IF NOT EXISTS bateo_ventas_rows (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            batch_id  INTEGER NOT NULL,
            row_index INTEGER NOT NULL,
            data_json TEXT NOT NULL,
            FOREIGN KEY(batch_id) REFERENCES ingest_batches(id)
        );`,
        `CREATE INDEX IF NOT EXISTS idx_bateo_rows_batch ON bateo_ventas_rows(batch_id);`,
    }
    for _, s := range stmts {
        if _, err := db.Exec(s); err != nil {
            return err
        }
    }
    return nil
}

// IngestBateoExcel ingests an export file (.xlsx or .csv) into SQLite as JSON rows
// grouped by an ingest batch keyed by the date range.
func IngestBateoExcel(dbPath, exportPath, rangeStart, rangeEnd string) (BatchInfo, error) {
    var info BatchInfo

    db, err := openDB(dbPath)
    if err != nil {
        return info, err
    }
    defer db.Close()

    if err := initSchema(db); err != nil {
        return info, err
    }

    tx, err := db.Begin()
    if err != nil {
        return info, err
    }
    defer func() {
        _ = tx.Rollback()
    }()

    now := time.Now().UTC().Format(time.RFC3339)
    res, err := tx.Exec(`INSERT INTO ingest_batches(range_start, range_end, filename, created_at) VALUES(?,?,?,?)`, rangeStart, rangeEnd, filepath.Base(exportPath), now)
    if err != nil {
        return info, err
    }
    batchID, err := res.LastInsertId()
    if err != nil {
        return info, err
    }

    // Detect extension and route parser
    ext := strings.ToLower(filepath.Ext(exportPath))
    var headers []string
    rowIndex := 0

    insertRow := func(idx int, data map[string]string) error {
        b, _ := json.Marshal(data)
        _, err := tx.Exec(`INSERT INTO bateo_ventas_rows(batch_id, row_index, data_json) VALUES(?,?,?)`, batchID, idx, string(b))
        return err
    }

    switch ext {
    case ".xlsx":
        f, err := excelize.OpenFile(exportPath)
        if err != nil {
            return info, err
        }
        defer func() { _ = f.Close() }()
        sheets := f.GetSheetList()
        if len(sheets) == 0 {
            return info, errors.New("xlsx has no sheets")
        }
        sheet := sheets[0]
        rows, err := f.GetRows(sheet)
        if err != nil {
            return info, err
        }
        for _, r := range rows {
            if headers == nil {
                headers = make([]string, len(r))
                for i, h := range r {
                    headers[i] = normalizeHeader(h, i)
                }
                continue
            }
            // Build row map
            rowIndex++
            data := make(map[string]string, len(headers))
            for i, h := range headers {
                var v string
                if i < len(r) {
                    v = strings.TrimSpace(r[i])
                }
                data[h] = v
            }
            // skip empty rows
            if rowIsEmpty(data) {
                continue
            }
            if err := insertRow(rowIndex, data); err != nil {
                return info, err
            }
        }

    case ".xls":
        wb, err := xls.Open(exportPath, "utf-8")
        if err != nil {
            return info, err
        }
        if wb.NumSheets() == 0 {
            return info, errors.New("xls has no sheets")
        }
        sh := wb.GetSheet(0)
        if sh == nil {
            return info, errors.New("failed to open first xls sheet")
        }
        // Iterate rows: first row -> headers
        for r := 0; r <= int(sh.MaxRow); r++ {
            row := sh.Row(r)
            if row == nil {
                continue
            }
            cols := row.LastCol()
            if headers == nil {
                headers = make([]string, cols)
                for i := 0; i < cols; i++ {
                    headers[i] = normalizeHeader(row.Col(i), i)
                }
                continue
            }
            rowIndex++
            data := make(map[string]string, len(headers))
            for i, h := range headers {
                var v string
                if i < cols {
                    v = strings.TrimSpace(row.Col(i))
                }
                data[h] = v
            }
            if rowIsEmpty(data) {
                continue
            }
            if err := insertRow(rowIndex, data); err != nil {
                return info, err
            }
        }

    case ".csv":
        fi, err := os.Open(exportPath)
        if err != nil {
            return info, err
        }
        defer fi.Close()
        r := csv.NewReader(fi)
        r.FieldsPerRecord = -1
        for {
            rec, err := r.Read()
            if err == io.EOF {
                break
            }
            if err != nil {
                return info, err
            }
            if headers == nil {
                headers = make([]string, len(rec))
                for i, h := range rec {
                    headers[i] = normalizeHeader(h, i)
                }
                continue
            }
            rowIndex++
            data := make(map[string]string, len(headers))
            for i, h := range headers {
                var v string
                if i < len(rec) {
                    v = strings.TrimSpace(rec[i])
                }
                data[h] = v
            }
            if rowIsEmpty(data) {
                continue
            }
            if err := insertRow(rowIndex, data); err != nil {
                return info, err
            }
        }
    default:
        return info, fmt.Errorf("unsupported export extension: %s", ext)
    }

    if err := tx.Commit(); err != nil {
        return info, err
    }

    info = BatchInfo{
        ID:         batchID,
        RangeStart: rangeStart,
        RangeEnd:   rangeEnd,
        Filename:   filepath.Base(exportPath),
        Rows:       rowIndex,
    }
    return info, nil
}

func normalizeHeader(h string, idx int) string {
    h = strings.TrimSpace(h)
    if h == "" {
        return fmt.Sprintf("col_%d", idx+1)
    }
    // Replace spaces and special chars with underscores; lower-case
    h = strings.ToLower(h)
    repl := func(r rune) rune {
        if r >= 'a' && r <= 'z' { return r }
        if r >= '0' && r <= '9' { return r }
        return '_'
    }
    return strings.Map(repl, h)
}

func rowIsEmpty(m map[string]string) bool {
    for _, v := range m {
        if strings.TrimSpace(v) != "" {
            return false
        }
    }
    return true
}

//go:build poc

package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"prods/internal/catalog"
	"prods/internal/inquiries"
)

var errPOC02ResponseLost = errors.New("simulated response loss after commit")

type poc02ImportRow struct {
	ID               string `json:"id"`
	PartNumber       string `json:"part_number"`
	Name             string `json:"name"`
	Manufacturer     string `json:"manufacturer"`
	Category         string `json:"category"`
	ExpectedRevision int64  `json:"expected_revision"`
}

type poc02RowError struct {
	Index int
	Code  string
}

type poc02ImportResult struct {
	Rows       int
	Inserted   int
	Updated    int
	Replay     bool
	Parse      time.Duration
	Validate   time.Duration
	WriterWait time.Duration
	Commit     time.Duration
	Errors     []poc02RowError
}

type poc02Fault int

const (
	poc02NoFault poc02Fault = iota
	poc02FaultBeforeCommit
	poc02FaultAfterCommit
)

type poc02Harness struct {
	store          *Store
	path           string
	gate           chan struct{}
	previewVersion string
	onAdmitted     func()
	holdAdmitted   <-chan struct{}
}

func newPOC02Harness(t *testing.T) *poc02Harness {
	t.Helper()
	path := filepath.Join(t.TempDir(), "poc02.db")
	store, err := CreatePOC(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	_, err = store.db.Exec(`
		CREATE TABLE poc02_categories (
			name TEXT PRIMARY KEY
		);
		CREATE TABLE poc02_product_categories (
			product_id TEXT PRIMARY KEY REFERENCES products(id),
			category_name TEXT NOT NULL REFERENCES poc02_categories(name)
		);
		CREATE TABLE poc02_import_receipts (
			import_key TEXT PRIMARY KEY,
			request_hash TEXT NOT NULL,
			result_json TEXT NOT NULL,
			committed_at TEXT NOT NULL
		);`)
	if err != nil {
		t.Fatal(err)
	}
	gate := make(chan struct{}, 1)
	gate <- struct{}{}
	return &poc02Harness{store: store, path: path, gate: gate, previewVersion: "preview-v1"}
}

func (h *poc02Harness) withWriter(ctx context.Context, fn func() error) (time.Duration, error) {
	started := time.Now()
	select {
	case <-ctx.Done():
		return time.Since(started), ctx.Err()
	case <-h.gate:
	}
	wait := time.Since(started)
	defer func() { h.gate <- struct{}{} }()
	return wait, fn()
}

func (h *poc02Harness) importRows(ctx context.Context, key, preview string, input []poc02ImportRow, fault poc02Fault) (poc02ImportResult, error) {
	started := time.Now()
	raw, err := json.Marshal(input)
	if err != nil {
		return poc02ImportResult{}, err
	}
	var rows []poc02ImportRow
	if err := json.Unmarshal(raw, &rows); err != nil {
		return poc02ImportResult{}, err
	}
	result := poc02ImportResult{Rows: len(rows), Parse: time.Since(started)}
	hashBytes := sha256.Sum256(raw)
	requestHash := hex.EncodeToString(hashBytes[:])
	if replay, found, err := h.importReceipt(ctx, key, requestHash); err != nil {
		return result, err
	} else if found {
		replay.Replay = true
		return replay, nil
	}

	validateStarted := time.Now()
	if preview != h.previewVersion {
		result.Errors = append(result.Errors, poc02RowError{Index: -1, Code: "stale_preview"})
	}
	counts := make(map[string]int, len(rows))
	for _, row := range rows {
		counts[strings.TrimSpace(row.ID)]++
	}
	prepared := make([]catalog.Product, len(rows))
	for index, row := range rows {
		if counts[strings.TrimSpace(row.ID)] > 1 {
			result.Errors = append(result.Errors, poc02RowError{Index: index, Code: "duplicate_id"})
			continue
		}
		if strings.TrimSpace(row.Category) == "" {
			result.Errors = append(result.Errors, poc02RowError{Index: index, Code: "missing_category"})
			continue
		}
		product := catalog.Product{ID: row.ID, PartNumber: row.PartNumber, Name: row.Name, Manufacturer: row.Manufacturer, Status: catalog.Published}
		if err := product.Prepare(); err != nil {
			result.Errors = append(result.Errors, poc02RowError{Index: index, Code: "invalid_product"})
			continue
		}
		prepared[index] = product
	}
	result.Validate = time.Since(validateStarted)
	if len(result.Errors) > 0 {
		return result, nil
	}

	wait, err := h.withWriter(ctx, func() error {
		if h.onAdmitted != nil {
			h.onAdmitted()
		}
		if h.holdAdmitted != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-h.holdAdmitted:
			}
		}
		if replay, found, err := h.importReceipt(ctx, key, requestHash); err != nil {
			return err
		} else if found {
			result = replay
			result.Replay = true
			return nil
		}

		commitStarted := time.Now()
		tx, err := h.store.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		categoryStatement, err := tx.PrepareContext(ctx, `INSERT INTO poc02_categories(name) VALUES (?) ON CONFLICT(name) DO NOTHING`)
		if err != nil {
			return err
		}
		defer categoryStatement.Close()
		insertStatement, err := tx.PrepareContext(ctx, `INSERT INTO products
			(id, part_number, name, manufacturer, description, specification, document_url, status, revision, search_folded, search_projection_version, created_at, updated_at)
			VALUES (?, ?, ?, ?, '', '', '', 'published', 1, ?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer insertStatement.Close()
		updateStatement, err := tx.PrepareContext(ctx, `UPDATE products SET part_number=?, name=?, manufacturer=?, revision=revision+1,
			search_folded=?, search_projection_version=?, updated_at=? WHERE id=? AND revision=?`)
		if err != nil {
			return err
		}
		defer updateStatement.Close()
		linkStatement, err := tx.PrepareContext(ctx, `INSERT INTO poc02_product_categories(product_id, category_name) VALUES (?, ?)
			ON CONFLICT(product_id) DO UPDATE SET category_name=excluded.category_name`)
		if err != nil {
			return err
		}
		defer linkStatement.Close()
		now := time.Now().UTC().Format(time.RFC3339Nano)
		for index, row := range rows {
			product := prepared[index]
			if _, err := categoryStatement.ExecContext(ctx, row.Category); err != nil {
				return err
			}
			var currentRevision int64
			err := tx.QueryRowContext(ctx, `SELECT revision FROM products WHERE id=?`, row.ID).Scan(&currentRevision)
			switch {
			case errors.Is(err, sql.ErrNoRows):
				if row.ExpectedRevision != 0 {
					return fmt.Errorf("row %d expected missing revision %d", index, row.ExpectedRevision)
				}
				if _, err := insertStatement.ExecContext(ctx, product.ID, product.PartNumber, product.Name, product.Manufacturer, product.SearchFolded, product.ProjectionVer, now, now); err != nil {
					return err
				}
				result.Inserted++
			case err != nil:
				return err
			default:
				if currentRevision != row.ExpectedRevision {
					return fmt.Errorf("row %d stale revision: have %d want %d", index, currentRevision, row.ExpectedRevision)
				}
				changed, err := updateStatement.ExecContext(ctx, product.PartNumber, product.Name, product.Manufacturer, product.SearchFolded, product.ProjectionVer, now, product.ID, row.ExpectedRevision)
				if err != nil {
					return err
				}
				if affected, _ := changed.RowsAffected(); affected != 1 {
					return fmt.Errorf("row %d revision changed during commit", index)
				}
				result.Updated++
			}
			if _, err := linkStatement.ExecContext(ctx, row.ID, row.Category); err != nil {
				return err
			}
		}
		if fault == poc02FaultBeforeCommit {
			return errors.New("simulated fault before commit")
		}
		result.Commit = time.Since(commitStarted)
		encoded, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO poc02_import_receipts(import_key, request_hash, result_json, committed_at) VALUES (?, ?, ?, ?)`, key, requestHash, encoded, now); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		if fault == poc02FaultAfterCommit {
			return errPOC02ResponseLost
		}
		return nil
	})
	result.WriterWait = wait
	return result, err
}

func (h *poc02Harness) importReceipt(ctx context.Context, key, requestHash string) (poc02ImportResult, bool, error) {
	var storedHash string
	var encoded []byte
	err := h.store.db.QueryRowContext(ctx, `SELECT request_hash, result_json FROM poc02_import_receipts WHERE import_key=?`, key).Scan(&storedHash, &encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return poc02ImportResult{}, false, nil
	}
	if err != nil {
		return poc02ImportResult{}, false, err
	}
	if storedHash != requestHash {
		return poc02ImportResult{}, false, errors.New("import key reused with different payload")
	}
	var result poc02ImportResult
	if err := json.Unmarshal(encoded, &result); err != nil {
		return poc02ImportResult{}, false, err
	}
	return result, true, nil
}

func poc02Rows(count int, prefix string) []poc02ImportRow {
	rows := make([]poc02ImportRow, count)
	for index := range rows {
		rows[index] = poc02ImportRow{
			ID:           fmt.Sprintf("%s-%06d", prefix, index),
			PartNumber:   fmt.Sprintf("%s-PN-%06d", strings.ToUpper(prefix), index),
			Name:         fmt.Sprintf("Synthetic product %d", index),
			Manufacturer: fmt.Sprintf("Maker-%02d", index%23),
			Category:     fmt.Sprintf("Category-%02d", index%31),
		}
	}
	return rows
}

func TestPOC02AtomicImportStaircase(t *testing.T) {
	for _, count := range []int{1_000, 10_000, 100_000} {
		t.Run(fmt.Sprintf("rows_%d", count), func(t *testing.T) {
			harness := newPOC02Harness(t)
			var before runtime.MemStats
			runtime.ReadMemStats(&before)
			cpuBefore := processCPU()
			readBefore, writeBefore := processIO()
			started := time.Now()
			result, err := harness.importRows(context.Background(), fmt.Sprintf("scale-%d", count), "preview-v1", poc02Rows(count, fmt.Sprintf("scale%d", count)), poc02NoFault)
			if err != nil {
				t.Fatal(err)
			}
			if result.Inserted != count || len(result.Errors) != 0 {
				t.Fatalf("result=%+v", result)
			}
			var stored int
			if err := harness.store.db.QueryRow(`SELECT COUNT(*) FROM products WHERE id LIKE ?`, fmt.Sprintf("scale%d-%%", count)).Scan(&stored); err != nil {
				t.Fatal(err)
			}
			if stored != count {
				t.Fatalf("stored=%d want=%d", stored, count)
			}
			var after runtime.MemStats
			runtime.ReadMemStats(&after)
			cpuAfter := processCPU()
			readAfter, writeAfter := processIO()
			dbSize := fileSize(harness.path)
			walSize := fileSize(harness.path + "-wal")
			query := fmt.Sprintf("%s-PN-%06d", strings.ToUpper(fmt.Sprintf("scale%d", count)), count-1)
			searchStarted := time.Now()
			_, total, err := harness.store.ListPublished(context.Background(), query, 1, 10)
			if err != nil || total != 1 {
				t.Fatalf("cold search total=%d err=%v", total, err)
			}
			connectionCold := time.Since(searchStarted)
			searchStarted = time.Now()
			for iteration := 0; iteration < 20; iteration++ {
				if _, _, err := harness.store.ListPublished(context.Background(), query, 1, 10); err != nil {
					t.Fatal(err)
				}
			}
			warmAverage := time.Since(searchStarted) / 20
			t.Logf("rows=%d total=%s parse=%s validate=%s writer_wait=%s commit=%s search_connection_cold=%s search_warm_avg=%s db_bytes=%d wal_bytes=%d heap_delta=%d rss_bytes=%d cpu=%s proc_read_bytes=%d proc_write_bytes=%d", count, time.Since(started), result.Parse, result.Validate, result.WriterWait, result.Commit, connectionCold, warmAverage, dbSize, walSize, int64(after.HeapAlloc)-int64(before.HeapAlloc), residentBytes(), cpuAfter-cpuBefore, readAfter-readBefore, writeAfter-writeBefore)
		})
	}
}

func TestPOC02ValidationAtomicityRevisionAndReceipts(t *testing.T) {
	harness := newPOC02Harness(t)
	ctx := context.Background()
	invalid := []poc02ImportRow{
		{ID: "duplicate", PartNumber: "A", Manufacturer: "M", Category: "New-Dictionary"},
		{ID: "duplicate", PartNumber: "B", Manufacturer: "M", Category: "New-Dictionary"},
		{ID: "missing-part", Manufacturer: "M", Category: "New-Dictionary"},
		{ID: "missing-category", PartNumber: "C", Manufacturer: "M"},
	}
	result, err := harness.importRows(ctx, "invalid", "preview-v1", invalid, poc02NoFault)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Errors) != 4 {
		t.Fatalf("reported errors=%v", result.Errors)
	}
	var invalidStored int
	if err := harness.store.db.QueryRow(`SELECT COUNT(*) FROM products WHERE id IN ('duplicate','missing-part','missing-category')`).Scan(&invalidStored); err != nil {
		t.Fatal(err)
	}
	if invalidStored != 0 {
		t.Fatal("invalid batch partially committed")
	}

	stale, err := harness.importRows(ctx, "stale-preview", "preview-v0", poc02Rows(10, "stale"), poc02NoFault)
	if err != nil || len(stale.Errors) != 1 || stale.Errors[0].Code != "stale_preview" {
		t.Fatalf("stale preview result=%+v err=%v", stale, err)
	}

	base := poc02Rows(1, "update")
	first, err := harness.importRows(ctx, "update-first", "preview-v1", base, poc02NoFault)
	if err != nil || first.Inserted != 1 {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	changed := base
	changed[0].Name = "Changed name"
	changed[0].Manufacturer = "Changed maker"
	changed[0].Category = "Changed category"
	changed[0].ExpectedRevision = 1
	second, err := harness.importRows(ctx, "update-second", "preview-v1", changed, poc02NoFault)
	if err != nil || second.Updated != 1 {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	var name, maker, category string
	var revision int64
	if err := harness.store.db.QueryRow(`SELECT p.name,p.manufacturer,p.revision,c.category_name FROM products p JOIN poc02_product_categories c ON c.product_id=p.id WHERE p.id=?`, base[0].ID).Scan(&name, &maker, &revision, &category); err != nil {
		t.Fatal(err)
	}
	if name != "Changed name" || maker != "Changed maker" || category != "Changed category" || revision != 2 {
		t.Fatalf("update state=%q %q %q r%d", name, maker, category, revision)
	}

	if _, err := harness.importRows(ctx, "before-commit", "preview-v1", poc02Rows(5, "before"), poc02FaultBeforeCommit); err == nil {
		t.Fatal("fault before commit unexpectedly succeeded")
	}
	var beforeRows, beforeReceipts int
	_ = harness.store.db.QueryRow(`SELECT COUNT(*) FROM products WHERE id LIKE 'before-%'`).Scan(&beforeRows)
	_ = harness.store.db.QueryRow(`SELECT COUNT(*) FROM poc02_import_receipts WHERE import_key='before-commit'`).Scan(&beforeReceipts)
	if beforeRows != 0 || beforeReceipts != 0 {
		t.Fatalf("before-commit fault leaked rows=%d receipts=%d", beforeRows, beforeReceipts)
	}

	lostRows := poc02Rows(7, "lost")
	if _, err := harness.importRows(ctx, "lost-response", "preview-v1", lostRows, poc02FaultAfterCommit); !errors.Is(err, errPOC02ResponseLost) {
		t.Fatalf("after-commit fault=%v", err)
	}
	replayed, err := harness.importRows(ctx, "lost-response", "preview-v1", lostRows, poc02NoFault)
	if err != nil || !replayed.Replay || replayed.Inserted != len(lostRows) {
		t.Fatalf("replay=%+v err=%v", replayed, err)
	}
	var lostStored int
	_ = harness.store.db.QueryRow(`SELECT COUNT(*) FROM products WHERE id LIKE 'lost-%'`).Scan(&lostStored)
	if lostStored != len(lostRows) {
		t.Fatalf("lost response duplicated rows=%d", lostStored)
	}
}

func TestPOC02ConcurrentImportRFQReadsAndAdminWrite(t *testing.T) {
	harness := newPOC02Harness(t)
	publicArtifact := filepath.Join(filepath.Dir(harness.path), "public-artifact.bin")
	if err := os.WriteFile(publicArtifact, make([]byte, 4<<20), 0o600); err != nil {
		t.Fatal(err)
	}
	admitted := make(chan struct{})
	release := make(chan struct{})
	harness.onAdmitted = func() { close(admitted) }
	harness.holdAdmitted = release
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	importDone := make(chan error, 1)
	go func() {
		_, err := harness.importRows(ctx, "contended-import", "preview-v1", poc02Rows(20_000, "contended"), poc02NoFault)
		importDone <- err
	}()
	<-admitted

	const rfqCount = 24
	latencies := make([]time.Duration, rfqCount)
	var wg sync.WaitGroup
	for index := 0; index < rfqCount; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			started := time.Now()
			_, err := harness.withWriter(ctx, func() error {
				key, err := NewKey()
				if err != nil {
					return err
				}
				_, err = harness.store.SubmitRFQ(ctx, key, inquiries.Submission{Name: fmt.Sprintf("Buyer %d", index), Email: fmt.Sprintf("buyer-%d@example.test", index), Items: []inquiries.Item{{Kind: "requested", Requested: fmt.Sprintf("REQUEST-%d", index)}}})
				return err
			})
			if err != nil {
				t.Errorf("RFQ %d: %v", index, err)
			}
			latencies[index] = time.Since(started)
		}(index)
	}

	adminDone := make(chan error, 1)
	go func() {
		_, err := harness.withWriter(ctx, func() error {
			return harness.store.InsertProduct(ctx, catalog.Product{ID: "admin-during-import", PartNumber: "ADMIN-WRITE", Manufacturer: "Example", Status: catalog.Published})
		})
		adminDone <- err
	}()

	readErrors := make(chan error, 32)
	var readWG sync.WaitGroup
	for index := 0; index < 32; index++ {
		readWG.Add(1)
		go func() {
			defer readWG.Done()
			if _, err := os.ReadFile(publicArtifact); err != nil {
				readErrors <- err
				return
			}
			_, _, err := harness.store.ListPublished(ctx, "SYNTH", 1, 10)
			if err != nil {
				readErrors <- err
			}
		}()
	}
	readWG.Wait()
	close(readErrors)
	for err := range readErrors {
		t.Errorf("public read: %v", err)
	}
	close(release)
	if err := <-importDone; err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if err := <-adminDone; err != nil {
		t.Fatal(err)
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p50 := latencies[len(latencies)*50/100]
	p95 := latencies[len(latencies)*95/100]
	p99 := latencies[len(latencies)*99/100]
	var rfqs int
	if err := harness.store.db.QueryRow(`SELECT COUNT(*) FROM rfqs WHERE name LIKE 'Buyer %'`).Scan(&rfqs); err != nil {
		t.Fatal(err)
	}
	if rfqs != rfqCount {
		t.Fatalf("RFQs=%d want=%d", rfqs, rfqCount)
	}
	t.Logf("contended_rows=20000 rfq_count=%d p50=%s p95=%s p99=%s max=%s", rfqCount, p50, p95, p99, latencies[len(latencies)-1])
}

func TestPOC02WriterAdmissionTimeoutIsBounded(t *testing.T) {
	harness := newPOC02Harness(t)
	<-harness.gate
	defer func() { harness.gate <- struct{}{} }()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	wait, err := harness.withWriter(ctx, func() error { return nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("writer wait error=%v", err)
	}
	if wait < 15*time.Millisecond || wait > 250*time.Millisecond {
		t.Fatalf("writer wait=%s outside bounded timeout evidence", wait)
	}
	t.Logf("writer_timeout=%s result=%v", wait, err)
}

func TestPOC02D6VectorsAndProjectionRebuild(t *testing.T) {
	harness := newPOC02Harness(t)
	ctx := context.Background()
	products := []catalog.Product{
		{ID: "identity-upper", PartNumber: "ABC123", Manufacturer: "Maker", Status: catalog.Published},
		{ID: "identity-lower", PartNumber: "abc123", Manufacturer: "Maker", Status: catalog.Published},
		{ID: "accent", PartNumber: "ÉCHO", Manufacturer: "Maker", Status: catalog.Published},
		{ID: "plain", PartNumber: "ECHO", Manufacturer: "Maker", Status: catalog.Published},
		{ID: "wildcards", PartNumber: `100%_REAL\PART`, Manufacturer: "Maker", Status: catalog.Published},
	}
	for _, product := range products {
		if err := harness.store.InsertProduct(ctx, product); err != nil {
			t.Fatal(err)
		}
	}
	for query, want := range map[string]int{"abc123": 2, "écho": 1, "echo": 1, "%": 1, "_": 1, `\`: 1} {
		_, total, err := harness.store.ListPublished(ctx, query, 1, 20)
		if err != nil {
			t.Fatal(err)
		}
		if total != want {
			t.Errorf("query %q total=%d want=%d", query, total, want)
		}
	}
	var before []string
	rows, err := harness.store.db.Query(`SELECT part_number FROM products WHERE id LIKE 'identity-%' ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var value string
		_ = rows.Scan(&value)
		before = append(before, value)
	}
	rows.Close()
	if _, err := harness.store.db.Exec(`UPDATE products SET search_folded='stale',search_projection_version='v0' WHERE id LIKE 'identity-%'`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"identity-upper", "identity-lower"} {
		product, err := scanProduct(harness.store.db.QueryRow(`SELECT `+productColumns+` FROM products WHERE id=?`, id))
		if err != nil {
			t.Fatal(err)
		}
		if err := product.Prepare(); err != nil {
			t.Fatal(err)
		}
		if _, err := harness.store.db.Exec(`UPDATE products SET search_folded=?,search_projection_version=? WHERE id=?`, product.SearchFolded, product.ProjectionVer, id); err != nil {
			t.Fatal(err)
		}
	}
	var after []string
	rows, _ = harness.store.db.Query(`SELECT part_number FROM products WHERE id LIKE 'identity-%' ORDER BY id`)
	for rows.Next() {
		var value string
		_ = rows.Scan(&value)
		after = append(after, value)
	}
	rows.Close()
	if fmt.Sprint(before) != fmt.Sprint(after) {
		t.Fatalf("projection rebuild changed source: before=%v after=%v", before, after)
	}
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func residentBytes() int64 {
	raw, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(raw))
	if len(fields) < 2 {
		return 0
	}
	pages, _ := strconv.ParseInt(fields[1], 10, 64)
	return pages * int64(os.Getpagesize())
}

func processIO() (int64, int64) {
	raw, err := os.ReadFile("/proc/self/io")
	if err != nil {
		return 0, 0
	}
	var readBytes, writeBytes int64
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		value, _ := strconv.ParseInt(fields[1], 10, 64)
		switch fields[0] {
		case "read_bytes:":
			readBytes = value
		case "write_bytes:":
			writeBytes = value
		}
	}
	return readBytes, writeBytes
}

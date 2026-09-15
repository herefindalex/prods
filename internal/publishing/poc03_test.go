//go:build poc

package publishing

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	_ "modernc.org/sqlite"
)

type poc03Fault string

const (
	poc03NoFault           poc03Fault = ""
	poc03StagingIncomplete poc03Fault = "staging-incomplete"
	poc03AfterPrepared     poc03Fault = "after-prepared"
	poc03DuringCommit      poc03Fault = "during-commit"
	poc03AfterDBCommit     poc03Fault = "after-db-commit"
	poc03AfterIndex        poc03Fault = "after-index-before-response"
)

var errPOC03Crash = errors.New("simulated process crash")

type poc03Product struct {
	ID         string
	PartNumber string
	Revision   int64
	Route      string
	Asset      string
	Dir        string
	ETag       string
}

type poc03Index struct {
	Epoch    int64
	Products map[string]poc03Product
	Routes   map[string]string
	Assets   map[string]int
}

type poc03Lease struct {
	Product poc03Product
	HTML    []byte
}

type poc03Protocol struct {
	db        *sql.DB
	dbPath    string
	root      string
	gate      sync.RWMutex
	index     *poc03Index
	aggregate map[string][]string
}

func newPOC03Protocol(t *testing.T) *poc03Protocol {
	t.Helper()
	root := t.TempDir()
	return openPOC03Protocol(t, filepath.Join(root, "publication.db"), filepath.Join(root, "public"))
}

func openPOC03Protocol(t *testing.T, dbPath, root string) *poc03Protocol {
	t.Helper()
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	for _, pragma := range []string{"PRAGMA journal_mode=WAL", "PRAGMA synchronous=FULL", "PRAGMA foreign_keys=ON"} {
		if _, err := db.Exec(pragma); err != nil {
			t.Fatal(err)
		}
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS poc03_site (singleton INTEGER PRIMARY KEY CHECK(singleton=1), epoch INTEGER NOT NULL, prefix TEXT NOT NULL);
		INSERT INTO poc03_site(singleton,epoch,prefix) VALUES(1,1,'/products') ON CONFLICT(singleton) DO NOTHING;
		CREATE TABLE IF NOT EXISTS poc03_products (
			id TEXT PRIMARY KEY, part_number TEXT NOT NULL, revision INTEGER NOT NULL,
			visibility TEXT NOT NULL CHECK(visibility IN ('published','hidden','archived')),
			route TEXT NOT NULL, asset TEXT NOT NULL, artifact_dir TEXT NOT NULL, manifest_hash TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS poc03_ops (
			op_id TEXT PRIMARY KEY, product_id TEXT NOT NULL, revision INTEGER NOT NULL,
			state TEXT NOT NULL, manifest_hash TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS poc03_dirty (kind TEXT NOT NULL, product_id TEXT NOT NULL, PRIMARY KEY(kind,product_id));`)
	if err != nil {
		t.Fatal(err)
	}
	p := &poc03Protocol{db: db, dbPath: dbPath, root: root, aggregate: map[string][]string{"search": {}, "category": {}, "manufacturer": {}, "brand": {}, "application": {}, "sitemap": {}, "manifest": {}}}
	if err := p.reconcile(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return p
}

func (p *poc03Protocol) close() error { return p.db.Close() }

func (p *poc03Protocol) renderUnit(id, part, route string, revision int64, asset string) map[string][]byte {
	view := PublicView{ID: id, Revision: revision, PartNumber: part, Name: "POC03 " + part, Manufacturer: "Example", CanonicalURL: "https://catalog.example.test" + route, Language: "en", RFQURL: "/rfq?product_id=" + id}
	if asset != "" {
		view.Documents = []Document{{Label: "Datasheet", URL: "/assets/" + asset}}
	}
	jsonBody, _ := JSON(view)
	jsonLD, _ := JSONLD(view)
	html := fmt.Sprintf("<!doctype html><article data-id=%q data-revision=%q><h1>%s</h1><a href=%q>RFQ</a></article><script type=\"application/ld+json\">%s</script>", id, fmt.Sprint(revision), part, view.RFQURL, jsonLD)
	return map[string][]byte{"index.html": []byte(html), "product.json": jsonBody, "product.jsonld": jsonLD, "product.md": Markdown(view), "route": []byte(route)}
}

func (p *poc03Protocol) stageUnit(opID, id, part, route string, revision int64, asset string, incomplete bool) (string, string, error) {
	stage := filepath.Join(p.root, "staging", opID)
	if err := os.MkdirAll(stage, 0o700); err != nil {
		return "", "", err
	}
	files := p.renderUnit(id, part, route, revision, asset)
	if incomplete {
		delete(files, "product.md")
	}
	hash := sha256.New()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		body := files[name]
		if err := os.WriteFile(filepath.Join(stage, name), body, 0o600); err != nil {
			return "", "", err
		}
		hash.Write([]byte(name))
		hash.Write(body)
	}
	manifestHash := hex.EncodeToString(hash.Sum(nil))
	if incomplete {
		return stage, manifestHash, errors.New("incomplete staged publication unit")
	}
	if err := os.WriteFile(filepath.Join(stage, "prepared"), []byte(manifestHash), 0o600); err != nil {
		return "", "", err
	}
	return stage, manifestHash, nil
}

func (p *poc03Protocol) publish(id, part, route, asset string, fault poc03Fault) error {
	p.gate.Lock()
	defer p.gate.Unlock()
	var current int64
	err := p.db.QueryRow(`SELECT revision FROM poc03_products WHERE id=?`, id).Scan(&current)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	revision := current + 1
	if route == "" {
		var prefix string
		if err := p.db.QueryRow(`SELECT prefix FROM poc03_site WHERE singleton=1`).Scan(&prefix); err != nil {
			return err
		}
		route = strings.TrimRight(prefix, "/") + "/" + id
	}
	opID := fmt.Sprintf("%s-r%d-%s", id, revision, fault)
	stage, manifestHash, err := p.stageUnit(opID, id, part, route, revision, asset, fault == poc03StagingIncomplete)
	if err != nil {
		return err
	}
	if _, err := p.db.Exec(`INSERT OR REPLACE INTO poc03_ops(op_id,product_id,revision,state,manifest_hash) VALUES(?,?,?,'prepared',?)`, opID, id, revision, manifestHash); err != nil {
		return err
	}
	if fault == poc03AfterPrepared {
		return errPOC03Crash
	}
	finalDir := filepath.Join(p.root, "revisions", id, fmt.Sprintf("r%d-%s", revision, manifestHash[:12]))
	if err := os.MkdirAll(filepath.Dir(finalDir), 0o700); err != nil {
		return err
	}
	if err := os.Rename(stage, finalDir); err != nil {
		return err
	}
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var routeOwner string
	err = tx.QueryRow(`SELECT id FROM poc03_products WHERE route=? AND visibility='published' AND id<>?`, route, id).Scan(&routeOwner)
	if err == nil {
		return fmt.Errorf("route already active for %s", routeOwner)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = tx.Exec(`INSERT INTO poc03_products(id,part_number,revision,visibility,route,asset,artifact_dir,manifest_hash)
		VALUES(?,?,?,'published',?,?,?,?) ON CONFLICT(id) DO UPDATE SET part_number=excluded.part_number,revision=excluded.revision,
		visibility='published',route=excluded.route,asset=excluded.asset,artifact_dir=excluded.artifact_dir,manifest_hash=excluded.manifest_hash`, id, part, revision, route, asset, finalDir, manifestHash)
	if err != nil {
		return err
	}
	for _, kind := range []string{"search", "category", "manufacturer", "sitemap", "manifest"} {
		if _, err := tx.Exec(`INSERT INTO poc03_dirty(kind,product_id) VALUES(?,?) ON CONFLICT DO NOTHING`, kind, id); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE poc03_ops SET state='activated' WHERE op_id=?`, opID); err != nil {
		return err
	}
	if fault == poc03DuringCommit {
		return errPOC03Crash
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if fault == poc03AfterDBCommit {
		return errPOC03Crash
	}
	if err := p.installIndexLocked(); err != nil {
		return err
	}
	if fault == poc03AfterIndex {
		return errPOC03Crash
	}
	return nil
}

func (p *poc03Protocol) hide(id string, archived bool) error {
	p.gate.Lock()
	defer p.gate.Unlock()
	visibility := "hidden"
	if archived {
		visibility = "archived"
	}
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE poc03_products SET visibility=? WHERE id=?`, visibility, id)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return sql.ErrNoRows
	}
	for _, kind := range []string{"search", "category", "manufacturer", "sitemap", "manifest"} {
		if _, err := tx.Exec(`INSERT INTO poc03_dirty(kind,product_id) VALUES(?,?) ON CONFLICT DO NOTHING`, kind, id); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return p.installIndexLocked()
}

func (p *poc03Protocol) reconcile() error {
	p.gate.Lock()
	defer p.gate.Unlock()
	return p.installIndexLocked()
}

func (p *poc03Protocol) installIndexLocked() error {
	var epoch int64
	if err := p.db.QueryRow(`SELECT epoch FROM poc03_site WHERE singleton=1`).Scan(&epoch); err != nil {
		return err
	}
	index := &poc03Index{Epoch: epoch, Products: map[string]poc03Product{}, Routes: map[string]string{}, Assets: map[string]int{}}
	rows, err := p.db.Query(`SELECT id,part_number,revision,route,asset,artifact_dir,manifest_hash FROM poc03_products WHERE visibility='published'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var product poc03Product
		var manifestHash string
		if err := rows.Scan(&product.ID, &product.PartNumber, &product.Revision, &product.Route, &product.Asset, &product.Dir, &manifestHash); err != nil {
			return err
		}
		if !unitComplete(product.Dir, manifestHash) {
			continue
		}
		product.ETag = fmt.Sprintf(`"%s-r%d"`, product.ID, product.Revision)
		index.Products[product.ID] = product
		index.Routes[product.Route] = product.ID
		if product.Asset != "" {
			index.Assets[product.Asset]++
		}
	}
	p.index = index
	return rows.Err()
}

func unitComplete(dir, manifestHash string) bool {
	prepared, err := os.ReadFile(filepath.Join(dir, "prepared"))
	if err != nil || string(prepared) != manifestHash {
		return false
	}
	for _, name := range []string{"index.html", "product.json", "product.jsonld", "product.md", "route"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			return false
		}
	}
	return true
}

func (p *poc03Protocol) admit(id string) (poc03Lease, bool) {
	p.gate.RLock()
	defer p.gate.RUnlock()
	product, ok := p.index.Products[id]
	if !ok {
		return poc03Lease{}, false
	}
	body, err := os.ReadFile(filepath.Join(product.Dir, "index.html"))
	if err != nil {
		return poc03Lease{}, false
	}
	return poc03Lease{Product: product, HTML: body}, true
}

func (p *poc03Protocol) serve(id, ifNoneMatch, byteRange string) (int, []byte) {
	return p.serveConditional(id, ifNoneMatch, byteRange, "")
}

func (p *poc03Protocol) serveConditional(id, ifNoneMatch, byteRange, ifRange string) (int, []byte) {
	lease, ok := p.admit(id)
	if !ok {
		return 404, nil
	}
	if ifNoneMatch == lease.Product.ETag {
		return 304, nil
	}
	if byteRange != "" {
		if ifRange != "" && ifRange != lease.Product.ETag {
			return 200, lease.HTML
		}
		if byteRange != "bytes=0-9" || len(lease.HTML) < 10 {
			return 416, nil
		}
		return 206, lease.HTML[:10]
	}
	return 200, lease.HTML
}

func (p *poc03Protocol) assetAllowed(asset string) bool {
	p.gate.RLock()
	defer p.gate.RUnlock()
	return p.index.Assets[asset] > 0
}

func (p *poc03Protocol) safeAggregate(kind string) []string {
	p.gate.RLock()
	defer p.gate.RUnlock()
	var visible []string
	for _, id := range p.aggregate[kind] {
		if _, ok := p.index.Products[id]; ok {
			visible = append(visible, id)
		}
	}
	return visible
}

func (p *poc03Protocol) convergeAggregates() error {
	p.gate.Lock()
	defer p.gate.Unlock()
	ids := make([]string, 0, len(p.index.Products))
	for id := range p.index.Products {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for kind := range p.aggregate {
		p.aggregate[kind] = append([]string(nil), ids...)
	}
	_, err := p.db.Exec(`DELETE FROM poc03_dirty`)
	return err
}

func TestPOC03PublicationUnitAndCrashReconciliation(t *testing.T) {
	for _, fault := range []poc03Fault{poc03StagingIncomplete, poc03AfterPrepared, poc03DuringCommit} {
		t.Run(string(fault), func(t *testing.T) {
			protocol := newPOC03Protocol(t)
			if err := protocol.publish("first", "FIRST-A", "", "asset-a", fault); err == nil {
				t.Fatal("fault unexpectedly succeeded")
			}
			if status, _ := protocol.serve("first", "", ""); status != 404 {
				t.Fatalf("partial first publish visible: %d", status)
			}
		})
	}

	protocol := newPOC03Protocol(t)
	if err := protocol.publish("product", "REV-A", "", "asset-a", poc03NoFault); err != nil {
		t.Fatal(err)
	}
	status, body := protocol.serve("product", "", "")
	if status != 200 || !strings.Contains(string(body), "REV-A") {
		t.Fatalf("revision A status=%d body=%s", status, body)
	}
	if err := protocol.publish("product", "REV-B-ROLLED-BACK", "", "asset-a", poc03DuringCommit); err == nil {
		t.Fatal("update fault unexpectedly succeeded")
	}
	status, body = protocol.serve("product", "", "")
	if status != 200 || !strings.Contains(string(body), "REV-A") || strings.Contains(string(body), "ROLLED-BACK") {
		t.Fatalf("failed update did not preserve A: %d %s", status, body)
	}

	if err := protocol.publish("product", "REV-B", "", "asset-a", poc03AfterDBCommit); !errors.Is(err, errPOC03Crash) {
		t.Fatalf("after DB fault=%v", err)
	}
	dbPath, root := protocol.dbPath, protocol.root
	if err := protocol.close(); err != nil {
		t.Fatal(err)
	}
	restarted := openPOC03Protocol(t, dbPath, root)
	status, body = restarted.serve("product", "", "")
	if status != 200 || !strings.Contains(string(body), "REV-B") || strings.Contains(string(body), "REV-A</h1>") {
		t.Fatalf("restart did not reconcile B: %d %s", status, body)
	}

	if err := restarted.publish("product", "REV-C", "", "asset-a", poc03AfterIndex); !errors.Is(err, errPOC03Crash) {
		t.Fatalf("after index fault=%v", err)
	}
	status, body = restarted.serve("product", "", "")
	if status != 200 || !strings.Contains(string(body), "REV-C") {
		t.Fatalf("response-loss activation not durable: %d %s", status, body)
	}
}

func TestPOC03MissingArtifactFailsClosedOnRestart(t *testing.T) {
	protocol := newPOC03Protocol(t)
	if err := protocol.publish("missing", "COMPLETE", "", "", poc03NoFault); err != nil {
		t.Fatal(err)
	}
	product := protocol.index.Products["missing"]
	if err := os.Remove(filepath.Join(product.Dir, "product.md")); err != nil {
		t.Fatal(err)
	}
	if err := protocol.reconcile(); err != nil {
		t.Fatal(err)
	}
	if status, _ := protocol.serve("missing", "", ""); status != 404 {
		t.Fatalf("incomplete durable unit admitted: %d", status)
	}
}

func TestPOC03RevocationAdmissionValidatorsAggregatesAndAssets(t *testing.T) {
	protocol := newPOC03Protocol(t)
	protocol.aggregate["search"] = []string{"not-yet-active"}
	if ids := protocol.safeAggregate("search"); len(ids) != 0 {
		t.Fatalf("aggregate created dead link before first activation: %v", ids)
	}
	if err := protocol.publish("one", "ONE", "", "shared.pdf", poc03NoFault); err != nil {
		t.Fatal(err)
	}
	if err := protocol.publish("two", "TWO", "", "shared.pdf", poc03NoFault); err != nil {
		t.Fatal(err)
	}
	if err := protocol.convergeAggregates(); err != nil {
		t.Fatal(err)
	}
	lease, ok := protocol.admit("one")
	if !ok {
		t.Fatal("pre-revocation admission failed")
	}
	if status, _ := protocol.serveConditional("one", "", "bytes=0-9", lease.Product.ETag); status != 206 {
		t.Fatalf("pre-hide range=%d", status)
	}
	if err := protocol.hide("one", false); err != nil {
		t.Fatal(err)
	}
	if status, _ := protocol.serve("one", lease.Product.ETag, ""); status != 404 {
		t.Fatalf("stale ETag after hide=%d", status)
	}
	if status, _ := protocol.serveConditional("one", "", "bytes=0-9", lease.Product.ETag); status != 404 {
		t.Fatalf("range after hide=%d", status)
	}
	if !strings.Contains(string(lease.HTML), "ONE") {
		t.Fatal("previously admitted transfer lost its immutable representation")
	}
	for _, kind := range []string{"search", "category", "manufacturer", "brand", "application", "sitemap", "manifest"} {
		if ids := protocol.safeAggregate(kind); len(ids) != 1 || ids[0] != "two" {
			t.Fatalf("%s leaked stale product: %v", kind, ids)
		}
	}
	var dirtyBefore int
	if err := protocol.db.QueryRow(`SELECT COUNT(*) FROM poc03_dirty WHERE product_id='one'`).Scan(&dirtyBefore); err != nil || dirtyBefore != 5 {
		t.Fatalf("dirty dependencies=%d err=%v", dirtyBefore, err)
	}
	if err := protocol.convergeAggregates(); err != nil {
		t.Fatal(err)
	}
	var dirtyAfter int
	if err := protocol.db.QueryRow(`SELECT COUNT(*) FROM poc03_dirty`).Scan(&dirtyAfter); err != nil || dirtyAfter != 0 {
		t.Fatalf("dirty dependencies after convergence=%d err=%v", dirtyAfter, err)
	}
	if !protocol.assetAllowed("shared.pdf") {
		t.Fatal("shared asset denied while another public reference remained")
	}
	if err := protocol.hide("two", true); err != nil {
		t.Fatal(err)
	}
	if protocol.assetAllowed("shared.pdf") {
		t.Fatal("shared asset remained authorized after last reference revoked")
	}
}

func TestPOC03GlobalPrefixSwitchAndExplicitRouteReuse(t *testing.T) {
	protocol := newPOC03Protocol(t)
	if err := protocol.publish("old", "OLD", "/products/reusable", "", poc03NoFault); err != nil {
		t.Fatal(err)
	}
	if err := protocol.hide("old", false); err != nil {
		t.Fatal(err)
	}
	if err := protocol.publish("new", "NEW", "/products/reusable", "", poc03NoFault); err != nil {
		t.Fatal(err)
	}
	protocol.gate.RLock()
	owner := protocol.index.Routes["/products/reusable"]
	protocol.gate.RUnlock()
	if owner != "new" {
		t.Fatalf("explicit historical route reuse owner=%q", owner)
	}
	if err := protocol.switchPrefix("/catalog/items"); err != nil {
		t.Fatal(err)
	}
	protocol.gate.RLock()
	_, oldExists := protocol.index.Routes["/products/reusable"]
	newOwner := protocol.index.Routes["/catalog/items/new"]
	epoch := protocol.index.Epoch
	protocol.gate.RUnlock()
	if oldExists || newOwner != "new" || epoch != 2 {
		t.Fatalf("site switch old=%v new=%q epoch=%d", oldExists, newOwner, epoch)
	}
}

func (p *poc03Protocol) switchPrefix(prefix string) error {
	p.gate.Lock()
	defer p.gate.Unlock()
	type replacement struct {
		id, route, dir, hash string
		revision             int64
	}
	var replacements []replacement
	rows, err := p.db.Query(`SELECT id,part_number,revision,asset FROM poc03_products WHERE visibility='published' ORDER BY id`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, part, asset string
		var revision int64
		if err := rows.Scan(&id, &part, &revision, &asset); err != nil {
			rows.Close()
			return err
		}
		revision++
		route := strings.TrimRight(prefix, "/") + "/" + id
		opID := fmt.Sprintf("site-%d-%s", revision, id)
		stage, hash, err := p.stageUnit(opID, id, part, route, revision, asset, false)
		if err != nil {
			rows.Close()
			return err
		}
		finalDir := filepath.Join(p.root, "revisions", id, fmt.Sprintf("r%d-%s", revision, hash[:12]))
		if err := os.MkdirAll(filepath.Dir(finalDir), 0o700); err != nil {
			rows.Close()
			return err
		}
		if err := os.Rename(stage, finalDir); err != nil {
			rows.Close()
			return err
		}
		replacements = append(replacements, replacement{id: id, route: route, dir: finalDir, hash: hash, revision: revision})
	}
	rows.Close()
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE poc03_site SET epoch=epoch+1,prefix=? WHERE singleton=1`, prefix); err != nil {
		return err
	}
	for _, item := range replacements {
		if _, err := tx.Exec(`UPDATE poc03_products SET revision=?,route=?,artifact_dir=?,manifest_hash=? WHERE id=?`, item.revision, item.route, item.dir, item.hash, item.id); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return p.installIndexLocked()
}

var _ = context.Background

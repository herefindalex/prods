package webapp

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"prods/internal/catalog"
	"prods/internal/publishing"
	"prods/internal/specnumeric"
	"prods/internal/storage/sqlite"
)

// This wrapper gates only real background work, not startup reconciliation or
// provider IO. Cancellation releases the gate so Engine.Close cannot deadlock.
type specWorkerBenchmarkRepository struct {
	*sqlite.Store
	entered, release                                   chan struct{}
	once                                               sync.Once
	claims, activations, profileCalls, profileProgress atomic.Int64
}

func (r *specWorkerBenchmarkRepository) wait(ctx context.Context) error {
	r.once.Do(func() { close(r.entered) })
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.release:
		return nil
	}
}

func (r *specWorkerBenchmarkRepository) ClaimPublicationIntent(ctx context.Context) (publishing.Intent, bool, error) {
	if err := r.wait(ctx); err != nil {
		return publishing.Intent{}, false, err
	}
	i, found, err := r.Store.ClaimPublicationIntent(ctx)
	if found && err == nil {
		r.claims.Add(1)
	}
	return i, found, err
}

func (r *specWorkerBenchmarkRepository) ProcessSpecNumericProfileWork(ctx context.Context, limit int) (bool, error) {
	if err := r.wait(ctx); err != nil {
		return false, err
	}
	r.profileCalls.Add(1)
	worked, err := r.Store.ProcessSpecNumericProfileWork(ctx, limit)
	if worked && err == nil {
		r.profileProgress.Add(1)
	}
	return worked, err
}

func (r *specWorkerBenchmarkRepository) ActivatePublication(ctx context.Context, i publishing.Intent, a publishing.ActivePublication, c publishing.VisibilityCoordinator) (bool, error) {
	activated, err := r.Store.ActivatePublication(ctx, i, a, c)
	if activated && err == nil {
		r.activations.Add(1)
	}
	return activated, err
}

func specWorkerBenchmarkProducts(t testing.TB) int {
	t.Helper()
	if raw := os.Getenv("PRODS_BENCH_WORKER_PRODUCTS"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			t.Fatalf("PRODS_BENCH_WORKER_PRODUCTS must be positive: %q", raw)
		}
		return n
	}
	return 200
}

type specWorkerDurableCounts struct {
	Pending, Processing, Failed, Completed, Superseded int
	ProfilePending, ProfileCompleted, ProfileFailed    int
}

func specWorkerCounts(t testing.TB, db *sql.DB) specWorkerDurableCounts {
	t.Helper()
	var c specWorkerDurableCounts
	err := db.QueryRowContext(t.Context(), `SELECT
 (SELECT COUNT(*) FROM publication_intents WHERE status='pending'),
 (SELECT COUNT(*) FROM publication_intents WHERE status='processing'),
 (SELECT COUNT(*) FROM publication_intents WHERE status='failed'),
 (SELECT COUNT(*) FROM publication_intents WHERE status='completed'),
 (SELECT COUNT(*) FROM publication_intents WHERE status='superseded'),
 (SELECT COUNT(*) FROM spec_numeric_profile_work WHERE status='pending'),
 (SELECT COUNT(*) FROM spec_numeric_profile_work WHERE status='completed'),
 (SELECT COUNT(*) FROM spec_numeric_profile_work WHERE status NOT IN ('pending','completed'))`).Scan(
		&c.Pending, &c.Processing, &c.Failed, &c.Completed, &c.Superseded, &c.ProfilePending, &c.ProfileCompleted, &c.ProfileFailed)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func specWorkerReadDB(t testing.TB, root string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(root, "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	return db
}

// Used only on a closed, owned test fixture. The exact derived search root is
// excluded for BOTH providers; DB, history and all generated units are copied.
// This is not a production Backup/Restore implementation or claim.
func specWorkerCopy(t testing.TB, source, target string) {
	t.Helper()
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == filepath.Join("search", "specs") {
			return filepath.SkipDir
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		out := filepath.Join(target, rel)
		if entry.IsDir() {
			return os.MkdirAll(out, 0700)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unexpected fixture file %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(out, data, info.Mode().Perm())
	})
	if err != nil {
		t.Fatal(err)
	}
}

func specWorkerFixtureDigest(t testing.TB, root string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == filepath.Join("search", "specs") {
			return filepath.SkipDir
		}
		if e.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		fmt.Fprintf(h, "%s\x00%x\n", filepath.ToSlash(rel), sum)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

type specWorkerBaseline struct {
	root, digest, actor string
	products            int
	queries             []specBenchmarkQuery
	sources             map[string]publishing.Source
	active              []publishing.ActivePublication
	counts              specWorkerDurableCounts
	nominal             catalog.SpecDefinition
	profile             specnumeric.Profile
}

func newSpecWorkerBaseline(b *testing.B, fanout bool) specWorkerBaseline {
	b.Helper()
	c := newSpecBenchmarkCorpusWithCount(b, "bleve", specWorkerBenchmarkProducts(b), fanout)
	baseline := specWorkerBaseline{root: b.TempDir(), actor: c.actor, products: c.products, queries: c.queries, sources: map[string]publishing.Source{}}
	definitions, err := c.store.ListSpecDefinitions(b.Context())
	if err != nil {
		b.Fatal(err)
	}
	for _, d := range definitions {
		if d.ID == c.queries[1].request.Predicates[1].SpecID {
			baseline.nominal = d
		}
	}
	active, err := c.store.ActivePublications(b.Context())
	if err != nil {
		b.Fatal(err)
	}
	baseline.active = active
	// IDs come from actual DB rows; every source is retained for starting-state
	// equality and for checking exactly the one profile-work revision increment.
	db := specWorkerReadDB(b, c.root)
	rows, err := db.QueryContext(b.Context(), `SELECT id FROM products ORDER BY id`)
	if err != nil {
		b.Fatal(err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			b.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		b.Fatal(err)
	}
	rows.Close()
	for _, id := range ids {
		s, err := c.store.PublicationSource(b.Context(), id)
		if err != nil {
			b.Fatal(err)
		}
		if len(s.NumericFacts) != 2 || len(s.NumericDefinitions) != 2 || len(s.Specs) != 2 {
			b.Fatalf("baseline %s incomplete source", id)
		}
		baseline.sources[id] = s
		for _, definition := range s.NumericDefinitions {
			if definition.SpecID == baseline.nominal.ID {
				baseline.profile = definition.Profile
			}
		}
	}
	baseline.counts = specWorkerCounts(b, db)
	db.Close()
	if len(ids) != baseline.products || baseline.nominal.ID == "" || baseline.profile.DeclaredInputUnit != "" {
		b.Fatal("invalid worker baseline")
	}
	if fanout {
		if len(active) != baseline.products || baseline.counts.Pending != 0 || baseline.counts.ProfilePending != 0 {
			b.Fatal("fanout baseline not converged")
		}
	} else if len(active) != 1 || baseline.counts.Pending == 0 || baseline.counts.ProfilePending != 0 {
		b.Fatal("initial baseline must contain one old seed activation and a real backlog")
	}
	c.engine.Close()
	if err := c.store.Close(); err != nil {
		b.Fatal(err)
	}
	specWorkerCopy(b, c.root, baseline.root)
	baseline.digest = specWorkerFixtureDigest(b, baseline.root)
	b.Logf("closed baseline products=%d seed_active=%d counts=%+v fixture_sha256=%s; excluded=search/specs; setup uses paused fixture ProcessOne/ProcessBatch outside timer", baseline.products, len(active), baseline.counts, baseline.digest)
	return baseline
}

type specWorkerRun struct {
	store      *sqlite.Store
	engine     *publishing.Engine
	repository *specWorkerBenchmarkRepository
	db         *sql.DB
	root       string
}

func (f *specWorkerRun) close() { f.engine.Close(); f.db.Close(); f.store.Close() }

func (base specWorkerBaseline) restore(b *testing.B, backend string) *specWorkerRun {
	b.Helper()
	root := b.TempDir()
	specWorkerCopy(b, base.root, root)
	if specWorkerFixtureDigest(b, root) != base.digest {
		b.Fatal("restored fixture differs from closed baseline")
	}
	store, err := sqlite.OpenReady(filepath.Join(root, "prods.db"))
	if err != nil {
		b.Fatal(err)
	}
	r := &specWorkerBenchmarkRepository{Store: store, entered: make(chan struct{}), release: make(chan struct{})}
	engine, err := publishing.NewEngine(b.Context(), r, publishing.Config{Root: filepath.Join(root, "generated"), BaseURL: "http://catalog.example.test", SpecSearchBackend: backend})
	if err != nil {
		store.Close()
		b.Fatal(err)
	}
	f := &specWorkerRun{store: store, engine: engine, repository: r, db: specWorkerReadDB(b, root), root: root}
	b.Cleanup(f.close)
	select {
	case <-r.entered:
	case <-time.After(30 * time.Second):
		b.Fatal("background worker did not enter test gate")
	}
	assertSpecProviderReady(b, engine, backend)
	if got := specWorkerCounts(b, f.db); got != base.counts {
		b.Fatalf("startup changed durable work: %+v != %+v", got, base.counts)
	}
	active, err := store.ActivePublications(b.Context())
	if err != nil || !reflect.DeepEqual(active, base.active) {
		b.Fatalf("starting activations differ: %v", err)
	}
	for id, expected := range base.sources {
		got, err := store.PublicationSource(b.Context(), id)
		if err != nil || !reflect.DeepEqual(got, expected) {
			b.Fatalf("starting source %s differs: %v", id, err)
		}
	}
	if r.claims.Load() != 0 || r.profileCalls.Load() != 0 {
		b.Fatal("background work escaped gate")
	}
	return f
}

// One durable snapshot and selected-provider diagnostics every 2ms, bounded at
// five minutes. That SQL/diagnostic polling and up-to-2ms sleep are INCLUDED in
// the worker elapsed time. Ready binds diagnostics to the current active digest.
func (f *specWorkerRun) converge(b *testing.B, backend string) (specWorkerDurableCounts, int) {
	b.Helper()
	deadline := time.Now().Add(5 * time.Minute)
	for polls := 1; ; polls++ {
		counts := specWorkerCounts(b, f.db)
		if counts.Failed != 0 || counts.ProfileFailed != 0 {
			b.Fatalf("unexpected durable failure: %+v", counts)
		}
		d := f.engine.SpecSearchDiagnostics()
		if counts.Pending == 0 && counts.Processing == 0 && counts.ProfilePending == 0 && d.Backend == backend && d.Ready && d.LastError == "" && d.SourceDigest != "" {
			return counts, polls
		}
		if time.Now().After(deadline) {
			b.Fatalf("worker convergence timed out: %+v diagnostics=%+v", counts, d)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func specWorkerVerifyUnit(b *testing.B, root string, a publishing.ActivePublication) {
	b.Helper()
	unit := filepath.Join(root, "generated", "units", a.ArtifactID)
	manifest, err := os.ReadFile(filepath.Join(unit, "manifest.json"))
	if err != nil {
		b.Fatal(err)
	}
	sum := sha256.Sum256(manifest)
	if hex.EncodeToString(sum[:]) != a.ManifestHash {
		b.Fatal("unit manifest hash differs")
	}
	prepared, err := os.ReadFile(filepath.Join(unit, "prepared"))
	if err != nil || string(prepared) != a.ManifestHash {
		b.Fatal("unit not durably prepared")
	}
	var m struct {
		ProductID             string            `json:"product_id"`
		Revision              int64             `json:"revision"`
		SiteEpoch             int64             `json:"site_epoch"`
		RenderContractVersion string            `json:"render_contract_version"`
		Files                 map[string]string `json:"files"`
	}
	if err := json.Unmarshal(manifest, &m); err != nil {
		b.Fatal(err)
	}
	if m.ProductID != a.ProductID || m.Revision != a.PublicRevision || m.SiteEpoch != a.SiteEpoch || m.RenderContractVersion != "shared-specs-1" || len(m.Files) != 4+8*len(a.View.SupportedLocales) {
		b.Fatal("incomplete or mismatched core/locale generated unit")
	}
	for _, name := range []string{"index.html", "product.json", "product.jsonld", "product.md"} {
		if m.Files[name] == "" {
			b.Fatalf("core generated file missing: %s", name)
		}
	}
	for path, expected := range m.Files {
		body, err := os.ReadFile(filepath.Join(unit, path))
		if err != nil {
			b.Fatal(err)
		}
		sum := sha256.Sum256(body)
		if hex.EncodeToString(sum[:]) != expected {
			b.Fatalf("generated file hash differs: %s", path)
		}
	}
}

func (base specWorkerBaseline) verify(b *testing.B, f *specWorkerRun, backend string, changed *catalog.SpecNumericProfile) [][]specBenchmarkPage {
	b.Helper()
	active, err := f.store.ActivePublications(b.Context())
	if err != nil || len(active) != base.products {
		b.Fatalf("final active=%d error=%v", len(active), err)
	}
	inventory, err := f.store.SpecSearchCandidates(b.Context(), nil, publishing.SpecProjectionVersion)
	if err != nil || len(inventory.Inventory) != base.products {
		b.Fatalf("projection inventory=%d error=%v", len(inventory.Inventory), err)
	}
	bindings := map[string]publishing.SpecCandidateBinding{}
	for _, binding := range inventory.Inventory {
		if _, duplicate := bindings[binding.ProductID]; duplicate {
			b.Fatal("duplicate actual projection binding")
		}
		bindings[binding.ProductID] = binding
	}
	for _, a := range active {
		source, err := f.store.PublicationSource(b.Context(), a.ProductID)
		if err != nil {
			b.Fatal(err)
		}
		expected := base.sources[a.ProductID]
		increment := int64(0)
		facts := 2
		if changed != nil {
			increment, facts = 1, 1
		}
		if source.Product.Revision != expected.Product.Revision+increment || a.SourceRevision != source.Product.Revision || a.PublicRevision != source.Product.Revision || a.View.Revision != source.Product.Revision || a.VisibilityGeneration <= 0 || a.SiteEpoch != source.SiteEpoch {
			b.Fatalf("final product/source/public binding invalid: %s", a.ProductID)
		}
		if len(source.NumericFacts) != facts || !reflect.DeepEqual(a.View.NumericFacts, source.NumericFacts) || !reflect.DeepEqual(a.View.NumericDefinitions, source.NumericDefinitions) || len(source.NumericDefinitions) != 2 {
			b.Fatalf("final frozen numeric facts/definitions differ: %s", a.ProductID)
		}
		for _, d := range a.View.NumericDefinitions {
			if changed != nil && d.SpecID == base.nominal.ID {
				if d.SpecSemanticVersion != changed.SpecSemanticVersion || !reflect.DeepEqual(d.Profile, changed.Profile) {
					b.Fatal("nominal definition did not converge")
				}
			} else {
				found := false
				for _, old := range expected.NumericDefinitions {
					found = found || reflect.DeepEqual(old, d)
				}
				if !found {
					b.Fatal("unchanged numeric definition changed")
				}
			}
		}
		if changed != nil {
			for _, fact := range a.View.NumericFacts {
				if fact.SpecID == base.nominal.ID {
					b.Fatal("stale nominal fact leaked into active view")
				}
				found := false
				for _, old := range expected.NumericFacts {
					found = found || reflect.DeepEqual(old, fact)
				}
				if !found {
					b.Fatal("unchanged support fact changed")
				}
			}
		} else if !reflect.DeepEqual(expected.NumericFacts, source.NumericFacts) {
			b.Fatal("initial facts changed")
		}
		if len(a.View.Specifications) != len(source.Specs) || len(source.Specs) != 2 || !reflect.DeepEqual(source.Specs, expected.Specs) {
			b.Fatal("raw shared sources missing")
		}
		for _, s := range source.Specs {
			var valueID string
			if err := f.db.QueryRowContext(b.Context(), `SELECT v.id FROM product_spec_sources ps JOIN product_spec_values v
 ON v.id=ps.spec_value_id AND v.product_id=ps.product_id AND v.spec_id=ps.spec_id
 WHERE ps.product_id=? AND ps.spec_id=? AND v.active=1`, a.ProductID, s.ID).Scan(&valueID); err != nil {
				b.Fatal(err)
			}
			c, err := f.store.SpecNormalizationContext(b.Context(), valueID)
			if err != nil {
				b.Fatal(err)
			}
			if c.ProductID != a.ProductID || c.SpecID != s.ID || c.SourceSelection.SpecValueID != valueID || c.SourceSelection.Revision <= 0 || c.Expected.SourceRevision != s.SourceRevision || c.Raw != s.RawValue || c.SourceLocale != s.Language || c.Selection.SelectedNormalizedValueID == "" {
				b.Fatal("actual source/normalization selection binding differs")
			}
			var status string
			var rawRevision, semantic, profileRevision int64
			if err := f.db.QueryRowContext(b.Context(), `SELECT status,source_revision,spec_semantic_version,profile_revision FROM normalized_values WHERE id=? AND spec_value_id=?`, c.Selection.SelectedNormalizedValueID, valueID).Scan(&status, &rawRevision, &semantic, &profileRevision); err != nil {
				b.Fatal(err)
			}
			wantStatus, versionIncrement := "current", int64(0)
			if changed != nil && s.ID == base.nominal.ID {
				wantStatus, versionIncrement = "stale", 1
			}
			if status != wantStatus || rawRevision != c.Expected.SourceRevision || semantic+versionIncrement != c.Expected.SpecSemanticVersion || profileRevision+versionIncrement != c.Expected.ProfileRevision {
				b.Fatal("selected normalized fact status/version binding differs")
			}
			found := false
			for _, v := range a.View.Specifications {
				found = found || (v.ID == s.ID && v.SourceRevision == s.SourceRevision && v.RawValue == s.RawValue && v.Language == s.Language)
			}
			if !found {
				b.Fatal("shared source binding differs")
			}
		}
		want := publishing.SpecCandidateBinding{ProductID: a.ProductID, PublicRevision: a.PublicRevision, SourceRevision: a.SourceRevision, SiteEpoch: a.SiteEpoch, VisibilityGeneration: a.VisibilityGeneration, ProjectionVersion: publishing.SpecProjectionVersion, SpecIDs: publishing.SpecProjectionIDs(a.View)}
		if !reflect.DeepEqual(bindings[a.ProductID], want) {
			b.Fatal("actual SQLite projection fields/revision binding differs")
		}
		specWorkerVerifyUnit(b, f.root, a)
	}
	if changed != nil {
		var current, stale int
		if err := f.db.QueryRowContext(b.Context(), `SELECT SUM(n.status='current'),SUM(n.status='stale') FROM normalized_values n JOIN product_spec_values v ON v.id=n.spec_value_id WHERE v.spec_id=?`, base.nominal.ID).Scan(&current, &stale); err != nil {
			b.Fatal(err)
		}
		if current != 0 || stale != base.products {
			b.Fatalf("nominal history current=%d stale=%d", current, stale)
		}
	}
	assertSpecProviderReady(b, f.engine, backend)
	var pages [][]specBenchmarkPage
	for q, query := range base.queries {
		request := query.request
		request.Page = 1
		var results []specBenchmarkPage
		var wantIDs []string
		// Independent fixed workload oracle, expressed from the synthetic raw
		// generator rather than trusting either candidate provider's own Total.
		for i := 0; i < base.products; i++ {
			n := i % 100
			matches := q == 2 || q == 0 && (i%2 == 0 && n <= 42 && n+2 >= 42 || i%2 == 1 && (n == 42 || n+2 == 42)) || q == 1 && changed == nil && n <= 60 && n+2 >= 40 && i%25 >= 10 && i%25 <= 15
			if matches {
				part := fmt.Sprintf("BENCH-%06d", i)
				for id, source := range base.sources {
					if source.Product.PartNumber == part {
						wantIDs = append(wantIDs, id)
					}
				}
			}
		}
		var allIDs []string
		for {
			result := specBenchmarkSearch(b, f.engine, backend, request)
			page := specBenchmarkPage{Total: result.Total, Page: result.Page, PageSize: result.PageSize, IDs: []string{}}
			for _, row := range result.Rows {
				page.IDs = append(page.IDs, row.ID)
			}
			results = append(results, page)
			allIDs = append(allIDs, page.IDs...)
			if result.Total != len(wantIDs) {
				b.Fatalf("%s MATCH count=%d want=%d", query.name, result.Total, len(wantIDs))
			}
			if len(result.Rows) == 0 {
				break
			}
			request.Page++
		}
		if !slices.Equal(allIDs, wantIDs) {
			b.Fatalf("%s ordered MATCH IDs differ from workload oracle", query.name)
		}
		pages = append(pages, results)
	}
	return pages
}

func benchmarkSpecWorker(b *testing.B, fanout bool) {
	base := newSpecWorkerBaseline(b, fanout)
	var paired [][]specBenchmarkPage
	for _, backend := range []string{"bleve", "sqlite"} {
		b.Run(backend, func(b *testing.B) {
			b.StopTimer()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				f := base.restore(b, backend)
				var changed *catalog.SpecNumericProfile
				profile := base.profile
				profile.DeclaredInputUnit = "V"
				b.StartTimer()
				if fanout {
					configured, err := f.store.ConfigureSpecNumericProfile(b.Context(), base.actor, base.nominal.ID, base.nominal.Revision, profile)
					if err != nil {
						b.Fatal(err)
					}
					changed = &configured
				}
				close(f.repository.release)
				counts, polls := f.converge(b, backend)
				b.StopTimer()
				pages := base.verify(b, f, backend, changed)
				if paired == nil {
					paired = pages
				} else if !reflect.DeepEqual(paired, pages) {
					b.Fatal("paired worker ordered IDs/Total/pages differ")
				}
				if f.repository.activations.Load() != int64(base.products) {
					b.Fatal("worker did not activate every workload product")
				}
				if fanout && (f.repository.profileProgress.Load() != int64(base.products/100+1) || counts.ProfileCompleted != base.counts.ProfileCompleted+1) {
					b.Fatal("real bounded profile progress did not complete")
				}
				b.Logf("iteration=%d products=%d baseline_sha256=%s starting_active=%d starting_counts=%+v final_counts=%+v claims=%d activations=%d profile_calls=%d profile_progress=%d full_units=%d polls=%d poll_interval=2ms totals=%d/%d/%d candidate_queries=%d fallback=%d digest=%s", i, base.products, base.digest, len(base.active), base.counts, counts, f.repository.claims.Load(), f.repository.activations.Load(), f.repository.profileCalls.Load(), f.repository.profileProgress.Load(), len(base.sources), polls, pages[0][0].Total, pages[1][0].Total, pages[2][0].Total, f.engine.SpecSearchDiagnostics().CandidateQueries, f.engine.SpecSearchDiagnostics().FallbackQueries, f.engine.SpecSearchDiagnostics().SourceDigest)
				b.ReportMetric(float64(base.products), "products/op")
				b.ReportMetric(float64(f.repository.activations.Load()), "activations/op")
				b.ReportMetric(float64(polls), "polls/op")
				f.close()
			}
		})
	}
}

// Initial timer: entered gate + verified identical starting sources/activations
// and startup-ready provider -> release -> durable queues empty AND Ready.
func BenchmarkSpecWorkerInitialPublication(b *testing.B) { benchmarkSpecWorker(b, false) }

// Fanout timer additionally includes the real profile configuration command.
// No readoption, manual drain, ProcessBatch or forced rebuild occurs in timing.
func BenchmarkSpecWorkerProfileFanout(b *testing.B) { benchmarkSpecWorker(b, true) }

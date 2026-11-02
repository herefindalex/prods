package publishing

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"prods/internal/platform"
	"prods/internal/site"
	"prods/internal/specnumeric"
)

type specBackendRepository struct {
	Repository
	active     []ActivePublication
	queries    atomic.Int64
	startError error
}

func (r *specBackendRepository) ResetPublicationClaims(context.Context) error { return r.startError }
func (r *specBackendRepository) ClaimPublicationIntent(context.Context) (Intent, bool, error) {
	return Intent{}, false, nil
}
func (r *specBackendRepository) ActivePublications(context.Context) ([]ActivePublication, error) {
	return r.active, nil
}
func (r *specBackendRepository) ActiveWebsiteConfiguration(context.Context) (site.Configuration, int64, int64, error) {
	return site.Configuration{}, 1, 2, nil
}
func (r *specBackendRepository) WebsiteCustomCSSState(context.Context) (bool, int64, error) {
	return false, 1, nil
}
func (r *specBackendRepository) ActiveRouteHistory(context.Context) ([]RouteHistory, error) {
	return nil, nil
}
func (r *specBackendRepository) RebuildSearchPublications(context.Context, []ActivePublication) error {
	return nil
}
func (r *specBackendRepository) RebuildSpecSearchProjection(context.Context, []ActivePublication) error {
	return nil
}
func (r *specBackendRepository) ConvergePublicationDependencies(context.Context, string, int64) error {
	return nil
}
func (r *specBackendRepository) SpecSearchCandidates(context.Context, []string, string) (SpecCandidateSnapshot, error) {
	r.queries.Add(1)
	s := SpecCandidateSnapshot{ProjectionVersion: SpecProjectionVersion}
	for _, a := range r.active {
		b := SpecCandidateBinding{ProductID: a.ProductID, PublicRevision: a.PublicRevision, SourceRevision: a.SourceRevision, SiteEpoch: a.SiteEpoch, VisibilityGeneration: a.VisibilityGeneration, ProjectionVersion: SpecProjectionVersion, SpecIDs: SpecProjectionIDs(a.View)}
		s.Inventory = append(s.Inventory, b)
		s.Candidates = append(s.Candidates, b)
	}
	return s, nil
}

func specBackendFixture(t *testing.T, backend string) (*Engine, *specBackendRepository, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "generated")
	if err := os.MkdirAll(filepath.Join(root, "units"), 0o700); err != nil {
		t.Fatal(err)
	}
	config := Config{Root: root, BaseURL: "https://example.test", SpecSearchBackend: backend, ResourceGate: platform.NewResourceGate(nil)}
	stager := &Engine{root: root, baseURL: config.BaseURL, resources: config.ResourceGate}
	a := blevePublication(t)
	a.Route = "/products/opaque-product"
	a.View.PartNumber = "PART"
	a.View.Revision = a.PublicRevision
	a.View.SiteEpoch = a.SiteEpoch
	a.View.Language = "en-US"
	a.View.DefaultLocale = "en-US"
	a.View.SupportedLocales = []string{"en-US"}
	a.View.CanonicalURL = config.BaseURL + a.Route
	var err error
	a.ArtifactID, a.ManifestHash, err = stager.stage(t.Context(), "fixture", a.View)
	if err != nil {
		t.Fatal(err)
	}
	r := &specBackendRepository{active: []ActivePublication{a}}
	e, err := NewEngine(t.Context(), r, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Close)
	return e, r, filepath.Join(filepath.Dir(root), "search", "specs")
}

func specBackendRequest(t *testing.T) SpecSearchRequest {
	q := bleveDecimal(t, "3.2")
	return SpecSearchRequest{Predicates: []specnumeric.Predicate{{SpecID: "voltage/opaque", Operator: specnumeric.OperatorContainsPoint, Value: &q, Unit: "V"}}, Page: 1, PageSize: 10}
}

func TestSpecSearchDefaultBleveAndStaleFallback(t *testing.T) {
	e, r, _ := specBackendFixture(t, "")
	result, err := e.SearchSpecs(t.Context(), specBackendRequest(t))
	if err != nil || result.Total != 1 {
		t.Fatalf("candidate result=%+v err=%v", result, err)
	}
	d := e.SpecSearchDiagnostics()
	if d.Backend != "bleve" || !d.Ready || d.CandidateQueries != 1 || d.FallbackQueries != 0 || r.queries.Load() != 0 {
		t.Fatalf("actual provider diagnostics=%+v sqlite queries=%d", d, r.queries.Load())
	}
	if err := e.specBleve.replace(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	result, err = e.SearchSpecs(t.Context(), specBackendRequest(t))
	if err != nil || result.Total != 1 {
		t.Fatalf("stale fallback result=%+v err=%v", result, err)
	}
	d = e.SpecSearchDiagnostics()
	if d.Ready || d.FallbackQueries != 1 {
		t.Fatalf("stale diagnostics=%+v", d)
	}
}

func TestSpecSearchSQLiteDoesNotOpenPrivateIndex(t *testing.T) {
	e, r, root := specBackendFixture(t, "sqlite")
	result, err := e.SearchSpecs(t.Context(), specBackendRequest(t))
	if err != nil || result.Total != 1 {
		t.Fatalf("sqlite result=%+v err=%v", result, err)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("sqlite touched private index: %v", err)
	}
	d := e.SpecSearchDiagnostics()
	if d.Backend != "sqlite" || !d.Ready || d.CandidateQueries != 1 || d.FallbackQueries != 0 || r.queries.Load() == 0 {
		t.Fatalf("sqlite diagnostics=%+v queries=%d", d, r.queries.Load())
	}
}

func TestSpecSearchConstructorFailureReleasesAdapterOwnership(t *testing.T) {
	root := filepath.Join(t.TempDir(), "generated")
	errStartup := errors.New("claim reset failed")
	config := Config{Root: root, BaseURL: "https://example.test"}
	engine, err := NewEngine(t.Context(), &specBackendRepository{startError: errStartup}, config)
	if engine != nil || !errors.Is(err, errStartup) {
		t.Fatalf("startup result engine=%v err=%v", engine, err)
	}
	// The constructor acquired the default provider before the authoritative
	// startup failure. A fresh adapter must immediately reacquire its ownership.
	b, err := newBleveSpecIndex(t.Context(), filepath.Join(filepath.Dir(root), "search", "specs"), config)
	if err != nil {
		t.Fatalf("startup leaked adapter ownership: %v", err)
	}
	defer b.close()
}

func TestSpecSearchOverlappingRefreshPreservesNewInstall(t *testing.T) {
	e, r, _ := specBackendFixture(t, "")
	// Keep this test's refresh scheduling deterministic; Close still owns the
	// adapter and existing readers keep using its real mutex and Scorch index.
	e.cancel()
	e.wg.Wait()
	started, release := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	var block atomic.Bool
	block.Store(true)
	e.specBleve.resources = platform.NewResourceGate(func(string) (platform.ResourceStats, error) {
		if block.Swap(false) {
			close(started)
			<-release
		}
		return platform.ResourceStats{VolumeID: "specs", TotalBytes: 1 << 40, FreeBytes: 1 << 40}, nil
	})
	done := make(chan error, 1)
	go func() { done <- e.RebuildSpecSearchIndex(t.Context()) }()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("refresh did not reach resource admission")
	}
	// IO has captured the old committed binding. A new gate installation must
	// remain possible and dirty even while the old adapter batch is waiting.
	a := r.active[0]
	a.PublicRevision++
	a.SourceRevision++
	a.VisibilityGeneration++
	a.View.Revision = a.PublicRevision
	e.gate.Lock()
	next := *e.index
	next.byID = map[string]ActivePublication{a.ProductID: a}
	next.views = []PublicView{a.View}
	e.index = &next
	e.installSpecSearchLocked(&next)
	e.gate.Unlock()
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !e.specDirty.Load() {
		t.Fatal("old refresh consumed the newer install's dirty marker")
	}
	result, err := e.SearchSpecs(t.Context(), specBackendRequest(t))
	if err != nil || result.Total != 1 || e.SpecSearchDiagnostics().FallbackQueries != 1 {
		t.Fatalf("overlap fallback=%+v err=%v diagnostics=%+v", result, err, e.SpecSearchDiagnostics())
	}
	if err := e.refreshSpecSearch(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	result, err = e.SearchSpecs(t.Context(), specBackendRequest(t))
	d := e.SpecSearchDiagnostics()
	if err != nil || result.Total != 1 || !d.Ready || d.CandidateQueries != 1 || e.specDirty.Load() {
		t.Fatalf("new snapshot result=%+v err=%v diagnostics=%+v dirty=%v", result, err, d, e.specDirty.Load())
	}
}

func TestSpecSearchMachineRefreshOverlapConvergesWhenWorkerIsIdle(t *testing.T) {
	e, r := workerRefreshFixture(t)
	r.sources = r.sources[:1]
	if worked, err := e.processOne(t.Context()); err != nil || !worked {
		t.Fatalf("install Product: worked=%v err=%v", worked, err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	var block atomic.Bool
	block.Store(true)
	e.specBleve.resources = platform.NewResourceGate(func(string) (platform.ResourceStats, error) {
		if block.Swap(false) {
			close(started)
			<-release
		}
		return platform.ResourceStats{VolumeID: "specs", TotalBytes: 1 << 40, FreeBytes: 1 << 40}, nil
	})
	done := make(chan error, 1)
	go func() { done <- e.refreshSpecSearch(t.Context(), false) }()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("refresh did not reach resource admission")
	}
	// Aggregate convergence installs a new pointer with the same public
	// authority. It must not consume the deferred digest's only retry signal.
	if err := e.refreshMachine(t.Context()); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if worked, err := e.processWorkerBatch(t.Context()); err != nil || worked {
		t.Fatalf("idle worker: worked=%v err=%v", worked, err)
	}
	result, err := e.SearchSpecs(t.Context(), specBackendRequest(t))
	d := e.SpecSearchDiagnostics()
	if err != nil || result.Total != 2 || !d.Ready || d.CandidateQueries != 1 || d.FallbackQueries != 0 {
		t.Fatalf("idle convergence total=%d err=%v diagnostics=%+v", result.Total, err, d)
	}
}

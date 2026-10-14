package publishing

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blevesearch/bleve/v2/index/scorch"
	"prods/internal/catalog"
)

// Keep the existing backend fixture's real staging, visibility installation,
// oracle and Scorch adapter; supply only the repository's queued work.
type workerRefreshRepository struct {
	*specBackendRepository
	sources    []Source
	claimed    int
	checkpoint chan struct{}
	release    chan struct{}
	idle       chan struct{}
	idleOnce   sync.Once
	failID     string
	failed     atomic.Int64
}

func (r *workerRefreshRepository) ClaimPublicationIntent(ctx context.Context) (Intent, bool, error) {
	if r.claimed == 1 && r.checkpoint != nil {
		close(r.checkpoint)
		select {
		case <-r.release:
		case <-ctx.Done():
			return Intent{}, false, ctx.Err()
		}
	}
	if r.claimed == len(r.sources) {
		r.idleOnce.Do(func() { close(r.idle) })
		return Intent{}, false, nil
	}
	s := r.sources[r.claimed]
	r.claimed++
	return Intent{ID: s.Product.ID, ProductID: s.Product.ID, DesiredRevision: s.Product.Revision}, true, nil
}

func (r *workerRefreshRepository) PublicationSource(_ context.Context, id string) (Source, error) {
	if id == r.failID {
		return Source{}, errors.New("publication source unavailable")
	}
	for _, s := range r.sources {
		if s.Product.ID == id {
			return s, nil
		}
	}
	return Source{}, errors.New("unknown product")
}

func (r *workerRefreshRepository) ActivatePublication(ctx context.Context, _ Intent, a ActivePublication, coordinator VisibilityCoordinator) (bool, error) {
	coordinator.LockVisibility()
	defer coordinator.UnlockVisibility()
	a.VisibilityGeneration = int64(len(r.active) + 10)
	r.active = append(r.active, a)
	return true, coordinator.InstallVisibility(ctx)
}

func (r *workerRefreshRepository) FailPublicationIntent(context.Context, string, string) error {
	r.failed.Add(1)
	return nil
}

func workerRefreshFixture(t *testing.T) (*Engine, *workerRefreshRepository) {
	t.Helper()
	e, base, _ := specBackendFixture(t, "bleve")
	e.cancel()
	e.wg.Wait()
	// Start the real worker only after its deterministic backlog is installed.
	r := &workerRefreshRepository{specBackendRepository: base, idle: make(chan struct{})}
	for _, id := range []string{"first", "second"} {
		a := blevePublication(t)
		r.sources = append(r.sources, Source{
			Product: catalog.Product{ID: id, PartNumber: id, Revision: 1, RecordState: catalog.RecordCurrent, Status: catalog.Published},
			Route:   "/products/" + id, SiteEpoch: 2, Language: "en-US", SourceLocale: "en-US", SupportedLocales: []string{"en-US"},
			NumericFacts: a.View.NumericFacts, NumericDefinitions: a.View.NumericDefinitions,
		})
	}
	e.repository = r
	return e, r
}

func startRefreshWorker(e *Engine) {
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	e.wg.Add(1)
	go e.worker(ctx)
	e.Wake()
}

func waitRefreshWorker(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(10 * time.Second):
		t.Fatal("publication worker did not reach checkpoint")
	}
}

func workerRefreshBatches(t *testing.T, e *Engine) uint64 {
	t.Helper()
	e.specBleve.mu.Lock()
	defer e.specBleve.mu.Unlock()
	advanced, err := e.specBleve.index.Advanced()
	if err != nil {
		t.Fatal(err)
	}
	return advanced.(*scorch.Scorch).StatsMap()["TotBatches"].(uint64)
}

func TestPublicationWorkerCoalescesRefreshWithoutDeferringVisibility(t *testing.T) {
	e, r := workerRefreshFixture(t)
	r.checkpoint, r.release = make(chan struct{}), make(chan struct{})
	before := workerRefreshBatches(t, e)
	startRefreshWorker(e)
	waitRefreshWorker(t, r.checkpoint)
	// The first Product is already admitted while the next intent is pending.
	result, err := e.SearchSpecs(t.Context(), specBackendRequest(t))
	if err != nil || result.Total != 2 {
		t.Fatalf("intermediate visibility total=%d err=%v", result.Total, err)
	}
	midBatches := workerRefreshBatches(t, e) - before
	midFallbacks := e.SpecSearchDiagnostics().FallbackQueries
	close(r.release)
	waitRefreshWorker(t, r.idle)
	// Claim's empty checkpoint is before the final refresh; wait for the
	// production processing lock to be released before inspecting the adapter.
	e.processMu.Lock()
	e.processMu.Unlock()
	if batches := workerRefreshBatches(t, e) - before; batches != 1 {
		t.Fatalf("two changed activations performed %d full Scorch batches; want 1", batches)
	}
	if midBatches != 0 || midFallbacks != 1 {
		t.Fatalf("intermediate batches=%d fallback queries=%d; want 0/1", midBatches, midFallbacks)
	}
	result, err = e.SearchSpecs(t.Context(), specBackendRequest(t))
	d := e.SpecSearchDiagnostics()
	if err != nil || result.Total != 3 || !d.Ready || d.CandidateQueries != 1 || e.specDirty.Load() {
		t.Fatalf("drained result=%+v err=%v diagnostics=%+v dirty=%v", result, err, d, e.specDirty.Load())
	}
	t.Log("activations=2 Scorch batches=1 intermediate fallback=1 final candidate=1")
}

func TestPublicationWorkerRefreshesInstalledChangesAfterIntentFailure(t *testing.T) {
	e, r := workerRefreshFixture(t)
	r.failID = "second"
	before := workerRefreshBatches(t, e)
	startRefreshWorker(e)
	waitRefreshWorker(t, r.idle)
	e.processMu.Lock()
	e.processMu.Unlock()
	result, err := e.SearchSpecs(t.Context(), specBackendRequest(t))
	d := e.SpecSearchDiagnostics()
	if err != nil || result.Total != 2 || r.failed.Load() != 1 || workerRefreshBatches(t, e)-before != 1 || !d.Ready || e.specDirty.Load() {
		t.Fatalf("failure drain result=%+v err=%v failed=%d batches=%d diagnostics=%+v dirty=%v", result, err, r.failed.Load(), workerRefreshBatches(t, e)-before, d, e.specDirty.Load())
	}
	e.BeginDrain()
	if worked, err := e.ProcessOne(t.Context()); err != nil || worked {
		t.Fatalf("after drain worked=%v err=%v", worked, err)
	}
	t.Log("activations=1 failed intents=1 Scorch batches=1; drain accepts no new work")
}

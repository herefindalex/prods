package webapp

import (
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/index/scorch"
	"prods/internal/platform"
	"prods/internal/publishing"
	"prods/internal/storage/sqlite"
)

func assertCommittedSpecActivation(t testing.TB, store *sqlite.Store, productID string) publishing.ActivePublication {
	t.Helper()
	active, err := store.ActivePublications(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].ProductID != productID || active[0].VisibilityGeneration <= 0 {
		t.Fatalf("expected one committed activation with durable generation: %+v", active)
	}
	return active[0]
}

func assertSpecProviderReady(t testing.TB, engine *publishing.Engine, backend string) {
	t.Helper()
	d := engine.SpecSearchDiagnostics()
	if d.Backend != backend || !d.Ready || d.SourceDigest == "" || d.LastError != "" {
		t.Fatalf("selected provider not ready: %+v", d)
	}
}

func assertSpecProviderQueries(t testing.TB, engine *publishing.Engine, backend string, before publishing.SpecSearchDiagnostics, candidates, fallbacks uint64) {
	t.Helper()
	after := engine.SpecSearchDiagnostics()
	if after.Backend != backend || after.CandidateQueries != before.CandidateQueries+candidates || after.FallbackQueries != before.FallbackQueries+fallbacks {
		t.Fatalf("provider query evidence: before=%+v after=%+v", before, after)
	}
}

func TestSpecSearchBothBackendsReopenAndRebuildPublishedFacts(t *testing.T) {
	for _, backend := range []string{"bleve", "sqlite"} {
		t.Run(backend, func(t *testing.T) {
			f := newNumericPublicationFixtureForBackend(t, backend)
			published := assertCommittedSpecActivation(t, f.store, f.productID)
			assertSpecProviderReady(t, f.engine, backend)
			digest := f.engine.SpecSearchDiagnostics().SourceDigest
			publishedRequest := specSearchRequest(t, f, "4")
			workingRequest := specSearchRequest(t, f, "8")
			setSpecSearchRaw(t, f, "7–9V")
			working, err := f.store.PublicationSource(t.Context(), f.productID)
			if err != nil {
				t.Fatal(err)
			}
			if working.Product.Revision <= published.PublicRevision || reflect.DeepEqual(working.NumericFacts, published.View.NumericFacts) {
				t.Fatal("fixture must retain distinct unactivated working facts")
			}
			f.engine.Close()
			if err = f.store.Close(); err != nil {
				t.Fatal(err)
			}
			if backend == "bleve" {
				// Open the actual persisted index while the publisher is closed.
				index, err := bleve.Open(filepath.Join(f.root, "search", "specs", "index"))
				if err != nil {
					t.Fatal(err)
				}
				advanced, advancedErr := index.Advanced()
				_, isScorch := advanced.(*scorch.Scorch)
				count, countErr := index.DocCount()
				closeErr := index.Close()
				if advancedErr != nil || !isScorch || countErr != nil || count != 1 || closeErr != nil {
					t.Fatalf("persisted Scorch: type=%T docs=%d errors=%v/%v/%v", advanced, count, advancedErr, countErr, closeErr)
				}
			}
			store, err := sqlite.OpenReady(filepath.Join(f.root, "prods.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { store.Close() })
			engine, err := publishing.NewEngine(t.Context(), &numericPublicationRepository{Store: store}, publishing.Config{Root: filepath.Join(f.root, "generated"), BaseURL: "http://catalog.example.test", SpecSearchBackend: backend})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(engine.Close)
			if got := assertCommittedSpecActivation(t, store, f.productID); got.VisibilityGeneration != published.VisibilityGeneration || got.PublicRevision != published.PublicRevision {
				t.Fatalf("reopen replaced committed activation: %+v", got)
			}
			assertPublished := func() {
				t.Helper()
				assertSpecProviderReady(t, engine, backend)
				before := engine.SpecSearchDiagnostics()
				if before.SourceDigest != digest {
					t.Fatalf("unactivated working content changed committed digest: %+v", before)
				}
				got, err := engine.SearchSpecs(t.Context(), publishedRequest)
				if err != nil || got.Total != 1 || len(got.Rows) != 1 || got.Rows[0].ID != f.productID || got.Rows[0].Revision != published.PublicRevision || !reflect.DeepEqual(got.Rows[0].NumericFacts, published.View.NumericFacts) {
					t.Fatalf("published A lost: %+v err=%v", got, err)
				}
				got, err = engine.SearchSpecs(t.Context(), workingRequest)
				if err != nil || got.Total != 0 || len(got.Rows) != 0 {
					t.Fatalf("working B leaked: %+v err=%v", got, err)
				}
				assertSpecProviderQueries(t, engine, backend, before, 2, 0)
			}
			assertPublished()
			if err = engine.RebuildSpecSearchIndex(t.Context()); err != nil {
				t.Fatal(err)
			}
			assertPublished()
		})
	}
}

func TestSpecSearchSiteEpochFallbackUsesCurrentFacts(t *testing.T) {
	for _, backend := range []string{"bleve", "sqlite"} {
		t.Run(backend, func(t *testing.T) {
			var holdPrevious atomic.Bool
			providerConfig := publishing.Config{SpecSearchBackend: backend, ResourceGate: platform.NewResourceGate(func(path string) (platform.ResourceStats, error) {
				if holdPrevious.Load() && filepath.Base(path) == "specs" && filepath.Base(filepath.Dir(path)) == "search" {
					return platform.ResourceStats{}, errors.New("test fixture holds the previous Scorch snapshot")
				}
				return platform.ResourceStats{VolumeID: "fixture", TotalBytes: 1 << 40, FreeBytes: 1 << 40}, nil
			})}
			f := newNumericPublicationFixtureWithConfig(t, providerConfig, &holdPrevious)
			old := assertCommittedSpecActivation(t, f.store, f.productID)
			request := specSearchRequest(t, f, "4")
			request.CategoryID = old.View.CategoryID
			assertSpecProviderReady(t, f.engine, backend)
			before := f.engine.SpecSearchDiagnostics()
			// Block derived refresh after the real provider is ready. Bleve's
			// failed admission may invalidate its handle; SQLite's stale binding
			// is pinned below. Authority activation and candidate reads stay real.
			holdPrevious.Store(true)
			epoch, config, err := f.store.PublicSiteRouteConfig(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			_, workingRevision, err := f.store.WorkingWebsiteConfiguration(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.engine.PublishSiteRoutes(t.Context(), publishing.SiteRouteRequest{ActorID: f.actor, ExpectedEpoch: epoch, ExpectedWorkingRevision: workingRevision, Config: config}); err != nil {
				t.Fatal(err)
			}
			current := assertCommittedSpecActivation(t, f.store, f.productID)
			if current.SiteEpoch <= old.SiteEpoch || !reflect.DeepEqual(current.View.NumericFacts, old.View.NumericFacts) {
				t.Fatalf("site-wide activation did not retain facts at new epoch: old=%+v current=%+v", old, current)
			}
			if backend == "sqlite" {
				// Make the real rebuildable projection retain the prior epoch.
				// The normal repository read must reject its obsolete binding.
				db, err := sql.Open("sqlite", filepath.Join(f.root, "prods.db"))
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				if _, err = db.Exec(`UPDATE public_spec_projection_products SET site_epoch=? WHERE product_id=?`, old.SiteEpoch, f.productID); err != nil {
					t.Fatal(err)
				}
			} else if d := f.engine.SpecSearchDiagnostics(); d.Ready || (d.SourceDigest != before.SourceDigest && d.LastError == "") {
				// Refresh failure may close the adapter handle, retaining its
				// old disk index but clearing the digest. Both states must deny
				// candidate readiness against the newly committed epoch.
				t.Fatalf("previous Scorch snapshot unexpectedly current: %+v", d)
			}
			got, err := f.engine.SearchSpecs(t.Context(), request)
			if err != nil || got.Total != 1 || len(got.Rows) != 1 || got.Rows[0].SiteEpoch != current.SiteEpoch || got.Rows[0].CategoryID != request.CategoryID || !reflect.DeepEqual(got.Rows[0].NumericFacts, current.View.NumericFacts) {
				t.Fatalf("stale candidate changed active scope/facts: %+v err=%v", got, err)
			}
			assertSpecProviderQueries(t, f.engine, backend, before, 0, 1)
			holdPrevious.Store(false)
			if err = f.engine.RebuildSpecSearchIndex(t.Context()); err != nil {
				t.Fatal(err)
			}
			assertSpecProviderReady(t, f.engine, backend)
			before = f.engine.SpecSearchDiagnostics()
			got, err = f.engine.SearchSpecs(t.Context(), request)
			if err != nil || got.Total != 1 || len(got.Rows) != 1 || got.Rows[0].SiteEpoch != current.SiteEpoch {
				t.Fatalf("rebuilt current epoch missing: %+v err=%v", got, err)
			}
			assertSpecProviderQueries(t, f.engine, backend, before, 1, 0)
		})
	}
}

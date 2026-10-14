package webapp

import (
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"prods/internal/catalog"
	"prods/internal/publishing"
	"prods/internal/specnumeric"
	"prods/internal/storage/sqlite"
)

func specSearchRequest(t testing.TB, f numericPublicationFixture, point string) publishing.SpecSearchRequest {
	t.Helper()
	c, err := f.store.SpecNormalizationContext(t.Context(), f.valueID)
	if err != nil {
		t.Fatal(err)
	}
	value, err := specnumeric.ParseDecimal(point)
	if err != nil {
		t.Fatal(err)
	}
	return publishing.SpecSearchRequest{Predicates: []specnumeric.Predicate{{SpecID: c.SpecID, Operator: specnumeric.OperatorContainsPoint, Value: &value, Unit: "V"}}, Page: 1, PageSize: 20}
}

func setSpecSearchRaw(t testing.TB, f numericPublicationFixture, raw string) {
	t.Helper()
	c, err := f.store.SpecNormalizationContext(t.Context(), f.valueID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.SaveSpecValue(t.Context(), f.actor, f.productID, c.SpecID, raw, "en-US", c.Expected.ProductRevision); err != nil {
		t.Fatal(err)
	}
	f.adopt(t)
}

func TestSpecSearchRestartAndProjectionRebuildUsePublishedFacts(t *testing.T) {
	f := newNumericPublicationFixtureForBackend(t, "sqlite")
	published := f.view(t)
	publishedRequest := specSearchRequest(t, f, "4")
	workingRequest := specSearchRequest(t, f, "8")
	setSpecSearchRaw(t, f, "7–9V")
	working, err := f.store.PublicationSource(t.Context(), f.productID)
	if err != nil {
		t.Fatal(err)
	}
	if len(working.NumericFacts) != 1 || reflect.DeepEqual(working.NumericFacts, published.NumericFacts) || working.Product.Revision <= published.Revision {
		t.Fatalf("fixture did not retain distinct unactivated working facts: %+v", working)
	}
	f.engine.Close()
	if err = f.store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := sqlite.OpenReady(filepath.Join(f.root, "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	engine, err := publishing.NewEngine(t.Context(), &numericPublicationRepository{Store: store}, publishing.Config{Root: filepath.Join(f.root, "generated"), BaseURL: "http://catalog.example.test", SpecSearchBackend: "sqlite"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engine.Close)
	assertPublishedFacts := func() {
		t.Helper()
		got, err := engine.SearchSpecs(t.Context(), publishedRequest)
		if err != nil {
			t.Fatal(err)
		}
		if got.Total != 1 || len(got.Rows) != 1 || got.Rows[0].ID != f.productID || got.Rows[0].Revision != published.Revision || !reflect.DeepEqual(got.Rows[0].NumericFacts, published.NumericFacts) {
			t.Fatalf("persisted published facts lost: %+v", got)
		}
		got, err = engine.SearchSpecs(t.Context(), workingRequest)
		if err != nil {
			t.Fatal(err)
		}
		if got.Total != 0 || len(got.Rows) != 0 {
			t.Fatalf("unactivated working facts leaked: %+v", got)
		}
	}
	assertPublishedFacts()
	db, err := sql.Open("sqlite", filepath.Join(f.root, "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`DELETE FROM public_spec_projection_fields WHERE product_id=?`, f.productID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`DELETE FROM public_spec_projection_products WHERE product_id=?`, f.productID); err != nil {
		t.Fatal(err)
	}
	assertSpecSearchProjectionCounts(t, db, f.productID, 0, 0)
	if err = engine.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	assertSpecSearchProjectionCounts(t, db, f.productID, 1, 1)
	assertPublishedFacts()
}

func TestSpecSearchRevocationRemovesRowsAndCount(t *testing.T) {
	for _, backend := range []string{"bleve", "sqlite"} {
		t.Run(backend, func(t *testing.T) {
			f := newNumericPublicationFixtureForBackend(t, backend)
			assertCommittedSpecActivation(t, f.store, f.productID)
			assertSpecProviderReady(t, f.engine, backend)
			diagnostics := f.engine.SpecSearchDiagnostics()
			request := specSearchRequest(t, f, "4")
			before, err := f.engine.SearchSpecs(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			if before.Total != 1 || len(before.Rows) != 1 || before.Rows[0].ID != f.productID {
				t.Fatalf("published match missing before Hide: %+v", before)
			}
			assertSpecProviderQueries(t, f.engine, backend, diagnostics, 1, 0)
			db, err := sql.Open("sqlite", filepath.Join(f.root, "prods.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			assertSpecSearchProjectionCounts(t, db, f.productID, 1, 1)
			product, err := f.store.Product(t.Context(), f.productID)
			if err != nil {
				t.Fatal(err)
			}
			if err = f.engine.Revoke(t.Context(), publishing.RevokeRequest{ActorID: f.actor, ProductID: f.productID, ExpectedRevision: product.Revision}); err != nil {
				t.Fatal(err)
			}
			// Read the physical rows immediately after successful Hide, before any
			// query or worker could mask stale projection records.
			assertSpecSearchProjectionCounts(t, db, f.productID, 0, 0)
			after, err := f.engine.SearchSpecs(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			if after.Total != 0 || len(after.Rows) != 0 {
				t.Fatalf("revoked product retained rows/count: %+v", after)
			}

		})
	}
}

func assertSpecSearchProjectionCounts(t *testing.T, db *sql.DB, productID string, wantProducts, wantFields int) {
	t.Helper()
	var products, fields int
	if err := db.QueryRow(`SELECT
		(SELECT COUNT(*) FROM public_spec_projection_products WHERE product_id=?),
		(SELECT COUNT(*) FROM public_spec_projection_fields WHERE product_id=?)`, productID, productID).Scan(&products, &fields); err != nil {
		t.Fatal(err)
	}
	if products != wantProducts || fields != wantFields {
		t.Fatalf("physical spec projection products/fields=%d/%d, want %d/%d", products, fields, wantProducts, wantFields)
	}
}

func TestSpecSearchPreservesIntervalGap(t *testing.T) {
	f := newNumericPublicationFixture(t)
	setSpecSearchRaw(t, f, "3–5 V, 7–9 V")
	f.drain(t)
	got, err := f.engine.SearchSpecs(t.Context(), specSearchRequest(t, f, "6"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 0 || len(got.Rows) != 0 {
		t.Fatalf("gap matched: %+v", got)
	}
}

func TestSpecSearchReturnsSupportedPoint(t *testing.T) {
	f := newNumericPublicationFixture(t)
	got, err := f.engine.SearchSpecs(t.Context(), specSearchRequest(t, f, "4"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 || len(got.Rows) != 1 || got.Rows[0].ID != f.productID || got.Rows[0].Revision != f.view(t).Revision {
		t.Fatalf("supported point: %+v", got)
	}
	// Activation must produce both the marker and its actual Spec-ID row.
	snapshot, err := f.store.SpecSearchCandidates(t.Context(), []string{got.Rows[0].NumericFacts[0].SpecID}, publishing.SpecProjectionVersion)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Inventory) != 1 || len(snapshot.Inventory[0].SpecIDs) != 1 || len(snapshot.Candidates) != 1 {
		t.Fatalf("projection: %+v", snapshot)
	}
}

func TestSpecSearchPaginatesAfterExactMatch(t *testing.T) {
	f := newNumericPublicationFixture(t)
	c, err := f.store.SpecNormalizationContext(t.Context(), f.valueID)
	if err != nil {
		t.Fatal(err)
	}
	original, err := f.store.Product(t.Context(), f.productID)
	if err != nil {
		t.Fatal(err)
	}
	var want string
	for _, part := range []string{"ZZ-MATCH-1", "ZZ-MATCH-2"} {
		product, err := f.store.CreateProduct(t.Context(), f.actor, catalog.Product{PartNumber: part, CategoryID: original.CategoryID, Status: catalog.Published})
		if err != nil {
			t.Fatal(err)
		}
		value, err := f.store.SaveSpecValue(t.Context(), f.actor, product.ID, c.SpecID, "7–9V", "en-US", product.Revision)
		if err != nil {
			t.Fatal(err)
		}
		next := f
		next.productID = product.ID
		next.valueID = value.ID
		next.adopt(t)
		if want == "" {
			want = product.ID
		}
	}
	f.drain(t)
	request := specSearchRequest(t, f, "8")
	request.PageSize = 1
	got, err := f.engine.SearchSpecs(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 2 || len(got.Rows) != 1 || got.Rows[0].ID != want {
		t.Fatalf("pagination before oracle: %+v", got)
	}
}

func TestSpecSearchFallsBackWhenFieldProjectionMissing(t *testing.T) {
	f := newNumericPublicationFixtureForBackend(t, "sqlite")
	db, err := sql.Open("sqlite", filepath.Join(f.root, "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`DELETE FROM public_spec_projection_fields WHERE product_id=?`, f.productID); err != nil {
		t.Fatal(err)
	}
	var markers int
	if err = db.QueryRow(`SELECT COUNT(*) FROM public_spec_projection_products WHERE product_id=?`, f.productID).Scan(&markers); err != nil || markers != 1 {
		t.Fatalf("marker=%d err=%v", markers, err)
	}
	got, err := f.engine.SearchSpecs(t.Context(), specSearchRequest(t, f, "4"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 || len(got.Rows) != 1 || got.Rows[0].ID != f.productID {
		t.Fatalf("missing field hid match: %+v", got)
	}
}

func TestSpecSearchKeepsActiveFactsUntilNextActivation(t *testing.T) {
	f := newNumericPublicationFixture(t)
	request := specSearchRequest(t, f, "4")
	before := f.view(t)
	setSpecSearchRaw(t, f, "7–9V")
	got, err := f.engine.SearchSpecs(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 || len(got.Rows) != 1 || got.Rows[0].Revision != before.Revision {
		t.Fatalf("working facts leaked: %+v", got)
	}
	f.drain(t)
	got, err = f.engine.SearchSpecs(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 0 {
		t.Fatalf("old facts survived activation: %+v", got)
	}
}

func TestSpecSearchAbsentDefinitionIsUnknown(t *testing.T) {
	f := newNumericPublicationFixture(t)
	request := specSearchRequest(t, f, "4")
	request.Predicates[0].SpecID = "spec_absent"
	got, err := f.engine.SearchSpecs(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 0 || len(got.Rows) != 0 {
		t.Fatalf("missing definition established Match: %+v", got)
	}
}

func TestSpecSearchRejectsIncompatibleScopedDefinitions(t *testing.T) {
	f := newNumericPublicationFixture(t)
	c, err := f.store.SpecNormalizationContext(t.Context(), f.valueID)
	if err != nil {
		t.Fatal(err)
	}
	category, err := f.store.CreateCategory(t.Context(), f.actor, catalog.Category{Name: "Other scope", Slug: "other-scope"})
	if err != nil {
		t.Fatal(err)
	}
	set, err := f.store.CreateSpecSet(t.Context(), f.actor, catalog.SpecSet{Name: "Other scope", SpecIDs: []string{c.SpecID}})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.store.SetCategorySpecSet(t.Context(), f.actor, category.ID, set.ID, category.Revision); err != nil {
		t.Fatal(err)
	}
	product, err := f.store.CreateProduct(t.Context(), f.actor, catalog.Product{PartNumber: "OTHER-SCOPE", CategoryID: category.ID, Status: catalog.Published})
	if err != nil {
		t.Fatal(err)
	}
	value, err := f.store.SaveSpecValue(t.Context(), f.actor, product.ID, c.SpecID, "3–5V", "en-US", product.Revision)
	if err != nil {
		t.Fatal(err)
	}
	other := f
	other.productID = product.ID
	other.valueID = value.ID
	other.adopt(t)
	f.drain(t)
	definitions, err := f.store.ListSpecDefinitions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var revision int64
	for _, definition := range definitions {
		if definition.ID == c.SpecID {
			revision = definition.Revision
		}
	}
	profile := c.Profile
	profile.DeclaredInputUnit = "V"
	if _, err = f.store.ConfigureSpecNumericProfile(t.Context(), f.actor, c.SpecID, revision, profile); err != nil {
		t.Fatal(err)
	}
	request := specSearchRequest(t, f, "4")
	// A working definition change alone cannot invalidate the frozen query.
	before, err := f.engine.SearchSpecs(t.Context(), request)
	if err != nil || before.Total != 2 {
		t.Fatalf("working profile changed active query: %+v, %v", before, err)
	}
	var unchanged publishing.PublicView
	for n := 0; n < 10; n++ {
		if _, err = f.engine.ProcessOne(f.ctx); err != nil {
			t.Fatal(err)
		}
		views, err := f.engine.Views()
		if err != nil {
			t.Fatal(err)
		}
		changed := 0
		for _, view := range views {
			if view.NumericDefinitions[0].Profile.DeclaredInputUnit == "V" {
				changed++
			} else {
				unchanged = view
			}
		}
		if changed == 1 {
			break
		}
		if n == 9 {
			t.Fatal("did not reach a partial profile rollout")
		}
	}
	if _, err = f.engine.SearchSpecs(t.Context(), request); !errors.Is(err, publishing.ErrPublicUnavailable) {
		t.Fatalf("mixed definitions error=%v", err)
	}
	// The incompatible definition is outside this exact frozen category scope.
	request.CategoryID = unchanged.CategoryID
	got, err := f.engine.SearchSpecs(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 || len(got.Rows) != 1 || got.Rows[0].ID != unchanged.ID {
		t.Fatalf("outside definition contaminated scope: %+v", got)
	}
}

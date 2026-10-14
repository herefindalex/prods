package webapp

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"prods/internal/catalog"
	"prods/internal/publishing"
	"prods/internal/specnumeric"
	"prods/internal/storage/sqlite"
)

// This fixture uses real controlled writes, adopted facts, publication units,
// file-backed SQLite and the selected production candidate provider. The
// inherited repository pauses publication wake-ups; ProcessBatch explicitly
// drains setup work. Once timed, the published corpus is immutable.
type specBenchmarkCorpus struct {
	numericPublicationFixture
	queries  []specBenchmarkQuery
	products int
}

type specBenchmarkQuery struct {
	name    string
	request publishing.SpecSearchRequest
}

func specBenchmarkProducts(t testing.TB) int {
	t.Helper()
	if raw := os.Getenv("PRODS_BENCH_PRODUCTS"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			t.Fatalf("PRODS_BENCH_PRODUCTS must be positive: %q", raw)
		}
		return n
	}
	return 1000
}

func newSpecBenchmarkCorpus(t testing.TB, backend string) *specBenchmarkCorpus {
	return newSpecBenchmarkCorpusWithCount(t, backend, specBenchmarkProducts(t), true)
}

// Worker benchmarks preserve the same controlled corpus writes but leave the
// initial publication backlog undrained. Explicit setup work is never timed.
func newSpecBenchmarkCorpusWithCount(t testing.TB, backend string, products int, drain bool) *specBenchmarkCorpus {
	t.Helper()
	f := newNumericPublicationFixtureForBackend(t, backend)
	seed, err := f.store.Product(t.Context(), f.productID)
	if err != nil {
		t.Fatal(err)
	}
	support, err := f.store.CreateSpecDefinition(t.Context(), f.actor, catalog.SpecDefinition{Name: "Benchmark support", Filterable: true})
	if err != nil {
		t.Fatal(err)
	}
	nominal, err := f.store.CreateSpecDefinition(t.Context(), f.actor, catalog.SpecDefinition{Name: "Benchmark nominal", Filterable: true})
	if err != nil {
		t.Fatal(err)
	}
	profile := specnumeric.Profile{Quantity: "voltage", SemanticRole: specnumeric.RoleOperatingSupport, AllowedShapes: []specnumeric.Shape{specnumeric.ShapeIntervalSet, specnumeric.ShapeDiscreteSet}, AllowedOperators: []specnumeric.Operator{specnumeric.OperatorContainsPoint, specnumeric.OperatorOverlapsRange}, CanonicalUnit: "V", NumericFormat: specnumeric.NumericFormatDecimalDot, SchemaVersion: specnumeric.SchemaVersion, ParserVersion: specnumeric.ParserVersion, UnitRegistryVersion: specnumeric.UnitRegistryVersion}
	if _, err = f.store.ConfigureSpecNumericProfile(t.Context(), f.actor, support.ID, support.Revision, profile); err != nil {
		t.Fatal(err)
	}
	profile.SemanticRole = specnumeric.RoleNominal
	profile.AllowedShapes = []specnumeric.Shape{specnumeric.ShapeScalar, specnumeric.ShapeTolerance}
	profile.AllowedOperators = []specnumeric.Operator{specnumeric.OperatorBetween}
	if _, err = f.store.ConfigureSpecNumericProfile(t.Context(), f.actor, nominal.ID, nominal.Revision, profile); err != nil {
		t.Fatal(err)
	}
	set, err := f.store.CreateSpecSet(t.Context(), f.actor, catalog.SpecSet{Name: "Benchmark", SpecIDs: []string{support.ID, nominal.ID}})
	if err != nil {
		t.Fatal(err)
	}
	category, err := f.store.Category(t.Context(), seed.CategoryID)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.store.SetCategorySpecSet(t.Context(), f.actor, category.ID, set.ID, category.Revision); err != nil {
		t.Fatal(err)
	}
	f.drain(t)
	for i := 0; i < products; i++ {
		var p catalog.Product
		if i == 0 {
			p, err = f.store.Product(t.Context(), seed.ID)
			if err == nil {
				p.PartNumber = fmt.Sprintf("BENCH-%06d", i)
				p, err = f.store.UpdateProduct(t.Context(), f.actor, p.Revision, p)
			}
		} else {
			p, err = f.store.CreateProduct(t.Context(), f.actor, catalog.Product{PartNumber: fmt.Sprintf("BENCH-%06d", i), CategoryID: category.ID, Status: catalog.Published, Specification: "Benchmark shared specification"})
		}
		if err != nil {
			t.Fatal(err)
		}
		n := i % 100
		supportRaw := fmt.Sprintf("%d–%d V", n, n+2)
		if i%2 == 1 {
			supportRaw = fmt.Sprintf("%d / %d V", n, n+2)
		}
		nominalRaw := fmt.Sprintf("%d V", i%25)
		if i%2 == 1 {
			nominalRaw = fmt.Sprintf("%d V ±0.5 V", i%25)
		}
		for _, value := range []struct{ id, raw string }{{support.ID, supportRaw}, {nominal.ID, nominalRaw}} {
			p, err = f.store.Product(t.Context(), p.ID)
			if err != nil {
				t.Fatal(err)
			}
			v, saveErr := f.store.SaveSpecValue(t.Context(), f.actor, p.ID, value.id, value.raw, "en-US", p.Revision)
			if saveErr != nil {
				t.Fatal(saveErr)
			}
			adopter := f
			adopter.valueID = v.ID
			adopter.adopt(t)
		}
	}
	if drain {
		if _, err = f.engine.ProcessBatch(f.ctx, 0); err != nil {
			t.Fatal(err)
		}
		views, err := f.engine.Views()
		if err != nil || len(views) != products {
			t.Fatalf("corpus views=%d err=%v", len(views), err)
		}
		for _, view := range views {
			if len(view.NumericFacts) != 2 {
				t.Fatalf("corpus product %s facts=%d", view.PartNumber, len(view.NumericFacts))
			}
		}
	}
	decimal := func(s string) *specnumeric.Decimal {
		d, err := specnumeric.ParseDecimal(s)
		if err != nil {
			t.Fatal(err)
		}
		return &d
	}
	request := func(predicates ...specnumeric.Predicate) publishing.SpecSearchRequest {
		return publishing.SpecSearchRequest{Predicates: predicates, Locale: "en-US", Page: 1, PageSize: 50}
	}
	c := &specBenchmarkCorpus{numericPublicationFixture: f, products: products, queries: []specBenchmarkQuery{
		{"selective-point", request(specnumeric.Predicate{SpecID: support.ID, Operator: specnumeric.OperatorContainsPoint, Value: decimal("42"), Unit: "V"})},
		{"and-ranges", request(specnumeric.Predicate{SpecID: support.ID, Operator: specnumeric.OperatorOverlapsRange, Lower: decimal("40"), Upper: decimal("60"), Unit: "V"}, specnumeric.Predicate{SpecID: nominal.ID, Operator: specnumeric.OperatorBetween, Lower: decimal("10"), Upper: decimal("15"), Unit: "V"})},
		{"broad-page-2", request(specnumeric.Predicate{SpecID: support.ID, Operator: specnumeric.OperatorOverlapsRange, Lower: decimal("0"), Upper: decimal("101"), Unit: "V"})},
	}}
	c.queries[2].request.Page = 2
	if drain {
		assertSpecProviderReady(t, c.engine, backend)
	}
	t.Cleanup(func() { c.engine.Close(); c.store.Close() })
	return c
}

func (c *specBenchmarkCorpus) reopen(t testing.TB, backend string) {
	t.Helper()
	c.engine.Close()
	if err := c.store.Close(); err != nil {
		t.Fatal(err)
	}
	c.open(t, backend)
}

func (c *specBenchmarkCorpus) open(t testing.TB, backend string) {
	t.Helper()
	store, err := sqlite.OpenReady(filepath.Join(c.root, "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	c.store = store
	engine, err := publishing.NewEngine(t.Context(), &numericPublicationRepository{Store: store}, publishing.Config{Root: filepath.Join(c.root, "generated"), BaseURL: "http://catalog.example.test", SpecSearchBackend: backend})
	if err != nil {
		t.Fatal(err)
	}
	c.engine = engine
}

func specBenchmarkSearch(t testing.TB, engine *publishing.Engine, backend string, request publishing.SpecSearchRequest) publishing.SpecSearchResult {
	t.Helper()
	before := engine.SpecSearchDiagnostics()
	result, err := engine.SearchSpecs(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	assertSpecProviderQueries(t, engine, backend, before, 1, 0)
	return result
}

type specBenchmarkPage struct {
	IDs                   []string
	Total, Page, PageSize int
}

// Pair the literal same persisted products, including every page and one past
// the end. Timing is not started until ordered IDs, Total and pages agree.
func (c *specBenchmarkCorpus) parity(t testing.TB) {
	t.Helper()
	var baseline [][]specBenchmarkPage
	for _, backend := range []string{"bleve", "sqlite"} {
		c.reopen(t, backend)
		assertSpecProviderReady(t, c.engine, backend)
		var pages [][]specBenchmarkPage
		for _, query := range c.queries {
			request := query.request
			request.Page = 1
			var results []specBenchmarkPage
			for {
				result := specBenchmarkSearch(t, c.engine, backend, request)
				page := specBenchmarkPage{Total: result.Total, Page: result.Page, PageSize: result.PageSize, IDs: []string{}}
				for _, row := range result.Rows {
					page.IDs = append(page.IDs, row.ID)
				}
				results = append(results, page)
				if len(result.Rows) == 0 {
					break
				}
				request.Page++
			}
			pages = append(pages, results)
		}
		if baseline == nil {
			baseline = pages
		} else if !reflect.DeepEqual(baseline, pages) {
			t.Fatal("paired provider ordered IDs/Total/pages differ")
		}
	}
	t.Logf("paired parity products=%d totals=%d/%d/%d pages=%d/%d/%d; zero fallback", c.products, baseline[0][0].Total, baseline[1][0].Total, baseline[2][0].Total, len(baseline[0]), len(baseline[1]), len(baseline[2]))
}

func BenchmarkSpecSearchBackends(b *testing.B) {
	c := newSpecBenchmarkCorpus(b, "bleve")
	c.parity(b)
	for _, backend := range []string{"bleve", "sqlite"} {
		c.reopen(b, backend)
		for _, query := range c.queries {
			b.Run(backend+"/"+query.name, func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					specBenchmarkSearch(b, c.engine, backend, query.request)
				}
			})
		}
	}
}

func BenchmarkSpecSearchRebuild(b *testing.B) {
	c := newSpecBenchmarkCorpus(b, "bleve")
	c.parity(b)
	for _, backend := range []string{"bleve", "sqlite"} {
		c.reopen(b, backend)
		b.Run(backend, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := c.engine.RebuildSpecSearchIndex(b.Context()); err != nil {
					b.Fatal(err)
				}
				b.StopTimer()
				assertSpecProviderReady(b, c.engine, backend)
				specBenchmarkSearch(b, c.engine, backend, c.queries[0].request)
				b.StartTimer()
			}
		})
	}
}

// Reopen includes Store.OpenReady, Engine.NewEngine reconciliation (including
// projection repair), and first query. Close is excluded; OS caches are kept.
func BenchmarkSpecSearchReopen(b *testing.B) {
	c := newSpecBenchmarkCorpus(b, "bleve")
	c.parity(b)
	for _, backend := range []string{"bleve", "sqlite"} {
		b.Run(backend, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				c.engine.Close()
				if err := c.store.Close(); err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
				c.open(b, backend)
				specBenchmarkSearch(b, c.engine, backend, c.queries[0].request)
			}
		})
	}
}

// Run a precompiled test binary in a fresh process per backend. This entry
// never opens the other provider or runs paired parity; RSS includes real
// corpus construction/publication and the selected engine's startup/cache.
func BenchmarkSpecSearchRSS(b *testing.B) {
	backend := os.Getenv("PRODS_BENCH_RSS_BACKEND")
	if backend != "bleve" && backend != "sqlite" {
		b.Skip("set PRODS_BENCH_RSS_BACKEND=bleve or sqlite")
	}
	c := newSpecBenchmarkCorpus(b, backend)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, query := range c.queries {
			specBenchmarkSearch(b, c.engine, backend, query.request)
		}
	}
}

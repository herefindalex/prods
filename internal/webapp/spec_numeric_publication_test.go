package webapp

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/publishing"
	"prods/internal/specnumeric"
	"prods/internal/storage/sqlite"
)

// The real publisher stages and activates normally; only its background wake-up
// is paused so the test can observe the admitted revision before activation.
type numericPublicationKey struct{}
type numericPublicationRepository struct {
	*sqlite.Store
	beforeActivate         func()
	specRefreshUnavailable *atomic.Bool
}

func (r *numericPublicationRepository) RebuildSpecSearchProjection(ctx context.Context, active []publishing.ActivePublication) error {
	if r.specRefreshUnavailable != nil && r.specRefreshUnavailable.Load() {
		return errors.New("test fixture holds the previous spec projection")
	}
	return r.Store.RebuildSpecSearchProjection(ctx, active)
}

func (r *numericPublicationRepository) ClaimPublicationIntent(ctx context.Context) (publishing.Intent, bool, error) {
	if ctx.Value(numericPublicationKey{}) == nil {
		return publishing.Intent{}, false, nil
	}
	return r.Store.ClaimPublicationIntent(ctx)
}
func (r *numericPublicationRepository) ProcessSpecNumericProfileWork(ctx context.Context, limit int) (bool, error) {
	if ctx.Value(numericPublicationKey{}) == nil {
		return false, nil
	}
	return r.Store.ProcessSpecNumericProfileWork(ctx, limit)
}

type numericPublicationFixture struct {
	store                           *sqlite.Store
	engine                          *publishing.Engine
	actor, productID, valueID, root string
	ctx                             context.Context
}

func newNumericPublicationFixture(t testing.TB) numericPublicationFixture {
	return newNumericPublicationFixtureForBackend(t, "")
}

func newNumericPublicationFixtureForBackend(t testing.TB, backend string) numericPublicationFixture {
	return newNumericPublicationFixtureWithConfig(t, publishing.Config{SpecSearchBackend: backend}, nil)
}

func newNumericPublicationFixtureWithConfig(t testing.TB, config publishing.Config, specRefreshUnavailable *atomic.Bool) numericPublicationFixture {
	t.Helper()
	root := t.TempDir()
	store, err := sqlite.Create(filepath.Join(root, "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	hash, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.CompleteInstallation(t.Context(), sqlite.Installation{OwnerEmail: "owner@example.test", PasswordHash: hash, DefaultLocale: "en-US", SupportedLocales: []string{"en-US", "zh-TW"}, TimeZone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	category, err := store.CreateCategory(t.Context(), owner.ID, catalog.Category{Name: "Numeric", Slug: "numeric"})
	if err != nil {
		t.Fatal(err)
	}
	spec, err := store.CreateSpecDefinition(t.Context(), owner.ID, catalog.SpecDefinition{Name: "Operating voltage", Filterable: true})
	if err != nil {
		t.Fatal(err)
	}
	set, err := store.CreateSpecSet(t.Context(), owner.ID, catalog.SpecSet{Name: "Numeric", SpecIDs: []string{spec.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.SetCategorySpecSet(t.Context(), owner.ID, category.ID, set.ID, category.Revision); err != nil {
		t.Fatal(err)
	}
	profile := specnumeric.Profile{Quantity: "voltage", SemanticRole: specnumeric.RoleOperatingSupport, AllowedShapes: []specnumeric.Shape{specnumeric.ShapeIntervalSet}, AllowedOperators: []specnumeric.Operator{specnumeric.OperatorContainsPoint}, CanonicalUnit: "V", NumericFormat: specnumeric.NumericFormatDecimalDot, SchemaVersion: specnumeric.SchemaVersion, ParserVersion: specnumeric.ParserVersion, UnitRegistryVersion: specnumeric.UnitRegistryVersion}
	if _, err = store.ConfigureSpecNumericProfile(t.Context(), owner.ID, spec.ID, spec.Revision, profile); err != nil {
		t.Fatal(err)
	}
	product, err := store.CreateProduct(t.Context(), owner.ID, catalog.Product{PartNumber: "NUMERIC-PUBLIC", CategoryID: category.ID, Status: catalog.Published, Specification: "Shared specification"})
	if err != nil {
		t.Fatal(err)
	}
	value, err := store.SaveSpecValue(t.Context(), owner.ID, product.ID, spec.ID, "3–5V", "en-US", product.Revision)
	if err != nil {
		t.Fatal(err)
	}
	config.Root = filepath.Join(root, "generated")
	config.BaseURL = "http://catalog.example.test"
	engine, err := publishing.NewEngine(t.Context(), &numericPublicationRepository{Store: store, specRefreshUnavailable: specRefreshUnavailable}, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engine.Close)
	f := numericPublicationFixture{store, engine, owner.ID, product.ID, value.ID, root, context.WithValue(t.Context(), numericPublicationKey{}, true)}
	f.adopt(t)
	f.drain(t)
	return f
}
func (f numericPublicationFixture) adopt(t testing.TB) {
	t.Helper()
	c, err := f.store.SpecNormalizationContext(t.Context(), f.valueID)
	if err != nil {
		t.Fatal(err)
	}
	result := specnumeric.Normalize(c.Raw, c.Profile)
	if result.Candidate == nil {
		t.Fatalf("parse: %+v", result)
	}
	encoded, err := specnumeric.MarshalValue(*result.Candidate)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.AdoptManualSpecValue(t.Context(), f.actor, catalog.AdoptSpecCommand{SpecValueID: f.valueID, Expected: c.Expected, ValueJSON: encoded}); err != nil {
		t.Fatal(err)
	}
}
func (f numericPublicationFixture) drain(t testing.TB) {
	t.Helper()
	for n := 0; n < 30; n++ {
		worked, err := f.engine.ProcessOne(f.ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !worked {
			return
		}
	}
	t.Fatal("publication work did not drain")
}
func (f numericPublicationFixture) view(t testing.TB) publishing.PublicView {
	t.Helper()
	views, err := f.engine.Views()
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 {
		t.Fatalf("views=%d", len(views))
	}
	return views[0]
}
func TestNumericFactsStayOnActiveRevisionUntilActivation(t *testing.T) {
	f := newNumericPublicationFixture(t)
	before := f.view(t)
	if len(before.NumericFacts) != 1 {
		t.Fatalf("adopted facts=%+v", before.NumericFacts)
	}
	c, err := f.store.SpecNormalizationContext(t.Context(), f.valueID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.SaveSpecValue(t.Context(), f.actor, f.productID, c.SpecID, "6–9V", "zh-TW", c.Expected.ProductRevision); err != nil {
		t.Fatal(err)
	}
	f.adopt(t)
	hits, err := f.engine.Search(t.Context(), "NUMERIC-PUBLIC")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || !reflect.DeepEqual(before.NumericFacts, hits[0].NumericFacts) || hits[0].Revision != before.Revision {
		t.Fatal("working facts leaked into active search")
	}
	source, err := f.store.PublicationSource(t.Context(), f.productID)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(source.NumericFacts, before.NumericFacts) {
		t.Fatal("working source did not change")
	}
	f.drain(t)
	after := f.view(t)
	if !reflect.DeepEqual(after.NumericFacts, source.NumericFacts) || after.Revision <= before.Revision {
		t.Fatal("activation did not install frozen facts")
	}
	if !reflect.DeepEqual(after.ForLocale("zh-TW").NumericFacts, after.NumericFacts) {
		t.Fatal("locale changed shared facts")
	}
}
func TestAdoptedNumericFactsPersistInPublicActivation(t *testing.T) {
	f := newNumericPublicationFixture(t)
	view := f.view(t)
	source, err := f.store.PublicationSource(t.Context(), f.productID)
	if err != nil {
		t.Fatal(err)
	}
	if len(source.NumericFacts) != 1 || !reflect.DeepEqual(source.NumericFacts, view.NumericFacts) {
		t.Fatalf("source/view facts: %+v / %+v", source.NumericFacts, view.NumericFacts)
	}
	// Site-wide activation must persist the same numeric definition and facts.
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
	if !reflect.DeepEqual(f.view(t).NumericFacts, view.NumericFacts) {
		t.Fatal("site activation changed facts")
	}
	f.engine.Close()
	if err = f.store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.OpenReady(filepath.Join(f.root, "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	active, err := reopened.ActivePublications(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || !reflect.DeepEqual(active[0].View.NumericFacts, view.NumericFacts) || !reflect.DeepEqual(active[0].View.NumericDefinitions, view.NumericDefinitions) {
		t.Fatalf("lost persisted facts/profile: %+v", active)
	}
	encoded, err := json.Marshal(active[0].View)
	if err != nil {
		t.Fatal(err)
	}
	var public map[string]json.RawMessage
	if err = json.Unmarshal(encoded, &public); err != nil {
		t.Fatal(err)
	}
	if len(public["numeric_facts"]) == 0 || len(public["numeric_definitions"]) == 0 {
		t.Fatal("numeric snapshot not serialized")
	}
}

func TestSharedSpecificationDoesNotUseLocaleFallback(t *testing.T) {
	f := newNumericPublicationFixture(t)
	view := f.view(t)
	view.Localizations = map[string]publishing.LocalizedContent{"zh-TW": {Name: "Localized name", Description: "Localized description", Features: "Localized features", Specification: "Legacy translated specification"}}
	localized := view.ForLocale("zh-TW")
	if localized.Specification != "Shared specification" {
		t.Fatalf("specification=%q", localized.Specification)
	}
	if localized.Name != "Localized name" || localized.Description != "Localized description" || localized.Features != "Localized features" {
		t.Fatal("approved content translations stopped resolving")
	}
	if _, ok := localized.FieldLocalization["specification"]; ok {
		t.Fatal("shared specification reports locale fallback")
	}
}

func TestSharedSpecManifestRejectsOldUnitAndRestages(t *testing.T) {
	f := newNumericPublicationFixture(t)
	active, err := f.store.ActivePublications(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	old := active[0]
	directory := filepath.Join(f.root, "generated", "units", old.ArtifactID)
	body, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err = json.Unmarshal(body, &manifest); err != nil {
		t.Fatal(err)
	}
	delete(manifest, "render_contract_version")
	body, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	if err = os.WriteFile(filepath.Join(directory, "manifest.json"), body, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(directory, "prepared"), []byte(hash), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(f.root, "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`UPDATE public_activations SET manifest_hash=? WHERE product_id=?`, hash, f.productID); err != nil {
		t.Fatal(err)
	}
	// Simulate the new source revision/intent supplied by the migration using an
	// ordinary controlled edit; the legacy activation and all files remain intact.
	p, err := f.store.Product(t.Context(), f.productID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.UpdateProduct(t.Context(), f.actor, p.Revision, p); err != nil {
		t.Fatal(err)
	}
	if err = f.engine.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	views, err := f.engine.Views()
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 0 {
		t.Fatal("incompatible unit admitted")
	}
	for _, suffix := range []string{"", ".json", ".md"} {
		recorder := httptest.NewRecorder()
		served := f.engine.ServePath(recorder, httptest.NewRequest("GET", old.Route+suffix, nil))
		if served && recorder.Code == 200 {
			t.Fatalf("incompatible format admitted: %s", suffix)
		}
	}
	f.drain(t)
	next, err := f.store.ActivePublications(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(next) != 1 || next[0].ArtifactID == old.ArtifactID || next[0].PublicRevision <= old.PublicRevision {
		t.Fatalf("restage=%+v", next)
	}
	body, err = os.ReadFile(filepath.Join(f.root, "generated", "units", next[0].ArtifactID, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"render_contract_version":"shared-specs-1"`) {
		t.Fatalf("manifest=%s", body)
	}
}

func TestNumericProfileChangeRejectsStagedFactsBeforeWorkerRuns(t *testing.T) {
	f := newNumericPublicationFixture(t)
	before := f.view(t)
	f.engine.Close()
	repository := &numericPublicationRepository{Store: f.store}
	engine, err := publishing.NewEngine(t.Context(), repository, publishing.Config{Root: filepath.Join(f.root, "generated"), BaseURL: "http://catalog.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	f.engine = engine
	c, err := f.store.SpecNormalizationContext(t.Context(), f.valueID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.SaveSpecValue(t.Context(), f.actor, f.productID, c.SpecID, "6–9V", "en-US", c.Expected.ProductRevision); err != nil {
		t.Fatal(err)
	}
	f.adopt(t)
	repository.beforeActivate = func() {
		definitions, err := f.store.ListSpecDefinitions(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		var revision int64
		for _, d := range definitions {
			if d.ID == c.SpecID {
				revision = d.Revision
			}
		}
		profile := c.Profile
		profile.DeclaredInputUnit = "V"
		if _, err = f.store.ConfigureSpecNumericProfile(t.Context(), f.actor, c.SpecID, revision, profile); err != nil {
			t.Fatal(err)
		}
	}
	// Older pending intents may be superseded first; stop immediately after the
	// actual staging/activation hook, before the next call can advance profile work.
	for n := 0; repository.beforeActivate != nil && n < 10; n++ {
		if _, err = engine.ProcessOne(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if repository.beforeActivate != nil {
		t.Fatal("candidate never staged")
	}
	got := f.view(t)
	if got.Revision != before.Revision || !reflect.DeepEqual(got.NumericFacts, before.NumericFacts) {
		t.Fatal("stale staged numeric profile activated before durable worker")
	}
	source, err := f.store.PublicationSource(t.Context(), f.productID)
	if err != nil {
		t.Fatal(err)
	}
	if len(source.NumericFacts) != 0 || len(source.NumericDefinitions) != 1 {
		t.Fatal("profile mismatch must invalidate working eligibility immediately")
	}
	f.drain(t)
	if len(f.view(t).NumericFacts) != 0 {
		t.Fatal("new profile revision retained stale numeric facts")
	}
}

func (r *numericPublicationRepository) ActivatePublication(ctx context.Context, intent publishing.Intent, activation publishing.ActivePublication, coordinator publishing.VisibilityCoordinator) (bool, error) {
	if r.beforeActivate != nil {
		hook := r.beforeActivate
		r.beforeActivate = nil
		hook()
	}
	return r.Store.ActivatePublication(ctx, intent, activation, coordinator)
}

func TestSharedSpecUnresolvedSourceRemainsPublishable(t *testing.T) {
	f := newNumericPublicationFixture(t)
	c, err := f.store.SpecNormalizationContext(t.Context(), f.valueID)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(f.root, "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`INSERT INTO product_spec_values(id,product_id,spec_id,raw_value,source_locale,source_origin_locale,source_revision,active,created_by,updated_by,created_at,updated_at)
 SELECT 'spv_legacy_public',product_id,spec_id,'6–8V','zh-TW','zh-TW',1,active,created_by,updated_by,created_at,updated_at FROM product_spec_values WHERE id=?`, f.valueID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`DELETE FROM product_spec_sources WHERE product_id=? AND spec_id=?`, f.productID, c.SpecID); err != nil {
		t.Fatal(err)
	}
	p, err := f.store.Product(t.Context(), f.productID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.UpdateProduct(t.Context(), f.actor, p.Revision, p); err != nil {
		t.Fatal(err)
	}
	f.drain(t)
	view := f.view(t)
	if len(view.Specifications) != 0 || len(view.NumericFacts) != 0 || len(view.NumericDefinitions) != 1 {
		t.Fatalf("unresolved source guessed or definition lost: %+v", view)
	}
	var count int
	if err = db.QueryRow(`SELECT COUNT(*) FROM product_spec_values WHERE product_id=?`, f.productID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatal("legacy Raw sources lost")
	}
	p, err = f.store.Product(t.Context(), f.productID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.SelectSpecSource(t.Context(), f.actor, catalog.SelectSpecSourceCommand{ProductID: f.productID, SpecID: c.SpecID, SpecValueID: "spv_legacy_public", ExpectedProductRevision: p.Revision, ExpectedSourceRevision: 1}); err != nil {
		t.Fatal(err)
	}
	f.drain(t)
	view = f.view(t)
	if len(view.Specifications) != 1 || view.Specifications[0].RawValue != "6–8V" || len(view.NumericFacts) != 0 {
		t.Fatalf("selected Raw without normalization: %+v", view.Specifications)
	}
	if !reflect.DeepEqual(view.Specifications, view.ForLocale("en-US").Specifications) {
		t.Fatal("locale selected structured Raw")
	}
}

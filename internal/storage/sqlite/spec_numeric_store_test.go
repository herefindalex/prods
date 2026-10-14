package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"prods/internal/specnumeric"
	"testing"

	"prods/internal/catalog"
)

type normalizationFixture struct {
	store *Store
	actor string
	value catalog.SpecValue
}

func newNormalizationFixture(t *testing.T) normalizationFixture {
	t.Helper()
	s, owner := installedStore(t)
	ctx := t.Context()
	category, err := s.CreateCategory(ctx, owner.ID, catalog.Category{Name: "Numeric", Slug: "numeric"})
	if err != nil {
		t.Fatal(err)
	}
	spec, err := s.CreateSpecDefinition(ctx, owner.ID, catalog.SpecDefinition{Name: "Voltage", Filterable: true})
	if err != nil {
		t.Fatal(err)
	}
	set, err := s.CreateSpecSet(ctx, owner.ID, catalog.SpecSet{Name: "Numeric", SpecIDs: []string{spec.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetCategorySpecSet(ctx, owner.ID, category.ID, set.ID, category.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConfigureSpecNumericProfile(ctx, owner.ID, spec.ID, spec.Revision, numericTestProfile()); err != nil {
		t.Fatal(err)
	}
	product, err := s.CreateProduct(ctx, owner.ID, catalog.Product{PartNumber: "NORMALIZATION", CategoryID: category.ID})
	if err != nil {
		t.Fatal(err)
	}
	value, err := s.SaveSpecValue(ctx, owner.ID, product.ID, spec.ID, "3–5V", "en-US", product.Revision)
	if err != nil {
		t.Fatal(err)
	}
	return normalizationFixture{s, owner.ID, value}
}

func (f normalizationFixture) snapshot(t *testing.T) catalog.SpecNormalizationContext {
	t.Helper()
	c, err := f.store.SpecNormalizationContext(t.Context(), f.value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c.SourceLocale == "" {
		t.Fatal("missing authoritative Raw origin")
	}
	return c
}

func (f normalizationFixture) adopt(t *testing.T) catalog.NormalizationOutcome {
	t.Helper()
	c := f.snapshot(t)
	parsed := specnumeric.Normalize(c.Raw, c.Profile)
	if parsed.Candidate == nil || parsed.Reason != "" {
		t.Fatalf("fixture parse: %+v", parsed)
	}
	encoded, err := specnumeric.MarshalValue(*parsed.Candidate)
	if err != nil {
		t.Fatal(err)
	}
	out, err := f.store.AdoptManualSpecValue(t.Context(), f.actor, catalog.AdoptSpecCommand{SpecValueID: c.SpecValueID, Expected: c.Expected, ValueJSON: encoded})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestStaleManualPreferencePreventsAutomaticAdoption(t *testing.T) {
	f := newNormalizationFixture(t)
	manual := f.adopt(t)
	c := f.snapshot(t)
	if _, err := f.store.SaveSpecValue(t.Context(), f.actor, c.ProductID, c.SpecID, "4–6V", "zh-TW", c.Expected.ProductRevision); err != nil {
		t.Fatal(err)
	}
	c = f.snapshot(t)
	out, err := f.store.RecordSpecNormalization(t.Context(), f.actor, catalog.NormalizeSpecCommand{SpecValueID: c.SpecValueID, Expected: c.Expected}, specnumeric.Normalize(c.Raw, c.Profile))
	if err != nil {
		t.Fatal(err)
	}
	if out.State != "candidate" || out.PublicationPending || out.ProductRevision != c.Expected.ProductRevision {
		t.Fatalf("candidate outcome: %+v", out)
	}
	selection := f.snapshot(t).Selection
	if selection.Preference != catalog.NormalizedManual || selection.SelectedNormalizedValueID != manual.NormalizedValueID || selection.Revision != c.Expected.SelectionRevision {
		t.Fatalf("manual replaced: %+v", selection)
	}
	var status string
	if err := f.store.db.QueryRow(`SELECT status FROM normalized_values WHERE id=?`, manual.NormalizedValueID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "stale" {
		t.Fatalf("manual status=%s", status)
	}
}

func TestExplicitAutomaticSelectionAdoptsCurrentCandidate(t *testing.T) {
	f := newNormalizationFixture(t)
	f.adopt(t)
	c := f.snapshot(t)
	candidate, err := f.store.RecordSpecNormalization(t.Context(), f.actor, catalog.NormalizeSpecCommand{SpecValueID: c.SpecValueID, Expected: c.Expected}, specnumeric.Normalize(c.Raw, c.Profile))
	if err != nil {
		t.Fatal(err)
	}
	var manualStatus string
	if err := f.store.db.QueryRow(`SELECT status FROM normalized_values WHERE id=?`, c.Selection.SelectedNormalizedValueID).Scan(&manualStatus); err != nil {
		t.Fatal(err)
	}
	if manualStatus != "current" {
		t.Fatalf("candidate staled unchanged manual: %s", manualStatus)
	}
	repeated, err := f.store.RecordSpecNormalization(t.Context(), f.actor, catalog.NormalizeSpecCommand{SpecValueID: c.SpecValueID, Expected: c.Expected}, specnumeric.Normalize(c.Raw, c.Profile))
	if err != nil {
		t.Fatal(err)
	}
	if repeated.NormalizedValueID != candidate.NormalizedValueID || repeated.ProductRevision != candidate.ProductRevision {
		t.Fatalf("identical candidate rerun churned: %+v", repeated)
	}
	out, err := f.store.SelectAutomaticSpecValue(t.Context(), f.actor, catalog.SelectAutomaticSpecCommand{SpecValueID: c.SpecValueID, Expected: c.Expected, CandidateID: candidate.NormalizedValueID})
	if err != nil {
		t.Fatal(err)
	}
	if out.State != "adopted" || !out.PublicationPending || out.ProductRevision != c.Expected.ProductRevision+1 || out.SelectionRevision != c.Expected.SelectionRevision+1 {
		t.Fatalf("adoption: %+v", out)
	}
	selection := f.snapshot(t).Selection
	if selection.Preference != catalog.NormalizedAutomatic || selection.SelectedNormalizedValueID != candidate.NormalizedValueID {
		t.Fatalf("selection: %+v", selection)
	}
	var audits, intents int
	if err := f.store.db.QueryRow(`SELECT COUNT(*) FROM admin_log WHERE action='product.spec_automatic_selected' AND target_id=?`, c.ProductID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if err := f.store.db.QueryRow(`SELECT COUNT(*) FROM publication_intents WHERE entity_type='product' AND entity_id=? AND desired_revision=?`, c.ProductID, out.ProductRevision).Scan(&intents); err != nil {
		t.Fatal(err)
	}
	if audits != 1 || intents != 1 {
		t.Fatalf("durable evidence: audits=%d intents=%d", audits, intents)
	}
	history, err := f.store.ProductSpecValues(t.Context(), c.ProductID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, detail := range history {
		for _, normalized := range detail.Normalized {
			if normalized.ID == candidate.NormalizedValueID {
				found = true
				if normalized.ProfileRevision != c.Expected.ProfileRevision || normalized.SourceOriginLocale != c.SourceLocale || normalized.NumericSchemaVersion != specnumeric.SchemaVersion {
					t.Fatalf("controlled history metadata: %+v", normalized)
				}
			}
		}
	}
	if !found {
		t.Fatal("adopted candidate missing from history")
	}
	current := f.snapshot(t)
	replayed, err := f.store.RecordSpecNormalization(t.Context(), f.actor, catalog.NormalizeSpecCommand{SpecValueID: current.SpecValueID, Expected: current.Expected}, specnumeric.Normalize(current.Raw, current.Profile))
	if err != nil {
		t.Fatal(err)
	}
	if replayed.State != "recorded" || replayed.NormalizedValueID != candidate.NormalizedValueID || replayed.ProductRevision != out.ProductRevision || replayed.SelectionRevision != out.SelectionRevision || replayed.PublicationPending {
		t.Fatalf("identical adopted rerun churned: %+v", replayed)
	}
}

func TestManualSelectionRejectsStaleSelectionRevision(t *testing.T) {
	f := newNormalizationFixture(t)
	f.adopt(t)
	c := f.snapshot(t)
	var encoded string
	if err := f.store.db.QueryRow(`SELECT value_json FROM normalized_values WHERE id=?`, c.Selection.SelectedNormalizedValueID).Scan(&encoded); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.db.Exec(`UPDATE spec_normalization_selections SET revision=revision+1 WHERE spec_value_id=?`, c.SpecValueID); err != nil {
		t.Fatal(err)
	}
	_, err := f.store.AdoptManualSpecValue(t.Context(), f.actor, catalog.AdoptSpecCommand{SpecValueID: c.SpecValueID, Expected: c.Expected, ValueJSON: encoded})
	if !errors.Is(err, catalog.ErrRevisionConflict) {
		t.Fatalf("selection CAS: %v", err)
	}
}

func TestManualAdoptionRejectsChangedProfileBeforeDecode(t *testing.T) {
	f := newNormalizationFixture(t)
	c := f.snapshot(t)
	parsed := specnumeric.Normalize(c.Raw, c.Profile)
	if parsed.Candidate == nil || parsed.Reason != "" {
		t.Fatalf("fixture parse: %+v", parsed)
	}
	encoded, err := specnumeric.MarshalValue(*parsed.Candidate)
	if err != nil {
		t.Fatal(err)
	}
	command := catalog.AdoptSpecCommand{SpecValueID: c.SpecValueID, Expected: c.Expected, ValueJSON: encoded}
	var specRevision int64
	if err := f.store.db.QueryRow(`SELECT revision FROM spec_definitions WHERE id=?`, c.SpecID).Scan(&specRevision); err != nil {
		t.Fatal(err)
	}
	profile := c.Profile
	profile.AllowedShapes = []specnumeric.Shape{specnumeric.ShapeScalar}
	if _, err := f.store.ConfigureSpecNumericProfile(t.Context(), f.actor, c.SpecID, specRevision, profile); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.AdoptManualSpecValue(t.Context(), f.actor, command); !errors.Is(err, catalog.ErrRevisionConflict) {
		t.Fatalf("stale manual profile: %v", err)
	}
	var history, selection int
	if err := f.store.db.QueryRow(`SELECT COUNT(*) FROM normalized_values WHERE spec_value_id=?`, c.SpecValueID).Scan(&history); err != nil {
		t.Fatal(err)
	}
	if err := f.store.db.QueryRow(`SELECT COUNT(*) FROM spec_normalization_selections WHERE spec_value_id=?`, c.SpecValueID).Scan(&selection); err != nil {
		t.Fatal(err)
	}
	if history != 0 || selection != 0 {
		t.Fatalf("stale manual adopted: history=%d selection=%d", history, selection)
	}
}

func TestNormalizationRejectsChangedRawSource(t *testing.T) {
	f := newNormalizationFixture(t)
	c := f.snapshot(t)
	parsed := specnumeric.Normalize(c.Raw, c.Profile)
	encoded, err := specnumeric.MarshalValue(*parsed.Candidate)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.SaveSpecValue(t.Context(), f.actor, c.ProductID, c.SpecID, "4–6V", "en-US", c.Expected.ProductRevision); err != nil {
		t.Fatal(err)
	}
	// A change before the storage context read must retain 409 conflict
	// precedence over candidate validation against the new Raw/profile.
	if _, err := f.store.RecordSpecNormalization(t.Context(), f.actor, catalog.NormalizeSpecCommand{SpecValueID: c.SpecValueID, Expected: c.Expected}, parsed); !errors.Is(err, catalog.ErrRevisionConflict) {
		t.Fatalf("old parsed context conflict precedence: %v", err)
	}
	// Supply the current Product revision so only the stale Raw snapshot/revision
	// can reject this already-parsed result.
	c.Expected.ProductRevision++
	_, err = f.store.recordControlledNormalization(t.Context(), f.actor, c, c.Expected, encoded, "", catalog.NormalizedAutomatic, false)
	if !errors.Is(err, catalog.ErrRevisionConflict) {
		t.Fatalf("parsed old Raw committed: %v", err)
	}
	var count int
	if err := f.store.db.QueryRow(`SELECT COUNT(*) FROM normalized_values WHERE spec_value_id=?`, c.SpecValueID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("old candidate persisted: %d", count)
	}
}

func TestSharedSpecRawDoesNotCreateLocaleVariant(t *testing.T) {
	f := newNormalizationFixture(t)
	c := f.snapshot(t)
	v, err := f.store.SaveSpecValue(t.Context(), f.actor, c.ProductID, c.SpecID, "4–6V", "zh-TW", c.Expected.ProductRevision)
	if err != nil {
		t.Fatal(err)
	}
	if v.ID != f.value.ID || v.SourceRevision != 2 {
		t.Fatalf("locale variant: %+v", v)
	}
	c = f.snapshot(t)
	if c.Raw != "4–6V" || c.SourceLocale != "zh-TW" || c.SourceSelection.SpecValueID != v.ID {
		t.Fatalf("shared source: %+v", c)
	}
}

func TestLegacySharedSpecSourceRequiresExplicitChoice(t *testing.T) {
	f := newNormalizationFixture(t)
	c := f.snapshot(t)
	if _, err := f.store.db.Exec(`DELETE FROM product_spec_sources WHERE product_id=? AND spec_id=?`, c.ProductID, c.SpecID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.db.Exec(`INSERT INTO product_spec_values(id,product_id,spec_id,raw_value,source_locale,source_origin_locale,source_revision,active,created_by,updated_by,created_at,updated_at) SELECT 'spv_legacy_choice',product_id,spec_id,'6–8V','zh-TW','zh-TW',1,active,created_by,updated_by,created_at,updated_at FROM product_spec_values WHERE id=?`, c.SpecValueID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.SaveSpecValue(t.Context(), f.actor, c.ProductID, c.SpecID, "9–10V", "en-US", c.Expected.ProductRevision); !errors.Is(err, catalog.ErrRevisionConflict) {
		t.Fatalf("ambiguous source guessed: %v", err)
	}
	// General source selection also works without a numeric profile.
	if _, err := f.store.db.Exec(`DELETE FROM spec_numeric_profiles WHERE spec_id=?`, c.SpecID); err != nil {
		t.Fatal(err)
	}
	selected, err := f.store.SelectSpecSource(context.Background(), f.actor, catalog.SelectSpecSourceCommand{ProductID: c.ProductID, SpecID: c.SpecID, SpecValueID: "spv_legacy_choice", ExpectedProductRevision: c.Expected.ProductRevision, ExpectedSourceRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	if selected.SpecValueID != "spv_legacy_choice" || selected.Revision != 1 {
		t.Fatalf("source selection: %+v", selected)
	}
	saved, err := f.store.SaveSpecValue(t.Context(), f.actor, c.ProductID, c.SpecID, "9–10V", "de-DE", c.Expected.ProductRevision+1)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID != selected.SpecValueID || saved.SourceRevision != 2 || saved.SourceLocale != "de-DE" {
		t.Fatalf("selected shared Raw update: %+v", saved)
	}
	var count int
	if err := f.store.db.QueryRow(`SELECT COUNT(*) FROM product_spec_values WHERE product_id=? AND spec_id=?`, c.ProductID, c.SpecID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("legacy sources lost: %d", count)
	}
}

func TestCloneSpecSourcePreservesExplicitSharedChoice(t *testing.T) {
	f := newNormalizationFixture(t)
	manual := f.adopt(t)
	c := f.snapshot(t)
	if _, err := f.store.db.Exec(`INSERT INTO product_spec_values(id,product_id,spec_id,raw_value,source_locale,source_origin_locale,source_revision,active,created_by,updated_by,created_at,updated_at)
	 SELECT 'spv_clone_source',product_id,spec_id,'6–8V','zh-TW','ja-JP',1,active,created_by,updated_by,created_at,updated_at FROM product_spec_values WHERE id=?`, c.SpecValueID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.SelectSpecSource(t.Context(), f.actor, catalog.SelectSpecSourceCommand{ProductID: c.ProductID, SpecID: c.SpecID, SpecValueID: "spv_clone_source", ExpectedProductRevision: c.Expected.ProductRevision, ExpectedSourceRevision: 1, ExpectedSourceSelectionRevision: c.SourceSelection.Revision}); err != nil {
		t.Fatal(err)
	}
	clone, err := f.store.CloneProduct(t.Context(), f.actor, c.ProductID, c.Expected.ProductRevision+1, catalog.Product{PartNumber: "CLONE-SHARED"})
	if err != nil {
		t.Fatal(err)
	}
	var raw, origin, id string
	if err := f.store.db.QueryRow(`SELECT pv.id,pv.raw_value,pv.source_origin_locale FROM product_spec_sources ps JOIN product_spec_values pv ON pv.id=ps.spec_value_id WHERE ps.product_id=? AND ps.spec_id=?`, clone.ID, c.SpecID).Scan(&id, &raw, &origin); err != nil {
		t.Fatal(err)
	}
	if id == "spv_clone_source" || raw != "6–8V" || origin != "ja-JP" {
		t.Fatalf("clone shared choice: %q %q %q", id, raw, origin)
	}
	var rawCount, selectionCount, historyCount int
	if err := f.store.db.QueryRow(`SELECT COUNT(*) FROM product_spec_values WHERE product_id=?`, clone.ID).Scan(&rawCount); err != nil {
		t.Fatal(err)
	}
	if err := f.store.db.QueryRow(`SELECT COUNT(*) FROM spec_normalization_selections ns JOIN product_spec_values pv ON pv.id=ns.spec_value_id WHERE pv.product_id=?`, clone.ID).Scan(&selectionCount); err != nil {
		t.Fatal(err)
	}
	if err := f.store.db.QueryRow(`SELECT COUNT(*) FROM normalized_values WHERE id=?`, manual.NormalizedValueID).Scan(&historyCount); err != nil {
		t.Fatal(err)
	}
	if rawCount != 2 || selectionCount != 0 || historyCount != 1 {
		t.Fatalf("clone preserved scope: raw=%d adoption=%d original_history=%d", rawCount, selectionCount, historyCount)
	}
}

// removeNumericSchema reconstructs the pre-19 shape for upgrade fixtures.
func removeNumericSchema(t *testing.T, store *Store) {
	t.Helper()
	if CurrentSchemaVersion < 19 {
		return
	}
	_, err := store.db.Exec(`
DROP TABLE public_spec_projection_fields;
DROP TABLE public_spec_projection_products;
DROP TABLE spec_numeric_profile_work;
DROP TABLE spec_normalization_selections;
DROP TABLE product_spec_sources;
DROP TABLE spec_numeric_profiles;
ALTER TABLE product_spec_values DROP COLUMN source_origin_locale;
ALTER TABLE normalized_values DROP COLUMN numeric_schema_version;
ALTER TABLE normalized_values DROP COLUMN source_origin_locale;
ALTER TABLE normalized_values DROP COLUMN profile_revision;
DELETE FROM schema_migrations WHERE version=19;
UPDATE system_state SET schema_version=18 WHERE singleton=1;`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestNumericMigrationPreservesLegacyRawAndHistory(t *testing.T) {
	store, owner := installedStore(t)
	ctx := t.Context()
	spec, err := store.CreateSpecDefinition(ctx, owner.ID, catalog.SpecDefinition{Name: "Voltage", PreferredUnit: "V"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateSpecDefinition(ctx, owner.ID, catalog.SpecDefinition{Name: "Legacy shared source"})
	if err != nil {
		t.Fatal(err)
	}
	product, err := store.CreateProduct(ctx, owner.ID, catalog.Product{PartNumber: "NUMERIC-LEGACY"})
	if err != nil {
		t.Fatal(err)
	}
	value, err := store.SaveSpecValue(ctx, owner.ID, product.ID, spec.ID, " 3.0–3.6V ", "en-US", 1)
	if err != nil {
		t.Fatal(err)
	}
	legacyJSON := `{"unit":"V","min":3.0,"max":3.6}`
	history, err := store.SaveNormalizedValue(ctx, owner.ID, catalog.NormalizedValue{SpecValueID: value.ID, ValueJSON: legacyJSON,
		SourceRevision: 1, SpecSemanticVersion: 1, NormalizerVersion: "manual-v1", Source: catalog.NormalizedManual, Status: catalog.NormalizedCurrent})
	if err != nil {
		t.Fatal(err)
	}
	// Reconstruct the pre-M19 locale-keyed writes directly: current writes share Raw.
	old, err := store.SaveSpecValue(ctx, owner.ID, product.ID, other.ID, "legacy en-US", "en-US", 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO product_spec_values(id,product_id,spec_id,raw_value,source_locale,source_origin_locale,source_revision,active,created_by,updated_by,created_at,updated_at)
	 SELECT 'spv_legacy_variant',product_id,spec_id,'legacy zh-TW','zh-TW','zh-TW',source_revision,active,created_by,updated_by,created_at,updated_at FROM product_spec_values WHERE id=?`, old.ID); err != nil {
		t.Fatal(err)
	}
	removeNumericSchema(t, store)
	if err := store.Upgrade(ctx, "verified-numeric-v18-backup"); err != nil {
		t.Fatal(err)
	}
	var raw, encoded, status, origin string
	if err := store.db.QueryRow(`SELECT raw_value,source_origin_locale FROM product_spec_values WHERE id=?`, value.ID).Scan(&raw, &origin); err != nil {
		t.Fatal(err)
	}
	if raw != value.RawValue || origin != "en-US" {
		t.Fatalf("raw/provenance changed: %q %q", raw, origin)
	}
	if err := store.db.QueryRow(`SELECT value_json,status FROM normalized_values WHERE id=?`, history.ID).Scan(&encoded, &status); err != nil {
		t.Fatal(err)
	}
	if encoded != legacyJSON || status != "current" {
		t.Fatalf("legacy history changed: %q %q", encoded, status)
	}
	var profileRevision, schemaVersion int
	if err := store.db.QueryRow(`SELECT profile_revision,numeric_schema_version FROM normalized_values WHERE id=?`, history.ID).Scan(&profileRevision, &schemaVersion); err != nil {
		t.Fatal(err)
	}
	if profileRevision != 0 || schemaVersion != 0 {
		t.Fatalf("legacy history marked trusted: profile=%d schema=%d", profileRevision, schemaVersion)
	}
	var adopted, sources, preserved int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM spec_normalization_selections`).Scan(&adopted); err != nil {
		t.Fatal(err)
	}
	if adopted != 0 {
		t.Fatalf("legacy values automatically adopted: %d", adopted)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM product_spec_sources WHERE product_id=?`, product.ID).Scan(&sources); err != nil {
		t.Fatal(err)
	}
	if sources != 1 {
		t.Fatalf("mapped sources=%d, want sole source only", sources)
	}
	var mapped string
	if err := store.db.QueryRow(`SELECT spec_value_id FROM product_spec_sources WHERE product_id=? AND spec_id=?`, product.ID, spec.ID).Scan(&mapped); err != nil {
		t.Fatal(err)
	}
	if mapped != value.ID {
		t.Fatalf("mapped source=%q", mapped)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM product_spec_values WHERE product_id=? AND spec_id=?`, product.ID, other.ID).Scan(&preserved); err != nil {
		t.Fatal(err)
	}
	if preserved != 2 {
		t.Fatalf("ambiguous legacy sources lost: %d", preserved)
	}
}

func TestNumericFreshInstallHasSameSchema(t *testing.T) {
	store, err := Create(filepath.Join(t.TempDir(), "fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var version, tables int
	if err := store.db.QueryRow(`SELECT schema_version FROM system_state`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 19 {
		t.Fatalf("fresh schema=%d, want 19", version)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN
		('spec_numeric_profiles','spec_normalization_selections','product_spec_sources','spec_numeric_profile_work','public_spec_projection_products','public_spec_projection_fields')`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 6 {
		t.Fatalf("numeric tables=%d, want 6", tables)
	}
	if _, err := store.db.Exec(`SELECT source_origin_locale FROM product_spec_values LIMIT 1`); err != nil {
		t.Fatal(err)
	}
}

func numericTestProfile() specnumeric.Profile {
	return specnumeric.Profile{Quantity: "voltage", SemanticRole: specnumeric.RoleOperatingSupport,
		AllowedShapes: []specnumeric.Shape{specnumeric.ShapeIntervalSet}, AllowedOperators: []specnumeric.Operator{specnumeric.OperatorContainsPoint},
		CanonicalUnit: "V", NumericFormat: specnumeric.NumericFormatDecimalDot, SchemaVersion: specnumeric.SchemaVersion,
		ParserVersion: specnumeric.ParserVersion, UnitRegistryVersion: specnumeric.UnitRegistryVersion}
}

func TestNumericProfileRejectsStaleRevision(t *testing.T) {
	store, owner := installedStore(t)
	spec, err := store.CreateSpecDefinition(t.Context(), owner.ID, catalog.SpecDefinition{Name: "Voltage"})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := store.ConfigureSpecNumericProfile(t.Context(), owner.ID, spec.ID, spec.Revision, numericTestProfile())
	if err != nil {
		t.Fatal(err)
	}
	if profile.SpecSemanticVersion != 2 || profile.Revision != 1 {
		t.Fatalf("profile=%+v", profile)
	}
	if _, err := store.ConfigureSpecNumericProfile(t.Context(), owner.ID, spec.ID, spec.Revision, numericTestProfile()); !errors.Is(err, catalog.ErrRevisionConflict) {
		t.Fatalf("stale configure=%v", err)
	}
	var work, audit int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM spec_numeric_profile_work WHERE spec_id=?`, spec.ID).Scan(&work); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM admin_log WHERE action='spec.numeric_profile_configured' AND target_id=?`, spec.ID).Scan(&audit); err != nil {
		t.Fatal(err)
	}
	if work != 1 || audit != 1 {
		t.Fatalf("durable work=%d audit=%d", work, audit)
	}
}

func TestNumericProfileCannotChangeSemanticIdentity(t *testing.T) {
	store, owner := installedStore(t)
	spec, err := store.CreateSpecDefinition(t.Context(), owner.ID, catalog.SpecDefinition{Name: "Voltage"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ConfigureSpecNumericProfile(t.Context(), owner.ID, spec.ID, spec.Revision, numericTestProfile()); err != nil {
		t.Fatal(err)
	}
	changed := numericTestProfile()
	changed.SemanticRole = specnumeric.RoleNominal
	if _, err := store.ConfigureSpecNumericProfile(t.Context(), owner.ID, spec.ID, spec.Revision+1, changed); !errors.Is(err, catalog.ErrInvalidSpec) {
		t.Fatalf("semantic identity change=%v", err)
	}
}

func TestNumericContextFreezesRawAndExplicitRevisions(t *testing.T) {
	store, owner := installedStore(t)
	spec, err := store.CreateSpecDefinition(t.Context(), owner.ID, catalog.SpecDefinition{Name: "Voltage", PreferredUnit: "mV"})
	if err != nil {
		t.Fatal(err)
	}
	product, err := store.CreateProduct(t.Context(), owner.ID, catalog.Product{PartNumber: "NUMERIC-CONTEXT"})
	if err != nil {
		t.Fatal(err)
	}
	value, err := store.SaveSpecValue(t.Context(), owner.ID, product.ID, spec.ID, " 3.0–3.6 V ", "zh-TW", product.Revision)
	if err != nil {
		t.Fatal(err)
	}
	// Until Task 5 adapts Raw Save, seed explicit provenance without asking
	// Context to infer it from the immutable legacy locale partition.
	if _, err := store.db.Exec(`UPDATE product_spec_values SET source_origin_locale='zh-TW' WHERE id=?`, value.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ConfigureSpecNumericProfile(t.Context(), owner.ID, spec.ID, spec.Revision, numericTestProfile()); err != nil {
		t.Fatal(err)
	}
	context, err := store.SpecNormalizationContext(t.Context(), value.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := catalog.NormalizationExpectation{ProductRevision: 2, SourceRevision: 1, SpecSemanticVersion: 2, ProfileRevision: 1}
	if context.Expected != want || context.Raw != value.RawValue || context.SourceLocale != "zh-TW" || context.Profile.DeclaredInputUnit != "" {
		t.Fatalf("context did not freeze authoritative inputs: %+v", context)
	}
}

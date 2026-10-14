package publishing

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/blevesearch/bleve/v2/index/scorch"
	"prods/internal/specnumeric"
)

func TestBleveRejectsSymlinkOwnershipTargetBeforeExternalLock(t *testing.T) {
	root := filepath.Join(t.TempDir(), "search", "specs")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(external, []byte("external"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, "ownership")); err != nil {
		t.Fatal(err)
	}
	b, err := newBleveSpecIndex(t.Context(), root, Config{})
	if b != nil {
		_ = b.close()
	}
	if err == nil {
		t.Fatal("symlink ownership target was admitted")
	}
	if _, err := os.Lstat(external + ".lock"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("external ownership lock was created: %v", err)
	}
}

func bleveDecimal(t *testing.T, text string) specnumeric.Decimal {
	t.Helper()
	d, err := specnumeric.ParseDecimal(text)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestSpecBleveKeyPreservesExactDecimalOrder(t *testing.T) {
	values := []string{"-999999999999999999999999.9999999999", "-1.000000000000000000000000000000001", "-1", "-0.000000000000000000000001", "0", "0.000000000000000000000001", "1", "1.000000000000000000000000000000001"}
	previous := ""
	for _, value := range values {
		key, err := specBleveDecimalKey(bleveDecimal(t, value))
		if err != nil {
			t.Fatal(err)
		}
		if len(key) != 83 || (previous != "" && previous >= key) {
			t.Fatalf("key order for %s: %q >= %q", value, previous, key)
		}
		previous = key
	}
}

func blevePublication(t *testing.T) ActivePublication {
	t.Helper()
	p := specnumeric.Profile{Quantity: "voltage", SemanticRole: specnumeric.RoleOperatingSupport, AllowedShapes: []specnumeric.Shape{specnumeric.ShapeIntervalSet}, AllowedOperators: []specnumeric.Operator{specnumeric.OperatorContainsPoint}, CanonicalUnit: "V", DeclaredInputUnit: "V", NumericFormat: specnumeric.NumericFormatDecimalDot, SchemaVersion: specnumeric.SchemaVersion, ParserVersion: specnumeric.ParserVersion, UnitRegistryVersion: specnumeric.UnitRegistryVersion}
	v := specnumeric.Value{SchemaVersion: p.SchemaVersion, Quantity: p.Quantity, SemanticRole: p.SemanticRole, Shape: specnumeric.ShapeIntervalSet, Unit: "V", UnitRegistryVersion: p.UnitRegistryVersion, Context: specnumeric.Context{Kind: specnumeric.ContextUnconditional}, Intervals: []specnumeric.Interval{{Lower: bleveDecimal(t, "3"), Upper: bleveDecimal(t, "3.6"), LowerInclusive: true, UpperInclusive: false}, {Lower: bleveDecimal(t, "4.5"), Upper: bleveDecimal(t, "5.5"), LowerInclusive: true, UpperInclusive: true}}}
	return ActivePublication{ProductID: "opaque-product", SourceRevision: 9007199254740993, PublicRevision: 7, SiteEpoch: 2, VisibilityGeneration: 9, View: PublicView{ID: "opaque-product", NumericDefinitions: []PublicNumericDefinition{{SpecID: "voltage/opaque", SpecSemanticVersion: 1, Profile: p}}, NumericFacts: []PublicNumericFact{{SpecID: "voltage/opaque", SpecSemanticVersion: 1, Profile: p, Value: v}}}}
}

func TestBleveCoarseEnvelopeRetainsGapPoint(t *testing.T) {
	a := blevePublication(t)
	docs, _, err := specBleveDocuments([]ActivePublication{a})
	if err != nil {
		t.Fatal(err)
	}
	q := bleveDecimal(t, "4")
	predicate := specnumeric.Predicate{SpecID: "voltage/opaque", Operator: specnumeric.OperatorContainsPoint, Unit: "V", Value: &q}
	plans := specBlevePlans(docs, []specnumeric.Predicate{predicate})
	if !specBleveCoarseMatch(docs[0], plans) {
		t.Fatal("coarse envelope dropped gap point")
	}
	if got := specnumeric.Evaluate(&a.View.NumericFacts[0].Value, a.View.NumericFacts[0].Profile, predicate); got != specnumeric.NoMatch {
		t.Fatalf("exact oracle = %s", got)
	}
}

func TestBlevePersistsAndRejectsMissingCandidateDocument(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "search", "specs")
	b, err := newBleveSpecIndex(ctx, root, Config{})
	if err != nil {
		t.Fatal(err)
	}
	a := blevePublication(t)
	if err := b.replace(ctx, []ActivePublication{a}); err != nil {
		t.Fatal(err)
	}
	advanced, err := b.index.Advanced()
	if err != nil {
		t.Fatal(err)
	}
	scorchIndex := advanced.(*scorch.Scorch)
	batchCount := scorchIndex.StatsMap()["TotBatches"].(uint64)
	if err := b.rebuild(ctx, []ActivePublication{a}); err != nil {
		t.Fatal(err)
	}
	if scorchIndex.StatsMap()["TotBatches"].(uint64) <= batchCount {
		t.Fatal("forced rebuild skipped the real Scorch Batch")
	}
	q := bleveDecimal(t, "4")
	request := SpecSearchRequest{Predicates: []specnumeric.Predicate{{SpecID: "voltage/opaque", Operator: specnumeric.OperatorContainsPoint, Unit: "V", Value: &q}}}
	before, err := b.candidates(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Candidates) != 1 || before.SourceDigest == "" || before.Inventory[0].SourceRevision != a.SourceRevision {
		t.Fatalf("snapshot = %#v", before)
	}
	if err := b.close(); err != nil {
		t.Fatal(err)
	}
	b, err = newBleveSpecIndex(ctx, root, Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.close() })
	after, err := b.candidates(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) || b.digest() != before.SourceDigest {
		t.Fatal("reopen changed coherent snapshot")
	}
	if err := b.index.Delete(specBleveProductDocID(a.ProductID)); err != nil {
		t.Fatal(err)
	}
	if _, err := b.candidates(ctx, request); err == nil {
		t.Fatal("missing matching document returned successful narrowing")
	}
	// Keep DocCount unchanged to distinguish the ALL-hit proof from a count
	// check: an unrelated index document cannot stand in for the missing hit.
	if err := b.index.Index("unexpected", map[string]any{"product": "yes"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.candidates(ctx, request); err == nil || !strings.Contains(err.Error(), "hits disagree") {
		t.Fatalf("missing hit with unchanged document count = %v", err)
	}
}

package referencecatalog

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestBuildIsDeterministicAndMatchesA01Baseline(t *testing.T) {
	first, firstManifest, err := Build(DefaultSeed)
	if err != nil {
		t.Fatal(err)
	}
	second, secondManifest, err := Build(DefaultSeed)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || !reflect.DeepEqual(firstManifest, secondManifest) {
		t.Fatal("same version and seed produced different logical datasets")
	}
	if firstManifest.Summary.CategoriesIncludingSystem != 60 || firstManifest.Summary.BusinessTopCategories != 9 {
		t.Fatalf("taxonomy summary = %+v", firstManifest.Summary)
	}
	if firstManifest.Summary.Products != 1200 || firstManifest.Summary.SpecSets != 24 {
		t.Fatalf("dataset summary = %+v", firstManifest.Summary)
	}
	if firstManifest.Summary.Published != 1020 || firstManifest.Summary.Hidden != 140 || firstManifest.Summary.Archived != 40 {
		t.Fatalf("state distribution = %+v", firstManifest.Summary)
	}
	encoded, err := json.Marshal(firstManifest)
	if err != nil || len(encoded) == 0 {
		t.Fatalf("manifest JSON err=%v bytes=%d", err, len(encoded))
	}
}

func TestManifestCoversDenseSparseEmptyCommonAndQueryCases(t *testing.T) {
	_, manifest, err := Build(DefaultSeed)
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]TaxonomyExpectation, len(manifest.Taxonomy))
	for _, item := range manifest.Taxonomy {
		byID[item.ID] = item
	}
	if byID[categoryID("semi-ldo-auto")].DirectPublic < 150 {
		t.Fatalf("dense leaf public count = %d", byID[categoryID("semi-ldo-auto")].DirectPublic)
	}
	if byID[categoryID("sensor-gyroscopes")].DirectPublic > 1 {
		t.Fatalf("sparse leaf public count = %d", byID[categoryID("sensor-gyroscopes")].DirectPublic)
	}
	if byID[categoryID("empty-evaluation")].SubtreePublic != 0 {
		t.Fatalf("empty category subtree count = %d", byID[categoryID("empty-evaluation")].SubtreePublic)
	}
	if len(manifest.CommonScopes) != 3 || len(manifest.CommonScopes[2].SpecIDs) != 0 {
		t.Fatalf("common expectations = %+v", manifest.CommonScopes)
	}
	queries := make(map[string]QueryExpectation, len(manifest.Queries))
	for _, query := range manifest.Queries {
		queries[query.Name] = query
	}
	if queries["literal-wildcards"].Count != 1 || queries["unicode-casefold"].Count != 1 || queries["certain-zero"].Count != 0 {
		t.Fatalf("query expectations = %+v", queries)
	}
}

package distribution

import (
	"errors"
	"testing"
)

func TestEmbeddedSampleMatchesCompiledRelease(t *testing.T) {
	sample, err := LoadEmbeddedSample("v0.6.8")
	if err != nil {
		t.Fatal(err)
	}
	if sample.Data.SchemaVersion != 1 || sample.Data.ReleaseVersion != "v0.6.8" || sample.Data.DatasetVersion == "" {
		t.Fatalf("embedded sample metadata = %+v", sample.Data)
	}
	if len(sample.Data.Dictionaries) != 64 || len(sample.Data.Categories) != 58 || len(sample.Data.Specs) != 34 ||
		len(sample.Data.SpecSets) != 24 || len(sample.Data.CategorySpecSets) != 58 || len(sample.Data.Products) != 1200 {
		t.Fatalf("embedded sample dimensions: dictionaries=%d categories=%d specs=%d sets=%d assignments=%d products=%d",
			len(sample.Data.Dictionaries), len(sample.Data.Categories), len(sample.Data.Specs), len(sample.Data.SpecSets),
			len(sample.Data.CategorySpecSets), len(sample.Data.Products))
	}
	values := 0
	for _, product := range sample.Data.Products {
		values += len(product.SpecValues)
	}
	if values != 4834 {
		t.Fatalf("embedded sample specification values=%d want=4834", values)
	}
	if len(sample.SHA256) != 64 || sample.SourceURL != "embedded:sample-data/prods-sample-data-v1.json" {
		t.Fatalf("embedded sample evidence = %+v", sample)
	}
}

func TestEmbeddedSampleRejectsDifferentBinaryVersion(t *testing.T) {
	if _, err := LoadEmbeddedSample("v0.6.9"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("mismatched binary version error=%v", err)
	}
	if EmbeddedSampleAvailable("v0.6.9") {
		t.Fatal("mismatched binary version reported embedded sample available")
	}
}

package catalog

import (
	"errors"
	"testing"
)

func TestProductDocumentRequiresExactlyOneSafeSource(t *testing.T) {
	valid := ProductDocument{ID: "doc_1", ProductID: "prd_1", Label: "Datasheet", DocumentTypeID: "dt_datasheet", ExternalURL: "https://example.test/data.pdf"}
	if err := valid.Prepare(); err != nil {
		t.Fatal(err)
	}
	for _, document := range []ProductDocument{
		{ID: "none", ProductID: "prd_1", Label: "None", DocumentTypeID: "dt_datasheet"},
		{ID: "both", ProductID: "prd_1", Label: "Both", DocumentTypeID: "dt_datasheet", AssetID: "ast_1", ExternalURL: "https://example.test/data.pdf"},
		{ID: "script", ProductID: "prd_1", Label: "Bad", DocumentTypeID: "dt_datasheet", ExternalURL: "javascript:alert(1)"},
		{ID: "credentials", ProductID: "prd_1", Label: "Bad", DocumentTypeID: "dt_datasheet", ExternalURL: "https://user:pass@example.test/data.pdf"},
	} {
		if err := document.Prepare(); !errors.Is(err, ErrInvalidDocument) {
			t.Errorf("document %+v: error=%v", document, err)
		}
	}
}

func TestCategoryAndDictionaryDefaults(t *testing.T) {
	category := Category{ID: "cat_power", ParentID: "cat_root", Name: "Power", Slug: "power"}
	if err := category.Prepare(); err != nil {
		t.Fatal(err)
	}
	if category.Status != EntryActive || category.Revision != 1 {
		t.Fatalf("category defaults = %+v", category)
	}
	entry := DictionaryEntry{ID: "mfr_ti", Kind: DictionaryManufacturer, Name: "Texas Instruments"}
	if err := entry.Prepare(); err != nil {
		t.Fatal(err)
	}
	if entry.Status != EntryActive || entry.Revision != 1 {
		t.Fatalf("dictionary defaults = %+v", entry)
	}
}

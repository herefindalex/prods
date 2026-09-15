package sqlite

import (
	"context"
	"errors"
	"testing"

	"prods/internal/catalog"
)

func TestCategoryTreeAllowsUnevenDepthAndRejectsCyclesAndSystemChanges(t *testing.T) {
	ctx := context.Background()
	store, owner := installedStore(t)
	parent, err := store.CreateCategory(ctx, owner.ID, catalog.Category{Name: "Semiconductors", Slug: "semiconductors"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateCategory(ctx, owner.ID, catalog.Category{ParentID: parent.ID, Name: "Power", Slug: "power"})
	if err != nil {
		t.Fatal(err)
	}
	grandchild, err := store.CreateCategory(ctx, owner.ID, catalog.Category{ParentID: child.ID, Name: "MOSFET", Slug: "mosfet"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MoveCategory(ctx, owner.ID, parent.ID, grandchild.ID, parent.Revision); !errors.Is(err, catalog.ErrCategoryCycle) {
		t.Fatalf("cycle move = %v", err)
	}
	if err := store.MoveCategory(ctx, owner.ID, "cat_root", child.ID, 1); !errors.Is(err, catalog.ErrSystemCategory) {
		t.Fatalf("root move = %v", err)
	}
	if err := store.DisableCategory(ctx, owner.ID, "cat_uncategorized", 1); !errors.Is(err, catalog.ErrSystemCategory) {
		t.Fatalf("uncategorized disable = %v", err)
	}
	if _, err := store.CreateCategory(ctx, owner.ID, catalog.Category{ParentID: parent.ID, Name: "Duplicate", Slug: "power"}); err == nil {
		t.Fatal("duplicate sibling slug unexpectedly succeeded")
	}
	if _, err := store.CreateCategory(ctx, owner.ID, catalog.Category{Name: "Power", Slug: "power"}); err != nil {
		t.Fatalf("same slug under a different parent: %v", err)
	}
}

func TestDisabledDictionaryCannotBeSelectedButExistingProductIsPreserved(t *testing.T) {
	ctx := context.Background()
	store, owner := installedStore(t)
	maker, err := store.CreateDictionaryEntry(ctx, owner.ID, catalog.DictionaryEntry{
		Kind: catalog.DictionaryManufacturer, Name: "Example Maker", Slug: "example-maker",
	})
	if err != nil {
		t.Fatal(err)
	}
	product, err := store.CreateProduct(ctx, owner.ID, catalog.Product{PartNumber: "DICT-1", ManufacturerID: maker.ID})
	if err != nil {
		t.Fatal(err)
	}
	if product.Manufacturer != "Example Maker" || product.IdentityMaker != "id:"+maker.ID {
		t.Fatalf("resolved manufacturer = %+v", product)
	}
	if err := store.DisableDictionaryEntry(ctx, owner.ID, maker.ID, maker.Revision); err != nil {
		t.Fatal(err)
	}
	persisted, err := store.Product(ctx, product.ID)
	if err != nil || persisted.Manufacturer != "Example Maker" {
		t.Fatalf("historical reference = %+v, %v", persisted, err)
	}
	if _, err := store.CreateProduct(ctx, owner.ID, catalog.Product{PartNumber: "DICT-2", ManufacturerID: maker.ID}); !errors.Is(err, catalog.ErrDisabledReference) {
		t.Fatalf("new disabled reference = %v", err)
	}
}

func TestSpecRawManualNormalizationCategoryChangeAndMultipleDocuments(t *testing.T) {
	ctx := context.Background()
	store, owner := installedStore(t)
	category, err := store.CreateCategory(ctx, owner.ID, catalog.Category{Name: "Power", Slug: "power"})
	if err != nil {
		t.Fatal(err)
	}
	otherCategory, err := store.CreateCategory(ctx, owner.ID, catalog.Category{Name: "Connectors", Slug: "connectors"})
	if err != nil {
		t.Fatal(err)
	}
	spec, err := store.CreateSpecDefinition(ctx, owner.ID, catalog.SpecDefinition{Name: "Input Voltage", PreferredUnit: "V", Filterable: true})
	if err != nil {
		t.Fatal(err)
	}
	set, err := store.CreateSpecSet(ctx, owner.ID, catalog.SpecSet{Name: "Power Specs", SpecIDs: []string{spec.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetCategorySpecSet(ctx, owner.ID, category.ID, set.ID, category.Revision); err != nil {
		t.Fatal(err)
	}
	product, err := store.CreateProduct(ctx, owner.ID, catalog.Product{PartNumber: "SPEC-DOC-1", CategoryID: category.ID})
	if err != nil {
		t.Fatal(err)
	}
	raw := "3.0–3.6V, 4.5–5.5V"
	value, err := store.SaveSpecValue(ctx, owner.ID, product.ID, spec.ID, raw, "en-US", product.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if value.RawValue != raw || !value.Active || value.SourceRevision != 1 {
		t.Fatalf("spec value = %+v", value)
	}
	manual, err := store.SaveNormalizedValue(ctx, owner.ID, catalog.NormalizedValue{
		SpecValueID: value.ID, ValueJSON: `{"ranges":[[3.0,3.6],[4.5,5.5]],"unit":"V"}`,
		SourceRevision: 1, SpecSemanticVersion: 1, NormalizerVersion: "manual-v1",
		Source: catalog.NormalizedManual, Status: catalog.NormalizedCurrent,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveNormalizedValue(ctx, owner.ID, catalog.NormalizedValue{
		SpecValueID: value.ID, ValueJSON: `{"min":3.0,"max":5.5,"unit":"V"}`,
		SourceRevision: 1, SpecSemanticVersion: 1, NormalizerVersion: "parser-v1",
		Source: catalog.NormalizedAutomatic, Status: catalog.NormalizedCurrent,
	}); !errors.Is(err, catalog.ErrManualNormalization) {
		t.Fatalf("automatic overwrite of manual value = %v", err)
	}

	value, err = store.SaveSpecValue(ctx, owner.ID, product.ID, spec.ID, "Depends on configuration", "en-US", 2)
	if err != nil {
		t.Fatal(err)
	}
	if value.SourceRevision != 2 {
		t.Fatalf("updated source revision = %d", value.SourceRevision)
	}
	var normalizedStatus catalog.NormalizedStatus
	if err := store.db.QueryRowContext(ctx, `SELECT status FROM normalized_values WHERE id=?`, manual.ID).Scan(&normalizedStatus); err != nil {
		t.Fatal(err)
	}
	if normalizedStatus != catalog.NormalizedStale {
		t.Fatalf("manual result after raw edit = %q", normalizedStatus)
	}

	product, err = store.Product(ctx, product.ID)
	if err != nil {
		t.Fatal(err)
	}
	product.CategoryID = otherCategory.ID
	product, err = store.UpdateProduct(ctx, owner.ID, product.Revision, product)
	if err != nil {
		t.Fatal(err)
	}
	var active bool
	if err := store.db.QueryRowContext(ctx, `SELECT active FROM product_spec_values WHERE id=?`, value.ID).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active {
		t.Fatal("spec value remained active after moving to a category without that SpecSet")
	}

	first, err := store.AddProductDocument(ctx, owner.ID, product.Revision, catalog.ProductDocument{
		ProductID: product.ID, Label: "Datasheet", DocumentTypeID: "dt_datasheet",
		ExternalURL: "https://example.test/datasheet.pdf", SortOrder: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" {
		t.Fatal("document id missing")
	}
	second, err := store.AddProductDocument(ctx, owner.ID, product.Revision+1, catalog.ProductDocument{
		ProductID: product.ID, Label: "Application note", DocumentTypeID: "dt_application_note",
		ExternalURL: "https://example.test/app-note.pdf", Language: "en-US", SortOrder: 20,
	})
	if err != nil || second.ID == "" {
		t.Fatalf("second document = %+v, %v", second, err)
	}
	documents, err := store.ProductDocuments(ctx, product.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(documents) != 2 || documents[0].ID != first.ID || documents[1].ID != second.ID {
		t.Fatalf("documents = %+v", documents)
	}
	if err := store.DisableDictionaryEntry(ctx, owner.ID, "dt_datasheet", 1); err != nil {
		t.Fatal(err)
	}
	product, err = store.Product(ctx, product.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddProductDocument(ctx, owner.ID, product.Revision, catalog.ProductDocument{
		ProductID: product.ID, Label: "New datasheet", DocumentTypeID: "dt_datasheet",
		ExternalURL: "https://example.test/new.pdf",
	}); !errors.Is(err, catalog.ErrDisabledReference) {
		t.Fatalf("new document with disabled type = %v", err)
	}
	documents, err = store.ProductDocuments(ctx, product.ID)
	if err != nil || len(documents) != 2 {
		t.Fatalf("existing documents after type disable = %+v, %v", documents, err)
	}
	if _, err := store.CloneProduct(ctx, owner.ID, product.ID, product.Revision, catalog.Product{}); !errors.Is(err, catalog.ErrIdentityConflict) {
		t.Fatalf("unmodified clone identity = %v", err)
	}
	clone, err := store.CloneProduct(ctx, owner.ID, product.ID, product.Revision, catalog.Product{PartNumber: "SPEC-DOC-2"})
	if err != nil {
		t.Fatal(err)
	}
	if clone.ID == product.ID || clone.ClonedFromID != product.ID || clone.RecordState != catalog.RecordCurrent || clone.Status != catalog.Hidden {
		t.Fatalf("clone = %+v", clone)
	}
	clonedDocuments, err := store.ProductDocuments(ctx, clone.ID)
	if err != nil || len(clonedDocuments) != 2 || clonedDocuments[0].ExternalURL != documents[0].ExternalURL {
		t.Fatalf("cloned documents = %+v, %v", clonedDocuments, err)
	}
	var clonedRaw string
	var clonedActive bool
	if err := store.db.QueryRowContext(ctx, `SELECT raw_value,active FROM product_spec_values WHERE product_id=?`, clone.ID).Scan(&clonedRaw, &clonedActive); err != nil {
		t.Fatal(err)
	}
	if clonedRaw != "Depends on configuration" || clonedActive {
		t.Fatalf("cloned raw spec = %q active=%t", clonedRaw, clonedActive)
	}
}

func TestTaxonomyUpdatePreviewsAndRequeuesAffectedProducts(t *testing.T) {
	ctx := context.Background()
	store, owner := installedStore(t)
	lifecycle, err := store.CreateDictionaryEntry(ctx, owner.ID, catalog.DictionaryEntry{
		Kind: catalog.DictionaryLifecycle, Name: "Active", Slug: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	product, err := store.CreateProduct(ctx, owner.ID, catalog.Product{
		PartNumber: "LIFE-1", LifecycleID: lifecycle.ID, Status: catalog.Published,
	})
	if err != nil {
		t.Fatal(err)
	}

	candidate := lifecycle
	candidate.Name = "Not Recommended for New Designs"
	preview, err := store.PreviewDictionaryUpdate(ctx, owner.ID, lifecycle.ID, lifecycle.Revision, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.AffectedProducts) != 1 || preview.AffectedProducts[0].ProductID != product.ID {
		t.Fatalf("preview = %+v", preview)
	}
	unchanged, err := store.Product(ctx, product.ID)
	if err != nil || unchanged.Revision != product.Revision || unchanged.Lifecycle != "Active" {
		t.Fatalf("preview mutated product = %+v, %v", unchanged, err)
	}

	updated, impact, err := store.UpdateDictionaryEntry(ctx, owner.ID, lifecycle.ID, lifecycle.Revision, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != lifecycle.Revision+1 || len(impact.AffectedProducts) != 1 {
		t.Fatalf("update = %+v impact=%+v", updated, impact)
	}
	changed, err := store.Product(ctx, product.ID)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Revision != product.Revision+1 || changed.Lifecycle != candidate.Name {
		t.Fatalf("affected product = %+v", changed)
	}
	products, total, err := store.ListPublished(ctx, "recommended", 1, 20)
	if err != nil || total != 1 || len(products) != 1 || products[0].ID != product.ID {
		t.Fatalf("renamed lifecycle search = %+v total=%d err=%v", products, total, err)
	}
	var pending int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM publication_intents WHERE entity_type='product' AND entity_id=? AND desired_revision=? AND status='pending'`, product.ID, changed.Revision).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 1 {
		t.Fatalf("pending re-publication intents = %d", pending)
	}
}

func TestCategoryParentUpdateIncludesDescendantProducts(t *testing.T) {
	ctx := context.Background()
	store, owner := installedStore(t)
	left, err := store.CreateCategory(ctx, owner.ID, catalog.Category{Name: "Left", Slug: "left"})
	if err != nil {
		t.Fatal(err)
	}
	right, err := store.CreateCategory(ctx, owner.ID, catalog.Category{Name: "Right", Slug: "right"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateCategory(ctx, owner.ID, catalog.Category{ParentID: left.ID, Name: "Child", Slug: "child"})
	if err != nil {
		t.Fatal(err)
	}
	product, err := store.CreateProduct(ctx, owner.ID, catalog.Product{PartNumber: "TREE-1", CategoryID: child.ID, Status: catalog.Published})
	if err != nil {
		t.Fatal(err)
	}
	candidate := left
	candidate.ParentID = right.ID
	preview, err := store.PreviewCategoryUpdate(ctx, owner.ID, left.ID, left.Revision, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.AffectedProducts) != 1 || preview.AffectedProducts[0].ProductID != product.ID {
		t.Fatalf("subtree preview = %+v", preview)
	}
	persistedLeft, err := store.Category(ctx, left.ID)
	if err != nil || persistedLeft.ParentID != "cat_root" {
		t.Fatalf("preview mutated category = %+v, %v", persistedLeft, err)
	}
	updated, impact, err := store.UpdateCategory(ctx, owner.ID, left.ID, left.Revision, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ParentID != right.ID || len(impact.AffectedProducts) != 1 {
		t.Fatalf("category update = %+v impact=%+v", updated, impact)
	}
	changed, err := store.Product(ctx, product.ID)
	if err != nil || changed.Revision != product.Revision+1 {
		t.Fatalf("descendant product = %+v, %v", changed, err)
	}
}

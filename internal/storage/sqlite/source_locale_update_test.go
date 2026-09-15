package sqlite

import (
	"errors"
	"testing"

	"prods/internal/catalog"
	"prods/internal/site"
)

func TestPerFieldSourceLocaleUpdatesAreRevisionedAndUseWorkingEditingGate(t *testing.T) {
	store, owner := installedStore(t)
	website, err := store.WebsiteState(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveWebsiteLocalization(t.Context(), owner.ID, website.WorkingRevision, site.WebsiteLocalization{
		DefaultLocale: "en-US", EnabledLocales: []string{"en-US"}, ContentEditingEnabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	product, err := store.CreateProduct(t.Context(), owner.ID, catalog.Product{PartNumber: "SOURCE-LOCALE-1", Name: "Source"})
	if err != nil {
		t.Fatal(err)
	}
	productContent, err := store.SaveProductSourceLocales(t.Context(), owner.ID, product.ID, product.Revision, "de-DE", map[string]string{"name": "de-DE", "description": "fr-FR", "features": "de-DE", "specification": "ja-JP"})
	if err != nil {
		t.Fatal(err)
	}
	if productContent.ProductRevision != product.Revision+1 || productContent.SourceLocale != "de-DE" || productContent.SourceLocales["description"] != "fr-FR" {
		t.Fatalf("product content = %+v", productContent)
	}
	if _, err := store.SaveProductSourceLocales(t.Context(), owner.ID, product.ID, product.Revision, "en-US", nil); !errors.Is(err, catalog.ErrRevisionConflict) {
		t.Fatalf("stale Product source update = %v", err)
	}

	category, err := store.CreateCategory(t.Context(), owner.ID, catalog.Category{Name: "Source category", Slug: "source-category"})
	if err != nil {
		t.Fatal(err)
	}
	taxonomyContent, err := store.SaveTaxonomySourceLocales(t.Context(), owner.ID, "category", category.ID, category.Revision, "it-IT", map[string]string{"name": "it-IT", "description": "es-ES"})
	if err != nil {
		t.Fatal(err)
	}
	if taxonomyContent.SubjectRevision != category.Revision+1 || taxonomyContent.SourceLocale != "it-IT" || taxonomyContent.SourceLocales["description"] != "es-ES" {
		t.Fatalf("taxonomy content = %+v", taxonomyContent)
	}
}

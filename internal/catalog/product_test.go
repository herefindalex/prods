package catalog

import (
	"errors"
	"testing"
)

func TestFoldSearchContract(t *testing.T) {
	if FoldSearch("É") != FoldSearch("é") {
		t.Fatal("Unicode case variants must fold together")
	}
	if FoldSearch("é") == FoldSearch("e") {
		t.Fatal("D6 does not strip diacritics")
	}
	if FoldSearch("ABC123") != FoldSearch("abc123") {
		t.Fatal("ASCII case variants must fold together")
	}
}

func TestEscapeLike(t *testing.T) {
	if got, want := EscapeLike(`A%_\B`), `A\%\_\\B`; got != want {
		t.Fatalf("EscapeLike() = %q, want %q", got, want)
	}
}

func TestPreparePreservesBusinessIdentity(t *testing.T) {
	upper := Product{ID: "upper", PartNumber: "ABC123", Manufacturer: "Example", Status: Published}
	lower := Product{ID: "lower", PartNumber: "abc123", Manufacturer: "Example", Status: Published}
	if err := upper.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := lower.Prepare(); err != nil {
		t.Fatal(err)
	}
	if upper.PartNumber == lower.PartNumber {
		t.Fatal("business identity must remain case-sensitive")
	}
	if upper.SearchFolded != lower.SearchFolded {
		t.Fatal("search projection should discover both case variants")
	}
}

func TestPrepareOnlyRequiresPartNumberAndPreservesOriginalIdentityInput(t *testing.T) {
	product := Product{ID: "part-only", PartNumber: "  ABC123  "}
	if err := product.Prepare(); err != nil {
		t.Fatal(err)
	}
	if product.PartNumber != "  ABC123  " {
		t.Fatalf("part number was rewritten: %q", product.PartNumber)
	}
	if product.IdentityPart != "ABC123" || product.IdentityMaker != "" {
		t.Fatalf("identity projection = %q/%q", product.IdentityMaker, product.IdentityPart)
	}
	if product.CategoryID != UncategorizedCategoryID || product.RecordState != RecordCurrent || product.Status != Hidden {
		t.Fatalf("defaults = category %q, record %q, publishing %q", product.CategoryID, product.RecordState, product.Status)
	}
	if product.DisplayName() != "ABC123" {
		t.Fatalf("display fallback = %q", product.DisplayName())
	}
}

func TestArchivedCannotBePublished(t *testing.T) {
	product := Product{ID: "archived", PartNumber: "ABC123", RecordState: RecordArchived, Status: Published}
	if err := product.Prepare(); !errors.Is(err, ErrInvalidProduct) {
		t.Fatalf("Prepare() = %v, want ErrInvalidProduct", err)
	}
}

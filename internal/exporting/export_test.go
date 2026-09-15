package exporting

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/inquiries"
	"prods/internal/storage/sqlite"

	"github.com/xuri/excelize/v2"
)

func TestProductXLSXAndRFQCSVExportsPreserveDataAndNeutralizeCSVFormulae(t *testing.T) {
	store, owner := exportTestStore(t)
	product, err := store.CreateProduct(t.Context(), owner.ID, catalog.Product{PartNumber: "=FORMULA", Name: "Export product"})
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	productPath := filepath.Join(work, "products.xlsx")
	rows, err := ProductsXLSX(t.Context(), store, owner.ID, productPath)
	if err != nil {
		t.Fatal(err)
	}
	if rows < 1 {
		t.Fatalf("product export rows = %d", rows)
	}
	workbook, err := excelize.OpenFile(productPath)
	if err != nil {
		t.Fatal(err)
	}
	defer workbook.Close()
	partNumber, err := workbook.GetCellValue("Products", "B2")
	if err != nil {
		t.Fatal(err)
	}
	formula, err := workbook.GetCellFormula("Products", "B2")
	if err != nil {
		t.Fatal(err)
	}
	if partNumber != product.PartNumber || formula != "" {
		t.Fatalf("xlsx part number=%q formula=%q", partNumber, formula)
	}

	if _, err := store.ReplaceRFQRecipients(t.Context(), owner.ID, mustSubmitRFQ(t, store, inquiries.Submission{
		Name: "=Spreadsheet command", Email: "buyer@example.test",
		Items: []inquiries.Item{{Kind: "requested", Requested: "+PART"}},
	}), 1, []inquiries.Recipient{{Kind: inquiries.RecipientEmail, Email: "sales@example.test"}}); err != nil {
		t.Fatal(err)
	}
	rfqPath := filepath.Join(work, "rfqs.csv")
	rows, err = RFQsCSV(t.Context(), store, owner.ID, rfqPath)
	if err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("RFQ export rows = %d", rows)
	}
	body, err := os.ReadFile(rfqPath)
	if err != nil {
		t.Fatal(err)
	}
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\uFEFF")))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0][10] != "Privacy State" || records[1][3] != "'=Spreadsheet command" ||
		records[1][10] != "retained" || records[1][17] != "'+PART" {
		t.Fatalf("CSV records = %#v", records)
	}
}

func exportTestStore(t *testing.T) (*sqlite.Store, identity.User) {
	t.Helper()
	store, err := sqlite.Create(filepath.Join(t.TempDir(), "export.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	password, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.CompleteInstallation(t.Context(), sqlite.Installation{
		OwnerEmail: "owner@example.test", OwnerDisplayName: "Owner", PasswordHash: password,
		DefaultLocale: "en-US", SupportedLocales: []string{"en-US"}, TimeZone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	return store, owner
}

func mustSubmitRFQ(t *testing.T, store *sqlite.Store, submission inquiries.Submission) string {
	t.Helper()
	key, err := sqlite.NewKey()
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := store.SubmitRFQ(t.Context(), key, submission)
	if err != nil {
		t.Fatal(err)
	}
	return receipt.RFQID
}

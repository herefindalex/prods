package webapp

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prods/internal/catalog"
	"prods/internal/inquiries"
	"prods/internal/storage/sqlite"

	"github.com/xuri/excelize/v2"
)

func TestAdminExportsRequireCSRFGenerateAuditedFilesAndCleanWork(t *testing.T) {
	fixture := newBackupWebFixture(t, nil)
	if _, err := fixture.store.CreateProduct(t.Context(), fixture.owner.ID, catalog.Product{PartNumber: "EXP-1", Name: "Exported"}); err != nil {
		t.Fatal(err)
	}
	key, _ := sqlite.NewKey()
	if _, err := fixture.store.SubmitRFQ(t.Context(), key, inquiries.Submission{
		Name: "=Buyer", Email: "buyer@example.test",
		Items: []inquiries.Item{{Kind: "requested", Requested: "+PART"}},
	}); err != nil {
		t.Fatal(err)
	}

	productURL := fixture.server.URL + "/admin/api/exports/products.xlsx"
	withoutCSRF := postAdminJSON(t, fixture.client, productURL, "", `{}`)
	if withoutCSRF.StatusCode != http.StatusForbidden {
		t.Fatalf("product export without CSRF = %d", withoutCSRF.StatusCode)
	}
	withoutCSRF.Body.Close()
	productResponse := postAdminJSON(t, fixture.client, productURL, fixture.csrf, `{}`)
	if productResponse.StatusCode != http.StatusOK || productResponse.Header.Get("Cache-Control") != "no-store" ||
		!strings.Contains(productResponse.Header.Get("Content-Disposition"), "prods-products.xlsx") {
		t.Fatalf("product export status=%d headers=%v", productResponse.StatusCode, productResponse.Header)
	}
	productBody, err := io.ReadAll(productResponse.Body)
	productResponse.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	workbook, err := excelize.OpenReader(strings.NewReader(string(productBody)))
	if err != nil {
		t.Fatal(err)
	}
	value, err := workbook.GetCellValue("Products", "B2")
	workbook.Close()
	if err != nil || value != "EXP-1" {
		t.Fatalf("exported product=%q err=%v", value, err)
	}

	rfqURL := fixture.server.URL + "/admin/api/exports/rfqs.csv"
	rfqResponse := postAdminJSON(t, fixture.client, rfqURL, fixture.csrf, `{}`)
	if rfqResponse.StatusCode != http.StatusOK || !strings.HasPrefix(rfqResponse.Header.Get("Content-Type"), "text/csv") {
		t.Fatalf("RFQ export status=%d type=%q", rfqResponse.StatusCode, rfqResponse.Header.Get("Content-Type"))
	}
	rfqBody, err := io.ReadAll(rfqResponse.Body)
	rfqResponse.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rfqBody), "'=Buyer") || !strings.Contains(string(rfqBody), "'+PART") {
		t.Fatalf("RFQ export did not neutralize spreadsheet formula cells: %q", rfqBody)
	}

	entries, err := fixture.store.ListAudit(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]int{}
	for _, entry := range entries {
		actions[entry.Action]++
	}
	if actions["product.export_generated"] != 1 || actions["rfq.export_generated"] != 1 {
		t.Fatalf("export audit actions = %#v", actions)
	}
	workEntries, err := os.ReadDir(filepath.Join(fixture.root, "work"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(workEntries) != 0 {
		t.Fatalf("export work residue = %#v", workEntries)
	}
}

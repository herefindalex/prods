package publishing

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prods/internal/platform"
)

func TestLocalizedPublicationUnitUsesOneViewIdentityAndFieldFallback(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "units"), 0o700); err != nil {
		t.Fatal(err)
	}
	engine := &Engine{root: root, resources: platform.NewResourceGate(func(string) (platform.ResourceStats, error) {
		return platform.ResourceStats{VolumeID: "test", TotalBytes: 1 << 30, FreeBytes: 1 << 30, FreeInodes: 1 << 20}, nil
	})}
	view := PublicView{ID: "prd_1", Revision: 3, SiteEpoch: 2, PartNumber: "ABC-1", Name: "Source name", Description: "Source description", CanonicalURL: "https://example.test/products/abc-1", Language: "en-US", SupportedLocales: []string{"en-US", "zh-TW"}, RFQURL: "/rfq?product_id=prd_1", Localizations: map[string]LocalizedContent{"zh-TW": {Name: "翻譯名稱"}}}
	artifact, _, err := engine.stage(context.Background(), "op-localized", view)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "units", artifact)
	zhJSON, err := os.ReadFile(filepath.Join(dir, localizedArtifactName("zh-TW", "product.json")))
	if err != nil {
		t.Fatal(err)
	}
	var decoded PublicView
	if err := json.Unmarshal(zhJSON, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ID != view.ID || decoded.PartNumber != view.PartNumber || decoded.CanonicalURL != view.CanonicalURL || decoded.Language != "zh-TW" || decoded.Name != "翻譯名稱" || decoded.Description != "Source description" {
		t.Fatalf("localized view = %+v", decoded)
	}
	zhHTML, err := os.ReadFile(filepath.Join(dir, localizedArtifactName("zh-TW", "index.html")))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(zhHTML), "翻譯名稱") || !strings.Contains(string(zhHTML), "Source description") {
		t.Fatalf("localized html = %s", zhHTML)
	}
	for _, name := range []string{"index.html", "product.json", "product.jsonld", "product.md"} {
		if _, err := os.Stat(filepath.Join(dir, localizedArtifactName("zh-TW", name))); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

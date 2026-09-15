package publishing

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"prods/internal/platform"
)

func TestProductGenerationResourceAdmissionPrecedesFilesystemWrites(t *testing.T) {
	root := filepath.Join(t.TempDir(), "generated")
	gate := platform.NewResourceGate(func(string) (platform.ResourceStats, error) {
		return platform.ResourceStats{VolumeID: "generated", TotalBytes: 100, FreeBytes: 1}, nil
	})
	engine := &Engine{root: root, resources: gate}
	view := PublicView{
		ID: "product-1", Revision: 1, PartNumber: "ABC-1", Name: "Product",
		Manufacturer: "Example", CanonicalURL: "https://catalog.example.test/products/ABC-1",
		Language: "en", RFQURL: "/rfq?product_id=product-1",
	}
	if _, _, err := engine.stage(t.Context(), "operation", view); !errors.Is(err, platform.ErrResourceCritical) {
		t.Fatalf("generation admission error = %v", err)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("generated root was touched before admission: %v", err)
	}
}

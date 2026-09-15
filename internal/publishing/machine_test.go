package publishing

import (
	"encoding/json"
	"strings"
	"testing"

	"prods/internal/site"
)

func TestMachineRepresentationsAreDeterministicAndUsePublicViews(t *testing.T) {
	configuration := site.DefaultConfiguration()
	configuration.Organization.DisplayName = "Example Components"
	configuration.SEO.DefaultDescription = "Public component catalog."
	views := []PublicView{
		{
			ID: "product-b", Revision: 3, SiteEpoch: 7, PartNumber: "B-200",
			CanonicalURL: "https://catalog.example.test/products/b-200",
		},
		{
			ID: "product-a", Revision: 2, SiteEpoch: 7, PartNumber: "A-100",
			CanonicalURL: "https://catalog.example.test/products/a-100",
		},
	}
	first, err := MachineRepresentations(views, configuration, "https://catalog.example.test/", 7)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MachineRepresentations(views, configuration, "https://catalog.example.test", 7)
	if err != nil {
		t.Fatal(err)
	}
	for path, representation := range first {
		other, exists := second[path]
		if !exists || string(representation.Body) != string(other.Body) || representation.ETag != other.ETag {
			t.Fatalf("machine output %s was not deterministic", path)
		}
	}

	manifestRepresentation := first[CatalogManifestPath]
	var manifest catalogManifest
	if err := json.Unmarshal(manifestRepresentation.Body, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.FormatVersion != CatalogFormat || manifest.SiteEpoch != 7 || manifest.ProductCount != 2 || len(manifest.Shards) != 1 {
		t.Fatalf("manifest = %+v", manifest)
	}
	shardReference := manifest.Shards[0]
	shardRepresentation, exists := first["/catalog/products-000001.json"]
	if !exists || shardReference.SHA256 != digest(shardRepresentation.Body) || shardReference.ProductCount != 2 {
		t.Fatalf("shard reference = %+v", shardReference)
	}
	var shard catalogShard
	if err := json.Unmarshal(shardRepresentation.Body, &shard); err != nil {
		t.Fatal(err)
	}
	if shard.FormatVersion != CatalogShardFormat || len(shard.Products) != 2 || shard.Products[0].ID != "product-a" || shard.Products[1].ID != "product-b" {
		t.Fatalf("shard = %+v", shard)
	}
	productJSON, err := JSON(views[1])
	if err != nil {
		t.Fatal(err)
	}
	if shard.Products[0].ProductJSONSHA256 != digest(productJSON) {
		t.Fatalf("product checksum = %q", shard.Products[0].ProductJSONSHA256)
	}

	for path, expected := range map[string][]string{
		"/robots.txt":  {"User-agent: *", "Sitemap: https://catalog.example.test/sitemap.xml", "Disallow: /admin/"},
		"/llms.txt":    {"# Example Components", "Catalog manifest", "https://catalog.example.test/catalog/manifest.json"},
		"/sitemap.xml": {"https://catalog.example.test/products/a-100", "https://catalog.example.test/products/b-200"},
	} {
		body := string(first[path].Body)
		for _, value := range expected {
			if !strings.Contains(body, value) {
				t.Errorf("%s missing %q: %s", path, value, body)
			}
		}
	}
}

func TestEmptyCatalogManifestHasNoInventedShard(t *testing.T) {
	outputs, err := MachineRepresentations(nil, site.DefaultConfiguration(), "https://catalog.example.test", 4)
	if err != nil {
		t.Fatal(err)
	}
	var manifest catalogManifest
	if err := json.Unmarshal(outputs[CatalogManifestPath].Body, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.ProductCount != 0 || len(manifest.Shards) != 0 {
		t.Fatalf("empty manifest = %+v", manifest)
	}
	if _, exists := outputs["/catalog/products-000001.json"]; exists {
		t.Fatal("empty catalog exposed an invented shard")
	}
}

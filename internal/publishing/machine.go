package publishing

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"prods/internal/site"
)

const (
	CatalogManifestPath = "/catalog/manifest.json"
	CatalogShardSize    = 1000
	CatalogFormat       = "prods.catalog-manifest.v1"
	CatalogShardFormat  = "prods.catalog-shard.v1"
)

// MachineRepresentation is an immutable public response derived from the
// active PublicView set. It contains no Admin DTO or database row.
type MachineRepresentation struct {
	ContentType string
	Body        []byte
	ETag        string
}

type catalogManifest struct {
	FormatVersion string                  `json:"format_version"`
	SiteEpoch     int64                   `json:"site_epoch"`
	ProductCount  int                     `json:"product_count"`
	Shards        []catalogShardReference `json:"shards"`
}

type catalogShardReference struct {
	Index        int    `json:"index"`
	URL          string `json:"url"`
	ProductCount int    `json:"product_count"`
	SHA256       string `json:"sha256"`
}

type catalogShard struct {
	FormatVersion string                    `json:"format_version"`
	SiteEpoch     int64                     `json:"site_epoch"`
	Index         int                       `json:"index"`
	Products      []catalogProductReference `json:"products"`
}

type catalogProductReference struct {
	ID                string `json:"id"`
	PublicRevision    int64  `json:"public_revision"`
	SiteEpoch         int64  `json:"site_epoch"`
	PartNumber        string `json:"part_number"`
	CanonicalURL      string `json:"canonical_url"`
	JSONURL           string `json:"json_url"`
	MarkdownURL       string `json:"markdown_url"`
	ProductJSONSHA256 string `json:"product_json_sha256"`
}

// MachineRepresentations builds deterministic public discovery outputs from
// one active visibility snapshot. Callers must obtain that snapshot through
// PublicGate before invoking this function.
func MachineRepresentations(views []PublicView, configuration site.Configuration, baseURL string, siteEpoch int64) (map[string]MachineRepresentation, error) {
	ordered := append([]PublicView(nil), views...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].PartNumber == ordered[j].PartNumber {
			return ordered[i].ID < ordered[j].ID
		}
		return ordered[i].PartNumber < ordered[j].PartNumber
	})

	baseURL = strings.TrimRight(baseURL, "/")
	outputs := make(map[string]MachineRepresentation)
	sitemap, err := Sitemap(ordered)
	if err != nil {
		return nil, err
	}
	outputs["/sitemap.xml"] = newMachineRepresentation("application/xml; charset=utf-8", sitemap)
	outputs["/robots.txt"] = newMachineRepresentation("text/plain; charset=utf-8", robotsText(baseURL))
	outputs["/llms.txt"] = newMachineRepresentation("text/markdown; charset=utf-8", llmsText(configuration, baseURL))

	manifest := catalogManifest{
		FormatVersion: CatalogFormat,
		SiteEpoch:     siteEpoch,
		ProductCount:  len(ordered),
		Shards:        make([]catalogShardReference, 0),
	}
	for start, shardIndex := 0, 1; start < len(ordered); start, shardIndex = start+CatalogShardSize, shardIndex+1 {
		end := start + CatalogShardSize
		if end > len(ordered) {
			end = len(ordered)
		}
		products := make([]catalogProductReference, 0, end-start)
		for _, view := range ordered[start:end] {
			productJSON, err := JSON(view)
			if err != nil {
				return nil, err
			}
			products = append(products, catalogProductReference{
				ID: view.ID, PublicRevision: view.Revision, SiteEpoch: view.SiteEpoch,
				PartNumber: view.PartNumber, CanonicalURL: view.CanonicalURL,
				JSONURL: view.CanonicalURL + ".json", MarkdownURL: view.CanonicalURL + ".md",
				ProductJSONSHA256: digest(productJSON),
			})
		}
		shard := catalogShard{
			FormatVersion: CatalogShardFormat,
			SiteEpoch:     siteEpoch,
			Index:         shardIndex,
			Products:      products,
		}
		body, err := marshalMachineJSON(shard)
		if err != nil {
			return nil, err
		}
		path := fmt.Sprintf("/catalog/products-%06d.json", shardIndex)
		outputs[path] = newMachineRepresentation("application/json; charset=utf-8", body)
		manifest.Shards = append(manifest.Shards, catalogShardReference{
			Index: shardIndex, URL: baseURL + path, ProductCount: len(products), SHA256: digest(body),
		})
	}
	manifestBody, err := marshalMachineJSON(manifest)
	if err != nil {
		return nil, err
	}
	outputs[CatalogManifestPath] = newMachineRepresentation("application/json; charset=utf-8", manifestBody)
	return outputs, nil
}

func newMachineRepresentation(contentType string, body []byte) MachineRepresentation {
	copyBody := append([]byte(nil), body...)
	return MachineRepresentation{
		ContentType: contentType,
		Body:        copyBody,
		ETag:        `"m-` + digest(copyBody)[:32] + `"`,
	}
}

func marshalMachineJSON(value any) ([]byte, error) {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}

func digest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func robotsText(baseURL string) []byte {
	return []byte(strings.Join([]string{
		"User-agent: *",
		"Allow: /",
		"Disallow: /admin/",
		"Disallow: /install",
		"Disallow: /set-password",
		"Disallow: /rfq",
		"Disallow: /api/",
		"Disallow: /search?",
		"Sitemap: " + baseURL + "/sitemap.xml",
		"",
	}, "\n"))
}

func llmsText(configuration site.Configuration, baseURL string) []byte {
	name := singleLine(configuration.Organization.DisplayName)
	if name == "" {
		name = "Product Catalog"
	}
	description := singleLine(configuration.SEO.DefaultDescription)
	if description == "" {
		description = "Public product information and request-for-quotation entry points."
	}
	return []byte(fmt.Sprintf(`# %s

> %s

## Public catalog

- [Browse products](%s/search)
- [Catalog manifest](%s%s)
- [Sitemap](%s/sitemap.xml)
- [Request a quote](%s/rfq)

Product pages provide alternate JSON and Markdown representations. Visibility and authorization are identical across HTML and machine-readable formats.
`, name, description, baseURL, baseURL, CatalogManifestPath, baseURL, baseURL))
}

func singleLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

// Serve writes a cache-revalidated immutable representation. Range handling
// is delegated to net/http after the caller has admitted this snapshot.
func (representation MachineRepresentation) Serve(w http.ResponseWriter, r *http.Request, name string) {
	w.Header().Set("Content-Type", representation.ContentType)
	w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
	w.Header().Set("ETag", representation.ETag)
	if etagMatches(r.Header.Get("If-None-Match"), representation.ETag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(representation.Body))
}

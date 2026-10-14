package webapp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"prods/internal/catalog"
	"prods/internal/publishing"
	"prods/internal/site"
	"prods/internal/specnumeric"
	"strings"
	"testing"
)

func numericHTTPFixture(t *testing.T) (numericPublicationFixture, *httptest.Server, *http.Client, string) {
	t.Helper()
	f := newNumericPublicationFixture(t)
	app, _, err := New(f.store, Config{BaseURL: "http://catalog.example.test", WorkDir: t.TempDir(), PublicDir: filepath.Join(f.root, "generated")})
	if err != nil {
		t.Fatal(err)
	}
	app.publisher.Close()
	app.publisher = f.engine
	t.Cleanup(app.Close)
	server := httptest.NewServer(app)
	t.Cleanup(server.Close)
	client := newCookieClient(t)
	csrf := loginNormalAdmin(t, client, server.URL, "owner@example.test", "ownerpass1")
	return f, server, client, csrf
}
func TestNumericCommandUsesServerValidatedAdoption(t *testing.T) {
	f, server, client, csrf := numericHTTPFixture(t)
	response := postAdminJSON(t, client, server.URL+"/admin/api/spec-values/"+f.valueID+"/normalized", csrf, `{"source":"automatic","status":"current"}`)
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("client trust markers status=%d", response.StatusCode)
	}
}
func TestNumericCommandAdoptsValidatedValue(t *testing.T) {
	f, server, client, csrf := numericHTTPFixture(t)
	response, err := client.Get(server.URL + "/admin/api/spec-values/" + f.valueID + "/normalization-context")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("context status=%d", response.StatusCode)
	}
	var c catalog.SpecNormalizationContext
	if err = json.NewDecoder(response.Body).Decode(&c); err != nil {
		t.Fatal(err)
	}
	if c.SpecValueID != f.valueID || c.SourceSelection.SpecValueID != f.valueID || c.SourceSelection.Revision < 1 {
		t.Fatalf("context=%+v", c)
	}
	value, err := specnumeric.MarshalValue(*specnumeric.Normalize(c.Raw, c.Profile).Candidate)
	if err != nil {
		t.Fatal(err)
	}
	unsupported, _ := json.Marshal(map[string]any{"expected": c.Expected, "value_json": strings.Replace(value, `"unit":"V"`, `"unit":"A"`, 1)})
	rejected := postAdminJSON(t, client, server.URL+"/admin/api/spec-values/"+f.valueID+"/normalized", csrf, string(unsupported))
	if body := responseBody(t, rejected); rejected.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unsupported manual unit status=%d body=%s", rejected.StatusCode, body)
	}
	body, _ := json.Marshal(map[string]any{"expected": c.Expected, "value_json": value})
	response = postAdminJSON(t, client, server.URL+"/admin/api/spec-values/"+f.valueID+"/normalized", csrf, string(body))
	defer response.Body.Close()
	if response.StatusCode != 201 {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("adoption status=%d body=%s", response.StatusCode, data)
	}
	var outcome catalog.NormalizationOutcome
	if err = json.NewDecoder(response.Body).Decode(&outcome); err != nil {
		t.Fatal(err)
	}
	if outcome.NormalizedValueID == "" || outcome.ProductRevision <= c.Expected.ProductRevision || outcome.SelectionRevision <= c.Expected.SelectionRevision {
		t.Fatalf("outcome=%+v", outcome)
	}
	// A stale command remains a conflict even when its payload is invalid.
	body, _ = json.Marshal(map[string]any{"expected": c.Expected, "value_json": "{}"})
	response = postAdminJSON(t, client, server.URL+"/admin/api/spec-values/"+f.valueID+"/normalized", csrf, string(body))
	defer response.Body.Close()
	if response.StatusCode != 409 {
		t.Fatalf("stale status=%d", response.StatusCode)
	}
}
func TestPublicSpecSearchRejectsUnsupportedOperator(t *testing.T) {
	f, server, _, _ := numericHTTPFixture(t)
	c, err := f.store.SpecNormalizationContext(t.Context(), f.valueID)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"predicates":[{"spec_id":"` + c.SpecID + `","operator":"approximately","value":"4","unit":"V"}],"page":1,"page_size":20}`
	response, err := http.Post(server.URL+"/api/search/specs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 422 {
		t.Fatalf("unsupported status=%d", response.StatusCode)
	}
	// A known operator rejected by the frozen profile must retain the typed
	// unsupported classification through the publishing engine.
	body = strings.Replace(body, `"operator":"approximately"`, `"operator":"contains_point"`, 1)
	body = strings.Replace(body, `"unit":"V"`, `"unit":"A"`, 1)
	response, err = http.Post(server.URL+"/api/search/specs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unsupported scoped unit status=%d", response.StatusCode)
	}

}
func TestPublicSpecSearchReturnsFrozenRows(t *testing.T) {
	f, server, _, _ := numericHTTPFixture(t)
	request := specSearchRequest(t, f, "4")
	body, _ := json.Marshal(map[string]any{"predicates": request.Predicates, "page": 1, "page_size": 20})
	response, err := http.Post(server.URL+"/api/search/specs", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if response.StatusCode != 200 {
		t.Fatalf("search status=%d body=%s", response.StatusCode, data)
	}
	var result publishing.SpecSearchResult
	if err = json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || len(result.Rows) != 1 || result.Rows[0].ID != f.productID {
		t.Fatalf("result=%+v", result)
	}
	if result.Rows[0].Language != f.view(t).DefaultLocale {
		t.Fatalf("search locale=%s", result.Rows[0].Language)
	}
	for _, private := range []string{"updated_by", "normalizer_version", "failure_reason", "source_origin_locale"} {
		if strings.Contains(string(data), `"`+private+`"`) {
			t.Fatalf("private field %s leaked", private)
		}
	}
}

// The engine must also enforce locale admission: an HTTP policy snapshot can
// precede activation, and an omitted locale is not an explicit old default.
func TestPublicSpecSearchUsesAdmittedLocalePolicy(t *testing.T) {
	f := newNumericPublicationFixture(t)
	request := specSearchRequest(t, f, "4")
	website, err := f.store.WebsiteState(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	working, err := f.store.SaveWebsiteLocalization(t.Context(), f.actor, website.WorkingRevision, site.WebsiteLocalization{DefaultLocale: "zh-TW", EnabledLocales: []string{"zh-TW"}, ContentEditingEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.engine.SearchSpecs(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || result.Rows[0].Language != "en-US" {
		t.Fatal("working locale affected active search")
	}
	epoch, config, err := f.store.PublicSiteRouteConfig(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.engine.PublishSiteRoutes(t.Context(), publishing.SiteRouteRequest{ActorID: f.actor, ExpectedEpoch: epoch, ExpectedWorkingRevision: working.WorkingRevision, Config: config}); err != nil {
		t.Fatal(err)
	}
	result, err = f.engine.SearchSpecs(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || result.Rows[0].Language != "zh-TW" {
		t.Fatal("omitted locale did not resolve newly admitted default")
	}
	request.Locale = "en-US"
	if _, err = f.engine.SearchSpecs(t.Context(), request); !errors.Is(err, publishing.ErrInvalidSpecSearch) {
		t.Fatalf("revoked locale admitted: %v", err)
	}
}

func TestPublicSpecSearchRejectsNullPointOperand(t *testing.T) {
	f, server, _, _ := numericHTTPFixture(t)
	request := specSearchRequest(t, f, "4")
	body := `{"predicates":[{"spec_id":"` + request.Predicates[0].SpecID + `","operator":"contains_point","value":"4","lower":null,"unit":"V"}],"page":1,"page_size":20}`
	response, err := http.Post(server.URL+"/api/search/specs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, response); response.StatusCode != http.StatusBadRequest {
		t.Fatalf("forbidden null point operand status=%d body=%s", response.StatusCode, body)
	}
}

func TestPublicSpecSearchRejectsNullRangeOperand(t *testing.T) {
	f, server, _, _ := numericHTTPFixture(t)
	request := specSearchRequest(t, f, "4")
	body := `{"predicates":[{"spec_id":"` + request.Predicates[0].SpecID + `","operator":"contains_range","lower":"3","upper":"5","value":null,"unit":"V"}],"page":1,"page_size":20}`
	response, err := http.Post(server.URL+"/api/search/specs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	// Reject the wire shape before consulting this profile's allowed operators.
	if body := responseBody(t, response); response.StatusCode != http.StatusBadRequest {
		t.Fatalf("forbidden null range operand status=%d body=%s", response.StatusCode, body)
	}
}

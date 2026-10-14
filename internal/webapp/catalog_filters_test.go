package webapp

import (
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestPublicCatalogFilterFormWorksWithoutJavaScript(t *testing.T) {
	f, server, client, _ := numericHTTPFixture(t)
	p, err := f.store.Product(t.Context(), f.productID)
	if err != nil {
		t.Fatal(err)
	}
	p.PackageFormFactor = "QFN-32"
	if _, err = f.store.UpdateProduct(t.Context(), f.actor, p.Revision, p); err != nil {
		t.Fatal(err)
	}
	f.drain(t)
	c := f.view(t)
	field := "spec:" + c.NumericDefinitions[0].SpecID
	values := url.Values{"f.package.op": {"contains"}, "f.package.value": {"QFN"}, "f." + field + ".op": {"contains_point"}, "f." + field + ".value": {"4"}, "f." + field + ".unit": {"V"}, "page_size": {"1"}}
	response, err := client.Get(server.URL + "/catalog?" + values.Encode())
	if err != nil {
		t.Fatal(err)
	}
	body := responseBody(t, response)
	if response.StatusCode != http.StatusOK || !strings.Contains(body, `name="f.`+field+`.value"`) || !strings.Contains(body, `name="f.package.value"`) || !strings.Contains(body, "NUMERIC-PUBLIC") || !strings.Contains(body, `method="get"`) {
		t.Fatalf("no-JS form status=%d body=%s", response.StatusCode, body)
	}
	values.Set("f."+field+".value", "8")
	response, err = client.Get(server.URL + "/catalog?" + values.Encode())
	if err != nil {
		t.Fatal(err)
	}
	body = responseBody(t, response)
	if response.StatusCode != http.StatusOK || strings.Contains(body, `data-product-row="`+f.productID+`"`) || !strings.Contains(body, `value="8"`) {
		t.Fatalf("numeric condition not applied: %d %s", response.StatusCode, body)
	}
	req := httptest.NewRequest("GET", "/catalog?"+values.Encode(), nil)
	next, err := url.Parse(listingPageURL(req, 2))
	if err != nil {
		t.Fatal(err)
	}
	if next.Query().Get("f."+field+".value") != "8" || next.Query().Get("f.package.value") != "QFN" {
		t.Fatalf("pagination lost conditions: %s", next)
	}
}

func TestUnknownCatalogFilterPreservesQuery(t *testing.T) {
	_, server, client, _ := numericHTTPFixture(t)
	raw := "KEEP <part> & edits"
	values := url.Values{"q": {raw}, "f.spec:unknown.op": {"contains"}, "f.spec:unknown.value": {"original"}}
	response, err := client.Get(server.URL + "/catalog?" + values.Encode())
	if err != nil {
		t.Fatal(err)
	}
	body := responseBody(t, response)
	if response.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, html.EscapeString(raw)) || !strings.Contains(body, "spec:unknown") || !strings.Contains(body, "original") {
		t.Fatalf("unknown condition lost input: %d %s", response.StatusCode, body)
	}
}

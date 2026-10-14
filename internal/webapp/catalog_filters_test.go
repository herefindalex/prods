package webapp

import (
	"bytes"
	"fmt"
	dom "golang.org/x/net/html"
	"html"
	tmpl "html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"prods/internal/catalogfilter"
	"prods/internal/localization"
	"prods/internal/publishing"
	"prods/internal/site"
	"prods/internal/specnumeric"
	"strings"
	"testing"
)

func TestCatalogCustomSpecWithPeriodCanBeSubmitted(t *testing.T) {
	v := publishing.PublicView{ID: "p", PartNumber: "ABC", Language: "en-US", SupportedLocales: []string{"en-US"}, ApplicableSpecifications: []publishing.ApplicableSpecification{{ID: "spc.custom", Name: "Custom"}}, Specifications: []publishing.Specification{{ID: "spc.custom", RawValue: "SPI"}}, FilterDefinitions: []publishing.PublicFilterDefinition{{Field: "spec:spc.custom", SemanticVersion: 1, ProfileRevision: 1, Profile: catalogfilter.DefaultTextProfile()}}}
	snapshot, err := publishing.AdmitCatalogViews(t.Context(), []publishing.PublicView{v}, site.DefaultConfiguration(), publishing.CatalogQuery{Scope: catalogfilter.Scope{Kind: "all"}})
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	template, err := tmpl.New("filters").ParseFS(content, "templates/catalog_filters.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	page := catalogPage{publicPage: publicPage{Text: localization.For("en-US"), Language: "en-US"}, FormAction: "/catalog", FilterControls: catalogFilterForm(snapshot, nil)}
	if err := template.ExecuteTemplate(&body, "catalog-filter-panel", page); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body.String(), `name="f.spec:spc.custom.value"`) {
		t.Fatal("custom field missing from rendered form")
	}
	clauses, err := parseCatalogFilters(url.Values{"f.spec:spc.custom.op": {"contains"}, "f.spec:spc.custom.value": {"SPI"}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := publishing.AdmitCatalogViews(t.Context(), []publishing.PublicView{v}, site.DefaultConfiguration(), publishing.CatalogQuery{Scope: catalogfilter.Scope{Kind: "all"}, Clauses: clauses})
	if err != nil || len(result.Views) != 1 {
		t.Fatalf("custom field submission: %+v %v", result.Views, err)
	}
}

func TestCatalogFieldFormPreservesOtherDottedSpec(t *testing.T) {
	snapshot := publishing.CatalogSnapshot{Locale: "en-US", Definitions: []publishing.PublicFilterDefinition{{Field: "spec:spc", Profile: catalogfilter.DefaultTextProfile()}, {Field: "spec:spc.custom", Profile: catalogfilter.DefaultTextProfile()}}}
	values := url.Values{"f.spec:spc.custom.op": {"contains"}, "f.spec:spc.custom.value": {"SPI"}}
	control := catalogFilterForm(snapshot, values)[0]
	submission := url.Values{}
	for _, hidden := range control.Hidden {
		submission.Add(hidden.Name, hidden.Value)
	}
	if submission.Get("f.spec:spc.custom.value") != "SPI" {
		t.Fatalf("other custom condition lost: %v", submission)
	}
	clear, err := url.Parse(control.ClearURL)
	if err != nil || clear.Query().Get("f.spec:spc.custom.value") != "SPI" {
		t.Fatalf("clear removed another condition: %s %v", control.ClearURL, err)
	}
}

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

func TestCatalogLargeFieldPanelSubmitsBoundedConditions(t *testing.T) {
	snapshot := publishing.CatalogSnapshot{Locale: "en-US"}
	var profile specnumeric.Profile
	for _, p := range specnumeric.SupportedProfiles() {
		if p.Quantity == "voltage" && p.SemanticRole == specnumeric.RoleOperatingSupport {
			profile = p
			break
		}
	}
	for i := range 34 {
		id := fmt.Sprintf("spc_%032d", i)
		snapshot.Definitions = append(snapshot.Definitions, publishing.PublicFilterDefinition{Field: "spec:" + id, Profile: catalogfilter.NumericProfile(profile)})
	}
	values := url.Values{"q": {"KEEP"}, "f.package.op": {"contains"}, "f.package.value": {"QFN"}, "sort": {"name"}, "direction": {"desc"}, "page_size": {"10"}, "page": {"2"}}
	page := catalogPage{publicPage: publicPage{Text: localization.For("en-US"), Language: "en-US"}, Query: "KEEP", FormAction: "/catalog", FilterControls: catalogFilterForm(snapshot, values), FilterHidden: catalogFilterHiddenFields(values)}
	template, err := tmpl.New("filters").ParseFS(content, "templates/catalog_filters.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	if err = template.ExecuteTemplate(&body, "catalog-filter-panel", page); err != nil {
		t.Fatal(err)
	}
	tree, err := dom.Parse(&body)
	if err != nil {
		t.Fatal(err)
	}
	attr := func(node *dom.Node, key string) string {
		for _, a := range node.Attr {
			if a.Key == key {
				return a.Val
			}
		}
		return ""
	}
	var visit func(*dom.Node)
	forms := 0
	visit = func(node *dom.Node) {
		if node.Type == dom.ElementNode && node.Data == "form" {
			forms++
			submission := url.Values{}
			var controls func(*dom.Node)
			controls = func(n *dom.Node) {
				name := attr(n, "name")
				if n.Data == "input" && name != "" {
					submission.Add(name, attr(n, "value"))
				}
				if n.Data == "select" && name != "" {
					for child := n.FirstChild; child != nil; child = child.NextSibling {
						if child.Type == dom.ElementNode && child.Data == "option" {
							submission.Add(name, attr(child, "value"))
							break
						}
					}
				}
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					controls(c)
				}
			}
			controls(node)
			if len(submission.Encode()) > 8192 {
				t.Fatalf("34-field native form exceeds request budget: %d bytes", len(submission.Encode()))
			}
			if submission.Get("q") != "KEEP" || submission.Get("f.package.value") != "QFN" {
				t.Fatalf("form lost existing conditions: %v", submission)
			}
			if submission.Get("sort") != "name" || submission.Get("direction") != "desc" || submission.Get("page_size") != "10" || submission.Has("page") {
				t.Fatalf("form lost sorting or did not reset pagination: %v", submission)
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(tree)
	if forms == 0 {
		t.Fatal("no usable native forms")
	}
}

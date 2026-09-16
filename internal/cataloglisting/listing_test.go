package cataloglisting

import (
	"reflect"
	"testing"

	"prods/internal/catalog"
	"prods/internal/publishing"
	"prods/internal/site"
)

func TestCommonUsesApplicabilityAcrossFullScopeNotCurrentPageValues(t *testing.T) {
	input := []publishing.PublicView{
		listingView("ldo-1", "LDO-1", "cat_ldo", []publishing.ApplicableSpecification{spec("vin"), spec("vout"), spec("iout"), spec("iq")}, map[string]string{"vin": "3-6 V", "vout": "3.3 V"}),
		listingView("buck-1", "BUCK-1", "cat_buck", []publishing.ApplicableSpecification{spec("vin"), spec("vout"), spec("iout"), spec("freq")}, map[string]string{"vin": "4-18 V", "vout": "5 V", "iout": "2 A"}),
	}
	result, err := Resolve(input, site.DefaultConfiguration(), Request{Scope: Scope{Kind: ScopeAll}, Page: 1, PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	var specKeys []string
	for _, column := range result.Columns {
		if column.Kind == "spec" {
			specKeys = append(specKeys, column.Key)
		}
	}
	if !reflect.DeepEqual(specKeys, []string{"spec:iout", "spec:vin", "spec:vout"}) {
		t.Fatalf("common columns = %v", specKeys)
	}
	if result.Total != 2 || len(result.Rows) != 1 || result.Rows[0].ProductID != "buck-1" {
		t.Fatalf("first page = %+v", result)
	}
	second, err := Resolve(input, site.DefaultConfiguration(), Request{Scope: Scope{Kind: ScopeAll}, Page: 2, PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Columns, second.Columns) {
		t.Fatal("columns changed across pages")
	}
	if len(second.Rows) != 1 || second.Rows[0].ProductID != "ldo-1" || second.Rows[0].Values["spec:iout"] != "" {
		t.Fatalf("missing common value must remain blank = %+v", second)
	}
}

func TestCategoryProfilesAreIndependentAndCannotForceNonCommonSpec(t *testing.T) {
	views := []publishing.PublicView{
		listingView("one", "ONE", "cat_one", []publishing.ApplicableSpecification{spec("vin"), spec("vout")}, map[string]string{"vin": "5 V", "vout": "3.3 V"}),
		listingView("two", "TWO", "cat_two", []publishing.ApplicableSpecification{spec("vin"), spec("vout")}, map[string]string{"vin": "12 V", "vout": "5 V"}),
	}
	configuration := site.DefaultConfiguration()
	configuration.CategoryListingProfiles = map[string]site.CategoryListingProfile{
		"cat_one": {VisibleColumns: []string{"part_number", "spec:vout", "spec:vin"}, MobileKeySpecs: []string{"spec:vout"}},
		"cat_two": {VisibleColumns: []string{"part_number", "spec:vin", "spec:vout"}, MobileKeySpecs: []string{"spec:vin"}},
	}
	first, err := Resolve(views, configuration, Request{Scope: Scope{Kind: ScopeCategory, ID: "cat_one"}, Page: 1})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Resolve(views, configuration, Request{Scope: Scope{Kind: ScopeCategory, ID: "cat_two"}, Page: 1})
	if err != nil {
		t.Fatal(err)
	}
	if first.Columns[1].Key != "spec:vout" || second.Columns[1].Key != "spec:vin" || !first.Columns[1].MobileKey || !second.Columns[1].MobileKey {
		t.Fatalf("independent profiles first=%+v second=%+v", first.Columns, second.Columns)
	}
}

func TestCategoryNavigationCountsDirectAndDescendantsWithoutRevokedViews(t *testing.T) {
	categories := []catalog.Category{
		{ID: "cat_parent", ParentID: "cat_root", Name: "Parent", Slug: "parent", Status: catalog.EntryActive},
		{ID: "cat_child", ParentID: "cat_parent", Name: "Child", Slug: "child", Status: catalog.EntryActive},
		{ID: "cat_empty", ParentID: "cat_root", Name: "Empty", Slug: "empty", Status: catalog.EntryActive},
	}
	views := []publishing.PublicView{
		{ID: "direct", CategoryID: "cat_parent", CategoryTrail: []publishing.CategoryRef{{ID: "cat_parent"}}},
		{ID: "child", CategoryID: "cat_child", CategoryTrail: []publishing.CategoryRef{{ID: "cat_parent"}, {ID: "cat_child"}}},
	}
	nodes := BuildCategoryNavigation(categories, views)
	byID := map[string]CategoryNode{}
	for _, node := range nodes {
		byID[node.ID] = node
	}
	if byID["cat_parent"].DirectPublic != 1 || byID["cat_parent"].SubtreePublic != 2 || byID["cat_child"].SubtreePublic != 1 || byID["cat_empty"].SubtreePublic != 0 {
		t.Fatalf("navigation counts = %+v", byID)
	}
}

func listingView(id, part, categoryID string, applicable []publishing.ApplicableSpecification, values map[string]string) publishing.PublicView {
	view := publishing.PublicView{ID: id, Revision: 1, PartNumber: part, Name: part + " name", CategoryID: categoryID, Category: categoryID, CanonicalURL: "/products/" + id, RFQURL: "/rfq?product_id=" + id, Language: "en-US", DefaultLocale: "en-US", SupportedLocales: []string{"en-US"}, ApplicableSpecifications: applicable}
	for specID, value := range values {
		view.Specifications = append(view.Specifications, publishing.Specification{ID: specID, Name: specID, RawValue: value})
	}
	return view
}

func spec(id string) publishing.ApplicableSpecification {
	return publishing.ApplicableSpecification{ID: id, Name: id}
}

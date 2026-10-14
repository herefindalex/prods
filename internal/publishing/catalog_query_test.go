package publishing

import (
	"fmt"
	"prods/internal/catalogfilter"
	"prods/internal/site"
	"prods/internal/specnumeric"
	"testing"
)

func TestCatalogCombinesTextAndNumericBeforePagination(t *testing.T) {
	base := blevePublication(t).View
	base.Language = "en-US"
	base.DefaultLocale = "en-US"
	base.SupportedLocales = []string{"en-US"}
	base.FilterDefinitions = []PublicFilterDefinition{{Field: "spec:voltage/opaque", SemanticVersion: 1, ProfileRevision: 1, Profile: catalogfilter.NumericProfile(base.NumericDefinitions[0].Profile)}, {Field: "spec:interface", SemanticVersion: 1, ProfileRevision: 1, Profile: catalogfilter.DefaultTextProfile()}}
	e := &Engine{index: &publicIndex{byID: map[string]ActivePublication{}, site: site.DefaultConfiguration()}}
	for i := range 30 {
		v := base
		v.ID = fmt.Sprintf("p%02d", i)
		v.PartNumber = v.ID
		raw := "SPI"
		if i >= 27 {
			raw = "I2C"
		}
		v.Specifications = []Specification{{ID: "interface", RawValue: raw}}
		e.index.views = append(e.index.views, v)
		e.index.byID[v.ID] = ActivePublication{ProductID: v.ID, View: v}
	}
	value := bleveDecimal(t, "3.2")
	q := CatalogQuery{Scope: catalogfilter.Scope{Kind: "all"}, Clauses: []catalogfilter.Clause{{Field: "spec:interface", Operator: "equals", Values: []string{"spi"}}, {Field: "spec:voltage/opaque", Operator: "contains_point", Numeric: &specnumeric.Predicate{SpecID: "voltage/opaque", Operator: specnumeric.OperatorContainsPoint, Value: &value, Unit: "V"}}}}
	got, err := e.AdmitCatalog(t.Context(), q)
	if err != nil || len(got.Views) != 27 || len(got.ScopeViews) != 30 {
		t.Fatalf("combined=%d scope=%d err=%v", len(got.Views), len(got.ScopeViews), err)
	}
	if got.Views[24].ID != "p24" || len(got.Views[24:]) != 3 {
		t.Fatalf("second page: %+v", got.Views[24:])
	}
}

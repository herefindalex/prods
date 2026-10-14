package publishing

import (
	"prods/internal/catalogfilter"
	"testing"
)

func TestCommonFiltersRejectMixedSemanticVersions(t *testing.T) {
	d := PublicFilterDefinition{Field: "spec:interface", SemanticVersion: 1, ProfileRevision: 2, Profile: catalogfilter.DefaultTextProfile()}
	a := PublicView{FilterDefinitions: []PublicFilterDefinition{d}}
	t.Run("mixed meaning", func(t *testing.T) {
		changed := d
		changed.SemanticVersion++
		if got := CommonFilterDefinitions([]PublicView{a, {FilterDefinitions: []PublicFilterDefinition{changed}}}); len(got) != 0 {
			t.Fatalf("mixed meanings: %+v", got)
		}
	})
	t.Run("missing raw remains applicable", func(t *testing.T) {
		b := a
		b.Specifications = []Specification{{ID: "interface", RawValue: "SPI"}}
		if got := CommonFilterDefinitions([]PublicView{a, b}); len(got) != 1 {
			t.Fatalf("missing Raw removed field: %+v", got)
		}
	})
}

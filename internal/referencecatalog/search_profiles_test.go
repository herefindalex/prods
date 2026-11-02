package referencecatalog

import (
	"prods/internal/catalogfilter"
	"prods/internal/specnumeric"
	"testing"
)

func TestSampleSearchProfilesCoverAllDefinitions(t *testing.T) {
	dataset, _, err := Build(DefaultSeed)
	if err != nil {
		t.Fatal(err)
	}
	if len(dataset.Specs) != 34 || len(dataset.SpecSearchProfiles) != 34 {
		t.Fatalf("complete profile inventory: specs=%d profiles=%d", len(dataset.Specs), len(dataset.SpecSearchProfiles))
	}
	for _, spec := range dataset.Specs {
		profile, ok := dataset.SpecSearchProfiles[spec.ID]
		if !ok {
			t.Fatalf("missing %s", spec.ID)
		}
		if err := catalogfilter.ValidateProfile(profile); err != nil {
			t.Fatalf("%s: %v", spec.ID, err)
		}
		if profile.Numeric != nil {
			supported := false
			for _, product := range dataset.Products {
				for _, value := range product.SpecValues {
					if value.SpecID == spec.ID && specnumeric.Normalize(value.RawValue, *profile.Numeric).Candidate != nil {
						supported = true
						break
					}
				}
				if supported {
					break
				}
			}
			if !supported {
				t.Fatalf("%s has no supported realistic sample", spec.ID)
			}
		}
		if spec.ID == "spc_package" && spec.Filterable {
			t.Fatal("Package Spec became filterable")
		}
	}
	for _, id := range []string{"spc_measurement_range", "spc_contact_rating"} {
		if dataset.SpecSearchProfiles[id].Kind != "text" {
			t.Fatalf("mixed quantities inferred: %s", id)
		}
	}
}

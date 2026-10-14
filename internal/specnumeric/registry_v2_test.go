package specnumeric

import "testing"

func TestCatalogValueRoundTrip(t *testing.T) {
	p := catalogProfileForTest("ratio", "%")
	p.SemanticRole = RoleErrorMagnitude
	p.AllowedShapes = []Shape{ShapeBoundClaim}
	p.AllowedOperators = []Operator{OperatorLessEqual}
	result := Normalize("±0.5 %", p)
	if result.Candidate == nil {
		t.Fatalf("normalize: %+v", result)
	}
	encoded, err := MarshalValue(*result.Candidate)
	if err != nil {
		t.Fatal(err)
	}
	value, err := UnmarshalValue(encoded, p)
	if err != nil {
		t.Fatal(err)
	}
	ceiling, _ := ParseDecimal("1")
	if Evaluate(&value, p, Predicate{SpecID: "accuracy", Operator: OperatorLessEqual, Value: &ceiling, Unit: "%"}) != Match {
		t.Fatal("stored accuracy lost meaning")
	}
}

func catalogProfileForTest(quantity, unit string) Profile {
	p := electricalProfile(quantity, unit)
	p.ParserVersion, p.UnitRegistryVersion = "catalog-parser-2", "catalog-units-2"
	return p
}

func TestCatalogRegistryConvertsDeclaredQuantities(t *testing.T) {
	for _, tc := range []struct{ quantity, canonical, raw, want string }{
		{"ratio", "%", "95 %", "95"},
		{"log_gain", "dB", "20 dB", "20"},
		{"memory", "bit", "1 kB", "8000"},
		{"memory", "bit", "1 KiB", "8192"},
		{"power", "W", "250 mW", "0.25"},
		{"length", "m", "2.54 mm", "0.00254"},
		{"inductance", "H", "10 µH", "0.00001"},
		{"luminous_intensity", "cd", "500 mcd", "0.5"},
		{"count", "1", "24", "24"},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			p := catalogProfileForTest(tc.quantity, tc.canonical)
			if tc.quantity == "count" {
				p.DeclaredInputUnit = "1"
			}
			r := Normalize(tc.raw, p)
			if r.Reason != "" || r.Candidate == nil || r.Candidate.Scalar == nil || r.Candidate.Scalar.String() != tc.want {
				t.Fatalf("Normalize(%q) = %+v, want scalar %s %s", tc.raw, r, tc.want, tc.canonical)
			}
		})
	}
}

func TestExtendedRegistryPreservesLegacyProfiles(t *testing.T) {
	p := electricalProfile("voltage", "V")
	r := Normalize("5000 mV", p)
	if r.Reason != "" || r.Candidate == nil || r.Candidate.Scalar.String() != "5" {
		t.Fatalf("legacy conversion: %+v", r)
	}
	p.Quantity, p.CanonicalUnit = "ratio", "%"
	if err := ValidateProfile(p); err == nil {
		t.Fatal("old registry acquired a new percentage meaning")
	}
}

func TestErrorMagnitudeUsesMaximumError(t *testing.T) {
	p := catalogProfileForTest("ratio", "%")
	p.SemanticRole, p.AllowedShapes, p.AllowedOperators = "error_magnitude", []Shape{ShapeBoundClaim}, []Operator{OperatorLessEqual}
	threshold := decimalForTest(t, "1")
	q := Predicate{SpecID: "accuracy", Operator: OperatorLessEqual, Unit: "%", Value: &threshold}
	for _, tc := range []struct {
		raw  string
		want Truth
	}{{"±0.5 %", Match}, {"1.5 %", NoMatch}} {
		r := Normalize(tc.raw, p)
		if r.Reason != "" || r.Candidate == nil {
			t.Fatalf("normalize %s: %+v", tc.raw, r)
		}
		if got := Evaluate(r.Candidate, p, q); got != tc.want {
			t.Errorf("%s <= 1%%: %s, want %s", tc.raw, got, tc.want)
		}
	}
}

func TestCountRejectsFractionalFacts(t *testing.T) {
	p := catalogProfileForTest("count", "1")
	p.DeclaredInputUnit = "1"
	if err := ValidateProfile(p); err != nil {
		t.Fatal(err)
	}
	r := Normalize("1.5", p)
	if r.Reason == "" || r.Candidate != nil {
		t.Fatalf("fractional count accepted: %+v", r)
	}
	v := Value{SchemaVersion: 1, Quantity: "count", SemanticRole: RoleOperatingSupport, Shape: ShapeScalar, Unit: "1", UnitRegistryVersion: "catalog-units-2", Context: Context{Kind: ContextUnconditional}}
	n := decimalForTest(t, "1.5")
	v.Scalar = &n
	if err := ValidateValue(v, p); err == nil {
		t.Fatal("manual fractional count accepted")
	}
}

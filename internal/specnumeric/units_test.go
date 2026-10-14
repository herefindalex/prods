package specnumeric

import "testing"

func electricalProfile(quantity, unit string) Profile {
	return Profile{Quantity: quantity, SemanticRole: "operating_support", AllowedShapes: []Shape{ShapeScalar}, AllowedOperators: []Operator{OperatorContainsPoint}, CanonicalUnit: unit, NumericFormat: NumericFormatDecimalDot, SchemaVersion: SchemaVersion, ParserVersion: ParserVersion, UnitRegistryVersion: UnitRegistryVersion}
}

func decimalForTest(t *testing.T, text string) Decimal {
	t.Helper()
	value, err := ParseDecimal(text)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestElectricalUnitsPreservePrefixCase(t *testing.T) {
	profile := electricalProfile("resistance", "Ω")
	for _, tc := range []struct{ unit, want string }{{"mΩ", "0.001"}, {"MΩ", "1000000"}} {
		got, err := Convert(decimalForTest(t, "1"), tc.unit, profile)
		if err != nil {
			t.Fatal(err)
		}
		if got.String() != tc.want {
			t.Errorf("1 %s = %s Ω, want %s", tc.unit, got, tc.want)
		}
	}
}

func TestCapacitanceEquivalentUnits(t *testing.T) {
	profile := electricalProfile("capacitance", "F")
	left, err := Convert(decimalForTest(t, "0.1"), "µF", profile)
	if err != nil {
		t.Fatal(err)
	}
	right, err := Convert(decimalForTest(t, "100"), "nF", profile)
	if err != nil {
		t.Fatal(err)
	}
	if left.Cmp(right) != 0 || left.String() != "0.0000001" {
		t.Fatalf("0.1 µF = %s F; 100 nF = %s F, want 0.0000001 F", left, right)
	}
}

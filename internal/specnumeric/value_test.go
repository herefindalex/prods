package specnumeric

import (
	"errors"
	"strings"
	"testing"
)

func TestDecimalRejectsUnsupportedPrecision(t *testing.T) {
	if _, err := ParseDecimal("1.2345678901234567890123456789012345"); !errors.Is(err, ErrUnsupportedPrecision) {
		t.Fatalf("35 significant digits: got %v, want unsupported precision", err)
	}
}

func TestDecimalCanonicalizesZero(t *testing.T) {
	value, err := ParseDecimal("-0.00000000000000000000000000")
	if err != nil || value.String() != "0" {
		t.Fatalf("exact zero: got %q, %v; want 0", value.String(), err)
	}
}

func TestConversionRejectsUnsupportedExponent(t *testing.T) {
	_, err := Convert(decimalForTest(t, "1e24"), "kV", electricalProfile("voltage", "V"))
	if !errors.Is(err, ErrUnsupportedPrecision) {
		t.Fatalf("converted exponent 27: got %v, want unsupported precision", err)
	}
}

func TestProfileRequiresExplicitSemanticRole(t *testing.T) {
	profile := electricalProfile("voltage", "V")
	profile.SemanticRole = ""
	if err := ValidateProfile(profile); err == nil {
		t.Fatal("blank semantic role was accepted")
	}
}

func TestControlledValueRejectsMixedShapes(t *testing.T) {
	profile := electricalProfile("voltage", "V")
	encoded := `{"schema_version":1,"quantity":"voltage","semantic_role":"operating_support","shape":"scalar","unit":"V","unit_registry_version":"electrical-1","scalar":"3.3","values":["5"],"context":{"kind":"unconditional"}}`
	if _, err := UnmarshalValue(encoded, profile); err == nil {
		t.Fatal("scalar carrying a discrete set was accepted")
	}
}

func TestControlledValueRejectsUnrelatedNullPayload(t *testing.T) {
	profile := electricalProfile("voltage", "V")
	encoded := `{"schema_version":1,"quantity":"voltage","semantic_role":"operating_support","shape":"scalar","unit":"V","unit_registry_version":"electrical-1","scalar":"3.3","bound":null,"context":{"kind":"unconditional"}}`
	if _, err := UnmarshalValue(encoded, profile); err == nil {
		t.Fatal("scalar carrying a null bound payload was accepted")
	}
}

func TestControlledValueRejectsCaseVariantNullPayload(t *testing.T) {
	profile := electricalProfile("voltage", "V")
	encoded := `{"schema_version":1,"quantity":"voltage","semantic_role":"operating_support","shape":"scalar","unit":"V","unit_registry_version":"electrical-1","scalar":"3.3","BOUND":null,"context":{"kind":"unconditional"}}`
	if _, err := UnmarshalValue(encoded, profile); err == nil {
		t.Fatal("case-variant incompatible payload was accepted")
	}
}

func TestControlledIntervalRequiresExplicitEndpointFlags(t *testing.T) {
	profile := electricalProfile("voltage", "V")
	profile.AllowedShapes = []Shape{ShapeIntervalSet}
	encoded := `{"schema_version":1,"quantity":"voltage","semantic_role":"operating_support","shape":"interval_set","unit":"V","unit_registry_version":"electrical-1","intervals":[{"lower":"3","upper":"3.6"}],"context":{"kind":"unconditional"}}`
	if _, err := UnmarshalValue(encoded, profile); err == nil {
		t.Fatal("omitted interval flags were accepted as exclusive endpoints")
	}
}

func TestControlledBoundRequiresExplicitInclusionFlag(t *testing.T) {
	profile := electricalProfile("voltage", "V")
	profile.SemanticRole = RoleMaximumLimit
	profile.AllowedShapes = []Shape{ShapeBoundClaim}
	encoded := `{"schema_version":1,"quantity":"voltage","semantic_role":"maximum_limit","shape":"bound_claim","unit":"V","unit_registry_version":"electrical-1","bound":{"direction":"upper","value":"5","inclusive":null},"context":{"kind":"unconditional"}}`
	if _, err := UnmarshalValue(encoded, profile); err == nil {
		t.Fatal("null bound flag was accepted as an exclusive bound")
	}
}

func TestControlledToleranceRejectsContradictoryInterval(t *testing.T) {
	profile := electricalProfile("voltage", "V")
	profile.SemanticRole = RoleNominal
	profile.AllowedShapes = []Shape{ShapeTolerance}
	encoded := `{"schema_version":1,"quantity":"voltage","semantic_role":"nominal","shape":"tolerance","unit":"V","unit_registry_version":"electrical-1","tolerance":{"nominal":"-5","type":"relative","amount":"10","interval":{"lower":"-5.5","upper":"-4.5","lower_inclusive":true,"upper_inclusive":true}},"context":{"kind":"unconditional"}}`
	if _, err := UnmarshalValue(encoded, profile); err != nil {
		t.Fatal(err)
	}
	if _, err := UnmarshalValue(strings.Replace(encoded, `"lower":"-5.5"`, `"lower":"-6"`, 1), profile); err == nil {
		t.Fatal("tolerance's derived interval contradicted its nominal and amount")
	}
}

func TestControlledValuePreservesIntervalGap(t *testing.T) {
	profile := electricalProfile("voltage", "V")
	profile.AllowedShapes = []Shape{ShapeIntervalSet}
	encoded := `{"schema_version":1,"quantity":"voltage","semantic_role":"operating_support","shape":"interval_set","unit":"V","unit_registry_version":"electrical-1","intervals":[{"lower":"3","upper":"3.6","lower_inclusive":true,"upper_inclusive":true},{"lower":"4.5","upper":"5.5","lower_inclusive":false,"upper_inclusive":true}],"context":{"kind":"unconditional"}}`
	value, err := UnmarshalValue(encoded, profile)
	if err != nil {
		t.Fatal(err)
	}
	got, err := MarshalValue(value)
	if err != nil {
		t.Fatal(err)
	}
	if got != encoded {
		t.Fatalf("round trip = %s, want %s", got, encoded)
	}
	if _, err := UnmarshalValue(strings.Replace(encoded, `"upper":"3.6"`, `"upper":"2"`, 1), profile); err == nil {
		t.Fatal("reversed interval was accepted")
	}
}

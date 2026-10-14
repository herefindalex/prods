package specnumeric

import (
	"strings"
	"testing"
)

func normalizationProfile(role string, shapes ...Shape) Profile {
	p := electricalProfile("voltage", "V")
	p.SemanticRole = role
	p.AllowedShapes = shapes
	return p
}

func normalizedCandidate(t *testing.T, raw string, p Profile, shape Shape) *Value {
	t.Helper()
	original := raw
	result := Normalize(raw, p)
	if result.Reason != "" || result.Candidate == nil {
		t.Fatalf("Normalize(%q): %+v, want usable candidate", raw, result)
	}
	if raw != original {
		t.Fatal("raw changed")
	}
	if result.Candidate.Shape != shape || result.Candidate.Context.Kind != ContextUnconditional {
		t.Fatalf("candidate = %+v, want unconditional %s", result.Candidate, shape)
	}
	if err := ValidateValue(*result.Candidate, p); err != nil {
		t.Fatalf("invalid candidate: %v", err)
	}
	return result.Candidate
}

func TestNormalizePreservesDisjointVoltageIntervals(t *testing.T) {
	v := normalizedCandidate(t, "3.0–3.6 V, 4.5–5.5 V", normalizationProfile(RoleOperatingSupport, ShapeIntervalSet), ShapeIntervalSet)
	if len(v.Intervals) != 2 || v.Intervals[0].Lower.String() != "3" || v.Intervals[0].Upper.String() != "3.6" || v.Intervals[1].Lower.String() != "4.5" || v.Intervals[1].Upper.String() != "5.5" {
		t.Fatalf("lost interval gap: %+v", v.Intervals)
	}
	for _, interval := range v.Intervals {
		if !interval.LowerInclusive || !interval.UpperInclusive {
			t.Fatal("unmarked range must include both endpoints")
		}
	}
}

func TestNormalizeSingleVoltageNeedsDeclaredMeaning(t *testing.T) {
	result := Normalize("5V", normalizationProfile(RoleOperatingSupport, ShapeIntervalSet))
	if result.Reason != ReasonAmbiguous || result.Candidate != nil {
		t.Fatalf("single voltage without permitted single-value meaning = %+v", result)
	}
}

func TestNormalizeRejectsDecimalCommaGuess(t *testing.T) {
	result := Normalize("3,3V", normalizationProfile(RoleOperatingSupport, ShapeDiscreteSet))
	if result.Reason != ReasonAmbiguous || result.Candidate != nil {
		t.Fatalf("decimal comma guessed: %+v", result)
	}
}

func TestNormalizeRetainsConditionalInput(t *testing.T) {
	raw := "3.0–3.6 V at 25°C"
	p := normalizationProfile(RoleOperatingSupport, ShapeIntervalSet)
	result := Normalize(raw, p)
	if result.Reason != ReasonContextRequired || result.Candidate == nil || result.Candidate.Context.Kind != ContextConditional || result.Candidate.Context.Raw != raw {
		t.Fatalf("condition was discarded: %+v", result)
	}
	if err := ValidateValue(*result.Candidate, p); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeNegativeNominalTolerance(t *testing.T) {
	v := normalizedCandidate(t, "-5 V ±10%", normalizationProfile(RoleNominal, ShapeTolerance), ShapeTolerance)
	if v.Tolerance.Nominal.String() != "-5" || v.Tolerance.Type != ToleranceRelative || v.Tolerance.Amount.String() != "10" || v.Tolerance.Interval.Lower.String() != "-5.5" || v.Tolerance.Interval.Upper.String() != "-4.5" {
		t.Fatalf("negative relative tolerance = %+v", v.Tolerance)
	}
}

func TestNormalizeBoundClaimPreservesUnknownExtent(t *testing.T) {
	v := normalizedCandidate(t, "≥3.3 V", normalizationProfile(RoleMinimumLimit, ShapeBoundClaim), ShapeBoundClaim)
	if v.Bound.Direction != BoundLower || v.Bound.Value.String() != "3.3" || !v.Bound.Inclusive || len(v.Intervals) != 0 {
		t.Fatalf("bound became a support interval: %+v", v)
	}
}

func TestNormalizeDeclaredScalarConvertsExactly(t *testing.T) {
	v := normalizedCandidate(t, "3300 mV", normalizationProfile(RoleNominal, ShapeScalar), ShapeScalar)
	if v.Scalar.String() != "3.3" {
		t.Fatalf("scalar = %s, want 3.3", v.Scalar.String())
	}
}

func TestNormalizeDiscreteSetAllowsSharedTrailingUnit(t *testing.T) {
	v := normalizedCandidate(t, "3.3 / 5 V", normalizationProfile(RoleOperatingSupport, ShapeDiscreteSet), ShapeDiscreteSet)
	if len(v.Values) != 2 || v.Values[0].String() != "3.3" || v.Values[1].String() != "5" {
		t.Fatalf("discrete values = %+v", v.Values)
	}
}

func TestNormalizeMissingUnitRequiresDeclaredInputUnit(t *testing.T) {
	p := normalizationProfile(RoleNominal, ShapeScalar)
	result := Normalize("3300", p)
	if result.Reason != Reason("missing_unit") || result.Candidate != nil {
		t.Fatalf("canonical unit used as implicit input unit: %+v", result)
	}
	p.DeclaredInputUnit = "mV"
	v := normalizedCandidate(t, "3300", p, ShapeScalar)
	if v.Scalar.String() != "3.3" {
		t.Fatalf("declared source unit ignored: %s", v.Scalar.String())
	}
}

func TestNormalizePreservesExplicitEndpointOpenness(t *testing.T) {
	v := normalizedCandidate(t, "(3.0, 3.6] V", normalizationProfile(RoleOperatingSupport, ShapeIntervalSet), ShapeIntervalSet)
	if v.Intervals[0].LowerInclusive || !v.Intervals[0].UpperInclusive {
		t.Fatalf("explicit endpoint flags lost: %+v", v.Intervals[0])
	}
}

func TestNormalizeRejectsReversedInterval(t *testing.T) {
	result := Normalize("5–3 V", normalizationProfile(RoleOperatingSupport, ShapeIntervalSet))
	if result.Reason != Reason("invalid_interval") || result.Candidate != nil {
		t.Fatalf("reversed endpoints swapped: %+v", result)
	}
}

func TestNormalizeRejectsConflictingRoleMarker(t *testing.T) {
	result := Normalize("5 V typ.", normalizationProfile(RoleOperatingSupport, ShapeScalar))
	if result.Reason != ReasonAmbiguous || result.Candidate != nil {
		t.Fatalf("typical value promoted to operating support: %+v", result)
	}
}

func TestNormalizeAbsoluteToleranceUsesCanonicalUnits(t *testing.T) {
	v := normalizedCandidate(t, "5 V ±100 mV", normalizationProfile(RoleNominal, ShapeTolerance), ShapeTolerance)
	if v.Tolerance.Type != ToleranceAbsolute || v.Tolerance.Amount.String() != "0.1" || v.Tolerance.Interval.Lower.String() != "4.9" || v.Tolerance.Interval.Upper.String() != "5.1" {
		t.Fatalf("absolute tolerance = %+v", v.Tolerance)
	}
}

func TestNormalizeRejectsPartialUnsupportedExpression(t *testing.T) {
	result := Normalize("3–5 V / unknown", normalizationProfile(RoleOperatingSupport, ShapeIntervalSet))
	if result.Reason == "" || result.Candidate != nil {
		t.Fatalf("partial expression became usable fact: %+v", result)
	}
}

func TestNormalizeBoundsInputWork(t *testing.T) {
	result := Normalize(strings.Repeat(" ", 64*1024+1), normalizationProfile(RoleNominal, ShapeScalar))
	if result.Reason != Reason("unsupported_input_size") || result.Candidate != nil {
		t.Fatalf("oversized input = %+v", result)
	}
}

func TestNormalizeRejectsUnsupportedPrecision(t *testing.T) {
	result := Normalize("1e25 V", normalizationProfile(RoleNominal, ShapeScalar))
	if result.Reason != ReasonUnsupportedPrecision || result.Candidate != nil {
		t.Fatalf("out-of-profile decimal = %+v", result)
	}
}

package specnumeric

import "testing"

func predicateProfile(role string, shapes []Shape, operators ...Operator) Profile {
	p := electricalProfile("voltage", "V")
	p.SemanticRole = role
	p.AllowedShapes = shapes
	p.AllowedOperators = operators
	return p
}

func predicateValue(profile Profile, shape Shape) Value {
	return Value{SchemaVersion: profile.SchemaVersion, Quantity: profile.Quantity, SemanticRole: profile.SemanticRole, Shape: shape, Unit: profile.CanonicalUnit, UnitRegistryVersion: profile.UnitRegistryVersion, Context: Context{Kind: ContextUnconditional}}
}

func disjointVoltageValue(t *testing.T, profile Profile) Value {
	t.Helper()
	v := predicateValue(profile, ShapeIntervalSet)
	v.Intervals = []Interval{
		{Lower: decimalForTest(t, "3"), Upper: decimalForTest(t, "3.6"), LowerInclusive: true, UpperInclusive: true},
		{Lower: decimalForTest(t, "4.5"), Upper: decimalForTest(t, "5.5"), LowerInclusive: true, UpperInclusive: true},
	}
	return v
}

func TestIntervalSetCannotBridgeGap(t *testing.T) {
	p := predicateProfile(RoleOperatingSupport, []Shape{ShapeIntervalSet}, OperatorContainsRange)
	v := disjointVoltageValue(t, p)
	lower, upper := decimalForTest(t, "3.3"), decimalForTest(t, "5")
	query := Predicate{SpecID: "operating-voltage", Operator: OperatorContainsRange, Lower: &lower, Upper: &upper, Unit: "V"}
	if got := Evaluate(&v, p, query); got != NoMatch {
		t.Fatalf("range across gap = %s, want NO_MATCH", got)
	}
}

func TestIntervalSetContainsSupportedPoint(t *testing.T) {
	p := predicateProfile(RoleOperatingSupport, []Shape{ShapeIntervalSet}, OperatorContainsPoint)
	v := disjointVoltageValue(t, p)
	point := decimalForTest(t, "3.3")
	query := Predicate{SpecID: "operating-voltage", Operator: OperatorContainsPoint, Value: &point, Unit: "V"}
	if got := Evaluate(&v, p, query); got != Match {
		t.Fatalf("supported point = %s, want MATCH", got)
	}
}

func TestMissingNumericValueIsUnknown(t *testing.T) {
	p := predicateProfile(RoleNominal, []Shape{ShapeScalar}, OperatorEqual)
	point := decimalForTest(t, "0")
	query := Predicate{SpecID: "nominal-voltage", Operator: OperatorEqual, Value: &point, Unit: "V"}
	if got := Evaluate(nil, p, query); got != Unknown {
		t.Fatalf("missing numeric value = %s, want UNKNOWN", got)
	}
}

func TestZeroScalarRemainsAValue(t *testing.T) {
	p := predicateProfile(RoleNominal, []Shape{ShapeScalar}, OperatorEqual)
	zero := decimalForTest(t, "0")
	v := predicateValue(p, ShapeScalar)
	v.Scalar = &zero
	query := Predicate{SpecID: "nominal-voltage", Operator: OperatorEqual, Value: &zero, Unit: "V"}
	if got := Evaluate(&v, p, query); got != Match {
		t.Fatalf("zero scalar equality = %s, want MATCH", got)
	}
}

func TestUpperBoundDoesNotEstablishExactLimit(t *testing.T) {
	p := predicateProfile(RoleMaximumLimit, []Shape{ShapeBoundClaim}, OperatorEqual)
	five := decimalForTest(t, "5")
	v := predicateValue(p, ShapeBoundClaim)
	v.Bound = &BoundClaim{Direction: BoundUpper, Value: five, Inclusive: true}
	query := Predicate{SpecID: "maximum-voltage", Operator: OperatorEqual, Value: &five, Unit: "V"}
	if got := Evaluate(&v, p, query); got != Unknown {
		t.Fatalf("upper-bound equality = %s, want UNKNOWN", got)
	}
}

func TestUpperBoundEstablishesGuaranteedLimit(t *testing.T) {
	p := predicateProfile(RoleMaximumLimit, []Shape{ShapeBoundClaim}, OperatorLessEqual)
	five := decimalForTest(t, "5")
	v := predicateValue(p, ShapeBoundClaim)
	v.Bound = &BoundClaim{Direction: BoundUpper, Value: five, Inclusive: true}
	query := Predicate{SpecID: "maximum-voltage", Operator: OperatorLessEqual, Value: &five, Unit: "V"}
	if got := Evaluate(&v, p, query); got != Match {
		t.Fatalf("guaranteed limit = %s, want MATCH", got)
	}
}

func TestNominalToleranceComparesRetainedNominal(t *testing.T) {
	p := predicateProfile(RoleNominal, []Shape{ShapeTolerance}, OperatorEqual)
	five := decimalForTest(t, "5")
	v := predicateValue(p, ShapeTolerance)
	v.Tolerance = &Tolerance{Nominal: five, Type: ToleranceRelative, Amount: decimalForTest(t, "10"), Interval: Interval{Lower: decimalForTest(t, "4.5"), Upper: decimalForTest(t, "5.5"), LowerInclusive: true, UpperInclusive: true}}
	query := Predicate{SpecID: "nominal-voltage", Operator: OperatorEqual, Value: &five, Unit: "V"}
	if got := Evaluate(&v, p, query); got != Match {
		t.Fatalf("nominal tolerance equality = %s, want MATCH", got)
	}
}

func TestConditionalNumericValueIsUnknown(t *testing.T) {
	p := predicateProfile(RoleOperatingSupport, []Shape{ShapeIntervalSet}, OperatorContainsPoint)
	v := disjointVoltageValue(t, p)
	v.Context = Context{Kind: ContextConditional, Raw: "mode A"}
	point := decimalForTest(t, "3.3")
	query := Predicate{SpecID: "operating-voltage", Operator: OperatorContainsPoint, Value: &point, Unit: "V"}
	if got := Evaluate(&v, p, query); got != Unknown {
		t.Fatalf("conditional fact = %s, want UNKNOWN", got)
	}
}

func TestScalarComparisonNormalizesQueryUnitExactly(t *testing.T) {
	p := predicateProfile(RoleNominal, []Shape{ShapeScalar}, OperatorLess)
	v := predicateValue(p, ShapeScalar)
	nominal, queryValue := decimalForTest(t, "3.3"), decimalForTest(t, "3300.0000000000000000000000000001")
	v.Scalar = &nominal
	query, err := NormalizePredicate(Predicate{SpecID: "nominal-voltage", Operator: OperatorLess, Value: &queryValue, Unit: "mV"}, p)
	if err != nil {
		t.Fatal(err)
	}
	if got := Evaluate(&v, p, query); got != Match {
		t.Fatalf("exact comparison after unit conversion = %s, want MATCH", got)
	}
}

func TestNormalizePredicateRejectsSupportQueryForNominalTolerance(t *testing.T) {
	p := predicateProfile(RoleNominal, []Shape{ShapeTolerance}, OperatorContainsPoint)
	point := decimalForTest(t, "5")
	_, err := NormalizePredicate(Predicate{SpecID: "nominal-voltage", Operator: OperatorContainsPoint, Value: &point, Unit: "V"}, p)
	if err == nil {
		t.Fatal("nominal tolerance accepted as operating support")
	}
}

func TestIntervalSetOverlapRespectsOpenEndpoint(t *testing.T) {
	p := predicateProfile(RoleOperatingSupport, []Shape{ShapeIntervalSet}, OperatorOverlapsRange)
	v := predicateValue(p, ShapeIntervalSet)
	v.Intervals = []Interval{{Lower: decimalForTest(t, "3"), Upper: decimalForTest(t, "3.6"), LowerInclusive: true, UpperInclusive: false}}
	lower, upper := decimalForTest(t, "3.6"), decimalForTest(t, "4")
	query := Predicate{SpecID: "operating-voltage", Operator: OperatorOverlapsRange, Lower: &lower, Upper: &upper, Unit: "V"}
	if got := Evaluate(&v, p, query); got != NoMatch {
		t.Fatalf("overlap at excluded endpoint = %s, want NO_MATCH", got)
	}
}

func TestIntervalSetCombinesConnectedCoverage(t *testing.T) {
	p := predicateProfile(RoleOperatingSupport, []Shape{ShapeIntervalSet}, OperatorContainsRange)
	v := predicateValue(p, ShapeIntervalSet)
	v.Intervals = []Interval{
		{Lower: decimalForTest(t, "3"), Upper: decimalForTest(t, "5"), LowerInclusive: false, UpperInclusive: true},
		{Lower: decimalForTest(t, "3"), Upper: decimalForTest(t, "4"), LowerInclusive: true, UpperInclusive: true},
	}
	lower, upper := decimalForTest(t, "3"), decimalForTest(t, "5")
	query := Predicate{SpecID: "operating-voltage", Operator: OperatorContainsRange, Lower: &lower, Upper: &upper, Unit: "V"}
	if got := Evaluate(&v, p, query); got != Match {
		t.Fatalf("connected complete coverage = %s, want MATCH", got)
	}
}

func TestAllPreservesThreeValuedConjunction(t *testing.T) {
	for _, test := range []struct {
		name   string
		values []Truth
		want   Truth
	}{
		{"no_match_dominates_unknown", []Truth{Unknown, NoMatch}, NoMatch},
		{"unknown_prevents_match", []Truth{Match, Unknown}, Unknown},
		{"all_match", []Truth{Match, Match}, Match},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := All(test.values); got != test.want {
				t.Fatalf("conjunction = %s, want %s", got, test.want)
			}
		})
	}
}

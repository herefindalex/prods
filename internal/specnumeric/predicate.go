package specnumeric

import (
	"fmt"
	"sort"
	"strings"
)

// NormalizePredicate validates the entire condition before converting its
// operands to the profile's canonical comparison unit. It never drops a query.
func NormalizePredicate(input Predicate, profile Profile) (Predicate, error) {
	if err := ValidateProfile(profile); err != nil {
		return Predicate{}, err
	}
	if strings.TrimSpace(input.SpecID) == "" || input.Unit == "" {
		return Predicate{}, fmt.Errorf("predicate requires spec ID and explicit unit")
	}
	compatible := false
	for _, shape := range profile.AllowedShapes {
		compatible = compatible || predicateCompatible(profile.SemanticRole, shape, input.Operator)
	}
	if !profile.AllowsOperator(input.Operator) || !compatible {
		return Predicate{}, fmt.Errorf("unsupported predicate operator %q for profile", input.Operator)
	}
	rangeQuery := input.Operator == OperatorBetween || input.Operator == OperatorContainsRange || input.Operator == OperatorOverlapsRange
	if rangeQuery {
		if input.Value != nil || input.Lower == nil || input.Upper == nil {
			return Predicate{}, fmt.Errorf("range predicate requires only lower and upper")
		}
	} else if input.Value == nil || input.Lower != nil || input.Upper != nil {
		return Predicate{}, fmt.Errorf("point predicate requires only value")
	}
	output := input
	for _, operand := range []struct {
		input  *Decimal
		output **Decimal
	}{{input.Value, &output.Value}, {input.Lower, &output.Lower}, {input.Upper, &output.Upper}} {
		if operand.input == nil {
			continue
		}
		converted, err := Convert(*operand.input, input.Unit, profile)
		if err != nil {
			return Predicate{}, err
		}
		*operand.output = &converted
	}
	if rangeQuery && output.Lower.Cmp(*output.Upper) > 0 {
		return Predicate{}, fmt.Errorf("reversed predicate range")
	}
	output.Unit = profile.CanonicalUnit
	return output, nil
}

func predicateCompatible(role string, shape Shape, operator Operator) bool {
	switch operator {
	case OperatorContainsPoint, OperatorContainsRange, OperatorOverlapsRange:
		return role == RoleOperatingSupport && (shape == ShapeScalar || shape == ShapeDiscreteSet || shape == ShapeIntervalSet)
	case OperatorEqual, OperatorLess, OperatorLessEqual, OperatorGreater, OperatorGreaterEqual, OperatorBetween:
		return shape == ShapeScalar || (shape == ShapeTolerance && role == RoleNominal) || (shape == ShapeBoundClaim && (role == RoleMinimumLimit || role == RoleMaximumLimit || role == RoleAbsoluteMaximum))
	}
	return false
}

// Evaluate is the exact oracle for a fact already associated with predicate's
// SpecID by its caller. Missing, invalid, conditional or inapplicable facts
// cannot establish either a match or a non-match.
func Evaluate(value *Value, profile Profile, predicate Predicate) Truth {
	if value == nil || ValidateValue(*value, profile) != nil || value.Context.Kind != ContextUnconditional || !predicateCompatible(profile.SemanticRole, value.Shape, predicate.Operator) {
		return Unknown
	}
	query, err := NormalizePredicate(predicate, profile)
	if err != nil {
		return Unknown
	}
	switch query.Operator {
	case OperatorContainsPoint, OperatorContainsRange, OperatorOverlapsRange:
		return evaluateSupport(*value, query)
	}
	switch value.Shape {
	case ShapeScalar:
		return evaluateScalar(*value.Scalar, query)
	case ShapeTolerance:
		return evaluateScalar(value.Tolerance.Nominal, query)
	case ShapeBoundClaim:
		return evaluateBound(*value.Bound, profile.SemanticRole, query)
	}
	return Unknown
}

func evaluateScalar(value Decimal, query Predicate) Truth {
	if query.Operator == OperatorBetween {
		return truthOf(value.Cmp(*query.Lower) >= 0 && value.Cmp(*query.Upper) <= 0)
	}
	comparison := value.Cmp(*query.Value)
	switch query.Operator {
	case OperatorEqual:
		return truthOf(comparison == 0)
	case OperatorLess:
		return truthOf(comparison < 0)
	case OperatorLessEqual:
		return truthOf(comparison <= 0)
	case OperatorGreater:
		return truthOf(comparison > 0)
	case OperatorGreaterEqual:
		return truthOf(comparison >= 0)
	}
	return Unknown
}

func evaluateSupport(value Value, query Predicate) Truth {
	intervals := append([]Interval(nil), value.Intervals...)
	points := value.Values
	if value.Shape == ShapeScalar {
		points = []Decimal{*value.Scalar}
	}
	for _, point := range points {
		intervals = append(intervals, Interval{Lower: point, Upper: point, LowerInclusive: true, UpperInclusive: true})
	}
	if query.Operator == OperatorContainsPoint {
		for _, interval := range intervals {
			if intervalContains(interval, *query.Value) {
				return Match
			}
		}
		return NoMatch
	}
	if query.Operator == OperatorOverlapsRange {
		for _, interval := range intervals {
			lower := interval.Upper.Cmp(*query.Lower)
			upper := interval.Lower.Cmp(*query.Upper)
			if (lower > 0 || lower == 0 && interval.UpperInclusive) && (upper < 0 || upper == 0 && interval.LowerInclusive) {
				return Match
			}
		}
		return NoMatch
	}
	// Union connected intervals without filling any excluded point or gap.
	sort.Slice(intervals, func(i, j int) bool {
		comparison := intervals[i].Lower.Cmp(intervals[j].Lower)
		if comparison == 0 {
			return intervals[i].LowerInclusive && !intervals[j].LowerInclusive
		}
		return comparison < 0
	})
	var covered *Interval
	for _, interval := range intervals {
		if covered == nil {
			if intervalContains(interval, *query.Lower) {
				current := interval
				covered = &current
			}
		} else {
			connection := interval.Lower.Cmp(covered.Upper)
			if connection > 0 || connection == 0 && !interval.LowerInclusive && !covered.UpperInclusive {
				return NoMatch
			}
			end := interval.Upper.Cmp(covered.Upper)
			if end > 0 {
				covered.Upper, covered.UpperInclusive = interval.Upper, interval.UpperInclusive
			} else if end == 0 {
				covered.UpperInclusive = covered.UpperInclusive || interval.UpperInclusive
			}
		}
		if covered != nil && intervalContains(*covered, *query.Upper) {
			return Match
		}
	}
	return NoMatch
}

func intervalContains(interval Interval, point Decimal) bool {
	lower, upper := point.Cmp(interval.Lower), point.Cmp(interval.Upper)
	return (lower > 0 || lower == 0 && interval.LowerInclusive) && (upper < 0 || upper == 0 && interval.UpperInclusive)
}

func evaluateBound(bound BoundClaim, role string, query Predicate) Truth {
	if bound.Direction == BoundLower && role != RoleMinimumLimit || bound.Direction == BoundUpper && role != RoleMaximumLimit && role != RoleAbsoluteMaximum {
		return Unknown
	}
	// A one-sided claim constrains possible limit values; it is never promoted
	// into an infinite operating-support interval or an exact endpoint value.
	if query.Operator == OperatorBetween {
		comparison := bound.Value.Cmp(*query.Lower)
		if bound.Direction == BoundLower {
			comparison = query.Upper.Cmp(bound.Value)
		}
		if comparison < 0 || comparison == 0 && !bound.Inclusive {
			return NoMatch
		}
		return Unknown
	}
	comparison, operator := bound.Value.Cmp(*query.Value), query.Operator
	if bound.Direction == BoundLower {
		comparison = -comparison
		switch operator {
		case OperatorLess:
			operator = OperatorGreater
		case OperatorLessEqual:
			operator = OperatorGreaterEqual
		case OperatorGreater:
			operator = OperatorLess
		case OperatorGreaterEqual:
			operator = OperatorLessEqual
		}
	}
	below := comparison < 0 || comparison == 0 && !bound.Inclusive
	switch operator {
	case OperatorEqual:
		if below {
			return NoMatch
		}
	case OperatorLess:
		if below {
			return Match
		}
	case OperatorLessEqual:
		if comparison <= 0 {
			return Match
		}
	case OperatorGreater:
		if comparison <= 0 {
			return NoMatch
		}
	case OperatorGreaterEqual:
		if below {
			return NoMatch
		}
	}
	return Unknown
}

func truthOf(condition bool) Truth {
	if condition {
		return Match
	}
	return NoMatch
}

// All implements conjunction: NO_MATCH dominates UNKNOWN; only all MATCH
// values (including an empty conjunction) establish MATCH.
func All(results []Truth) Truth {
	result := Match
	for _, value := range results {
		if value == NoMatch {
			return NoMatch
		}
		if value != Match {
			result = Unknown
		}
	}
	return result
}

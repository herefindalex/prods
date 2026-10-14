package specnumeric

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"strings"
)

type Shape string

const (
	ShapeScalar      Shape = "scalar"
	ShapeDiscreteSet Shape = "discrete_set"
	ShapeIntervalSet Shape = "interval_set"
	ShapeTolerance   Shape = "tolerance"
	ShapeBoundClaim  Shape = "bound_claim"
)

type Operator string

const (
	OperatorContainsPoint Operator = "contains_point"
	OperatorContainsRange Operator = "contains_range"
	OperatorOverlapsRange Operator = "overlaps_range"
	OperatorEqual         Operator = "="
	OperatorLess          Operator = "<"
	OperatorLessEqual     Operator = "<="
	OperatorGreater       Operator = ">"
	OperatorGreaterEqual  Operator = ">="
	OperatorBetween       Operator = "between"
)

type Truth string

const (
	Match   Truth = "MATCH"
	NoMatch Truth = "NO_MATCH"
	Unknown Truth = "UNKNOWN"
)

type Reason string

const (
	ReasonUnsupported          Reason = "unsupported"
	ReasonUnsupportedShape     Reason = "unsupported_shape"
	ReasonUnsupportedUnit      Reason = "unsupported_unit"
	ReasonUnsupportedPrecision Reason = "unsupported_precision"
	ReasonAmbiguous            Reason = "ambiguous"
	ReasonContextRequired      Reason = "context_required"
	ReasonMissing              Reason = "missing"
	ReasonMissingUnit          Reason = "missing_unit"
	ReasonInvalidInterval      Reason = "invalid_interval"
	ReasonUnsupportedInputSize Reason = "unsupported_input_size"
)

type Result struct {
	Candidate *Value `json:"candidate,omitempty"`
	Reason    Reason `json:"reason,omitempty"`
}

type Predicate struct {
	SpecID   string   `json:"spec_id"`
	Operator Operator `json:"operator"`
	Value    *Decimal `json:"value,omitempty"`
	Lower    *Decimal `json:"lower,omitempty"`
	Upper    *Decimal `json:"upper,omitempty"`
	Unit     string   `json:"unit"`
}

type ContextKind string

const (
	ContextUnconditional ContextKind = "unconditional"
	ContextConditional   ContextKind = "conditional"
)

type Context struct {
	Kind ContextKind `json:"kind"`
	Raw  string      `json:"raw,omitempty"`
}

type Interval struct {
	Lower          Decimal `json:"lower"`
	Upper          Decimal `json:"upper"`
	LowerInclusive bool    `json:"lower_inclusive"`
	UpperInclusive bool    `json:"upper_inclusive"`
}

// Controlled JSON must state openness explicitly. The Raw parser owns any
// syntax defaults; omission or null here cannot silently become false.
func (interval *Interval) UnmarshalJSON(encoded []byte) error {
	var wire struct {
		Lower          Decimal `json:"lower"`
		Upper          Decimal `json:"upper"`
		LowerInclusive *bool   `json:"lower_inclusive"`
		UpperInclusive *bool   `json:"upper_inclusive"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return err
	}
	if wire.LowerInclusive == nil || wire.UpperInclusive == nil {
		return fmt.Errorf("interval requires non-null lower_inclusive and upper_inclusive")
	}
	*interval = Interval{Lower: wire.Lower, Upper: wire.Upper, LowerInclusive: *wire.LowerInclusive, UpperInclusive: *wire.UpperInclusive}
	return nil
}

type ToleranceType string

const (
	ToleranceAbsolute ToleranceType = "absolute"
	ToleranceRelative ToleranceType = "relative"
)

// Amount is in canonical units for absolute tolerance and percent for relative.
type Tolerance struct {
	Nominal  Decimal       `json:"nominal"`
	Type     ToleranceType `json:"type"`
	Amount   Decimal       `json:"amount"`
	Interval Interval      `json:"interval"`
}

type BoundDirection string

const (
	BoundLower BoundDirection = "lower"
	BoundUpper BoundDirection = "upper"
)

type BoundClaim struct {
	Direction BoundDirection `json:"direction"`
	Value     Decimal        `json:"value"`
	Inclusive bool           `json:"inclusive"`
}

func (bound *BoundClaim) UnmarshalJSON(encoded []byte) error {
	var wire struct {
		Direction BoundDirection `json:"direction"`
		Value     Decimal        `json:"value"`
		Inclusive *bool          `json:"inclusive"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return err
	}
	if wire.Inclusive == nil {
		return fmt.Errorf("bound requires non-null inclusive")
	}
	*bound = BoundClaim{Direction: wire.Direction, Value: wire.Value, Inclusive: *wire.Inclusive}
	return nil
}

// Value is a flattened tagged union: exactly one payload must match Shape.
type Value struct {
	SchemaVersion       int         `json:"schema_version"`
	Quantity            string      `json:"quantity"`
	SemanticRole        string      `json:"semantic_role"`
	Shape               Shape       `json:"shape"`
	Unit                string      `json:"unit"`
	UnitRegistryVersion string      `json:"unit_registry_version"`
	Scalar              *Decimal    `json:"scalar,omitempty"`
	Values              []Decimal   `json:"values,omitempty"`
	Intervals           []Interval  `json:"intervals,omitempty"`
	Tolerance           *Tolerance  `json:"tolerance,omitempty"`
	Bound               *BoundClaim `json:"bound,omitempty"`
	Context             Context     `json:"context"`
}

func knownShape(shape Shape) bool {
	switch shape {
	case ShapeScalar, ShapeDiscreteSet, ShapeIntervalSet, ShapeTolerance, ShapeBoundClaim:
		return true
	}
	return false
}
func knownOperator(operator Operator) bool {
	switch operator {
	case OperatorContainsPoint, OperatorContainsRange, OperatorOverlapsRange, OperatorEqual, OperatorLess, OperatorLessEqual, OperatorGreater, OperatorGreaterEqual, OperatorBetween:
		return true
	}
	return false
}

func ValidateValue(value Value, profile Profile) error {
	if err := ValidateProfile(profile); err != nil {
		return err
	}
	if value.SchemaVersion != profile.SchemaVersion || value.Quantity != profile.Quantity || value.SemanticRole != profile.SemanticRole || value.Unit != profile.CanonicalUnit || value.UnitRegistryVersion != profile.UnitRegistryVersion {
		return fmt.Errorf("value does not match profile")
	}
	if !profile.AllowsShape(value.Shape) {
		return fmt.Errorf("shape not allowed by profile")
	}
	switch value.Context.Kind {
	case ContextUnconditional:
		if value.Context.Raw != "" {
			return fmt.Errorf("unconditional value has conditional text")
		}
	case ContextConditional:
		if strings.TrimSpace(value.Context.Raw) == "" {
			return fmt.Errorf("conditional value requires original context")
		}
	default:
		return fmt.Errorf("invalid context kind")
	}
	payloads := 0
	for _, present := range []bool{value.Scalar != nil, value.Values != nil, value.Intervals != nil, value.Tolerance != nil, value.Bound != nil} {
		if present {
			payloads++
		}
	}
	if payloads != 1 {
		return fmt.Errorf("value must have exactly one shape payload")
	}
	switch value.Shape {
	case ShapeScalar:
		if value.Scalar == nil || !value.Scalar.valid() {
			return ErrInvalidDecimal
		}
	case ShapeDiscreteSet:
		if len(value.Values) == 0 {
			return fmt.Errorf("empty discrete set")
		}
		for _, number := range value.Values {
			if !number.valid() {
				return ErrInvalidDecimal
			}
		}
	case ShapeIntervalSet:
		if len(value.Intervals) == 0 {
			return fmt.Errorf("empty interval set")
		}
		for _, interval := range value.Intervals {
			if err := validateInterval(interval); err != nil {
				return err
			}
		}
	case ShapeTolerance:
		if value.Tolerance == nil {
			return fmt.Errorf("missing tolerance")
		}
		if err := validateTolerance(*value.Tolerance); err != nil {
			return err
		}
	case ShapeBoundClaim:
		if value.Bound == nil || !value.Bound.Value.valid() {
			return fmt.Errorf("missing bound")
		}
		if value.Bound.Direction != BoundLower && value.Bound.Direction != BoundUpper {
			return fmt.Errorf("invalid bound direction")
		}
		if profile.SemanticRole == RoleErrorMagnitude && (value.Bound.Direction != BoundUpper || value.Bound.Value.rat().Sign() < 0) {
			return fmt.Errorf("error magnitude requires a nonnegative upper bound")
		}
	default:
		return fmt.Errorf("unsupported shape")
	}
	if profile.Quantity == "count" {
		var numbers []Decimal
		switch value.Shape {
		case ShapeScalar:
			numbers = append(numbers, *value.Scalar)
		case ShapeDiscreteSet:
			numbers = value.Values
		case ShapeIntervalSet:
			for _, interval := range value.Intervals {
				numbers = append(numbers, interval.Lower, interval.Upper)
			}
		case ShapeBoundClaim:
			numbers = append(numbers, value.Bound.Value)
		}
		for _, number := range numbers {
			if number.rat().Sign() < 0 || !number.rat().IsInt() {
				return fmt.Errorf("count requires nonnegative integers")
			}
		}
	}
	return nil
}

func validateInterval(interval Interval) error {
	if !interval.Lower.valid() || !interval.Upper.valid() {
		return ErrInvalidDecimal
	}
	comparison := interval.Lower.Cmp(interval.Upper)
	if comparison > 0 || (comparison == 0 && (!interval.LowerInclusive || !interval.UpperInclusive)) {
		return fmt.Errorf("empty or reversed interval")
	}
	return nil
}

func validateTolerance(tolerance Tolerance) error {
	if !tolerance.Nominal.valid() || !tolerance.Amount.valid() {
		return ErrInvalidDecimal
	}
	amount := tolerance.Amount.rat()
	if amount.Sign() < 0 {
		return fmt.Errorf("negative tolerance")
	}
	switch tolerance.Type {
	case ToleranceAbsolute:
	case ToleranceRelative:
		amount.Mul(amount, new(big.Rat).Abs(tolerance.Nominal.rat()))
		amount.Quo(amount, big.NewRat(100, 1))
	default:
		return fmt.Errorf("unsupported tolerance type")
	}
	lower, err := decimalFromRat(new(big.Rat).Sub(tolerance.Nominal.rat(), amount))
	if err != nil {
		return err
	}
	upper, err := decimalFromRat(new(big.Rat).Add(tolerance.Nominal.rat(), amount))
	if err != nil {
		return err
	}
	if err := validateInterval(tolerance.Interval); err != nil {
		return err
	}
	if !tolerance.Interval.LowerInclusive || !tolerance.Interval.UpperInclusive || lower.Cmp(tolerance.Interval.Lower) != 0 || upper.Cmp(tolerance.Interval.Upper) != 0 {
		return fmt.Errorf("tolerance interval does not match nominal and amount")
	}
	return nil
}

func MarshalValue(value Value) (string, error) {
	profile := Profile{Quantity: value.Quantity, SemanticRole: value.SemanticRole, AllowedShapes: []Shape{value.Shape}, AllowedOperators: []Operator{OperatorEqual}, CanonicalUnit: value.Unit, NumericFormat: NumericFormatDecimalDot, SchemaVersion: value.SchemaVersion, ParserVersion: ParserVersion, UnitRegistryVersion: value.UnitRegistryVersion}
	if err := ValidateValue(value, profile); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(value)
	return string(encoded), err
}

func UnmarshalValue(encoded string, profile Profile) (Value, error) {
	decoder := json.NewDecoder(strings.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var value Value
	if err := decoder.Decode(&value); err != nil {
		return Value{}, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Value{}, fmt.Errorf("value must contain one JSON object")
	}
	// Presence matters at the wire boundary: an unrelated null payload cannot
	// disappear into the Go pointer's zero value and bypass the tagged union.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(encoded), &fields); err != nil {
		return Value{}, err
	}
	payload := map[Shape]string{ShapeScalar: "scalar", ShapeDiscreteSet: "values", ShapeIntervalSet: "intervals", ShapeTolerance: "tolerance", ShapeBoundClaim: "bound"}[value.Shape]
	for name := range fields {
		// The controlled union uses canonical wire keys, rather than allowing
		// decoder case folding to disagree with payload presence validation.
		switch name {
		case "schema_version", "quantity", "semantic_role", "shape", "unit", "unit_registry_version", "context":
		case "scalar", "values", "intervals", "tolerance", "bound":
			if name != payload {
				return Value{}, fmt.Errorf("payload %q incompatible with shape", name)
			}
		default:
			return Value{}, fmt.Errorf("noncanonical value field %q", name)
		}
	}
	if err := ValidateValue(value, profile); err != nil {
		return Value{}, err
	}
	return value, nil
}

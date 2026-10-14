package specnumeric

import (
	"errors"
	"math/big"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxNormalizationInput = 64 * 1024

var rawNumberPattern = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?`)

type rawAtom struct {
	number Decimal
	unit   string
}

type rawSegment struct {
	lower, upper                   rawAtom
	interval                       bool
	lowerInclusive, upperInclusive bool
}

type rawParser struct {
	rest  string
	units map[string]unitDefinition
}

// Normalize derives a candidate under an explicit profile. A nonempty Reason
// always prevents treating that candidate as an unconditional numeric fact.
// Raw remains the caller's authoritative text; parsing only consumes a copy.
func Normalize(raw string, profile Profile) Result {
	if len(raw) > maxNormalizationInput {
		return Result{Reason: ReasonUnsupportedInputSize}
	}
	input := strings.TrimSpace(raw)
	if input == "" {
		return Result{Reason: ReasonMissing}
	}
	if profile.SemanticRole == "" || len(profile.AllowedShapes) == 0 {
		return Result{Reason: ReasonAmbiguous}
	}
	if err := ValidateProfile(profile); err != nil {
		return Result{Reason: ReasonUnsupported}
	}
	// Markers must agree with the configured role; they never redefine it.
	if rest, role := consumeRoleMarker(input); role != "" {
		if role != profile.SemanticRole {
			return Result{Reason: ReasonAmbiguous}
		}
		input = rest
	}
	p := rawParser{rest: input, units: registryForVersion(profile.UnitRegistryVersion)}
	value := Value{SchemaVersion: profile.SchemaVersion, Quantity: profile.Quantity,
		SemanticRole: profile.SemanticRole, Unit: profile.CanonicalUnit,
		UnitRegistryVersion: profile.UnitRegistryVersion, Context: Context{Kind: ContextUnconditional}}
	var reason Reason
	if profile.SemanticRole == RoleErrorMagnitude {
		marker := p.takeOne("±", "<=", "≤", "<")
		atom, failure := p.atom()
		if failure != "" {
			return Result{Reason: failure}
		}
		number, failure := canonicalAtom(atom, "", profile)
		if failure != "" {
			return Result{Reason: failure}
		}
		value.Shape = ShapeBoundClaim
		value.Bound = &BoundClaim{Direction: BoundUpper, Value: number, Inclusive: marker != "<"}
	} else if operator := p.takeOne(">=", "<=", "≥", "≤", ">", "<"); operator != "" {
		value.Shape = ShapeBoundClaim
		atom, failure := p.atom()
		if failure != "" {
			return Result{Reason: failure}
		}
		number, failure := canonicalAtom(atom, "", profile)
		if failure != "" {
			return Result{Reason: failure}
		}
		direction := BoundUpper
		if strings.ContainsAny(operator, ">≥") {
			direction = BoundLower
		}
		value.Bound = &BoundClaim{Direction: direction, Value: number, Inclusive: operator != ">" && operator != "<"}
	} else {
		segment, failure := p.segment()
		if failure != "" {
			return Result{Reason: failure}
		}
		if !segment.interval && p.takeOne("±") != "" {
			value.Shape = ShapeTolerance
			value.Tolerance, reason = p.tolerance(segment.lower, profile)
		} else {
			segments := []rawSegment{segment}
			for {
				before := p.rest
				separator := p.takeOne(",", "/")
				if separator == "" {
					break
				}
				// Adjacent bare numbers with comma are ambiguous decimal/grouping
				// notation, even if a source unit is declared. A bracket's comma
				// is consumed inside segment and is not a list separator.
				if separator == "," && !segments[len(segments)-1].interval && segments[len(segments)-1].lower.unit == "" && !strings.HasPrefix(before, " ") && !strings.HasPrefix(p.rest, " ") {
					return Result{Reason: ReasonAmbiguous}
				}
				next, failure := p.segment()
				if failure != "" {
					return Result{Reason: failure}
				}
				if next.interval != segment.interval {
					return Result{Reason: ReasonUnsupportedShape}
				}
				segments = append(segments, next)
			}
			reason = value.setSegments(segments, profile)
		}
	}
	if reason != "" {
		return Result{Reason: reason}
	}
	if !profile.AllowsShape(value.Shape) {
		if value.Shape == ShapeScalar {
			return Result{Reason: ReasonAmbiguous}
		}
		return Result{Reason: ReasonUnsupportedShape}
	}
	if rest, role := consumeRoleMarker(strings.TrimSpace(p.rest)); role != "" {
		if role != profile.SemanticRole {
			return Result{Reason: ReasonAmbiguous}
		}
		p.rest = rest
	}
	if remainder := strings.TrimSpace(p.rest); remainder != "" {
		if !conditionalSuffix(remainder) {
			return Result{Reason: ReasonUnsupportedShape}
		}
		value.Context = Context{Kind: ContextConditional, Raw: raw}
		reason = ReasonContextRequired
	}
	if err := ValidateValue(value, profile); err != nil {
		return Result{Reason: ReasonUnsupportedShape}
	}
	return Result{Candidate: &value, Reason: reason}
}

func (p *rawParser) takeOne(tokens ...string) string {
	rest := strings.TrimLeftFunc(p.rest, unicode.IsSpace)
	for _, token := range tokens {
		if strings.HasPrefix(rest, token) {
			p.rest = rest[len(token):]
			return token
		}
	}
	return ""
}

func (p *rawParser) atom() (rawAtom, Reason) {
	rest := strings.TrimLeftFunc(p.rest, unicode.IsSpace)
	text := rawNumberPattern.FindString(rest)
	if text == "" {
		return rawAtom{}, ReasonUnsupportedShape
	}
	number, err := ParseDecimal(text)
	if err != nil {
		return rawAtom{}, decimalReason(err)
	}
	rest = rest[len(text):]
	unitRest := strings.TrimLeftFunc(rest, unicode.IsSpace)
	end := p.unitLength(unitRest)
	unit := unitRest[:end]
	// Only registered electrical units are consumed. Words marking roles or
	// conditions remain suffixes; other adjacent unit text fails closed.
	if unit != "" {
		if _, known := p.units[unit]; known {
			rest = unitRest[end:]
		} else if !conditionalSuffix(unitRest) && !startsRoleMarker(unitRest) {
			return rawAtom{}, ReasonUnsupportedUnit
		} else {
			unit = ""
		}
	}
	p.rest = rest
	return rawAtom{number: number, unit: unit}, ""
}

func (p *rawParser) unitLength(rest string) int {
	if _, ok := p.units["1"]; ok && strings.HasPrefix(rest, "1") {
		return 1
	}
	_, percent := p.units["%"]
	end := 0
	for end < len(rest) {
		r, size := utf8.DecodeRuneInString(rest[end:])
		if !unicode.IsLetter(r) && !(percent && r == '%') {
			break
		}
		end += size
	}
	return end
}

func (p *rawParser) segment() (rawSegment, Reason) {
	opening := p.takeOne("[", "(")
	lower, reason := p.atom()
	if reason != "" {
		return rawSegment{}, reason
	}
	segment := rawSegment{lower: lower, lowerInclusive: true, upperInclusive: true}
	delimiters := []string{"–", "-", "—"}
	if opening != "" {
		delimiters = append(delimiters, ",")
	}
	if p.takeOne(delimiters...) == "" {
		if opening != "" {
			return rawSegment{}, ReasonUnsupportedShape
		}
		return segment, ""
	}
	segment.interval = true
	segment.upper, reason = p.atom()
	if reason != "" {
		return rawSegment{}, reason
	}
	if opening != "" {
		closing := p.takeOne("]", ")")
		if closing == "" {
			return rawSegment{}, ReasonUnsupportedShape
		}
		segment.lowerInclusive = opening == "["
		segment.upperInclusive = closing == "]"
		// A unit following the closing bracket applies to both endpoints.
		rest := strings.TrimLeftFunc(p.rest, unicode.IsSpace)
		end := p.unitLength(rest)
		if end > 0 {
			unit := rest[:end]
			if _, known := p.units[unit]; known {
				if segment.lower.unit != "" || segment.upper.unit != "" {
					return rawSegment{}, ReasonAmbiguous
				}
				segment.upper.unit = unit
				p.rest = rest[end:]
			}
		}
	}
	return segment, ""
}

func (v *Value) setSegments(segments []rawSegment, profile Profile) Reason {
	sharedUnit := ""
	last := segments[len(segments)-1]
	if last.interval {
		sharedUnit = last.upper.unit
	} else if len(segments) > 1 {
		sharedUnit = last.lower.unit
	}
	if last.interval {
		v.Shape = ShapeIntervalSet
	} else if len(segments) > 1 {
		v.Shape = ShapeDiscreteSet
	} else {
		v.Shape = ShapeScalar
	}
	for _, segment := range segments {
		unit := sharedUnit
		if segment.interval && segment.upper.unit != "" {
			unit = segment.upper.unit
		}
		lower, reason := canonicalAtom(segment.lower, unit, profile)
		if reason != "" {
			return reason
		}
		switch v.Shape {
		case ShapeScalar:
			v.Scalar = &lower
		case ShapeDiscreteSet:
			v.Values = append(v.Values, lower)
		case ShapeIntervalSet:
			upper, reason := canonicalAtom(segment.upper, sharedUnit, profile)
			if reason != "" {
				return reason
			}
			interval := Interval{Lower: lower, Upper: upper, LowerInclusive: segment.lowerInclusive, UpperInclusive: segment.upperInclusive}
			if err := validateInterval(interval); err != nil {
				return ReasonInvalidInterval
			}
			v.Intervals = append(v.Intervals, interval)
		}
	}
	return ""
}

func (p *rawParser) tolerance(nominalAtom rawAtom, profile Profile) (*Tolerance, Reason) {
	amountAtom, reason := p.atom()
	if reason != "" {
		return nil, reason
	}
	nominal, reason := canonicalAtom(nominalAtom, "", profile)
	if reason != "" {
		return nil, reason
	}
	tolerance := &Tolerance{Nominal: nominal, Type: ToleranceAbsolute}
	if amountAtom.unit == "%" || p.takeOne("%") != "" {
		if amountAtom.unit != "" && amountAtom.unit != "%" {
			return nil, ReasonUnsupportedShape
		}
		tolerance.Type = ToleranceRelative
		tolerance.Amount = amountAtom.number
	} else {
		tolerance.Amount, reason = canonicalAtom(amountAtom, "", profile)
		if reason != "" {
			return nil, reason
		}
	}
	amount := tolerance.Amount.rat()
	if amount.Sign() < 0 {
		return nil, ReasonUnsupportedShape
	}
	if tolerance.Type == ToleranceRelative {
		amount.Mul(amount, new(big.Rat).Abs(nominal.rat()))
		amount.Quo(amount, big.NewRat(100, 1))
	}
	lower, err := decimalFromRat(new(big.Rat).Sub(nominal.rat(), amount))
	if err != nil {
		return nil, decimalReason(err)
	}
	upper, err := decimalFromRat(new(big.Rat).Add(nominal.rat(), amount))
	if err != nil {
		return nil, decimalReason(err)
	}
	tolerance.Interval = Interval{Lower: lower, Upper: upper, LowerInclusive: true, UpperInclusive: true}
	return tolerance, ""
}

func canonicalAtom(atom rawAtom, sharedUnit string, profile Profile) (Decimal, Reason) {
	unit := atom.unit
	if unit == "" {
		unit = sharedUnit
	}
	if unit == "" && profile.DeclaredInputUnit == "" {
		return Decimal{}, ReasonMissingUnit
	}
	number, err := Convert(atom.number, unit, profile)
	if err != nil {
		if errors.Is(err, ErrUnsupportedPrecision) {
			return Decimal{}, ReasonUnsupportedPrecision
		}
		return Decimal{}, ReasonUnsupportedUnit
	}
	return number, ""
}

func decimalReason(err error) Reason {
	if errors.Is(err, ErrUnsupportedPrecision) {
		return ReasonUnsupportedPrecision
	}
	return ReasonUnsupportedShape
}

func consumeRoleMarker(input string) (string, string) {
	lower := strings.ToLower(input)
	for _, marker := range []struct{ text, role string }{
		{"absolute maximum", RoleAbsoluteMaximum}, {"nominal", RoleNominal},
		{"typical", RoleTypical}, {"typ.", RoleTypical},
	} {
		if strings.HasPrefix(lower, marker.text) {
			rest := input[len(marker.text):]
			if rest == "" || unicode.IsSpace([]rune(rest)[0]) {
				return strings.TrimSpace(rest), marker.role
			}
		}
	}
	return input, ""
}

func startsRoleMarker(input string) bool {
	_, role := consumeRoleMarker(input)
	return role != ""
}

func conditionalSuffix(input string) bool {
	lower := strings.ToLower(input)
	if strings.HasPrefix(lower, "@") {
		return true
	}
	for _, marker := range []string{"at ", "when ", "if ", "under ", "with ", "depends on "} {
		if strings.HasPrefix(lower, marker) {
			return true
		}
	}
	return false
}

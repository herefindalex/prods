package specnumeric

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

var (
	ErrInvalidDecimal       = errors.New("invalid decimal-dot value")
	ErrUnsupportedPrecision = errors.New("unsupported_precision")
	decimalPattern          = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)
)

// Decimal is an immutable exact decimal. Its zero value is not a valid number.
// Authority is its canonical decimal string, never a floating-point value.
type Decimal struct{ text string }

func ParseDecimal(text string) (Decimal, error) {
	// Bound work before regex matching and any big integer allocation.
	if len(text) > 256 {
		return Decimal{}, ErrUnsupportedPrecision
	}
	if !decimalPattern.MatchString(text) {
		return Decimal{}, ErrInvalidDecimal
	}
	negative := strings.HasPrefix(text, "-")
	text = strings.TrimPrefix(strings.TrimPrefix(text, "-"), "+")
	exponent := int64(0)
	if i := strings.IndexAny(text, "eE"); i >= 0 {
		var err error
		exponent, err = strconv.ParseInt(text[i+1:], 10, 32)
		if err != nil {
			return Decimal{}, ErrUnsupportedPrecision
		}
		text = text[:i]
	}
	if i := strings.IndexByte(text, '.'); i >= 0 {
		exponent -= int64(len(text) - i - 1)
		text = text[:i] + text[i+1:]
	}
	digits := strings.TrimLeft(text, "0")
	if digits == "" {
		// Zero has no scientific order; insignificant fractional zeroes cannot
		// turn an exact zero into an out-of-range numeric value.
		return Decimal{text: "0"}, nil
	}
	trimmed := strings.TrimRight(digits, "0")
	exponent += int64(len(digits) - len(trimmed))
	digits = trimmed
	order := int64(len(digits)) - 1 + exponent
	if len(digits) > 34 || order < -24 || order > 24 {
		return Decimal{}, ErrUnsupportedPrecision
	}
	var canonical string
	point := int64(len(digits)) + exponent
	switch {
	case point <= 0:
		canonical = "0." + strings.Repeat("0", int(-point)) + digits
	case point >= int64(len(digits)):
		canonical = digits + strings.Repeat("0", int(point)-len(digits))
	default:
		canonical = digits[:point] + "." + digits[point:]
	}
	if negative {
		canonical = "-" + canonical
	}
	return Decimal{text: canonical}, nil
}

func (d Decimal) String() string { return d.text }

func (d Decimal) valid() bool { return d.text != "" }

func (d Decimal) rat() *big.Rat {
	value, ok := new(big.Rat).SetString(d.text)
	if !ok {
		panic("specnumeric: comparison of invalid Decimal")
	}
	return value
}

// Cmp compares valid decimals exactly. Invalid zero-value Decimals are rejected
// by boundary validators and must not be passed to arithmetic or comparison.
func (d Decimal) Cmp(other Decimal) int { return d.rat().Cmp(other.rat()) }

func (d Decimal) MarshalJSON() ([]byte, error) {
	if !d.valid() {
		return nil, ErrInvalidDecimal
	}
	return json.Marshal(d.text)
}

func (d *Decimal) UnmarshalJSON(encoded []byte) error {
	var text string
	if err := json.Unmarshal(encoded, &text); err != nil {
		return fmt.Errorf("decimal must be a string: %w", err)
	}
	value, err := ParseDecimal(text)
	if err != nil {
		return err
	}
	*d = value
	return nil
}

// decimalFromRat accepts only terminating decimals within the same v1 bounds.
// It never rounds intermediate or converted values.
func decimalFromRat(value *big.Rat) (Decimal, error) {
	denominator := new(big.Int).Set(value.Denom())
	two, five := big.NewInt(2), big.NewInt(5)
	counts := [2]int{}
	for i, factor := range []*big.Int{two, five} {
		for new(big.Int).Mod(denominator, factor).Sign() == 0 {
			denominator.Quo(denominator, factor)
			counts[i]++
		}
	}
	if denominator.Cmp(big.NewInt(1)) != 0 {
		return Decimal{}, ErrUnsupportedPrecision
	}
	places := max(counts[0], counts[1])
	return ParseDecimal(value.FloatString(places))
}

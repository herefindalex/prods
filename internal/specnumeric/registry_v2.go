package specnumeric

import (
	"maps"
	"math/big"
	"sort"
)

const (
	CatalogParserVersion       = "catalog-parser-2"
	CatalogUnitRegistryVersion = "catalog-units-2"
	RoleErrorMagnitude         = "error_magnitude"
)

var catalogUnits = func() map[string]unitDefinition {
	units := maps.Clone(electricalUnits)
	for unit, quantity := range map[string]string{"%": "ratio", "dB": "log_gain", "bit": "memory", "W": "power", "m": "length", "H": "inductance", "cd": "luminous_intensity", "1": "count"} {
		units[unit] = unitDefinition{quantity, unit, 0}
	}
	for unit, definition := range map[string]unitDefinition{
		"mW": {"power", "W", -3}, "kW": {"power", "W", 3},
		"mm": {"length", "m", -3}, "µm": {"length", "m", -6}, "um": {"length", "m", -6}, "μm": {"length", "m", -6}, "nm": {"length", "m", -9}, "cm": {"length", "m", -2},
		"mH": {"inductance", "H", -3}, "µH": {"inductance", "H", -6}, "uH": {"inductance", "H", -6}, "μH": {"inductance", "H", -6}, "nH": {"inductance", "H", -9},
		"mcd": {"luminous_intensity", "cd", -3}, "count": {"count", "1", 0},
	} {
		units[unit] = definition
	}
	for unit := range memoryFactors {
		units[unit] = unitDefinition{"memory", "bit", 0}
	}
	return units
}()

var memoryFactors = map[string]int64{
	"bit": 1, "B": 8, "byte": 8,
	"kbit": 1000, "Mbit": 1000000, "Gbit": 1000000000,
	"kB": 8000, "MB": 8000000, "GB": 8000000000,
	"Kibit": 1024, "Mibit": 1048576, "Gibit": 1073741824,
	"KiB": 8192, "MiB": 8388608, "GiB": 8589934592,
}

func registryForVersion(version string) map[string]unitDefinition {
	switch version {
	case UnitRegistryVersion:
		return electricalUnits
	case CatalogUnitRegistryVersion:
		return catalogUnits
	default:
		return nil
	}
}

func validProfileVersions(p Profile) bool {
	return p.ParserVersion == ParserVersion && p.UnitRegistryVersion == UnitRegistryVersion || p.ParserVersion == CatalogParserVersion && p.UnitRegistryVersion == CatalogUnitRegistryVersion
}

func unitFactor(unit string, definition unitDefinition, version string) *big.Rat {
	if version == CatalogUnitRegistryVersion {
		if factor, ok := memoryFactors[unit]; ok {
			return big.NewRat(factor, 1)
		}
	}
	factor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(abs(definition.exponent))), nil)
	if definition.exponent < 0 {
		return new(big.Rat).SetFrac(big.NewInt(1), factor)
	}
	return new(big.Rat).SetInt(factor)
}

// SupportedProfiles returns explicit templates, never a rule inferred from a
// Spec's name or display unit. Callers may narrow shapes/operators deliberately.
func SupportedProfiles() []Profile {
	canonical := make(map[string]string)
	for unit, definition := range catalogUnits {
		if unit == definition.canonical {
			canonical[definition.quantity] = unit
		}
	}
	quantities := make([]string, 0, len(canonical))
	for quantity := range canonical {
		quantities = append(quantities, quantity)
	}
	sort.Strings(quantities)
	var result []Profile
	for _, quantity := range quantities {
		for _, role := range []string{RoleNominal, RoleTypical, RoleOperatingSupport, RoleMinimumLimit, RoleMaximumLimit, RoleAbsoluteMaximum, RoleErrorMagnitude} {
			p := Profile{Quantity: quantity, CanonicalUnit: canonical[quantity], SemanticRole: role, NumericFormat: NumericFormatDecimalDot, SchemaVersion: SchemaVersion, ParserVersion: CatalogParserVersion, UnitRegistryVersion: CatalogUnitRegistryVersion}
			p.AllowedShapes = []Shape{ShapeScalar}
			p.AllowedOperators = []Operator{OperatorEqual, OperatorLess, OperatorLessEqual, OperatorGreater, OperatorGreaterEqual, OperatorBetween}
			switch role {
			case RoleOperatingSupport:
				p.AllowedShapes = []Shape{ShapeScalar, ShapeDiscreteSet, ShapeIntervalSet}
				p.AllowedOperators = []Operator{OperatorContainsPoint, OperatorContainsRange, OperatorOverlapsRange}
			case RoleNominal:
				if quantity != "count" {
					p.AllowedShapes = append(p.AllowedShapes, ShapeTolerance)
				}
			case RoleMinimumLimit, RoleMaximumLimit, RoleAbsoluteMaximum:
				p.AllowedShapes = append(p.AllowedShapes, ShapeBoundClaim)
			case RoleErrorMagnitude:
				if quantity != "ratio" {
					continue
				}
				p.AllowedShapes = []Shape{ShapeBoundClaim}
				p.AllowedOperators = []Operator{OperatorLess, OperatorLessEqual}
			}
			if quantity == "count" {
				p.DeclaredInputUnit = "1"
			}
			result = append(result, p)
		}
	}
	return result
}

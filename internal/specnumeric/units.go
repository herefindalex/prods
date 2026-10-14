package specnumeric

import (
	"fmt"
)

const UnitRegistryVersion = "electrical-1"

type unitDefinition struct {
	quantity, canonical string
	exponent            int
}

var electricalUnits = map[string]unitDefinition{
	"V": {"voltage", "V", 0}, "mV": {"voltage", "V", -3}, "µV": {"voltage", "V", -6}, "uV": {"voltage", "V", -6}, "μV": {"voltage", "V", -6}, "kV": {"voltage", "V", 3},
	"A": {"current", "A", 0}, "mA": {"current", "A", -3}, "µA": {"current", "A", -6}, "uA": {"current", "A", -6}, "μA": {"current", "A", -6},
	"F": {"capacitance", "F", 0}, "mF": {"capacitance", "F", -3}, "µF": {"capacitance", "F", -6}, "uF": {"capacitance", "F", -6}, "μF": {"capacitance", "F", -6}, "nF": {"capacitance", "F", -9}, "pF": {"capacitance", "F", -12},
	"Ω": {"resistance", "Ω", 0}, "Ohm": {"resistance", "Ω", 0}, "mΩ": {"resistance", "Ω", -3}, "mOhm": {"resistance", "Ω", -3}, "kΩ": {"resistance", "Ω", 3}, "kOhm": {"resistance", "Ω", 3}, "MΩ": {"resistance", "Ω", 6}, "MOhm": {"resistance", "Ω", 6},
	"Hz": {"frequency", "Hz", 0}, "kHz": {"frequency", "Hz", 3}, "MHz": {"frequency", "Hz", 6}, "GHz": {"frequency", "Hz", 9},
}

func Convert(value Decimal, unit string, profile Profile) (Decimal, error) {
	if err := ValidateProfile(profile); err != nil {
		return Decimal{}, err
	}
	if !value.valid() {
		return Decimal{}, ErrInvalidDecimal
	}
	if unit == "" {
		unit = profile.DeclaredInputUnit
	}
	definition, ok := registryForVersion(profile.UnitRegistryVersion)[unit]
	if !ok || definition.quantity != profile.Quantity || definition.canonical != profile.CanonicalUnit {
		return Decimal{}, fmt.Errorf("unsupported_unit: %q", unit)
	}
	converted := value.rat()
	converted.Mul(converted, unitFactor(unit, definition, profile.UnitRegistryVersion))
	return decimalFromRat(converted)
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

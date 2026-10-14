package referencecatalog

import (
	"prods/internal/catalogfilter"
	"prods/internal/specnumeric"
)

// Stable IDs explicitly declare quantity and purpose. Names and preferred
// display units never decide searchable meaning.
func searchProfiles() map[string]catalogfilter.Profile {
	numeric := func(quantity, role string) catalogfilter.Profile {
		for _, p := range specnumeric.SupportedProfiles() {
			if p.Quantity == quantity && p.SemanticRole == role {
				return catalogfilter.NumericProfile(p)
			}
		}
		panic("missing sample numeric profile")
	}
	choice := catalogfilter.Profile{Version: 1, Kind: "choice", TextRule: catalogfilter.TextRule, Operators: []string{"equals"}}
	multi := catalogfilter.Profile{Version: 1, Kind: "multi_choice", TextRule: catalogfilter.TextRule, Delimiter: ";", Operators: []string{"equals"}}
	return map[string]catalogfilter.Profile{
		"spc_input_voltage":         numeric("voltage", specnumeric.RoleOperatingSupport),
		"spc_output_voltage":        numeric("voltage", specnumeric.RoleNominal),
		"spc_output_current":        numeric("current", specnumeric.RoleMaximumLimit),
		"spc_quiescent_current":     numeric("current", specnumeric.RoleTypical),
		"spc_dropout":               numeric("voltage", specnumeric.RoleMaximumLimit),
		"spc_switching_frequency":   numeric("frequency", specnumeric.RoleNominal),
		"spc_efficiency":            numeric("ratio", specnumeric.RoleNominal),
		"spc_gain":                  numeric("log_gain", specnumeric.RoleNominal),
		"spc_bandwidth":             numeric("frequency", specnumeric.RoleNominal),
		"spc_logic_family":          choice,
		"spc_memory_density":        numeric("memory", specnumeric.RoleNominal),
		"spc_rds_on":                numeric("resistance", specnumeric.RoleMaximumLimit),
		"spc_reverse_voltage":       numeric("voltage", specnumeric.RoleMaximumLimit),
		"spc_power":                 numeric("power", specnumeric.RoleNominal),
		"spc_measurement_range":     catalogfilter.DefaultTextProfile(),
		"spc_interface":             multi,
		"spc_accuracy":              numeric("ratio", specnumeric.RoleErrorMagnitude),
		"spc_positions":             numeric("count", specnumeric.RoleNominal),
		"spc_pitch":                 numeric("length", specnumeric.RoleNominal),
		"spc_rated_contact_current": numeric("current", specnumeric.RoleMaximumLimit),
		"spc_mounting":              choice,
		"spc_resistance":            numeric("resistance", specnumeric.RoleNominal),
		"spc_capacitance":           numeric("capacitance", specnumeric.RoleNominal),
		"spc_inductance":            numeric("inductance", specnumeric.RoleNominal),
		"spc_tolerance":             numeric("ratio", specnumeric.RoleErrorMagnitude),
		"spc_wavelength":            numeric("length", specnumeric.RoleNominal),
		"spc_luminous_intensity":    numeric("luminous_intensity", specnumeric.RoleNominal),
		"spc_coil_voltage":          numeric("voltage", specnumeric.RoleNominal),
		"spc_contact_rating":        catalogfilter.DefaultTextProfile(),
		"spc_core":                  choice,
		"spc_clock":                 numeric("frequency", specnumeric.RoleNominal),
		"spc_wireless_standard":     multi,
		"spc_breakdown_voltage":     numeric("voltage", specnumeric.RoleMinimumLimit),
		"spc_package":               catalogfilter.DefaultTextProfile(),
	}
}

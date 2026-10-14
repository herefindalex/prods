package specnumeric

import "fmt"

const (
	SchemaVersion           = 1
	ParserVersion           = "electrical-parser-1"
	NumericFormatDecimalDot = "decimal-dot"
	RoleOperatingSupport    = "operating_support"
	RoleNominal             = "nominal"
	RoleTypical             = "typical"
	RoleMinimumLimit        = "minimum_limit"
	RoleMaximumLimit        = "maximum_limit"
	RoleAbsoluteMaximum     = "absolute_maximum"
)

type Profile struct {
	Quantity            string     `json:"quantity"`
	SemanticRole        string     `json:"semantic_role"`
	AllowedShapes       []Shape    `json:"allowed_shapes"`
	AllowedOperators    []Operator `json:"allowed_operators"`
	CanonicalUnit       string     `json:"canonical_unit"`
	DeclaredInputUnit   string     `json:"declared_input_unit"`
	NumericFormat       string     `json:"numeric_format"`
	SchemaVersion       int        `json:"schema_version"`
	ParserVersion       string     `json:"parser_version"`
	UnitRegistryVersion string     `json:"unit_registry_version"`
}

func ValidateProfile(profile Profile) error {
	if profile.SchemaVersion != SchemaVersion || !validProfileVersions(profile) || profile.NumericFormat != NumericFormatDecimalDot {
		return fmt.Errorf("unsupported profile version or numeric format")
	}
	switch profile.SemanticRole {
	case RoleOperatingSupport, RoleNominal, RoleTypical, RoleMinimumLimit, RoleMaximumLimit, RoleAbsoluteMaximum:
	case RoleErrorMagnitude:
		if profile.ParserVersion != CatalogParserVersion || profile.Quantity != "ratio" {
			return fmt.Errorf("unsupported error magnitude profile")
		}
	default:
		return fmt.Errorf("unsupported semantic role: %q", profile.SemanticRole)
	}
	units := registryForVersion(profile.UnitRegistryVersion)
	unit, ok := units[profile.CanonicalUnit]
	if !ok || unit.exponent != 0 || unit.canonical != profile.CanonicalUnit || unit.quantity != profile.Quantity {
		return fmt.Errorf("invalid canonical unit for quantity")
	}
	if profile.DeclaredInputUnit != "" {
		input, ok := units[profile.DeclaredInputUnit]
		if !ok || input.quantity != profile.Quantity {
			return fmt.Errorf("invalid declared input unit for quantity")
		}
	}
	if len(profile.AllowedShapes) == 0 || len(profile.AllowedOperators) == 0 {
		return fmt.Errorf("profile must explicitly allow shapes and operators")
	}
	shapes := map[Shape]bool{}
	for _, shape := range profile.AllowedShapes {
		if profile.SemanticRole == RoleErrorMagnitude && shape != ShapeBoundClaim || profile.Quantity == "count" && shape == ShapeTolerance {
			return fmt.Errorf("shape incompatible with quantity or role")
		}
		if !knownShape(shape) || shapes[shape] {
			return fmt.Errorf("invalid or duplicate allowed shape: %q", shape)
		}
		shapes[shape] = true
	}
	operators := map[Operator]bool{}
	for _, operator := range profile.AllowedOperators {
		if profile.SemanticRole == RoleErrorMagnitude && operator != OperatorLess && operator != OperatorLessEqual {
			return fmt.Errorf("unsupported error magnitude operator")
		}
		if !knownOperator(operator) || operators[operator] {
			return fmt.Errorf("invalid or duplicate allowed operator: %q", operator)
		}
		operators[operator] = true
	}
	return nil
}

func (p Profile) AllowsShape(shape Shape) bool {
	for _, allowed := range p.AllowedShapes {
		if shape == allowed {
			return true
		}
	}
	return false
}
func (p Profile) AllowsOperator(operator Operator) bool {
	for _, allowed := range p.AllowedOperators {
		if operator == allowed {
			return true
		}
	}
	return false
}

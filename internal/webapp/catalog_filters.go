package webapp

import (
	"fmt"
	"net/url"
	"prods/internal/catalogfilter"
	"prods/internal/localization"
	"prods/internal/publishing"
	"prods/internal/specnumeric"
	"slices"
	"sort"
	"strings"
)

type catalogFilterControl struct {
	Hidden                                                         []catalogHiddenField
	ClearURL                                                       string
	Field, Label, Kind, Operator, Value, Lower, Upper, Unit, Error string
	Operators                                                      []catalogFilterOperator
	Options                                                        []catalogFilterOption
	Selected                                                       []string
}
type catalogFilterOperator struct {
	Value, Label string
	Selected     bool
}
type catalogFilterOption struct {
	Value, Label string
	Selected     bool
}
type catalogHiddenField struct{ Name, Value string }

func filterField(key string) (string, string, bool) {
	if !strings.HasPrefix(key, "f.") {
		return "", "", false
	}
	field, property, ok := strings.Cut(strings.TrimPrefix(key, "f."), ".")
	return field, property, ok && field != ""
}
func parseCatalogFilters(values url.Values) ([]catalogfilter.Clause, error) {
	grouped := map[string]url.Values{}
	invalid := func(field, message string) ([]catalogfilter.Clause, error) {
		return nil, &publishing.CatalogFilterError{Field: field, Message: message}
	}
	for key, value := range values {
		if !strings.HasPrefix(key, "f.") {
			continue
		}
		field, property, ok := filterField(key)
		if !ok || !slices.Contains([]string{"op", "value", "values", "lower", "upper", "unit"}, property) {
			return invalid(field, "unknown filter parameter")
		}
		if property != "values" && len(value) != 1 {
			return invalid(field, "duplicate filter parameter")
		}
		for _, v := range value {
			if len(v) > 512 {
				return invalid(field, "value exceeds 512 bytes")
			}
		}
		if grouped[field] == nil {
			grouped[field] = url.Values{}
		}
		grouped[field][property] = value
	}
	fields := make([]string, 0, len(grouped))
	for field := range grouped {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	clauses := []catalogfilter.Clause{}
	specs := 0
	for _, field := range fields {
		v := grouped[field]
		selected := []string{}
		for _, value := range append(slices.Clone(v["values"]), v["value"]...) {
			if strings.TrimSpace(value) != "" {
				selected = append(selected, value)
			}
		}
		if len(v["values"]) > 20 || len(selected) > 20 {
			return invalid(field, "at most twenty values are allowed")
		}
		lower, upper := strings.TrimSpace(v.Get("lower")), strings.TrimSpace(v.Get("upper"))
		if len(selected) == 0 && lower == "" && upper == "" {
			continue
		}
		clause := catalogfilter.Clause{Field: field, Operator: v.Get("op"), Values: selected}
		if clause.Operator == "" {
			return invalid(field, "operator required")
		}
		if clause.Operator != "equals" && clause.Operator != "contains" {
			if !strings.HasPrefix(field, "spec:") {
				return invalid(field, "unsupported text operator")
			}
			predicate := specnumeric.Predicate{SpecID: strings.TrimPrefix(field, "spec:"), Operator: specnumeric.Operator(clause.Operator), Unit: v.Get("unit")}
			decimal := func(raw string) (*specnumeric.Decimal, error) {
				if raw == "" {
					return nil, nil
				}
				value, err := specnumeric.ParseDecimal(raw)
				return &value, err
			}
			var err error
			if len(selected) > 1 {
				return invalid(field, "numeric condition requires one operand")
			}
			if len(selected) == 1 {
				predicate.Value, err = decimal(selected[0])
				if err != nil {
					return invalid(field, "invalid number")
				}
			}
			predicate.Lower, err = decimal(lower)
			if err != nil {
				return invalid(field, "invalid lower bound")
			}
			predicate.Upper, err = decimal(upper)
			if err != nil {
				return invalid(field, "invalid upper bound")
			}
			clause.Numeric = &predicate
			clause.Values = nil
		} else if lower != "" || upper != "" || v.Get("unit") != "" {
			return invalid(field, "text condition cannot use numeric operands")
		}
		if strings.HasPrefix(field, "spec:") {
			specs++
			if specs > 16 {
				return invalid(field, "at most sixteen specification conditions are allowed")
			}
		}
		clauses = append(clauses, clause)
	}
	// Preserve the existing stable-ID URL parameters as conditions.
	for _, field := range []string{"manufacturer", "brand", "category"} {
		if value := strings.TrimSpace(values.Get(field + "_id")); value != "" {
			clauses = append(clauses, catalogfilter.Clause{Field: field, Operator: "equals", Values: []string{value}})
		}
	}
	return clauses, nil
}

func catalogFilterForm(snapshot publishing.CatalogSnapshot, values url.Values) []catalogFilterControl {
	text := localization.For(snapshot.Locale)
	controls := []catalogFilterControl{}
	labels := map[string]string{"manufacturer": text.Manufacturer, "brand": text.Brand, "category": text.Category, "application": text.Applications, "lifecycle": text.Lifecycle, "package": text.Package}
	for _, view := range snapshot.ScopeViews {
		for _, spec := range view.ApplicableSpecifications {
			labels["spec:"+spec.ID] = spec.Name
		}
	}
	definitions := slices.Clone(snapshot.Definitions)
	known := map[string]bool{}
	for _, d := range definitions {
		known[d.Field] = true
	}
	unknown := map[string]bool{}
	for key := range values {
		field, _, ok := filterField(key)
		if ok && !known[field] {
			unknown[field] = true
		}
	}
	keys := []string{}
	for field := range unknown {
		keys = append(keys, field)
	}
	sort.Strings(keys)
	for _, field := range keys {
		definitions = append(definitions, publishing.PublicFilterDefinition{Field: field, Profile: catalogfilter.DefaultTextProfile()})
	}
	for _, d := range definitions {
		prefix := "f." + d.Field + "."
		control := catalogFilterControl{Field: d.Field, Label: labels[d.Field], Kind: d.Profile.Kind, Operator: values.Get(prefix + "op"), Value: values.Get(prefix + "value"), Lower: values.Get(prefix + "lower"), Upper: values.Get(prefix + "upper"), Unit: values.Get(prefix + "unit"), Selected: slices.Clone(values[prefix+"values"])}
		if control.Label == "" {
			control.Label = d.Field
		}
		if slices.Contains([]string{"manufacturer", "brand", "category"}, d.Field) {
			if legacy := values.Get(d.Field + "_id"); legacy != "" {
				control.Selected = append(control.Selected, legacy)
			}
		}
		if !known[d.Field] {
			control.Error = text.FilterUnavailable
		}
		if control.Operator == "" && len(d.Profile.Operators) > 0 {
			control.Operator = d.Profile.Operators[0]
		}
		operators := slices.Clone(d.Profile.Operators)
		if control.Operator != "" && !slices.Contains(operators, control.Operator) {
			operators = append(operators, control.Operator)
		}
		for _, op := range operators {
			label := op
			switch op {
			case "equals":
				label = text.FilterEquals
			case "contains":
				label = text.FilterContains
			case "contains_point":
				label = text.FilterSupports
			case "contains_range":
				label = text.FilterCovers
			case "between":
				label = text.FilterBetween
			}
			control.Operators = append(control.Operators, catalogFilterOperator{Value: op, Label: label, Selected: op == control.Operator})
		}
		if d.Profile.Numeric != nil && control.Unit == "" {
			control.Unit = d.Profile.Numeric.CanonicalUnit
		}
		options := slices.Clone(snapshot.Options[d.Field])
		selected := append(slices.Clone(control.Selected), control.Value)
		for _, value := range selected {
			if value == "" {
				continue
			}
			found := false
			for _, option := range options {
				if option.Value == value {
					found = true
				}
			}
			if !found {
				options = append(options, publishing.CatalogFilterOption{Value: value, Label: value})
			}
		}
		for _, option := range options {
			control.Options = append(control.Options, catalogFilterOption{Value: option.Value, Label: option.Label, Selected: slices.Contains(selected, option.Value)})
		}
		for _, hidden := range catalogFilterHiddenFields(values) {
			if !strings.HasPrefix(hidden.Name, "f."+d.Field+".") && hidden.Name != d.Field+"_id" {
				control.Hidden = append(control.Hidden, hidden)
			}
		}
		clear := url.Values{}
		for key, items := range values {
			if key != "page" && !strings.HasPrefix(key, "f."+d.Field+".") && key != d.Field+"_id" {
				clear[key] = slices.Clone(items)
			}
		}
		control.ClearURL = "?" + clear.Encode()
		controls = append(controls, control)
	}
	return controls
}
func validateCatalogFilterFields(snapshot publishing.CatalogSnapshot, values url.Values) error {
	known := map[string]bool{}
	for _, d := range snapshot.Definitions {
		known[d.Field] = true
	}
	keys := []string{}
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if field, _, ok := filterField(key); ok && !known[field] {
			return &publishing.CatalogFilterError{Field: field, Message: "field is not applicable with one published meaning in this scope"}
		}
	}
	return nil
}
func catalogFilterHiddenFields(values url.Values) []catalogHiddenField {
	active := map[string]bool{}
	for key, items := range values {
		if field, property, ok := filterField(key); ok && slices.Contains([]string{"value", "values", "lower", "upper"}, property) {
			for _, value := range items {
				if strings.TrimSpace(value) != "" {
					active[field] = true
				}
			}
		}
	}
	keys := []string{}
	for key := range values {
		field, _, isFilter := filterField(key)
		if (isFilter && active[field]) || slices.Contains([]string{"manufacturer_id", "brand_id", "category_id", "page_size"}, key) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	result := []catalogHiddenField{}
	for _, key := range keys {
		for _, value := range values[key] {
			result = append(result, catalogHiddenField{Name: key, Value: value})
		}
	}
	return result
}
func catalogFilterErrorLabel(err error, text localization.Messages) string {
	return fmt.Sprintf("%s: %s", text.FilterError, err)
}

package catalogfilter

import "prods/internal/specnumeric"

type Scope struct {
	Kind  string `json:"kind"`
	ID    string `json:"id,omitempty"`
	Query string `json:"query,omitempty"`
}
type Clause struct {
	Field    string                 `json:"field"`
	Operator string                 `json:"operator"`
	Values   []string               `json:"values,omitempty"`
	Numeric  *specnumeric.Predicate `json:"numeric,omitempty"`
}

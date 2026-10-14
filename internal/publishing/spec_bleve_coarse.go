package publishing

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"prods/internal/specnumeric"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search/query"
)

const specBleveVersion = "bleve-specs-1"

// Every supported decimal has at most 57 fractional places and magnitude
// below 10^25. Scaling by 10^57 therefore fits exactly into 82 digits.
func specBleveDecimalKey(value specnumeric.Decimal) (string, error) {
	d, err := specnumeric.ParseDecimal(value.String())
	if err != nil {
		return "", err
	}
	text := d.String()
	negative := strings.HasPrefix(text, "-")
	text = strings.TrimPrefix(text, "-")
	parts := strings.SplitN(text, ".", 2)
	frac := ""
	if len(parts) == 2 {
		frac = parts[1]
	}
	if len(frac) > 57 {
		return "", specnumeric.ErrUnsupportedPrecision
	}
	digits := strings.TrimLeft(parts[0]+frac+strings.Repeat("0", 57-len(frac)), "0")
	if len(digits) > 82 {
		return "", specnumeric.ErrUnsupportedPrecision
	}
	digits = strings.Repeat("0", 82-len(digits)) + digits
	if !negative {
		return "1" + digits, nil
	}
	complement := []byte(digits)
	for i, c := range complement {
		complement[i] = '9' - (c - '0')
	}
	return "0" + string(complement), nil
}

type specBleveDocument struct {
	Binding        SpecCandidateBinding      `json:"binding"`
	CategoryID     string                    `json:"category_id"`
	CategoryTrail  []string                  `json:"category_trail"`
	ManufacturerID string                    `json:"manufacturer_id"`
	Definitions    []PublicNumericDefinition `json:"definitions"`
	Facts          []PublicNumericFact       `json:"facts"`
}

func specBleveDocuments(active []ActivePublication) ([]specBleveDocument, string, error) {
	docs := make([]specBleveDocument, 0, len(active))
	seen := make(map[string]bool, len(active))
	for _, a := range active {
		if a.ProductID == "" || a.View.ID != a.ProductID || seen[a.ProductID] || a.PublicRevision <= 0 || a.SourceRevision <= 0 || a.SiteEpoch <= 0 || a.VisibilityGeneration < 0 {
			return nil, "", fmt.Errorf("invalid Bleve publication binding: %q", a.ProductID)
		}
		seen[a.ProductID] = true
		for _, def := range a.View.NumericDefinitions {
			if def.SpecID == "" || def.SpecSemanticVersion <= 0 || specnumeric.ValidateProfile(def.Profile) != nil {
				return nil, "", fmt.Errorf("invalid Bleve numeric definition")
			}
		}
		for _, fact := range a.View.NumericFacts {
			if fact.SpecID == "" || fact.SpecSemanticVersion <= 0 || specnumeric.ValidateProfile(fact.Profile) != nil || specnumeric.ValidateValue(fact.Value, fact.Profile) != nil {
				return nil, "", fmt.Errorf("invalid Bleve numeric fact")
			}
		}
		d := specBleveDocument{Binding: SpecCandidateBinding{ProductID: a.ProductID, PublicRevision: a.PublicRevision, SourceRevision: a.SourceRevision, SiteEpoch: a.SiteEpoch, VisibilityGeneration: a.VisibilityGeneration, ProjectionVersion: SpecProjectionVersion, SpecIDs: SpecProjectionIDs(a.View)}, CategoryID: a.View.CategoryID, ManufacturerID: a.View.ManufacturerID, Definitions: a.View.NumericDefinitions, Facts: a.View.NumericFacts, CategoryTrail: []string{}}
		for _, c := range a.View.CategoryTrail {
			d.CategoryTrail = append(d.CategoryTrail, c.ID)
		}
		docs = append(docs, d)
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].Binding.ProductID < docs[j].Binding.ProductID })
	// Round-trip is also the immutable deep copy: caller-owned nested slices and
	// values can never alter cache contents after an atomic replacement.
	encoded, err := json.Marshal(docs)
	if err != nil {
		return nil, "", err
	}
	if err = json.Unmarshal(encoded, &docs); err != nil {
		return nil, "", err
	}
	digest := sha256.Sum256(encoded)
	return docs, hex.EncodeToString(digest[:]), nil
}

func specActiveDigest(active []ActivePublication) (string, error) {
	_, digest, err := specBleveDocuments(active)
	return digest, err
}

func specBleveValidateDocuments(docs []specBleveDocument) ([]specBleveDocument, string, error) {
	active := make([]ActivePublication, 0, len(docs))
	for _, d := range docs {
		a := ActivePublication{ProductID: d.Binding.ProductID, PublicRevision: d.Binding.PublicRevision, SourceRevision: d.Binding.SourceRevision, SiteEpoch: d.Binding.SiteEpoch, VisibilityGeneration: d.Binding.VisibilityGeneration, View: PublicView{ID: d.Binding.ProductID, CategoryID: d.CategoryID, ManufacturerID: d.ManufacturerID, NumericDefinitions: d.Definitions, NumericFacts: d.Facts}}
		for _, id := range d.CategoryTrail {
			a.View.CategoryTrail = append(a.View.CategoryTrail, CategoryRef{ID: id})
		}
		active = append(active, a)
	}
	canonical, digest, err := specBleveDocuments(active)
	if err != nil {
		return nil, "", err
	}
	a, _ := json.Marshal(docs)
	b, _ := json.Marshal(canonical)
	if !bytes.Equal(a, b) {
		return nil, "", fmt.Errorf("noncanonical Bleve inventory payload")
	}
	return canonical, digest, nil
}

type specBleveEnvelope struct {
	present, broad bool
	low, high      string
}

func specBleveEnvelopes(d specBleveDocument) map[string]specBleveEnvelope {
	result := make(map[string]specBleveEnvelope, len(d.Binding.SpecIDs))
	for _, id := range d.Binding.SpecIDs {
		result[id] = specBleveEnvelope{present: true}
	}
	for _, f := range d.Facts {
		e := result[f.SpecID]
		var values []specnumeric.Decimal
		if f.Value.Context.Kind != specnumeric.ContextUnconditional {
			e.broad = true
		} else {
			switch f.Value.Shape {
			case specnumeric.ShapeScalar:
				values = append(values, *f.Value.Scalar)
			case specnumeric.ShapeTolerance:
				if f.Profile.SemanticRole == specnumeric.RoleNominal {
					values = append(values, f.Value.Tolerance.Nominal)
				} else {
					e.broad = true
				}
			case specnumeric.ShapeDiscreteSet:
				if f.Profile.SemanticRole == specnumeric.RoleOperatingSupport {
					values = f.Value.Values
				} else {
					e.broad = true
				}
			case specnumeric.ShapeIntervalSet:
				if f.Profile.SemanticRole == specnumeric.RoleOperatingSupport {
					for _, interval := range f.Value.Intervals {
						values = append(values, interval.Lower, interval.Upper)
					}
				} else {
					e.broad = true
				}
			default:
				e.broad = true
			}
		}
		for _, v := range values {
			key, err := specBleveDecimalKey(v)
			if err != nil {
				e.broad = true
				continue
			}
			if e.low == "" || key < e.low {
				e.low = key
			}
			if e.high == "" || key > e.high {
				e.high = key
			}
		}
		result[f.SpecID] = e
	}
	return result
}

func specBleveNamespace(id string) string {
	h := sha256.Sum256([]byte(id))
	return "s_" + hex.EncodeToString(h[:])
}
func specBleveProductDocID(id string) string { return "product:" + id }

type specBlevePlan struct{ id, lowMax, highMin string }

// A global profile conflict can change unit interpretation. In that case use
// presence only, leaving profile validation and exact truth to the engine.
func specBlevePlans(docs []specBleveDocument, predicates []specnumeric.Predicate) []specBlevePlan {
	plans := make([]specBlevePlan, 0, len(predicates))
	for _, p := range predicates {
		plan := specBlevePlan{id: p.SpecID}
		var definition *PublicNumericDefinition
		compatible := true
		for _, d := range docs {
			defs := slices.Clone(d.Definitions)
			for _, f := range d.Facts {
				defs = append(defs, PublicNumericDefinition{SpecID: f.SpecID, SpecSemanticVersion: f.SpecSemanticVersion, Profile: f.Profile})
			}
			for _, def := range defs {
				if def.SpecID != p.SpecID {
					continue
				}
				if definition == nil {
					copy := def
					definition = &copy
				} else if !compatibleSpecDefinitions(*definition, def) {
					compatible = false
				}
			}
		}
		if definition != nil && compatible {
			if normalized, err := specnumeric.NormalizePredicate(p, definition.Profile); err == nil {
				key := func(d *specnumeric.Decimal) string {
					if d == nil {
						return ""
					}
					v, _ := specBleveDecimalKey(*d)
					return v
				}
				switch normalized.Operator {
				case specnumeric.OperatorEqual, specnumeric.OperatorContainsPoint:
					plan.lowMax = key(normalized.Value)
					plan.highMin = plan.lowMax
				case specnumeric.OperatorContainsRange:
					plan.lowMax = key(normalized.Lower)
					plan.highMin = key(normalized.Upper)
				case specnumeric.OperatorBetween, specnumeric.OperatorOverlapsRange:
					plan.lowMax = key(normalized.Upper)
					plan.highMin = key(normalized.Lower)
				case specnumeric.OperatorLess, specnumeric.OperatorLessEqual:
					plan.lowMax = key(normalized.Value)
				case specnumeric.OperatorGreater, specnumeric.OperatorGreaterEqual:
					plan.highMin = key(normalized.Value)
				}
			}
		}
		plans = append(plans, plan)
	}
	return plans
}

func specBleveCoarseMatch(doc specBleveDocument, plans []specBlevePlan) bool {
	envelopes := specBleveEnvelopes(doc)
	for _, p := range plans {
		e := envelopes[p.id]
		if !e.present {
			return false
		}
		if e.broad {
			continue
		}
		if p.lowMax != "" && (e.low == "" || e.low > p.lowMax) {
			return false
		}
		if p.highMin != "" && (e.high == "" || e.high < p.highMin) {
			return false
		}
	}
	return true
}

func specBleveQuery(plans []specBlevePlan) query.Query {
	term := func(field, value string) query.Query { q := bleve.NewTermQuery(value); q.SetField(field); return q }
	clauses := []query.Query{term("product", "yes")}
	inclusive := true
	for _, p := range plans {
		field := specBleveNamespace(p.id)
		clauses = append(clauses, term(field+"_present", "yes"))
		var bounds []query.Query
		if p.lowMax != "" {
			q := bleve.NewTermRangeInclusiveQuery("", p.lowMax, &inclusive, &inclusive)
			q.SetField(field + "_low")
			bounds = append(bounds, q)
		}
		if p.highMin != "" {
			q := bleve.NewTermRangeInclusiveQuery(p.highMin, "", &inclusive, &inclusive)
			q.SetField(field + "_high")
			bounds = append(bounds, q)
		}
		if len(bounds) > 0 {
			clauses = append(clauses, bleve.NewDisjunctionQuery(term(field+"_broad", "yes"), bleve.NewConjunctionQuery(bounds...)))
		}
	}
	return bleve.NewConjunctionQuery(clauses...)
}

func specBleveIndexFields(doc specBleveDocument) (map[string]any, error) {
	payload, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	fields := map[string]any{"product": "yes", "payload": string(payload)}
	for id, e := range specBleveEnvelopes(doc) {
		ns := specBleveNamespace(id)
		fields[ns+"_present"] = "yes"
		if e.broad {
			fields[ns+"_broad"] = "yes"
		}
		if e.low != "" {
			fields[ns+"_low"] = e.low
			fields[ns+"_high"] = e.high
		}
	}
	return fields, nil
}

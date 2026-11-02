package publishing

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

	"prods/internal/specnumeric"
)

const SpecProjectionVersion = "spec-ids-1"

var ErrInvalidSpecSearch = errors.New("invalid spec search")

// ErrUnsupportedSpecSearch distinguishes valid query shapes unsupported by a frozen profile.
var ErrUnsupportedSpecSearch = errors.New("unsupported spec search")

// SpecCandidateBinding includes the actual field-row inventory, never a stored
// row count. All revision fields bind the projection to one frozen activation.
type SpecCandidateBinding struct {
	ProductID            string
	PublicRevision       int64
	SourceRevision       int64
	SiteEpoch            int64
	VisibilityGeneration int64
	ProjectionVersion    string
	SpecIDs              []string
}

type SpecCandidateSnapshot struct {
	ProjectionVersion string
	SourceDigest      string
	Inventory         []SpecCandidateBinding
	Candidates        []SpecCandidateBinding
}

// SpecSearchRepository is optional so older publication repositories can use
// the same exact oracle over their complete admitted active views.
type SpecSearchRepository interface {
	SpecSearchCandidates(context.Context, []string, string) (SpecCandidateSnapshot, error)
	RebuildSpecSearchProjection(context.Context, []ActivePublication) error
}

type SpecSearchRequest struct {
	Predicates     []specnumeric.Predicate
	CategoryID     string
	ManufacturerID string
	Locale         string
	Page           int
	PageSize       int
}

type SpecSearchResult struct {
	Rows     []PublicView `json:"rows"`
	Total    int          `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
}

// SpecProjectionIDs is deliberately coarse: no interval envelopes or numeric
// comparisons are stored. Exact facts are read only from the active PublicView.
func SpecProjectionIDs(view PublicView) []string {
	ids := make([]string, 0, len(view.NumericFacts))
	for _, fact := range view.NumericFacts {
		if fact.SpecID != "" && specnumeric.ValidateProfile(fact.Profile) == nil && specnumeric.ValidateValue(fact.Value, fact.Profile) == nil {
			ids = append(ids, fact.SpecID)
		}
	}
	sort.Strings(ids)
	return slices.Compact(ids)
}

func (e *Engine) SearchSpecs(ctx context.Context, request SpecSearchRequest) (SpecSearchResult, error) {
	result := SpecSearchResult{Rows: []PublicView{}, Page: request.Page, PageSize: request.PageSize}
	if request.Page < 1 || request.PageSize < 1 || request.PageSize > 100 {
		return result, fmt.Errorf("%w: page must be positive and page_size must be 1..100", ErrInvalidSpecSearch)
	}
	ids := make([]string, 0, len(request.Predicates))
	for _, predicate := range request.Predicates {
		if err := validateSpecPredicate(predicate); err != nil {
			return result, err
		}
		ids = append(ids, predicate.SpecID)
	}
	snapshot, projected, candidateErr := e.readSpecCandidates(ctx, request)
	// The database read is outside the gate. Everything establishing truth,
	// visibility, scope, count and pagination uses this one current index.
	e.gate.RLock()
	defer e.gate.RUnlock()
	if e.index == nil || e.index.unhealthy != nil {
		return result, ErrPublicUnavailable
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	// Resolve and recheck locale against the same admitted site epoch as facts.
	// HTTP may have read active Website settings before a site-wide activation.
	if len(e.index.views) > 0 {
		view := e.index.views[0]
		if request.Locale == "" {
			request.Locale = view.DefaultLocale
			if request.Locale == "" {
				request.Locale = view.Language
			}
		}
		if !slices.Contains(publicSupportedLocales(view), request.Locale) {
			return result, fmt.Errorf("%w: locale is not enabled", ErrInvalidSpecSearch)
		}
	}

	definitions := make(map[string]PublicNumericDefinition)
	requested := make(map[string]bool, len(ids))
	for _, id := range ids {
		requested[id] = true
	}
	for _, active := range e.index.byID {
		if !specSearchScope(active.View, request) {
			continue
		}
		for _, definition := range active.View.NumericDefinitions {
			if !requested[definition.SpecID] {
				continue
			}
			if specnumeric.ValidateProfile(definition.Profile) != nil {
				return result, ErrPublicUnavailable
			}
			if prior, ok := definitions[definition.SpecID]; ok && !compatibleSpecDefinitions(prior, definition) {
				return result, ErrPublicUnavailable
			}
			definitions[definition.SpecID] = definition
		}
	}
	for _, predicate := range request.Predicates {
		if definition, exists := definitions[predicate.SpecID]; exists {
			if _, err := specnumeric.NormalizePredicate(predicate, definition.Profile); err != nil {
				return result, fmt.Errorf("%w: %w: %s: %v", ErrInvalidSpecSearch, ErrUnsupportedSpecSearch, predicate.SpecID, err)
			}
		}
	}
	candidates, projected := e.admitSpecCandidatesLocked(snapshot, projected, candidateErr)
	matches := make([]PublicView, 0)
	for id, active := range e.index.byID {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if projected && !candidates[id] || !specSearchScope(active.View, request) {
			continue
		}
		if specViewTruth(active.View, request.Predicates) == specnumeric.Match {
			view := e.admittedViewLocked(active.View)
			if request.Locale != "" {
				// ForLocale resolves labels in these slices. Keep the active
				// index immutable while concurrent readers hold the gate.
				view.CategoryTrail = slices.Clone(view.CategoryTrail)
				view.Applications = slices.Clone(view.Applications)
				view = view.ForLocale(request.Locale)
			}
			matches = append(matches, view)
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].PartNumber != matches[j].PartNumber {
			return matches[i].PartNumber < matches[j].PartNumber
		}
		return matches[i].ID < matches[j].ID
	})
	result.Total = len(matches)
	// Divide before multiplying so arbitrarily large pages cannot overflow.
	if len(matches) == 0 || request.Page-1 > (len(matches)-1)/request.PageSize {
		return result, nil
	}
	start := (request.Page - 1) * request.PageSize
	end := start + min(request.PageSize, len(matches)-start)
	result.Rows = matches[start:end]
	return result, nil
}

func (e *Engine) completeSpecInventory(snapshot SpecCandidateSnapshot) bool {
	if len(e.index.specInventory) != len(e.index.byID) {
		return false
	}
	return specInventoryMatches(snapshot, e.index.specInventory)
}

func specBindingMatches(binding SpecCandidateBinding, active ActivePublication) bool {
	return binding.ProjectionVersion == SpecProjectionVersion && binding.ProductID == active.ProductID && binding.PublicRevision == active.PublicRevision && binding.SourceRevision == active.SourceRevision && binding.SiteEpoch == active.SiteEpoch && binding.VisibilityGeneration == active.VisibilityGeneration
}

func specSearchScope(view PublicView, request SpecSearchRequest) bool {
	if request.ManufacturerID != "" && request.ManufacturerID != view.ManufacturerID {
		return false
	}
	if request.CategoryID == "" || request.CategoryID == view.CategoryID {
		return true
	}
	for _, category := range view.CategoryTrail {
		if category.ID == request.CategoryID {
			return true
		}
	}
	return false
}

func compatibleSpecDefinitions(a, b PublicNumericDefinition) bool {
	if a.SpecSemanticVersion != b.SpecSemanticVersion {
		return false
	}
	normalized := func(profile specnumeric.Profile) specnumeric.Profile {
		profile.AllowedShapes = append([]specnumeric.Shape(nil), profile.AllowedShapes...)
		profile.AllowedOperators = append([]specnumeric.Operator(nil), profile.AllowedOperators...)
		slices.Sort(profile.AllowedShapes)
		slices.Sort(profile.AllowedOperators)
		profile.AllowedShapes = slices.Compact(profile.AllowedShapes)
		profile.AllowedOperators = slices.Compact(profile.AllowedOperators)
		return profile
	}
	return reflect.DeepEqual(normalized(a.Profile), normalized(b.Profile))
}

func specViewTruth(view PublicView, predicates []specnumeric.Predicate) specnumeric.Truth {
	truths := make([]specnumeric.Truth, 0, len(predicates))
	for _, predicate := range predicates {
		truth := specnumeric.Unknown
		for _, definition := range view.NumericDefinitions {
			if definition.SpecID != predicate.SpecID {
				continue
			}
			for _, fact := range view.NumericFacts {
				if fact.SpecID == predicate.SpecID && compatibleSpecDefinitions(definition, PublicNumericDefinition{SpecID: fact.SpecID, SpecSemanticVersion: fact.SpecSemanticVersion, Profile: fact.Profile}) {
					truth = specnumeric.Evaluate(&fact.Value, fact.Profile, predicate)
					break
				}
			}
			break
		}
		truths = append(truths, truth)
	}
	return specnumeric.All(truths)
}

// Structural validation still applies when every scoped definition is absent.
// Absence establishes UNKNOWN; it cannot supply quantity or role semantics.
func validateSpecPredicate(predicate specnumeric.Predicate) error {
	invalid := func(message string) error { return fmt.Errorf("%w: %s", ErrInvalidSpecSearch, message) }
	if strings.TrimSpace(predicate.SpecID) == "" || strings.TrimSpace(predicate.Unit) == "" {
		return invalid("predicate requires spec ID and explicit unit")
	}
	rangeQuery := false
	switch predicate.Operator {
	case specnumeric.OperatorBetween, specnumeric.OperatorContainsRange, specnumeric.OperatorOverlapsRange:
		rangeQuery = true
	case specnumeric.OperatorContainsPoint, specnumeric.OperatorEqual, specnumeric.OperatorLess, specnumeric.OperatorLessEqual, specnumeric.OperatorGreater, specnumeric.OperatorGreaterEqual:
	default:
		return fmt.Errorf("%w: %w: unknown operator", ErrInvalidSpecSearch, ErrUnsupportedSpecSearch)
	}
	if rangeQuery {
		if predicate.Value != nil || predicate.Lower == nil || predicate.Upper == nil {
			return invalid("range predicate requires only lower and upper")
		}
	} else if predicate.Value == nil || predicate.Lower != nil || predicate.Upper != nil {
		return invalid("point predicate requires only value")
	}
	for _, operand := range []*specnumeric.Decimal{predicate.Value, predicate.Lower, predicate.Upper} {
		if operand != nil {
			if _, err := specnumeric.ParseDecimal(operand.String()); err != nil {
				return invalid("invalid decimal operand")
			}
		}
	}
	if rangeQuery && predicate.Lower.Cmp(*predicate.Upper) > 0 {
		return invalid("reversed predicate range")
	}
	return nil
}

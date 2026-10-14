package publishing

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strings"

	"prods/internal/catalog"
	"prods/internal/catalogfilter"
	"prods/internal/localization"
	"prods/internal/site"
	"prods/internal/specnumeric"
)

type CatalogQuery struct {
	ScopeURL string
	Scope    catalogfilter.Scope
	Locale   string
	Clauses  []catalogfilter.Clause
}
type CatalogFilterOption struct {
	Value string
	Label string
}
type CatalogSnapshot struct {
	Scope        catalogfilter.Scope
	AllViews     []PublicView
	Presentation PublicView
	Views        []PublicView
	ScopeViews   []PublicView
	Definitions  []PublicFilterDefinition
	Options      map[string][]CatalogFilterOption
	Site         site.Configuration
	Locale       string
	Generation   int64
}

var ErrCatalogScopeNotFound = errors.New("public catalog scope not found")

type CatalogFilterError struct{ Field, Message string }

func (e *CatalogFilterError) Error() string { return e.Field + ": " + e.Message }

func coreFilterDefinitions() []PublicFilterDefinition {
	result := []PublicFilterDefinition{}
	for _, field := range []string{"manufacturer", "brand", "category", "application", "lifecycle", "package"} {
		p := catalogfilter.DefaultTextProfile()
		if field != "package" {
			p.Kind = "choice"
			p.Operators = []string{"equals"}
		}
		result = append(result, PublicFilterDefinition{Field: field, SemanticVersion: 1, ProfileRevision: 1, Profile: p})
	}
	return result
}

// Candidate providers are read outside the public gate. Every fact, option,
// count and definition is then resolved against one admitted immutable index.
func (e *Engine) AdmitCatalog(ctx context.Context, request CatalogQuery) (CatalogSnapshot, error) {
	var ids []string
	var predicates []specnumeric.Predicate
	for _, c := range request.Clauses {
		if c.Numeric != nil {
			ids = append(ids, c.Numeric.SpecID)
			predicates = append(predicates, *c.Numeric)
		}
	}
	var candidates SpecCandidateSnapshot
	var projected bool
	var candidateErr error
	if len(ids) > 0 {
		candidates, projected, candidateErr = e.readSpecCandidates(ctx, SpecSearchRequest{Predicates: predicates})
	}
	e.gate.RLock()
	defer e.gate.RUnlock()
	if e.index == nil || e.index.unhealthy != nil {
		return CatalogSnapshot{}, ErrPublicUnavailable
	}
	if err := ctx.Err(); err != nil {
		return CatalogSnapshot{}, err
	}
	snapshot := CatalogSnapshot{Site: e.index.site, Generation: e.index.siteEpoch, Options: map[string][]CatalogFilterOption{}, Views: []PublicView{}, ScopeViews: []PublicView{}}
	presentation := PublicView{Site: e.index.site, SiteEpoch: e.index.siteEpoch, Language: e.index.localization.DefaultLocale, DefaultLocale: e.index.localization.DefaultLocale, SupportedLocales: slices.Clone(e.index.localization.EnabledLocales), PublicCopyDefaults: map[string]map[string]string{}, PublicCopyOverrides: e.index.localization.PublicCopyOverrides}
	if len(e.index.views) > 0 {
		presentation = e.admittedViewLocked(e.index.views[0])
		presentation.Site = e.index.site
		presentation.SiteEpoch = e.index.siteEpoch
	}
	if presentation.DefaultLocale == "" {
		presentation.DefaultLocale = "en-US"
	}
	if presentation.Language == "" {
		presentation.Language = presentation.DefaultLocale
	}
	if len(presentation.SupportedLocales) == 0 {
		presentation.SupportedLocales = []string{presentation.DefaultLocale}
	}
	if len(presentation.PublicCopyDefaults) == 0 {
		presentation.PublicCopyDefaults = map[string]map[string]string{}
		for _, item := range localization.OfficialPublicCopyCatalog().Defaults {
			if presentation.PublicCopyDefaults[item.Key] == nil {
				presentation.PublicCopyDefaults[item.Key] = map[string]string{}
			}
			presentation.PublicCopyDefaults[item.Key][item.Locale] = item.Value
		}
	}
	snapshot.Presentation = presentation
	locale := request.Locale
	if locale == "" {
		locale = presentation.DefaultLocale
	}
	if !slices.Contains(presentation.SupportedLocales, locale) {
		return snapshot, localization.ErrUnsupportedLocale
	}
	if request.ScopeURL != "" && request.Scope.ID == "" {
		for _, v := range e.index.views {
			switch request.Scope.Kind {
			case "manufacturer":
				if v.ManufacturerURL == request.ScopeURL {
					request.Scope.ID = v.ManufacturerID
				}
			case "brand":
				if v.BrandURL == request.ScopeURL {
					request.Scope.ID = v.BrandID
				}
			case "application":
				for _, a := range v.Applications {
					if a.URL == request.ScopeURL {
						request.Scope.ID = a.ID
					}
				}
			}
		}
		if request.Scope.ID == "" {
			return snapshot, ErrCatalogScopeNotFound
		}
	}
	snapshot.Scope = request.Scope
	if len(e.index.views) > 0 {
		view := e.index.views[0]
		if locale == "" {
			locale = view.DefaultLocale
			if locale == "" {
				locale = view.Language
			}
		}
		if !slices.Contains(publicSupportedLocales(view), locale) {
			return snapshot, localization.ErrUnsupportedLocale
		}
	}
	if locale == "" {
		locale = "en-US"
	}
	snapshot.Locale = locale
	switch request.Scope.Kind {
	case "all":
		if request.Scope.ID != "" {
			return snapshot, &CatalogFilterError{Message: "invalid scope"}
		}
	case "search":
		if strings.TrimSpace(request.Scope.Query) == "" {
			return snapshot, &CatalogFilterError{Message: "empty search scope"}
		}
	case "category", "manufacturer", "brand", "application":
		if request.Scope.ID == "" {
			return snapshot, &CatalogFilterError{Message: "missing scope ID"}
		}
	default:
		return snapshot, &CatalogFilterError{Message: "unknown scope"}
	}
	keywordRanks := map[string]int{}
	for _, view := range e.index.views {
		view = e.admittedViewLocked(view)
		view.Applications = slices.Clone(view.Applications)
		view.CategoryTrail = slices.Clone(view.CategoryTrail)
		// Match the admitted source projection before replacing display labels.
		matchesScope := catalogScopeMatches(view, request.Scope)
		rank := catalogKeywordRank(view, request.Scope.Query)
		keywordRanks[view.ID] = rank
		view = view.ForLocale(locale)
		snapshot.AllViews = append(snapshot.AllViews, view)
		if matchesScope && (request.Scope.Kind == "search" || strings.TrimSpace(request.Scope.Query) == "" || rank >= 0) {
			snapshot.ScopeViews = append(snapshot.ScopeViews, view)
		}
	}
	sort.SliceStable(snapshot.ScopeViews, func(i, j int) bool {
		a, b := snapshot.ScopeViews[i], snapshot.ScopeViews[j]
		if request.Scope.Kind == "search" {
			ar, br := keywordRanks[a.ID], keywordRanks[b.ID]
			if ar != br {
				return ar < br
			}
		}
		ap, bp := catalog.FoldSearch(a.PartNumber), catalog.FoldSearch(b.PartNumber)
		if ap != bp {
			return ap < bp
		}
		return a.ID < b.ID
	})
	snapshot.Definitions = append(coreFilterDefinitions(), CommonFilterDefinitions(snapshot.ScopeViews)...)
	byField := map[string]PublicFilterDefinition{}
	for _, d := range snapshot.Definitions {
		byField[d.Field] = d
		if d.Profile.Kind == "numeric" {
			continue
		}
		options := map[string]string{}
		for _, view := range snapshot.ScopeViews {
			for _, option := range catalogFieldOptions(view, d) {
				if option.Value != "" {
					if _, exists := options[option.Value]; !exists {
						options[option.Value] = option.Label
					}
				}
			}
		}
		for value, label := range options {
			snapshot.Options[d.Field] = append(snapshot.Options[d.Field], CatalogFilterOption{Value: value, Label: label})
		}
		sort.Slice(snapshot.Options[d.Field], func(i, j int) bool {
			a, b := snapshot.Options[d.Field][i], snapshot.Options[d.Field][j]
			if a.Label != b.Label {
				return a.Label < b.Label
			}
			return a.Value < b.Value
		})
	}
	seen := map[string]bool{}
	for _, clause := range request.Clauses {
		d, ok := byField[clause.Field]
		invalid := func(message string) (CatalogSnapshot, error) {
			return snapshot, &CatalogFilterError{Field: clause.Field, Message: message}
		}
		if !ok {
			return invalid("field is not applicable with one published meaning in this scope")
		}
		if seen[clause.Field] {
			return invalid("duplicate condition")
		}
		seen[clause.Field] = true
		if !slices.Contains(d.Profile.Operators, clause.Operator) {
			return invalid("unsupported operator")
		}
		if d.Profile.Kind == "numeric" {
			if clause.Numeric == nil || len(clause.Values) != 0 || clause.Numeric.SpecID != strings.TrimPrefix(clause.Field, "spec:") || string(clause.Numeric.Operator) != clause.Operator {
				return invalid("numeric condition requires matching operands")
			}
			if err := validateSpecPredicate(*clause.Numeric); err != nil {
				return invalid(err.Error())
			}
			if _, err := specnumeric.NormalizePredicate(*clause.Numeric, *d.Profile.Numeric); err != nil {
				return invalid(err.Error())
			}
		} else {
			if clause.Numeric != nil || len(clause.Values) == 0 || len(clause.Values) > 20 {
				return invalid("condition requires one to twenty values")
			}
			for _, value := range clause.Values {
				if len(value) > 512 || catalogfilter.FoldText(value) == "" {
					return invalid("invalid text value")
				}
			}
		}
	}
	var candidateIDs map[string]bool
	if len(ids) > 0 {
		candidateIDs, projected = e.admitSpecCandidatesLocked(candidates, projected, candidateErr)
	}
	for _, view := range snapshot.ScopeViews {
		if projected && !candidateIDs[view.ID] {
			continue
		}
		match := true
		for _, c := range request.Clauses {
			if c.Numeric != nil {
				if specViewTruth(view, []specnumeric.Predicate{*c.Numeric}) != specnumeric.Match {
					match = false
					break
				}
			} else if !catalogTextMatches(view, byField[c.Field], c) {
				match = false
				break
			}
		}
		if match {
			snapshot.Views = append(snapshot.Views, view)
		}
	}
	return snapshot, nil
}

func catalogScopeMatches(v PublicView, s catalogfilter.Scope) bool {
	switch s.Kind {
	case "all":
		return true
	case "search":
		return catalogKeywordRank(v, s.Query) >= 0
	case "manufacturer":
		return v.ManufacturerID == s.ID
	case "brand":
		return v.BrandID == s.ID
	case "category":
		if v.CategoryID == s.ID {
			return true
		}
		for _, c := range v.CategoryTrail {
			if c.ID == s.ID {
				return true
			}
		}
	case "application":
		for _, a := range v.Applications {
			if a.ID == s.ID {
				return true
			}
		}
	}
	return false
}
func catalogKeywordRank(v PublicView, q string) int {
	q = catalog.FoldSearch(q)
	part := catalog.FoldSearch(v.PartNumber)
	if part == q {
		return 0
	}
	if strings.HasPrefix(part, q) {
		return 1
	}
	if strings.Contains(part, q) {
		return 2
	}
	for _, value := range []string{v.Name, v.Manufacturer, v.Brand} {
		if strings.Contains(catalog.FoldSearch(value), q) {
			return 3
		}
	}
	// Keep the existing keyword field boundary and enabled localized names.
	all := []string{v.PartNumber, v.Name, v.Manufacturer, v.Brand, v.Category, v.Lifecycle, applicationNames(v.Applications)}
	for _, localized := range v.Localizations {
		all = append(all, localized.Name)
	}
	if strings.Contains(catalog.FoldSearch(strings.Join(all, " ")), q) {
		return 4
	}
	return -1
}
func catalogFieldOptions(v PublicView, d PublicFilterDefinition) []CatalogFilterOption {
	option := func(value, label string) []CatalogFilterOption {
		return []CatalogFilterOption{{Value: value, Label: label}}
	}
	switch d.Field {
	case "manufacturer":
		return option(v.ManufacturerID, v.Manufacturer)
	case "brand":
		return option(v.BrandID, v.Brand)
	case "category":
		r := option(v.CategoryID, v.Category)
		for _, c := range v.CategoryTrail {
			r = append(r, CatalogFilterOption{Value: c.ID, Label: c.Name})
		}
		return r
	case "application":
		r := []CatalogFilterOption{}
		for _, a := range v.Applications {
			r = append(r, CatalogFilterOption{Value: a.ID, Label: a.Name})
		}
		return r
	case "lifecycle":
		return option(v.LifecycleID, v.Lifecycle)
	case "package":
		return option(v.PackageFormFactor, v.PackageFormFactor)
	}
	id := strings.TrimPrefix(d.Field, "spec:")
	for _, s := range v.Specifications {
		if s.ID == id {
			values := []string{s.RawValue}
			if d.Profile.Kind == "multi_choice" {
				values = strings.Split(s.RawValue, d.Profile.Delimiter)
			}
			r := []CatalogFilterOption{}
			for _, value := range values {
				value = strings.TrimSpace(value)
				if value != "" {
					r = append(r, CatalogFilterOption{Value: value, Label: value})
				}
			}
			return r
		}
	}
	return nil
}
func catalogTextMatches(v PublicView, d PublicFilterDefinition, c catalogfilter.Clause) bool {
	for _, actual := range catalogFieldOptions(v, d) {
		for _, value := range c.Values {
			if d.Profile.Kind == "choice" && !strings.HasPrefix(d.Field, "spec:") {
				if value == actual.Value {
					return true
				}
				continue
			}
			a, b := catalogfilter.FoldText(actual.Value), catalogfilter.FoldText(value)
			if (c.Operator == "equals" && a == b) || (c.Operator == "contains" && strings.Contains(a, b)) {
				return true
			}
		}
	}
	return false
}

func (e *Engine) readSpecCandidates(ctx context.Context, request SpecSearchRequest) (SpecCandidateSnapshot, bool, error) {
	switch e.specBackend {
	case "bleve":
		if e.specBleve != nil {
			snapshot, err := e.specBleve.candidates(ctx, request)
			return snapshot, err == nil, err
		}
	case "sqlite":
		if r, ok := e.repository.(SpecSearchRepository); ok {
			ids := make([]string, 0, len(request.Predicates))
			for _, p := range request.Predicates {
				ids = append(ids, p.SpecID)
			}
			snapshot, err := r.SpecSearchCandidates(ctx, ids, SpecProjectionVersion)
			return snapshot, err == nil, err
		}
	}
	return SpecCandidateSnapshot{}, false, errors.New("spec search candidate provider unavailable")
}
func (e *Engine) admitSpecCandidatesLocked(snapshot SpecCandidateSnapshot, projected bool, candidateErr error) (map[string]bool, bool) {
	candidates := map[string]bool{}
	if projected && e.index.specDigestError == nil && e.completeSpecInventory(snapshot) && (e.specBackend != "bleve" || snapshot.SourceDigest == e.index.specDigest) {
		for _, candidate := range snapshot.Candidates {
			current, exists := e.index.byID[candidate.ProductID]
			if !exists || !specBindingMatches(candidate, current) {
				projected = false
				break
			}
			candidates[candidate.ProductID] = true
		}
	} else {
		projected = false
	}
	if projected {
		e.specCandidateQueries.Add(1)
		e.specStatus.Store(&specSearchStatus{digest: e.index.specDigest})
	} else {
		e.specFallbackQueries.Add(1)
		if candidateErr == nil {
			candidateErr = errors.New("spec search candidate snapshot does not match admitted public inventory")
		}
		if e.index.specDigestError != nil {
			candidateErr = e.index.specDigestError
		}
		e.specStatus.Store(&specSearchStatus{digest: snapshot.SourceDigest, lastError: candidateErr.Error()})
	}
	return candidates, projected
}

// AdmitCatalogViews retains the explicitly enabled PoC fallback with the same oracle.
func AdmitCatalogViews(ctx context.Context, views []PublicView, configuration site.Configuration, query CatalogQuery) (CatalogSnapshot, error) {
	e := &Engine{index: &publicIndex{views: views, byID: map[string]ActivePublication{}, site: configuration}}
	for _, v := range views {
		e.index.byID[v.ID] = ActivePublication{ProductID: v.ID, View: v}
	}
	return e.AdmitCatalog(ctx, query)
}

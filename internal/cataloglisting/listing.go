package cataloglisting

import (
	"errors"
	"slices"
	"sort"
	"strings"

	"prods/internal/catalog"
	"prods/internal/publishing"
	"prods/internal/site"
)

type ScopeKind string

const (
	ScopeAll          ScopeKind = "all"
	ScopeCategory     ScopeKind = "category"
	ScopeSearch       ScopeKind = "search"
	ScopeManufacturer ScopeKind = "manufacturer"
	ScopeBrand        ScopeKind = "brand"
	ScopeApplication  ScopeKind = "application"
)

var ErrInvalidRequest = errors.New("invalid catalog listing request")

type Scope struct {
	Kind  ScopeKind `json:"kind"`
	ID    string    `json:"id,omitempty"`
	Query string    `json:"query,omitempty"`
}

type Request struct {
	Scope         Scope  `json:"scope"`
	Page          int    `json:"page"`
	PageSize      int    `json:"page_size"`
	Sort          string `json:"sort,omitempty"`
	SortDirection string `json:"sort_direction,omitempty"`
	Locale        string `json:"locale,omitempty"`
}

type Column struct {
	Key           string `json:"key"`
	Label         string `json:"label"`
	Kind          string `json:"kind"`
	PreferredUnit string `json:"preferred_unit,omitempty"`
	Sortable      bool   `json:"sortable"`
	MobileKey     bool   `json:"mobile_key"`
	MobileVisible bool   `json:"mobile_visible"`
}

type Row struct {
	ProductID  string                `json:"product_id"`
	Revision   int64                 `json:"revision"`
	ProductURL string                `json:"product_url"`
	RFQURL     string                `json:"rfq_url"`
	Values     map[string]string     `json:"values"`
	Documents  []publishing.Document `json:"documents,omitempty"`
}

type Result struct {
	Scope     Scope    `json:"scope"`
	Columns   []Column `json:"columns"`
	Rows      []Row    `json:"rows"`
	Total     int      `json:"total"`
	Page      int      `json:"page"`
	PageSize  int      `json:"page_size"`
	PageCount int      `json:"page_count"`
	Sort      string   `json:"sort"`
	Direction string   `json:"sort_direction"`
}

type CategoryNode struct {
	ID            string              `json:"id"`
	ParentID      string              `json:"parent_id,omitempty"`
	Name          string              `json:"name"`
	Slug          string              `json:"slug"`
	Status        catalog.EntryStatus `json:"status"`
	DirectPublic  int                 `json:"direct_public"`
	SubtreePublic int                 `json:"subtree_public"`
}

func Resolve(views []publishing.PublicView, configuration site.Configuration, request Request) (Result, error) {
	if request.Page < 1 {
		request.Page = 1
	}
	if request.PageSize < 1 {
		request.PageSize = 20
	}
	if request.PageSize > 100 {
		request.PageSize = 100
	}
	if !validScope(request.Scope) {
		return Result{}, ErrInvalidRequest
	}
	scopeViews := filterScope(views, request.Scope)
	localized := make([]publishing.PublicView, len(scopeViews))
	for index := range scopeViews {
		locale := request.Locale
		if locale == "" {
			locale = scopeViews[index].Language
		}
		localized[index] = scopeViews[index].ForLocale(locale)
	}
	profile, hasProfile := configuration.CategoryListingProfiles[request.Scope.ID]
	common := commonApplicable(localized)
	columns := resolveColumns(request.Scope, profile, hasProfile, common)
	sortKey, direction, err := resolveSort(request, profile, hasProfile)
	if err != nil {
		return Result{}, err
	}
	if request.Scope.Kind != ScopeSearch || request.Sort != "" {
		sortViews(localized, sortKey, direction)
	}
	total := len(localized)
	pageCount := 0
	if total > 0 {
		pageCount = (total + request.PageSize - 1) / request.PageSize
	}
	start := (request.Page - 1) * request.PageSize
	if start > total {
		start = total
	}
	end := min(start+request.PageSize, total)
	rows := make([]Row, 0, end-start)
	for _, view := range localized[start:end] {
		rows = append(rows, rowFromView(view, columns))
	}
	return Result{Scope: request.Scope, Columns: columns, Rows: rows, Total: total, Page: request.Page, PageSize: request.PageSize, PageCount: pageCount, Sort: sortKey, Direction: direction}, nil
}

func BuildCategoryNavigation(categories []catalog.Category, views []publishing.PublicView) []CategoryNode {
	direct := make(map[string]map[string]struct{})
	subtree := make(map[string]map[string]struct{})
	for _, view := range views {
		addProduct(direct, view.CategoryID, view.ID)
		addProduct(subtree, view.CategoryID, view.ID)
		for _, category := range view.CategoryTrail {
			addProduct(subtree, category.ID, view.ID)
		}
	}
	nodes := make([]CategoryNode, 0, len(categories))
	for _, category := range categories {
		nodes = append(nodes, CategoryNode{
			ID: category.ID, ParentID: category.ParentID, Name: category.Name, Slug: category.Slug, Status: category.Status,
			DirectPublic: len(direct[category.ID]), SubtreePublic: len(subtree[category.ID]),
		})
	}
	sort.SliceStable(nodes, func(left, right int) bool {
		if nodes[left].ParentID != nodes[right].ParentID {
			return nodes[left].ParentID < nodes[right].ParentID
		}
		if nodes[left].Name != nodes[right].Name {
			return nodes[left].Name < nodes[right].Name
		}
		return nodes[left].ID < nodes[right].ID
	})
	return nodes
}

func validScope(scope Scope) bool {
	switch scope.Kind {
	case ScopeAll:
		return scope.ID == ""
	case ScopeSearch:
		return strings.TrimSpace(scope.Query) != ""
	case ScopeCategory, ScopeManufacturer, ScopeBrand, ScopeApplication:
		return strings.TrimSpace(scope.ID) != ""
	default:
		return false
	}
}

func filterScope(views []publishing.PublicView, scope Scope) []publishing.PublicView {
	if scope.Kind == ScopeAll || scope.Kind == ScopeSearch {
		return append([]publishing.PublicView(nil), views...)
	}
	result := make([]publishing.PublicView, 0, len(views))
	for _, view := range views {
		matches := false
		switch scope.Kind {
		case ScopeCategory:
			matches = view.CategoryID == scope.ID
			for _, category := range view.CategoryTrail {
				matches = matches || category.ID == scope.ID
			}
		case ScopeManufacturer:
			matches = view.ManufacturerID == scope.ID
		case ScopeBrand:
			matches = view.BrandID == scope.ID
		case ScopeApplication:
			for _, application := range view.Applications {
				matches = matches || application.ID == scope.ID
			}
		}
		if matches {
			result = append(result, view)
		}
	}
	return result
}

func commonApplicable(views []publishing.PublicView) map[string]publishing.ApplicableSpecification {
	common := make(map[string]publishing.ApplicableSpecification)
	if len(views) == 0 {
		return common
	}
	for _, spec := range views[0].ApplicableSpecifications {
		common[spec.ID] = spec
	}
	for _, view := range views[1:] {
		applicable := make(map[string]publishing.ApplicableSpecification, len(view.ApplicableSpecifications))
		for _, spec := range view.ApplicableSpecifications {
			applicable[spec.ID] = spec
		}
		for id := range common {
			if _, exists := applicable[id]; !exists {
				delete(common, id)
			}
		}
	}
	return common
}

func resolveColumns(scope Scope, profile site.CategoryListingProfile, hasProfile bool, common map[string]publishing.ApplicableSpecification) []Column {
	keys := []string{"part_number", "name", "manufacturer", "package", "lifecycle"}
	if scope.Kind != ScopeCategory {
		keys = append(keys, "category")
	}
	commonIDs := make([]string, 0, len(common))
	for id := range common {
		commonIDs = append(commonIDs, id)
	}
	slices.Sort(commonIDs)
	for _, id := range commonIDs[:min(4, len(commonIDs))] {
		keys = append(keys, "spec:"+id)
	}
	keys = append(keys, "documents", "rfq")
	if hasProfile && scope.Kind == ScopeCategory {
		keys = append([]string(nil), profile.VisibleColumns...)
	}
	mobile := make(map[string]bool)
	if hasProfile {
		for _, key := range profile.MobileKeySpecs {
			mobile[key] = true
		}
	}
	if len(mobile) == 0 {
		remaining := 4
		for _, key := range keys {
			if strings.HasPrefix(key, "spec:") && remaining > 0 {
				mobile[key] = true
				remaining--
			}
		}
	}
	columns := make([]Column, 0, len(keys))
	for _, key := range keys {
		if strings.HasPrefix(key, "spec:") {
			spec, exists := common[strings.TrimPrefix(key, "spec:")]
			if !exists {
				continue
			}
			columns = append(columns, Column{Key: key, Label: spec.Name, Kind: "spec", PreferredUnit: spec.PreferredUnit, MobileKey: mobile[key], MobileVisible: mobile[key]})
			continue
		}
		columns = append(columns, Column{
			Key: key, Label: baseLabel(key), Kind: "base", Sortable: sortableBase(key),
			MobileVisible: key == "part_number" || key == "name" || key == "manufacturer" || key == "brand" || key == "documents" || key == "rfq",
		})
	}
	return columns
}

func resolveSort(request Request, profile site.CategoryListingProfile, hasProfile bool) (string, string, error) {
	key := request.Sort
	direction := strings.ToLower(request.SortDirection)
	if key == "" && hasProfile && request.Scope.Kind == ScopeCategory {
		key = profile.DefaultSort
		direction = profile.DefaultSortDirection
	}
	if key == "" {
		key = "part_number"
	}
	if direction == "" {
		direction = "asc"
	}
	if !sortableBase(key) || (direction != "asc" && direction != "desc") {
		return "", "", ErrInvalidRequest
	}
	return key, direction, nil
}

func sortViews(views []publishing.PublicView, key, direction string) {
	sort.SliceStable(views, func(left, right int) bool {
		leftValue := catalog.FoldSearch(baseValue(views[left], key))
		rightValue := catalog.FoldSearch(baseValue(views[right], key))
		if leftValue == rightValue {
			if direction == "desc" {
				return views[left].ID > views[right].ID
			}
			return views[left].ID < views[right].ID
		}
		if direction == "desc" {
			return leftValue > rightValue
		}
		return leftValue < rightValue
	})
}

func rowFromView(view publishing.PublicView, columns []Column) Row {
	values := make(map[string]string, len(columns))
	specValues := make(map[string]string, len(view.Specifications))
	for _, spec := range view.Specifications {
		specValues[spec.ID] = spec.RawValue
	}
	for _, column := range columns {
		if strings.HasPrefix(column.Key, "spec:") {
			values[column.Key] = specValues[strings.TrimPrefix(column.Key, "spec:")]
		} else {
			values[column.Key] = baseValue(view, column.Key)
		}
	}
	if values["name"] == "" {
		values["name"] = view.PartNumber
	}
	return Row{ProductID: view.ID, Revision: view.Revision, ProductURL: view.CanonicalURL, RFQURL: view.RFQURL, Values: values, Documents: append([]publishing.Document(nil), view.Documents...)}
}

func baseValue(view publishing.PublicView, key string) string {
	switch key {
	case "part_number":
		return view.PartNumber
	case "name":
		return view.Name
	case "manufacturer":
		return view.Manufacturer
	case "brand":
		return view.Brand
	case "category":
		return view.Category
	case "package":
		return view.PackageFormFactor
	case "lifecycle":
		return view.Lifecycle
	default:
		return ""
	}
}

func baseLabel(key string) string {
	switch key {
	case "part_number":
		return "Part number"
	case "name":
		return "Product name"
	case "manufacturer":
		return "Manufacturer"
	case "brand":
		return "Brand"
	case "category":
		return "Category"
	case "package":
		return "Package"
	case "lifecycle":
		return "Lifecycle"
	case "documents":
		return "Documents"
	case "rfq":
		return "RFQ"
	default:
		return key
	}
}

func sortableBase(key string) bool {
	switch key {
	case "part_number", "name", "manufacturer", "brand", "lifecycle":
		return true
	default:
		return false
	}
}

func addProduct(target map[string]map[string]struct{}, categoryID, productID string) {
	if categoryID == "" || productID == "" {
		return
	}
	if target[categoryID] == nil {
		target[categoryID] = make(map[string]struct{})
	}
	target[categoryID][productID] = struct{}{}
}

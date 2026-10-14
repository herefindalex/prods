package webapp

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"prods/internal/catalog"
	"prods/internal/catalogfilter"
	"prods/internal/cataloglisting"
	"prods/internal/publishing"
	"prods/internal/site"
)

const publicListingPageSize = 24

type catalogPage struct {
	FilterControls []catalogFilterControl
	FilterError    string
	FilterHidden   []catalogHiddenField
	publicPage
	Title               string
	Query               string
	Listing             cataloglisting.Result
	NoResults           bool
	RequestURL          string
	PreviousURL         string
	NextURL             string
	FormAction          string
	ClearURL            string
	ShowFilters         bool
	ManufacturerID      string
	BrandID             string
	CategoryID          string
	ManufacturerOptions []searchFilterOption
	BrandOptions        []searchFilterOption
	CategoryOptions     []searchFilterOption
	CategoryLinks       []publicCategoryLink
	Breadcrumbs         []publicCategoryLink
	ChildCategories     []publicCategoryLink
}

type publicCategoryLink struct {
	ID            string
	Name          string
	URL           string
	DirectPublic  int
	SubtreePublic int
}

func (s *Server) renderCatalogSearch(w http.ResponseWriter, r *http.Request) {
	s.renderFilteredCatalog(w, r, "")
}
func (s *Server) renderCatalogAggregate(w http.ResponseWriter, r *http.Request, kind string) {
	s.renderFilteredCatalog(w, r, kind)
}

func (s *Server) renderFilteredCatalog(w http.ResponseWriter, r *http.Request, kind string) {
	if len(r.URL.RawQuery) > 8192 {
		http.Error(w, "catalog query exceeds 8192 bytes", http.StatusBadRequest)
		return
	}
	values := r.URL.Query()
	query := values.Get("q")
	scope := catalogfilter.Scope{Kind: "all", Query: query}
	if strings.TrimSpace(query) != "" {
		scope.Kind = "search"
	}
	var categories []catalog.Category
	var err error
	title := ""
	scopeURL := ""
	if kind != "" {
		scope.Kind = kind
		if kind == "category" {
			categories, err = s.store.ListCategories(r.Context())
			if err != nil {
				s.internalError(w, err)
				return
			}
			urls := categoryURLs(categories)
			for _, category := range categories {
				if category.Status == catalog.EntryActive && urls[category.ID] == r.URL.Path {
					scope.ID = category.ID
					title = category.Name
					break
				}
			}
			if scope.ID == "" {
				http.NotFound(w, r)
				return
			}
		} else {
			scopeURL = strings.TrimRight(s.config.BaseURL, "/") + r.URL.Path
		}
	}
	clauses, filterErr := parseCatalogFilters(values)
	request := publishing.CatalogQuery{Scope: scope, ScopeURL: scopeURL, Locale: values.Get("lang"), Clauses: clauses}
	var snapshot publishing.CatalogSnapshot
	if s.publisher != nil {
		snapshot, err = s.publisher.AdmitCatalog(r.Context(), request)
	} else {
		var views []publishing.PublicView
		views, err = s.publicViews(r.Context())
		if err == nil {
			configuration := site.DefaultConfiguration()
			if len(views) > 0 {
				configuration = views[0].Site
			}
			snapshot, err = publishing.AdmitCatalogViews(r.Context(), views, configuration, request)
		}
	}
	var conditionErr *publishing.CatalogFilterError
	if errors.As(err, &conditionErr) {
		filterErr = err
	} else if errors.Is(err, publishing.ErrCatalogScopeNotFound) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		s.writePublicPageError(w, err)
		return
	}
	if filterErr == nil {
		filterErr = validateCatalogFilterFields(snapshot, values)
	}
	presentation, err := s.currentPublicPage(r, []publishing.PublicView{snapshot.Presentation})
	if err != nil {
		s.writePublicPageError(w, err)
		return
	}
	if kind == "" {
		title = presentation.Text.Catalog
	} else if title == "" && len(snapshot.ScopeViews) > 0 {
		v := snapshot.ScopeViews[0]
		switch kind {
		case "manufacturer":
			title = v.Manufacturer
		case "brand":
			title = v.Brand
		case "application":
			for _, a := range v.Applications {
				if a.ID == snapshot.Scope.ID {
					title = a.Name
				}
			}
		}
	}
	listingScope := cataloglisting.Scope{Kind: cataloglisting.ScopeKind(snapshot.Scope.Kind), ID: snapshot.Scope.ID, Query: snapshot.Scope.Query}
	if filterErr != nil {
		snapshot.Views = nil
	}
	listing, err := cataloglisting.ResolveSnapshot(snapshot, catalogListingRequest(r, listingScope, presentation.Language))
	if err != nil {
		http.Error(w, "invalid catalog listing request", http.StatusBadRequest)
		return
	}
	preparePublicListing(&listing, presentation)
	data := catalogPage{publicPage: presentation, Title: title, Query: query, Listing: listing, NoResults: filterErr == nil && strings.TrimSpace(query) != "" && listing.Total == 0, RequestURL: withLanguage("/rfq?query="+url.QueryEscape(query), presentation.Language), FormAction: r.URL.Path, ShowFilters: true, FilterControls: catalogFilterForm(snapshot, values), FilterHidden: catalogFilterHiddenFields(values), ClearURL: withLanguage(r.URL.Path, presentation.Language)}
	if kind == "" {
		if categories == nil {
			categories, err = s.store.ListCategories(r.Context())
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				s.internalError(w, err)
				return
			}
		}
		data.CategoryLinks, _, _, _ = publicCategoryNavigation(categories, snapshot.AllViews, "")
	}
	if kind == "category" {
		_, data.Breadcrumbs, data.ChildCategories, _ = publicCategoryNavigation(categories, snapshot.AllViews, scope.ID)
	}
	setCatalogPagination(r, &data)
	w.Header().Set("Content-Language", presentation.Language)
	w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
	if filterErr != nil {
		data.FilterError = catalogFilterErrorLabel(filterErr, presentation.Text)
		w.WriteHeader(http.StatusUnprocessableEntity)
	} else {
		material, _ := json.Marshal(struct {
			Query      string
			Snapshot   publishing.CatalogSnapshot
			Listing    cataloglisting.Result
			Categories []catalog.Category
		}{r.URL.RawQuery, snapshot, listing, categories})
		sum := sha256.Sum256(material)
		etag := `"c-` + hex.EncodeToString(sum[:])[:32] + `"`
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	s.render(w, "catalog-listing", data)
}

var errPublicAggregateNotFound = errors.New("public aggregate not found")

func (s *Server) resolvePublicAggregate(r *http.Request, kind, targetURL string, views []publishing.PublicView) (cataloglisting.Scope, string, []catalog.Category, error) {
	if kind == "category" {
		categories, err := s.store.ListCategories(r.Context())
		if err != nil {
			return cataloglisting.Scope{}, "", nil, err
		}
		byURL := categoryURLs(categories)
		for _, category := range categories {
			if category.Status == catalog.EntryActive && byURL[category.ID] == r.URL.Path {
				return cataloglisting.Scope{Kind: cataloglisting.ScopeCategory, ID: category.ID}, category.Name, categories, nil
			}
		}
		return cataloglisting.Scope{}, "", categories, errPublicAggregateNotFound
	}
	for _, view := range views {
		switch kind {
		case "manufacturer":
			if view.ManufacturerURL == targetURL {
				return cataloglisting.Scope{Kind: cataloglisting.ScopeManufacturer, ID: view.ManufacturerID}, view.Manufacturer, nil, nil
			}
		case "brand":
			if view.BrandURL == targetURL {
				return cataloglisting.Scope{Kind: cataloglisting.ScopeBrand, ID: view.BrandID}, view.Brand, nil, nil
			}
		case "application":
			for _, application := range view.Applications {
				if application.URL == targetURL {
					return cataloglisting.Scope{Kind: cataloglisting.ScopeApplication, ID: application.ID}, application.Name, nil, nil
				}
			}
		default:
			return cataloglisting.Scope{}, "", nil, fmt.Errorf("unknown public aggregate kind %q", kind)
		}
	}
	return cataloglisting.Scope{}, "", nil, errPublicAggregateNotFound
}

func catalogListingRequest(r *http.Request, scope cataloglisting.Scope, locale string) cataloglisting.Request {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize := publicListingPageSize
	if requested, err := strconv.Atoi(r.URL.Query().Get("page_size")); err == nil && requested >= 1 && requested <= 100 {
		pageSize = requested
	}
	return cataloglisting.Request{
		Scope: scope, Page: page, PageSize: pageSize,
		Sort: strings.TrimSpace(r.URL.Query().Get("sort")), SortDirection: strings.TrimSpace(r.URL.Query().Get("direction")), Locale: locale,
	}
}

func preparePublicListing(listing *cataloglisting.Result, presentation publicPage) {
	for index := range listing.Columns {
		switch listing.Columns[index].Key {
		case "part_number":
			listing.Columns[index].Label = presentation.Text.PartNumber
		case "name":
			listing.Columns[index].Label = presentation.Text.Name
		case "manufacturer":
			listing.Columns[index].Label = presentation.Text.Manufacturer
		case "brand":
			listing.Columns[index].Label = presentation.Text.Brand
		case "category":
			listing.Columns[index].Label = presentation.Text.Category
		case "lifecycle":
			listing.Columns[index].Label = presentation.Text.Lifecycle
		case "documents":
			listing.Columns[index].Label = presentation.Text.Documents
		case "rfq":
			listing.Columns[index].Label = presentation.Text.RequestQuote
		}
	}
	for index := range listing.Rows {
		listing.Rows[index].ProductURL = withLanguage(listing.Rows[index].ProductURL, presentation.Language)
		listing.Rows[index].RFQURL = withLanguage(listing.Rows[index].RFQURL, presentation.Language)
	}
}

func fallbackPublicSearch(views []publishing.PublicView, query string) []publishing.PublicView {
	folded := catalog.FoldSearch(query)
	filtered := make([]publishing.PublicView, 0, len(views))
	for _, view := range views {
		haystack := catalog.FoldSearch(strings.Join([]string{
			view.PartNumber, view.Name, view.Manufacturer, view.Brand, view.Category, view.Lifecycle, publicApplicationNames(view.Applications),
		}, " "))
		if strings.Contains(haystack, folded) {
			filtered = append(filtered, view)
		}
	}
	return filtered
}

func setCatalogPagination(r *http.Request, page *catalogPage) {
	if page.Listing.Page > 1 {
		page.PreviousURL = listingPageURL(r, page.Listing.Page-1)
	}
	if page.Listing.Page < page.Listing.PageCount {
		page.NextURL = listingPageURL(r, page.Listing.Page+1)
	}
}

func listingPageURL(r *http.Request, page int) string {
	query := r.URL.Query()
	query.Set("page", strconv.Itoa(page))
	return r.URL.Path + "?" + query.Encode()
}

func aggregateListingETag(target string, listing cataloglisting.Result, categories []catalog.Category) string {
	hasher := sha256.New()
	_, _ = fmt.Fprintf(hasher, "%s\x00%s\x00%s", target, listing.Sort, listing.Direction)
	for _, row := range listing.Rows {
		_, _ = fmt.Fprintf(hasher, "\x00%s\x00%d", row.ProductID, row.Revision)
	}
	for _, category := range categories {
		_, _ = fmt.Fprintf(hasher, "\x00%s\x00%d\x00%s", category.ID, category.Revision, category.Status)
	}
	return `"g-` + hex.EncodeToString(hasher.Sum(nil))[:32] + `"`
}

func categoryURLs(categories []catalog.Category) map[string]string {
	byID := make(map[string]catalog.Category, len(categories))
	for _, category := range categories {
		byID[category.ID] = category
	}
	result := make(map[string]string, len(categories))
	var resolve func(string, map[string]bool) (string, bool)
	resolve = func(id string, seen map[string]bool) (string, bool) {
		if path, ok := result[id]; ok {
			return path, true
		}
		category, ok := byID[id]
		if !ok || seen[id] {
			return "", false
		}
		seen[id] = true
		if category.SystemKey == "root" {
			result[id] = ""
			return "", true
		}
		parentPath, ok := resolve(category.ParentID, seen)
		if !ok {
			return "", false
		}
		path := "/categories/" + category.Slug
		if parentPath != "" {
			path = strings.TrimRight(parentPath, "/") + "/" + category.Slug
		}
		result[id] = path
		return path, true
	}
	for _, category := range categories {
		_, _ = resolve(category.ID, make(map[string]bool))
	}
	return result
}

func publicCategoryNavigation(categories []catalog.Category, views []publishing.PublicView, currentID string) (top, breadcrumbs, children []publicCategoryLink, found bool) {
	counts := cataloglisting.BuildCategoryNavigation(categories, views)
	countByID := make(map[string]cataloglisting.CategoryNode, len(counts))
	categoryByID := make(map[string]catalog.Category, len(categories))
	rootID := ""
	for _, node := range counts {
		countByID[node.ID] = node
	}
	for _, category := range categories {
		categoryByID[category.ID] = category
		if category.SystemKey == "root" {
			rootID = category.ID
		}
	}
	urls := categoryURLs(categories)
	linkFor := func(category catalog.Category) publicCategoryLink {
		count := countByID[category.ID]
		return publicCategoryLink{ID: category.ID, Name: category.Name, URL: urls[category.ID], DirectPublic: count.DirectPublic, SubtreePublic: count.SubtreePublic}
	}
	for _, category := range categories {
		if category.Status != catalog.EntryActive || category.SystemKey == "root" {
			continue
		}
		if category.ParentID == rootID {
			top = append(top, linkFor(category))
		}
		if currentID != "" && category.ParentID == currentID {
			children = append(children, linkFor(category))
		}
	}
	if currentID != "" {
		current, ok := categoryByID[currentID]
		found = ok
		seen := make(map[string]bool)
		for ok && current.SystemKey != "root" && !seen[current.ID] {
			seen[current.ID] = true
			breadcrumbs = append(breadcrumbs, linkFor(current))
			current, ok = categoryByID[current.ParentID]
		}
		for left, right := 0, len(breadcrumbs)-1; left < right; left, right = left+1, right-1 {
			breadcrumbs[left], breadcrumbs[right] = breadcrumbs[right], breadcrumbs[left]
		}
	}
	sort.SliceStable(top, func(i, j int) bool { return top[i].Name < top[j].Name })
	sort.SliceStable(children, func(i, j int) bool { return children[i].Name < children[j].Name })
	return top, breadcrumbs, children, found
}

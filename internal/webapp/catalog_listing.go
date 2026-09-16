package webapp

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"prods/internal/catalog"
	"prods/internal/cataloglisting"
	"prods/internal/publishing"
)

const publicListingPageSize = 24

type catalogPage struct {
	publicPage
	Title               string
	Query               string
	Listing             cataloglisting.Result
	NoResults           bool
	RequestURL          string
	PreviousURL         string
	NextURL             string
	FormAction          string
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
	query := r.URL.Query().Get("q")
	filters := publicSearchFilters{
		ManufacturerID: strings.TrimSpace(r.URL.Query().Get("manufacturer_id")),
		BrandID:        strings.TrimSpace(r.URL.Query().Get("brand_id")),
		CategoryID:     strings.TrimSpace(r.URL.Query().Get("category_id")),
	}
	allViews, err := s.publicViews(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	views := allViews
	if strings.TrimSpace(query) != "" && s.publisher != nil {
		views, err = s.publisher.Search(r.Context(), query)
	}
	if err != nil {
		s.internalError(w, err)
		return
	}
	if strings.TrimSpace(query) != "" && s.publisher == nil {
		views = fallbackPublicSearch(views, query)
	}
	views = filterPublicViews(views, filters)
	presentation, err := s.currentPublicPage(r, views)
	if err != nil {
		s.writePublicPageError(w, err)
		return
	}
	scope := cataloglisting.Scope{Kind: cataloglisting.ScopeAll}
	if strings.TrimSpace(query) != "" {
		scope = cataloglisting.Scope{Kind: cataloglisting.ScopeSearch, Query: query}
	}
	listing, err := cataloglisting.Resolve(views, presentation.Site, catalogListingRequest(r, scope, presentation.Language))
	if err != nil {
		http.Error(w, "invalid catalog listing request", http.StatusBadRequest)
		return
	}
	preparePublicListing(&listing, presentation)
	manufacturerOptions, brandOptions, categoryOptions := publicSearchFilterOptions(allViews, filters)
	categories, categoryErr := s.store.ListCategories(r.Context())
	if categoryErr != nil && !errors.Is(categoryErr, sql.ErrNoRows) {
		s.internalError(w, categoryErr)
		return
	}
	categoryLinks, _, _, _ := publicCategoryNavigation(categories, allViews, "")
	data := catalogPage{
		publicPage: presentation, Title: presentation.Text.Catalog, Query: query, Listing: listing,
		NoResults:  strings.TrimSpace(query) != "" && listing.Total == 0,
		RequestURL: withLanguage("/rfq?query="+url.QueryEscape(query), presentation.Language),
		FormAction: r.URL.Path, ShowFilters: true, ManufacturerID: filters.ManufacturerID, BrandID: filters.BrandID, CategoryID: filters.CategoryID,
		ManufacturerOptions: manufacturerOptions, BrandOptions: brandOptions, CategoryOptions: categoryOptions,
		CategoryLinks: categoryLinks,
	}
	setCatalogPagination(r, &data)
	w.Header().Set("Content-Language", presentation.Language)
	s.render(w, "catalog-listing", data)
}

func (s *Server) renderCatalogAggregate(w http.ResponseWriter, r *http.Request, kind string) {
	allViews, err := s.publicViews(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	targetURL := strings.TrimRight(s.config.BaseURL, "/") + r.URL.Path
	scope, title, categories, err := s.resolvePublicAggregate(r, kind, targetURL, allViews)
	if err != nil {
		if errors.Is(err, errPublicAggregateNotFound) {
			http.NotFound(w, r)
			return
		}
		s.internalError(w, err)
		return
	}
	presentation, err := s.currentPublicPage(r, allViews)
	if err != nil {
		s.writePublicPageError(w, err)
		return
	}
	listing, err := cataloglisting.Resolve(allViews, presentation.Site, catalogListingRequest(r, scope, presentation.Language))
	if err != nil {
		http.Error(w, "invalid catalog listing request", http.StatusBadRequest)
		return
	}
	preparePublicListing(&listing, presentation)
	etag := aggregateListingETag(targetURL, listing, categories)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	data := catalogPage{publicPage: presentation, Title: title, Listing: listing, FormAction: r.URL.Path}
	if kind == "category" {
		_, data.Breadcrumbs, data.ChildCategories, _ = publicCategoryNavigation(categories, allViews, scope.ID)
	}
	setCatalogPagination(r, &data)
	w.Header().Set("Content-Language", presentation.Language)
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

package webapp

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/publicsuffix"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/site"
)

const (
	brandImportSchemaVersion = "prods.website_brand_import.v1"
	brandImportPromptVersion = "website-brand-prompt.v1"
	brandImportRequestTTL    = 2 * time.Hour
)

//go:embed brand_import_schema.json
var brandImportSchema string

type pendingBrandImport struct {
	UserID       string
	SourceURL    string
	SourceLocale string
	ExpiresAt    time.Time
	Validated    *validatedBrandImport
	Used         bool
}

type validatedBrandImport struct {
	Envelope        brandImportEnvelope
	Proposed        site.Configuration
	Diff            []brandImportFieldChange
	WorkingRevision int64
}

type brandImportFieldChange struct {
	Field  string `json:"field"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

type brandImportRequestInfo struct {
	RequestID     string    `json:"request_id"`
	SourceURL     string    `json:"source_url"`
	SourceLocale  string    `json:"source_locale"`
	SchemaVersion string    `json:"schema_version"`
	PromptVersion string    `json:"prompt_version"`
	Prompt        string    `json:"prompt"`
	ExpiresAt     time.Time `json:"expires_at"`
}

type brandImportValidationResponse struct {
	RequestID             string                   `json:"request_id"`
	WorkingRevision       int64                    `json:"working_revision"`
	Status                string                   `json:"status"`
	Limitations           []string                 `json:"limitations,omitempty"`
	ProposedConfiguration *site.Configuration      `json:"proposed_configuration,omitempty"`
	Diff                  []brandImportFieldChange `json:"diff"`
}

type brandImportEnvelope struct {
	SchemaVersion string                  `json:"schema_version"`
	Request       brandImportRequestEcho  `json:"request"`
	Capture       brandImportCapture      `json:"capture"`
	Observations  brandImportObservations `json:"observations"`
	Proposal      *brandImportProposal    `json:"proposal"`
	Evidence      []brandImportEvidence   `json:"evidence"`
}

type brandImportRequestEcho struct {
	RequestID     string `json:"request_id"`
	PromptVersion string `json:"prompt_version"`
	RequestedURL  string `json:"requested_url"`
}

type brandImportCapture struct {
	Status       string   `json:"status"`
	ObservedURL  string   `json:"observed_url"`
	ObservedAt   string   `json:"observed_at"`
	SourceLocale string   `json:"source_locale"`
	PageTitle    string   `json:"page_title,omitempty"`
	Limitations  []string `json:"limitations,omitempty"`
}

type brandImportObservations struct {
	Header      brandImportHeaderObservation `json:"header"`
	Footer      brandImportFooterObservation `json:"footer"`
	Unsupported []string                     `json:"unsupported_source_features"`
}

type brandImportHeaderObservation struct {
	RowTypes        []string `json:"row_types"`
	Controls        []string `json:"controls"`
	NavigationDepth int      `json:"navigation_depth"`
	HasMegaMenu     bool     `json:"has_mega_menu"`
}

type brandImportFooterObservation struct {
	SectionHeadings []string `json:"section_headings"`
	Elements        []string `json:"elements"`
}

type brandImportEvidence struct {
	FieldPath   string `json:"field_path"`
	SourceURL   string `json:"source_url"`
	Observation string `json:"observation"`
	Confidence  string `json:"confidence"`
}

type brandImportProposal struct {
	Organization brandImportOrganization `json:"organization"`
	Header       brandImportHeader       `json:"header"`
	Footer       brandImportFooter       `json:"footer"`
	Theme        brandImportTheme        `json:"theme"`
}

type brandImportOrganization struct {
	DisplayName       string               `json:"display_name"`
	LegalName         string               `json:"legal_name,omitempty"`
	OfficialWebsite   string               `json:"official_website"`
	Description       string               `json:"description,omitempty"`
	Contacts          []brandImportContact `json:"contacts"`
	SocialLinks       []brandImportSocial  `json:"social_links"`
	RegistrationLines []string             `json:"registration_lines,omitempty"`
}

type brandImportContact struct {
	Key, Type, Label, Value, URL string
	Order                        int
}

func (value *brandImportContact) UnmarshalJSON(data []byte) error {
	type wire struct {
		Key   string `json:"key"`
		Type  string `json:"type"`
		Label string `json:"label"`
		Value string `json:"value"`
		URL   string `json:"url,omitempty"`
		Order int    `json:"order"`
	}
	var decoded wire
	if err := strictJSON(data, &decoded); err != nil {
		return err
	}
	*value = brandImportContact(decoded)
	return nil
}

type brandImportSocial struct {
	Key, Network, Label, URL string
	Order                    int
}

func (value *brandImportSocial) UnmarshalJSON(data []byte) error {
	type wire struct {
		Key     string `json:"key"`
		Network string `json:"network"`
		Label   string `json:"label"`
		URL     string `json:"url"`
		Order   int    `json:"order"`
	}
	var decoded wire
	if err := strictJSON(data, &decoded); err != nil {
		return err
	}
	*value = brandImportSocial(decoded)
	return nil
}

type brandImportHeader struct {
	Layout            string                  `json:"layout"`
	Sticky            bool                    `json:"sticky"`
	BrandPresentation string                  `json:"brand_presentation"`
	Rows              []brandImportHeaderRow  `json:"rows"`
	Navigation        []brandImportNavigation `json:"navigation"`
}

type brandImportHeaderRow struct {
	Key, Type string
	Order     int
	Items     []brandImportHeaderItem
}

func (value *brandImportHeaderRow) UnmarshalJSON(data []byte) error {
	type wire struct {
		Key   string                  `json:"key"`
		Type  string                  `json:"type"`
		Order int                     `json:"order"`
		Items []brandImportHeaderItem `json:"items"`
	}
	var decoded wire
	if err := strictJSON(data, &decoded); err != nil {
		return err
	}
	*value = brandImportHeaderRow(decoded)
	return nil
}

type brandImportHeaderItem struct {
	Key, Kind, Label, URL, Action, Text string
	Order                               int
}

func (value *brandImportHeaderItem) UnmarshalJSON(data []byte) error {
	type wire struct {
		Key    string `json:"key"`
		Kind   string `json:"kind"`
		Label  string `json:"label,omitempty"`
		URL    string `json:"url,omitempty"`
		Action string `json:"action,omitempty"`
		Text   string `json:"text,omitempty"`
		Order  int    `json:"order"`
	}
	var decoded wire
	if err := strictJSON(data, &decoded); err != nil {
		return err
	}
	*value = brandImportHeaderItem(decoded)
	return nil
}

type brandImportNavigation struct {
	Key          string
	ParentKey    *string
	Label        string
	Order        int
	Target       brandImportNavigationTarget
	Presentation string
}

func (value *brandImportNavigation) UnmarshalJSON(data []byte) error {
	type wire struct {
		Key          string                      `json:"key"`
		ParentKey    *string                     `json:"parent_key"`
		Label        string                      `json:"label"`
		Order        int                         `json:"order"`
		Target       brandImportNavigationTarget `json:"target"`
		Presentation string                      `json:"presentation"`
	}
	var decoded wire
	if err := strictJSON(data, &decoded); err != nil {
		return err
	}
	*value = brandImportNavigation(decoded)
	return nil
}

type brandImportNavigationTarget struct {
	Type, URL, Action string
	OpenNewWindow     bool
}

func (value *brandImportNavigationTarget) UnmarshalJSON(data []byte) error {
	type wire struct {
		Type          string `json:"type"`
		URL           string `json:"url,omitempty"`
		Action        string `json:"action,omitempty"`
		OpenNewWindow bool   `json:"open_new_window,omitempty"`
	}
	var decoded wire
	if err := strictJSON(data, &decoded); err != nil {
		return err
	}
	*value = brandImportNavigationTarget(decoded)
	return nil
}

type brandImportFooter struct {
	Layout            string                      `json:"layout"`
	BrandBlock        brandImportFooterBrandBlock `json:"brand_block"`
	Sections          []brandImportFooterSection  `json:"sections"`
	ShowSocialLinks   bool                        `json:"show_social_links"`
	LegalLinks        []brandImportLinkItem       `json:"legal_links"`
	CopyrightText     string                      `json:"copyright_text"`
	RegistrationLines []string                    `json:"registration_lines"`
	Disclaimer        string                      `json:"disclaimer,omitempty"`
	LocaleControl     brandImportLocaleControl    `json:"locale_control"`
}

type brandImportFooterBrandBlock struct {
	ShowBrand       bool     `json:"show_brand"`
	ShowDescription bool     `json:"show_description"`
	ContactKeys     []string `json:"contact_keys"`
}
type brandImportLocaleControl struct {
	Enabled   bool   `json:"enabled"`
	Placement string `json:"placement"`
}
type brandImportFooterSection struct {
	Key, Heading string
	Order        int
	Collapsible  bool
	Items        []brandImportFooterItem
}

func (value *brandImportFooterSection) UnmarshalJSON(data []byte) error {
	type wire struct {
		Key         string                  `json:"key"`
		Heading     string                  `json:"heading"`
		Order       int                     `json:"order"`
		Collapsible bool                    `json:"collapsible_on_mobile"`
		Items       []brandImportFooterItem `json:"items"`
	}
	var decoded wire
	if err := strictJSON(data, &decoded); err != nil {
		return err
	}
	*value = brandImportFooterSection(decoded)
	return nil
}

type brandImportFooterItem struct {
	Key, Kind, Label, URL, Text, ContactKey, Action string
	Order                                           int
}

func (value *brandImportFooterItem) UnmarshalJSON(data []byte) error {
	type wire struct {
		Key        string `json:"key"`
		Kind       string `json:"kind"`
		Label      string `json:"label,omitempty"`
		URL        string `json:"url,omitempty"`
		Text       string `json:"text,omitempty"`
		ContactKey string `json:"contact_key,omitempty"`
		Action     string `json:"action,omitempty"`
		Order      int    `json:"order"`
	}
	var decoded wire
	if err := strictJSON(data, &decoded); err != nil {
		return err
	}
	*value = brandImportFooterItem(decoded)
	return nil
}

type brandImportLinkItem struct {
	Key, Kind, Label, URL string
	Order                 int
}

func (value *brandImportLinkItem) UnmarshalJSON(data []byte) error {
	type wire struct {
		Key   string `json:"key"`
		Kind  string `json:"kind"`
		Label string `json:"label"`
		URL   string `json:"url"`
		Order int    `json:"order"`
	}
	var decoded wire
	if err := strictJSON(data, &decoded); err != nil {
		return err
	}
	*value = brandImportLinkItem(decoded)
	return nil
}

type brandImportTheme struct {
	PrimaryColor      string `json:"primary_color"`
	AccentColor       string `json:"accent_color"`
	BodyTextColor     string `json:"body_text_color"`
	BorderColor       string `json:"border_color"`
	HeaderBackground  string `json:"header_background"`
	HeaderTextColor   string `json:"header_text_color"`
	FooterBackground  string `json:"footer_background"`
	FooterTextColor   string `json:"footer_text_color"`
	TypographyProfile string `json:"typography_profile"`
	Density           string `json:"density"`
	Radius            string `json:"radius"`
	ContentWidthPX    int    `json:"content_width_px"`
}

func (s *Server) adminCreateBrandImportRequest(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, true)
	if !ok {
		return
	}
	var request struct {
		SourceURL string `json:"source_url"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	sourceURL, err := normalizeBrandURL(request.SourceURL)
	if err != nil {
		s.writeAPIError(w, r, http.StatusUnprocessableEntity, apiCodeValidationFailed)
		return
	}
	state, err := s.store.WebsiteState(r.Context())
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	requestID, err := secureToken(24)
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	now := time.Now().UTC()
	pending := &pendingBrandImport{UserID: current.UserID, SourceURL: sourceURL, SourceLocale: state.WorkingLocalization.DefaultLocale, ExpiresAt: now.Add(brandImportRequestTTL)}
	s.brandImportMu.Lock()
	s.pruneBrandImportsLocked(now)
	s.brandImports[requestID] = pending
	s.brandImportMu.Unlock()
	writeJSON(w, http.StatusCreated, brandImportRequestInfo{RequestID: requestID, SourceURL: sourceURL, SourceLocale: pending.SourceLocale, SchemaVersion: brandImportSchemaVersion, PromptVersion: brandImportPromptVersion, Prompt: brandImportPrompt(requestID, sourceURL, pending.SourceLocale), ExpiresAt: pending.ExpiresAt})
}

func (s *Server) adminValidateBrandImport(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, true)
	if !ok {
		return
	}
	var request struct {
		RequestID string `json:"request_id"`
		Payload   string `json:"payload"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	pending, ok := s.brandImportRequest(request.RequestID, current.UserID)
	if !ok {
		s.writeAPIError(w, r, http.StatusUnprocessableEntity, apiCodeValidationFailed)
		return
	}
	if containsForbiddenBrandPayload(request.Payload) {
		s.writeAPIError(w, r, http.StatusUnprocessableEntity, apiCodeValidationFailed)
		return
	}
	var envelope brandImportEnvelope
	if err := strictJSON([]byte(request.Payload), &envelope); err != nil {
		s.writeAPIError(w, r, http.StatusUnprocessableEntity, apiCodeValidationFailed)
		return
	}
	if err := validateBrandImportEnvelope(envelope, request.RequestID, pending); err != nil {
		s.writeAPIError(w, r, http.StatusUnprocessableEntity, apiCodeValidationFailed)
		return
	}
	state, err := s.store.WebsiteState(r.Context())
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	response := brandImportValidationResponse{RequestID: request.RequestID, WorkingRevision: state.WorkingRevision, Status: envelope.Capture.Status, Limitations: envelope.Capture.Limitations, Diff: []brandImportFieldChange{}}
	if envelope.Capture.Status == "unavailable" {
		s.setValidatedBrandImport(request.RequestID, current.UserID, nil)
		writeJSON(w, http.StatusOK, response)
		return
	}
	proposed, err := buildBrandImportConfiguration(state.Working, envelope, request.RequestID)
	if err != nil {
		s.writeAPIError(w, r, http.StatusUnprocessableEntity, apiCodeValidationFailed)
		return
	}
	diff := brandImportDiff(state.Working, proposed)
	validated := &validatedBrandImport{Envelope: envelope, Proposed: proposed, Diff: diff, WorkingRevision: state.WorkingRevision}
	if !s.setValidatedBrandImport(request.RequestID, current.UserID, validated) {
		s.writeAPIError(w, r, http.StatusUnprocessableEntity, apiCodeValidationFailed)
		return
	}
	response.ProposedConfiguration = &proposed
	response.Diff = diff
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) adminApplyBrandImport(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, true)
	if !ok {
		return
	}
	var request struct {
		RequestID        string `json:"request_id"`
		ExpectedRevision int64  `json:"expected_revision"`
		RightsConfirmed  bool   `json:"rights_confirmed"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	if !request.RightsConfirmed {
		s.writeAPIError(w, r, http.StatusUnprocessableEntity, apiCodeValidationFailed)
		return
	}
	pending, ok := s.brandImportRequest(request.RequestID, current.UserID)
	if !ok || pending.Validated == nil || pending.Used {
		s.writeAPIError(w, r, http.StatusUnprocessableEntity, apiCodeValidationFailed)
		return
	}
	if request.ExpectedRevision != pending.Validated.WorkingRevision {
		s.writeAPIError(w, r, http.StatusConflict, apiCodeRevisionConflict)
		return
	}
	state, err := s.store.SaveWebsiteWorking(r.Context(), current.UserID, request.ExpectedRevision, pending.Validated.Proposed)
	if err != nil {
		if errors.Is(err, catalog.ErrRevisionConflict) {
			s.writeAPIError(w, r, http.StatusConflict, apiCodeRevisionConflict)
			return
		}
		s.internalAPIError(w, r, err)
		return
	}
	s.markBrandImportUsed(request.RequestID, current.UserID)
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) brandImportRequest(requestID, userID string) (pendingBrandImport, bool) {
	now := time.Now().UTC()
	s.brandImportMu.Lock()
	defer s.brandImportMu.Unlock()
	s.pruneBrandImportsLocked(now)
	pending, ok := s.brandImports[requestID]
	if !ok || pending.UserID != userID || pending.Used {
		return pendingBrandImport{}, false
	}
	return *pending, true
}

func (s *Server) setValidatedBrandImport(requestID, userID string, validated *validatedBrandImport) bool {
	now := time.Now().UTC()
	s.brandImportMu.Lock()
	defer s.brandImportMu.Unlock()
	s.pruneBrandImportsLocked(now)
	pending, ok := s.brandImports[requestID]
	if !ok || pending.UserID != userID || pending.Used {
		return false
	}
	pending.Validated = validated
	return true
}

func (s *Server) markBrandImportUsed(requestID, userID string) {
	s.brandImportMu.Lock()
	defer s.brandImportMu.Unlock()
	if pending, ok := s.brandImports[requestID]; ok && pending.UserID == userID {
		pending.Used = true
	}
}

func (s *Server) pruneBrandImportsLocked(now time.Time) {
	for id, pending := range s.brandImports {
		if !pending.ExpiresAt.After(now) || pending.Used {
			delete(s.brandImports, id)
		}
	}
}

func brandImportPrompt(requestID, sourceURL, sourceLocale string) string {
	return fmt.Sprintf(`You are preparing a structured Website Brand proposal for Prods.

REQUEST_ID: %s
CUSTOMER_SPECIFIED_OFFICIAL_URL: %s
SITE_DEFAULT_LOCALE: %s
SCHEMA_VERSION: %s
PROMPT_VERSION: %s

Open the exact customer-specified URL with browsing enabled. Follow only same-registrable-domain links reached from that site. Never replace it with another company, search result, archive, reseller, social profile, or guessed URL. If the site is unavailable or redirects across registrable domains, return status unavailable, proposal null, and no evidence; do not infer missing content.

Inspect the rendered desktop and mobile Header and Footer. Produce a Prods proposal rather than page code. Never output HTML, CSS, JavaScript, images, SVG, media, font files, data URLs, base64, cookies, tracking code, or third-party widgets. Account, cart, newsletter, and unsupported widgets belong only in observations. Supported system actions are catalog_search, catalog, rfq, and locale. Navigation is a flat hierarchy with key and parent_key, with at most five levels. Put primary navigation links only in header.navigation; do not repeat the same source link or action in header.rows and header.navigation. Keep imported text in the observed source locale. Every proposed group needs evidence tied to the exact source URL.

Return exactly one JSON object with no Markdown fence or surrounding commentary. It must validate against this JSON Schema:

%s`, requestID, sourceURL, sourceLocale, brandImportSchemaVersion, brandImportPromptVersion, brandImportSchema)
}

func validateBrandImportEnvelope(envelope brandImportEnvelope, requestID string, pending pendingBrandImport) error {
	if envelope.SchemaVersion != brandImportSchemaVersion || envelope.Request.RequestID != requestID || envelope.Request.PromptVersion != brandImportPromptVersion {
		return errors.New("brand import version or request mismatch")
	}
	requestedURL, err := normalizeBrandURL(envelope.Request.RequestedURL)
	if err != nil || requestedURL != pending.SourceURL {
		return errors.New("brand import source URL mismatch")
	}
	observedURL, err := normalizeBrandURL(envelope.Capture.ObservedURL)
	if err != nil || !sameRegistrableDomain(pending.SourceURL, observedURL) {
		return errors.New("brand import observed URL is outside the requested site")
	}
	if envelope.Capture.SourceLocale == "" || len(envelope.Capture.SourceLocale) > 35 {
		return errors.New("brand import source locale is invalid")
	}
	if _, err := time.Parse(time.RFC3339, envelope.Capture.ObservedAt); err != nil {
		return errors.New("brand import observed time is invalid")
	}
	if envelope.Capture.Status == "unavailable" {
		if envelope.Proposal != nil || len(envelope.Evidence) != 0 {
			return errors.New("unavailable brand import must not include a proposal")
		}
		return nil
	}
	if envelope.Capture.Status != "complete" && envelope.Capture.Status != "partial" {
		return errors.New("brand import status is invalid")
	}
	if envelope.Proposal == nil || len(envelope.Evidence) == 0 {
		return errors.New("brand import proposal and evidence are required")
	}
	for _, evidence := range envelope.Evidence {
		normalized, err := normalizeBrandURL(evidence.SourceURL)
		if err != nil || !sameRegistrableDomain(pending.SourceURL, normalized) || !strings.HasPrefix(evidence.FieldPath, "proposal.") || !oneOfString(evidence.Confidence, "high", "medium", "low") {
			return errors.New("brand import evidence is invalid")
		}
	}
	return nil
}

func buildBrandImportConfiguration(current site.Configuration, envelope brandImportEnvelope, requestID string) (site.Configuration, error) {
	proposal := envelope.Proposal
	next := current
	next.Organization.DisplayName = proposal.Organization.DisplayName
	next.Organization.LegalName = proposal.Organization.LegalName
	next.Organization.OfficialWebsite = proposal.Organization.OfficialWebsite
	next.Organization.Description = proposal.Organization.Description
	next.Organization.RegistrationLines = append([]string(nil), proposal.Organization.RegistrationLines...)
	contactIDs := make(map[string]string, len(proposal.Organization.Contacts))
	next.Organization.Contacts = make([]site.ContactMethod, 0, len(proposal.Organization.Contacts))
	for _, contact := range proposal.Organization.Contacts {
		id := importOpaqueID(requestID, "contact", contact.Key)
		contactIDs[contact.Key] = id
		next.Organization.Contacts = append(next.Organization.Contacts, site.ContactMethod{ID: id, Type: contact.Type, Label: contact.Label, Value: contact.Value, URL: contact.URL, SortOrder: contact.Order})
	}
	next.Organization.SocialLinks = make([]site.SocialLink, 0, len(proposal.Organization.SocialLinks))
	for _, link := range proposal.Organization.SocialLinks {
		next.Organization.SocialLinks = append(next.Organization.SocialLinks, site.SocialLink{ID: importOpaqueID(requestID, "social", link.Key), Network: link.Network, Label: link.Label, URL: link.URL, SortOrder: link.Order})
	}
	next.Header = site.Header{Layout: proposal.Header.Layout, Sticky: proposal.Header.Sticky, BrandPresentation: proposal.Header.BrandPresentation}
	for _, row := range proposal.Header.Rows {
		mapped := site.HeaderRow{ID: importOpaqueID(requestID, "header-row", row.Key), Type: row.Type, SortOrder: row.Order}
		for _, item := range row.Items {
			mapped.Items = append(mapped.Items, site.HeaderItem{ID: importOpaqueID(requestID, "header-item", row.Key+":"+item.Key), Kind: item.Kind, Label: item.Label, URL: item.URL, SystemAction: item.Action, Text: item.Text, SortOrder: item.Order})
		}
		next.Header.Rows = append(next.Header.Rows, mapped)
	}
	navigationIDs := make(map[string]string, len(proposal.Header.Navigation))
	for _, item := range proposal.Header.Navigation {
		navigationIDs[item.Key] = importOpaqueID(requestID, "navigation", item.Key)
	}
	next.Navigation = make([]site.NavigationItem, 0, len(proposal.Header.Navigation))
	for _, item := range proposal.Header.Navigation {
		parentID := ""
		if item.ParentKey != nil {
			var exists bool
			parentID, exists = navigationIDs[*item.ParentKey]
			if !exists {
				return site.Configuration{}, fmt.Errorf("navigation parent %q does not exist", *item.ParentKey)
			}
		}
		next.Navigation = append(next.Navigation, site.NavigationItem{ID: navigationIDs[item.Key], ParentID: parentID, Label: item.Label, URL: item.Target.URL, TargetType: item.Target.Type, SystemAction: item.Target.Action, Presentation: item.Presentation, OpenNewWindow: item.Target.OpenNewWindow, SortOrder: item.Order, Visible: true})
	}
	next.Footer = site.Footer{Layout: proposal.Footer.Layout, BrandBlock: site.FooterBrandBlock{ShowBrand: proposal.Footer.BrandBlock.ShowBrand, ShowDescription: proposal.Footer.BrandBlock.ShowDescription}, ShowSocialLinks: proposal.Footer.ShowSocialLinks, CopyrightText: proposal.Footer.CopyrightText, RegistrationLines: append([]string(nil), proposal.Footer.RegistrationLines...), Disclaimer: proposal.Footer.Disclaimer, LocaleControl: site.FooterLocaleControl{Enabled: proposal.Footer.LocaleControl.Enabled, Placement: proposal.Footer.LocaleControl.Placement}}
	for _, key := range proposal.Footer.BrandBlock.ContactKeys {
		next.Footer.BrandBlock.ContactIDs = append(next.Footer.BrandBlock.ContactIDs, contactIDs[key])
	}
	for _, section := range proposal.Footer.Sections {
		mapped := site.FooterSection{ID: importOpaqueID(requestID, "footer-section", section.Key), Heading: section.Heading, SortOrder: section.Order, CollapsibleOnMobile: section.Collapsible}
		for _, item := range section.Items {
			mapped.Items = append(mapped.Items, site.FooterItem{ID: importOpaqueID(requestID, "footer-item", section.Key+":"+item.Key), Kind: item.Kind, Label: item.Label, URL: item.URL, Text: item.Text, ContactID: contactIDs[item.ContactKey], SystemAction: item.Action, SortOrder: item.Order})
		}
		next.Footer.Sections = append(next.Footer.Sections, mapped)
	}
	for _, link := range proposal.Footer.LegalLinks {
		next.Footer.LegalLinks = append(next.Footer.LegalLinks, site.ContactLink{ID: importOpaqueID(requestID, "legal", link.Key), Label: link.Label, URL: link.URL, SortOrder: link.Order})
	}
	next.Theme.PrimaryColor = proposal.Theme.PrimaryColor
	next.Theme.SecondaryColor = proposal.Theme.AccentColor
	next.Theme.AccentColor = proposal.Theme.AccentColor
	next.Theme.BodyTextColor = proposal.Theme.BodyTextColor
	next.Theme.BorderColor = proposal.Theme.BorderColor
	next.Theme.HeaderBackground = proposal.Theme.HeaderBackground
	next.Theme.HeaderTextColor = proposal.Theme.HeaderTextColor
	next.Theme.FooterBackground = proposal.Theme.FooterBackground
	next.Theme.FooterTextColor = proposal.Theme.FooterTextColor
	next.Theme.TypographyProfile = proposal.Theme.TypographyProfile
	next.Theme.Density = proposal.Theme.Density
	next.Theme.Radius = proposal.Theme.Radius
	next.Theme.ContentWidthPX = proposal.Theme.ContentWidthPX
	next.Theme.FontFamily = typographyFontStack(proposal.Theme.TypographyProfile)
	next.BrandImport = &site.BrandImportProvenance{SourceURL: envelope.Request.RequestedURL, ObservedURL: envelope.Capture.ObservedURL, SourceLocale: envelope.Capture.SourceLocale, PromptVersion: envelope.Request.PromptVersion, ImportedAt: time.Now().UTC()}
	if err := next.Prepare(); err != nil {
		return site.Configuration{}, err
	}
	return next, nil
}

func brandImportDiff(before, after site.Configuration) []brandImportFieldChange {
	pairs := []struct {
		name          string
		before, after any
	}{{"organization", before.Organization, after.Organization}, {"header", before.Header, after.Header}, {"navigation", before.Navigation, after.Navigation}, {"footer", before.Footer, after.Footer}, {"theme", before.Theme, after.Theme}, {"brand_import", before.BrandImport, after.BrandImport}}
	changes := make([]brandImportFieldChange, 0, len(pairs))
	for _, pair := range pairs {
		left, _ := json.Marshal(pair.before)
		right, _ := json.Marshal(pair.after)
		if string(left) != string(right) {
			changes = append(changes, brandImportFieldChange{Field: pair.name, Before: pair.before, After: pair.after})
		}
	}
	return changes
}

func normalizeBrandURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.Hostname() == "" || parsed.Fragment != "" {
		return "", errors.New("invalid website URL")
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return "", errors.New("local website URL is not allowed")
	}
	if ip := net.ParseIP(host); ip != nil && (!ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback()) {
		return "", errors.New("private website URL is not allowed")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	return parsed.String(), nil
}
func sameRegistrableDomain(first, second string) bool {
	a, _ := url.Parse(first)
	b, _ := url.Parse(second)
	ah := strings.ToLower(a.Hostname())
	bh := strings.ToLower(b.Hostname())
	if ah == bh {
		return true
	}
	ad, ae := publicsuffix.EffectiveTLDPlusOne(ah)
	bd, be := publicsuffix.EffectiveTLDPlusOne(bh)
	return ae == nil && be == nil && ad == bd
}
func containsForbiddenBrandPayload(payload string) bool {
	lower := strings.ToLower(payload)
	for _, value := range []string{"<script", "<style", "javascript:", "data:", "base64,", "@import", "url("} {
		if strings.Contains(lower, value) {
			return true
		}
	}
	return false
}
func importOpaqueID(requestID, kind, key string) string {
	sum := sha256.Sum256([]byte(requestID + ":" + kind + ":" + key))
	return "bi_" + hex.EncodeToString(sum[:10])
}
func typographyFontStack(profile string) string {
	switch profile {
	case "geometric_sans":
		return "Avenir Next, Century Gothic, system-ui, sans-serif"
	case "humanist_sans":
		return "Segoe UI, Frutiger, system-ui, sans-serif"
	case "industrial_sans":
		return "Arial, Helvetica, system-ui, sans-serif"
	case "serif_accent":
		return "Georgia, Times New Roman, serif"
	default:
		return "system-ui, sans-serif"
	}
}
func strictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}
func oneOfString(value string, values ...string) bool {
	sort.Strings(values)
	index := sort.SearchStrings(values, value)
	return index < len(values) && values[index] == value
}

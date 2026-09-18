package webapp

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"prods/internal/site"
)

func TestBrandImportRequestRequiresAdminCSRF(t *testing.T) {
	server, client, _, _, _ := normalImportServer(t)
	response := postAdminJSON(t, client, server.URL+"/admin/api/website/brand-import/requests", "", `{"source_url":"https://example.com/"}`)
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", response.StatusCode, responseBody(t, response))
	}
}

func TestBrandImportValidatesThenAppliesOnlyToWorkingCopy(t *testing.T) {
	server, client, store, _, csrf := normalImportServer(t)

	state, err := store.WebsiteState(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	info := createBrandImportRequest(t, client, server.URL, csrf, "https://example.com/")
	if !strings.Contains(info.Prompt, info.RequestID) || !strings.Contains(info.Prompt, info.SourceURL) || !strings.Contains(info.Prompt, `"$schema"`) || !strings.Contains(info.Prompt, "do not repeat the same source link or action") {
		t.Fatalf("generated prompt is not bound to request/schema: %q", info.Prompt)
	}

	payload := validBrandImportPayload(t, info, "https://example.com/")
	validationResponse := postAdminJSON(t, client, server.URL+"/admin/api/website/brand-import/validate", csrf, marshalBrandImportPayload(t, info.RequestID, payload))
	if validationResponse.StatusCode != http.StatusOK {
		t.Fatalf("validate status=%d body=%s", validationResponse.StatusCode, responseBody(t, validationResponse))
	}
	var validation brandImportValidationResponse
	decodeResponseJSON(t, validationResponse, &validation)
	if validation.Status != "complete" || validation.WorkingRevision != state.WorkingRevision || validation.ProposedConfiguration == nil || len(validation.Diff) == 0 {
		t.Fatalf("unexpected validation response: %+v", validation)
	}

	afterValidation, err := store.WebsiteState(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if afterValidation.Working.Organization.DisplayName != state.Working.Organization.DisplayName || afterValidation.Active.Organization.DisplayName != state.Active.Organization.DisplayName {
		t.Fatal("validation mutated Website state")
	}

	withoutRights := postAdminJSON(t, client, server.URL+"/admin/api/website/brand-import/apply", csrf,
		`{"request_id":"`+info.RequestID+`","expected_revision":`+jsonNumber(t, state.WorkingRevision)+`,"rights_confirmed":false}`)
	if withoutRights.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("apply without rights status=%d body=%s", withoutRights.StatusCode, responseBody(t, withoutRights))
	}

	applied := postAdminJSON(t, client, server.URL+"/admin/api/website/brand-import/apply", csrf,
		`{"request_id":"`+info.RequestID+`","expected_revision":`+jsonNumber(t, state.WorkingRevision)+`,"rights_confirmed":true}`)
	if applied.StatusCode != http.StatusOK {
		t.Fatalf("apply status=%d body=%s", applied.StatusCode, responseBody(t, applied))
	}
	var appliedState site.State
	decodeResponseJSON(t, applied, &appliedState)
	if appliedState.Working.Organization.DisplayName != "Example Components" || appliedState.Working.BrandImport == nil {
		t.Fatalf("working copy not updated: %+v", appliedState.Working)
	}
	if appliedState.Active.Organization.DisplayName == "Example Components" {
		t.Fatal("brand import bypassed Website publish and changed Active")
	}

	replayed := postAdminJSON(t, client, server.URL+"/admin/api/website/brand-import/apply", csrf,
		`{"request_id":"`+info.RequestID+`","expected_revision":`+jsonNumber(t, appliedState.WorkingRevision)+`,"rights_confirmed":true}`)
	if replayed.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("used request status=%d body=%s", replayed.StatusCode, responseBody(t, replayed))
	}
}

func TestBuildBrandImportPreservesAssetsSEOListingsAndCustomCSS(t *testing.T) {
	info := brandImportRequestInfo{RequestID: "request_preserve", SourceURL: "https://example.com/", SchemaVersion: brandImportSchemaVersion, PromptVersion: brandImportPromptVersion}
	payload := validBrandImportPayload(t, info, info.SourceURL)
	var envelope brandImportEnvelope
	if err := strictJSON([]byte(mustJSON(t, payload)), &envelope); err != nil {
		t.Fatal(err)
	}
	current := site.DefaultConfiguration()
	current.Organization.PrimaryLogoAsset = "asset_existing_logo"
	current.Organization.DarkLogoAsset = "asset_existing_dark_logo"
	current.Organization.FaviconAsset = "asset_existing_favicon"
	current.Organization.SocialImageAsset = "asset_existing_social"
	current.SEO.DefaultTitle = "Existing SEO title"
	current.Theme.CustomCSS = ".existing{color:red}"
	current.Theme.CustomCSSEnabled = true
	current.CategoryListingProfiles = map[string]site.CategoryListingProfile{
		"category_1": {VisibleColumns: []string{"part_number"}},
	}

	proposed, err := buildBrandImportConfiguration(current, envelope, info.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if proposed.Organization.PrimaryLogoAsset != current.Organization.PrimaryLogoAsset ||
		proposed.Organization.DarkLogoAsset != current.Organization.DarkLogoAsset ||
		proposed.Organization.FaviconAsset != current.Organization.FaviconAsset ||
		proposed.Organization.SocialImageAsset != current.Organization.SocialImageAsset {
		t.Fatal("brand import changed protected asset IDs")
	}
	if proposed.SEO != current.SEO || proposed.Theme.CustomCSS != current.Theme.CustomCSS || proposed.Theme.CustomCSSEnabled != current.Theme.CustomCSSEnabled {
		t.Fatal("brand import changed SEO or custom CSS")
	}
	if len(proposed.CategoryListingProfiles) != 1 || len(proposed.CategoryListingProfiles["category_1"].VisibleColumns) != 1 {
		t.Fatal("brand import changed category listing profiles")
	}
}

func TestBrandImportRejectsMismatchedAndCrossDomainSources(t *testing.T) {
	server, client, _, _, csrf := normalImportServer(t)
	info := createBrandImportRequest(t, client, server.URL, csrf, "https://example.com/catalog")

	for name, mutate := range map[string]func(map[string]any){
		"requested URL mismatch": func(payload map[string]any) {
			payload["request"].(map[string]any)["requested_url"] = "https://example.com/other"
		},
		"cross domain redirect": func(payload map[string]any) {
			payload["capture"].(map[string]any)["observed_url"] = "https://other.example.net/"
		},
		"unknown navigation parent": func(payload map[string]any) {
			navigation := payload["proposal"].(map[string]any)["header"].(map[string]any)["navigation"].([]any)
			navigation[1].(map[string]any)["parent_key"] = "missing_parent"
		},
	} {
		t.Run(name, func(t *testing.T) {
			payload := validBrandImportPayload(t, info, "https://example.com/catalog")
			mutate(payload)
			response := postAdminJSON(t, client, server.URL+"/admin/api/website/brand-import/validate", csrf, marshalBrandImportPayload(t, info.RequestID, payload))
			if response.StatusCode != http.StatusUnprocessableEntity {
				t.Fatalf("status=%d body=%s", response.StatusCode, responseBody(t, response))
			}
		})
	}
}

func TestBrandImportUnavailableMustNotContainProposalOrEvidence(t *testing.T) {
	server, client, _, _, csrf := normalImportServer(t)
	info := createBrandImportRequest(t, client, server.URL, csrf, "https://example.com/")
	payload := validBrandImportPayload(t, info, "https://example.com/")
	payload["capture"].(map[string]any)["status"] = "unavailable"

	response := postAdminJSON(t, client, server.URL+"/admin/api/website/brand-import/validate", csrf, marshalBrandImportPayload(t, info.RequestID, payload))
	if response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", response.StatusCode, responseBody(t, response))
	}

	payload["proposal"] = nil
	payload["evidence"] = []any{}
	response = postAdminJSON(t, client, server.URL+"/admin/api/website/brand-import/validate", csrf, marshalBrandImportPayload(t, info.RequestID, payload))
	if response.StatusCode != http.StatusOK {
		t.Fatalf("unavailable status=%d body=%s", response.StatusCode, responseBody(t, response))
	}
}

func TestBrandImportRevisionConflictRequiresFreshValidation(t *testing.T) {
	server, client, store, owner, csrf := normalImportServer(t)
	info := createBrandImportRequest(t, client, server.URL, csrf, "https://example.com/")
	payload := validBrandImportPayload(t, info, info.SourceURL)

	validatedResponse := postAdminJSON(t, client, server.URL+"/admin/api/website/brand-import/validate", csrf, marshalBrandImportPayload(t, info.RequestID, payload))
	if validatedResponse.StatusCode != http.StatusOK {
		t.Fatalf("validate status=%d body=%s", validatedResponse.StatusCode, responseBody(t, validatedResponse))
	}
	var validated brandImportValidationResponse
	decodeResponseJSON(t, validatedResponse, &validated)

	state, err := store.WebsiteState(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	concurrent := state.Working
	concurrent.SEO.DefaultTitle = "Concurrent edit"
	state, err = store.SaveWebsiteWorking(t.Context(), owner.ID, state.WorkingRevision, concurrent)
	if err != nil {
		t.Fatal(err)
	}

	for _, revision := range []int64{validated.WorkingRevision, state.WorkingRevision} {
		response := postAdminJSON(t, client, server.URL+"/admin/api/website/brand-import/apply", csrf,
			`{"request_id":"`+info.RequestID+`","expected_revision":`+jsonNumber(t, revision)+`,"rights_confirmed":true}`)
		if response.StatusCode != http.StatusConflict {
			t.Fatalf("stale apply revision=%d status=%d body=%s", revision, response.StatusCode, responseBody(t, response))
		}
	}

	validatedResponse = postAdminJSON(t, client, server.URL+"/admin/api/website/brand-import/validate", csrf, marshalBrandImportPayload(t, info.RequestID, payload))
	if validatedResponse.StatusCode != http.StatusOK {
		t.Fatalf("revalidate status=%d body=%s", validatedResponse.StatusCode, responseBody(t, validatedResponse))
	}
	decodeResponseJSON(t, validatedResponse, &validated)
	if validated.WorkingRevision != state.WorkingRevision {
		t.Fatalf("revalidated revision=%d want=%d", validated.WorkingRevision, state.WorkingRevision)
	}
	response := postAdminJSON(t, client, server.URL+"/admin/api/website/brand-import/apply", csrf,
		`{"request_id":"`+info.RequestID+`","expected_revision":`+jsonNumber(t, validated.WorkingRevision)+`,"rights_confirmed":true}`)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("fresh apply status=%d body=%s", response.StatusCode, responseBody(t, response))
	}
}

func TestStrictJSONRejectsTrailingValue(t *testing.T) {
	var target map[string]any
	if err := strictJSON([]byte(`{"ok":true} {"extra":true}`), &target); err == nil {
		t.Fatal("strictJSON accepted a trailing JSON value")
	}
}

func createBrandImportRequest(t *testing.T, client *http.Client, baseURL, csrf, sourceURL string) brandImportRequestInfo {
	t.Helper()
	body, err := json.Marshal(map[string]string{"source_url": sourceURL})
	if err != nil {
		t.Fatal(err)
	}
	response := postAdminJSON(t, client, baseURL+"/admin/api/website/brand-import/requests", csrf, string(body))
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create request status=%d body=%s", response.StatusCode, responseBody(t, response))
	}
	var info brandImportRequestInfo
	decodeResponseJSON(t, response, &info)
	return info
}

func validBrandImportPayload(t *testing.T, info brandImportRequestInfo, observedURL string) map[string]any {
	t.Helper()
	return map[string]any{
		"schema_version": info.SchemaVersion,
		"request":        map[string]any{"request_id": info.RequestID, "prompt_version": info.PromptVersion, "requested_url": info.SourceURL},
		"capture":        map[string]any{"status": "complete", "observed_url": observedURL, "observed_at": "2026-09-17T12:00:00Z", "source_locale": "en-US", "limitations": []any{}},
		"observations": map[string]any{
			"header":                      map[string]any{"row_types": []any{"main", "primary_navigation"}, "controls": []any{"search", "quote"}, "navigation_depth": 2, "has_mega_menu": false},
			"footer":                      map[string]any{"section_headings": []any{"Products"}, "elements": []any{"brand", "link_columns", "legal_links"}},
			"unsupported_source_features": []any{"Account and cart controls are observations only."},
		},
		"proposal": map[string]any{
			"organization": map[string]any{"display_name": "Example Components", "legal_name": "Example Components, Inc.", "official_website": info.SourceURL, "description": "Electronic components and technical support.", "contacts": []any{}, "social_links": []any{}, "registration_lines": []any{}},
			"header": map[string]any{
				"layout": "commerce", "sticky": true, "brand_presentation": "existing_logo_or_display_name",
				"rows": []any{
					map[string]any{"key": "main", "type": "main", "order": 10, "items": []any{map[string]any{"key": "search", "kind": "system_action", "action": "catalog_search", "order": 10}, map[string]any{"key": "rfq", "kind": "system_action", "action": "rfq", "order": 20}}},
					map[string]any{"key": "primary_navigation", "type": "primary_navigation", "order": 20, "items": []any{}},
				},
				"navigation": []any{
					map[string]any{"key": "products", "parent_key": nil, "label": "Products", "order": 10, "target": map[string]any{"type": "group"}, "presentation": "dropdown"},
					map[string]any{"key": "catalog", "parent_key": "products", "label": "Catalog", "order": 10, "target": map[string]any{"type": "system_action", "action": "catalog"}, "presentation": "direct"},
				},
			},
			"footer": map[string]any{
				"layout": "brand_columns", "brand_block": map[string]any{"show_brand": true, "show_description": true, "contact_keys": []any{}},
				"sections":          []any{map[string]any{"key": "products", "heading": "Products", "order": 10, "collapsible_on_mobile": true, "items": []any{map[string]any{"key": "catalog", "kind": "system_action", "action": "catalog", "order": 10}}}},
				"show_social_links": false, "legal_links": []any{}, "copyright_text": "Copyright Example Components, Inc.", "registration_lines": []any{}, "disclaimer": "Product information is subject to change.",
				"locale_control": map[string]any{"enabled": true, "placement": "both"},
			},
			"theme": map[string]any{"primary_color": "#135C86", "accent_color": "#E38B1A", "body_text_color": "#172033", "border_color": "#D7DDE7", "header_background": "#FFFFFF", "header_text_color": "#172033", "footer_background": "#0C334A", "footer_text_color": "#FFFFFF", "typography_profile": "industrial_sans", "density": "comfortable", "radius": "small", "content_width_px": 1200},
		},
		"evidence": []any{map[string]any{"field_path": "proposal.header", "source_url": observedURL, "observation": "Rendered header and hierarchical navigation observed.", "confidence": "high"}},
	}
}

func marshalBrandImportPayload(t *testing.T, requestID string, payload map[string]any) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{"request_id": requestID, "payload": mustJSON(t, payload)})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func jsonNumber(t *testing.T, value int64) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

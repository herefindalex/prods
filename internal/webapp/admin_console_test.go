package webapp

import (
	"net/http"
	"strings"
	"testing"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/storage/sqlite"
)

func TestAdminConsoleRoutesSessionAndLogout(t *testing.T) {
	server, client, _, owner, csrf := normalImportServer(t)
	for _, path := range []string{"/admin/catalog", "/admin/jobs", "/admin/health"} {
		response, err := client.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		if body := responseBody(t, response); response.StatusCode != 200 || !strings.Contains(body, `id="admin-root"`) {
			t.Fatalf("deep link %s: %d %s", path, response.StatusCode, body)
		}
	}
	response, err := client.Get(server.URL + "/admin/api/session")
	if err != nil {
		t.Fatal(err)
	}
	var session struct {
		ID           string                       `json:"id"`
		Capabilities map[identity.Capability]bool `json:"capabilities"`
	}
	decodeResponseJSON(t, response, &session)
	if session.ID != owner.ID || !session.Capabilities[identity.CapabilityCatalogView] || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("session=%+v headers=%v", session, response.Header)
	}
	for _, path := range []string{"/admin/api/not-a-route", "/admin/not-a-page", "/admin/catalog/not-a-page", "/admin/previews/not-a-preview"} {
		response, err := client.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		if body := responseBody(t, response); response.StatusCode == 200 || strings.Contains(body, `id="admin-root"`) {
			t.Fatalf("unexpected SPA fallback %s: %d %s", path, response.StatusCode, body)
		}
	}
	request, err := http.NewRequest(http.MethodPost, server.URL+"/admin/logout", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, response); response.StatusCode != 204 {
		t.Fatalf("logout: %d %s", response.StatusCode, body)
	}
	response, err = client.Get(server.URL + "/admin/api/session")
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, response); response.StatusCode != 401 || strings.Contains(body, `id="admin-root"`) {
		t.Fatalf("expired session: %d %s", response.StatusCode, body)
	}
}

func TestAdminProductPageContract(t *testing.T) {
	server, client, store, owner, _ := normalImportServer(t)
	for _, part := range []string{"B", "A", "C"} {
		if _, err := store.CreateProduct(t.Context(), owner.ID, catalog.Product{PartNumber: part}); err != nil {
			t.Fatal(err)
		}
	}
	response, err := client.Get(server.URL + "/admin/api/products?page=2&page_size=2&sort=part_number&order=asc")
	if err != nil {
		t.Fatal(err)
	}
	var page sqlite.ProductPage
	decodeResponseJSON(t, response, &page)
	if response.StatusCode != 200 || page.Total != 3 || len(page.Data) != 1 || page.Data[0].PartNumber != "C" || page.Data[0].ID == "" {
		t.Fatalf("page=%+v status=%d", page, response.StatusCode)
	}
	for _, query := range []string{"page=0", "page=bad", "page_size=101", "sort=unknown", "order=unknown", "page=1&include_archived=bad"} {
		response, err := client.Get(server.URL + "/admin/api/products?" + query)
		if err != nil {
			t.Fatal(err)
		}
		if body := responseBody(t, response); response.StatusCode != 400 || !strings.Contains(body, `"code":"validation_failed"`) {
			t.Fatalf("query %s: %d %s", query, response.StatusCode, body)
		}
	}
	response, err = http.Get(server.URL + "/admin/api/products?page=1")
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, response); response.StatusCode != 401 {
		t.Fatalf("anonymous page: %d %s", response.StatusCode, body)
	}
}

func TestAdminClientScopeCannotFollowAReplacedCookieSession(t *testing.T) {
	server, client, _, owner, csrf := normalImportServer(t)
	oldScope := identity.TokenDigest(csrf)
	newCSRF := loginNormalAdmin(t, client, server.URL, owner.Email, "ownerpass1")
	if oldScope == identity.TokenDigest(newCSRF) {
		t.Fatal("new login reused session scope")
	}
	for _, methodAndPath := range [][2]string{{"GET", "/admin/api/products?page=1"}, {"POST", "/admin/logout"}} {
		request, err := http.NewRequest(methodAndPath[0], server.URL+methodAndPath[1], nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("X-Prods-Session-Scope", oldScope)
		request.Header.Set("X-CSRF-Token", csrf)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		if body := responseBody(t, response); response.StatusCode != 401 {
			t.Fatalf("old client accessed replacement session: %d %s", response.StatusCode, body)
		}
	}
	response, err := client.Get(server.URL + "/admin/api/session")
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, response); response.StatusCode != 200 {
		t.Fatalf("old client revoked replacement session: %d %s", response.StatusCode, body)
	}
}

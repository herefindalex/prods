package webapp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"prods/internal/catalog"
	"prods/internal/distribution"
	"prods/internal/sampledata"
	"prods/internal/storage/sqlite"
)

func TestInstallerUsesEmbeddedMatchingVersionSampleData(t *testing.T) {
	root := t.TempDir()
	store, err := sqlite.Create(filepath.Join(root, "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	installer, _, err := NewInstaller(store, InstallerConfig{
		BootstrapToken: "bootstrap", DataDir: filepath.Join(root, "data"), BackupDir: filepath.Join(root, "backups"),
		DefaultLocale: "en-US", DefaultTimeZone: "UTC", ApplicationVersion: "v0.6.6",
	})
	if err != nil {
		t.Fatal(err)
	}
	claim := jsonFormRequest(installer, http.MethodPost, "/install/claim", url.Values{"token": {"bootstrap"}})
	cookie := cookieNamed(claim.Result(), "prods_install")
	var state installerState
	if err := json.Unmarshal(claim.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if !state.SampleDataAvailable || state.ApplicationVersion != "v0.6.6" {
		t.Fatalf("installer sample state = %+v", state)
	}
	complete := formRequestWithCookies(installer, http.MethodPost, "/install/complete", url.Values{
		"csrf_token": {state.CSRFToken}, "owner_email": {"owner@example.test"}, "password": {"ownerpass1"},
		"password_confirm": {"ownerpass1"}, "default_locale": {"en-US"}, "supported_locales": {"en-US"},
		"time_zone": {"UTC"}, "use_sample_data": {"true"},
	}, cookie)
	if complete.Code != http.StatusOK {
		t.Fatalf("sample completion status=%d body=%s", complete.Code, complete.Body.String())
	}
	count, err := store.ProductCount(t.Context())
	if err != nil || count != 1200 {
		t.Fatalf("sample product count=%d err=%v", count, err)
	}
	if _, err := os.Stat(filepath.Join(root, "data", "sample-data", distribution.SampleAssetName)); !os.IsNotExist(err) {
		t.Fatalf("embedded sample unexpectedly wrote a runtime payload: %v", err)
	}
}

func TestInstallerEmbeddedSampleVersionMismatchStillCompletesBlankInstallation(t *testing.T) {
	root := t.TempDir()
	store, err := sqlite.Create(filepath.Join(root, "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	installer, _, err := NewInstaller(store, InstallerConfig{
		BootstrapToken: "bootstrap", DataDir: filepath.Join(root, "data"), BackupDir: filepath.Join(root, "backups"),
		DefaultLocale: "en-US", DefaultTimeZone: "UTC", ApplicationVersion: "v1.2.3",
	})
	if err != nil {
		t.Fatal(err)
	}
	claim := jsonFormRequest(installer, http.MethodPost, "/install/claim", url.Values{"token": {"bootstrap"}})
	cookie := cookieNamed(claim.Result(), "prods_install")
	var state installerState
	if err := json.Unmarshal(claim.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.SampleDataAvailable || state.CSRFToken == "" {
		t.Fatalf("unpublished repository state = %+v", state)
	}
	complete := formRequestWithCookies(installer, http.MethodPost, "/install/complete", url.Values{
		"csrf_token": {state.CSRFToken}, "owner_email": {"owner@example.test"}, "password": {"ownerpass1"},
		"password_confirm": {"ownerpass1"}, "default_locale": {"en-US"}, "supported_locales": {"en-US"},
		"time_zone": {"UTC"},
	}, cookie)
	if complete.Code != http.StatusOK {
		t.Fatalf("blank completion status=%d body=%s", complete.Code, complete.Body.String())
	}
	if count, err := store.ProductCount(t.Context()); err != nil || count != 0 {
		t.Fatalf("blank product count=%d err=%v", count, err)
	}
}

func TestInstallerCommitsCompleteEmbeddedSampleData(t *testing.T) {
	const version = "v0.6.6"
	sample, err := sampledata.Generate(version)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	store, err := sqlite.Create(filepath.Join(root, "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	installer, _, err := NewInstaller(store, InstallerConfig{
		BootstrapToken: "bootstrap", DataDir: filepath.Join(root, "data"), BackupDir: filepath.Join(root, "backups"),
		DefaultLocale: "en-US", DefaultTimeZone: "UTC", ApplicationVersion: version,
	})
	if err != nil {
		t.Fatal(err)
	}
	claim := jsonFormRequest(installer, http.MethodPost, "/install/claim", url.Values{"token": {"bootstrap"}})
	cookie := cookieNamed(claim.Result(), "prods_install")
	var state installerState
	if err := json.Unmarshal(claim.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	complete := formRequestWithCookies(installer, http.MethodPost, "/install/complete", url.Values{
		"csrf_token": {state.CSRFToken}, "owner_email": {"owner@example.test"}, "password": {"ownerpass1"},
		"password_confirm": {"ownerpass1"}, "default_locale": {"en-US"}, "supported_locales": {"en-US"},
		"time_zone": {"UTC"}, "use_sample_data": {"true"},
	}, cookie)
	if complete.Code != http.StatusOK {
		t.Fatalf("generated sample completion status=%d body=%s", complete.Code, complete.Body.String())
	}
	if count, err := store.ProductCount(t.Context()); err != nil || count != len(sample.Products) {
		t.Fatalf("generated sample product count=%d want=%d err=%v", count, len(sample.Products), err)
	}
	for _, check := range []struct {
		kind catalog.DictionaryKind
		want int
	}{
		{catalog.DictionaryManufacturer, 20},
		{catalog.DictionaryBrand, 15},
		{catalog.DictionaryApplication, 24},
		{catalog.DictionaryLifecycle, 5},
		{catalog.DictionaryDocumentType, 4},
	} {
		entries, err := store.ListDictionaryEntries(t.Context(), check.kind)
		if err != nil || len(entries) != check.want {
			t.Fatalf("generated sample %s count=%d want=%d err=%v", check.kind, len(entries), check.want, err)
		}
	}
	specs, err := store.ListSpecDefinitions(t.Context())
	if err != nil || len(specs) != 34 {
		t.Fatalf("generated sample specification count=%d want=34 err=%v", len(specs), err)
	}
	sets, err := store.ListSpecSets(t.Context())
	if err != nil || len(sets) != 24 {
		t.Fatalf("generated sample specification set count=%d want=24 err=%v", len(sets), err)
	}
	var richProduct distribution.Product
	for _, source := range sample.Products {
		if source.ManufacturerID != "" && len(source.ApplicationIDs) > 0 && len(source.SpecValues) > 0 {
			richProduct = source
			break
		}
	}
	if richProduct.ID == "" {
		t.Fatal("generated sample has no product with reference and specification values")
	}
	storedProduct, err := store.Product(t.Context(), richProduct.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedProduct.ManufacturerID != richProduct.ManufacturerID || len(storedProduct.ApplicationIDs) != len(richProduct.ApplicationIDs) {
		t.Fatalf("generated sample product references were not preserved: got=%+v want=%+v", storedProduct, richProduct)
	}
	values, err := store.ProductSpecValues(t.Context(), richProduct.ID)
	if err != nil || len(values) != len(richProduct.SpecValues) {
		t.Fatalf("generated sample product specification count=%d want=%d err=%v", len(values), len(richProduct.SpecValues), err)
	}
	set, err := store.CategorySpecSet(t.Context(), richProduct.CategoryID)
	if err != nil || set.ID == "" {
		t.Fatalf("generated sample category specification set=%+v err=%v", set, err)
	}
	for _, source := range sample.Products {
		if source.RecordState != "archived" {
			continue
		}
		product, err := store.Product(t.Context(), source.ID)
		if err != nil {
			t.Fatal(err)
		}
		if product.RecordState != "archived" {
			t.Fatalf("generated archived product %q state=%q", source.ID, product.RecordState)
		}
		return
	}
	t.Fatal("generated sample did not contain an archived product")
}

func TestInstallerOwnershipRestartAndMinimumJourney(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	backupDir := filepath.Join(root, "backups")
	store, err := sqlite.Create(filepath.Join(root, "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	first, _, err := NewInstaller(store, InstallerConfig{
		BootstrapToken: "first-bootstrap", DataDir: dataDir, BackupDir: backupDir,
		DefaultLocale: "zh-TW", DefaultTimeZone: "Asia/Taipei",
	})
	if err != nil {
		t.Fatal(err)
	}
	claimResults := make(chan *httptest.ResponseRecorder, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimResults <- formRequest(first, http.MethodPost, "/install/claim", url.Values{"token": {"first-bootstrap"}})
		}()
	}
	wg.Wait()
	close(claimResults)
	var installCookie *http.Cookie
	statuses := map[int]int{}
	for response := range claimResults {
		statuses[response.Code]++
		if response.Code == http.StatusSeeOther {
			installCookie = cookieNamed(response.Result(), "prods_install")
		}
	}
	if statuses[http.StatusSeeOther] != 1 || statuses[http.StatusConflict] != 1 || installCookie == nil {
		t.Fatalf("claim statuses=%v cookie=%v", statuses, installCookie)
	}

	// A process restart invalidates the old in-memory installer ownership and
	// requires the newly printed token; it does not recreate or replace the DB.
	completed := make(chan struct{}, 1)
	second, _, err := NewInstaller(store, InstallerConfig{
		BootstrapToken: "second-bootstrap", DataDir: dataDir, BackupDir: backupDir,
		DefaultLocale: "zh-TW", DefaultTimeZone: "Asia/Taipei",
		OnComplete: func() { completed <- struct{}{} },
	})
	if err != nil {
		t.Fatal(err)
	}
	oldSession := requestWithCookies(second, http.MethodGet, "/install/api/state", nil, installCookie)
	var oldState installerState
	if oldSession.Code != http.StatusOK || json.Unmarshal(oldSession.Body.Bytes(), &oldState) != nil || oldState.Stage != "claim" {
		t.Fatalf("old installer session remained valid: status=%d state=%+v body=%s", oldSession.Code, oldState, oldSession.Body.String())
	}
	claimed := formRequest(second, http.MethodPost, "/install/claim", url.Values{"token": {"second-bootstrap"}})
	if claimed.Code != http.StatusSeeOther {
		t.Fatalf("new claim status=%d body=%s", claimed.Code, claimed.Body.String())
	}
	installCookie = cookieNamed(claimed.Result(), "prods_install")
	stateResponse := requestWithCookies(second, http.MethodGet, "/install/api/state", nil, installCookie)
	var setupState installerState
	if err := json.Unmarshal(stateResponse.Body.Bytes(), &setupState); err != nil || setupState.Stage != "setup" || setupState.CSRFToken == "" {
		t.Fatalf("installer setup state=%+v err=%v body=%s", setupState, err, stateResponse.Body.String())
	}
	completedRequest := httptest.NewRequest(http.MethodPost, "/install/complete", strings.NewReader(url.Values{
		"csrf_token": {setupState.CSRFToken}, "owner_email": {"Owner@Example.test"}, "owner_display_name": {"First Owner"},
		"password": {"ownerpass1"}, "password_confirm": {"ownerpass1"}, "default_locale": {"zh-TW"},
		"supported_locales": {"zh-TW, en-US"}, "time_zone": {"Asia/Taipei"},
	}.Encode()))
	completedRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	completedRequest.Header.Set("Accept", "application/json")
	completedRequest.AddCookie(installCookie)
	completedResponse := httptest.NewRecorder()
	second.ServeHTTP(completedResponse, completedRequest)
	var completedState installerState
	if completedResponse.Code != http.StatusOK || json.Unmarshal(completedResponse.Body.Bytes(), &completedState) != nil || completedState.Stage != "complete" {
		t.Fatalf("completion status=%d body=%s", completedResponse.Code, completedResponse.Body.String())
	}
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("installer completion did not request process stop")
	}
	if inspection := sqlite.Inspect(filepath.Join(root, "prods.db")); inspection.State != sqlite.DatabaseReady || inspection.Kind != sqlite.DatabaseKindSite {
		t.Fatalf("inspection = %+v", inspection)
	}
	if count, err := store.ProductCount(t.Context()); err != nil || count != 0 {
		t.Fatalf("empty installation product count=%d err=%v", count, err)
	}

	app, generated, err := New(store, Config{BaseURL: "https://catalog.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if generated != "" {
		t.Fatal("normal site unexpectedly generated a temporary Admin token")
	}
	wrongLogin := formRequest(app, http.MethodPost, "/admin/login", url.Values{"email": {"owner@example.test"}, "password": {"wrongpass1"}})
	if wrongLogin.Code != http.StatusUnauthorized {
		t.Fatalf("wrong login status=%d", wrongLogin.Code)
	}
	login := formRequest(app, http.MethodPost, "/admin/login", url.Values{"email": {"owner@example.test"}, "password": {"ownerpass1"}})
	if login.Code != http.StatusSeeOther {
		t.Fatalf("login status=%d body=%s", login.Code, login.Body.String())
	}
	adminCookie := cookieNamed(login.Result(), "prods_admin")
	adminPage := requestWithCookies(app, http.MethodGet, "/admin", nil, adminCookie)
	if adminPage.Code != http.StatusOK {
		t.Fatalf("admin status=%d body=%s", adminPage.Code, adminPage.Body.String())
	}
	adminCSRF := matchValue(t, adminPage.Body.String(), `name="csrf-token" content="([^"]+)"`)

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = sqlite.OpenReady(filepath.Join(root, "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	app, _, err = New(store, Config{BaseURL: "https://catalog.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	adminAfterRestart := requestWithCookies(app, http.MethodGet, "/admin", nil, adminCookie)
	if adminAfterRestart.Code != http.StatusOK {
		t.Fatalf("durable Admin session after restart status=%d", adminAfterRestart.Code)
	}

	productRequest := httptest.NewRequest(http.MethodPost, "/admin/api/products", strings.NewReader(`{"id":"m1-product","part_number":"M1-100","manufacturer":"Example","status":"published"}`))
	productRequest.Header.Set("Content-Type", "application/json")
	productRequest.Header.Set("X-CSRF-Token", adminCSRF)
	productRequest.AddCookie(adminCookie)
	productResponse := httptest.NewRecorder()
	app.ServeHTTP(productResponse, productRequest)
	if productResponse.Code != http.StatusCreated {
		t.Fatalf("create product status=%d body=%s", productResponse.Code, productResponse.Body.String())
	}
	var publicProduct *httptest.ResponseRecorder
	deadline := time.Now().Add(2 * time.Second)
	for {
		publicProduct = requestWithCookies(app, http.MethodGet, "/products/M1-100", nil)
		if publicProduct.Code == http.StatusOK || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if publicProduct.Code != http.StatusOK || !strings.Contains(publicProduct.Body.String(), "M1-100") {
		t.Fatalf("public product status=%d body=%s", publicProduct.Code, publicProduct.Body.String())
	}

	rfqForm := requestWithCookies(app, http.MethodGet, "/rfq?product_id=m1-product", nil)
	anonCookie := cookieNamed(rfqForm.Result(), "prods_anon")
	rfqCSRF := matchValue(t, rfqForm.Body.String(), `name="csrf_token" value="([^"]+)"`)
	submissionKey := matchValue(t, rfqForm.Body.String(), `name="submission_key" value="([^"]+)"`)
	rfqResponse := formRequestWithCookies(app, http.MethodPost, "/rfq", url.Values{
		"csrf_token": {rfqCSRF}, "submission_key": {submissionKey}, "kind": {"catalog"},
		"product_id": {"m1-product"}, "name": {"Buyer"}, "email": {"buyer@example.test"},
	}, anonCookie)
	if rfqResponse.Code != http.StatusOK || !strings.Contains(rfqResponse.Body.String(), "已收到詢價") || !strings.Contains(rfqResponse.Body.String(), `lang="zh-TW"`) {
		t.Fatalf("RFQ status=%d body=%s", rfqResponse.Code, rfqResponse.Body.String())
	}

	listRequest := httptest.NewRequest(http.MethodGet, "/admin/api/rfqs", nil)
	listRequest.AddCookie(adminCookie)
	listResponse := httptest.NewRecorder()
	app.ServeHTTP(listResponse, listRequest)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("Admin RFQ list status=%d body=%s", listResponse.Code, listResponse.Body.String())
	}
	var rfqs []sqlite.RFQSummary
	if err := json.Unmarshal(listResponse.Body.Bytes(), &rfqs); err != nil {
		t.Fatal(err)
	}
	if len(rfqs) != 1 || len(rfqs[0].Items) != 1 || rfqs[0].Items[0].ProductID != "m1-product" {
		t.Fatalf("persisted RFQs = %+v", rfqs)
	}
}

func TestInstallerJSONClaimReturnsOwnedSetupState(t *testing.T) {
	store, err := sqlite.Create(filepath.Join(t.TempDir(), "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	server, _, err := NewInstaller(store, InstallerConfig{
		BootstrapToken:  "json-bootstrap",
		DataDir:         t.TempDir(),
		BackupDir:       t.TempDir(),
		DefaultLocale:   "zh-TW",
		DefaultTimeZone: "Asia/Taipei",
	})
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/install/claim", strings.NewReader(url.Values{
		"token": {"json-bootstrap"},
	}.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	var state installerState
	if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || state.Stage != "setup" || state.CSRFToken == "" ||
		state.DefaultLocale != "zh-TW" || state.SupportedLocales != "zh-TW" || state.TimeZone != "Asia/Taipei" {
		t.Fatalf("claim status=%d state=%+v body=%s", response.Code, state, response.Body.String())
	}
	if cookieNamed(response.Result(), "prods_install") == nil {
		t.Fatal("successful JSON claim did not issue installer session cookie")
	}
}

func formRequest(handler http.Handler, method, path string, values url.Values) *httptest.ResponseRecorder {
	return formRequestWithCookies(handler, method, path, values)
}

func jsonFormRequest(handler http.Handler, method, path string, values url.Values) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(values.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func formRequestWithCookies(handler http.Handler, method, path string, values url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(values.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, cookie := range cookies {
		if cookie != nil {
			request.AddCookie(cookie)
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func requestWithCookies(handler http.Handler, method, path string, body io.Reader, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, body)
	for _, cookie := range cookies {
		if cookie != nil {
			request.AddCookie(cookie)
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func cookieNamed(response *http.Response, name string) *http.Cookie {
	for _, cookie := range response.Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func matchValue(t *testing.T, body, expression string) string {
	t.Helper()
	match := regexp.MustCompile(expression).FindStringSubmatch(body)
	if len(match) != 2 {
		t.Fatalf("missing %s in %s", expression, body)
	}
	return match[1]
}

func TestInstallerRoutesExcludeNormalApplication(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "prods.db")
	store, err := sqlite.Create(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app, _, err := NewInstaller(store, InstallerConfig{BootstrapToken: "token", DataDir: t.TempDir(), BackupDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/search", "/rfq", "/admin", "/products/example", "/admin/api/rfqs"} {
		response := requestWithCookies(app, http.MethodGet, path, nil)
		if response.Code != http.StatusNotFound {
			t.Errorf("%s status=%d", path, response.Code)
		}
	}
	if response := requestWithCookies(app, http.MethodGet, "/health/live", nil); response.Code != http.StatusOK {
		t.Fatalf("installer liveness status=%d", response.Code)
	}
	if response := requestWithCookies(app, http.MethodGet, "/health/ready", nil); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("installer readiness status=%d", response.Code)
	}
	if state := sqlite.Inspect(dbPath); state.State != sqlite.DatabaseInstalling {
		t.Fatalf("installer routes changed database state: %+v", state)
	}
}

func TestInstallerLanguageCanBeSelectedBeforeInstallation(t *testing.T) {
	root := t.TempDir()
	store, err := sqlite.Create(filepath.Join(root, "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	server, _, err := NewInstaller(store, InstallerConfig{
		BootstrapToken: "bootstrap", DataDir: filepath.Join(root, "data"), BackupDir: filepath.Join(root, "backups"),
	})
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/install?lang=zh-TW&token=bootstrap", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `lang="zh-TW"`) || !strings.Contains(body, `data-mode="installer"`) || !strings.Contains(body, `/static/system/system.js`) || strings.Contains(body, "token=bootstrap") {
		t.Fatalf("traditional Chinese installer status=%d body=%s", response.Code, body)
	}

	request = httptest.NewRequest(http.MethodGet, "/install", nil)
	request.Header.Set("Accept-Language", "zh-TW,zh;q=0.9,en;q=0.5")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if body := response.Body.String(); response.Code != http.StatusOK || !strings.Contains(body, `data-mode="installer"`) {
		t.Fatalf("negotiated installer status=%d body=%s", response.Code, body)
	}

	request = httptest.NewRequest(http.MethodPost, "/install/claim", strings.NewReader(url.Values{
		"token": {"wrong"}, "interface_locale": {"zh-TW"},
	}.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "Bootstrap token 無效") || !strings.Contains(response.Body.String(), `"field":"token"`) {
		t.Fatalf("localized installer error status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestInstallerOnlyAcceptsEmbeddedInterfaceLocales(t *testing.T) {
	base := installerPage{
		OwnerEmail:       "owner@example.test",
		DefaultLocale:    "en-US",
		SupportedLocales: "en-US,zh-TW",
		TimeZone:         "UTC",
	}
	if locales, err := validateInstallationForm(base, "ownerpass1", "ownerpass1"); err != nil || len(locales) != 2 {
		t.Fatalf("valid embedded locales = %v, err=%v", locales, err)
	}

	unsupportedDefault := base
	unsupportedDefault.DefaultLocale = "ru-RU"
	unsupportedDefault.SupportedLocales = "ru-RU"
	if _, err := validateInstallationForm(unsupportedDefault, "ownerpass1", "ownerpass1"); err == nil {
		t.Fatal("unsupported default locale was accepted")
	} else if fieldErr, ok := err.(*installerFieldError); !ok || fieldErr.Field != "default_locale" {
		t.Fatalf("unsupported default locale error = %#v", err)
	}

	unsupportedList := base
	unsupportedList.SupportedLocales = "en-US,ru-RU"
	if _, err := validateInstallationForm(unsupportedList, "ownerpass1", "ownerpass1"); err == nil {
		t.Fatal("unsupported locale list was accepted")
	} else if fieldErr, ok := err.(*installerFieldError); !ok || fieldErr.Field != "supported_locales" {
		t.Fatalf("unsupported locale list error = %#v", err)
	}

	canonical := base
	canonical.DefaultLocale = "en-us"
	canonical.SupportedLocales = "en-US,zh-tw"
	if locales, err := validateInstallationForm(canonical, "ownerpass1", "ownerpass1"); err != nil || len(locales) != 2 || locales[1] != "zh-TW" {
		t.Fatalf("canonical embedded locales = %v, err=%v", locales, err)
	}
}

func TestLoginThrottleIsTemporaryAndProgressive(t *testing.T) {
	server := &Server{loginAttempts: make(map[string]loginAttempt)}
	now := time.Now().UTC()
	for range 4 {
		server.recordLoginFailure("Owner@Example.test", now)
		if _, allowed := server.loginAllowed("owner@example.test", now); !allowed {
			t.Fatal("login throttled before the temporary threshold")
		}
	}
	server.recordLoginFailure("owner@example.test", now)
	firstDelay, allowed := server.loginAllowed("OWNER@example.test", now)
	if allowed || firstDelay < time.Second {
		t.Fatalf("first delay=%s allowed=%v", firstDelay, allowed)
	}
	server.recordLoginFailure("owner@example.test", now.Add(2*time.Second))
	secondDelay, allowed := server.loginAllowed("owner@example.test", now.Add(2*time.Second))
	if allowed || secondDelay < 2*time.Second {
		t.Fatalf("second delay=%s allowed=%v", secondDelay, allowed)
	}
	server.clearLoginFailures("owner@example.test")
	if _, allowed := server.loginAllowed("owner@example.test", now); !allowed {
		t.Fatal("successful login did not clear temporary throttle")
	}
}

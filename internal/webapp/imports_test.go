package webapp

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/importing"
	"prods/internal/storage/sqlite"
)

func normalImportServer(t *testing.T) (*httptest.Server, *http.Client, *sqlite.Store, identity.User, string) {
	t.Helper()
	store, err := sqlite.Create(filepath.Join(t.TempDir(), "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	hash, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.CompleteInstallation(t.Context(), sqlite.Installation{
		OwnerEmail: "owner@example.test", PasswordHash: hash,
		DefaultLocale: "en-US", SupportedLocales: []string{"en-US"}, TimeZone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	app, _, err := New(store, Config{BaseURL: "http://catalog.example.test", WorkDir: filepath.Join(t.TempDir(), "work")})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app)
	t.Cleanup(server.Close)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	response, err := client.PostForm(server.URL+"/admin/login", url.Values{"email": {owner.Email}, "password": {"ownerpass1"}})
	if err != nil {
		t.Fatal(err)
	}
	body := responseBody(t, response)
	match := regexp.MustCompile(`name="csrf-token" content="([^"]+)"`).FindStringSubmatch(body)
	if len(match) != 2 {
		t.Fatalf("login page missing CSRF: %s", body)
	}
	return server, client, store, owner, match[1]
}

func xlsxForImport(t *testing.T, rows ...[]string) []byte {
	t.Helper()
	file := excelize.NewFile()
	sheet := file.GetSheetName(0)
	all := append([][]string{{"P/N", "Maker", "Name"}}, rows...)
	for rowIndex, row := range all {
		for columnIndex, value := range row {
			cell, err := excelize.CoordinatesToCellName(columnIndex+1, rowIndex+1)
			if err != nil {
				t.Fatal(err)
			}
			if err := file.SetCellValue(sheet, cell, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	var encoded bytes.Buffer
	if err := file.Write(&encoded); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func submitImport(t *testing.T, client *http.Client, baseURL, csrf string, file []byte, snapshot importing.TemplateSnapshot) importing.Job {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	templatePart, err := writer.CreateFormField("template")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(templatePart).Encode(snapshot); err != nil {
		t.Fatal(err)
	}
	filePart, err := writer.CreateFormFile("file", "supplier.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := filePart.Write(file); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest(http.MethodPost, baseURL+"/admin/api/imports", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("X-CSRF-Token", csrf)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	encoded := responseBody(t, response)
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("submit import status=%d body=%s", response.StatusCode, encoded)
	}
	var job importing.Job
	if err := json.Unmarshal([]byte(encoded), &job); err != nil {
		t.Fatal(err)
	}
	return job
}

func waitImportJob(t *testing.T, client *http.Client, baseURL, id string) struct {
	Job     importing.Job      `json:"job"`
	Preview *importing.Preview `json:"preview"`
} {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get(baseURL + "/admin/api/imports/" + id)
		if err != nil {
			t.Fatal(err)
		}
		encoded := responseBody(t, response)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("get import status=%d body=%s", response.StatusCode, encoded)
		}
		var result struct {
			Job     importing.Job      `json:"job"`
			Preview *importing.Preview `json:"preview"`
		}
		if err := json.Unmarshal([]byte(encoded), &result); err != nil {
			t.Fatal(err)
		}
		if result.Job.Status == importing.JobPreviewReady || result.Job.Status == importing.JobFailed ||
			result.Job.Status == importing.JobCancelled || result.Job.Status == importing.JobInterrupted {
			return result
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for Import preview")
	return struct {
		Job     importing.Job      `json:"job"`
		Preview *importing.Preview `json:"preview"`
	}{}
}

func TestBackgroundXLSXImportProducesFullReportAndCommitsAtomicPlan(t *testing.T) {
	server, client, store, owner, csrf := normalImportServer(t)
	maker, err := store.CreateDictionaryEntry(t.Context(), owner.ID, catalog.DictionaryEntry{Kind: catalog.DictionaryManufacturer, Name: "Maker"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := importing.TemplateSnapshot{HeaderRow: 1, IdentityMode: importing.IdentityComposite, Mappings: []importing.ColumnMapping{
		{SourceIndex: 0, SourceName: "P/N", Target: importing.TargetPartNumber},
		{SourceIndex: 1, SourceName: "Maker", Target: importing.TargetManufacturerID},
		{SourceIndex: 2, SourceName: "Name", Target: importing.TargetProductName},
	}}
	invalid := submitImport(t, client, server.URL, csrf, xlsxForImport(t,
		[]string{"", maker.ID, "Missing part"},
		[]string{"BAD-MAKER", "missing", "Unknown maker"},
	), snapshot)
	invalidResult := waitImportJob(t, client, server.URL, invalid.ID)
	if invalidResult.Job.Status != importing.JobPreviewReady || invalidResult.Preview == nil ||
		invalidResult.Preview.FullyValidated || len(invalidResult.Preview.Issues) != 2 {
		t.Fatalf("invalid result = %+v", invalidResult)
	}
	report, err := client.Get(server.URL + "/admin/api/imports/" + invalid.ID + "/report")
	if err != nil {
		t.Fatal(err)
	}
	reportBody, err := io.ReadAll(report.Body)
	report.Body.Close()
	if err != nil || report.StatusCode != http.StatusOK || !strings.Contains(string(reportBody), "part_number_required") ||
		!strings.Contains(string(reportBody), "invalid_reference") {
		t.Fatalf("report status=%d body=%s err=%v", report.StatusCode, reportBody, err)
	}

	valid := submitImport(t, client, server.URL, csrf, xlsxForImport(t,
		[]string{"HTTP-IMP-1", maker.ID, "Imported one"},
		[]string{"HTTP-IMP-2", maker.ID, "Imported two"},
	), snapshot)
	validResult := waitImportJob(t, client, server.URL, valid.ID)
	if validResult.Job.Status != importing.JobPreviewReady || validResult.Preview == nil ||
		!validResult.Preview.FullyValidated || validResult.Preview.CreateCount != 2 {
		t.Fatalf("valid result = %+v", validResult)
	}
	commit, _ := http.NewRequest(http.MethodPost, server.URL+"/admin/api/imports/"+valid.ID+"/commit", nil)
	commit.Header.Set("X-CSRF-Token", csrf)
	commitResponse, err := client.Do(commit)
	if err != nil {
		t.Fatal(err)
	}
	commitBody := responseBody(t, commitResponse)
	if commitResponse.StatusCode != http.StatusOK || !strings.Contains(commitBody, `"created":2`) {
		t.Fatalf("commit status=%d body=%s", commitResponse.StatusCode, commitBody)
	}
	job, err := store.ImportJob(t.Context(), valid.ID)
	if err != nil || job.Status != importing.JobCommitted {
		t.Fatalf("committed job = %+v, %v", job, err)
	}
}

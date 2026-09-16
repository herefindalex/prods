package webapp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prods/internal/storage/sqlite"
)

func TestRuntimeLogTailRequiresSystemCapabilityAndHidesHostPath(t *testing.T) {
	root := t.TempDir()
	logPath := filepath.Join(root, "private", "prods.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte("first\nsecond\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath+".1", []byte("older\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := sqlite.CreatePOC(filepath.Join(root, "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	app, _, err := New(store, Config{
		BaseURL: "http://catalog.example.test", AdminToken: "test-admin-token", EnablePOCAdmin: true,
		RuntimeLogPath: logPath, RuntimeLogFiles: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	server := httptest.NewServer(app)
	t.Cleanup(server.Close)

	unauthorized, err := http.Get(server.URL + "/admin/api/system/runtime-log")
	if err != nil {
		t.Fatal(err)
	}
	_ = unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized runtime log status=%d", unauthorized.StatusCode)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	loginAdmin(t, client, server.URL)

	response, err := client.Get(server.URL + "/admin/api/system/runtime-log")
	if err != nil {
		t.Fatal(err)
	}
	var current runtimeLogResponse
	if err := json.NewDecoder(response.Body).Decode(&current); err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || !current.Available || current.FileName != "prods.log" || len(current.Lines) != 2 {
		t.Fatalf("current runtime log status=%d body=%+v", response.StatusCode, current)
	}
	encoded, err := json.Marshal(current)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), root) {
		t.Fatalf("runtime log response exposed host path: %s", encoded)
	}

	rotated, err := client.Get(server.URL + "/admin/api/system/runtime-log?generation=1")
	if err != nil {
		t.Fatal(err)
	}
	var previous runtimeLogResponse
	if err := json.NewDecoder(rotated.Body).Decode(&previous); err != nil {
		t.Fatal(err)
	}
	_ = rotated.Body.Close()
	if rotated.StatusCode != http.StatusOK || previous.FileName != "prods.log.1" || len(previous.Lines) != 1 || previous.Lines[0] != "older" {
		t.Fatalf("rotated runtime log status=%d body=%+v", rotated.StatusCode, previous)
	}

	missing, err := client.Get(server.URL + "/admin/api/system/runtime-log?generation=2")
	if err != nil {
		t.Fatal(err)
	}
	var unavailable runtimeLogResponse
	if err := json.NewDecoder(missing.Body).Decode(&unavailable); err != nil {
		t.Fatal(err)
	}
	_ = missing.Body.Close()
	if missing.StatusCode != http.StatusOK || unavailable.Available || unavailable.Lines == nil {
		t.Fatalf("missing runtime log status=%d body=%+v", missing.StatusCode, unavailable)
	}

	invalid, err := client.Get(server.URL + "/admin/api/system/runtime-log?generation=3")
	if err != nil {
		t.Fatal(err)
	}
	_ = invalid.Body.Close()
	if invalid.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid runtime log generation status=%d", invalid.StatusCode)
	}

	unauthorizedText, err := http.Get(server.URL + "/admin/api/system/runtime-log/text")
	if err != nil {
		t.Fatal(err)
	}
	_ = unauthorizedText.Body.Close()
	if unauthorizedText.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized runtime log text status=%d", unauthorizedText.StatusCode)
	}

	textResponse, err := client.Get(server.URL + "/admin/api/system/runtime-log/text")
	if err != nil {
		t.Fatal(err)
	}
	textBody, err := io.ReadAll(textResponse.Body)
	_ = textResponse.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if textResponse.StatusCode != http.StatusOK || textResponse.Header.Get("Content-Type") != "text/plain; charset=utf-8" || string(textBody) != "first\nsecond\n" {
		t.Fatalf("runtime log text status=%d content-type=%q body=%q", textResponse.StatusCode, textResponse.Header.Get("Content-Type"), textBody)
	}

	rotatedText, err := client.Get(server.URL + "/admin/api/system/runtime-log/text?generation=1")
	if err != nil {
		t.Fatal(err)
	}
	rotatedBody, err := io.ReadAll(rotatedText.Body)
	_ = rotatedText.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if rotatedText.StatusCode != http.StatusOK || string(rotatedBody) != "older\n" {
		t.Fatalf("rotated runtime log text status=%d body=%q", rotatedText.StatusCode, rotatedBody)
	}

	missingText, err := client.Get(server.URL + "/admin/api/system/runtime-log/text?generation=2")
	if err != nil {
		t.Fatal(err)
	}
	_ = missingText.Body.Close()
	if missingText.StatusCode != http.StatusNotFound {
		t.Fatalf("missing runtime log text status=%d", missingText.StatusCode)
	}
}

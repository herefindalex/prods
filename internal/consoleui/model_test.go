package consoleui

import (
	"bytes"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestViewKeepsProductLineOutsideStatusAndLogPanels(t *testing.T) {
	m := newModel(Guide{
		Status: "等待安裝", Action: "請在瀏覽器開啟以下網址以進行安裝。", URL: "http://127.0.0.1:8080/install?token=test",
		Version: "v1.0.0", Host: "host (windows/amd64)", Listen: ":8080", BaseURL: "http://127.0.0.1:8080", AdminURL: "http://127.0.0.1:8080/admin", ConfigPath: `C:\\Prods\\prods.ini`,
		DataDir: `C:\\Prods\\data`, BackupDir: `C:\\Prods\\backups`, DatabasePath: `C:\\Prods\\data\\prods.db`,
	}, []string{
		`{"time":"2026-09-16T06:01:51-04:00","level":"INFO","msg":"resolved Prods runtime","version":"dev","go":"go1.27.1","config":"/tmp/prods/prods.ini","listen":"127.0.0.1:18559","base_url":"http://127.0.0.1:18559","data_dir":"/tmp/prods/data","backup_dir":"/tmp/prods/backups","database":"/tmp/prods/data/prods.db","database_state":"fresh","instance_kind":""}`,
		`{"time":"2026-09-16T06:01:51-04:00","level":"INFO","msg":"Prods listening","address":"127.0.0.1:18559","mode":"installer","data_dir":"/tmp/prods/data","backup_dir":"/tmp/prods/backups"}`,
	}, nil, nil)
	m.width, m.height = 100, 28
	rendered := m.View()
	if height := lipgloss.Height(rendered); height > m.height {
		t.Fatalf("rendered height=%d exceeds terminal height=%d", height, m.height)
	}
	if !strings.Contains(rendered, ansi.SetHyperlink("http://127.0.0.1:8080/install?token=test")) {
		t.Fatal("installer URL was not rendered as an OSC 8 hyperlink")
	}
	view := ansi.Strip(rendered)
	lines := strings.Split(view, "\n")
	if strings.TrimSpace(lines[0]) != "Prods" {
		t.Fatalf("first product line = %q", lines[0])
	}
	for _, expected := range []string{"服務狀態與操作指引", "等待安裝", "http://127.0.0.1:8080/install", "Admin：http://127.0.0.1:8080/admin", `C:\\Prods\\data`, "Log", "Prods listening"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("view missing %q:\n%s", expected, view)
		}
	}
}

func TestLogPanelScrollsAndReturnsToTail(t *testing.T) {
	logs := make([]string, 0, 30)
	for index := 1; index <= 30; index++ {
		logs = append(logs, "log entry "+strings.Repeat("x", index))
	}
	m := newModel(Guide{Status: "正常執行"}, logs, nil, nil)
	m.width, m.height = 80, 18
	if height := lipgloss.Height(m.View()); height > m.height {
		t.Fatalf("rendered log height=%d exceeds terminal height=%d", height, m.height)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	scrolled := updated.(model)
	if scrolled.scrollFromBottom == 0 {
		t.Fatal("page up did not scroll log panel")
	}
	updated, _ = scrolled.Update(tea.KeyMsg{Type: tea.KeyEnd})
	if tail := updated.(model); tail.scrollFromBottom != 0 {
		t.Fatalf("end did not return to log tail: %d", tail.scrollFromBottom)
	}
}

func TestNonInteractiveSessionPreservesPlainJSONOutput(t *testing.T) {
	var output bytes.Buffer
	session := newSession(strings.NewReader(""), &output, false)
	body := []byte("{\"level\":\"INFO\",\"msg\":\"ready\"}\n")
	if _, err := session.Write(body); err != nil {
		t.Fatal(err)
	}
	if output.String() != string(body) {
		t.Fatalf("plain output = %q", output.String())
	}
}

func TestInteractiveSessionFlushesStartupErrorsWhenTUIWasNeverStarted(t *testing.T) {
	var output bytes.Buffer
	session := newSession(strings.NewReader(""), &output, true)
	body := []byte("{\"level\":\"ERROR\",\"msg\":\"startup failed\"}\n")
	if _, err := session.Write(body); err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 {
		t.Fatalf("interactive startup log was written before TUI decision: %q", output.String())
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if output.String() != string(body) {
		t.Fatalf("startup failure output = %q", output.String())
	}
}

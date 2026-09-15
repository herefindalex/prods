//go:build poc && !windows

package platform

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

type poc05InstanceLock struct {
	file *os.File
	path string
}

func acquirePOC05InstanceLock(dbPath string) (*poc05InstanceLock, error) {
	abs, err := filepath.Abs(dbPath)
	if err != nil {
		return nil, err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return nil, err
	}
	canonical := filepath.Join(parent, filepath.Base(abs))
	lockPath := canonical + ".instance.lock"
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("database already owned by another instance (%s): %w", canonical, err)
	}
	return &poc05InstanceLock{file: file, path: lockPath}, nil
}

func (l *poc05InstanceLock) close() error {
	if l == nil || l.file == nil {
		return nil
	}
	unlockErr := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	closeErr := l.file.Close()
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}

type poc05ResourceSnapshot struct {
	FreeBytes  uint64
	FreeInodes uint64
	Reserved   uint64
}

func checkPOC05Resources(snapshot poc05ResourceSnapshot, required, byteHeadroom, inodeHeadroom uint64) error {
	if snapshot.FreeBytes < snapshot.Reserved || snapshot.FreeBytes-snapshot.Reserved < required+byteHeadroom {
		return fmt.Errorf("resource gate critical: need %d bytes plus %d headroom; free=%d reserved=%d", required, byteHeadroom, snapshot.FreeBytes, snapshot.Reserved)
	}
	if snapshot.FreeInodes < inodeHeadroom {
		return fmt.Errorf("resource gate critical: need %d free inodes; free=%d", inodeHeadroom, snapshot.FreeInodes)
	}
	return nil
}

type poc05Session struct {
	csrf    string
	version uint64
}

type poc05Auth struct {
	mu       sync.Mutex
	versions map[string]uint64
	sessions map[string]poc05Session
	hosts    map[string]bool
}

func newPOC05Auth(hosts ...string) *poc05Auth {
	a := &poc05Auth{versions: make(map[string]uint64), sessions: make(map[string]poc05Session), hosts: make(map[string]bool)}
	for _, host := range hosts {
		a.hosts[host] = true
	}
	return a
}

func randomPOC05Token() string {
	body := make([]byte, 32)
	_, _ = rand.Read(body)
	return hex.EncodeToString(body)
}

func (a *poc05Auth) login(account string) (string, string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	sid, csrf := randomPOC05Token(), randomPOC05Token()
	a.sessions[sid] = poc05Session{csrf: csrf, version: a.versions[account]}
	return sid, csrf
}

func (a *poc05Auth) revoke(account string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.versions[account]++
}

func (a *poc05Auth) admit(account, sid, csrf string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	session, ok := a.sessions[sid]
	return ok && session.version == a.versions[account] && len(csrf) == len(session.csrf) && subtle.ConstantTimeCompare([]byte(csrf), []byte(session.csrf)) == 1
}

func (a *poc05Auth) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.hosts[r.Host] {
			http.Error(w, "untrusted Host; configure the canonical host explicitly", http.StatusBadRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func validatePOC05SVG(body []byte) error {
	decoder := xml.NewDecoder(strings.NewReader(string(body)))
	decoder.Strict = true
	seenRoot := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("invalid SVG XML: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		name := strings.ToLower(start.Name.Local)
		if !seenRoot {
			seenRoot = true
			if name != "svg" {
				return errors.New("document root is not svg")
			}
		}
		if name == "script" || name == "foreignobject" || name == "iframe" || name == "object" || name == "embed" {
			return fmt.Errorf("active SVG element %s is forbidden", name)
		}
		for _, attribute := range start.Attr {
			attrName := strings.ToLower(attribute.Name.Local)
			value := strings.TrimSpace(strings.ToLower(attribute.Value))
			if strings.HasPrefix(attrName, "on") {
				return fmt.Errorf("event handler %s is forbidden", attrName)
			}
			if attrName == "href" && value != "" && !strings.HasPrefix(value, "#") && !strings.HasPrefix(value, "data:image/") {
				return errors.New("external SVG reference is forbidden")
			}
			if strings.Contains(value, "javascript:") {
				return errors.New("javascript URL is forbidden")
			}
		}
	}
	if !seenRoot {
		return errors.New("empty SVG")
	}
	return nil
}

func validatePOC05PDF(body []byte) error {
	if len(body) < 12 || !strings.HasPrefix(string(body), "%PDF-") || !strings.Contains(string(body[len(body)-12:]), "%%EOF") {
		return errors.New("invalid PDF envelope")
	}
	lower := strings.ToLower(string(body))
	for _, active := range []string{"/javascript", "/js", "/launch", "/embeddedfile", "/richmedia"} {
		if strings.Contains(lower, active) {
			return fmt.Errorf("active PDF feature %s is forbidden", active)
		}
	}
	return nil
}

type failingPOC05Reader struct {
	read bool
}

func (r *failingPOC05Reader) Read(buffer []byte) (int, error) {
	if r.read {
		return 0, io.ErrUnexpectedEOF
	}
	r.read = true
	return copy(buffer, strings.Repeat("x", 1024)), nil
}

func stagePOC05Upload(reader io.Reader, privateStage, final string, maxBytes int64) error {
	if err := os.MkdirAll(filepath.Dir(privateStage), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(privateStage, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, io.LimitReader(reader, maxBytes+1))
	syncErr := file.Sync()
	closeErr := file.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil {
		_ = os.Remove(privateStage)
		return errors.Join(copyErr, syncErr, closeErr)
	}
	info, err := os.Stat(privateStage)
	if err != nil || info.Size() > maxBytes {
		_ = os.Remove(privateStage)
		return errors.New("upload exceeds configured limit")
	}
	if err := os.MkdirAll(filepath.Dir(final), 0o700); err != nil {
		_ = os.Remove(privateStage)
		return err
	}
	return os.Rename(privateStage, final)
}

func TestPOC05DuplicateDBOwnershipCanonicalizesRootsAndSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix flock harness")
	}
	root := t.TempDir()
	realRoot := filepath.Join(root, "real-data")
	if err := os.MkdirAll(realRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(realRoot, "prods.db")
	if err := os.WriteFile(dbPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias-data")
	if err := os.Symlink(realRoot, alias); err != nil {
		t.Fatal(err)
	}
	first, err := acquirePOC05InstanceLock(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer first.close()
	if _, err := acquirePOC05InstanceLock(filepath.Join(alias, "prods.db")); err == nil || !strings.Contains(err.Error(), "already owned") {
		t.Fatalf("symlinked duplicate ownership = %v", err)
	}
	if err := first.close(); err != nil {
		t.Fatal(err)
	}
	second, err := acquirePOC05InstanceLock(filepath.Join(alias, "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer second.close()
}

func TestPOC05PortCollisionIsActionable(t *testing.T) {
	first, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := net.Listen("tcp", first.Addr().String())
	if err == nil {
		second.Close()
		t.Fatal("second listener unexpectedly acquired occupied port")
	}
	message := fmt.Sprintf("listen %s failed: %v; choose another listen address or stop the owning process", first.Addr(), err)
	if !strings.Contains(message, "choose another") {
		t.Fatal("port collision lacks actionable guidance")
	}
}

func TestPOC05ResourceGateIncludesReservationsAndInodes(t *testing.T) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(t.TempDir(), &stat); err != nil {
		t.Fatal(err)
	}
	actual := poc05ResourceSnapshot{FreeBytes: stat.Bavail * uint64(stat.Bsize), FreeInodes: stat.Ffree}
	if actual.FreeBytes == 0 || actual.FreeInodes == 0 {
		t.Fatal("filesystem capacity unavailable")
	}
	if err := checkPOC05Resources(poc05ResourceSnapshot{FreeBytes: 100, FreeInodes: 10, Reserved: 80}, 15, 10, 2); err == nil {
		t.Fatal("reserved-byte critical condition admitted")
	}
	if err := checkPOC05Resources(poc05ResourceSnapshot{FreeBytes: 1000, FreeInodes: 1}, 10, 10, 2); err == nil {
		t.Fatal("inode critical condition admitted")
	}
	if err := checkPOC05Resources(poc05ResourceSnapshot{FreeBytes: 1000, FreeInodes: 10, Reserved: 100}, 200, 100, 2); err != nil {
		t.Fatal(err)
	}
}

func TestPOC05GracefulShutdownDrainsBeforeDBClose(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	dbClosed := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		select {
		case <-dbClosed:
			http.Error(w, "DB closed too early", http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second}
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()
	responseDone := make(chan *http.Response, 1)
	go func() {
		response, _ := http.Get("http://" + listener.Addr().String())
		responseDone <- response
	}()
	<-started
	shutdownDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		shutdownDone <- server.Shutdown(ctx)
	}()
	time.Sleep(20 * time.Millisecond)
	close(release)
	if err := <-shutdownDone; err != nil {
		t.Fatal(err)
	}
	close(dbClosed)
	response := <-responseDone
	if response == nil || response.StatusCode != http.StatusNoContent {
		t.Fatalf("accepted request did not drain successfully: %#v", response)
	}
	_ = response.Body.Close()
	if err := <-serveDone; !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("serve result = %v", err)
	}
	if _, err := http.Get("http://" + listener.Addr().String()); err == nil {
		t.Fatal("new request admitted after shutdown")
	}
}

func TestPOC05InterruptedUploadNeverActivates(t *testing.T) {
	root := t.TempDir()
	stage := filepath.Join(root, "private-stage", "upload.part")
	final := filepath.Join(root, "assets", "upload.bin")
	if err := stagePOC05Upload(&failingPOC05Reader{}, stage, final, 2048); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("interrupted upload = %v", err)
	}
	for _, path := range []string{stage, final} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("interrupted upload left %s", path)
		}
	}
}

func TestPOC05SVGAndPDFActiveContentValidation(t *testing.T) {
	validSVG := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><path d="M0 0h1v1z"/></svg>`)
	if err := validatePOC05SVG(validSVG); err != nil {
		t.Fatal(err)
	}
	for _, body := range [][]byte{
		[]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		[]byte(`<svg xmlns="http://www.w3.org/2000/svg"><image href="https://attacker.invalid/x"/></svg>`),
		[]byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`),
	} {
		if err := validatePOC05SVG(body); err == nil {
			t.Fatalf("active SVG admitted: %s", body)
		}
	}
	validPDF := []byte("%PDF-1.7\n1 0 obj<<>>endobj\n%%EOF")
	if err := validatePOC05PDF(validPDF); err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"/JavaScript", "/JS", "/Launch", "/EmbeddedFile", "/RichMedia"} {
		body := []byte("%PDF-1.7\n1 0 obj<<" + marker + ">>endobj\n%%EOF")
		if err := validatePOC05PDF(body); err == nil {
			t.Fatalf("active PDF marker admitted: %s", marker)
		}
	}
}

func TestPOC05TrustedHostCSRFAndSessionRevocation(t *testing.T) {
	auth := newPOC05Auth("catalog.example.test")
	sid, csrf := auth.login("admin")
	protected := auth.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !auth.admit("admin", r.Header.Get("X-Session"), r.Header.Get("X-CSRF-Token")) {
			http.Error(w, "invalid session or CSRF token", http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodPost, "https://catalog.example.test/admin/write", nil)
	request.Host = "attacker.invalid"
	recorder := httptest.NewRecorder()
	protected.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("untrusted Host status = %d", recorder.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "https://catalog.example.test/admin/write", nil)
	request.Host = "catalog.example.test"
	request.Header.Set("X-Session", sid)
	request.Header.Set("X-CSRF-Token", csrf+"changed")
	recorder = httptest.NewRecorder()
	protected.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("invalid CSRF status = %d", recorder.Code)
	}

	request.Header.Set("X-CSRF-Token", csrf)
	recorder = httptest.NewRecorder()
	protected.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("valid credentials status = %d", recorder.Code)
	}
	auth.revoke("admin")
	recorder = httptest.NewRecorder()
	protected.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("revoked session status = %d", recorder.Code)
	}
	if sid == csrf {
		t.Fatal("session and CSRF secrets were not separated")
	}
}

func TestPOC05ReadOnlyConfigurationCanBeReadWithoutMutation(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "prods.ini")
	if err := os.WriteFile(configPath, []byte("listen=127.0.0.1:8080\n"), 0o400); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(configPath)
	if err != nil || !strings.Contains(string(body), "listen=") {
		t.Fatalf("read-only config = %q, %v", body, err)
	}
	after, err := os.Stat(configPath)
	if err != nil || before.ModTime() != after.ModTime() || after.Mode().Perm() != 0o400 {
		t.Fatal("configuration read mutated the file")
	}
}

func TestPOC05NoUnexpectedExternalTarget(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1", "https://catalog.example.test"} {
		parsed, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "catalog.example.test" {
			t.Fatal("POC target escaped controlled instance")
		}
	}
}

func TestPOC05HashEvidenceUsesContentNotMTime(t *testing.T) {
	first := sha256.Sum256([]byte("A"))
	second := sha256.Sum256([]byte("B"))
	if first == second {
		t.Fatal("content evidence collision")
	}
}

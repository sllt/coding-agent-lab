package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sllt/agentlab/internal/bootstrap"
)

func TestCSPForbidsInlineScript(t *testing.T) {
	_, rt, _ := newAPI(t)
	rec := call(rt, http.MethodGet, "/api/v1/session", nil, "", "")
	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("no CSP header")
	}
	for _, part := range strings.Split(csp, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "script-src") && strings.Contains(part, "unsafe") {
			t.Fatalf("script-src allows unsafe: %q", csp)
		}
		if strings.HasPrefix(part, "connect-src") && strings.Contains(part, "ws:") {
			t.Fatalf("connect-src allows any ws: %q", csp)
		}
	}
}

func TestWebhookSignatureCoversTimestampAndDedupe(t *testing.T) {
	api, rt, _ := newAPI(t)
	secret, err := api.webhookSecret()
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"n":1}`)
	ts := nowTS()
	sig := Sign(secret, ts, "k1", raw)
	// A captured signature cannot be replayed under a new dedupe key.
	if rec := webhook(rt, raw, sig, "k2"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("swapped dedupe %d %s", rec.Code, rec.Body.String())
	}
	// A signed but stale request is rejected.
	old := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	if rec := signedWebhook(rt, raw, Sign(secret, old, "k3", raw), "k3", old); rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "stale") {
		t.Fatalf("stale %d %s", rec.Code, rec.Body.String())
	}
	// Missing timestamp is rejected even when signed over the empty string.
	if rec := signedWebhook(rt, raw, Sign(secret, "", "k4", raw), "k4", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing ts %d %s", rec.Code, rec.Body.String())
	}
	if rec := signedWebhook(rt, raw, sig, "k1", ts); rec.Code != http.StatusOK {
		t.Fatalf("good %d %s", rec.Code, rec.Body.String())
	}
}

func signedWebhook(rt *bootstrap.Runtime, body []byte, sig, dedupe, ts string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks", bytes.NewReader(body))
	req.Host = "127.0.0.1"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agentlab-Signature", sig)
	req.Header.Set("X-Agentlab-Dedupe", dedupe)
	if ts != "" {
		req.Header.Set("X-Agentlab-Timestamp", ts)
	}
	rec := httptest.NewRecorder()
	rt.Handler.ServeHTTP(rec, req)
	return rec
}

func TestLoginLockoutReturns429(t *testing.T) {
	_, rt, _ := newAPI(t)
	setupLogin(t, rt)
	var last int
	var retry string
	for i := 0; i < 8; i++ {
		rec := call(rt, http.MethodPost, "/api/v1/login", map[string]string{"username": "malong", "password": "wrong"}, "", "")
		last, retry = rec.Code, rec.Header().Get("Retry-After")
		if last == http.StatusTooManyRequests {
			break
		}
		if last != http.StatusUnauthorized {
			t.Fatalf("attempt %d: %d %s", i, last, rec.Body.String())
		}
	}
	if last != http.StatusTooManyRequests || retry == "" {
		t.Fatalf("no lockout: %d retry=%q", last, retry)
	}
	// Even the right password is refused while locked.
	rec := call(rt, http.MethodPost, "/api/v1/login", map[string]string{"username": "malong", "password": "local-pass"}, "", "")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("locked login with correct password: %d", rec.Code)
	}
}

func TestRestrictedNetworkIsRefused(t *testing.T) {
	_, rt, _ := newAPI(t)
	token, csrf := setupLogin(t, rt)
	for _, network := range []string{"restricted", "offline"} {
		rec := call(rt, http.MethodPost, "/api/v1/environments", map[string]any{"name": "e-" + network, "network": network}, token, csrf)
		if rec.Code != 422 {
			t.Fatalf("%s: %d %s", network, rec.Code, rec.Body.String())
		}
	}
}

func TestConcurrentBackupIs409AndKeepsOperatorMaintenance(t *testing.T) {
	_, rt, svc := newAPI(t)
	token, csrf := setupLogin(t, rt)
	release, err := svc.BeginBackup()
	if err != nil {
		t.Fatal(err)
	}
	rec := call(rt, http.MethodPost, "/api/v1/maintenance/backup", map[string]any{}, token, csrf)
	release()
	if rec.Code != http.StatusConflict {
		t.Fatalf("concurrent backup %d %s", rec.Code, rec.Body.String())
	}
	svc.SetMaintenance(true)
	release, err = svc.BeginBackup()
	if err != nil {
		t.Fatal(err)
	}
	release()
	if !svc.Maintenance() {
		t.Fatal("backup cleared operator maintenance")
	}
	svc.SetMaintenance(false)
}

func TestEventTailOnlyEmitsCompleteLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.ndjson")
	if err := os.WriteFile(path, []byte(`{"sequence":1,"type":"a"}`+"\n"+`{"sequence":2,"ty`), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := writeEvents(context.Background(), &buf, "att", path, 0, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "att:1") || strings.Contains(buf.String(), `"ty`+"\n") || strings.Contains(buf.String(), "att:0") {
		t.Fatalf("partial line emitted: %q", buf.String())
	}
	tail := &eventTail{path: path}
	first, _ := tail.next()
	if len(first) != 1 {
		t.Fatalf("first read %q", first)
	}
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	_, _ = f.WriteString(`pe":"b"}` + "\n")
	_ = f.Close()
	second, _ := tail.next()
	if len(second) != 1 || eventSeq(second[0]) != 2 {
		t.Fatalf("completed line lost: %q", second)
	}
}

func TestRedactText(t *testing.T) {
	in := `{"msg":"export OPENAI_API_KEY=sk-abcdefghijklmnopqrstu and Bearer abcdefghijklmnopqrstuv ghp_abcdefghijklmnopqrstuvwxyz0123"}`
	out := redactText(in)
	for _, leak := range []string{"sk-abcdefghijklmnopqrstu", "abcdefghijklmnopqrstuv", "ghp_abcdefghijklmnopqrstuvwxyz0123"} {
		if strings.Contains(out, leak) {
			t.Fatalf("leak %q in %s", leak, out)
		}
	}
	if !json.Valid([]byte(out)) {
		t.Fatalf("redaction broke JSON: %s", out)
	}
	if got := dataRel("/srv/lab", "/srv/lab/backups/x.db"); got != "$DATA/backups/x.db" {
		t.Fatal(got)
	}
	if got := dataRel("/srv/lab", "/etc/passwd"); got != "passwd" {
		t.Fatal(got)
	}
}

func TestWebhookRejectsForeignNotifyURL(t *testing.T) {
	api, rt, _ := newAPI(t)
	secret, err := api.webhookSecret()
	if err != nil {
		t.Fatal(err)
	}
	for i, target := range []string{"http://169.254.169.254:80/x", "http://127.0.0.1@evil.example:80/", "file:///etc/passwd", "https://example.com/hook"} {
		raw := []byte(`{"notify_url":"` + target + `"}`)
		key := "n" + strconv.Itoa(i)
		ts := nowTS()
		rec := signedWebhook(rt, raw, Sign(secret, ts, key, raw), key, ts)
		if rec.Code != 422 {
			t.Fatalf("%s accepted: %d %s", target, rec.Code, rec.Body.String())
		}
	}
}

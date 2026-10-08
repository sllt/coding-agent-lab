package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNonLoopbackNotificationsFail(t *testing.T) {
	svc := newLab(t)
	ctx := context.Background()
	targets := []string{
		"file:///tmp/hook",
		"https://example.com/hook",
		"http://8.8.8.8/hook",
		"https://127.0.0.1:9/hook",
	}
	for _, target := range targets {
		if err := svc.Store.InsertNotification(ctx, "webhook_result", target, `{}`); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.Store.InsertNotification(ctx, "webhook_result", "local", `{}`); err != nil {
		t.Fatal(err)
	}
	if err := svc.Store.InsertNotification(ctx, "webhook_result", "", `{}`); err != nil {
		t.Fatal(err)
	}
	svc.RetryNotifications(ctx)
	rows, err := svc.Store.DB().Query(`SELECT target, state, last_error FROM notifications ORDER BY target`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := map[string]string{}
	for rows.Next() {
		var target, state, last string
		if err := rows.Scan(&target, &state, &last); err != nil {
			t.Fatal(err)
		}
		seen[target] = state
		if target == "" || target == "local" {
			if state != "delivered" {
				t.Fatalf("no remote target %q state %s", target, state)
			}
			continue
		}
		if state != "failed" {
			t.Fatalf("%s state %s error %s", target, state, last)
		}
		if last == "" {
			t.Fatalf("%s failed without a reason", target)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if seen[target] != "failed" {
			t.Fatalf("missing failure for %s (%q)", target, seen[target])
		}
	}
}

func TestLoopbackNotificationDeliversOnlyAfterAttempt(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	svc := newLab(t)
	ctx := context.Background()
	if err := svc.Store.InsertNotification(ctx, "webhook_result", srv.URL+"/hook", `{"ok":true}`); err != nil {
		t.Fatal(err)
	}
	if err := svc.Store.InsertNotification(ctx, "webhook_result", "http://127.0.0.1:1/missing", `{}`); err != nil {
		t.Fatal(err)
	}
	svc.RetryNotifications(ctx)
	if hits != 1 {
		t.Fatalf("loopback attempts %d", hits)
	}
	var delivered, pending int
	rows, err := svc.Store.DB().Query(`SELECT target, state FROM notifications`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var target, state string
		if err := rows.Scan(&target, &state); err != nil {
			t.Fatal(err)
		}
		switch state {
		case "delivered":
			delivered++
			if target != srv.URL+"/hook" {
				t.Fatalf("delivered %s", target)
			}
		case "pending":
			pending++
			if target != "http://127.0.0.1:1/missing" {
				t.Fatalf("pending %s", target)
			}
		default:
			t.Fatalf("%s %s", target, state)
		}
	}
	if delivered != 1 || pending != 1 {
		t.Fatalf("delivered %d pending %d", delivered, pending)
	}
}

func TestNotifyURLParsingRejectsUserinfoTricks(t *testing.T) {
	for _, bad := range []string{
		"http://127.0.0.1@evil.example/hook",
		"http://127.0.0.1:80@evil.example:80/hook",
		"http://localhost:1@169.254.169.254/latest",
		"http://user:pass@127.0.0.1:9000/hook",
		"http://127.0.0.1.evil.example:9000/hook",
		"http://[::ffff:8.8.8.8]:9000/hook",
		"http://127.0.0.1/hook",
		"gopher://127.0.0.1:70/",
		"http:127.0.0.1:9000",
	} {
		if _, err := ValidateNotifyURL(bad); err == nil {
			t.Fatalf("%s was accepted", bad)
		}
	}
	for _, good := range []string{"http://127.0.0.1:9000/hook", "http://localhost:8080/x", "http://[::1]:9000/y"} {
		if _, err := ValidateNotifyURL(good); err != nil {
			t.Fatalf("%s rejected: %v", good, err)
		}
	}
}

func TestNotifyDoesNotFollowRedirects(t *testing.T) {
	hit := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	svc := newLab(t)
	ctx := context.Background()
	if err := svc.Store.InsertNotification(ctx, "webhook_result", redirect.URL+"/hook", `{}`); err != nil {
		t.Fatal(err)
	}
	svc.RetryNotifications(ctx)
	var state string
	if err := svc.Store.DB().QueryRow(`SELECT state FROM notifications`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if hit || state != "failed" {
		t.Fatalf("redirect followed=%v state=%s", hit, state)
	}
}

package bootstrap

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sllt/pi/pkg/pi"

	"github.com/sllt/agentlab/internal/version"
)

func TestPiModuleIsPinnedCommit(t *testing.T) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		t.Fatal("no build info")
	}
	var found string
	for _, m := range info.Deps {
		if m.Path == "github.com/sllt/pi" {
			found = m.Version
		}
	}
	if found == "" {
		t.Fatal("github.com/sllt/pi is not linked")
	}
	if !strings.Contains(found, version.PiCommit[:12]) {
		t.Fatalf("pi version %s does not contain reviewed commit %s", found, version.PiCommit)
	}
}

func TestHTTPHandlerAndStreamDoNotUseInventedAPI(t *testing.T) {
	rt, err := Build(Config{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Host = "127.0.0.1"
	rec := httptest.NewRecorder()
	rt.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"service":"agentlab"`) {
		t.Fatalf("body %s", rec.Body.String())
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatal("missing request id")
	}

	sreq := httptest.NewRequest(http.MethodGet, "/api/v1/health/stream", nil)
	sreq.Host = "127.0.0.1"
	srec := httptest.NewRecorder()
	rt.Handler.ServeHTTP(srec, sreq)
	body := srec.Body.String()
	if !strings.Contains(body, "event: health") {
		t.Fatalf("sse body %s", body)
	}
	if strings.Contains(body, `"code":`) {
		t.Fatalf("stream appended a JSON error envelope: %s", body)
	}
}

func TestRejectsNonLoopbackAndForeignOrigin(t *testing.T) {
	if _, err := Build(Config{Addr: "0.0.0.0:43117"}); err == nil {
		t.Fatal("expected loopback rejection")
	}
	rt, err := Build(Config{Addr: "127.0.0.1:43117"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Host = "evil.example"
	rec := httptest.NewRecorder()
	rt.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("host status %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Host = "127.0.0.1:43117"
	req.Header.Set("Origin", "http://evil.example")
	rec = httptest.NewRecorder()
	rt.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("origin status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestResourceStartStopOnceAndFailedStartRollsBack(t *testing.T) {
	var starts, stops atomic.Int32
	rt, err := Build(Config{
		Addr: "127.0.0.1:0",
		ExtraResources: []pi.Resource{{
			Name:      "probe",
			Ownership: pi.Owned,
			Start: func(context.Context) error {
				starts.Add(1)
				return nil
			},
			Stop: func(context.Context) error {
				stops.Add(1)
				return nil
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rt.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if starts.Load() != 1 {
		t.Fatalf("starts %d", starts.Load())
	}
	if err := rt.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if stops.Load() != 1 {
		t.Fatalf("stops %d", stops.Load())
	}
	if err := rt.Start(context.Background()); err == nil {
		t.Fatal("stopped app must not start again")
	}

	var okStarts, okStops atomic.Int32
	rt2, err := Build(Config{
		Addr: "127.0.0.1:0",
		ExtraResources: []pi.Resource{
			{
				Name:      "ok",
				Ownership: pi.Owned,
				Start: func(context.Context) error {
					okStarts.Add(1)
					return nil
				},
				Stop: func(context.Context) error {
					okStops.Add(1)
					return nil
				},
			},
			{
				Name:      "boom",
				Ownership: pi.Owned,
				Start:     func(context.Context) error { return errors.New("boom") },
				Stop:      func(context.Context) error { return nil },
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = rt2.Start(context.Background())
	if err == nil {
		t.Fatal("expected start failure")
	}
	if okStarts.Load() != 1 || okStops.Load() != 1 {
		t.Fatalf("partial start starts=%d stops=%d", okStarts.Load(), okStops.Load())
	}
}

func TestTrialFailureDoesNotStopApplication(t *testing.T) {
	rt, err := Build(Config{
		Addr: "127.0.0.1:0",
		Worker: func(ctx context.Context) error {
			// A trial failure is recorded by the worker and is not returned.
			_ = io.EOF
			<-ctx.Done()
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if err := rt.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestBorrowedResourceIsNotStarted(t *testing.T) {
	var starts atomic.Int32
	rt, err := Build(Config{
		Addr: "127.0.0.1:0",
		ExtraResources: []pi.Resource{{
			Name:      "borrowed",
			Ownership: pi.Borrowed,
			Start: func(context.Context) error {
				starts.Add(1)
				return nil
			},
			Stop: func(context.Context) error {
				starts.Add(10)
				return nil
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := rt.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if starts.Load() != 0 {
		t.Fatalf("borrowed hooks ran: %d", starts.Load())
	}
}

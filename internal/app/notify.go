package app

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"
)

// RetryNotifications delivers pending result notices. Only loopback URLs are
// contacted. A local target is recorded as delivered without pretending a
// remote server answered.
func (s *Service) RetryNotifications(ctx context.Context) {
	items, err := s.Store.DueNotifications(ctx)
	if err != nil {
		return
	}
	client := &http.Client{Timeout: 2 * time.Second}
	for _, item := range items {
		attempts := item.Attempts + 1
		if item.Target == "" || item.Target == "local" {
			_ = s.Store.FinishNotification(ctx, item.ID, "delivered", "", attempts)
			continue
		}
		if !strings.HasPrefix(item.Target, "http://127.0.0.1:") && !strings.HasPrefix(item.Target, "http://localhost:") {
			_ = s.Store.FinishNotification(ctx, item.ID, "refused", "notify host is not loopback", attempts)
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, item.Target, bytes.NewReader([]byte(item.Body)))
		if err != nil {
			_ = s.Store.FinishNotification(ctx, item.ID, "pending", err.Error(), attempts)
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			state := "pending"
			if attempts >= 3 {
				state = "failed"
			}
			_ = s.Store.FinishNotification(ctx, item.ID, state, err.Error(), attempts)
			continue
		}
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
		if res.StatusCode >= 200 && res.StatusCode < 300 {
			_ = s.Store.FinishNotification(ctx, item.ID, "delivered", "", attempts)
			continue
		}
		state := "pending"
		if attempts >= 3 {
			state = "failed"
		}
		_ = s.Store.FinishNotification(ctx, item.ID, state, res.Status, attempts)
	}
}

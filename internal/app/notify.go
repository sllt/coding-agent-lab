package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// ValidateNotifyURL accepts only plain http to a loopback host. It parses the
// URL instead of matching a prefix: "http://127.0.0.1:80@evil.example/" starts
// with the loopback prefix but its host is evil.example.
func ValidateNotifyURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("notify url is not a url")
	}
	if u.Scheme != "http" {
		return nil, fmt.Errorf("notify url must be http to a loopback host")
	}
	if u.User != nil {
		return nil, fmt.Errorf("notify url must not carry userinfo")
	}
	if u.Opaque != "" || u.Host == "" {
		return nil, fmt.Errorf("notify url has no host")
	}
	if !loopbackHost(u.Hostname()) {
		return nil, fmt.Errorf("notify host is not loopback")
	}
	if u.Port() == "" {
		return nil, fmt.Errorf("notify url needs an explicit port")
	}
	return u, nil
}

func loopbackHost(host string) bool {
	switch strings.ToLower(host) {
	case "localhost":
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// notifyClient never follows redirects and refuses to connect to anything
// but a loopback address, even if "localhost" resolved elsewhere.
func notifyClient() *http.Client {
	dialer := &net.Dialer{
		Timeout: 2 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
				return errors.New("notify dial target is not loopback")
			}
			return nil
		},
	}
	return &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			Proxy:             nil,
			DialContext:       dialer.DialContext,
			DisableKeepAlives: true,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// RetryNotifications delivers pending result notices. Only http loopback
// URLs are contacted, and they are marked delivered only after that request
// succeeds with a 2xx (a redirect is not success). Every other address is a
// failure: the remote is not pretended to have received the notice. An empty
// or local target has no remote.
func (s *Service) RetryNotifications(ctx context.Context) {
	items, err := s.Store.DueNotifications(ctx)
	if err != nil {
		return
	}
	client := notifyClient()
	for _, item := range items {
		attempts := item.Attempts + 1
		if item.Target == "" || item.Target == "local" {
			_ = s.Store.FinishNotification(ctx, item.ID, "delivered", "", attempts)
			continue
		}
		u, err := ValidateNotifyURL(item.Target)
		if err != nil {
			_ = s.Store.FinishNotification(ctx, item.ID, "failed", err.Error(), attempts)
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader([]byte(item.Body)))
		if err != nil {
			_ = s.Store.FinishNotification(ctx, item.ID, "failed", "bad request", attempts)
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			state := "pending"
			if attempts >= 3 {
				state = "failed"
			}
			_ = s.Store.FinishNotification(ctx, item.ID, state, "delivery failed", attempts)
			continue
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
		_ = res.Body.Close()
		if res.StatusCode >= 200 && res.StatusCode < 300 {
			_ = s.Store.FinishNotification(ctx, item.ID, "delivered", "", attempts)
			continue
		}
		state := "pending"
		if attempts >= 3 || (res.StatusCode >= 300 && res.StatusCode < 400) {
			state = "failed"
		}
		_ = s.Store.FinishNotification(ctx, item.ID, state, res.Status, attempts)
	}
}

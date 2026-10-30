// Package jira implements the Jira Cloud REST API used by TaskSamurai.
package jira

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/snonux/tasksamurai/internal/config"
)

const maxResponseBytes = 4 << 20

// Client owns credentials and may be used concurrently by async commands.
type Client struct {
	site     string
	email    string
	settings config.Jira
	http     *http.Client
	logger   *slog.Logger
	// The live integration harness lowers this to exercise real pagination
	// without creating hundreds of disposable issues.
	searchPageSize int
	mu             sync.RWMutex
	token          string
}

// APIError reports a bounded, credential-redacted Jira diagnostic.
type APIError struct {
	StatusCode int
	Message    string
}

// New loads credentials. A supplied HTTP client permits an HTTP test origin;
// production callers pass nil and use the HTTPS origin validated by config.
func New(ctx context.Context, settings config.Jira, httpClient *http.Client) (*Client, error) {
	origin, err := url.Parse(settings.Site)
	if err != nil || origin.Hostname() == "" || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.ForceQuery || strings.Contains(settings.Site, "#") ||
		(origin.Scheme != "https" && !(origin.Scheme == "http" && httpClient != nil)) {
		return nil, errors.New("Jira: invalid site origin")
	}
	if settings.Timeout <= 0 || settings.MaxResults <= 0 || settings.Email == "" {
		return nil, errors.New("Jira: email, positive timeout and result limit required")
	}
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	copyClient := *httpClient
	// Disable redirects, including same-origin POST rewrites. The caller's
	// transport remains injectable, but credentials never follow a Location.
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	c := &Client{site: settings.Site, email: settings.Email, settings: settings, http: &copyClient, logger: slog.Default(), searchPageSize: 100}
	if err := c.ReloadToken(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

// Error returns the safe diagnostic.
func (e *APIError) Error() string { return e.Message }

// String prevents diagnostic formatting from exposing credentials.
func (c *Client) String() string { return "jira.Client{credentials:redacted}" }

// GoString also redacts Go-syntax formatting in debug dumps.
func (c *Client) GoString() string { return c.String() }

// ReloadToken rereads the credential file; failed reads retain the old token.
// Callers must abort that refresh on error rather than use stale credentials.
func (c *Client) ReloadToken(ctx context.Context) error {
	token, err := c.settings.ReadToken(ctx)
	if err != nil {
		return fmt.Errorf("Jira: reload credentials: %w", err)
	}
	c.mu.Lock()
	c.token = token
	c.mu.Unlock()
	return nil
}

func (c *Client) request(ctx context.Context, method, path string, body, out any, readOnly bool) error {
	ctx, cancel := context.WithTimeout(ctx, c.settings.Timeout)
	defer cancel()
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			return errors.New("Jira: could not encode request")
		}
	}
	c.mu.RLock()
	token := c.token
	c.mu.RUnlock()
	for attempt := 0; ; attempt++ {
		payload, status, retryAfter, err := c.exchange(ctx, method, path, data, token)
		if err != nil {
			return err
		}
		if readOnly && attempt == 0 && (status == 429 || status >= 500) {
			if err := waitRetry(ctx, retryAfter); err != nil {
				return fmt.Errorf("Jira: retry canceled: %w", err)
			}
			continue
		}
		if status < 200 || status >= 300 {
			return c.responseError(status, payload, token)
		}
		if out != nil {
			if len(payload) == 0 || bytes.Equal(bytes.TrimSpace(payload), []byte("null")) || json.Unmarshal(payload, out) != nil {
				return &APIError{status, fmt.Sprintf("unexpected response from Jira (HTTP %d)", status)}
			}
		} else if len(bytes.TrimSpace(payload)) > 0 && !json.Valid(payload) {
			return &APIError{status, fmt.Sprintf("unexpected response from Jira (HTTP %d)", status)}
		}
		return nil
	}
}

func (c *Client) exchange(ctx context.Context, method, path string, data []byte, token string) ([]byte, int, string, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.site+path, bytes.NewReader(data))
	if err != nil {
		return nil, 0, "", errors.New("Jira: invalid request")
	}
	req.SetBasicAuth(c.email, token)
	req.Header.Set("Accept", "application/json")
	if data != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		// Transport errors can contain URLs or arbitrary transport text. Preserve
		// cancellation semantics without echoing their potentially sensitive data.
		if ctx.Err() != nil {
			return nil, 0, "", fmt.Errorf("Jira request: %w", ctx.Err())
		}
		return nil, 0, "", errors.New("Jira: request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, 0, "", errors.New("Jira: could not read response")
	}
	if len(payload) > maxResponseBytes {
		return nil, 0, "", errors.New("Jira: response exceeds size limit")
	}
	return payload, resp.StatusCode, resp.Header.Get("Retry-After"), nil
}

func (c *Client) responseError(status int, data []byte, token string) error {
	message := fmt.Sprintf("Jira: HTTP %d", status)
	switch status {
	case 401:
		message = "Jira: wrong email or truncated token (the =… suffix is part of the token)"
	case 403:
		message = "Jira: no permission for this Jira operation"
	case 404:
		message = "Jira issue not found"
	default:
		var response struct {
			Messages []string          `json:"errorMessages"`
			Errors   map[string]string `json:"errors"`
		}
		if json.Unmarshal(data, &response) != nil {
			return &APIError{status, fmt.Sprintf("unexpected response from Jira (HTTP %d)", status)}
		}
		if status == 400 && strings.Contains(strings.ToLower(string(data)), "unbounded") {
			message = "this Jira site requires a restricted filter — press F to pick one"
		} else if len(response.Messages) > 0 {
			message += ": " + strings.Join(response.Messages, "; ")
		} else if len(response.Errors) > 0 {
			keys := make([]string, 0, len(response.Errors))
			for key := range response.Errors {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				message += ": " + key + ": " + response.Errors[key]
			}
		}
	}
	message = redact(message, c.email, token)
	// Cap only after redaction so truncation cannot leak a partial credential.
	if len(message) > 1024 {
		message = message[:1024] + "…"
	}
	return &APIError{status, message}
}

func redact(message, email, token string) string {
	for _, secret := range []string{token, base64.StdEncoding.EncodeToString([]byte(email + ":" + token))} {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[redacted]")
		}
	}
	return message
}

func waitRetry(ctx context.Context, value string) error {
	delay := 100 * time.Millisecond
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		delay = time.Duration(min(seconds, 10)) * time.Second
	} else if when, err := http.ParseTime(value); err == nil {
		delay = max(0, min(time.Until(when), 10*time.Second))
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

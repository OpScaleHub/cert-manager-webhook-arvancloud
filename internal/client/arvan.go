// Package client implements a minimal REST client for the ArvanCloud
// CDN/DNS API v4 (https://napi.arvancloud.ir/cdn/4.0), scoped to the
// operations the cert-manager DNS-01 solver needs: list, create and delete
// TXT records.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/OpScaleHub/cert-manager-webhook-arvancloud/internal/obs"
)

// DefaultBaseURL is the ArvanCloud API v4 base endpoint.
const DefaultBaseURL = "https://napi.arvancloud.ir"

// DefaultTimeout bounds every individual HTTP call so a slow ArvanCloud
// response can never stall a cert-manager queue worker indefinitely.
const DefaultTimeout = 15 * time.Second

const (
	maxRetries    = 3
	retryBackoff  = 500 * time.Millisecond
	cdnAPIVersion = "cdn/4.0"
)

// sharedTransport is a process-wide, connection-pooled transport. Every
// Client reuses it (unless one is injected via WithHTTPClient) so that
// high-density clusters issuing many concurrent challenges do not each open
// their own connection pool and exhaust ephemeral ports.
var sharedTransport = &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          100,
	MaxIdleConnsPerHost:   10,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
}

// ErrRecordNotFound is returned by DeleteRecord when the target record no
// longer exists. Callers treating deletion as idempotent should ignore it.
var ErrRecordNotFound = errors.New("arvancloud: dns record not found")

// Client is a concurrency-safe ArvanCloud DNS API client.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// Option customises a Client.
type Option func(*Client)

// WithBaseURL overrides the API base URL (useful for tests).
func WithBaseURL(raw string) Option {
	return func(c *Client) {
		if raw != "" {
			c.baseURL = strings.TrimRight(raw, "/")
		}
	}
}

// WithHTTPClient injects a custom *http.Client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		if hc != nil {
			c.httpClient = hc
		}
	}
}

// New builds a Client. The apiKey is the ArvanCloud Machine User key, passed
// verbatim in the "Authorization: Apikey <key>" header.
func New(apiKey string, opts ...Option) (*Client, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("arvancloud: api key must not be empty")
	}
	c := &Client{
		apiKey:     strings.TrimSpace(apiKey),
		baseURL:    DefaultBaseURL,
		httpClient: &http.Client{Timeout: DefaultTimeout, Transport: sharedTransport},
	}
	for _, o := range opts {
		o(c)
	}
	return c, nil
}

// FindTXTRecords returns every TXT record in domain whose name matches the
// given relative name (e.g. "_acme-challenge.www").
func (c *Client) FindTXTRecords(ctx context.Context, domain, name string) ([]Record, error) {
	name = normaliseName(name, domain)
	var matches []Record
	page := 1
	for {
		path := fmt.Sprintf("%s/domains/%s/dns-records?type=txt&per_page=100&page=%d",
			cdnAPIVersion, url.PathEscape(domain), page)

		var out listRecordsResponse
		if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
			return nil, err
		}
		for _, r := range out.Data {
			if strings.EqualFold(r.Type, RecordTypeTXT) && strings.EqualFold(r.Name, name) {
				matches = append(matches, r)
			}
		}
		if out.Meta.LastPage <= page || len(out.Data) == 0 {
			break
		}
		page++
	}
	return matches, nil
}

// CreateTXTRecord creates a TXT record and returns its ID. If an identical
// record (same name and value) already exists its ID is returned unchanged,
// making Present idempotent.
func (c *Client) CreateTXTRecord(ctx context.Context, domain, name, value string, ttl int) (string, error) {
	name = normaliseName(name, domain)

	existing, err := c.FindTXTRecords(ctx, domain, name)
	if err != nil {
		return "", err
	}
	for _, r := range existing {
		if r.Value.Text == value {
			return r.ID, nil
		}
	}

	body := createRecordRequest{
		Type:  RecordTypeTXT,
		Name:  name,
		Value: txtValue{Text: value},
		TTL:   ttl,
	}
	path := fmt.Sprintf("%s/domains/%s/dns-records", cdnAPIVersion, url.PathEscape(domain))

	var out struct {
		Data Record `json:"data"`
	}
	if err := c.do(ctx, http.MethodPost, path, body, &out); err != nil {
		return "", err
	}
	return out.Data.ID, nil
}

// DeleteRecord removes a record by ID. A 404 from the API is surfaced as
// ErrRecordNotFound so callers can implement idempotent cleanup.
func (c *Client) DeleteRecord(ctx context.Context, domain, id string) error {
	if id == "" {
		return ErrRecordNotFound
	}
	path := fmt.Sprintf("%s/domains/%s/dns-records/%s",
		cdnAPIVersion, url.PathEscape(domain), url.PathEscape(id))
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

// do performs a single API request with bounded retries on transient errors.
func (c *Client) do(ctx context.Context, method, path string, reqBody, out any) error {
	var payload []byte
	if reqBody != nil {
		var err error
		if payload, err = json.Marshal(reqBody); err != nil {
			return fmt.Errorf("arvancloud: encoding request: %w", err)
		}
	}

	endpoint := c.baseURL + "/" + strings.TrimLeft(path, "/")

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryBackoff * time.Duration(attempt-1)):
			}
		}

		var reader io.Reader
		if payload != nil {
			reader = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
		if err != nil {
			return fmt.Errorf("arvancloud: building request: %w", err)
		}
		req.Header.Set("Authorization", "Apikey "+c.apiKey)
		req.Header.Set("Accept", "application/json")
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		attemptStart := time.Now()
		resp, err := c.httpClient.Do(req)
		if err != nil {
			obs.ObserveAPIRequest(method, 0, err, attemptStart)
			lastErr = fmt.Errorf("arvancloud: %s %s: %w", method, path, err)
			continue // network error: retry
		}
		obs.ObserveAPIRequest(method, resp.StatusCode, nil, attemptStart)

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("arvancloud: reading response: %w", readErr)
			continue
		}

		switch {
		case resp.StatusCode == http.StatusNotFound:
			return ErrRecordNotFound
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			lastErr = fmt.Errorf("arvancloud: %s %s: %s", method, path, apiErrorText(body, resp.StatusCode))
			continue // transient: retry
		case resp.StatusCode >= 400:
			return fmt.Errorf("arvancloud: %s %s: %s", method, path, apiErrorText(body, resp.StatusCode))
		}

		if out != nil && len(body) > 0 {
			if err := json.Unmarshal(body, out); err != nil {
				return fmt.Errorf("arvancloud: decoding response: %w", err)
			}
		}
		return nil
	}
	return lastErr
}

// UnmarshalJSON accepts both {"text": "..."} and a bare "..." string.
func (v *RecordValue) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		v.Text = s
		return nil
	}
	var obj struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	v.Text = obj.Text
	return nil
}

// normaliseName converts an FQDN-ish name into the zone-relative form
// ArvanCloud expects: it strips a trailing dot and a trailing ".<domain>".
func normaliseName(name, domain string) string {
	name = strings.TrimSuffix(strings.TrimSpace(name), ".")
	domain = strings.TrimSuffix(strings.TrimSpace(domain), ".")
	if name == domain {
		return "@"
	}
	if suffix := "." + domain; strings.HasSuffix(name, suffix) {
		name = strings.TrimSuffix(name, suffix)
	}
	if name == "" {
		return "@"
	}
	return name
}

func apiErrorText(body []byte, status int) string {
	var e apiError
	if err := json.Unmarshal(body, &e); err == nil && e.Message != "" {
		if len(e.Errors) > 0 {
			parts := make([]string, 0, len(e.Errors))
			for field, msgs := range e.Errors {
				parts = append(parts, field+": "+strings.Join(msgs, ", "))
			}
			return fmt.Sprintf("HTTP %d: %s (%s)", status, e.Message, strings.Join(parts, "; "))
		}
		return fmt.Sprintf("HTTP %d: %s", status, e.Message)
	}
	return "HTTP " + strconv.Itoa(status)
}

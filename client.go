// Copyright © 2026 Michael Shields
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package main implements the unifi-sync CLI tool.
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
)

type client struct {
	http      *http.Client
	baseURL   string
	csrfToken string
	mu        sync.RWMutex
}

func newClient(baseURL string, insecure bool) *client {
	jar, _ := cookiejar.New(nil) //nolint:errcheck // error is documented as always nil
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure},
		Proxy:           http.ProxyFromEnvironment,
	}
	return &client{
		http: &http.Client{
			Jar:       jar,
			Transport: transport,
			Timeout:   30 * time.Second,
		},
		baseURL: strings.TrimRight(baseURL, "/"),
	}
}

// drainBody reads the response body to completion so the connection can be reused.
func drainBody(resp *http.Response) {
	_, _ = io.Copy(io.Discard, resp.Body) //nolint:errcheck // best-effort drain
	_ = resp.Body.Close()                 //nolint:errcheck // best-effort close
}

func (c *client) login(ctx context.Context, username, password string) error {
	body, _ := json.Marshal(map[string]string{ //nolint:errcheck,errchkjson // map[string]string cannot fail to marshal
		"username": username,
		"password": password,
	})
	loginURL := c.baseURL + "/api/login"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}
	defer drainBody(resp)
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body) //nolint:errcheck // best-effort read for error message
		return fmt.Errorf("login failed (status %d): %s", resp.StatusCode, msg)
	}
	if token := resp.Header.Get("X-Csrf-Token"); token != "" {
		c.mu.Lock()
		c.csrfToken = token
		c.mu.Unlock()
	}
	return nil
}

// doJSON builds an HTTP request with JSON body and CSRF token, executes it,
// and returns the response.
func (c *client) doJSON(ctx context.Context, method, reqURL string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.mu.RLock()
	token := c.csrfToken
	c.mu.RUnlock()
	if token != "" {
		req.Header.Set("X-Csrf-Token", token)
	}
	return c.http.Do(req)
}

func (c *client) list(ctx context.Context, site, resourceType string) ([]map[string]any, error) {
	u := fmt.Sprintf("%s/api/s/%s/rest/%s",
		c.baseURL, url.PathEscape(site), url.PathEscape(resourceType))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", resourceType, err)
	}
	defer drainBody(resp)
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body) //nolint:errcheck // best-effort read for error message
		return nil, fmt.Errorf("list %s (status %d): %s", resourceType, resp.StatusCode, msg)
	}
	return decodeDataEnvelope(resp.Body)
}

func (c *client) get(
	ctx context.Context, site, resourceType, id string, //nolint:unparam // site is part of the API contract
) (map[string]any, error) {
	u := fmt.Sprintf("%s/api/s/%s/rest/%s/%s",
		c.baseURL, url.PathEscape(site),
		url.PathEscape(resourceType), url.PathEscape(id))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get %s/%s: %w", resourceType, id, err)
	}
	defer drainBody(resp)
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body) //nolint:errcheck // best-effort read for error message
		return nil, fmt.Errorf("get %s/%s (status %d): %s", resourceType, id, resp.StatusCode, msg)
	}
	items, err := decodeDataEnvelope(resp.Body)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("get %s/%s: empty response", resourceType, id)
	}
	return items[0], nil
}

func (c *client) put(ctx context.Context, site, resourceType, id string, data map[string]any) error {
	body, err := json.Marshal(data)
	if err != nil {
		return err
	}
	u := fmt.Sprintf("%s/api/s/%s/rest/%s/%s",
		c.baseURL, url.PathEscape(site),
		url.PathEscape(resourceType), url.PathEscape(id))
	resp, err := c.doJSON(ctx, http.MethodPut, u, body)
	if err != nil {
		return fmt.Errorf("put %s/%s: %w", resourceType, id, err)
	}
	defer drainBody(resp)
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body) //nolint:errcheck // best-effort read for error message
		return fmt.Errorf("put %s/%s (status %d): %s", resourceType, id, resp.StatusCode, msg)
	}
	// Validate the response envelope to detect API-level errors.
	_, err = decodeDataEnvelope(resp.Body)
	return err
}

func (c *client) post(ctx context.Context, site, resourceType string, data map[string]any) (map[string]any, error) {
	body, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	u := fmt.Sprintf("%s/api/s/%s/rest/%s",
		c.baseURL, url.PathEscape(site), url.PathEscape(resourceType))
	resp, err := c.doJSON(ctx, http.MethodPost, u, body)
	if err != nil {
		return nil, fmt.Errorf("post %s: %w", resourceType, err)
	}
	defer drainBody(resp)
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body) //nolint:errcheck // best-effort read for error message
		return nil, fmt.Errorf("post %s (status %d): %s", resourceType, resp.StatusCode, msg)
	}
	items, err := decodeDataEnvelope(resp.Body)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("post %s: empty response", resourceType)
	}
	return items[0], nil
}

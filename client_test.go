package main

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	testAPILogin         = "/api/login"
	testAPINetworkconf   = "/api/s/default/rest/networkconf"
	testAPINetworkconfID = "/api/s/default/rest/networkconf/abc123"
	testNameHomeNet      = "HomeNet"
)

func unifiResponse(data any) []byte {
	b, err := json.Marshal(map[string]any{"meta": map[string]any{"rc": "ok"}, "data": data})
	if err != nil {
		panic("unifiResponse marshal: " + err.Error())
	}
	return b
}

func TestNewClient(t *testing.T) {
	c := newClient("https://example.com", true)
	if c.baseURL != "https://example.com" {
		t.Errorf("baseURL = %q", c.baseURL)
	}
	if c.http.Jar == nil {
		t.Error("cookie jar not set")
	}
}

func TestNewClientSecure(t *testing.T) {
	c := newClient("https://example.com", false)
	if c.http.Jar == nil {
		t.Error("cookie jar not set")
	}
}

func TestNewClientTrailingSlash(t *testing.T) {
	c := newClient("https://example.com/", true)
	if c.baseURL != "https://example.com" {
		t.Errorf("baseURL = %q, want without trailing slash", c.baseURL)
	}
}

func TestLogin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != testAPILogin {
			t.Errorf("path = %q", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		var creds map[string]string
		if err := json.Unmarshal(body, &creds); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if creds["username"] != "admin" || creds["password"] != "pass" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("bad creds")) //nolint:errcheck,revive // test handler
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newClient(srv.URL, false)
	if err := c.login(context.Background(), "admin", "pass"); err != nil {
		t.Fatalf("login() error = %v", err)
	}
}

func TestLoginCapturesCsrfToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Csrf-Token", "test-csrf-token")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newClient(srv.URL, false)
	if err := c.login(context.Background(), "admin", "pass"); err != nil {
		t.Fatalf("login() error = %v", err)
	}
	if c.csrfToken != "test-csrf-token" {
		t.Errorf("csrfToken = %q, want %q", c.csrfToken, "test-csrf-token")
	}
}

func TestLoginNoCsrfToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newClient(srv.URL, false)
	if err := c.login(context.Background(), "admin", "pass"); err != nil {
		t.Fatalf("login() error = %v", err)
	}
	if c.csrfToken != "" {
		t.Errorf("csrfToken = %q, want empty", c.csrfToken)
	}
}

func TestCsrfTokenSentOnPut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testAPILogin {
			w.Header().Set("X-Csrf-Token", "my-csrf")
			w.WriteHeader(http.StatusOK)
			return
		}
		if got := r.Header.Get("X-Csrf-Token"); got != "my-csrf" {
			t.Errorf("X-Csrf-Token = %q, want %q", got, "my-csrf")
		}
		w.Write(unifiResponse([]map[string]any{{"_id": "abc123", "name": "test"}})) //nolint:errcheck,revive // test handler
	}))
	defer srv.Close()

	ctx := context.Background()
	c := newClient(srv.URL, false)
	if err := c.login(ctx, "admin", "pass"); err != nil {
		t.Fatalf("login: %v", err)
	}
	if err := c.put(ctx, "default", "networkconf", "abc123", map[string]any{"name": "test"}); err != nil {
		t.Fatalf("put: %v", err)
	}
}

func TestCsrfTokenSentOnPost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testAPILogin {
			w.Header().Set("X-Csrf-Token", "my-csrf")
			w.WriteHeader(http.StatusOK)
			return
		}
		if got := r.Header.Get("X-Csrf-Token"); got != "my-csrf" {
			t.Errorf("X-Csrf-Token = %q, want %q", got, "my-csrf")
		}
		w.Write(unifiResponse([]map[string]any{{"_id": "1", "name": "test"}})) //nolint:errcheck,revive // test handler
	}))
	defer srv.Close()

	ctx := context.Background()
	c := newClient(srv.URL, false)
	if err := c.login(ctx, "admin", "pass"); err != nil {
		t.Fatalf("login: %v", err)
	}
	if _, err := c.post(ctx, "default", "networkconf", map[string]any{"name": "test"}); err != nil {
		t.Fatalf("post: %v", err)
	}
}

func TestLoginFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("unauthorized")) //nolint:errcheck,revive // test handler
	}))
	defer srv.Close()

	c := newClient(srv.URL, false)
	err := c.login(context.Background(), "bad", "creds")
	if err == nil {
		t.Error("login() should return error for 401")
	}
}

func TestLoginNetworkError(t *testing.T) {
	c := newClient("http://127.0.0.1:1", false)
	err := c.login(context.Background(), "admin", "pass")
	if err == nil {
		t.Error("login() should return error for network failure")
	}
}

func TestLoginBadURL(t *testing.T) {
	c := newClient("://bad\x00url", false)
	err := c.login(context.Background(), "admin", "pass")
	if err == nil {
		t.Error("login() should return error for invalid URL")
	}
}

func TestList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != testAPINetworkconf {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Write(unifiResponse([]map[string]any{ //nolint:errcheck,revive // test handler
			{"_id": "1", "name": testNameHomeNet},
			{"_id": "2", "name": "Guest Network"},
		}))
	}))
	defer srv.Close()

	c := newClient(srv.URL, false)
	items, err := c.list(context.Background(), "default", "networkconf")
	if err != nil {
		t.Fatalf("list() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len = %d, want 2", len(items))
	}
}

func TestListHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error")) //nolint:errcheck,revive // test handler
	}))
	defer srv.Close()

	c := newClient(srv.URL, false)
	_, err := c.list(context.Background(), "default", "networkconf")
	if err == nil {
		t.Error("list() should return error for 500")
	}
}

func TestListNetworkError(t *testing.T) {
	c := newClient("http://127.0.0.1:1", false)
	_, err := c.list(context.Background(), "default", "networkconf")
	if err == nil {
		t.Error("list() should return error for network failure")
	}
}

func TestListBadURL(t *testing.T) {
	c := newClient("://bad\x00url", false)
	_, err := c.list(context.Background(), "default", "networkconf")
	if err == nil {
		t.Error("list() should return error for invalid URL")
	}
}

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != testAPINetworkconfID {
			t.Errorf("path = %q", r.URL.Path)
		}
		resp := []map[string]any{{"_id": "abc123", "name": testNameHomeNet}}
		w.Write(unifiResponse(resp)) //nolint:errcheck,revive // test handler
	}))
	defer srv.Close()

	c := newClient(srv.URL, false)
	obj, err := c.get(context.Background(), "default", "networkconf", "abc123")
	if err != nil {
		t.Fatalf("get() error = %v", err)
	}
	if obj["name"] != testNameHomeNet {
		t.Errorf("name = %v", obj["name"])
	}
}

func TestGetHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found")) //nolint:errcheck,revive // test handler
	}))
	defer srv.Close()

	c := newClient(srv.URL, false)
	_, err := c.get(context.Background(), "default", "networkconf", "abc123")
	if err == nil {
		t.Error("get() should return error for 404")
	}
}

func TestGetNetworkError(t *testing.T) {
	c := newClient("http://127.0.0.1:1", false)
	_, err := c.get(context.Background(), "default", "networkconf", "abc123")
	if err == nil {
		t.Error("get() should return error for network failure")
	}
}

func TestGetBadURL(t *testing.T) {
	c := newClient("://bad\x00url", false)
	_, err := c.get(context.Background(), "default", "networkconf", "abc123")
	if err == nil {
		t.Error("get() should return error for invalid URL")
	}
}

func TestGetEmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(unifiResponse([]map[string]any{})) //nolint:errcheck,revive // test handler
	}))
	defer srv.Close()

	c := newClient(srv.URL, false)
	_, err := c.get(context.Background(), "default", "networkconf", "abc123")
	if err == nil {
		t.Error("get() should return error for empty data")
	}
}

func TestPut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %q, want PUT", r.Method)
		}
		if r.URL.Path != testAPINetworkconfID {
			t.Errorf("path = %q", r.URL.Path)
		}
		resp := []map[string]any{{"_id": "abc123", "name": "Updated"}}
		w.Write(unifiResponse(resp)) //nolint:errcheck,revive // test handler
	}))
	defer srv.Close()

	c := newClient(srv.URL, false)
	err := c.put(context.Background(), "default", "networkconf", "abc123", map[string]any{"name": "Updated"})
	if err != nil {
		t.Fatalf("put() error = %v", err)
	}
}

func TestPutHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("forbidden")) //nolint:errcheck,revive // test handler
	}))
	defer srv.Close()

	c := newClient(srv.URL, false)
	err := c.put(context.Background(), "default", "networkconf", "abc123", map[string]any{"name": "test"})
	if err == nil {
		t.Error("put() should return error for 403")
	}
}

func TestPutNetworkError(t *testing.T) {
	c := newClient("http://127.0.0.1:1", false)
	err := c.put(context.Background(), "default", "networkconf", "abc123", map[string]any{"name": "test"})
	if err == nil {
		t.Error("put() should return error for network failure")
	}
}

func TestPost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.URL.Path != testAPINetworkconf {
			t.Errorf("path = %q", r.URL.Path)
		}
		resp := []map[string]any{{"_id": "new123", "name": "New Net"}}
		w.Write(unifiResponse(resp)) //nolint:errcheck,revive // test handler
	}))
	defer srv.Close()

	c := newClient(srv.URL, false)
	obj, err := c.post(context.Background(), "default", "networkconf", map[string]any{"name": "New Net"})
	if err != nil {
		t.Fatalf("post() error = %v", err)
	}
	if obj["_id"] != "new123" {
		t.Errorf("_id = %v", obj["_id"])
	}
}

func TestPostHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request")) //nolint:errcheck,revive // test handler
	}))
	defer srv.Close()

	c := newClient(srv.URL, false)
	_, err := c.post(context.Background(), "default", "networkconf", map[string]any{"name": "test"})
	if err == nil {
		t.Error("post() should return error for 400")
	}
}

func TestPostNetworkError(t *testing.T) {
	c := newClient("http://127.0.0.1:1", false)
	_, err := c.post(context.Background(), "default", "networkconf", map[string]any{"name": "test"})
	if err == nil {
		t.Error("post() should return error for network failure")
	}
}

func TestGetBadResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("{invalid json")) //nolint:errcheck,revive // test handler
	}))
	defer srv.Close()

	c := newClient(srv.URL, false)
	_, err := c.get(context.Background(), "default", "networkconf", "abc123")
	if err == nil {
		t.Error("get() should return error for bad response body")
	}
}

func TestPutBadResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("{invalid json")) //nolint:errcheck,revive // test handler
	}))
	defer srv.Close()

	c := newClient(srv.URL, false)
	err := c.put(context.Background(), "default", "networkconf", "abc123", map[string]any{"name": "test"})
	if err == nil {
		t.Error("put() should return error for bad response body")
	}
}

func TestPutMarshalError(t *testing.T) {
	c := newClient("http://example.com", false)
	err := c.put(context.Background(), "default", "networkconf", "abc123", map[string]any{"bad": math.Inf(1)})
	if err == nil {
		t.Error("put() should return error for unmarshalable data")
	}
}

func TestPutBadURL(t *testing.T) {
	c := newClient("://bad\x00url", false)
	err := c.put(context.Background(), "default", "networkconf", "abc123", map[string]any{"name": "test"})
	if err == nil {
		t.Error("put() should return error for invalid URL")
	}
}

func TestPostBadURL(t *testing.T) {
	c := newClient("://bad\x00url", false)
	_, err := c.post(context.Background(), "default", "networkconf", map[string]any{"name": "test"})
	if err == nil {
		t.Error("post() should return error for invalid URL")
	}
}

func TestPostMarshalError(t *testing.T) {
	c := newClient("http://example.com", false)
	_, err := c.post(context.Background(), "default", "networkconf", map[string]any{"bad": math.Inf(1)})
	if err == nil {
		t.Error("post() should return error for unmarshalable data")
	}
}

func TestPostBadResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("{invalid json")) //nolint:errcheck,revive // test handler
	}))
	defer srv.Close()

	c := newClient(srv.URL, false)
	_, err := c.post(context.Background(), "default", "networkconf", map[string]any{"name": "test"})
	if err == nil {
		t.Error("post() should return error for bad response body")
	}
}

func TestPostEmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(unifiResponse([]map[string]any{})) //nolint:errcheck,revive // test handler
	}))
	defer srv.Close()

	c := newClient(srv.URL, false)
	_, err := c.post(context.Background(), "default", "networkconf", map[string]any{"name": "test"})
	if err == nil {
		t.Error("post() should return error for empty response")
	}
}

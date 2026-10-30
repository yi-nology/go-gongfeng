package gongfeng

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// --- Paginate ---

// TestPaginateWalksAllPages verifies the walker follows X-Next-Page to the
// end and assembles items in page order.
func TestPaginateWalksAllPages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")
		switch page {
		case "", "1":
			w.Header().Set("X-Next-Page", "2")
			_, _ = w.Write([]byte(`[{"id":1},{"id":2}]`))
		case "2":
			w.Header().Set("X-Next-Page", "3")
			_, _ = w.Write([]byte(`[{"id":3}]`))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer srv.Close()

	client, err := NewClient("t", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	var pages []int
	items, err := Paginate(func(page int) ([]*struct {
		ID int `json:"id"`
	}, *Response, error) {
		pages = append(pages, page)
		req, _ := client.NewRequest(context.Background(), http.MethodGet, "projects",
			&ListOptions{Page: page, PerPage: 100})
		var out []*struct {
			ID int `json:"id"`
		}
		resp, err := client.Do(req, &out)
		return out, resp, err
	})
	if err != nil {
		t.Fatalf("Paginate: %v", err)
	}
	if len(items) != 3 || items[0].ID != 1 || items[2].ID != 3 {
		t.Fatalf("unexpected items: %+v", items)
	}
	if len(pages) != 3 {
		t.Errorf("walked %d pages, want 3", len(pages))
	}
}

// TestPaginateStopsWithoutNextPage verifies a nil/zero NextPage terminates
// the walk even when items remain empty.
func TestPaginateStopsWithoutNextPage(t *testing.T) {
	calls := 0
	_, err := Paginate(func(page int) ([]int, *Response, error) {
		calls++
		return []int{page}, &Response{NextPage: 0}, nil
	})
	if err != nil {
		t.Fatalf("Paginate: %v", err)
	}
	if calls != 1 {
		t.Errorf("walked %d pages without NextPage, want 1", calls)
	}
}

// TestPaginateWrapsPageErrors verifies a per-page failure surfaces wrapped
// with the page number.
func TestPaginateWrapsPageErrors(t *testing.T) {
	_, err := Paginate(func(page int) ([]int, *Response, error) {
		return nil, nil, errors.New("boom")
	})
	if err == nil || !strings.Contains(err.Error(), "page") {
		t.Fatalf("expected wrapped error mentioning page, got %v", err)
	}
}

// TestPaginateBudgetCap verifies the walk stops at maxPaginatePages when a
// misbehaving server never clears NextPage.
func TestPaginateBudgetCap(t *testing.T) {
	_, err := Paginate(func(page int) ([]int, *Response, error) {
		return []int{page}, &Response{NextPage: page + 1}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("expected budget-cap error, got %v", err)
	}
}

// --- retry ---

// TestRetryRecoversFrom503 verifies a GET retried after 503s eventually
// succeeds, and the server saw every attempt.
func TestRetryRecoversFrom503(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"message":"503"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	defer srv.Close()

	client, err := NewClient("t", WithBaseURL(srv.URL), WithRetry(3))
	if err != nil {
		t.Fatal(err)
	}
	req, _ := client.NewRequest(context.Background(), http.MethodGet, "projects/1", nil)
	var out map[string]any
	if _, err := client.Do(req, &out); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if hits != 3 {
		t.Errorf("server hit %d times, want 3", hits)
	}
}

// TestRetryNeverReplaysWrites verifies a failing POST is not replayed even
// with retry enabled.
func TestRetryNeverReplaysWrites(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client, _ := NewClient("t", WithBaseURL(srv.URL), WithRetry(3))
	req, _ := client.NewRequest(context.Background(), http.MethodPost, "projects", map[string]any{"a": "b"})
	if _, err := client.Do(req, nil); err == nil {
		t.Fatal("expected the 503 to surface")
	}
	if hits != 1 {
		t.Errorf("POST replayed %d times, want exactly 1", hits)
	}
}

// TestRetrySkipsNotFound verifies 4xx (non-429) responses are not retried.
func TestRetrySkipsNotFound(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client, _ := NewClient("t", WithBaseURL(srv.URL), WithRetry(3))
	req, _ := client.NewRequest(context.Background(), http.MethodGet, "projects/1", nil)
	if _, err := client.Do(req, nil); err == nil {
		t.Fatal("expected the 404 to surface")
	}
	if hits != 1 {
		t.Errorf("404 retried, server hit %d times, want 1", hits)
	}
}

// TestRetryExhaustionSurfacesLastError verifies the final error carries the
// last response's status when the budget runs out.
func TestRetryExhaustionSurfacesLastError(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	client, _ := NewClient("t", WithBaseURL(srv.URL), WithRetry(3))
	req, _ := client.NewRequest(context.Background(), http.MethodGet, "projects/1", nil)
	_, err := client.Do(req, nil)
	if err == nil {
		t.Fatal("expected an error after exhaustion")
	}
	if hits != 3 {
		t.Errorf("server hit %d times, want 3 (attempts cap)", hits)
	}
	if !IsRateLimited(err) {
		t.Errorf("exhausted error should classify as rate-limited, got %v", err)
	}
}

// TestRetryDisabledByDefault verifies the default client performs exactly
// one attempt.
func TestRetryDisabledByDefault(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	client, _ := NewClient("t", WithBaseURL(srv.URL))
	req, _ := client.NewRequest(context.Background(), http.MethodGet, "projects/1", nil)
	if _, err := client.Do(req, nil); err == nil {
		t.Fatal("expected error")
	}
	if hits != 1 {
		t.Errorf("default client retried; hits = %d, want 1", hits)
	}
}

// --- error helpers & options ---

// TestErrorHelpers verifies the typed-error classification helpers.
func TestErrorHelpers(t *testing.T) {
	mk := func(code int) *ErrorResponse {
		rec := httptest.NewRecorder()
		rec.WriteHeader(code)
		return &ErrorResponse{Response: rec.Result(), Message: "m"}
	}
	cases := []struct {
		code int
		want func(error) bool
	}{
		{401, IsUnauthorized},
		{403, IsForbidden},
		{404, IsNotFound},
		{429, IsRateLimited},
	}
	for _, tc := range cases {
		if !tc.want(mk(tc.code)) {
			t.Errorf("code %d not classified by its helper", tc.code)
		}
		if tc.want(mk(200)) {
			t.Errorf("code %d misclassified by helper at 200", tc.code)
		}
	}
	code, ok := StatusCode(mk(404))
	if !ok || code != 404 {
		t.Errorf("StatusCode = %d,%v want 404,true", code, ok)
	}
	if _, ok := StatusCode(errors.New("plain")); ok {
		t.Error("plain error must not carry a status code")
	}
}

// TestRetryAfterParsing verifies the Retry-After parser.
func TestRetryAfterParsing(t *testing.T) {
	rec := httptest.NewRecorder()
	rec.Header().Set("Retry-After", "7")
	resp := rec.Result()
	d, ok := retryAfter(resp)
	if !ok || d != 7*time.Second {
		t.Errorf("retryAfter = %v,%v want 7s,true", d, ok)
	}
	rec2 := httptest.NewRecorder()
	if _, ok := retryAfter(rec2.Result()); ok {
		t.Error("missing header must report false")
	}
}

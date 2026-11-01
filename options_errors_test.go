package gongfeng

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// recordingLogger captures kv-style log lines for injection assertions.
type recordingLogger struct {
	lines []string
}

func (l *recordingLogger) log(level, msg string, _ ...any) {
	l.lines = append(l.lines, level+" "+msg)
}

func (l *recordingLogger) Debug(msg string, _ ...any) { l.log("debug", msg) }
func (l *recordingLogger) Info(msg string, _ ...any)  { l.log("info", msg) }
func (l *recordingLogger) Warn(msg string, _ ...any)  { l.log("warn", msg) }
func (l *recordingLogger) Error(msg string, _ ...any) { l.log("error", msg) }

// recordingTransport observes that a custom http.Client is actually used.
type recordingTransport struct {
	roundTrips int
	base       http.RoundTripper
}

func (t *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.roundTrips++
	return t.base.RoundTrip(req)
}

func TestWithHTTPClientUsesCustomClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	tr := &recordingTransport{base: http.DefaultTransport}
	client, err := NewClient("t", WithBaseURL(srv.URL), WithHTTPClient(&http.Client{Transport: tr}))
	if err != nil {
		t.Fatal(err)
	}
	req, _ := client.NewRequest(context.Background(), http.MethodGet, "projects", nil)
	if _, err := client.Do(req, nil); err != nil {
		t.Fatal(err)
	}
	if tr.roundTrips != 1 {
		t.Errorf("custom transport used %d times, want 1", tr.roundTrips)
	}
}

func TestWithLoggerInjectsDebugLines(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	logger := &recordingLogger{}
	client, err := NewClient("t", WithBaseURL(srv.URL), WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}
	req, _ := client.NewRequest(context.Background(), http.MethodGet, "projects/1", nil)
	if _, err := client.Do(req, nil); err != nil {
		t.Fatal(err)
	}
	if len(logger.lines) != 1 || !strings.HasPrefix(logger.lines[0], "debug gongfeng request") {
		t.Errorf("expected one debug request line, got %v", logger.lines)
	}
}

func TestWithLoggerNilIsNoop(t *testing.T) {
	client, err := NewClient("t", WithLogger(nil))
	if err != nil {
		t.Fatal(err)
	}
	if client.logger == nil {
		t.Fatal("nil logger injection must keep the default noop logger")
	}
	// The noop logger swallows everything without panicking.
	client.logger.Debug("d")
	client.logger.Info("i")
	client.logger.Warn("w")
	client.logger.Error("e")
}

func TestNewNoopLoggerSwallowsAll(t *testing.T) {
	l := NewNoopLogger()
	if l == nil {
		t.Fatal("NewNoopLogger returned nil")
	}
	l.Debug("d")
	l.Info("i")
	l.Warn("w")
	l.Error("e")
}

func TestErrorResponseMessage(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://example.com/api/v3/projects/1", nil)
	resp := &http.Response{StatusCode: http.StatusNotFound, Request: req}
	err := &ErrorResponse{Response: resp, Message: "resource not found"}
	want := "GET http://example.com/api/v3/projects/1: 404 resource not found"
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

// TestRetryExhaustionWrapsWithAttempts pins the exhaustion sentinel: the
// error keeps its cause chain (IsRateLimited still classifies) and names
// the retry budget.
func TestRetryExhaustionWrapsWithAttempts(t *testing.T) {
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
	if !strings.Contains(err.Error(), "retries exhausted") || !strings.Contains(err.Error(), "attempts=3") {
		t.Errorf("exhausted wrapper missing context: %v", err)
	}
	// The raw last response error stays reachable through the chain.
	var respErr *ErrorResponse
	if !errors.As(err, &respErr) || respErr.Response.StatusCode != http.StatusTooManyRequests {
		t.Errorf("cause chain lost the final *ErrorResponse: %v", err)
	}
}

// TestSingleAttemptDoesNotWrapExhaustion pins the boundary: with retry
// disabled (attempts=1) a failure is not an "exhaustion" and returns raw.
func TestSingleAttemptDoesNotWrapExhaustion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	client, _ := NewClient("t", WithBaseURL(srv.URL)) // default: retry off
	req, _ := client.NewRequest(context.Background(), http.MethodGet, "projects/1", nil)
	_, err := client.Do(req, nil)
	if err == nil {
		t.Fatal("expected the raw error")
	}
	if strings.Contains(err.Error(), "retries exhausted") {
		t.Errorf("single-attempt failure must not be wrapped as exhaustion: %v", err)
	}
	if !IsRateLimited(err) {
		t.Errorf("raw error should classify as rate-limited, got %v", err)
	}
}

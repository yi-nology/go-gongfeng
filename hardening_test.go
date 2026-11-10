package gongfeng

// Hardening tests for issues #3 (Paginate keeps fetched pages on failure)
// and #4 (backoff prefers the server's Retry-After).

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// --- issue #3: Paginate partial results ---

func TestPaginateKeepsFetchedPagesOnFailure(t *testing.T) {
	tests := []struct {
		name        string
		fetch       func(page int) ([]int, *Response, error)
		wantItems   []int
		wantErr     bool
		wantErrText string
	}{
		{
			name: "single page failure returns fetched pages",
			fetch: func(page int) ([]int, *Response, error) {
				if page == 3 {
					return nil, nil, errors.New("boom: network reset")
				}
				return []int{page}, &Response{NextPage: page + 1}, nil
			},
			wantItems:   []int{1, 2},
			wantErr:     true,
			wantErrText: "fetch page failed",
		},
		{
			name: "complete walk returns everything",
			fetch: func(page int) ([]int, *Response, error) {
				if page == 2 {
					return []int{20}, &Response{}, nil // no NextPage: end
				}
				return []int{10}, &Response{NextPage: 2}, nil
			},
			wantItems: []int{10, 20},
			wantErr:   false,
		},
		{
			name: "nil response ends the walk",
			fetch: func(page int) ([]int, *Response, error) {
				return []int{1}, nil, nil
			},
			wantItems: []int{1},
			wantErr:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, err := Paginate(tt.fetch)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, want error %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.wantErrText != "" {
				if !strings.Contains(err.Error(), tt.wantErrText) {
					t.Errorf("error %q does not mention %q", err.Error(), tt.wantErrText)
				}
			}
			if len(items) != len(tt.wantItems) {
				t.Fatalf("items = %v, want %v", items, tt.wantItems)
			}
			for i := range tt.wantItems {
				if items[i] != tt.wantItems[i] {
					t.Errorf("items[%d] = %d, want %d", i, items[i], tt.wantItems[i])
				}
			}
		})
	}
}

// TestGetReleaseByTagFoundDespiteLaterPageFailure: the target was scanned
// on page 1 and page 3 fails with a network error — the lookup must still
// return the release instead of discarding it (issue #3).
func TestGetReleaseByTagFoundDespiteLaterPageFailure(t *testing.T) {
	client, mux := setup(t)

	mux.HandleFunc("/api/v3/projects/1/releases", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("page") {
		case "", "1":
			w.Header().Set("X-Next-Page", "2")
			fmt.Fprint(w, `[{"id":1,"tag":"v1.0"},{"id":2,"tag":"v2.0"}]`)
		case "2":
			w.Header().Set("X-Next-Page", "3")
			fmt.Fprint(w, `[{"id":3,"tag":"v3.0"}]`)
		default:
			// Network-level failure on page 3.
			panic(http.ErrAbortHandler)
		}
	})

	release, _, err := client.Releases.GetReleaseByTag(context.Background(), 1, "v2.0")
	if err != nil {
		t.Fatalf("expected the already-scanned release to be returned, got error: %v", err)
	}
	if release == nil || release.ID != 2 || release.Tag != "v2.0" {
		t.Fatalf("unexpected release: %+v", release)
	}
}

// TestGetReleaseByTagSurfacesErrorWhenTargetNotScanned: the target is not
// in the fetched prefix and a later page fails — the fetch error must win
// over ErrReleaseNotFound so callers can tell "list incomplete" from
// "scanned everything, no match" (issue #3).
func TestGetReleaseByTagSurfacesErrorWhenTargetNotScanned(t *testing.T) {
	client, mux := setup(t)

	mux.HandleFunc("/api/v3/projects/1/releases", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "2" {
			panic(http.ErrAbortHandler)
		}
		w.Header().Set("X-Next-Page", "2")
		fmt.Fprint(w, `[{"id":1,"tag":"v1.0"}]`)
	})

	_, _, err := client.Releases.GetReleaseByTag(context.Background(), 1, "v9.9")
	if err == nil {
		t.Fatal("expected the page-2 fetch error to surface")
	}
	if errors.Is(err, ErrReleaseNotFound) {
		t.Fatalf("error must not be ErrReleaseNotFound (the list was never fully scanned): %v", err)
	}
}

// --- issue #4: backoff prefers the server's Retry-After ---

func TestRetryDelayPrefersServerRetryAfter(t *testing.T) {
	client, _ := NewClient("t")
	// retryBaseDelay defaults to 200ms → exponential 200ms/400ms/800ms/1.6s…

	header := func(v string) *Response {
		h := http.Header{}
		if v != "" {
			h.Set("Retry-After", v)
		}
		return &Response{Response: &http.Response{Header: h}}
	}

	tests := []struct {
		name    string
		attempt int
		resp    *Response
		want    time.Duration
	}{
		{"no header uses exponential", 1, header(""), 200 * time.Millisecond},
		{"no header attempt 3", 3, header(""), 800 * time.Millisecond},
		{"no header capped", 10, header(""), maxRetryDelay},
		{"nil response uses exponential", 1, nil, 200 * time.Millisecond},
		// issue #4 的形态：服务端建议比本地退避更长，必须采用（旧实现忽略它）。
		{"server longer than local wins", 1, header("3"), 3 * time.Second},
		{"server shorter than local also wins", 5, header("1"), time.Second},
		{"server suggestion capped", 1, header("3600"), maxRetryDelay},
		{"zero seconds respected", 3, header("0"), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := client.retryDelay(tt.attempt, tt.resp)
			if got != tt.want {
				t.Errorf("retryDelay() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestRetryDelayHTTPDate: the HTTP-date form of Retry-After is honored
// (capped), not silently ignored.
func TestRetryDelayHTTPDate(t *testing.T) {
	client, _ := NewClient("t")
	h := http.Header{}
	h.Set("Retry-After", time.Now().Add(3*time.Second).UTC().Format(http.TimeFormat))
	got := client.retryDelay(1, &Response{Response: &http.Response{Header: h}})
	if got < 2*time.Second || got > maxRetryDelay {
		t.Errorf("retryDelay(http-date +3s) = %v, want ~3s (capped)", got)
	}
}

// TestBackoffHonorsRetryAfterOn429 is the wire-level repro of issue #4:
// the server demands 3 seconds on a 429; the gaps between attempts must
// honor it instead of the local 200ms/400ms exponential.
func TestBackoffHonorsRetryAfterOn429(t *testing.T) {
	var calls int32
	gapsCh := make(chan time.Duration, 16)
	var last atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if prev := last.Swap(time.Now().UnixNano()); prev != 0 {
			gapsCh <- time.Duration(time.Now().UnixNano() - prev)
		}
		w.Header().Set("Content-Type", "application/json")
		if n < 3 {
			w.Header().Set("Retry-After", "1") // 1s keeps the test fast; > base 200ms
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"message":"rate limited"}`)
			return
		}
		fmt.Fprint(w, `{"id":1}`)
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
	close(gapsCh)
	var gaps []time.Duration
	for g := range gapsCh {
		gaps = append(gaps, g)
	}
	if len(gaps) != 2 {
		t.Fatalf("gaps recorded = %d, want 2", len(gaps))
	}
	for i, g := range gaps {
		if g < 900*time.Millisecond {
			t.Errorf("gap %d = %v, want >= ~1s (server Retry-After; issue #4 used to wait 200ms)", i+1, g)
		}
	}
}

// TestRetryAfterForms exercises the parser added for issue #4: both
// delay-seconds and HTTP-date, with rejection cases.
func TestRetryAfterForms(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name   string
		value  string
		wantOK bool
		check  func(time.Duration) bool
	}{
		{"delay-seconds", "7", true, func(d time.Duration) bool { return d == 7*time.Second }},
		{"zero allowed (server says now)", "0", true, func(d time.Duration) bool { return d == 0 }},
		{"negative rejected", "-5", false, nil},
		{"garbage rejected", "soon", false, nil},
		{"http-date future", now.Add(time.Minute).UTC().Format(http.TimeFormat), true, func(d time.Duration) bool {
			return d > 55*time.Second && d <= time.Minute
		}},
		{"http-date past rejected", now.Add(-time.Minute).UTC().Format(http.TimeFormat), false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.Header{}
			h.Set("Retry-After", tt.value)
			d, ok := retryAfter(&http.Response{Header: h})
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if tt.wantOK && !tt.check(d) {
				t.Errorf("duration = %v, failed check", d)
			}
		})
	}
}

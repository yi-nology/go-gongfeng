package gongfeng

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"
)

// 测试里的响应体逐字取自 docs/api/commit_check.md 的返回值样例：
// 检测状态的 wire 键名是 state（issue #9），不是 status。

func TestCreateCommitStatus(t *testing.T) {
	client, mux := setup(t)

	mux.HandleFunc("/api/v3/projects/1/commit/abc123/statuses", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.Header.Get("PRIVATE-TOKEN") == "" {
			t.Fatal("missing PRIVATE-TOKEN header")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":365,"state":"success","target_url":"https://git.code.tencent.com/mr/check","description":"check success","context":"jenkins/mr"}`)
	})

	opts := &CreateCommitStatusOptions{
		State: Ptr("success"),
		Name:  Ptr("ci"),
	}
	status, _, err := client.CommitStatuses.CreateCommitStatus(context.Background(), 1, "abc123", opts)
	if err != nil {
		t.Fatal(err)
	}
	if status.ID != 365 {
		t.Fatalf("expected ID=365, got %d", status.ID)
	}
	if status.Status != "success" {
		t.Fatalf("expected status 'success' (from wire key state), got %q", status.Status)
	}
}

func TestListCommitStatuses(t *testing.T) {
	client, mux := setup(t)

	mux.HandleFunc("/api/v3/projects/1/commits/abc123/statuses", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Total", "1")
		fmt.Fprint(w, `[{"id":21,"state":"success","target_url":"https://git.code.tencent.com/mr/check"}]`)
	})

	statuses, resp, err := client.CommitStatuses.ListCommitStatuses(context.Background(), 1, "abc123", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].Status != "success" {
		t.Fatalf("expected status 'success' (from wire key state), got %q", statuses[0].Status)
	}
	if resp.TotalItems != 1 {
		t.Fatalf("expected TotalItems=1, got %d", resp.TotalItems)
	}
}

// TestGetCommitStatusResult feeds the exact response sample from
// docs/api/commit_check.md and pins that Status reads the wire key state
// (issue #9: the SDK used to read status, so Status stayed empty forever).
func TestGetCommitStatusResult(t *testing.T) {
	client, mux := setup(t)

	mux.HandleFunc("/api/v3/projects/1/commits/main/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"state":"pending","block":false,"statuses":[{"id":21,"state":"pending","target_url":"https://git.code.tencent.com/push/check","description":"check pending","context":"jenkins/push","block":true}],"sha":"b5e3f65af2fd6d2895414a679290cad7664217b3","total_count":2}`)
	})

	result, _, err := client.CommitStatuses.GetCommitStatusResult(context.Background(), 1, "main", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "pending" {
		t.Fatalf("expected Status 'pending' (from wire key state), got %q", result.Status)
	}
	if result.TotalCount != 2 {
		t.Fatalf("expected TotalCount=2, got %d", result.TotalCount)
	}
	if len(result.Statuses) != 1 {
		t.Fatalf("expected 1 status entry, got %d", len(result.Statuses))
	}
	if result.Statuses[0].Status != "pending" {
		t.Fatalf("expected statuses[0].Status 'pending', got %q", result.Statuses[0].Status)
	}
}

// TestGetCommitStatusResultPassesPagination pins the documented page/per_page
// parameters (issue #11: the endpoint had no options, so the query was always
// empty and callers could never reach page 2 of the combined statuses).
func TestGetCommitStatusResultPassesPagination(t *testing.T) {
	client, mux := setup(t)

	var got url.Values
	mux.HandleFunc("/api/v3/projects/1/commits/main/status", func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"state":"success","statuses":[],"total_count":1}`)
	})

	opts := &ListOptions{Page: 3, PerPage: 5}
	if _, _, err := client.CommitStatuses.GetCommitStatusResult(context.Background(), 1, "main", opts); err != nil {
		t.Fatal(err)
	}
	if got.Get("page") != "3" {
		t.Errorf("query page = %q, want 3 (full query: %v)", got.Get("page"), got)
	}
	if got.Get("per_page") != "5" {
		t.Errorf("query per_page = %q, want 5 (full query: %v)", got.Get("per_page"), got)
	}
}

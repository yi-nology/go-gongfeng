package gongfeng

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestCreateMergeRequest(t *testing.T) {
	client, mux := setup(t)

	mux.HandleFunc("/api/v3/projects/1/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.Header.Get("PRIVATE-TOKEN") == "" {
			t.Fatal("missing PRIVATE-TOKEN header")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":1,"iid":1,"title":"MR title","state":"opened"}`)
	})

	opts := &CreateMergeRequestOptions{
		SourceBranch: Ptr("feature"),
		TargetBranch: Ptr("main"),
		Title:        Ptr("MR title"),
	}
	mr, _, err := client.MergeRequests.CreateMergeRequest(context.Background(), 1, opts)
	if err != nil {
		t.Fatal(err)
	}
	if mr.ID != 1 {
		t.Fatalf("expected MR ID 1, got %d", mr.ID)
	}
	if mr.Title != "MR title" {
		t.Fatalf("expected title 'MR title', got %q", mr.Title)
	}
	if mr.State != "opened" {
		t.Fatalf("expected state 'opened', got %q", mr.State)
	}
}

func TestAcceptMergeRequest(t *testing.T) {
	client, mux := setup(t)

	mux.HandleFunc("/api/v3/projects/1/merge_request/1/merge", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":1,"iid":1,"state":"merged"}`)
	})

	mr, _, err := client.MergeRequests.AcceptMergeRequest(context.Background(), 1, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if mr.State != "merged" {
		t.Fatalf("expected state 'merged', got %q", mr.State)
	}
}

func TestListMergeRequests(t *testing.T) {
	client, mux := setup(t)

	mux.HandleFunc("/api/v3/projects/1/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Total", "1")
		fmt.Fprint(w, `[{"id":1,"iid":1,"title":"MR1"}]`)
	})

	mrs, resp, err := client.MergeRequests.ListMergeRequests(context.Background(), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(mrs) != 1 {
		t.Fatalf("expected 1 MR, got %d", len(mrs))
	}
	if mrs[0].Title != "MR1" {
		t.Fatalf("expected title 'MR1', got %q", mrs[0].Title)
	}
	if resp.TotalItems != 1 {
		t.Fatalf("expected TotalItems=1, got %d", resp.TotalItems)
	}
}

func TestGetMergeRequest(t *testing.T) {
	client, mux := setup(t)

	mux.HandleFunc("/api/v3/projects/1/merge_request/1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":1,"iid":1,"title":"MR1","state":"opened"}`)
	})

	mr, _, err := client.MergeRequests.GetMergeRequest(context.Background(), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if mr.ID != 1 {
		t.Fatalf("expected MR ID 1, got %d", mr.ID)
	}
	if mr.State != "opened" {
		t.Fatalf("expected state 'opened', got %q", mr.State)
	}
}

func TestGetMergeRequestChanges(t *testing.T) {
	client, mux := setup(t)

	mux.HandleFunc("/api/v3/projects/1/merge_request/1/changes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":1,"iid":1,"files":[{"old_path":"a.go","new_path":"a.go"}]}`)
	})

	changes, _, err := client.MergeRequests.GetMergeRequestChanges(context.Background(), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if changes.ID != 1 {
		t.Fatalf("expected MR ID 1, got %d", changes.ID)
	}
	if len(changes.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(changes.Files))
	}
	if changes.Files[0].OldPath != "a.go" {
		t.Fatalf("expected old_path 'a.go', got %q", changes.Files[0].OldPath)
	}
}

func TestListMergeRequestCommits(t *testing.T) {
	client, mux := setup(t)

	mux.HandleFunc("/api/v3/projects/1/merge_request/1/commits", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":"abc","title":"commit"}]`)
	})

	commits, _, err := client.MergeRequests.ListMergeRequestCommits(context.Background(), 1, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(commits))
	}
	if commits[0].ID != "abc" {
		t.Fatalf("expected commit ID 'abc', got %q", commits[0].ID)
	}
}

// TestCreateMRComment pins the documented request contract (issue #8): the
// request body must carry the note key (body is a response-side field name),
// otherwise the server silently stores an empty comment.
func TestCreateMRComment(t *testing.T) {
	client, mux := setup(t)

	mux.HandleFunc("/api/v3/projects/1/merge_request/1/comments", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		var reqBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if _, ok := reqBody["note"]; !ok {
			t.Errorf("request body must carry the note key, got %v", reqBody)
		}
		if _, ok := reqBody["body"]; ok {
			t.Errorf("request body must not carry the body key (response-side name), got %v", reqBody)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":1,"body":"nice"}`)
	})

	opts := &CreateMRCommentOptions{
		Note: Ptr("nice"),
	}
	comment, _, err := client.MergeRequests.CreateMRComment(context.Background(), 1, 1, opts)
	if err != nil {
		t.Fatal(err)
	}
	if comment.ID != 1 {
		t.Fatalf("expected comment ID 1, got %d", comment.ID)
	}
	if comment.Body != "nice" {
		t.Fatalf("expected body 'nice', got %q", comment.Body)
	}
}

func TestListMRComments(t *testing.T) {
	client, mux := setup(t)

	mux.HandleFunc("/api/v3/projects/1/merge_request/1/comments", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":1,"body":"nice"}]`)
	})

	comments, _, err := client.MergeRequests.ListMRComments(context.Background(), 1, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	if comments[0].Body != "nice" {
		t.Fatalf("expected body 'nice', got %q", comments[0].Body)
	}
}

func TestUpdateMergeRequest(t *testing.T) {
	client, mux := setup(t)

	mux.HandleFunc("/api/v3/projects/1/merge_request/1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":1,"iid":1,"title":"Updated"}`)
	})

	opts := &UpdateMergeRequestOptions{
		Title: Ptr("Updated"),
	}
	mr, _, err := client.MergeRequests.UpdateMergeRequest(context.Background(), 1, 1, opts)
	if err != nil {
		t.Fatal(err)
	}
	if mr.Title != "Updated" {
		t.Fatalf("expected title 'Updated', got %q", mr.Title)
	}
}

// TestGetMRSubscription pins the documented contract (issue #7): the GET
// subscribe endpoint returns a bare bool, not an object.
func TestGetMRSubscription(t *testing.T) {
	client, mux := setup(t)

	mux.HandleFunc("/api/v3/projects/1/merge_request/1/subscribe", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `true`)
	})

	subscribed, _, err := client.MergeRequests.GetMRSubscription(context.Background(), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !subscribed {
		t.Fatal("expected subscribed=true")
	}
}

// TestSubscribeMR pins the documented contract (issue #7): the PUT subscribe
// endpoint returns only a status code with an empty body, and the SDK must
// not report that as an error.
func TestSubscribeMR(t *testing.T) {
	client, mux := setup(t)

	mux.HandleFunc("/api/v3/projects/1/merge_request/1/subscribe", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
	})

	resp, err := client.MergeRequests.SubscribeMR(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("subscribe with empty 201 body must succeed, got %v", err)
	}
	if resp == nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 response, got %+v", resp)
	}
}

// TestUnsubscribeMR pins the documented contract (issue #7): the PUT
// unsubscribe endpoint returns only a status code with an empty body.
func TestUnsubscribeMR(t *testing.T) {
	client, mux := setup(t)

	mux.HandleFunc("/api/v3/projects/1/merge_request/1/unsubscribe", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	})

	resp, err := client.MergeRequests.UnsubscribeMR(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("unsubscribe with empty body must succeed, got %v", err)
	}
	if resp == nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 response, got %+v", resp)
	}
}

func TestDownloadMergeRequestChangedFiles(t *testing.T) {
	client, mux := setup(t)

	mux.HandleFunc("/api/v3/projects/1/merge_request/1/changed_files", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		fmt.Fprint(w, "mr changed files")
	})

	var buf bytes.Buffer
	if _, err := client.MergeRequests.DownloadMergeRequestChangedFiles(context.Background(), 1, 1, &buf); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "mr changed files" {
		t.Fatalf("unexpected body: %q", buf.String())
	}
}

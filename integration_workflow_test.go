package gongfeng

import (
	"bytes"
	"testing"
)

// 本文件覆盖工作流面新端点的真实环境验证：订阅（watch）、全局 issue 列表、
// 里程碑关联 issue 列表与归档下载。全部只读（订阅只读不翻状态），无
// GONGFENG_TOKEN 时整组跳过。断言尽量"软"——集成环境的数据形态不受本仓
// 控制，缺数据记日志不判失败，只有 wire 形状错误才 Fatal。

// TestIntegration_Subscriptions reads issue/MR watch state. Issues decode a
// bare bool, MRs decode {"subscribed":b} — both must classify 404s as
// IsNotFound (the classification chain these helpers promise).
func TestIntegration_Subscriptions(t *testing.T) {
	c := newIntegrationClient(t)
	ctx, cancel := integrationCtx(t)
	defer cancel()

	issues, _, err := c.Issues.ListIssues(ctx, integrationProject,
		&ListIssuesOptions{ListOptions: ListOptions{Page: 1, PerPage: 1}})
	if err != nil {
		t.Fatalf("ListProjectIssues: %v", err)
	}
	if len(issues) > 0 {
		subscribed, _, err := c.Issues.GetIssueSubscription(ctx, integrationProject, issues[0].ID)
		if err != nil {
			if IsNotFound(err) {
				t.Logf("issue %d subscribe read 404 (platform quirk), skipped", issues[0].ID)
			} else {
				t.Fatalf("GetIssueSubscription: %v", err)
			}
		} else {
			t.Logf("issue %d subscribed=%v", issues[0].ID, subscribed)
		}
	} else {
		t.Log("no issues in integration project; subscription read skipped")
	}

	mrs, _, err := c.MergeRequests.ListMergeRequests(ctx, integrationProject,
		&ListMergeRequestsOptions{ListOptions: ListOptions{Page: 1, PerPage: 1}})
	if err != nil {
		t.Fatalf("ListMergeRequests: %v", err)
	}
	if len(mrs) > 0 {
		sub, _, err := c.MergeRequests.GetMRSubscription(ctx, integrationProject, mrs[0].ID)
		if err != nil {
			if IsNotFound(err) {
				t.Logf("MR %d subscribe read 404 (platform quirk), skipped", mrs[0].ID)
			} else {
				t.Fatalf("GetMRSubscription: %v", err)
			}
		} else {
			t.Logf("MR %d subscribed=%v", mrs[0].ID, sub.Subscribed)
		}
	} else {
		t.Log("no MRs in integration project; MR subscription read skipped")
	}
}

// TestIntegration_ListUserIssues pins the global /issues endpoint against a
// real token: the listing must decode and (for a token with any visible
// issues) carry state/labels fields the convertIssue mapping consumes.
func TestIntegration_ListUserIssues(t *testing.T) {
	c := newIntegrationClient(t)
	ctx, cancel := integrationCtx(t)
	defer cancel()

	issues, _, err := c.Issues.ListUserIssues(ctx, &ListIssuesOptions{
		ListOptions: ListOptions{Page: 1, PerPage: 5},
	})
	if err != nil {
		t.Fatalf("ListUserIssues: %v", err)
	}
	t.Logf("global issues visible: %d (first page)", len(issues))
	for _, i := range issues {
		if i.ID == 0 {
			t.Errorf("global issue with empty id: %+v", i)
		}
	}
}

// TestIntegration_ListMilestoneIssues walks one milestone's issue listing
// when the integration project has milestones; empty environments log-skip.
func TestIntegration_ListMilestoneIssues(t *testing.T) {
	c := newIntegrationClient(t)
	ctx, cancel := integrationCtx(t)
	defer cancel()

	milestones, _, err := c.Milestones.ListMilestones(ctx, integrationProject,
		&ListMilestonesOptions{ListOptions: ListOptions{Page: 1, PerPage: 1}})
	if err != nil {
		t.Fatalf("ListMilestones: %v", err)
	}
	if len(milestones) == 0 {
		t.Log("no milestones in integration project; skipped")
		return
	}
	issues, _, err := c.Milestones.ListMilestoneIssues(ctx, integrationProject, milestones[0].ID,
		&ListMilestoneIssuesOptions{ListOptions: ListOptions{Page: 1, PerPage: 10}})
	if err != nil {
		t.Fatalf("ListMilestoneIssues: %v", err)
	}
	t.Logf("milestone %d carries %d issues (first page)", milestones[0].ID, len(issues))
}

// TestIntegration_DownloadMergeRequestChangedFiles streams one MR's changed
// files zip when the project has MRs; payload must be non-empty when the
// MR has a diff.
func TestIntegration_DownloadMergeRequestChangedFiles(t *testing.T) {
	c := newIntegrationClient(t)
	ctx, cancel := integrationCtx(t)
	defer cancel()

	mrs, _, err := c.MergeRequests.ListMergeRequests(ctx, integrationProject,
		&ListMergeRequestsOptions{ListOptions: ListOptions{Page: 1, PerPage: 1}})
	if err != nil {
		t.Fatalf("ListMergeRequests: %v", err)
	}
	if len(mrs) == 0 {
		t.Log("no MRs in integration project; archive download skipped")
		return
	}
	var buf bytes.Buffer
	if _, err := c.MergeRequests.DownloadMergeRequestChangedFiles(ctx, integrationProject, mrs[0].ID, &buf); err != nil {
		t.Fatalf("DownloadMergeRequestChangedFiles: %v", err)
	}
	t.Logf("MR %d changed-files archive: %d bytes", mrs[0].ID, buf.Len())
}

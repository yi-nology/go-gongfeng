package gongfeng

import (
	"context"
	"fmt"
	"net/http"

	oekiterrors "github.com/oekit/pkg/errors"
)

// ReleaseRef 表示 Release 关联的 Tag 引用信息。
type ReleaseRef struct {
	Name    string  `json:"name,omitempty"`
	Message string  `json:"message,omitempty"`
	Commit  *Commit `json:"commit,omitempty"`
}

// Release 表示一个项目发布版本。
type Release struct {
	ID          int         `json:"id,omitempty"`
	ProjectID   int         `json:"project_id,omitempty"`
	Tag         string      `json:"tag,omitempty"`
	Title       string      `json:"title,omitempty"`
	Type        string      `json:"type,omitempty"`
	Description string      `json:"description,omitempty"`
	Attachments interface{} `json:"attachments,omitempty"`
	Ref         *ReleaseRef `json:"ref,omitempty"`
	CreatedAt   Time        `json:"created_at,omitempty"`
	UpdatedAt   Time        `json:"updated_at,omitempty"`
	Author      *User       `json:"author,omitempty"`
}

// ReleasesService 处理与 Release 相关的 API 调用。
type ReleasesService struct {
	client *Client
}

// ListReleasesOptions 是 ListReleases 的可选参数。
type ListReleasesOptions struct {
	ListOptions
}

// ListReleases 获取项目的所有 Release。
func (s *ReleasesService) ListReleases(ctx context.Context, pid interface{}, opts *ListReleasesOptions) ([]*Release, *Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, nil, err
	}
	path := fmt.Sprintf("projects/%s/releases", project)

	req, err := s.client.NewRequest(ctx, http.MethodGet, path, opts)
	if err != nil {
		return nil, nil, err
	}

	var releases []*Release
	resp, err := s.client.Do(req, &releases)
	if err != nil {
		return nil, resp, err
	}

	return releases, resp, nil
}

// GetRelease 获取项目中指定 ID 的 Release。
func (s *ReleasesService) GetRelease(ctx context.Context, pid interface{}, releaseID int) (*Release, *Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, nil, err
	}
	path := fmt.Sprintf("projects/%s/releases/%d", project, releaseID)

	req, err := s.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}

	var release Release
	resp, err := s.client.Do(req, &release)
	if err != nil {
		return nil, resp, err
	}

	return &release, resp, nil
}

// ErrReleaseNotFound 在项目下没有匹配 tag 的 Release 时由 GetReleaseByTag
// 返回（工蜂 release 端点只支持 ID 寻址，by-tag 只能在 SDK 层判定）。
var ErrReleaseNotFound = oekiterrors.New("gongfeng: release not found for tag")

// GetReleaseByTag 按 tag 取单个 Release：工蜂 wire 没有 by-tag 端点，
// 这里用 Paginate 扫全量列表匹配 tag，把这个 O(n) detour 收进 SDK、
// 所有消费方共享。命中即返回（无缓存，列表多时每次都是全量扫描；
// 重复查询请自行缓存返回的 Release.ID，后续走 GetRelease/UpdateRelease/
// DeleteRelease 仍是 ID 寻址）。未命中返回 ErrReleaseNotFound，可用
// errors.Is 判定。
func (s *ReleasesService) GetReleaseByTag(ctx context.Context, pid interface{}, tag string) (*Release, *Response, error) {
	releases, err := Paginate(func(page int) ([]*Release, *Response, error) {
		return s.ListReleases(ctx, pid, &ListReleasesOptions{
			ListOptions: ListOptions{Page: page, PerPage: 100},
		})
	})
	if err != nil {
		return nil, nil, err
	}
	for _, r := range releases {
		if r != nil && r.Tag == tag {
			return r, nil, nil
		}
	}
	return nil, nil, oekiterrors.Wrap(ErrReleaseNotFound, "gongfeng: no release matched tag", "tag", tag)
}

// CreateReleaseOptions 是 CreateRelease 的可选参数。
type CreateReleaseOptions struct {
	Tag         *string `json:"tag,omitempty" url:"tag,omitempty"`
	StartPoint  *string `json:"start_point,omitempty" url:"start_point,omitempty"`
	Title       *string `json:"title,omitempty" url:"title,omitempty"`
	Type        *string `json:"type,omitempty" url:"type,omitempty"`
	Description *string `json:"description,omitempty" url:"description,omitempty"`
}

// CreateRelease 在项目中创建一个新的 Release。
func (s *ReleasesService) CreateRelease(ctx context.Context, pid interface{}, opts *CreateReleaseOptions) (*Release, *Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, nil, err
	}
	path := fmt.Sprintf("projects/%s/releases", project)

	req, err := s.client.NewRequest(ctx, http.MethodPost, path, opts)
	if err != nil {
		return nil, nil, err
	}

	var release Release
	resp, err := s.client.Do(req, &release)
	if err != nil {
		return nil, resp, err
	}

	return &release, resp, nil
}

// UpdateReleaseOptions 是 UpdateRelease 的可选参数。
type UpdateReleaseOptions struct {
	Description *string `json:"description,omitempty" url:"description,omitempty"`
}

// UpdateRelease 更新项目中指定 ID 的 Release。
func (s *ReleasesService) UpdateRelease(ctx context.Context, pid interface{}, releaseID int, opts *UpdateReleaseOptions) (*Release, *Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, nil, err
	}
	path := fmt.Sprintf("projects/%s/releases/%d", project, releaseID)

	req, err := s.client.NewRequest(ctx, http.MethodPut, path, opts)
	if err != nil {
		return nil, nil, err
	}

	var release Release
	resp, err := s.client.Do(req, &release)
	if err != nil {
		return nil, resp, err
	}

	return &release, resp, nil
}

// DeleteRelease 删除项目中指定 ID 的 Release。
func (s *ReleasesService) DeleteRelease(ctx context.Context, pid interface{}, releaseID int) (*Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("projects/%s/releases/%d", project, releaseID)

	req, err := s.client.NewRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return nil, err
	}

	return s.client.Do(req, nil)
}

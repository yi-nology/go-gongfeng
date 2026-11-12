package gongfeng

import (
	"fmt"

	oekiterrors "github.com/oekit/pkg/errors"
)

// maxPaginatePages 封顶分页游走（与消费方 go-git-platform 的 AllPages
// 预算一致）：per_page=100 × 50 页远超任何真实列表，服务端异常导致
// NextPage 永不清零时在此止步。
const maxPaginatePages = 50

// Paginate 走完一个列表端点的全部分页：fetch 负责拉取单页（page 从 1
// 起），SDK 依据响应的 X-Next-Page 头翻页，直到取完或触及
// maxPaginatePages 预算。两条错误路径（单页失败 / 触及页数预算）都
// 以 oekiterrors 包装返回，且**已取回的页不回滚**——err != nil 时
// 返回的 slice 是中断前已成功拉取的部分结果，调用方可先在部分结果
// 里找目标、再决定如何处置错误（issue #3：单页失败一支此前返回
// nil，恰与本文档相反）。
//
//	client.Paginate(func(page int) ([]*gongfeng.Project, *gongfeng.Response, error) {
//	    return client.Projects.ListProjects(ctx, &gongfeng.ListProjectsOptions{
//	        ListOptions: gongfeng.ListOptions{Page: page, PerPage: 100},
//	    })
//	})
func Paginate[T any](fetch func(page int) ([]T, *Response, error)) ([]T, error) {
	var all []T
	for page := 1; page <= maxPaginatePages; page++ {
		list, resp, err := fetch(page)
		if err != nil {
			// 已取回的页不回滚：与触及预算一支同口径，返回部分结果
			// + 包装错误（issue #3）。
			return all, oekiterrors.Wrap(err, "gongfeng: fetch page failed", "page", page)
		}
		all = append(all, list...)
		if resp == nil || resp.NextPage == 0 {
			return all, nil
		}
	}
	return all, fmt.Errorf("gongfeng: pagination exceeded %d pages", maxPaginatePages)
}

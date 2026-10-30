# go-gongfeng

腾讯工蜂（Tencent CODE / git.code.tencent.com）REST API 的 Go 客户端。

> 文档来源：官方 API（https://code.tencent.com/help/api/prepare ），完整镜像
> 见 [docs/api/](docs/api/)（22 个能力域：prepare / projects / groups /
> namespaces / users / session / issues / labels / milestones / watchers /
> repositories / branches / tags / commits / commit_check / commit_review /
> releases / webhooks / merge_requests / notes / mr_review / forks）。
> 上游删过 tag 的教训同样适用于文档，镜像随仓库留存。

## 身世（lineage）

- 源起 [studyzy/gongfeng-sdk-go](https://github.com/studyzy/gongfeng-sdk-go)
  v0.6.0（官方随后删除了该 tag）；
- [zy84338719/gongfeng-sdk-go](https://github.com/zy84338719/gongfeng-sdk-go)
  按官方文档重建并修正（Release 改 ID 寻址、commit status 与 MR 路径修正、
  移除官方不存在的端点、补齐 review_events/token 等字段，带
  `GONGFENG_TOKEN` 真实环境集成测试）；
- 本仓是其收编延续：叠加了 oekit 适配层（见下），作为独立组件供
  [go-git-platform](https://github.com/yi-nology/go-git-platform) 的
  Tencent Code backend 依赖。

## 快速上手

```go
client, err := gongfeng.NewClient(token,
    gongfeng.WithBaseURL("https://git.code.tencent.com"), // 默认即公有云
    gongfeng.WithLogger(myLogger), // 可选；默认 noop
)
// 20 个服务域挂在 Client 上：
mr, _, err := client.MergeRequests.CreateMergeRequest(ctx, "owner/repo",
    &gongfeng.CreateMergeRequestOptions{SourceBranch: gongfeng.Ptr("f"), TargetBranch: gongfeng.Ptr("main")})
```

认证：`PRIVATE-TOKEN` 头（每个请求自动携带）。所有列表接口支持
`page` / `per_page` 分页，`Response` 解析 `X-Total` / `X-Total-Pages` 等
分页头。

## 能力增强

**分页游走**——`Paginate` 依据 `X-Next-Page` 走完整个列表（50 页预算
封顶，单页失败带 page kv 报错）：

```go
projects, err := gongfeng.Paginate(func(page int) ([]*gongfeng.Project, *gongfeng.Response, error) {
    return client.Projects.ListProjects(ctx, &gongfeng.ListProjectsOptions{
        ListOptions: gongfeng.ListOptions{Page: page, PerPage: 100},
    })
})
```

**可选重试**——`WithRetry(attempts)` 对幂等请求（GET/HEAD）的
网络错误与 429/500/502/503/504 自动指数退避（尊重 `Retry-After`，单次
封顶 10s）；写请求永不重放。默认关闭。

**错误判定**——`IsNotFound` / `IsUnauthorized` / `IsForbidden` /
`IsRateLimited` / `StatusCode(err)` 基于类型化 `*ErrorResponse` 的
errors.As 链判定，跨包装可用。

## 端点覆盖审计

以 docs/api/ 官方文档镜像为准做了端点级比对（143 个 METHOD+path）：
SDK 覆盖率 **100%**。两个已知的文档侧错误在 SDK 中按实际情况处理——
`GET projects/:id/merge_requests/:id/changed_files`（复数旧路径，官方
文档遗留；SDK 走官方当前生效的 `merge_request` 单数形态）与
`PUT …/reviews/:id/notes/note_id`（文档笔误，`note_id` 缺 `:` 占位）。

## 设计约定

- **错误**走 `github.com/oekit/pkg/errors`：带格式的构造（含 `%w`）用
  `oekiterrors.Errorf`，静态错误用 `oekiterrors.New`；
- **日志**：`Client` 接受最小 kv 风格 `Logger` 接口（4 方法，默认 noop），
  与 `oekit/logx`、宿主工程的同构接口结构兼容，注入无需适配层；`Do` 记录
  请求 debug 行（method/path/status/耗时）与失败 error 行；
- **并发**：SDK 保持无 goroutine 的同步设计；有界并发由消费方承担
  （例如 go-git-platform 的批量原语）；
- **TLS**：默认客户端启用 TLS 1.2 + 宽密码套件（兼容部分工蜂实例的旧
  TLS 配置）；可用 `WithHTTPClient` 整体替换。

## 升级上游

同步 fork/上游提交后整体覆盖 `*.go`（保留 `logger.go` 与 go.mod），重新
套用 oekit 适配层，并保持 LICENSE（Apache-2.0，承自上游）与 docs/api 镜像。

## License

Apache-2.0（承自上游 SDK）。

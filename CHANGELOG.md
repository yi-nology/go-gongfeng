# Changelog

All notable changes to this project are documented in this file. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

### Changed

- 依赖升级：`github.com/oekit/pkg` v0.28.0 → v0.42.0（`go get -u ./...`，
  全量测试通过，无破坏性变更）。

### Fixed

- **Paginate 单页失败不再回滚已取回的页（issue #3）**：单页失败一支此前
  返回 `nil`，与自身 doc「已取回的页不回滚」直接相反（触及页数预算
  一支却返回部分结果，两条错误路径口径相反）。现在统一为「部分结果
  + 包装错误」：`err != nil` 时返回的 slice 是中断前已成功拉取的部分。
  `GetReleaseByTag` 同步改为先在已取回的部分里找 tag、命中即返回
  （网络抖动不再丢掉已扫到的 release——工蜂无 by-tag 端点，下游无路
  可绕）；未命中且 `err != nil` 时错误优先于 `ErrReleaseNotFound`
  上抛，调用方可区分「列表没扫完」与「扫完了但没有」。
- **backoffSleep 优先采用服务端 Retry-After（issue #4）**：旧条件
  `ra < delay` 只在服务端建议**更短**时才采用，限流窗口的常态（服务端
  要求等 3s/60s、本地退避才 200ms/400ms）下 Retry-After 被完全忽略，
  SDK 在限流窗口内持续撞墙，与注释「优先采用」相反。取舍逻辑抽为
  `retryDelay`：响应携带合法 Retry-After 时一律采用服务端建议（无论
  长短，封顶 `maxRetryDelay`），否则本地指数退避（同样封顶）。
  `retryAfter` 解析顺带支持 HTTP-date 形式（`http.ParseTime`，过去的
  日期拒绝），与姊妹仓 go-gitcode 的口径对齐。

## [0.3.0] - 2026-11-06
### Added

- `Releases.GetReleaseByTag(ctx, pid, tag)`：按 tag 取单个 Release 的语法糖
  （工蜂 wire 无 by-tag 端点，SDK 内用 `Paginate` 扫列表匹配，把下游
  GetReleaseByTag/UpdateRelease/DeleteRelease 三条统一面的「扫列表解析 ID」
  detour 收进上游共享）。命中即返回、无缓存；未命中返回哨兵
  `ErrReleaseNotFound`（`errors.Is` 可判定）。附三组单测（命中/未命中/跨页）。
- `UpdateMergeRequestOptions.AssigneeID` 字段注释显式标注：工蜂 MR 更新
  wire 只有单 `assignee_id`，多指派不可表达，防下游按 GitLab `assignee_ids`
  血统误传数组（go-git-platform 分歧台账登记的两处 wire 缺口，见 issue #1）。
## [0.2.1] - 2026-10-30

### Fixed

- **重试耗尽现按注释契约返回 `errRetryExhausted` 包装**：`WithRetry` 预算耗尽
  （开重试、最后一次失败仍属可重试类）时错误经 `oekiterrors.Wrap` 标注
  "retries exhausted" 并带 `attempts` kv——该哨兵在 v0.2.0 已写但未接线，属于
  遗漏。Wrap 保 cause 链，`IsNotFound`/`IsRateLimited` 等 errors.As 判定不受
  影响；单次尝试（默认不开重试）与 body 不可重放的提前收尾仍原样返回裸错误。

### Added

- 选项与错误面测试补齐：`WithHTTPClient`（自定义传输实挂验证）、`WithLogger`
  （注入 debug 行 + nil 回落 noop）、`ErrorResponse.Error` 文案、noop logger
  零 panic；重试耗尽断言扩到包装文案与 cause 链。覆盖率 75.9% → 76.5%（剩余
  0% 项为 noop 空函数体的统计伪影，无语句可覆盖）。
- `examples/quickstart`：可运行示例（客户端构造 / 分页游走 / 错误分类 /
  webhook 增删查改），README 快速上手已链接。

## [0.2.0] - 2026-10-30

### Added

- `Paginate[T]`：沿 `X-Next-Page` 游走完整列表（50 页预算封顶；单页失败带
  page kv 包装）。
- `WithRetry(attempts)`：幂等请求（GET/HEAD）对网络错误与 429/500/502/503/504
  指数退避重放（尊重 `Retry-After`，单次封顶 10s）；写请求与 body 不可重建的
  请求永不重放；默认关闭（1 = off），上限 5。
- 错误判定助手：`StatusCode`/`IsNotFound`/`IsUnauthorized`/`IsForbidden`/
  `IsRateLimited`，基于类型化 `*ErrorResponse` 的 errors.As 链；`retryAfter`
  解析器。
- 11 个能力测试（分页游走/封顶/错误包装、重试恢复/永不重放写/跳过 404/耗尽/
  默认关闭、错误助手、Retry-After 解析）。

## [0.1.0] - 2026-10-30

### Added

- 工蜂 REST SDK 独立组件。lineage：studyzy/gongfeng-sdk-go v0.6.0（上游已删
  tag）→ zy84338719/gongfeng-sdk-go（官方文档对齐修正 + 真实环境集成测试）→
  本组件。20 个服务域 143 个官方文档端点 100% 覆盖（审计方式见 README）。
- SDK 自有最小 kv 风格 `Logger` 接口（默认 noop）+ `WithLogger` 注入与请求
  debug/error 日志行，打断对下游 adapter 的依赖方向。
- 错误面 oekit/pkg errors（带格式构造 `Errorf`、静态 `New`）。
- 官方 API 文档镜像（22 个能力域，docs/api/）。

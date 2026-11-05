# Changelog

All notable changes to this project are documented in this file. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

### Changed

### Fixed

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

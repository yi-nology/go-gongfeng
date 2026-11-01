// Command quickstart 演示 go-gongfeng 的核心用法：构造客户端、分页游走、
// 可选重试、错误分类与 webhook 增删查改。全部只依赖本 SDK 与标准库；
// token 从环境变量 GONGFENG_TOKEN 读取，BaseURL 可用 GONGFENG_BASE_URL
// 覆盖（默认 https://git.code.tencent.com）。
//
// 运行：
//
//	GONGFENG_TOKEN=xxx go run examples/quickstart/main.go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	gongfeng "github.com/yi-nology/go-gongfeng"
)

func main() {
	token := os.Getenv("GONGFENG_TOKEN")
	if token == "" {
		log.Fatal("GONGFENG_TOKEN is required")
	}

	options := []gongfeng.ClientOptionFunc{
		// 幂等请求（GET/HEAD）对网络错误与 5xx/429 自动退避重试，
		// 写请求永不重放；不开重试是默认行为。
		gongfeng.WithRetry(3),
		// 调试日志：Logger 是 4 方法 kv 风格最小接口，oekit logx /
		// slog 等的主流形状可直接注入（此处用标准库演示）。
		gongfeng.WithLogger(stdLogger{}),
	}
	if base := os.Getenv("GONGFENG_BASE_URL"); base != "" {
		options = append(options, gongfeng.WithBaseURL(base))
	}
	client, err := gongfeng.NewClient(token, options...)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1) 跨项目列出名下项目：Paginate 沿 X-Next-Page 游走全部页，
	//    单页失败会带 page kv 包装错误。
	projects, err := gongfeng.Paginate(func(page int) ([]*gongfeng.Project, *gongfeng.Response, error) {
		return client.Projects.ListOwnedProjects(ctx,
			&gongfeng.ListProjectsOptions{ListOptions: gongfeng.ListOptions{Page: page, PerPage: 30}})
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("owned projects: %d\n", len(projects))
	for i, p := range projects {
		if i >= 5 {
			fmt.Printf("  … and %d more\n", len(projects)-5)
			break
		}
		fmt.Printf("  #%d %s (%s)\n", p.ID, p.Path, p.Description)
	}

	// 2) 错误分类：errors.As 链上判定，不匹配消息文本。
	if _, _, err := client.Projects.GetProject(ctx, "no-such-org/no-such-repo"); err != nil {
		switch {
		case gongfeng.IsNotFound(err):
			fmt.Println("missing project classified as NotFound (as expected)")
		case gongfeng.IsUnauthorized(err):
			log.Fatal("token invalid")
		default:
			log.Printf("other error: %v", err)
		}
	}

	// 3) webhook 增删查改（idempotent demo）：建 → 查 → 改事件 → 删。
	const demoRepo = "no-such-org/no-such-repo" // 换成你有权限的仓库路径
	url := "https://ci.example.com/hook"
	if _, _, err := client.Webhooks.AddWebhook(ctx, demoRepo, &gongfeng.AddWebhookOptions{
		URL:        gongfeng.Ptr(url),
		PushEvents: gongfeng.Ptr(true),
	}); err != nil {
		if !gongfeng.IsNotFound(err) && !gongfeng.IsForbidden(err) {
			log.Fatal(err)
		}
		fmt.Println("webhook demo skipped (repo unavailable in this environment)")
		return
	}
	hooks, err := gongfeng.Paginate(func(page int) ([]*gongfeng.Webhook, *gongfeng.Response, error) {
		return client.Webhooks.ListWebhooks(ctx, demoRepo,
			&gongfeng.ListWebhooksOptions{ListOptions: gongfeng.ListOptions{Page: page, PerPage: 100}})
	})
	if err != nil {
		log.Fatal(err)
	}
	for _, h := range hooks {
		if _, _, err := client.Webhooks.EditWebhook(ctx, demoRepo, h.ID,
			&gongfeng.EditWebhookOptions{TagPushEvents: gongfeng.Ptr(true)}); err != nil {
			log.Fatal(err)
		}
		if _, err := client.Webhooks.DeleteWebhook(ctx, demoRepo, h.ID); err != nil {
			log.Fatal(err)
		}
	}
	fmt.Println("webhook CRUD round-trip ok")
}

// stdLogger 把 SDK 的 kv 风格日志行打到标准 log。
type stdLogger struct{}

func (stdLogger) Debug(msg string, kv ...any) { logLine("DEBUG", msg, kv) }
func (stdLogger) Info(msg string, kv ...any)  { logLine("INFO", msg, kv) }
func (stdLogger) Warn(msg string, kv ...any)  { logLine("WARN", msg, kv) }
func (stdLogger) Error(msg string, kv ...any) { logLine("ERROR", msg, kv) }

func logLine(level, msg string, kv []any) {
	line := level + " " + msg
	for i := 0; i+1 < len(kv); i += 2 {
		line += fmt.Sprintf(" %v=%v", kv[i], kv[i+1])
	}
	log.Println(line)
}

// Package gongfeng 提供腾讯工蜂 REST API 的 Go 客户端。
package gongfeng

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	oekiterrors "github.com/oekit/pkg/errors"

	"github.com/google/go-querystring/query"
)

const (
	// DefaultBaseURL 是工蜂公有云的默认地址。
	DefaultBaseURL = "https://git.code.tencent.com/"

	apiVersionPath = "api/v3"
	userAgent      = "gongfeng-sdk-go"

	// maxRetryAttempts 封顶 WithRetry 的尝试次数。
	maxRetryAttempts = 5
	// defaultRetryBaseDelay 是指数退避的基数。
	defaultRetryBaseDelay = 200 * time.Millisecond
	// maxRetryDelay 封顶单次退避时长（含 Retry-After 携带的值）。
	maxRetryDelay = 10 * time.Second
)

// Client 是腾讯工蜂 API 的客户端入口。
type Client struct {
	// baseURL 是 API 的基础 URL，包含 /api/v3/ 路径。
	baseURL *url.URL

	// token 是用于认证的 PRIVATE-TOKEN。
	token string

	// httpClient 是执行 HTTP 请求的客户端。
	httpClient *http.Client

	// userAgent 是请求中使用的 User-Agent 标头值。
	userAgent string

	// logger 是可选的调试日志出口（Logger，kv 风格；默认 noop）。
	// 注入方传 oekit/logx 或宿主的同构接口实现即可。
	logger Logger

	// retryMax 是幂等请求（GET/HEAD）的最大尝试次数（含首次），1 = 关闭
	// 重试（默认）；上限 maxRetryAttempts。retryBaseDelay 是指数退避基数。
	retryMax       int
	retryBaseDelay time.Duration

	// Services

	Branches       *BranchesService
	CommitStatuses *CommitStatusService
	Commits        *CommitsService
	Forks          *ForksService
	Groups         *GroupsService
	Issues         *IssuesService
	Labels         *LabelsService
	MergeRequests  *MergeRequestsService
	Milestones     *MilestonesService
	Namespaces     *NamespacesService
	Notes          *NotesService
	Projects       *ProjectsService
	Releases       *ReleasesService
	Repositories   *RepositoriesService
	Reviews        *ReviewsService
	Session        *SessionService
	Tags           *TagsService
	Users          *UsersService
	Watchers       *WatchersService
	Webhooks       *WebhooksService
}

// ClientOptionFunc 是客户端的函数式选项。
type ClientOptionFunc func(*Client) error

// WithBaseURL 设置工蜂实例的 URL。
func WithBaseURL(urlStr string) ClientOptionFunc {
	return func(c *Client) error {
		return c.setBaseURL(urlStr)
	}
}

// WithRetry 打开幂等请求（GET/HEAD）的自动重试：对网络错误与
// 429/500/502/503/504 响应按指数退避重试（尊重 Retry-After，单次封顶
// 10s）。attempts 含首次请求，1 = 关闭（默认），上限 5。写请求永不重放。
func WithRetry(attempts int) ClientOptionFunc {
	return func(c *Client) error {
		if attempts < 1 {
			attempts = 1
		}
		if attempts > maxRetryAttempts {
			attempts = maxRetryAttempts
		}
		c.retryMax = attempts
		return nil
	}
}

// WithHTTPClient 设置自定义的 HTTP 客户端。
func WithHTTPClient(httpClient *http.Client) ClientOptionFunc {
	return func(c *Client) error {
		c.httpClient = httpClient
		return nil
	}
}

// WithLogger 注入调试日志出口（Logger，kv 风格）。默认 noop：不传即静默。
// oekit 生态消费方可传 logx.Delegating(component)（与 Logger 结构兼容）。
func WithLogger(logger Logger) ClientOptionFunc {
	return func(c *Client) error {
		if logger == nil {
			return nil
		}
		c.logger = logger
		return nil
	}
}

// NewClient 创建一个工蜂 API 客户端。token 为用户的 Private Token。
func NewClient(token string, options ...ClientOptionFunc) (*Client, error) {
	if strings.TrimSpace(token) == "" {
		return nil, oekiterrors.New("gongfeng: token is required")
	}

	c := &Client{
		token:          token,
		userAgent:      userAgent,
		logger:         NewNoopLogger(),
		retryMax:       1,
		retryBaseDelay: defaultRetryBaseDelay,
	}

	// 设置默认 BaseURL
	if err := c.setBaseURL(DefaultBaseURL); err != nil {
		return nil, err
	}

	// 应用选项
	for _, opt := range options {
		if err := opt(c); err != nil {
			return nil, err
		}
	}

	if c.httpClient == nil {
		c.httpClient = newDefaultHTTPClient()
	}

	// 初始化所有 Service
	c.Branches = &BranchesService{client: c}
	c.CommitStatuses = &CommitStatusService{client: c}
	c.Commits = &CommitsService{client: c}
	c.Forks = &ForksService{client: c}
	c.Groups = &GroupsService{client: c}
	c.Issues = &IssuesService{client: c}
	c.Labels = &LabelsService{client: c}
	c.MergeRequests = &MergeRequestsService{client: c}
	c.Milestones = &MilestonesService{client: c}
	c.Namespaces = &NamespacesService{client: c}
	c.Notes = &NotesService{client: c}
	c.Projects = &ProjectsService{client: c}
	c.Releases = &ReleasesService{client: c}
	c.Repositories = &RepositoriesService{client: c}
	c.Reviews = &ReviewsService{client: c}
	c.Session = &SessionService{client: c}
	c.Tags = &TagsService{client: c}
	c.Users = &UsersService{client: c}
	c.Watchers = &WatchersService{client: c}
	c.Webhooks = &WebhooksService{client: c}

	return c, nil
}

// newDefaultHTTPClient 创建默认 HTTP 客户端，启用兼容旧版 TLS 密码套件，
// 以支持部分工蜂实例使用的较旧 TLS 配置。
func newDefaultHTTPClient() *http.Client {
	var suites []uint16
	for _, s := range tls.CipherSuites() {
		suites = append(suites, s.ID)
	}
	for _, s := range tls.InsecureCipherSuites() {
		suites = append(suites, s.ID)
	}
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion:   tls.VersionTLS12,
				CipherSuites: suites,
			},
		},
	}
}

// setBaseURL 解析并设置基础 URL，自动拼接 /api/v3/ 路径。
func (c *Client) setBaseURL(urlStr string) error {
	if !strings.HasSuffix(urlStr, "/") {
		urlStr += "/"
	}

	baseURL, err := url.Parse(urlStr)
	if err != nil {
		return oekiterrors.Errorf("gongfeng: invalid base url %q: %w", urlStr, err)
	}

	// 拼接 api/v3/ 路径
	baseURL.Path += apiVersionPath + "/"
	c.baseURL = baseURL

	return nil
}

// Response 包装 http.Response，附加工蜂 API 的分页信息。
type Response struct {
	*http.Response

	// TotalItems 是查询结果的总条目数。
	TotalItems int

	// TotalPages 是总页数。
	TotalPages int

	// ItemsPerPage 是每页条目数。
	ItemsPerPage int

	// CurrentPage 是当前页码（从 1 开始）。
	CurrentPage int

	// NextPage 是下一页页码。
	NextPage int

	// PreviousPage 是上一页页码。
	PreviousPage int
}

// newResponse 从 http.Response 创建 Response，解析分页 Header。
func newResponse(r *http.Response) *Response {
	resp := &Response{Response: r}
	resp.TotalItems = parseIntHeader(r, "X-Total")
	resp.TotalPages = parseIntHeader(r, "X-Total-Pages")
	resp.ItemsPerPage = parseIntHeader(r, "X-Per-Page")
	resp.CurrentPage = parseIntHeader(r, "X-Page")
	resp.NextPage = parseIntHeader(r, "X-Next-Page")
	resp.PreviousPage = parseIntHeader(r, "X-Prev-Page")
	return resp
}

func parseIntHeader(r *http.Response, key string) int {
	v, _ := strconv.Atoi(r.Header.Get(key))
	return v
}

// ErrorResponse 表示工蜂 API 返回的错误。
type ErrorResponse struct {
	Response *http.Response
	Message  string `json:"message"`
}

// Error 实现 error 接口。
func (e *ErrorResponse) Error() string {
	return fmt.Sprintf("%s %s: %d %s",
		e.Response.Request.Method,
		e.Response.Request.URL,
		e.Response.StatusCode,
		e.Message,
	)
}

// NewRequest 构造一个 API 请求。
//
// path 是相对于 /api/v3/ 的路径（不带前导 /）。
// 对于 GET 请求，body 会被编码为 URL query 参数。
// 对于其他方法，body 会被 JSON 编码为请求体。
func (c *Client) NewRequest(ctx context.Context, method, path string, body interface{}) (*http.Request, error) {
	u := *c.baseURL
	unescaped, err := url.PathUnescape(path)
	if err != nil {
		return nil, err
	}

	// 解析相对路径并拼接到 baseURL
	u.RawPath = c.baseURL.Path + path
	u.Path = c.baseURL.Path + unescaped

	reqHeaders := make(http.Header)
	reqHeaders.Set("PRIVATE-TOKEN", c.token)
	reqHeaders.Set("User-Agent", c.userAgent)

	var reqBody io.Reader
	switch {
	case method == http.MethodGet && body != nil:
		q, err := query.Values(body)
		if err != nil {
			return nil, err
		}
		u.RawQuery = q.Encode()
	case body != nil:
		reqHeaders.Set("Content-Type", "application/json")
		buf := new(bytes.Buffer)
		if err := json.NewEncoder(buf).Encode(body); err != nil {
			return nil, err
		}
		reqBody = buf
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), reqBody)
	if err != nil {
		return nil, err
	}

	for k, v := range reqHeaders {
		req.Header[k] = v
	}

	return req, nil
}

// Do 执行 API 请求并将 JSON 响应解码到 v。
// 如果 v 为 nil 则不解码响应体。
// 如果 v 实现了 io.Writer，则将响应体直接写入。
//
// 经 WithRetry 打开重试后，幂等请求（GET/HEAD）对网络错误与
// 429/500/502/503/504 自动退避重放；写请求与 body 不可重建的请求永不
// 重放。
func (c *Client) Do(req *http.Request, v interface{}) (*Response, error) {
	attempts := c.retryMax
	if attempts < 1 {
		attempts = 1
	}

	var resp *Response
	var err error
	for attempt := 1; ; attempt++ {
		attemptReq := req
		if attempt > 1 && req.Body != nil {
			// 有请求体的重放需要可重建的 body；无 body 的 GET/HEAD 直接
			// 复用原请求。
			if req.GetBody == nil {
				break // body 不可重建：无法重放，用上一次结果收尾
			}
			cloned := req.Clone(req.Context())
			body, berr := req.GetBody()
			if berr != nil {
				break
			}
			cloned.Body = io.NopCloser(body)
			attemptReq = cloned
		}

		resp, err = c.execute(attemptReq, v)

		if attempt >= attempts || !c.retryable(req, resp, err) {
			// 重试预算内耗尽（开了重试、最后一次失败仍属可重试类）才标
			// exhausted：单次尝试（未开重试）与 body 不可重放的提前收尾
			// 都原样返回。Wrap 保 cause 链，IsNotFound/IsRateLimited 等
			// 判定不受影响。
			if err != nil && attempts > 1 && attempt >= attempts && c.retryable(req, resp, err) {
				return resp, errRetryExhausted(attempts, err)
			}
			return resp, err
		}
		c.logger.Warn("gongfeng retrying request",
			"method", req.Method, "path", req.URL.Path, "attempt", attempt)
		c.backoffSleep(req.Context(), attempt, resp)
	}
	return resp, err
}

// execute 发起单次 HTTP 请求并解码响应（Do 的无重试内核）。
func (c *Client) execute(req *http.Request, v interface{}) (*Response, error) {
	start := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("gongfeng request failed",
			"method", req.Method, "path", req.URL.Path, "err", err)
		return nil, err
	}
	defer resp.Body.Close()

	response := newResponse(resp)

	c.logger.Debug("gongfeng request",
		"method", req.Method, "path", req.URL.Path,
		"status", resp.StatusCode, "duration_ms", time.Since(start).Milliseconds())

	if err := checkResponse(resp); err != nil {
		return response, err
	}

	if v != nil {
		if w, ok := v.(io.Writer); ok {
			_, err = io.Copy(w, resp.Body)
		} else {
			err = json.NewDecoder(resp.Body).Decode(v)
		}
	}

	return response, err
}

// retryable 判定一次尝试是否值得重放：仅幂等动词；网络错误（execute 未
// 产出响应）可重试；HTTP 层面仅 429/500/502/503/504 可重试，其余 4xx 不
// 重放。注意 SDK 约定 HTTP 错误时 resp 与 err 同时非空，故不能以 err 单
// 独判定。
func (c *Client) retryable(req *http.Request, resp *Response, err error) bool {
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		return false
	}
	if resp == nil || resp.Response == nil {
		return err != nil
	}
	switch resp.Response.StatusCode {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}
	return false
}

// retryDelay 计算下一次尝试前的等待时长：响应携带 Retry-After 时
// **服务端建议优先**（无论比本地指数退避长还是短，一律封顶
// maxRetryDelay），否则 base × 2^(attempt-1)（同样封顶）。
//
// 取舍方向是修过的 bug（issue #4）：旧条件 `ra < delay` 只在服务端
// 建议更短时才采用，而限流窗口的常态恰恰是服务端要求等得更久
// （`Retry-After: 60` 对上本地退避 1s/2s/4s）——Retry-After 被完全
// 忽略，SDK 在服务端限流窗口内持续撞墙，与「优先采用」的注释相反。
func (c *Client) retryDelay(attempt int, resp *Response) time.Duration {
	delay := min(c.retryBaseDelay<<(attempt-1), maxRetryDelay)
	if resp != nil && resp.Response != nil {
		if ra, ok := retryAfter(resp.Response); ok {
			delay = min(ra, maxRetryDelay)
		}
	}
	return delay
}

// backoffSleep 按 retryDelay 的计算结果退避等待。ctx 取消时提前返回
// ——下一次请求会自然携带取消错误失败。
func (c *Client) backoffSleep(ctx context.Context, attempt int, resp *Response) {
	timer := time.NewTimer(c.retryDelay(attempt, resp))
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

// checkResponse 检查 HTTP 状态码，非 2xx 返回 ErrorResponse。
func checkResponse(r *http.Response) error {
	if c := r.StatusCode; c >= 200 && c <= 299 {
		return nil
	}

	errorResponse := &ErrorResponse{Response: r}
	data, err := io.ReadAll(r.Body)
	if err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, errorResponse)
		if errorResponse.Message == "" {
			errorResponse.Message = string(data)
		}
	}
	// 替换 body 以便后续读取
	r.Body = io.NopCloser(bytes.NewBuffer(data))

	return errorResponse
}

// pathEscape 对路径参数进行 URL 编码。
func pathEscape(s string) string {
	return url.PathEscape(s)
}

// parseID 将项目/资源 ID 转换为 URL 路径段。
// 支持 int（直接转字符串）和 string（URL 编码，用于 namespace/project 格式）。
func parseID(id interface{}) (string, error) {
	switch v := id.(type) {
	case int:
		return strconv.Itoa(v), nil
	case string:
		return pathEscape(v), nil
	default:
		return "", oekiterrors.Errorf("gongfeng: invalid ID type %T, expected int or string", id)
	}
}

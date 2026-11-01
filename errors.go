package gongfeng

import (
	"errors"
	oekiterrors "github.com/oekit/pkg/errors"
	"net/http"
	"strconv"
	"time"
)

// StatusCode 从错误链中提取 HTTP 状态码：err（或其包装链）携带
// *ErrorResponse 时返回其状态码，否则第二个返回值为 false。
func StatusCode(err error) (int, bool) {
	var er *ErrorResponse
	if errors.As(err, &er) && er.Response != nil {
		return er.Response.StatusCode, true
	}
	return 0, false
}

// IsNotFound 报告 err 是否为 404（工蜂对无权限资源同样返回 404 防撞库）。
func IsNotFound(err error) bool {
	code, ok := StatusCode(err)
	return ok && code == http.StatusNotFound
}

// IsUnauthorized 报告 err 是否为 401 认证失败（token 缺失/过期）。
func IsUnauthorized(err error) bool {
	code, ok := StatusCode(err)
	return ok && code == http.StatusUnauthorized
}

// IsForbidden 报告 err 是否为 403 权限不足。
func IsForbidden(err error) bool {
	code, ok := StatusCode(err)
	return ok && code == http.StatusForbidden
}

// IsRateLimited 报告 err 是否为 429 限流。
func IsRateLimited(err error) bool {
	code, ok := StatusCode(err)
	return ok && code == http.StatusTooManyRequests
}

// retryAfter 从 429/503 响应里解析 Retry-After 秒数；缺失或非法返回
// false。
func retryAfter(r *http.Response) (time.Duration, bool) {
	if r == nil {
		return 0, false
	}
	v := r.Header.Get("Retry-After")
	if v == "" {
		return 0, false
	}
	sec, err := strconv.Atoi(v)
	if err != nil || sec < 0 {
		return 0, false
	}
	return time.Duration(sec) * time.Second, true
}

// errRetryExhausted 在重试预算耗尽后返回的哨兵包装：cause 是最后一次的
// 响应错误，kv 带 attempts。
func errRetryExhausted(attempts int, cause error) error {
	return oekiterrors.Wrap(cause, "gongfeng: retries exhausted", "attempts", attempts)
}

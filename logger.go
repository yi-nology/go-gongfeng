package gongfeng

// Logger 是 SDK 的最小日志接口（kv 风格），刻意与主流日志库及 oekit 生态
// 结构兼容：oekit/logx 的 Delegating、go-git-platform 的 provider.Logger
// 均天然满足该接口，注入方无需适配层。SDK 默认 noop——不注入即静默。
type Logger interface {
	Debug(msg string, keysAndValues ...any)
	Info(msg string, keysAndValues ...any)
	Warn(msg string, keysAndValues ...any)
	Error(msg string, keysAndValues ...any)
}

// noopLogger 丢弃全部输出。
type noopLogger struct{}

func (noopLogger) Debug(string, ...any) {}
func (noopLogger) Info(string, ...any)  {}
func (noopLogger) Warn(string, ...any)  {}
func (noopLogger) Error(string, ...any) {}

// NewNoopLogger 返回丢弃输出的 Logger（Client 的默认值）。
func NewNoopLogger() Logger { return noopLogger{} }

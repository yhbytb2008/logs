package logs

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
)

/*
	格式化处理，基于 slog.Handler 实现
	默认输出格式:
		[信息] 2006-01-02 15:04:05 format.go:12 [tag] 默认输出 key=value
*/

// FormatRecord 格式化上下文，聚合 formatter 所需的全部信息
// 持久配置（Name/Tags）来自 handler，per-call 数据（Time/PC/Msg/Attrs）来自 slog.Record
type FormatRecord struct {
	Name  string      // 实体名
	Tags  []string    // 标签
	Time  time.Time   // 日志时间
	PC    uintptr     // 调用者 PC
	Msg   string      // 日志消息
	Attrs []slog.Attr // 属性（With 绑定 + 本次调用）
}

// Formatter 格式化接口，自定义输出格式
type Formatter interface {
	Format(r *FormatRecord) string
}

// FormatFunc 函数类型适配 IFormatter
type FormatFunc func(r *FormatRecord) string

func (f FormatFunc) Format(r *FormatRecord) string {
	return f(r)
}

// 默认格式化器变量
var (
	DefaultFormatter = FormatFunc(defaultFormat)
	TimeFormatter    = FormatFunc(timeFormat)
	JsonFormatter    = FormatFunc(jsonFormat)
)

// defaultFormat 默认格式：[name] date time file [tags] msg key=value
// 始终输出时间和调用者位置（Time/PC 提供时）
func defaultFormat(r *FormatRecord) string {
	var b strings.Builder
	// [name]
	if r.Name != "" {
		b.WriteString("[")
		b.WriteString(r.Name)
		b.WriteString("] ")
	}
	// time
	if !r.Time.IsZero() {
		b.WriteString(r.Time.Format(time.DateTime))
		b.WriteByte(' ')
	}
	// caller file:line
	if r.PC != 0 {
		f, l := sourceLine(r.PC)
		if f != "" {
			b.WriteString(f)
			b.WriteByte(':')
			b.WriteString(strconv.Itoa(l))
			b.WriteByte(' ')
		}
	}
	// tags
	for _, t := range r.Tags {
		b.WriteString("[")
		b.WriteString(t)
		b.WriteString("]")
	}
	// msg
	b.WriteString(r.Msg)
	// attrs
	for _, a := range r.Attrs {
		b.WriteByte(' ')
		b.WriteString(a.String())
	}
	return b.String()
}

// timeFormat 时间格式：HH:MM:SS [tags] msg key=value
// 只输出时间，不输出文件路径
func timeFormat(r *FormatRecord) string {
	var b strings.Builder
	b.WriteString(r.Time.Format(time.TimeOnly))
	b.WriteByte(' ')
	// [name]
	if r.Name != "" {
		b.WriteString("[")
		b.WriteString(r.Name)
		b.WriteString("] ")
	}
	// tags
	for _, t := range r.Tags {
		b.WriteString("[")
		b.WriteString(t)
		b.WriteString("]")
	}
	// msg
	b.WriteString(r.Msg)
	// attrs
	for _, a := range r.Attrs {
		b.WriteByte(' ')
		b.WriteString(a.String())
	}
	return b.String()
}

// jsonFormat JSON 格式，attrs 嵌入 JSON 对象
func jsonFormat(r *FormatRecord) string {
	m := map[string]interface{}{
		"name": r.Name,
		"time": r.Time.Format(time.DateTime),
		"tag":  r.Tags,
		"msg":  r.Msg,
	}
	for _, a := range r.Attrs {
		m[a.Key] = a.Value.Any()
	}
	b, _ := json.Marshal(m)
	return string(b)
}

//================================= handlerConfig =================================

// handlerConfig textHandler 的配置
type handlerConfig struct {
	level     slog.Leveler    // 等级控制
	name      string          // 实体名称
	tags      []string        // 标签
	color     color.Attribute // 颜色
	showColor bool            // 是否显示颜色
	formatter Formatter       // 自定义格式化器（可选，nil 时用 DefaultFormatter）
}

//================================= textHandler =================================

// textHandler 自定义 slog.Handler，实现文本格式化输出
type textHandler struct {
	w     io.Writer
	cfg   handlerConfig
	attrs []slog.Attr
	mu    sync.Mutex
}

func newTextHandler(w io.Writer, cfg *handlerConfig) slog.Handler {
	return &textHandler{w: w, cfg: *cfg}
}

func (h *textHandler) Enabled(_ context.Context, l slog.Level) bool {
	minLevel := slog.LevelInfo
	if h.cfg.level != nil {
		minLevel = h.cfg.level.Level()
	}
	return l >= minLevel
}

func (h *textHandler) Handle(_ context.Context, r slog.Record) error {
	// 收集所有 attrs：pre-bound + record
	allAttrs := make([]slog.Attr, 0, len(h.attrs)+r.NumAttrs())
	allAttrs = append(allAttrs, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		allAttrs = append(allAttrs, a)
		return true
	})

	// 构造 FormatRecord，聚合 formatter 所需的全部信息
	rec := &FormatRecord{
		Name:  h.cfg.name,
		Tags:  h.cfg.tags,
		Time:  r.Time,
		PC:    r.PC,
		Msg:   r.Message,
		Attrs: allAttrs,
	}

	// 统一走 formatter，无自定义 formatter 时用 DefaultFormatter
	formatter := h.cfg.formatter
	if formatter == nil {
		formatter = DefaultFormatter
	}
	output := formatter.Format(rec)

	if !strings.HasSuffix(output, "\n") {
		output += "\n"
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	data := []byte(output)
	// 仅当显式启用颜色且配置了非默认颜色时着色
	// color.Attribute(0) 即 color.Reset，等同于无颜色，跳过避免冗余 ANSI 码
	if h.cfg.showColor && h.cfg.color != 0 {
		data = []byte(color.New(h.cfg.color).Sprint(output))
	}

	_, err := h.w.Write(data)
	return err
}

func (h *textHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	newAttrs = append(newAttrs, h.attrs...)
	newAttrs = append(newAttrs, attrs...)
	return &textHandler{
		w:     h.w,
		cfg:   h.cfg,
		attrs: newAttrs,
	}
}

func (h *textHandler) WithGroup(name string) slog.Handler {
	// 简化实现：不改变行为
	return h
}

//================================= multiHandler =================================

// multiHandler 多输出 handler，将日志分发到多个子 handler
type multiHandler struct {
	mu        sync.RWMutex
	handlers  []slog.Handler
	level     slog.Leveler
	formatter Formatter
}

func newMultiHandler(level slog.Leveler) *multiHandler {
	return &multiHandler{level: level}
}

func (m *multiHandler) Enabled(ctx context.Context, l slog.Level) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, h := range m.handlers {
		if h.Enabled(ctx, l) {
			return true
		}
	}
	return false
}

func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	m.mu.RLock()
	handlers := make([]slog.Handler, len(m.handlers))
	copy(handlers, m.handlers)
	m.mu.RUnlock()

	// 收集第一个错误，但不中断其他 handler 的输出
	var firstErr error
	for _, h := range handlers {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r.Clone()); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	m.mu.RLock()
	defer m.mu.RUnlock()
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithAttrs(attrs)
	}
	return &multiHandler{
		handlers:  handlers,
		level:     m.level,
		formatter: m.formatter,
	}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	m.mu.RLock()
	defer m.mu.RUnlock()
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithGroup(name)
	}
	return &multiHandler{
		handlers:  handlers,
		level:     m.level,
		formatter: m.formatter,
	}
}

// addHandler 添加子 handler
func (m *multiHandler) addHandler(h slog.Handler) {
	m.mu.Lock()
	m.handlers = append(m.handlers, h)
	m.mu.Unlock()
}

// setHandlers 设置子 handlers（覆盖）
func (m *multiHandler) setHandlers(hs []slog.Handler) {
	m.mu.Lock()
	m.handlers = hs
	m.mu.Unlock()
}

// rebuild 重建所有子 handlers
func (m *multiHandler) rebuild(hs []slog.Handler) {
	m.mu.Lock()
	m.handlers = hs
	m.mu.Unlock()
}

// getFormatter 获取格式化器
func (m *multiHandler) getFormatter() Formatter {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.formatter
}

// setFormatter 设置格式化器
func (m *multiHandler) setFormatter(f Formatter) {
	m.mu.Lock()
	m.formatter = f
	m.mu.Unlock()
}

//================================= 辅助函数 =================================

// sourceLine 从 PC 获取文件名和行号
func sourceLine(pc uintptr) (string, int) {
	if pc == 0 {
		return "", 0
	}
	fn := runtime.FuncForPC(pc - 1)
	if fn == nil {
		return "", 0
	}
	file, line := fn.FileLine(pc - 1)
	// 短文件名
	if idx := strings.LastIndex(file, "/"); idx >= 0 {
		file = file[idx+1:]
	}
	if idx := strings.LastIndex(file, "\\"); idx >= 0 {
		file = file[idx+1:]
	}
	return file, line
}

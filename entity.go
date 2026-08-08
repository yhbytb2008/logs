package logs

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
)

// Level 日志等级，基于 slog.Level
// 级别越高越严重，handler 只输出 >= 设定等级的日志
type Level = slog.Level

// Stdout 标准输出（支持颜色和过滤），在 log_write.go 中实现
// 声明在此确保初始化顺序早于 DefaultXxx 实体
var Stdout io.Writer = newStdout()

const (
	LevelAll   = slog.Level(-12) // 全部
	LevelTrace = slog.Level(-8)  // 追溯
	LevelDebug = slog.Level(-4)  // 调试
	LevelRead  = slog.Level(-3)  // 读取
	LevelWrite = slog.Level(-2)  // 写入
	LevelInfo  = slog.Level(0)   // 信息
	LevelWarn  = slog.Level(4)   // 警告
	LevelError = slog.Level(8)   // 错误
	LevelNone  = slog.Level(127) // 无
)

// ParseLevel 从字符串解析日志等级
func ParseLevel(s string) Level {
	switch strings.ToLower(s) {
	case "all":
		return LevelAll
	case "trace":
		return LevelTrace
	case "debug":
		return LevelDebug
	case "write":
		return LevelWrite
	case "read":
		return LevelRead
	case "info":
		return LevelInfo
	case "warn":
		return LevelWarn
	case "err", "error":
		return LevelError
	case "none":
		return LevelNone
	default:
		return LevelAll
	}
}

// StringLevel 将等级转为字符串
func StringLevel(l Level) string {
	switch {
	case l <= LevelAll:
		return "ALL"
	case l <= LevelTrace:
		return "TRACE"
	case l <= LevelDebug:
		return "DEBUG"
	case l <= LevelRead:
		return "READ"
	case l <= LevelWrite:
		return "WRITE"
	case l <= LevelInfo:
		return "INFO"
	case l <= LevelWarn:
		return "WARN"
	case l <= LevelError:
		return "ERROR"
	default:
		return "NONE"
	}
}

// NewEntity 新建日志实体，默认输出到控制台
func NewEntity(name string) *Entity {
	levelVar := new(slog.LevelVar)
	levelVar.Set(LevelAll)
	handler := newMultiHandler(levelVar)
	e := &Entity{
		name:       name,
		showColor:  true,
		color:      0,
		selfLevel:  LevelNone,
		levelVar:   levelVar,
		handler:    handler,
		callerSkip: 0,
		callerBase: 0,
	}
	e.logger = slog.New(handler)
	// 默认输出到控制台
	e.WriteToConsole()
	return e
}

// Entity 日志实体，线程安全，基于 slog
type Entity struct {
	mu         sync.RWMutex
	name       string
	tags       []string
	color      color.Attribute
	showColor  bool
	selfLevel  Level          // 自身日志等级，决定 Print/Println 等输出时的等级
	levelVar   *slog.LevelVar // 动态等级控制，线程安全
	handler    *multiHandler  // 多输出处理器
	logger     *slog.Logger
	writers    []io.Writer // 追踪添加的 writers，用于重建 handler
	callerSkip int         // 调用者层级偏移
	callerBase int         // 内置层级偏移
	retry      int         // 重试次数（暂保留，网络写入用）
}

//================================= Getter =================================

func (e *Entity) GetName() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.name
}

func (e *Entity) GetTag() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.tags
}

func (e *Entity) GetColor() color.Attribute {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.color
}

func (e *Entity) GetSelfLevel() Level {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.selfLevel
}

//================================= Setter =================================

// SetName 设置名称
func (e *Entity) SetName(name string) *Entity {
	e.mu.Lock()
	e.name = name
	e.mu.Unlock()
	e.rebuildHandlers()
	return e
}

// SetTag 设置标签
func (e *Entity) SetTag(s ...string) *Entity {
	e.mu.Lock()
	e.tags = s
	e.mu.Unlock()
	e.rebuildHandlers()
	return e
}

// SetColor 设置颜色
func (e *Entity) SetColor(c color.Attribute) *Entity {
	e.mu.Lock()
	e.color = c
	e.mu.Unlock()
	e.rebuildHandlers()
	return e
}

// SetShowColor 是否显示颜色
func (e *Entity) SetShowColor(b ...bool) *Entity {
	e.mu.Lock()
	e.showColor = !(len(b) > 0 && !b[0])
	e.mu.Unlock()
	e.rebuildHandlers()
	return e
}

// SetLevel 设置日志等级（影响 handler 的最低输出等级，线程安全）
func (e *Entity) SetLevel(level Level) *Entity {
	e.mu.Lock()
	e.levelVar.Set(level)
	e.mu.Unlock()
	return e
}

// SetSelfLevel 设置自身日志等级（决定 Println 等以什么级别输出）
func (e *Entity) SetSelfLevel(level Level) *Entity {
	e.mu.Lock()
	e.selfLevel = level
	e.mu.Unlock()
	return e
}

// SetRetry 设置重试次数
func (e *Entity) SetRetry(retry int) *Entity {
	e.mu.Lock()
	e.retry = retry
	e.mu.Unlock()
	return e
}

// SetCaller 设置调用者层级偏移
func (e *Entity) SetCaller(n int) *Entity {
	e.mu.Lock()
	e.callerSkip = n
	e.mu.Unlock()
	return e
}

// setCaller 内置层级偏移，不公开
func (e *Entity) setCaller(n int) *Entity {
	e.mu.Lock()
	e.callerBase = n
	e.mu.Unlock()
	return e
}

// SetFormatter 设置格式化器（兼容旧 API，内部转换为 handler）
func (e *Entity) SetFormatter(f Formatter) *Entity {
	if f != nil {
		e.mu.Lock()
		e.handler.setFormatter(f)
		e.mu.Unlock()
		e.rebuildHandlers()
	}
	return e
}

// SetWriter 设置输出（覆盖之前的输出）
func (e *Entity) SetWriter(writers ...io.Writer) *Entity {
	e.mu.Lock()
	defer e.mu.Unlock()
	// 过滤 nil
	ws := make([]io.Writer, 0, len(writers))
	for _, w := range writers {
		if w != nil {
			ws = append(ws, w)
		}
	}
	e.writers = ws
	e.handler.setHandlers(e.buildTextHandlersLocked())
	return e
}

// AddWriter 添加输出
func (e *Entity) AddWriter(writers ...io.Writer) *Entity {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, w := range writers {
		if w == nil {
			continue
		}
		e.writers = append(e.writers, w)
		e.handler.addHandler(e.buildTextHandlerLocked(w))
	}
	return e
}

// WriteToConsole 输出到控制台
func (e *Entity) WriteToConsole() *Entity {
	e.AddWriter(Stdout)
	return e
}

// WriteToFile 输出到文件（使用 lumberjack 实现轮转）
// filename 支持时间模板如 "./output/logs/{type}.log"，{type} 会被替换为实体名
// maxSize 单文件最大字节数，<=0 表示不限制
// maxBackups 保留的旧文件数量
// maxAge 保留天数
// compress 是否压缩
func (e *Entity) WriteToFile(filename string, opts ...FileOption) *Entity {
	filename = strings.ReplaceAll(filename, "{type}", e.GetName())
	e.AddWriter(NewFile(filename, opts...))
	return e
}

// WriteToTrunk 写入消息总线
func (e *Entity) WriteToTrunk() *Entity {
	e.AddWriter(Trunk)
	return e
}

// WriteToTCPClient 写入 TCP 客户端
func (e *Entity) WriteToTCPClient(addr string, color ...bool) error {
	w, err := NewTCPClient(addr)
	if err != nil {
		return err
	}
	if len(color) > 0 && color[0] {
		w = NewWriteColor(w)
	}
	e.AddWriter(w)
	return nil
}

// WriteToTCPServer 写入 TCP 服务端
func (e *Entity) WriteToTCPServer(port int, color ...bool) error {
	w, err := NewTCPServer(port)
	if err != nil {
		return err
	}
	if len(color) > 0 && color[0] {
		w = NewWriteColor(w)
	}
	e.AddWriter(w)
	return nil
}

// WriteToHTTPServer 写入 HTTP 服务端
func (e *Entity) WriteToHTTPServer(method, url string, color ...bool) error {
	w := NewHTTPClient(method, url)
	if len(color) > 0 && color[0] {
		w = NewWriteColor(w)
	}
	e.AddWriter(w)
	return nil
}

//================================= 输出方法 =================================

// Sprint 格式化
func (e *Entity) Sprint(v ...interface{}) string {
	return fmt.Sprint(v...)
}

// Sprintf 格式化
func (e *Entity) Sprintf(format string, v ...interface{}) string {
	return fmt.Sprintf(format, v...)
}

// Sprintln 格式化换行
func (e *Entity) Sprintln(v ...interface{}) string {
	return fmt.Sprintln(v...)
}

// Printf 格式化写入
func (e *Entity) Printf(format string, v ...interface{}) (int, error) {
	return e.logMsg(e.getSelfLevel(), fmt.Sprintf(format, v...))
}

// Print 写入内容
func (e *Entity) Print(v ...interface{}) (int, error) {
	return e.logMsg(e.getSelfLevel(), fmt.Sprint(v...))
}

// Println 写入内容并换行
func (e *Entity) Println(v ...interface{}) (int, error) {
	msg := fmt.Sprintln(v...)
	msg = strings.TrimSuffix(msg, "\n")
	return e.logMsg(e.getSelfLevel(), msg)
}

// Write 实现 io.Writer 接口
func (e *Entity) Write(p []byte) (int, error) {
	msg := string(p)
	msg = strings.TrimSuffix(msg, "\n")
	return e.logMsg(e.getSelfLevel(), msg)
}

//================================= 结构化日志（新增） =================================

// Log 结构化日志输出
// 调用链: 用户 → Log → logWithSkip → logWithSkipCtx → Callers
// skip=4 到达 Log 的调用者（用户代码）；callerBase 补偿包级函数多出的一层
func (e *Entity) Log(level Level, msg string, args ...any) {
	_ = e.logWithSkip(4, level, msg, args...)
}

// LogCtx 带 context 的结构化日志输出
// 调用链: 用户 → LogCtx → logWithSkipCtx → Callers
// skip=4 到达 LogCtx 的调用者（用户代码）；callerBase 补偿包级函数多出的一层
func (e *Entity) LogCtx(ctx context.Context, level Level, msg string, args ...any) {
	if ctx == nil {
		ctx = context.Background()
	}
	_ = e.logWithSkipCtx(ctx, 4, level, msg, args...)
}

// With 绑定键值对，返回新的 Entity（共享 handler 但附加属性）
func (e *Entity) With(args ...any) *Entity {
	e.mu.RLock()
	defer e.mu.RUnlock()
	logger := e.logger.With(args...)
	// 复制 tags 避免外部修改影响
	tags := make([]string, len(e.tags))
	copy(tags, e.tags)
	return &Entity{
		name:       e.name,
		tags:       tags,
		color:      e.color,
		showColor:  e.showColor,
		selfLevel:  e.selfLevel,
		levelVar:   e.levelVar,
		handler:    e.handler,
		logger:     logger,
		callerSkip: e.callerSkip,
		callerBase: e.callerBase,
	}
}

//================================= 内部方法 =================================

// getSelfLevel 获取自身等级
func (e *Entity) getSelfLevel() Level {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.selfLevel
}

// logMsg 输出日志消息
// 调用链: 用户 → [包级函数?] → Println/Print/Printf/Write → logMsg → logWithSkip → logWithSkipCtx → Callers
// skip=5 到达 Println 的调用者；callerBase 补偿包级函数多出的一层
func (e *Entity) logMsg(level Level, msg string) (int, error) {
	err := e.logWithSkip(5, level, msg)
	return len(msg), err
}

// logWithSkip 内部使用，skip 是从 runtime.Callers 算起的跳过层数
// skip=1: Callers 自身；skip=2: logWithSkip；skip>=3: 调用链
func (e *Entity) logWithSkip(skip int, level Level, msg string, args ...any) error {
	return e.logWithSkipCtx(context.Background(), skip, level, msg, args...)
}

// logWithSkipCtx 带 context 的内部日志输出
func (e *Entity) logWithSkipCtx(ctx context.Context, skip int, level Level, msg string, args ...any) error {
	e.mu.RLock()
	logger := e.logger
	callerSkip := e.callerSkip + e.callerBase
	e.mu.RUnlock()
	if logger == nil {
		return nil
	}
	// runtime.Callers(skip, ...) 中 skip=1 是 Callers 自身
	// 加上 callerSkip 让用户可调整
	var pcs [1]uintptr
	runtime.Callers(skip+callerSkip, pcs[:])
	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	if len(args) > 0 {
		r.Add(args...)
	}
	return logger.Handler().Handle(ctx, r)
}

// buildTextHandlerLocked 为 io.Writer 创建带格式的 handler（调用者已持锁）
func (e *Entity) buildTextHandlerLocked(w io.Writer) slog.Handler {
	return newTextHandler(w, &handlerConfig{
		level:     e.levelVar,
		name:      e.name,
		tags:      e.tags,
		color:     e.color,
		showColor: e.showColor && isColorWriter(w),
		formatter: e.handler.getFormatter(),
	})
}

// rebuildHandlers 当配置变化时重建所有子 handler
func (e *Entity) rebuildHandlers() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.handler.rebuild(e.buildTextHandlersLocked())
}

// buildTextHandlersLocked 调用者已持锁
func (e *Entity) buildTextHandlersLocked() []slog.Handler {
	handlers := make([]slog.Handler, 0, len(e.writers))
	cfg := &handlerConfig{
		level:     e.levelVar,
		name:      e.name,
		tags:      e.tags,
		color:     e.color,
		showColor: e.showColor,
		formatter: e.handler.getFormatter(),
	}
	for _, w := range e.writers {
		if w == nil {
			continue
		}
		c := *cfg
		c.showColor = e.showColor && isColorWriter(w)
		handlers = append(handlers, newTextHandler(w, &c))
	}
	return handlers
}

// isColorWriter 判断 writer 是否支持颜色输出
func isColorWriter(w io.Writer) bool {
	if val, ok := w.(ColorWriter); ok {
		return val.Color()
	}
	// 标准输出默认支持颜色
	return w == os.Stdout || w == os.Stderr
}

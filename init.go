package logs

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/fatih/color"
)

var (
	m            = sync.Map{}
	DefaultTrace = NewEntity("追溯").SetSelfLevel(LevelTrace).setCaller(1).SetColor(color.FgGreen)
	DefaultWrite = NewEntity("写入").SetSelfLevel(LevelWrite).setCaller(1).SetColor(color.FgBlue)
	DefaultRead  = NewEntity("读取").SetSelfLevel(LevelRead).setCaller(1).SetColor(color.FgBlue)
	DefaultInfo  = NewEntity("信息").SetSelfLevel(LevelInfo).setCaller(1).SetColor(color.FgCyan)
	DefaultDebug = NewEntity("调试").SetSelfLevel(LevelDebug).setCaller(1).SetColor(color.FgYellow)
	DefaultWarn  = NewEntity("警告").SetSelfLevel(LevelWarn).setCaller(1).SetColor(color.FgMagenta)
	DefaultErr   = NewEntity("错误").SetSelfLevel(LevelError).setCaller(1).SetColor(color.FgRed)

	// Trunk 消息总线，公共 Writer
	Trunk = newTrunk(1000)
)

func init() {
	// 强制启用 fatih/color 的 ANSI 码输出（OS TTY 检测不可靠）
	// 实际是否对某个 writer 输出颜色由 isColorWriter 控制：
	// Stdout/Stderr 和实现 Color() 的 writer 会显示颜色，文件不会
	color.NoColor = false
	m.Store(DefaultTrace.GetName(), DefaultTrace)
	m.Store(DefaultWrite.GetName(), DefaultWrite)
	m.Store(DefaultRead.GetName(), DefaultRead)
	m.Store(DefaultInfo.GetName(), DefaultInfo)
	m.Store(DefaultDebug.GetName(), DefaultDebug)
	m.Store(DefaultWarn.GetName(), DefaultWarn)
	m.Store(DefaultErr.GetName(), DefaultErr)
}

// New 新建日志实体，相同名称返回同一实例
func New(name string) *Entity {
	val, has := m.Load(name)
	if has && val != nil {
		if val, ok := val.(*Entity); ok {
			return val
		}
	}
	newEntity := NewEntity(name)
	m.Store(name, newEntity)
	return newEntity
}

//================================= 全局配置 =================================

// SetWriter 覆盖所有实体的 io.Writer
func SetWriter(fn ...io.Writer) {
	m.Range(func(key, value interface{}) bool {
		value.(*Entity).SetWriter(fn...)
		return true
	})
}

// AddWriter 给所有实体添加 io.Writer
func AddWriter(fn ...io.Writer) {
	m.Range(func(key, value interface{}) bool {
		value.(*Entity).AddWriter(fn...)
		return true
	})
}

// WriteToTCPClient 所有日志写入 TCP 客户端
func WriteToTCPClient(addr string, color ...bool) (err error) {
	var writer io.Writer
	writer, err = NewTCPClient(addr)
	if err != nil {
		return err
	}
	if len(color) > 0 && color[0] {
		writer = NewWriteColor(writer)
	}
	AddWriter(writer)
	return nil
}

// WriteToTCPServer 所有日志写入 TCP 服务端
func WriteToTCPServer(port int, color ...bool) (err error) {
	var writer io.Writer
	writer, err = NewTCPServer(port)
	if err != nil {
		return err
	}
	if len(color) > 0 && color[0] {
		writer = NewWriteColor(writer)
	}
	AddWriter(writer)
	return nil
}

// WriteToHTTPServer 所有日志写入 HTTP 服务端
func WriteToHTTPServer(method, url string, color ...bool) (err error) {
	var writer io.Writer
	writer = NewHTTPClient(method, url)
	if len(color) > 0 && color[0] {
		writer = NewWriteColor(writer)
	}
	AddWriter(writer)
	return nil
}

// SetCaller 设置所有实体的调用者层级
func SetCaller(n int) {
	m.Range(func(key, value interface{}) bool {
		value.(*Entity).SetCaller(n)
		return true
	})
}

// SetShowColor 设置所有实体是否显示颜色
func SetShowColor(b ...bool) {
	m.Range(func(key, value interface{}) bool {
		value.(*Entity).SetShowColor(b...)
		return true
	})
}

// SetLevel 设置所有实体的日志等级
func SetLevel(level Level) {
	m.Range(func(key, value interface{}) bool {
		value.(*Entity).SetLevel(level)
		return true
	})
}

// SetLevelWithAll 设置日志等级为全部
func SetLevelWithAll() {
	SetLevel(LevelAll)
}

// SetFormatter 设置所有实体的输出格式
func SetFormatter(f Formatter) {
	m.Range(func(key, value interface{}) bool {
		value.(*Entity).SetFormatter(f)
		return true
	})
}

// SetFormatterWithDefault 设置输出格式为默认
func SetFormatterWithDefault() {
	SetFormatter(DefaultFormatter)
}

// SetFormatterWithTime 设置输出格式为仅时间
func SetFormatterWithTime() {
	SetFormatter(TimeFormatter)
}

//================================= 预设日志函数 =================================

// PrintErr 打印错误，有错误才打印
func PrintErr(err error) bool {
	if err != nil {
		Err(err.Error())
	}
	return err != nil
}

// PanicErr 有错误时 panic
func PanicErr(err error) bool {
	if err != nil {
		Panic(err.Error())
	}
	return err != nil
}

// Spend 记录耗时，用法: defer Spend("前缀")()
func Spend(prefix ...interface{}) func() {
	now := time.Now()
	return func() {
		DefaultDebug.Println(fmt.Sprint(prefix...) + time.Since(now).String())
	}
}

// Trace 预设追溯 绿色
func Trace(s ...interface{}) (int, error) {
	return DefaultTrace.Println(s...)
}

// Tracef 预设追溯 绿色
func Tracef(format string, s ...interface{}) (int, error) {
	return DefaultTrace.Printf(format, s...)
}

// Debug 预设调试 黄色
func Debug(s ...interface{}) (int, error) {
	return DefaultDebug.Println(s...)
}

// Debugf 预设调试 黄色
func Debugf(format string, s ...interface{}) (int, error) {
	return DefaultDebug.Printf(format, s...)
}

// Read 预设读取 蓝色
func Read(s ...interface{}) (int, error) {
	return DefaultRead.Println(s...)
}

// Readf 预设读取 蓝色
func Readf(format string, s ...interface{}) (int, error) {
	return DefaultRead.Printf(format, s...)
}

// Write 预设写入 蓝色
func Write(s ...interface{}) (int, error) {
	return DefaultWrite.Println(s...)
}

// Writef 预设写入 蓝色
func Writef(format string, s ...interface{}) (int, error) {
	return DefaultWrite.Printf(format, s...)
}

// Info 预设信息 青色
func Info(s ...interface{}) (int, error) {
	return DefaultInfo.Println(s...)
}

// Infof 预设信息 青色
func Infof(format string, s ...interface{}) (int, error) {
	return DefaultInfo.Printf(format, s...)
}

// Warn 预设警告
func Warn(s ...interface{}) (int, error) {
	return DefaultWarn.Println(s...)
}

// Warnf 警告
func Warnf(format string, s ...interface{}) (int, error) {
	return DefaultWarn.Printf(format, s...)
}

// Err 预设错误 红色
func Err(s ...interface{}) (int, error) {
	return DefaultErr.Println(s...)
}

// Errf 预设错误 红色
func Errf(format string, s ...interface{}) (int, error) {
	return DefaultErr.Printf(format, s...)
}

// Error 预设错误 红色
func Error(s ...interface{}) (int, error) {
	return DefaultErr.Println(s...)
}

// Errorf 预设错误 红色
func Errorf(format string, s ...interface{}) (int, error) {
	return DefaultErr.Printf(format, s...)
}

// Panic 预设错误 红色，输出后 panic
func Panic(s ...interface{}) (int, error) {
	msg := fmt.Sprint(s...)
	_, _ = DefaultErr.Println(msg)
	panic(msg)
}

// Panicf 预设错误 红色，输出后 panic
func Panicf(format string, s ...interface{}) (int, error) {
	msg := fmt.Sprintf(format, s...)
	_, _ = DefaultErr.Println(msg)
	panic(msg)
}

// Fatal 预设错误 红色，输出后退出
func Fatal(s ...interface{}) (int, error) {
	n, err := DefaultErr.Println(s...)
	os.Exit(1)
	return n, err
}

// Fatalf 预设错误 红色，输出后退出
func Fatalf(format string, s ...interface{}) (int, error) {
	n, err := DefaultErr.Printf(format, s...)
	os.Exit(1)
	return n, err
}

//================================= 结构化日志（新增） =================================

// Log 结构化日志输出
func Log(level Level, msg string, args ...any) {
	DefaultInfo.Log(level, msg, args...)
}

// LogCtx 带 context 的结构化日志输出
func LogCtx(ctx context.Context, level Level, msg string, args ...any) {
	DefaultInfo.LogCtx(ctx, level, msg, args...)
}

// With 绑定键值对到默认实体
func With(args ...any) *Entity {
	return DefaultInfo.With(args...)
}

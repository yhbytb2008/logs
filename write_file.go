package logs

import (
	"io"

	"gopkg.in/natefinch/lumberjack.v2"
)

//============================== WriteFile ==============================

// FileOption 文件轮转配置选项
type FileOption func(*lumberjack.Logger)

// WithMaxSize 设置单文件最大大小（MB）
func WithMaxSize(mb int) FileOption {
	return func(l *lumberjack.Logger) {
		l.MaxSize = mb
	}
}

// WithMaxBackups 设置保留的旧文件数量
func WithMaxBackups(n int) FileOption {
	return func(l *lumberjack.Logger) {
		l.MaxBackups = n
	}
}

// WithMaxAge 设置保留天数
func WithMaxAge(days int) FileOption {
	return func(l *lumberjack.Logger) {
		l.MaxAge = days
	}
}

// WithCompress 设置是否压缩旧文件
func WithCompress(b bool) FileOption {
	return func(l *lumberjack.Logger) {
		l.Compress = b
	}
}

// WithLocalTime 设置是否使用本地时间
func WithLocalTime(b bool) FileOption {
	return func(l *lumberjack.Logger) {
		l.LocalTime = b
	}
}

// FileWriter 文件写入器，包装 lumberjack.Logger 并实现 io.Closer
type FileWriter struct {
	*lumberjack.Logger
}

// Close 关闭文件写入器，释放底层文件资源
func (f *FileWriter) Close() error {
	if f.Logger == nil {
		return nil
	}
	return f.Logger.Close()
}

// NewFile 创建文件输出（基于 lumberjack 实现自动轮转）
// filename 文件路径
// opts 配置选项（大小、备份数、保留天数、压缩等）
// 返回值实现 io.Writer 和 io.Closer，调用者可类型断言为 *FileWriter 调用 Close
func NewFile(filename string, opts ...FileOption) io.Writer {
	l := &lumberjack.Logger{
		Filename: filename,
	}
	for _, opt := range opts {
		opt(l)
	}
	return &FileWriter{Logger: l}
}

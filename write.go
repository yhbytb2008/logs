package logs

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
)

/*
	实现 io.Writer 接口的输出目标
*/

// ColorWriter 支持颜色输出的 writer，实现此接口的 writer 会被识别为支持颜色
type ColorWriter interface{ Color() bool }

//============================== Color Writer ==============================

// writeColor 包装 io.Writer，标记为支持颜色输出
type writeColor struct{ io.Writer }

func (this *writeColor) Color() bool { return true }

// NewWriteColor 包装 writer 为支持颜色输出的 writer
func NewWriteColor(writer io.Writer) io.Writer {
	if writer == nil {
		return nil
	}
	return &writeColor{writer}
}

//============================== Carriage Return Writer ==============================

const carriageReturnPrefix = "\r\x1b[K"

// writeCarriageReturn 包装 io.Writer，在每次写入前添加 \r\033[K 清除当前行
// 适用于进度条、实时状态更新等需要原地刷新覆盖的场景
type writeCarriageReturn struct{ io.Writer }

func (w *writeCarriageReturn) Write(p []byte) (int, error) {
	data := append([]byte(carriageReturnPrefix), p...)
	_, err := w.Writer.Write(data)
	// 返回原始数据长度，避免调用者误解写入字节数
	return len(p), err
}

// Color 透传内层 writer 的颜色支持能力
func (w *writeCarriageReturn) Color() bool {
	if c, ok := w.Writer.(ColorWriter); ok {
		return c.Color()
	}
	return false
}

// NewCarriageReturnWriter 包装 writer，每次写入前添加 \r\033[K 清除当前行
// 可组合使用：logs.NewCarriageReturnWriter(logs.Stdout)
// 注意：仅适用于终端输出，写入文件会产生多余的 \r 和 ANSI 控制序列
func NewCarriageReturnWriter(writer io.Writer) io.Writer {
	if writer == nil {
		return nil
	}
	return &writeCarriageReturn{writer}
}

//============================== Stdout ==============================

// stdoutWriter 标准输出，支持过滤
type stdoutWriter struct {
	mu     sync.Mutex
	filter *Filter
	once   sync.Once
	cancel context.CancelFunc // 用于停止 filter goroutine
}

func newStdout() *stdoutWriter {
	return &stdoutWriter{filter: NewFilter(nil)}
}

// Write 实现 io.Writer
func (s *stdoutWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	filter := s.filter
	s.mu.Unlock()
	if filter != nil && filter.IsEnabled() {
		if !filter.Valid(p) {
			return len(p), nil
		}
	}
	return os.Stdout.Write(p)
}

// Color 标记支持颜色输出
func (s *stdoutWriter) Color() bool { return true }

// EnableFilter 启用交互式过滤（从 stdin 读取关键词）
// goroutine 会一直运行直到程序退出；如需停止可调用 StopFilter
func (s *stdoutWriter) EnableFilter() {
	s.once.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		s.mu.Lock()
		s.cancel = cancel
		s.mu.Unlock()
		go func() {
			for {
				var input string
				// 使用 fmt.Scanln 阻塞读取，无法被 ctx 取消
				// 程序退出时 goroutine 自然终止
				fmt.Scanln(&input)
				select {
				case <-ctx.Done():
					return
				default:
				}
				s.mu.Lock()
				s.filter.SetLike(input)
				s.mu.Unlock()
			}
		}()
	})
}

// StopFilter 停止过滤 goroutine
func (s *stdoutWriter) StopFilter() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.mu.Unlock()
}

// SetFilter 设置过滤器
func (s *stdoutWriter) SetFilter(f *Filter) {
	s.mu.Lock()
	s.filter = f
	s.mu.Unlock()
}

// EnableStdoutFilter 启用标准输出的交互式过滤（从 stdin 读取关键词）
func EnableStdoutFilter() {
	if s, ok := Stdout.(*stdoutWriter); ok {
		s.EnableFilter()
	}
}

// StopStdoutFilter 停止标准输出的交互式过滤
func StopStdoutFilter() {
	if s, ok := Stdout.(*stdoutWriter); ok {
		s.StopFilter()
	}
}

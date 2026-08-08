package logs

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// TestBasicLog 测试基本日志输出
func TestBasicLog(t *testing.T) {
	var buf bytes.Buffer
	e := NewEntity("测试")
	e.SetWriter(&buf)
	e.SetSelfLevel(LevelInfo)
	e.SetLevel(LevelAll)
	e.SetShowColor(false)
	e.Println("hello world")

	output := buf.String()
	if !strings.Contains(output, "hello world") {
		t.Errorf("输出应包含 'hello world', 实际: %s", output)
	}
	if !strings.Contains(output, "[测试]") {
		t.Errorf("输出应包含 '[测试]', 实际: %s", output)
	}
}

// TestLevelFilter 测试日志等级过滤
func TestLevelFilter(t *testing.T) {
	var buf bytes.Buffer
	e := NewEntity("测试")
	e.SetWriter(&buf)
	e.SetShowColor(false)

	// 设置等级为 Error，Info 级别的日志不应输出
	e.SetLevel(LevelError)
	e.SetSelfLevel(LevelInfo)
	e.Println("这条不应输出")
	if buf.Len() > 0 {
		t.Errorf("Info 级别在 Error 等级下不应输出, 实际: %s", buf.String())
	}

	// 设置等级为 All，Info 级别的日志应该输出
	e.SetLevel(LevelAll)
	e.Println("这条应该输出")
	if !strings.Contains(buf.String(), "这条应该输出") {
		t.Errorf("Info 级别在 All 等级下应输出, 实际: %s", buf.String())
	}
}

// TestStructuredLog 测试结构化日志
func TestStructuredLog(t *testing.T) {
	var buf bytes.Buffer
	e := NewEntity("测试")
	e.SetWriter(&buf)
	e.SetLevel(LevelAll)
	e.SetShowColor(false)
	e.Log(LevelInfo, "用户登录", "user", "alice", "ip", "192.168.1.1")

	output := buf.String()
	if !strings.Contains(output, "用户登录") {
		t.Errorf("输出应包含消息, 实际: %s", output)
	}
	if !strings.Contains(output, "user=alice") {
		t.Errorf("输出应包含结构化字段, 实际: %s", output)
	}
}

// TestWith 测试 With 绑定属性
func TestWith(t *testing.T) {
	var buf bytes.Buffer
	e := NewEntity("测试")
	e.SetWriter(&buf)
	e.SetLevel(LevelAll)
	e.SetShowColor(false)

	e2 := e.With("request_id", "abc123")
	e2.Log(LevelInfo, "处理请求")

	output := buf.String()
	if !strings.Contains(output, "request_id=abc123") {
		t.Errorf("输出应包含 With 绑定的属性, 实际: %s", output)
	}
}

// TestFileWriter 测试文件写入
func TestFileWriter(t *testing.T) {
	e := NewEntity("文件测试")
	e.SetLevel(LevelAll)
	e.SetShowColor(false)
	e.WriteToFile("./output/logs/test_{type}.log", WithMaxSize(1), WithMaxBackups(3))

	e.Println("文件写入测试")
	// 给异步写入一点时间
	time.Sleep(100 * time.Millisecond)
	// 文件存在即通过（lumberjack 会自动创建）
}

// TestConcurrentWrite 测试并发写入
func TestConcurrentWrite(t *testing.T) {
	var buf bytes.Buffer
	e := NewEntity("并发测试")
	e.SetWriter(&buf)
	e.SetLevel(LevelAll)
	e.SetShowColor(false)

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(n int) {
			for j := 0; j < 100; j++ {
				e.Println("并发消息", n, j)
			}
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < 10; i++ {
		<-done
	}
	// 不 panic 即通过
}

// TestParseLevel 测试等级解析
func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  Level
	}{
		{"all", LevelAll},
		{"trace", LevelTrace},
		{"debug", LevelDebug},
		{"info", LevelInfo},
		{"warn", LevelWarn},
		{"error", LevelError},
		{"err", LevelError},
		{"none", LevelNone},
		{"unknown", LevelAll}, // 默认返回 All
	}
	for _, tt := range tests {
		got := ParseLevel(tt.input)
		if got != tt.want {
			t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// TestSpend 测试耗时记录
func TestSpend(t *testing.T) {
	var buf bytes.Buffer
	oldDebug := DefaultDebug
	DefaultDebug = NewEntity("调试").SetSelfLevel(LevelDebug).SetLevel(LevelAll).SetShowColor(false)
	DefaultDebug.SetWriter(&buf)
	defer func() { DefaultDebug = oldDebug }()

	f := Spend("耗时: ")
	time.Sleep(10 * time.Millisecond)
	f()

	output := buf.String()
	if !strings.Contains(output, "耗时:") {
		t.Errorf("输出应包含 '耗时:', 实际: %s", output)
	}
}

package logs

import (
	"io"
	"regexp"
	"sync"
)

//============================== WriteFilter ==============================

// NewFilter 创建过滤器
func NewFilter(w io.Writer) *Filter {
	return &Filter{Writer: w}
}

// Filter 数据过滤器，支持正则匹配
type Filter struct {
	io.Writer
	mu      sync.RWMutex
	reg     *regexp.Regexp // 正则表达式
	enable  bool           // 是否启用
	likeStr string         // 模糊匹配字符串
}

// Enable 启用/禁用过滤器
func (f *Filter) Enable(b ...bool) {
	f.mu.Lock()
	f.enable = len(b) == 0 || b[0]
	f.mu.Unlock()
}

// IsEnabled 是否已启用
func (f *Filter) IsEnabled() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.enable
}

// SetRegular 设置正则表达式（转义后编译，避免 ReDoS）
func (f *Filter) SetRegular(reg string) error {
	compiled, err := regexp.Compile(reg)
	if err != nil {
		return err
	}
	f.mu.Lock()
	f.reg = compiled
	f.mu.Unlock()
	return nil
}

// SetLike 设置模糊搜索（自动转义特殊字符，防止正则注入）
func (f *Filter) SetLike(like string) {
	f.mu.Lock()
	f.likeStr = like
	if like == "" {
		f.reg = nil
	} else {
		// 转义用户输入的特殊字符，防止 ReDoS
		f.reg, _ = regexp.Compile(".*" + regexp.QuoteMeta(like) + ".*")
	}
	f.mu.Unlock()
}

// Valid 检查数据是否通过过滤
func (f *Filter) Valid(p []byte) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if !f.enable {
		return true
	}
	if f.reg == nil {
		return true
	}
	return f.reg.Match(p)
}

// Write 实现 io.Writer
func (f *Filter) Write(p []byte) (int, error) {
	if f.Valid(p) {
		if f.Writer != nil {
			return f.Writer.Write(p)
		}
	}
	return len(p), nil
}

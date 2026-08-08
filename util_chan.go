package logs

import (
	"context"
)

// newChan 创建异步通道，handler 必须在创建时传入以避免时序竞态
func newChan(cap int, handler func(ctx context.Context, bs []byte)) *Chan {
	ctx, cancel := context.WithCancel(context.Background())
	data := &Chan{
		c:       make(chan []byte, cap),
		handler: handler,
		ctx:     ctx,
		cancel:  cancel,
	}
	go data.run()
	return data
}

// Chan 异步通道，非阻塞写入，后台 goroutine 处理
type Chan struct {
	c       chan []byte                          // 数据通道
	handler func(ctx context.Context, bs []byte) // 数据处理函数（创建时设置，运行期间只读）
	ctx     context.Context
	cancel  context.CancelFunc
}

// Write 实现 io.Writer，非阻塞写入通道
func (this *Chan) Write(p []byte) (int, error) {
	return len(p), this.Try(p)
}

// Try 尝试加入队列（队列满则丢弃）
func (this *Chan) Try(data ...[]byte) error {
	for _, v := range data {
		select {
		case <-this.ctx.Done():
			return nil
		case this.c <- v:
		default:
			// 队列满，丢弃数据
		}
	}
	return nil
}

// Close 关闭通道，停止后台 goroutine
func (this *Chan) Close() {
	if this.cancel != nil {
		this.cancel()
	}
}

// run 后台处理 goroutine
func (this *Chan) run() {
	handler := this.handler
	for {
		select {
		case <-this.ctx.Done():
			return
		case v := <-this.c:
			if handler != nil {
				handler(this.ctx, v)
			}
		}
	}
}

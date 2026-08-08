package logs

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"
)

//============================== WriteTrunk ==============================

// newTrunk 创建消息总线
func newTrunk(cap int) *trunk {
	return &trunk{
		c: make(chan []byte, cap),
	}
}

// trunk 消息总线，发布订阅模式，线程安全
type trunk struct {
	mu        sync.RWMutex
	c         chan []byte
	subscribe []*trunkSubscribe
}

// Write 实现 io.Writer
func (this *trunk) Write(p []byte) (int, error) {
	this.Publish(p)
	return len(p), nil
}

// Publish 发布数据到所有订阅者
func (this *trunk) Publish(data ...[]byte) {
	this.mu.RLock()
	subs := make([]*trunkSubscribe, len(this.subscribe))
	copy(subs, this.subscribe)
	this.mu.RUnlock()

	for _, sub := range subs {
		if sub != nil {
			sub.try(data...)
		}
	}
}

// Subscribe 订阅消息总线
func (this *trunk) Subscribe(bufSize int, handler func(data []byte)) string {
	key := fmt.Sprintf("%p-%p-%d", this, handler, time.Now().UnixNano())
	ctx, cancel := context.WithCancel(context.Background())
	sub := &trunkSubscribe{
		key:     key,
		handler: handler,
		c:       make(chan []byte, bufSize),
		ctx:     ctx,
		cancel:  cancel,
	}
	go sub.run()

	this.mu.Lock()
	this.subscribe = append(this.subscribe, sub)
	this.mu.Unlock()
	return key
}

// Unsubscribe 取消订阅
func (this *trunk) Unsubscribe(key string) bool {
	if len(key) == 0 {
		return false
	}
	this.mu.Lock()
	defer this.mu.Unlock()
	for i, v := range this.subscribe {
		if v.key == key {
			this.subscribe = append(this.subscribe[:i], this.subscribe[i+1:]...)
			v.cancel()
			return true
		}
	}
	return false
}

// Close 关闭消息总线，取消所有订阅
func (this *trunk) Close() {
	this.mu.Lock()
	defer this.mu.Unlock()
	for _, sub := range this.subscribe {
		if sub != nil {
			sub.cancel()
		}
	}
	this.subscribe = nil
}

type trunkSubscribe struct {
	key     string
	handler func(data []byte)
	c       chan []byte
	ctx     context.Context
	cancel  context.CancelFunc
}

func (this *trunkSubscribe) try(data ...[]byte) {
	for _, v := range data {
		select {
		case <-this.ctx.Done():
			return
		case this.c <- v:
		default:
			// 订阅者队列满，丢弃
		}
	}
}

func (this *trunkSubscribe) run() {
	defer func() {
		// handler panic 不应终止订阅 goroutine，但为防止 goroutine 泄漏，
		// panic 时打印到 stderr 后退出
		if r := recover(); r != nil {
			fmt.Fprint(os.Stderr, "logs: trunk subscribe handler panic: ", r, "\n")
		}
	}()
	for {
		select {
		case <-this.ctx.Done():
			return
		case data := <-this.c:
			if this.handler != nil {
				this.handler(data)
			}
		}
	}
}

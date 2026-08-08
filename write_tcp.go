package logs

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

//============================== TCP Client ==============================

// NewTCPClient 创建 TCP 客户端，断线自动重连
func NewTCPClient(addr string) (io.Writer, error) {
	c, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	t := &tcpClient{
		addr: addr,
	}
	t.mu.Lock()
	t.conn = c
	t.mu.Unlock()
	// handler 在 newChan 启动 goroutine 前传入，避免时序竞态
	t.Chan = newChan(100, func(ctx context.Context, bs []byte) {
		t.mu.Lock()
		conn := t.conn
		t.mu.Unlock()
		// 断线重连
		if conn == nil {
			var err error
			conn, err = net.Dial("tcp", t.addr)
			if err != nil {
				return
			}
			t.mu.Lock()
			t.conn = conn
			t.mu.Unlock()
		}
		_, err := conn.Write(bs)
		if err != nil {
			conn.Close()
			t.mu.Lock()
			t.conn = nil
			t.mu.Unlock()
		}
	})
	return t, nil
}

// tcpClient TCP 客户端，线程安全
type tcpClient struct {
	mu   sync.Mutex
	conn net.Conn
	addr string
	*Chan
}

func (this *tcpClient) Write(p []byte) (int, error) {
	return this.Chan.Write(p)
}

// Close 关闭 TCP 客户端
func (this *tcpClient) Close() error {
	this.mu.Lock()
	defer this.mu.Unlock()
	if this.conn != nil {
		err := this.conn.Close()
		this.conn = nil
		this.Chan.Close()
		return err
	}
	this.Chan.Close()
	return nil
}

//============================== DialTCP ==============================

// TCPDialer TCP 拨号器，支持断线重连和关闭
type TCPDialer struct {
	cancel context.CancelFunc
}

// Close 停止重连和读取
func (d *TCPDialer) Close() error {
	if d.cancel != nil {
		d.cancel()
	}
	return nil
}

// DialTCP 连接 TCP 服务器并读取数据，断线自动重连（指数退避）
// 返回 TCPDialer 用于停止后台 goroutine，避免泄漏
func DialTCP(addr string, dealFunc func(p []byte)) (*TCPDialer, error) {
	// 初始连接测试
	c, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	d := &TCPDialer{cancel: cancel}

	go func() {
		backoff := time.Second
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			c, err := net.Dial("tcp", addr)
			if err != nil {
				// 等待时支持取消
				timer := time.NewTimer(backoff)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
				if backoff < 32*time.Second {
					backoff *= 2
				}
				continue
			}
			backoff = time.Second
			buf := bufio.NewReader(c)
			for {
				select {
				case <-ctx.Done():
					c.Close()
					return
				default:
				}
				line, err := buf.ReadBytes('\n')
				if len(line) > 0 {
					dealFunc(line)
				}
				if err != nil {
					c.Close()
					break
				}
			}
		}
	}()
	return d, nil
}

//============================== TCP Server ==============================

// NewTCPServer 创建 TCP 服务端，接收客户端连接并广播日志
func NewTCPServer(port int) (io.Writer, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}
	writer := &tcpServer{
		listener: listener,
	}
	// handler 在 newChan 启动 goroutine 前传入，避免时序竞态
	writer.Chan = newChan(100, func(ctx context.Context, bs []byte) {
		writer.broadcast(bs)
	})
	go writer.run()
	return writer, nil
}

// tcpServer TCP 服务端，线程安全
type tcpServer struct {
	mu       sync.RWMutex
	listener net.Listener
	conns    map[string]net.Conn
	*Chan
}

func (this *tcpServer) run() {
	for {
		c, err := this.listener.Accept()
		if err != nil {
			// 监听关闭或出错，停止接收
			return
		}
		key := c.RemoteAddr().String()
		this.mu.Lock()
		if this.conns == nil {
			this.conns = make(map[string]net.Conn)
		}
		this.conns[key] = c
		this.mu.Unlock()
	}
}

// broadcast 向所有客户端广播数据
func (this *tcpServer) broadcast(bs []byte) {
	this.mu.RLock()
	dead := []string(nil)
	for key, conn := range this.conns {
		if _, err := conn.Write(bs); err != nil {
			dead = append(dead, key)
		}
	}
	this.mu.RUnlock()
	// 清理断开的连接
	if len(dead) > 0 {
		this.mu.Lock()
		for _, key := range dead {
			if conn, ok := this.conns[key]; ok {
				conn.Close()
				delete(this.conns, key)
			}
		}
		this.mu.Unlock()
	}
}

// Close 关闭 TCP 服务端
func (this *tcpServer) Close() error {
	this.mu.Lock()
	defer this.mu.Unlock()
	for _, conn := range this.conns {
		conn.Close()
	}
	this.conns = nil
	this.Chan.Close()
	return this.listener.Close()
}

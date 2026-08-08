package main

import (
	"strings"
	"time"

	"github.com/injoyai/logs/v2"
)

type _format struct{}

func (_format) Format(f *logs.FormatRecord) string {
	var b strings.Builder
	b.WriteString("[Format] ")
	b.WriteString(f.Msg)
	for _, a := range f.Attrs {
		b.WriteByte(' ')
		b.WriteString(a.String())
	}
	return b.String()
}

func main() {

	logs.Info("测试xxln,xxf,write")
	logs.Debugf("默认换行")
	logs.Debugf("换行\n")
	logs.Debugf("换行*2\n\n")
	logs.Debug("456")
	logs.Debug("789")
	logs.DefaultDebug.Write([]byte("123456\n"))
	logs.DefaultDebug.Println()
	logs.New("测试").Println(666)

	//===================测试TCP Client===================

	logs.Info("测试TCP Client")
	w, err := logs.NewTCPClient(":10089")
	logs.Err(err)
	w2, err := logs.NewTCPServer(10086)
	logs.Err(err)
	logs.AddWriter(logs.NewWriteColor(w), w2)
	<-time.After(time.Second * 5)

	logs.EnableStdoutFilter()

	<-time.After(time.Second * 5)

	//===================测试Color===================

	logs.Info("测试Color")
	logs.Trace("trace")
	logs.Write("write")
	logs.Read("read")
	logs.Debug("Debug")
	logs.Err("Err")
	logs.Warn("Warn")
	logs.SetShowColor(false)
	logs.Debug("Debug")
	logs.Err("Err")
	logs.Warn("Warn")
	logs.SetShowColor(true)

	//===================测试Level===================

	logs.Info("测试Level")
	logs.Debug("Level Debug Before")
	logs.Info("Level Info Before")
	logs.Err("Level Err Before")
	logs.SetLevel(logs.LevelError)
	logs.Debug("Level Debug After")
	logs.Info("Level Info After")
	logs.Err("Level Err After")
	logs.SetLevel(logs.LevelAll)

	//===================测试Formatter===================

	logs.Info("测试Formatter")
	logs.SetFormatter(new(_format))
	logs.Debug("Format Debug")
	logs.Info("Format Info")
	logs.Err("Format Err")
	logs.SetFormatterWithTime()
	logs.Debug("Format Debug")
	logs.Info("Format Info")
	logs.Err("Format Err")
	logs.SetFormatterWithDefault()
	logs.Err("Format Err")

	//===================测试结构化日志（新增）===================

	logs.Info("测试结构化日志")
	logs.Log(logs.LevelInfo, "用户登录", "user", "alice", "ip", "192.168.1.1")
	logs.With("request_id", "abc123").Log(logs.LevelInfo, "处理请求")

	//===================测试Panic和Fatal===================

	logs.Info("测试Panic和Fatal")
	func() {
		defer logs.Spend("总", "耗时")()
		defer logs.Spend()()

		<-time.After(time.Second * 5)
	}()

	testPanic()
	logs.Fatal("Fatal")
	logs.Err("结束")

}

func testPanic() {
	defer func() { recover() }()
	logs.Panic("Panic")
}

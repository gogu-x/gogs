package natsclient

import (
	"github.com/nats-io/nats.go"
)

var nc *nats.Conn

// Init 初始化 NATS 连接，进程启动时调用一次
func Init(url string) error {
	var err error
	nc, err = nats.Connect(url)
	return err
}

// Conn 返回全局 NATS 连接
func Conn() *nats.Conn {
	return nc
}

// Close 关闭连接
func Close() {
	if nc != nil {
		nc.Drain()
	}
}

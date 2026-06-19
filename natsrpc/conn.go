package natsrpc

import natsgo "github.com/nats-io/nats.go"

var nc *natsgo.Conn

func Init(url string) error {
	var err error
	nc, err = natsgo.Connect(url)
	return err
}

func Conn() *natsgo.Conn { return nc }

func Close() {
	if nc != nil {
		nc.Drain()
	}
}

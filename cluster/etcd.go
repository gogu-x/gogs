package cluster

import (
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

var Client *clientv3.Client

func Init(endpoints []string) error {
	var err error
	Client, err = clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	})
	return err
}

func Close() {
	if Client != nil {
		Client.Close()
	}
}

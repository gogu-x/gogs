// Package cluster 提供基于 etcd 的服务注册与发现功能，供 gate、game 等进程共用。
package cluster

import (
	"context"
	"fmt"
	"log"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

const (
	gameServerPrefix = "/game/server/"
	defaultTTL       = int64(30) // 默认租约 30 秒
)

var Client *clientv3.Client

// Init 初始化 etcd 客户端
func Init(endpoints []string) error {
	var err error
	Client, err = clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	})
	return err
}

// Close 关闭 etcd 客户端
func Close() {
	if Client != nil {
		Client.Close()
	}
}

// Register 注册服务节点到 etcd，带自动续租
// key: /game/server/{serverID}  value: gRPC 地址
func Register(serverID, addr string) error {
	return RegisterWithTTL(serverID, addr, defaultTTL)
}

func RegisterWithTTL(serverID, addr string, ttl int64) error {
	lease, err := Client.Grant(context.Background(), ttl)
	if err != nil {
		return err
	}

	key := gameServerKey(serverID)
	_, err = Client.Put(context.Background(), key, addr, clientv3.WithLease(lease.ID))
	if err != nil {
		return err
	}

	ch, err := Client.KeepAlive(context.Background(), lease.ID)
	if err != nil {
		return err
	}

	go func() {
		for range ch {
		}
		// channel 关闭说明续租中断，重新注册
		log.Printf("cluster: keepalive lost for [%s], re-registering...", serverID)
		for {
			time.Sleep(2 * time.Second)
			if err := RegisterWithTTL(serverID, addr, ttl); err != nil {
				log.Printf("cluster: re-register failed: %v", err)
				continue
			}
			log.Printf("cluster: re-registered [%s]", serverID)
			return
		}
	}()
	return nil
}

// GetAddr 根据 serverID 查询 gRPC 地址
func GetAddr(serverID string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	resp, err := Client.Get(ctx, gameServerKey(serverID))
	if err != nil {
		return "", err
	}
	if len(resp.Kvs) == 0 {
		return "", fmt.Errorf("cluster: server %s not found", serverID)
	}
	return string(resp.Kvs[0].Value), nil
}

// GetAll 获取所有已注册的节点 serverID -> addr
func GetAll() (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	resp, err := Client.Get(ctx, gameServerPrefix, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}

	result := make(map[string]string, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		serverID := string(kv.Key)[len(gameServerPrefix):]
		result[serverID] = string(kv.Value)
	}
	return result, nil
}

// Watch 监听所有节点变化，返回事件 channel
// Event.Type: "put"(上线/更新) 或 "delete"(下线)
type Event struct {
	ServerID string
	Addr     string // 下线时为空
	Type     string // "put" | "delete"
}

func Watch(ctx context.Context) <-chan Event {
	ch := make(chan Event, 32)
	go func() {
		defer close(ch)
		watchCh := Client.Watch(ctx, gameServerPrefix, clientv3.WithPrefix())
		for wresp := range watchCh {
			for _, ev := range wresp.Events {
				serverID := string(ev.Kv.Key)[len(gameServerPrefix):]
				switch ev.Type {
				case clientv3.EventTypePut:
					ch <- Event{ServerID: serverID, Addr: string(ev.Kv.Value), Type: "put"}
				case clientv3.EventTypeDelete:
					ch <- Event{ServerID: serverID, Type: "delete"}
				}
			}
		}
	}()
	return ch
}

func gameServerKey(serverID string) string {
	return gameServerPrefix + serverID
}

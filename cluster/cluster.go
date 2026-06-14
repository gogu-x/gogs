// Package cluster 提供基于 etcd 的服务注册与发现功能，供 gate、game 等进程共用。
package cluster

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

const (
	gameServerPrefix = "/game/server/" // /game/server/{serverID}/{instID}
	gameDrainPrefix  = "/game/drain/"  // /game/drain/{serverID}/{instID}
	defaultTTL       = int64(30)
	drainTTL         = int64(60)
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

// Register 注册服务节点，key: /game/server/{serverID}/{instID}
func Register(serverID, instID, addr string) error {
	return registerWithTTL(serverID, instID, addr, defaultTTL)
}

func registerWithTTL(serverID, instID, addr string, ttl int64) error {
	lease, err := Client.Grant(context.Background(), ttl)
	if err != nil {
		return err
	}
	key := serverInstKey(serverID, instID)
	if _, err = Client.Put(context.Background(), key, addr, clientv3.WithLease(lease.ID)); err != nil {
		return err
	}
	ch, err := Client.KeepAlive(context.Background(), lease.ID)
	if err != nil {
		return err
	}
	go func() {
		for range ch {
		}
		log.Printf("cluster: keepalive lost for [%s/%s], re-registering...", serverID, instID)
		for {
			time.Sleep(2 * time.Second)
			if err := registerWithTTL(serverID, instID, addr, ttl); err != nil {
				log.Printf("cluster: re-register failed: %v", err)
				continue
			}
			return
		}
	}()
	return nil
}

// Deregister 主动删除服务节点 key
func Deregister(serverID, instID string) error {
	_, err := Client.Delete(context.Background(), serverInstKey(serverID, instID))
	return err
}

// SetDrain 写入 drain 标记，TTL 60s 自动清理
func SetDrain(serverID, instID string) error {
	lease, err := Client.Grant(context.Background(), drainTTL)
	if err != nil {
		return err
	}
	_, err = Client.Put(context.Background(), drainInstKey(serverID, instID), "1", clientv3.WithLease(lease.ID))
	return err
}

// InstInfo 实例信息
type InstInfo struct {
	InstID string
	Addr   string
}

// GetInstances 返回 serverID 下所有实例，按注册时间降序（最新的在前）
func GetInstances(serverID string) ([]InstInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	prefix := gameServerPrefix + serverID + "/"
	resp, err := Client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	// 按 CreateRevision 降序，最新注册的排最前
	kvs := resp.Kvs
	for i := 0; i < len(kvs)-1; i++ {
		for j := i + 1; j < len(kvs); j++ {
			if kvs[j].CreateRevision > kvs[i].CreateRevision {
				kvs[i], kvs[j] = kvs[j], kvs[i]
			}
		}
	}
	result := make([]InstInfo, 0, len(kvs))
	for _, kv := range kvs {
		instID := strings.TrimPrefix(string(kv.Key), prefix)
		result = append(result, InstInfo{InstID: instID, Addr: string(kv.Value)})
	}
	return result, nil
}

// GetAddr 返回 serverID 下最新注册的实例地址
func GetAddr(serverID string) (string, error) {
	instances, err := GetInstances(serverID)
	if err != nil {
		return "", err
	}
	if len(instances) == 0 {
		return "", fmt.Errorf("cluster: server %s not found", serverID)
	}
	return instances[0].Addr, nil
}

// GetAll 获取所有已注册节点 serverID -> addr（取每个 serverID 的第一个实例）
func GetAll() (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resp, err := Client.Get(ctx, gameServerPrefix, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for _, kv := range resp.Kvs {
		// key = /game/server/{serverID}/{instID}
		parts := strings.TrimPrefix(string(kv.Key), gameServerPrefix)
		segs := strings.SplitN(parts, "/", 2)
		if len(segs) == 2 {
			if _, exists := result[segs[0]]; !exists {
				result[segs[0]] = string(kv.Value)
			}
		}
	}
	return result, nil
}

// InstanceEvent 实例变更事件
type InstanceEvent struct {
	ServerID string
	InstID   string
	Addr     string // delete 时为空
	Type     string // "put" | "delete" | "drain"
}

// WatchInstances 同时监听 server 和 drain 前缀的变化
func WatchInstances(ctx context.Context) <-chan InstanceEvent {
	ch := make(chan InstanceEvent, 64)

	// 监听 /game/server/ 前缀：put → "put"，delete → "delete"
	go func() {
		wch := Client.Watch(ctx, gameServerPrefix, clientv3.WithPrefix())
		for wresp := range wch {
			for _, ev := range wresp.Events {
				key := strings.TrimPrefix(string(ev.Kv.Key), gameServerPrefix)
				segs := strings.SplitN(key, "/", 2)
				if len(segs) != 2 {
					continue
				}
				t := "put"
				if ev.Type == clientv3.EventTypeDelete {
					t = "delete"
				} else if ev.Kv.CreateRevision != ev.Kv.ModRevision {
					// keepalive 续期，忽略
					continue
				}
				ch <- InstanceEvent{
					ServerID: segs[0],
					InstID:   segs[1],
					Addr:     string(ev.Kv.Value),
					Type:     t,
				}
			}
		}
	}()

	// 监听 /game/drain/ 前缀：只关注 put（写入 drain 标记），忽略 delete（TTL 自动清理）
	go func() {
		wch := Client.Watch(ctx, gameDrainPrefix, clientv3.WithPrefix())
		for wresp := range wch {
			for _, ev := range wresp.Events {
				if ev.Type != clientv3.EventTypePut {
					continue // 忽略 drain key 的 delete（TTL 到期自动清理，不需处理）
				}
				key := strings.TrimPrefix(string(ev.Kv.Key), gameDrainPrefix)
				segs := strings.SplitN(key, "/", 2)
				if len(segs) != 2 {
					continue
				}
				ch <- InstanceEvent{
					ServerID: segs[0],
					InstID:   segs[1],
					Type:     "drain",
				}
			}
		}
	}()

	return ch
}

func serverInstKey(serverID, instID string) string {
	return gameServerPrefix + serverID + "/" + instID
}

func drainInstKey(serverID, instID string) string {
	return gameDrainPrefix + serverID + "/" + instID
}

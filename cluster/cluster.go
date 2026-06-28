// Package cluster 提供基于 etcd 的服务注册与发现功能，供 gate、game 等进程共用。
package cluster

import (
	"context"
	"log"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

const (
	gameServerPrefix = "/game/server/" // /game/server/{serverID}/{NodeID}
	gameDrainPrefix  = "/game/drain/"  // /game/drain/{serverID}/{NodeID}

	timePrefix = "/game/time/" //时间偏移量、

	defaultTTL = int64(30)
	drainTTL   = int64(60)
)

// game节点数量  hash 路由到对应的game
var (
	rings   = make(map[string][]InstInfo) // serverID -> 节点列表
	ringsMu sync.RWMutex
)

// UpdateNodes 更新指定 serverID 下的节点列表，由 gate 在 etcd 事件回调中调用。
func UpdateNodes(serverID string, nodes []InstInfo) {
	ringsMu.Lock()
	if len(nodes) == 0 {
		delete(rings, serverID)
	} else {
		rings[serverID] = nodes
	}
	ringsMu.Unlock()
}

// Register 注册服务节点，key: /game/server/{serverID}/{NodeID}
func Register(serverID, NodeID, addr string) error {
	return registerWithTTL(serverID, NodeID, addr, defaultTTL)
}

func registerWithTTL(serverID, NodeID, addr string, ttl int64) error {
	lease, err := Client.Grant(context.Background(), ttl)
	if err != nil {
		return err
	}
	key := serverInstKey(serverID, NodeID)
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
		log.Printf("cluster: keepalive lost for [%s/%s], re-registering...", serverID, NodeID)
		for {
			time.Sleep(2 * time.Second)
			if err := registerWithTTL(serverID, NodeID, addr, ttl); err != nil {
				log.Printf("cluster: re-register failed: %v", err)
				continue
			}
			return
		}
	}()
	return nil
}

// Deregister 主动删除服务节点 key
func Deregister(serverID, NodeID string) error {
	_, err := Client.Delete(context.Background(), serverInstKey(serverID, NodeID))
	return err
}

// SetDrain 写入 drain 标记，TTL 60s 自动清理
func SetDrain(serverID, NodeID string) error {
	lease, err := Client.Grant(context.Background(), drainTTL)
	if err != nil {
		return err
	}
	_, err = Client.Put(context.Background(), drainInstKey(serverID, NodeID), "1", clientv3.WithLease(lease.ID))
	return err
}

func serverInstKey(serverID, NodeID string) string {
	return gameServerPrefix + serverID + "/" + NodeID
}

func drainInstKey(serverID, NodeID string) string {
	return gameDrainPrefix + serverID + "/" + NodeID
}

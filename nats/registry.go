package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"
)

type ServerInfo struct {
	ServerID string `json:"server_id"` // "game-1-A"
	Zone     string `json:"zone"`      // "game-1"
	Addr     string `json:"addr"`      // "192.168.1.10:8080"
	IsMaster bool   `json:"is_master"`
}

// Register 实例启动时注册自己，并定期发心跳
func Register(nc *nats.Conn, info ServerInfo) {
	data, _ := json.Marshal(info)

	// 每5秒发一次心跳
	go func() {
		for {
			nc.Publish("server.heartbeat."+info.Zone, data)
			time.Sleep(5 * time.Second)
		}
	}()

	log.Printf("[%s] 已注册，地址: %s", info.ServerID, info.Addr)
}

// Watch 监听某个区服所有实例心跳，超过15秒没心跳视为下线
func Watch(nc *nats.Conn, zone string) {
	servers := map[string]*ServerInfo{}
	lastSeen := map[string]time.Time{}

	// 订阅心跳
	nc.Subscribe("server.heartbeat."+zone, func(msg *nats.Msg) {
		var info ServerInfo
		json.Unmarshal(msg.Data, &info)
		servers[info.ServerID] = &info
		lastSeen[info.ServerID] = time.Now()
	})

	// 定期检查哪些实例下线了
	go func() {
		for {
			time.Sleep(5 * time.Second)
			for id, t := range lastSeen {
				if time.Since(t) > 15*time.Second {
					log.Printf("[监控] %s 已下线！", id)
					delete(servers, id)
					delete(lastSeen, id)
				} else {
					s := servers[id]
					fmt.Printf("[监控] %s addr=%s master=%v 正常\n", id, s.Addr, s.IsMaster)
				}
			}
		}
	}()
}

// GetMaster 获取某个区服当前主实例地址（玩家登录时调用）
func GetMaster(nc *nats.Conn, zone string) (*ServerInfo, error) {
	// 向该区服所有实例查询，取第一个 master
	reply, err := nc.Request("server.query."+zone, []byte("who-is-master"), 3*time.Second)
	if err != nil {
		return nil, fmt.Errorf("区服 %s 无响应: %v", zone, err)
	}
	var info ServerInfo
	json.Unmarshal(reply.Data, &info)
	return &info, nil
}

// AnswerQuery 实例响应"谁是主"的查询
func AnswerQuery(nc *nats.Conn, info ServerInfo) {
	data, _ := json.Marshal(info)
	nc.Subscribe("server.query."+info.Zone, func(msg *nats.Msg) {
		if info.IsMaster {
			msg.Respond(data) // 只有主实例响应
		}
	})
}

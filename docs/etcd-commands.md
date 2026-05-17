# etcd 常用操作

## 进入容器

```bash
docker exec -it etcd sh
```

## 基本操作

### 写入
```bash
etcdctl put key value
etcdctl put server/game-1 '{"addr":"192.168.1.10:8080","master":true}'
```

### 读取
```bash
etcdctl get key
etcdctl get server/game-1
```

### 删除
```bash
etcdctl del key
```

---

## 查看所有 key

```bash
# 查看所有 key（只显示 key）
etcdctl get "" --prefix --keys-only

# 查看所有 key 和 value
etcdctl get "" --prefix

# 查看某个前缀下的所有 key
etcdctl get server/ --prefix
etcdctl get server/ --prefix --keys-only
```

---

## 监听（watch）

```bash
# 监听某个 key 变化
etcdctl watch key

# 监听某个前缀下所有 key 变化
etcdctl watch server/ --prefix
```

有变化时自动打印，Ctrl+C 退出。

---

## TTL（自动过期）

```bash
# 创建一个租约，10秒过期
etcdctl lease grant 10

# 返回：lease 694d5e3b0b3d4f2b granted with TTL(10s)

# 绑定 key 到租约（key 随租约过期自动删除）
etcdctl put server/game-1 "ip:port" --lease=694d5e3b0b3d4f2b

# 续期租约
etcdctl lease keep-alive 694d5e3b0b3d4f2b

# 查看租约剩余时间
etcdctl lease timetolive 694d5e3b0b3d4f2b
```

---

## 查看集群状态

```bash
# 查看成员列表
etcdctl member list

# 查看集群健康状态
etcdctl endpoint health

# 查看集群状态（leader、版本等）
etcdctl endpoint status
```

---

## 常用场景

### 服务注册（心跳续期）
```bash
# 创建10秒租约
LEASE=$(etcdctl lease grant 10 | awk '{print $2}')

# 注册服务
etcdctl put server/game-1 '{"addr":"192.168.1.10:8080"}' --lease=$LEASE

# 持续续期（进程存活期间一直跑）
etcdctl lease keep-alive $LEASE
```

### 查看所有注册的服务
```bash
etcdctl get server/ --prefix
```

### 抢主（分布式锁）
```bash
etcdctl lock my-lock   # 抢到锁后阻塞，Ctrl+C 释放
```

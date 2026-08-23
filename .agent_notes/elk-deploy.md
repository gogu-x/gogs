# 日志监控平台（Elastic Stack）部署文档

> 目标：部署一套与线上 `http://106.53.123.224:5601`（Kibana 8.17.4）等价的日志采集与监控平台，
> 用于采集游戏服务器（game/gate/chat 等进程）日志，支持 Kibana Discover 查询、聊天监控。
> 单机部署即可满足个人/小团队需求，生产环境可横向扩展。

---

## 1. 架构总览

```
┌────────────┐    ┌──────────────┐    ┌─────────────────┐
│ 游戏服务器 │    │  采集端       │    │   存储 + 检索    │
│ game/gate  │───▶│ Filebeat     │───▶│ Elasticsearch   │
│ chat       │    │ (读日志文件)  │    │ (索引/搜索)      │
└────────────┘    └──────────────┘    └────────┬────────┘
                                               │
                                      ┌────────▼────────┐
                                      │ Kibana          │
                                      │ (Discover/Space)│
                                      └─────────────────┘
```

| 组件 | 版本 | 作用 |
|------|------|------|
| Elasticsearch | 8.17.4 | 日志存储、索引、全文搜索 |
| Kibana | 8.17.4 | Web 界面：Discover 查询、Space 多空间、可视化 |
| Filebeat | 8.17.4 | 采集服务器日志文件，转发到 ES |
| （可选）Logstash | 8.17.4 | 日志清洗/转换，简单场景可省 |

> 本平台以 **Filebeat 直连 ES**（无 Logstash）为基准，与线上一致。日志采用 **data stream / 按天索引**。

---

## 2. 环境要求

### 2.1 硬件（单机最小配置）

| 项目 | 最低 | 建议 |
|------|------|------|
| CPU | 2 核 | 4 核 |
| 内存 | 8 GB | 16 GB（ES 占 4GB，Kibana 1GB）|
| 磁盘 | 50 GB | 按日志量 ×30 天保留计算 |
| OS | CentOS 7+ / Ubuntu 20.04+ | Linux 64 位 |

### 2.2 软件

- Docker 24+ 与 docker-compose（推荐，快速部署）
- 或 JDK 17（非 Docker 部署 ES 8 自带，无需额外装）

---

## 3. 快速部署（Docker Compose 方式）

### 3.1 目录结构

```bash
mkdir -p ~/elk/{config,data,logs}
cd ~/elk
```

### 3.2 `docker-compose.yml`

```yaml
version: '3.8'

services:
  elasticsearch:
    image: docker.elastic.co/elasticsearch/elasticsearch:8.17.4
    container_name: es
    environment:
      - node.name=es01
      - cluster.name=elk-cluster
      - discovery.type=single-node          # 单节点
      - bootstrap.memory_lock=true
      - "ES_JAVA_OPTS=-Xms4g -Xmx4g"        # 堆内存（建议=物理内存一半）
      - xpack.security.enabled=true          # 开启认证（与线上一致）
      - xpack.security.authc.api_key.enabled=true
      - xpack.security.enrollment.enabled=true
      - xpack.security.http.ssl.enabled=false
      - xpack.security.transport.ssl.enabled=false
      - action.destructive_requires_name=false
      - "TZ=Asia/Shanghai"
    ulimits:
      memlock:
        soft: -1
        hard: -1
    volumes:
      - ./data/es:/usr/share/elasticsearch/data
    ports:
      - "9200:9200"
    healthcheck:
      test: ["CMD-SHELL", "curl -s http://localhost:9200 >/dev/null || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 12

  kibana:
    image: docker.elastic.co/kibana/kibana:8.17.4
    container_name: kibana
    environment:
      - SERVER_NAME=kibana
      - ELASTICSEARCH_HOSTS=http://elasticsearch:9200
      - ELASTICSEARCH_USERNAME=kibana_system
      - ELASTICSEARCH_PASSWORD=CHANGE_ME          # 见 3.3 步骤 5
      - XPACK_SECURITY_ENABLED=true
      - I18N_LOCALE=zh-CN                         # 中文界面
      - SERVER_PUBLICBASEURL=http://0.0.0.0:5601
      - "TZ=Asia/Shanghai"
    depends_on:
      elasticsearch:
        condition: service_healthy
    volumes:
      - ./data/kibana:/usr/share/kibana/data
    ports:
      - "5601:5601"

  filebeat:
    image: docker.elastic.co/beats/filebeat:8.17.4
    container_name: filebeat
    user: root
    volumes:
      - ./config/filebeat.yml:/usr/share/filebeat/filebeat.yml:ro
      - /var/log/game:/var/log/game:ro       # 游戏日志目录挂载
      - /var/lib/docker/containers:/var/lib/docker/containers:ro
    depends_on:
      - elasticsearch
    command: ["--strict.perms=false"]
```

### 3.3 初始化步骤

```bash
# 1. 启动 ES
docker compose up -d elasticsearch
# 等待 healthcheck 通过：healthy

# 2. 重置/获取密码（首次启动会自动生成 elastic 密码，看日志）
docker logs es 2>&1 | grep -A5 "Password for the elastic user"

# 3. 手动设置统一密码（推荐，方便配置）
docker exec -it es bash
  /usr/share/elasticsearch/bin/elasticsearch-setup-passwords interactive
  # 依次设置 elastic / kibana_system 等账号密码
  # 记住 elastic 和 kibana_system 的密码
exit

# 4. 把 kibana_system 密码填进 docker-compose.yml 的 ELASTICSEARCH_PASSWORD
# 5. 启动 kibana
docker compose up -d kibana

# 6. 访问
# 浏览器打开 http://<你的IP>:5601  → 用 elastic / <密码> 登录
```

---

## 4. 配置详解

### 4.1 Elasticsearch 关键配置

**生产多节点**（非单机）追加到 `elasticsearch.yml`：

```yaml
# 追加配置：集群节点发现
discovery.seed_hosts: ["es01", "es02", "es03"]
cluster.initial_master_nodes: ["es01"]

# 内存锁
bootstrap.memory_lock: true

# 数据保留策略（配合 ILM）
xpack.ccr.enabled: true
```

**索引生命周期管理（ILM）— 自动清理 30 天前日志**，在 Kibana DevTools 执行：

```json
PUT _ilm/policy/log-retention-30d
{
  "policy": {
    "phases": {
      "hot": { "min_age": "0ms", "actions": { "rollover": { "max_size": "50gb", "max_age": "1d" } } },
      "delete": { "min_age": "30d", "actions": { "delete": {} } }
    }
  }
}
```

### 4.2 Filebeat 配置（采集游戏日志）

`config/filebeat.yml` —— 对应线上 `game-h5prod-*` 索引：

```yaml
filebeat.inputs:
  # game 服务器日志（主日志）
  - type: filestream
    id: game-logs
    enabled: true
    paths:
      - /var/log/game/game*.log          # 按你的日志路径调整
    fields:
      log_type: game
    fields_under_root: false

  # chat 聊天服务器日志（对应线上 chat* 服务器）
  - type: filestream
    id: chat-logs
    enabled: true
    paths:
      - /var/log/game/chat*.log
    fields:
      log_type: chat

  # gate 网关日志
  - type: filestream
    id: gate-logs
    enabled: true
    paths:
      - /var/log/game/gate*.log
    fields:
      log_type: gate

# 输出到 Elasticsearch（data stream 方式，与线上一致）
output.elasticsearch:
  hosts: ["http://elasticsearch:9200"]
  username: "elastic"
  password: "CHANGE_ME"                     # elastic 用户密码
  index: "game-h5prod-%{+yyyy.MM.dd}"       # 按天索引，匹配 data view game-h5prod-*

# 索引模板
setup.template.enabled: true
setup.template.name: "game-h5prod"
setup.template.pattern: "game-h5prod-*"

# 生命周期管理（关联 4.1 的 ILM 策略）
setup.ilm.enabled: true
setup.ilm.policy_name: "log-retention-30d"

# 日志解析
processors:
  # 把日志文件路径作为字段
  - add_host_metadata:
      when.not.contains.tags: forwarded
  - add_cloud_metadata: ~

# 状态/监控
logging.level: info
```

### 4.3 关键：如何匹配"服务器 ID"字段

线上每条日志都有 `serverid`（如 `game864`、`chat2`、`gate6`），这是**从文件名/路径推导**的。
Filebeat 默认不解析该字段，需用 processor 从文件名提取：

```yaml
processors:
  - dissect:
      when:
        contains:
          log.file.path: "/var/log/game/"
      tokenizer: "/var/log/game/%{serverid}.log"
      field: "log.file.path"
      target_prefix: ""
```

### 4.4 索引映射（Index Mapping）— 让 `message` 支持全文检索

线上 `message` 是全文可搜的。建索引时确保 mapping：

```json
PUT /_index_template/game-h5prod
{
  "index_patterns": ["game-h5prod-*"],
  "template": {
    "mappings": {
      "properties": {
        "@timestamp": { "type": "date" },
        "serverid":    { "type": "keyword" },
        "message":     { "type": "text", "fields": { "keyword": { "type": "keyword" } } },
        "log_type":    { "type": "keyword" },
        "src_location":{ "type": "keyword" }
      }
    },
    "settings": {
      "index.lifecycle.name": "log-retention-30d"
    }
  }
}
```

> `message` 同时建 `text`（全文搜索）和 `keyword`（精确聚合）子字段，与线上一致，这样 Kibana 里既能搜内容也能聚合。

---

## 5. Kibana 配置（复刻 chat_monitor 空间）

### 5.1 创建空间（Space）

登录 Kibana → 左侧 **管理 → Spaces → 创建空间**：

```json
{
  "id": "chat_monitor",
  "name": "聊天监控",
  "description": "聊天日志监控",
  "color": "#D36086",
  "initials": "聊",
  "disabledFeatures": ["canvas", "maps", "ml", "visualize"]
}
```

> `disabledFeatures` 与线上一致：禁用与聊天监控无关的功能，简化界面。

### 5.2 创建数据视图（Data View）

1. 进入 **chat_monitor 空间**（右上角切换）
2. 左侧 **管理 → 数据视图 → 创建数据视图**：
   - **名称**：`h5prod`
   - **索引模式**：`game-h5prod-*`
   - **时间戳字段**：`@timestamp`
3. 保存后，Discover 即可查询

### 5.3 常用查询（与线上一致）

Kibana Discover 里用 **KQL**（默认）或 **Lucene** 查询：

```kql
# 聊天服务器日志
serverid: chat*

# 指定聊天服务器 + 关键字
serverid: chat2 AND message: "敏感词"

# 玩家相关
message: "playerID:11050709"

# 指定服务器 + 时间
serverid: game864 AND message: "OrderCreateReq"

# 按服务器聚合
# 用左侧可视化，或 Discover 的"可视化"面板
```

---

## 6. 服务器端日志接入（gs 项目配合）

### 6.1 确认游戏日志输出

本项目使用 `tlog` 输出日志。确认日志文件路径（通常在 `bin/` 运行目录的 `logs/` 下），
把该路径挂载到 Filebeat 容器（见 3.2 的 `./data` 卷映射）。

### 6.2 日志格式与字段

线上每条日志包含：`@timestamp`、`serverid`、`src_location`、`message`。
如果 `src_location` 需要单独字段，可在 Filebeat 用 dissect 或 grok 解析 message：

```yaml
processors:
  - grok:
      field: "message"
      patterns:
        - '^\[%{TIMESTAMP_ISO8601:ts}\] %{GREEDYDATA:msg}'
```

---

## 7. 生产环境扩展（可选）

| 场景 | 方案 |
|------|------|
| 日志量巨大 | ES 多节点 + 分片，Filebeat 多实例 |
| 需要清洗 | 加 Logstash 在 Filebeat 与 ES 之间 |
| 安全加固 | 开启 ES TLS、Kibana 反向代理 + HTTPS |
| 告警 | Kibana Alerting（8.x 内置）或 ElastAlert |
| 可视化大屏 | Kibana Dashboard / Lens |

---

## 8. 常见问题排查

| 现象 | 排查 |
|------|------|
| Kibana 连不上 ES | 检查 `ELASTICSEARCH_HOSTS`、`kibana_system` 密码 |
| Filebeat 无数据 | `docker logs filebeat` 看错误；确认日志路径挂载正确 |
| 索引不存在 | 检查 `setup.template.pattern` 与 data view 是否匹配 |
| 中文乱码 | 日志需 UTF-8；ES 装 `analysis-icu` 分词插件 |
| 内存不足 | 调小 `ES_JAVA_OPTS`，或加 swap |
| 时间字段为空 | 确认日志里有 `@timestamp` 或启用 Filebeat 的 `@timestamp` 注入 |

---

## 9. 附：非 Docker 原生部署（裸机）

```bash
# 下载并解压（与 Docker 镜像版本一致 8.17.4）
curl -L -O https://artifacts.elastic.co/downloads/elasticsearch/elasticsearch-8.17.4-linux-x86_64.tar.gz
curl -L -O https://artifacts.elastic.co/downloads/kibana/kibana-8.17.4-linux-x86_64.tar.gz
curl -L -O https://artifacts.elastic.co/downloads/beats/filebeat/filebeat-8.17.4-linux-x86_64.tar.gz

tar -xzf elasticsearch-8.17.4-linux-x86_64.tar.gz
cd elasticsearch-8.17.4
# 启动
bin/elasticsearch

# 另开终端
cd ../kibana-8.17.4
bin/kibana

# Filebeat 配置同上，bin/filebeat -e
```

---

## 10. 部署检查清单

- [ ] ES 启动成功，`curl http://localhost:9200` 返回 JSON
- [ ] 设置了 elastic / kibana_system 密码
- [ ] Kibana 能登录，出现 Discover
- [ ] 创建了 `chat_monitor` 空间，禁用了指定功能
- [ ] 创建了 data view `game-h5prod-*`
- [ ] Filebeat 采集到游戏日志，Discover 能搜到 `serverid: chat*`
- [ ] 设置了 ILM 30 天清理策略
- [ ] （可选）中文分词插件验证

---

> 本文档基于线上平台 `Kibana 8.17.4`（`/s/chat_monitor` 空间，data view `game-h5prod-*`）编写，
> 所有版本、配置与线上对齐，可直接照做。

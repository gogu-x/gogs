# op.sh 可视化部署工具方案（Docker SDK 版）

> 参考脚本：`.agent_notes/op.sh`
> 本文档只是方案设计，尚未实现。

## 一、背景：op.sh 能力盘点

op.sh 是一个基于 Docker 的服务节点部署/管理脚本，支持 11 种节点类型和 9 种操作。

**节点类型（11种）**：gate / game / door / platform / chat / mail / pay / webserver / rank / global / cross / gdconfig

**操作（9种）**：
| 操作 | 说明 |
|---|---|
| deploy | 部署（先删旧容器，再按节点类型拼参数起新容器） |
| start / stop | 启动/停止已存在的容器（stop 带 600 秒超时） |
| del | 停止并删除容器 |
| status | 查看容器状态（`docker ps -a \| grep`） |
| showlog | 查看日志最后 100 行（读 `/data/gslog/{fullname}.log`） |
| download | 下载镜像/离线包（`docker pull` 或走 COS/OSS） |
| cleardb | **高危**，清空 MongoDB 数据库（`dropDatabase()`） |
| initdb | 执行 `init.js` 初始化数据库 |

**关键入参**：`name`（节点类型）、`id`（可批量，逗号分隔）、`image`（镜像 tag）、`gameid`

**环境变量**（硬编码在脚本头部，本质是环境画像）：`env` / `pfenv` / `csv` / `dburl` / `etcd` / `nats` / `redis` / `harbor` 等

**高危点**：
- `cleardb` 直接 `dropDatabase()`，不可逆
- `stop` 有 600 秒等待，脚本层面是阻塞的

## 二、三种可选架构对比

| 方案 | 说明 | 风险 | 工作量 |
|---|---|---|---|
| A. Go/Web 外壳 + 原样调用 op.sh | 不改脚本逻辑，`exec.Command` 调用，套一层 Web UI | 最低 | 小，2-3 天 |
| B. Docker SDK 替代执行层（本文档主推） | 保留 op.sh 的部署规则知识，但用 Go 结构化配置 + Docker Engine API 重新实现，不再拼 shell 字符串 | 中，需逐节点类型验证等价性 | 中等 |
| C. K8s/Nomad 云原生编排 | 彻底改变部署模型（Deployment/StatefulSet 替代裸容器） | 高，涉及网络、存储、配置管理重新设计 | 大，属于独立基础设施立项 |

**结论**：不建议从 A 直接跳到 C。C 的收益（弹性伸缩、自愈）需要独立评估是否值得投入，不应作为"给 op.sh 加个界面"的副产品。A 起步风险最低，B 是更彻底、可长期维护的方案，本文档详细展开 B。

### 为什么不建议完全推翻重写

op.sh 真正有价值、有风险的部分不是"跑 docker run"这个动作本身，而是 **11 种节点类型的参数拼装规则**——端口计算公式、每个节点专属启动参数、`--doorid 2 --globalid 2 --rankid 2` 这类隐藏的环境耦合。这些是长期生产实践磨出来的隐性知识。重写这套规则，抄错一个端口公式，故障排查成本远大于脚本可维护性问题本身。

因此方案 B 的核心思路是：**保留规则、替换执行层**——把 shell case 分支翻译成 Go 结构化配置，用 Docker SDK 原生调用替代 `docker` CLI 拼命令。

## 三、方案 B 整体架构

```
浏览器 (单页 Web UI)
      │ HTTP / SSE
      ▼
Go 后端服务 (tools/deploy-web)
      │
      ├── Docker Engine API  (github.com/docker/docker/client)   → 容器生命周期
      ├── MongoDB Go Driver  (go.mongodb.org/mongo-driver)       → cleardb / initdb
      ├── 云存储 SDK (COS/OSS Go SDK)                             → 离线镜像包下载
      └── 本地文件 IO                                             → showlog（读 /data/gslog）
      │
      ▼
Docker Engine (unix:///var/run/docker.sock，本机或远程 TCP)
```

不再 `exec.Command` 调 op.sh，也不再 `exec.Command` 调 `docker` CLI，全部走 SDK 原生调用。op.sh 本身保留在 `.agent_notes/` 作为参照/应急后备，不删除。

## 四、op.sh 能力与替代方式对照表

| 操作 | 现在怎么做 | Docker SDK 能否覆盖 |
|---|---|---|
| deploy | `docker run ...` | ✅ `ContainerCreate` + `ContainerStart` |
| start/stop/del | `docker start/stop/rm` | ✅ 直接对应 SDK 方法 |
| status | `docker ps -a \| grep` | ✅ `ContainerList` + name filter 精确匹配 |
| showlog | `tail -100 /data/gslog/xxx.log` | ❌ 读的是挂载出来的日志文件，不是 `docker logs`，需用文件 IO 或改造成读容器 stdout（`ContainerLogs`） |
| download（镜像） | `docker pull` | ✅ `ImagePull`，且能拿到进度流 |
| download（离线包） | `coscli-linux`/`ossutil64` cp | ❌ 不是 Docker 的事，需云厂商 Go SDK |
| cleardb / initdb | `mongo ... dropDatabase()` / `init.js` | ❌ 需 MongoDB Go Driver |
| gdconfig 节点 | `docker run --entrypoint python3 ...` + `while docker ps` 轮询 | ✅ 本质也是跑容器，`ContainerWait` 替代轮询，更优雅 |

**结论**：部署/启停/状态/镜像拉取这条主线，Docker SDK 完全能替代且做得更好；日志、离线包下载、MongoDB 操作这三块本来就不是 Docker 的职责，需要配套的其他组件。

## 五、目录结构

```
tools/deploy-web/
├── external.go                 # 对外暴露的 StartServer() 入口
├── internal/
│   ├── config/
│   │   ├── env.go              # 环境画像：env/pfenv/csv/dburl/etcd/nats/redis/harbor
│   │   └── nodes.go            # 11种节点类型的部署规则定义（端口公式、挂载、entrypoint、启动参数模板）
│   ├── docker/
│   │   ├── client.go           # Docker client 初始化、连接池
│   │   ├── deploy.go           # 对应 op.sh deploy()，按节点类型构造 ContainerConfig
│   │   ├── lifecycle.go        # start/stop/del/status
│   │   └── image.go            # ImagePull + docker tag、拉取进度流
│   ├── mongo/
│   │   ├── cleardb.go          # 对应 cleardb
│   │   └── initdb.go           # 对应 initdb
│   ├── storage/
│   │   └── download.go         # COS/OSS 下载离线镜像包
│   ├── logs/
│   │   └── tail.go             # 读 /data/gslog/{fullname}.log 最后 N 行 + SSE 持续推送
│   ├── handler/
│   │   ├── deploy_handler.go   # /api/deploy /api/start /api/stop /api/del
│   │   ├── status_handler.go   # /api/status
│   │   ├── log_handler.go      # /api/log (SSE)
│   │   ├── db_handler.go       # /api/cleardb /api/initdb（高危，独立鉴权）
│   │   └── download_handler.go # /api/download
│   └── audit/
│       └── audit.go            # 操作审计落盘
├── static/
│   ├── index.html
│   ├── app.js
│   └── style.css
└── main.go
```

## 六、核心模块设计

### 1. 节点规则结构化（替代 op.sh 的 case 分支）

把 `deploy()` 函数里 11 个 case 分支翻译成 Go 里的声明式规则：

```go
type NodeSpec struct {
    Name        string
    NetworkMode string                          // "" / "host"
    PortRule    func(id int) nat.PortMap         // 端口计算公式
    Binds       []string                        // 挂载
    Entrypoint  []string
    ArgsBuilder func(ctx DeployCtx) []string     // 启动参数
    WaitForExit bool                             // gdconfig 这种需要等执行完的节点
}

var NodeRegistry = map[string]NodeSpec{
    "game": {
        NetworkMode: "host",
        PortRule: func(id int) nat.PortMap {
            port := fmt.Sprintf("%d/tcp", 22000+id)
            return nat.PortMap{nat.Port(port): []nat.PortBinding{{HostPort: fmt.Sprint(22000 + id)}}}
        },
        Binds:      []string{"/data/bilog:/go/src/gs/bilog", "/data/glog:/go/src/gs/glog"},
        Entrypoint: []string{"/go/src/gs/game"},
        ArgsBuilder: func(c DeployCtx) []string {
            return []string{
                "--dburl", c.Env.DBUrl,
                "--pfenv", c.Env.Pfenv,
                "--etcdcenter", c.Env.Etcd,
                "--serverid", fmt.Sprint(c.ID),
                "--csv", c.Env.CSV,
                "--servername", c.Name + fmt.Sprint(c.ID),
                "--nats", c.Env.Nats,
                "--namespace", c.Env.Env,
                "--doorid", "2", "--globalid", "2", "--rankid", "2",
            }
        },
    },
    "door": {
        PortRule: func(id int) nat.PortMap {
            return nat.PortMap{
                nat.Port(fmt.Sprintf("%d/tcp", 23000+id)): []nat.PortBinding{{HostPort: fmt.Sprint(23000 + id)}}, // gmport
                nat.Port(fmt.Sprintf("%d/tcp", 25000+id)): []nat.PortBinding{{HostPort: fmt.Sprint(25000 + id)}}, // arkport
                nat.Port(fmt.Sprintf("%d/tcp", 25001+id)): []nat.PortBinding{{HostPort: fmt.Sprint(25001 + id)}}, // apiport
                nat.Port(fmt.Sprintf("%d/tcp", 38000+id)): []nat.PortBinding{{HostPort: fmt.Sprint(38000 + id)}}, // pprofport
            }
        },
        Entrypoint: []string{"/go/src/gs/door"},
        // ...
    },
    // gate / actv / platform / chat / mail / pay / webserver / rank / global / cross / gdconfig 同理逐条搬
}
```

每个节点类型独立一段，逐条对照 op.sh 原文核对，避免遗漏参数。注意 `webserver` 分支里有一行被注释掉的旧版本命令，说明历史上参数有过变化，迁移时以当前生效的那行为准。

### 2. Deploy 流程

```go
func Deploy(ctx context.Context, cli *client.Client, req DeployRequest) (*DeployResult, error) {
    spec, ok := NodeRegistry[req.NodeType]
    if !ok {
        return nil, fmt.Errorf("unknown node type: %s", req.NodeType)
    }
    fullname := fmt.Sprintf("%s_%s%d", req.Env.Env, req.NodeType, req.ID)

    // 1. 等价于 del()：先停后删旧容器
    if err := removeIfExists(ctx, cli, fullname); err != nil {
        return nil, fmt.Errorf("remove existing container %s: %w", fullname, err)
    }

    // 2. 镜像检查/拉取，等价于 download()
    if err := ensureImage(ctx, cli, req.Image, req.Env.Harbor); err != nil {
        return nil, fmt.Errorf("ensure image %s: %w", req.Image, err)
    }

    // 3. 构造 container config
    containerConfig := &container.Config{
        Image:      req.Image,
        Entrypoint: spec.Entrypoint,
        Cmd:        spec.ArgsBuilder(DeployCtx{Env: req.Env, Name: req.NodeType, ID: req.ID}),
        Labels:     map[string]string{"deploy-tool": "sdk-v1"},
    }
    hostConfig := &container.HostConfig{
        NetworkMode:   container.NetworkMode(spec.NetworkMode),
        Binds:         append(commonBinds(), spec.Binds...),
        PortBindings:  spec.PortRule(req.ID),
        RestartPolicy: container.RestartPolicy{Name: "always"},
        LogConfig: container.LogConfig{
            Config: map[string]string{"tag": fmt.Sprintf("%s-%s", req.Image, fullname)},
        },
    }

    resp, err := cli.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, fullname)
    if err != nil {
        return nil, fmt.Errorf("create container: %w", err)
    }
    if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
        return nil, fmt.Errorf("start container: %w", err)
    }

    // 4. gdconfig 类节点需要等待执行完成（对应脚本里的 while 轮询）
    if spec.WaitForExit {
        statusCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
        select {
        case <-statusCh:
        case err := <-errCh:
            return nil, err
        case <-ctx.Done():
            return nil, ctx.Err()
        }
    }

    audit.Record(req, resp.ID, "success")
    return &DeployResult{ContainerID: resp.ID, FullName: fullname}, nil
}
```

### 3. Status（结构化查询替代 grep）

```go
func GetStatus(ctx context.Context, cli *client.Client, nodeType string, id int) (*NodeStatus, error) {
    fullname := fmt.Sprintf("%s_%s%d", envName, nodeType, id)
    containers, err := cli.ContainerList(ctx, container.ListOptions{
        All:     true,
        Filters: filters.NewArgs(filters.Arg("name", "^/"+fullname+"$")),
    })
    if err != nil {
        return nil, err
    }
    if len(containers) == 0 {
        return &NodeStatus{Found: false}, nil
    }
    c := containers[0]
    return &NodeStatus{
        Found:   true,
        State:   c.State,      // running/exited/created
        Status:  c.Status,     // "Up 3 hours"
        Ports:   c.Ports,
        Created: c.Created,
    }, nil
}
```

前端拿到结构化 JSON 直接渲染状态矩阵（绿/灰/红色块），比解析 `docker ps -a | grep` 的文本输出更可靠——用精确 name filter 不存在类似 `game1` 匹配到 `game10` 的误中风险（脚本里用 `grep "\<$fullname\>"` 打了单词边界补丁，本质仍是文本匹配）。

### 4. Stop（带超时 + 状态轮询，替代阻塞600秒）

```go
func Stop(ctx context.Context, cli *client.Client, containerID string) error {
    timeout := 600
    return cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout})
}
```

Web 端发起 stop 后立即拿到 "stopping" 状态，前端每 2 秒轮询一次 `GetStatus`，容器状态变成 exited 就提示完成，用户不再对着转圈按钮死等 10 分钟。

### 5. 镜像拉取进度流（SDK 独有能力，shell 做不到）

```go
func PullImageWithProgress(ctx context.Context, cli *client.Client, imageRef string, w io.Writer) error {
    reader, err := cli.ImagePull(ctx, imageRef, image.PullOptions{})
    if err != nil {
        return err
    }
    defer reader.Close()
    decoder := json.NewDecoder(reader)
    for {
        var event struct {
            Status   string `json:"status"`
            Progress string `json:"progress"`
            ID       string `json:"id"`
        }
        if err := decoder.Decode(&event); err == io.EOF {
            break
        } else if err != nil {
            return err
        }
        fmt.Fprintf(w, "data: %s %s %s\n\n", event.ID, event.Status, event.Progress) // SSE 格式
    }
    return nil
}
```

### 6. Harbor 拉取 + Tag（对应脚本里 `docker tag ${harbor}$1 $1`）

```go
func ensureImage(ctx context.Context, cli *client.Client, image, harbor string) error {
    _, _, err := cli.ImageInspectWithRaw(ctx, image)
    if err == nil {
        return nil // 已存在，跳过（对应脚本 cnt<2 判断）
    }
    fullRef := harbor + image
    if err := pullQuiet(ctx, cli, fullRef); err != nil {
        return err
    }
    return cli.ImageTag(ctx, fullRef, image)
}
```

### 7. MongoDB 操作（cleardb/initdb，SDK 覆盖不到，单独用官方 driver）

```go
func ClearDB(ctx context.Context, dburl, dbname string) error {
    client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://"+dburl))
    if err != nil {
        return err
    }
    defer client.Disconnect(ctx)
    return client.Database(dbname).Drop(ctx)
}
```

该操作在 Web 界面必须是独立的高危页面，二次确认要求用户手动输入库名完整拼一遍，日志强制记录操作人。

### 8. 日志读取（纯文件 IO + SSE 持续推送）

```go
func TailFile(path string, lines int) ([]string, error) { /* 读最后 N 行 */ }
func FollowFile(ctx context.Context, path string, w io.Writer) error {
    // 用 fsnotify 监听文件变化，或简单 poll + SeekEnd 实现 tail -f 效果，通过 SSE 推给前端
}
```

## 七、Web 界面设计

### 1. 环境总览页
- 顶部展示当前环境画像：env / pfenv / csv / dburl / etcd / nats / redis（只读展示，不做在线修改，改环境变量必须回到配置文件，避免 Web 层和实际状态不一致）
- 节点类型 × 服务器 ID 的状态矩阵（批量调用 `GetStatus` 刷新），用颜色区分 running / stopped / not found

### 2. 部署面板（对应 deploy）
- 表单：节点类型下拉（11种）→ 根据选择动态展示该类型的端口计算规则（从 `NodeRegistry` 读出来渲染，用户不需要再翻脚本源码理解"door 节点占了哪几个端口段"）
- ID 输入支持单个/多个/范围（如 "3,5,7" 或 "1-10"，后端转换成批量请求）
- 镜像 tag 输入，带"最近使用"下拉记忆
- 提交前展示最终生成的 `ContainerConfig` JSON 预览，用户确认后才真正执行——保证部署过程可审计、可复制，不是黑盒点一下按钮

### 3. 操作面板（start/stop/del）
- 与部署面板共享节点选择组件
- stop 因为有 600 秒超时，前端要有明显的"执行中"状态和轮询进度提示
- del 前二次确认（先 stop 再删容器）

### 4. 高危操作隔离区（cleardb / initdb）
- 独立页面，红色/警告样式与常规操作区分
- cleardb 需要用户手动输入节点类型+ID 完整拼一遍做二次确认（类似 GitHub 删除仓库要求输入仓库名），因为直接 `dropDatabase()`，不可逆
- 该区域默认收起，需要二次点击才能展开，防止误触

### 5. 日志查看（showlog）
- 选择节点类型+ID后，展示最近 100 行（对齐脚本 `tail -100`）
- 增加"持续跟踪"开关，用 SSE 实现 `tail -f` 效果（对原脚本能力的增强，原脚本只支持一次性看 100 行）

### 6. 镜像下载（download）
- 输入镜像名，展示实时拉取进度（`ImagePull` 事件流转 SSE 推送到前端）

### 7. 操作审计日志
- 独立历史记录页，记录：时间、操作人、操作类型、节点、ID、镜像、执行结果（成功/失败）、最终参数
- 原脚本完全没有这个能力，对多人协作部署环境（内网 GM 部署）尤其有价值

## 八、安全设计

- 工具默认只监听内网地址，不暴露公网
- cleardb / stop / del 等操作要求二次确认，deploy 要求参数预览确认
- 建议加一个简单的操作人身份标识（哪怕只是输入姓名/工号，不做复杂登录系统），配合审计日志使用
- 所有操作日志落盘，便于事后排查"谁清了哪个库"

## 九、依赖清单

```go
require (
    github.com/docker/docker v25.x        // Docker Engine API client
    go.mongodb.org/mongo-driver v1.x      // MongoDB 操作
    // 云存储 SDK 按实际用的厂商选：
    // github.com/tencentyun/cos-go-sdk-v5   （若 publisher=ice 对应腾讯云 COS）
    // github.com/aliyun/aliyun-oss-go-sdk   （默认分支对应阿里云 OSS）
)
```

## 十、迁移与验证策略

Docker SDK 方案的最大风险不是写代码，而是 **11 种节点类型的行为等价性验证**。建议按以下顺序推进：

| 阶段 | 内容 | 验证方式 |
|---|---|---|
| 1 | 用 SDK 实现 `status`（只读） | 与 op.sh 的 `status` 输出人工对比，确认容器识别逻辑一致 |
| 2 | 逐个节点类型迁移 `deploy`，从最简单的开始（如 `chat`、`pay`，端口固定无公式） | 每迁移一个节点类型，在测试环境部署一次，用 `docker inspect` 比对 SDK 创建的容器和 op.sh 创建的容器，确认 Cmd/Entrypoint/PortBindings/Binds/NetworkMode 完全一致 |
| 3 | 迁移端口公式复杂的节点（`door`、`game`、`webserver`），**重点验证 `--network host` 下 PortBindings 的行为**（host 网络模式下 `-p` 参数实际被忽略，SDK 若仍传 PortBindings 可能有兼容性差异，需真实环境测试确认） | 同上 + 实际起容器测端口连通性 |
| 4 | 迁移 `start/stop/del`，验证 600 秒超时行为一致 | 手动触发 stop，观察超时和优雅退出日志 |
| 5 | 迁移 `download`（ImagePull + Tag） | 对比拉取后镜像 ID、Tag 是否与脚本手动执行结果一致 |
| 6 | 补齐 MongoDB（cleardb/initdb）和日志模块 | 与 Docker SDK 无关，风险相对独立，可并行做 |
| 7 | 双轨运行期：Web 工具和 op.sh 并存至少一个迭代周期，出问题随时切回脚本 | 观察期建议不少于 1-2 周实际部署使用 |

## 十一、方案 B 相对纯 shell 包装方案的核心优势

1. **结构化替代文本拼接**：容器配置是 Go struct，不是拼出来的字符串，参数错误能在构造阶段发现
2. **进度可观测**：拉镜像、stop 超时都能给前端推实时进度，shell 方案只能等结果
3. **错误处理规范**：Docker API 返回结构化 error，不需要解析 stdout/exit code
4. **状态查询精确**：`ContainerList` + name filter 精确匹配，不存在 grep 误中风险
5. **规则可视化**：`NodeRegistry` 直接驱动前端表单展示每个节点类型的端口规则

代价是工程量明显大于纯包装方案，且需要认真做等价性验证，不能急于上线替换生产部署入口。

## 十二、实现顺序建议（如果后续要落地）

1. 先做只读功能：`status` 矩阵 + `showlog`，风险最低，立刻能用
2. 逐个节点类型迁移 `deploy`，按第十节的验证策略推进
3. 迁移 `start/stop/del`
4. 补齐 `download`（镜像拉取进度）
5. 最后做高危区 `cleardb/initdb` 和审计日志
6. 双轨运行观察期后，评估是否完全替代 op.sh 作为生产部署入口

# Docker 与 Jenkins 构建部署指南

本文档整理自项目 Dockerfile 分析、Jenkins CI 流水线搭建、Docker 底层机制等相关讨论，供团队参考。

---

## 一、项目 Dockerfile 概览

项目根目录及子目录下共有 7 个 Dockerfile，用途不同：

| 文件 | 用途 |
|---|---|
| `Dockerfile` | 主/生产用，三阶段构建（前端 + Go 编译 + 发布），详见下文 |
| `Dockerfile_release` | 类似主 Dockerfile，无前端构建阶段，无调试编译参数，含额外调试工具 |
| `Dockerfile_dbg` | 调试版，带 `-race` 竞态检测编译 |
| `Dockerfile_dlv` | 专为 delve 远程调试准备，内置 `dlv` 工具 |
| `Dockerfile_test` | 仅编译打包 `chat`(jabber) 单服务，用于测试 |
| `conf_Dockerfile` | 基于 Python，用于生成测试数据（Excel/Mongo/Jinja2） |
| `pkgall_Dockerfile` | 基于 Python，用于配置/元数据打包发布（gdconf 更新） |

所有业务服务共用**同一个镜像**，靠不同的 `ENTRYPOINT`/`--entrypoint` 参数区分跑哪个进程（game/gate/door/pay/chat/mail/webserver/actv/global/cross/chatclient）。

---

## 二、主 Dockerfile 三阶段构建详解

### Stage 1：`gmtBuilder` — 前端资源构建
```dockerfile
FROM harbor-dev.330stars.com/public/gmt_base_img:latest AS gmtBuilder
COPY ./gmt .
RUN make
```
基于内部 Harbor 自定义基础镜像，构建 `gmt`（前端项目，umi + yarn），产物落在 `/build-www/build`。

### Stage 2：`compiler` — Go 服务编译

```dockerfile
FROM harbor-dev.330stars.com/public/golang:1.24.4 AS compiler
ADD ../docs /go/src/gs
WORKDIR /go/src/gs
RUN go build -gcflags "-N -l" -mod vendor -o bin/xxx git.tap4fun.com/k3x/gs/xxx   # ×11
```
- `-mod vendor`：强制使用 vendor 目录依赖，离线可复现构建
- `-gcflags "-N -l"`：禁用优化和内联，属于**调试构建标志**，即使用于生产镜像也保留，便于线上 dlv/gdb attach 调试，但会牺牲运行性能和增大二进制体积
- 逐服务单独 `RUN go build`，编译 11 个二进制：gate, game, door, pay, chat(jabber), mail, webserver, actv, global, cross, chatclient

### Stage 3：发布阶段 — 精简运行镜像
```dockerfile
FROM harbor-dev.330stars.com/public/debian:12
```
- 替换 apt 源为阿里云镜像，解决国内网络/TLS 证书问题
- 安装 `ca-certificates`，清理 apt 缓存减小镜像体积
- 拷贝 compiler 阶段产物（bin/gconf/scripts/web/.cert）+ gmtBuilder 阶段产物（www）
- `ARG GitCommit` + `LABEL GitCommit=${GitCommit}`：构建时注入版本追溯信息
- `ENTRYPOINT ["./game"]`：默认入口，实际部署通过 `--entrypoint` 切换其他服务

### 已知问题与优化建议
1. **构建缓存效率低**：`ADD . /go/src/gs` 把整仓库当一层，任何源码改动都会使该层缓存失效，导致 11 个 `go build` 全部重新执行。优化方向：使用 BuildKit 的 Go build cache mount：
   ```dockerfile
   RUN --mount=type=cache,target=/root/.cache/go-build \
       go build -mod vendor -o bin/gate git.tap4fun.com/k3x/gs/gate
   ```
2. **调试标志用于生产**：建议评估长期生产环境是否需要去掉 `-gcflags "-N -l"`。
3. **未设置非 root 用户**：容器内进程默认 root 运行，建议后续加固时考虑非特权用户运行。
4. **一镜像多进程 vs 拆分独立镜像**：现状（版本强一致性好，但改动一处需全量重新编译）适合游戏服务器这种协议强耦合场景；如需优化，建议按"变更频率+故障隔离需求"分组（核心 gate/game/door 保持耦合，边缘服务 mail/webserver/chatclient 可考虑拆出独立镜像），而非全部拆散。

---

## 三、构建流程实测分析（BuildKit 并行机制）

一次典型构建日志显示：
- BuildKit 会自动识别无依赖关系的 stage（`gmtBuilder`/`compiler`/发布阶段）并**并行调度**
- 耗时分布：`game` 编译最长（~81.5s），`gate` 次之（~67.3s），其余服务因复用 Go build cache 只需 4~11s
- `gmt_base_img` 基础镜像单层高达 1.24GB，下载解压耗时约占 gmtbuilder 阶段一半以上，是可优化的瓶颈
- `docker push` 阶段大部分层因历史构建缓存显示 `Layer already exists`，只有真正变化的层会新推送
- 已知遗留问题：`gmt_base_img` 内没有安装 git，导致构建脚本里 `git rev-parse --short HEAD` 失败（`/bin/sh: git: not found`），影响产物 tar 包命名（非致命，但需关注下游是否依赖该文件名）

---

## 四、Jenkins 与 Docker 的关系（原理）

### Jenkins 不构建镜像，只是编排/触发工具
Jenkins 本身不理解"镜像"，它只做三件事：**监听触发条件 → 在执行节点(Agent)上按顺序跑 shell 命令 → 收集展示结果**。真正的 `docker build`/`docker push` 是调用 Agent 机器上的 Docker 命令完成的。

```
Jenkins Job 触发（手动/webhook/定时）
    ↓
分配 Agent 执行节点
    ↓
Agent 执行 Shell 步骤：git checkout → docker build → docker tag → docker push
    ↓
Jenkins 抓取 stdout/stderr 显示在 Console Output（不做任何解析/干预）
```

### Docker 镜像构建原理（与 Jenkins 无关）
- **镜像层**：每条 `FROM`/`RUN`/`COPY` 生成一层，联合文件系统叠加成完整镜像
- **构建上下文**：`docker build ... .` 把当前目录（排除 `.dockerignore`）打包发给 Docker daemon
- **多阶段并行**：BuildKit 分析 stage 依赖关系，无依赖的并行执行
- **层缓存**：指令内容和依赖父层不变则复用缓存，跳过实际执行，缓存存于 Agent 机器本地磁盘

### Jenkins Agent 环境要求
- 需要安装 Docker，且 Jenkins 运行用户需在 `docker` 组内
- 需要能访问镜像仓库（Harbor），网络请求从 Agent 机器发出
- 需要足够的 CPU/内存/磁盘资源（构建过程实打实消耗算力）

### Jenkins 的增值点
- 触发管理（webhook/定时/手动参数化触发）
- 凭据管理（Harbor 密码、Git token 加密存储）
- 流水线编排（多阶段串联，失败即停止并通知）
- 审计与历史记录（日志、触发人、耗时、状态）
- 并发控制/排队

---

## 五、镜像构建产物的存放位置

```
[构建阶段]  Jenkins Agent 本机磁盘 (/var/lib/docker/overlay2/...)
                    │
                    │ docker tag（打别名，无数据移动，纯元数据操作）
                    ▼
[打标签]    仍在本机，多了个别名指向 harbor-dev.330stars.com/gs/...
                    │
                    │ docker push（真正的网络传输）
                    ▼
[最终存放]  harbor-dev.330stars.com Harbor 服务器存储后端
            → 任何有权限访问该地址的机器可 docker pull 拉取
```

- `docker tag` 不产生新数据，只是给同一份镜像层加别名
- `docker push` 才是真正上传，未变化的层在远程显示 `Layer already exists`
- Agent 本机镜像是临时产物，磁盘有限需定期清理（`docker image prune`）；Harbor 才是团队共享、可追溯的正式存放地

---

## 六、Jenkins Freestyle Job 搭建步骤

### 1. 新建 Job
New Item → 输入名称 → 选择 `Freestyle project`

### 2. 源码管理
- Git → Repository URL 填仓库地址
- Credentials 选择/新建对应凭据
- Branches to build 填 `*/分支名`

### 3.（可选）构建触发器
手动触发 / webhook 自动触发 / Poll SCM 定时轮询 / Build periodically 定时构建

### 4. 构建步骤（Execute shell）
```bash
#!/bin/bash
set -e

GIT_COMMIT_SHORT=$(git rev-parse --short=10 HEAD)
IMAGE_NAME="gs_h5_new_tf_v1046"
IMAGE_TAG="${GIT_COMMIT_SHORT}"
HARBOR_REPO="harbor-dev.330stars.com/gs/${IMAGE_NAME}"

echo "开始构建镜像: ${IMAGE_NAME}:${IMAGE_TAG}"

docker build -t ${IMAGE_NAME}:${IMAGE_TAG} \
  --build-arg GitCommit=${GIT_COMMIT_SHORT} \
  -f Dockerfile .

docker tag ${IMAGE_NAME}:${IMAGE_TAG} ${HARBOR_REPO}:${IMAGE_TAG}
docker push ${HARBOR_REPO}:${IMAGE_TAG}

echo "构建完成: ${HARBOR_REPO}:${IMAGE_TAG}"
```

### 5. 环境前提
- Agent 机器装 Docker，Jenkins 用户在 docker 组
- Agent 已登录 Harbor（`docker login`）或用 Jenkins Credentials 绑定
- 磁盘空间充足，建议定时 `docker system prune -f`

---

## 七、分支参数化配置

### 方式一：String Parameter（简单，需手动输入分支名）
1. 勾选 **This project is parameterized**
2. Add Parameter → String Parameter，Name=`BRANCH_NAME`，Default Value=常用分支名
3. Source Code Management → Branches to build 改为 `*/${BRANCH_NAME}`
4. Shell 脚本中可直接引用 `$BRANCH_NAME` 环境变量

### 方式二：Git Parameter 插件（推荐，下拉选择分支，避免手误）
1. Manage Jenkins → Plugins → 安装 `Git Parameter` 插件
2. Add Parameter → Git Parameter，Parameter Type 选 `Branch`，Branch Filter 填 `origin/(.*)`
3. 保存 Job 后重新进入配置页刷新才能看到分支下拉列表

两者对比：String Parameter 适合分支固定、少变化场景；Git Parameter 适合分支多、切换频繁的团队。

---

## 八、Jenkins 部署方式对比与排错

### 容器化 Jenkins（`/var/jenkins_home` 路径特征）vs 裸机安装（`/var/lib/jenkins` 路径特征）
两者数据目录完全独立，不能直接切换复用。

### 常见报错：`ERROR: open /certs/client/ca.pem: no such file or directory`
**原因**：容器化 Jenkins 默认配置走 DinD（Docker-in-Docker）+ TLS 双向认证模式，环境变量 `DOCKER_TLS_VERIFY=1`、`DOCKER_CERT_PATH=/certs/client` 指向的证书文件未正确生成/挂载。

**推荐修复方案（DooD，Docker outside of Docker）**：改为挂载宿主机 `docker.sock`，不走 TLS：

```bash
docker run -d \
  --name jenkins \
  -p 8080:8080 -p 50000:50000 \
  -v jenkins_home:/var/jenkins_home \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -u root \
  jenkins/jenkins:lts
```

或 docker-compose 形式：
```yaml
services:
  jenkins:
    image: jenkins/jenkins:lts
    user: root
    ports:
      - "8080:8080"
      - "50000:50000"
    volumes:
      - jenkins_home:/var/jenkins_home
      - /var/run/docker.sock:/var/run/docker.sock

volumes:
  jenkins_home:
```

容器内需额外安装 docker 客户端（基础镜像若未内置）：
```bash
docker exec -u root -it jenkins bash
apt-get update && apt-get install -y docker.io
```

并清理残留的 `DOCKER_HOST`/`DOCKER_TLS_VERIFY`/`DOCKER_CERT_PATH` 环境变量。

### 常见报错：`permission denied while trying to connect to the docker API at unix:///var/run/docker.sock`
**原因**：宿主机当前登录用户不在 `docker` 组，Docker socket 默认权限为 `root:docker`。

**修复**：
```bash
sudo usermod -aG docker <当前用户名>
newgrp docker   # 或重新登录使其生效
```

### volumes 配置说明
```yaml
volumes:
  - jenkins_home:/var/jenkins_home            # 命名 volume，Docker 托管存储位置，持久化 Jenkins 数据
  - /var/run/docker.sock:/var/run/docker.sock # Bind mount，直接映射宿主机已存在的具体文件

volumes:
  jenkins_home:   # 顶层声明区，命名 volume 使用前必须在此登记
```
- 命名 volume：适合"数据要持久化但不关心具体存哪"，如 Jenkins 数据
- Bind mount：适合"要跟宿主机某个具体资源打通"，如 docker.sock

---

## 九、进入 Docker 容器的方法

```bash
docker exec -it jenkins bash        # 进入运行中的容器（推荐）
docker exec -u root -it jenkins bash # 以 root 身份进入
docker start jenkins                 # 容器已停止时先启动
```
退出容器：`exit` 或 `Ctrl+D`（不影响容器主进程运行）。

**注意**：`docker attach` 是接到容器主进程的标准输入输出，`Ctrl+C` 可能直接杀掉主进程，不建议用于日常操作，`exec` 更安全。

---

## 十、Docker Socket 通信原理

Docker 分为两部分：
```
docker（CLI 客户端，发指令）  ←— Unix Socket 通信 —→  dockerd（守护进程，真正执行）
```
- `dockerd` 监听 `/var/run/docker.sock`，是真正管理镜像/容器/网络/存储的引擎
- `docker` 命令仅将请求打包通过 socket 发给 `dockerd`，自身不做实际构建工作

挂载 `-v /var/run/docker.sock:/var/run/docker.sock` 后，容器内的 docker 客户端实际是**穿透容器边界，直接指挥宿主机的 dockerd**，容器内并未新起一个独立 Docker 引擎（这就是 DooD 模式）。因此：
- 容器内构建出的镜像实际存放在宿主机的 `/var/lib/docker/...` 下
- 在宿主机执行 `docker images` 能看到容器内构建的镜像

与之相对的 DinD（容器内独立运行 dockerd）默认启用 TLS 双向认证，是之前 `/certs/client/ca.pem` 报错的根源。

---

## 十一、参考：为游戏进程实现命令监听接口

若需让游戏进程具备类似 `dockerd` 监听 socket、接收外部指令的能力，项目中已有现成参考实现：`door/telnet` 模块（`/data/330gs/gs/door/telnet/`）。

### 现有实现原理
```
telnet.Module 监听 TCP 端口
     ↑
外部 telnet 客户端连接，输入 GM 命令
     ↓
Module 解析命令，通过 gRPC ExecCmd 转发给对应游戏进程执行
```

### 方案对比

| | Unix Domain Socket（新写） | TCP + 复用现有 telnet 框架（推荐） |
|---|---|---|
| 适用场景 | 单机本地管理，安全性要求高 | 分布式游戏服架构，需远程管理/跨进程调用 |
| 与现有代码风格 | 不一致 | 完全一致，复用现成基础设施 |
| 权限分级支持 | 需自行实现 | 已有（如 `cmds.Owner` 权限级别设计） |

**建议**：游戏服务器项目已是分布式多进程架构（gate/game/door 等通过 gRPC 通信），且已有成熟 GM 命令框架，新增管理指令应优先复用 `door/telnet` 模式（往现有框架加新指令分支），而非另起一套 Unix socket 机制，以保持运维工具链和权限体系的一致性。

---

*本文档整理自项目构建部署相关讨论，如实际操作步骤与团队现行规范存在差异，请以团队 Wiki/最新约定为准。*

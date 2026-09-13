# Docker 部署

## 服务与端口

`docker-compose.yml` 编排 `battle-1`、`game-1`、`gate`、`platform` 以及内部 MongoDB、etcd、NATS。

对宿主机仅开放：gate WebSocket 的 `${GATE_PORT}`（默认 `8081`）和 platform 支付回调的 `${PLATFORM_WEBHOOK_PORT}`（默认 `7001`）。MongoDB、etcd、NATS、game gRPC、battle 和 platform gRPC 均只在 Compose 网络内可见。

> gate 的实际监听端口为 `gate-base-port + gate-id`，默认即 `8080 + 1 = 8081`。

## 初次部署

1. 在服务器安装 Docker Engine（含 Compose v2），并把本目录中的 `.env.example` 复制为 `.env`。
2. 将 `.env` 中的 `MONGO_ROOT_PASSWORD`、`JWT_SECRET` 替换为高强度随机值；不要将 `.env` 提交到 Git。
3. 启动基础依赖并导入 `../glconf` 配置表到内部 MongoDB：

   ```bash
   docker compose --env-file .env up -d mongodb etcd nats

### Docker 构建主机无法使用 Go 基础镜像时

若服务器 Docker 的内容存储损坏，无法拉取 `golang` 镜像，可在 Windows 构建机从 `gogs` 目录执行下列命令，生成静态 Linux/amd64 二进制到仓库根的 `deploy/bin`：

```powershell
New-Item -ItemType Directory -Force ..\deploy\bin
$env:GOOS = 'linux'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
go build -trimpath -ldflags='-s -w' -o ..\deploy\bin\battle ./battle
go build -trimpath -ldflags='-s -w' -o ..\deploy\bin\game ./game
go build -trimpath -ldflags='-s -w' -o ..\deploy\bin\gate ./gate
go build -trimpath -ldflags='-s -w' -o ..\deploy\bin\platform ./platform
```

当前 Compose 使用 `runtime.Dockerfile`（`FROM scratch`）封装这些静态二进制，不需要服务器拉取 Go 或 Alpine 基础镜像。

   docker compose --env-file .env --profile seed run --rm config-import
   ```

   `config-import` 为一次性容器，会以仓库内容替换**当前 Compose 项目内部 MongoDB**的同名配置集合。生产数据操作前请先备份。
4. 构建并启动业务服务：

   ```bash
   docker compose --env-file .env build
   docker compose --env-file .env up -d
   docker compose --env-file .env ps
   docker compose --env-file .env logs --tail=100 gate game-1 battle-1 platform
   ```

## 镜像交付到另一台服务器

没有镜像仓库时，在构建机执行：

```bash
docker compose --env-file .env build
docker save -o gotree-images.tar gotree/battle:v1 gotree/game:v1 gotree/gate:v1 gotree/platform:v1
```

把 `gotree-images.tar`、`docker-compose.yml`、`.env` 复制到服务器；服务器执行 `docker load -i gotree-images.tar` 后，使用 `docker compose --env-file .env up -d --no-build` 启动。基础依赖镜像（MongoDB、etcd、NATS）仍需由服务器拉取，或另行 `docker save` 传输。

部署前应在云安全组和服务器防火墙中只放行 `GATE_PORT`、`PLATFORM_WEBHOOK_PORT`，以及 SSH 管理端口。为支付回调设置 HTTPS 反向代理后再对公网开放 `PLATFORM_WEBHOOK_PORT`。

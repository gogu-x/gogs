# Docker 部署

## 服务与端口

`docker-compose.yml` 编排 `battle-1`、`game-1`、`gate`、`platform` 以及内部 MongoDB、etcd、NATS。

对宿主机仅开放：gate WebSocket 的 `${GATE_PORT}`（默认 `8081`）和 platform 支付回调的 `${PLATFORM_WEBHOOK_PORT}`（默认 `7001`）。MongoDB、etcd、NATS、game gRPC、battle 和 platform gRPC 均只在 Compose 网络内可见。

> gate 的实际监听端口为 `gate-base-port + gate-id`，默认即 `8080 + 1 = 8081`。

## 初次部署

1. 在服务器安装 Docker Engine（含 Compose v2），并把本目录中的 `.env.example` 复制为 `.env`。
2. 将 `.env` 中的 `MONGO_ROOT_PASSWORD`、`JWT_SECRET` 替换为高强度随机值；不要将 `.env` 提交到 Git。
3. **在启动 game/battle 前**将 `../glconf` 内的配置表导入 `${GCONF_DB}`。项目现有工具可执行：

   ```bash
   python3 tools/import-conf.py \
     "mongodb://<user>:<password>@<mongo-host>:27017/?authSource=admin" \
     "gs_conf_dev" ../glconf
   ```

   该导入会以仓库内容替换目标数据库中的同名配置集合，生产数据库操作前请先备份。
4. 构建并启动：

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

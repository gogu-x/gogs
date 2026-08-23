# Xray TLS 证书续期与服务重启排查指南

本文档整理一次 `VLESS + XTLS Vision + TLS` 节点延迟显示 `-1 ms` 的完整排查过程，覆盖证书过期判断、Certbot 续期、Nginx 端口冲突、残留进程处理、Xray 证书重载及自动续期配置。

> 适用环境：Ubuntu、systemd、Xray、Nginx、Certbot（Let's Encrypt）。示例域名为 `secure3.cn`，Xray 端口为 `8088`，请按实际环境替换。

---

## 一、故障现象

客户端出现以下现象：

- 节点延迟显示 `-1 ms`、`none`；
- 本地 v2rayN 和 Xray 核心仍在运行；
- 本地 SOCKS 端口能够接受连接；
- 日志出现 TLS、EOF、连接超时或 `closed pipe`；
- 经代理访问 HTTPS 地址时 TLS 握手失败。

`-1 ms` 不是实际网络延迟，而是**测速请求未获得有效结果**。可能原因包括节点不可达、TLS 握手失败、证书失效、协议参数不匹配或服务端进程异常。

本次故障中，本地代理端口工作正常，节点 TCP 端口也能连接，但服务端 TLS/应用层无法正常完成请求。

---

## 二、配置关系

服务端核心参数如下：

| 参数 | 值 |
|---|---|
| 协议 | VLESS |
| Flow | `xtls-rprx-vision` |
| 传输 | TCP（新版配置中客户端可能显示为 `raw`） |
| 安全层 | TLS |
| 服务端口 | `8088` |
| 证书 | `/etc/letsencrypt/live/secure3.cn/fullchain.pem` |
| 私钥 | `/etc/letsencrypt/live/secure3.cn/privkey.pem` |

客户端必须对应使用 VLESS、`xtls-rprx-vision` 和 TLS。Vision 不应开启 Mux；客户端 `raw` 与服务端 `tcp` 在新版 Xray 中是兼容表示。

> 安全提示：VLESS UUID 等同于访问凭据。文档、截图和聊天记录中必须脱敏；如果 UUID 已公开，应立即在服务端生成新 UUID，并同步更新客户端。

---

## 三、排查链路

### 1. 确认本地代理核心和监听端口

Windows 客户端可执行：

```powershell
Get-Process v2rayN,xray -ErrorAction SilentlyContinue
Get-NetTCPConnection -State Listen |
  Where-Object LocalPort -eq 10808
```

如果 Xray 正常监听 `127.0.0.1:10808`，但经该端口访问 HTTPS 仍失败，问题更可能位于当前上游节点。

可进行一次实际代理请求：

```powershell
curl.exe --max-time 15 `
  --proxy socks5h://127.0.0.1:10808 `
  --output NUL `
  --write-out "http=%{http_code} connect=%{time_connect}s tls=%{time_appconnect}s total=%{time_total}s`n" `
  https://www.google.com/generate_204
```

返回 `http=000`、TLS 握手失败或超时，说明测速显示 `-1` 有真实连接故障，不只是 UI 显示问题。

### 2. 确认节点 TCP 端口

在服务端执行：

```bash
sudo ss -lntp | grep ':8088'
```

客户端可测试端口是否可达：

```powershell
Test-NetConnection secure3.cn -Port 8088
```

`TcpTestSucceeded=True` 只能说明 TCP 三次握手成功，不能证明 TLS、VLESS 或证书正常。

### 3. 直接检查服务端提供的 TLS 证书

在服务器本机执行：

```bash
openssl s_client \
  -connect 127.0.0.1:8088 \
  -servername secure3.cn \
  -tls1_3 -brief </dev/null
```

本次故障得到：

```text
verify error:num=10:certificate has expired
notAfter=Aug 13 14:41:27 2026 GMT
Verification error: certificate has expired
```

这证明 Xray 当时实际提供的是已过期证书。`notAfter` 使用 GMT/UTC，比较时间时需注意服务器本地时区。

---

## 四、区分“磁盘证书过期”和“Xray 未重载”

`openssl s_client` 检查的是**运行中的 Xray 实际提供的证书**；下面的命令检查的是**磁盘上的证书文件**：

```bash
sudo openssl x509 \
  -in /etc/letsencrypt/live/secure3.cn/fullchain.pem \
  -noout -dates -serial -subject -issuer
```

根据结果分两种情况处理。

### 情况 A：磁盘证书也已过期

需要先运行 Certbot 续期：

```bash
sudo certbot renew
```

续期成功后重启 Xray：

```bash
sudo systemctl restart xray
```

### 情况 B：磁盘证书有效，但 Xray 仍提供旧证书

例如 Certbot 显示：

```text
Certificate not yet due for renewal
/etc/letsencrypt/live/secure3.cn/fullchain.pem expires on 2026-11-11
```

而 `openssl s_client` 仍显示旧证书在 8 月 13 日过期，说明：

1. Certbot 已更新 `/etc/letsencrypt/live/...` 指向的新证书；
2. Xray 启动时把旧证书加载进内存；
3. 证书更新后没有重启 Xray。

此时无需强制续期，只需：

```bash
sudo systemctl restart xray
sudo systemctl status xray --no-pager -l
```

> 不建议此时使用 `certbot renew --force-renewal`。磁盘证书已经有效，重复签发没有必要，还可能触发 Let's Encrypt 频率限制。

---

## 五、Certbot 无法绑定 80 端口

### 1. 错误表现

```text
Could not bind TCP port 80 because it is already in use by another process
```

这通常表示当前证书使用 Certbot `standalone` 验证方式。Certbot 需要临时监听公网 TCP 80 完成 HTTP-01 验证，但 Nginx、Apache、Caddy、Docker 端口映射或其他进程已占用该端口。

先检查续期方式：

```bash
sudo grep -E '^(authenticator|installer|webroot_path)' \
  /etc/letsencrypt/renewal/secure3.cn.conf
```

再检查端口占用者：

```bash
sudo ss -ltnp | grep -E '(:80[[:space:]])'
sudo lsof -nP -iTCP:80 -sTCP:LISTEN
sudo fuser -v 80/tcp
```

### 2. 不要盲目结束端口占用进程

停止 80 端口服务会造成 HTTP 服务中断。操作前必须确认进程用途，并保留恢复命令。

常见占用者：

| 进程 | 说明 |
|---|---|
| `nginx` | Nginx Web 服务 |
| `apache2` | Apache Web 服务 |
| `caddy` | Caddy Web 服务 |
| `docker-proxy` | Docker 容器映射了宿主机 80 端口 |

确认是由 systemd 管理的 Nginx 后，临时续期可使用：

```bash
sudo systemctl stop nginx
sudo certbot renew
renew_status=$?
sudo systemctl start nginx

if [ "$renew_status" -eq 0 ]; then
  sudo systemctl restart xray
fi
```

无论 Certbot 成功还是失败，都必须恢复 Nginx。

---

## 六、Nginx 停止后仍占用 80 端口

### 1. 现象

执行 `systemctl stop nginx` 后，端口仍被 Nginx 占用：

```text
LISTEN ... 0.0.0.0:80 ... users:(("nginx",pid=...,fd=...))
```

执行 `nginx -s quit` 又出现：

```text
open() "/run/nginx.pid" failed (2: No such file or directory)
```

这说明可能存在未被 systemd 正确管理的残留 Nginx 进程，且 PID 文件已经丢失。

### 2. 确认 master 和 worker

```bash
sudo systemctl show nginx \
  -p ActiveState -p SubState -p MainPID

ps -o pid,ppid,user,stat,lstart,cmd -p <PID1>,<PID2>
```

命令行包含 `nginx: master process` 的是主进程；包含 `nginx: worker process` 的是工作进程。

### 3. 优雅停止残留主进程

确认主进程 PID 后：

```bash
sudo kill -QUIT <NGINX_MASTER_PID>
sleep 3

sudo ss -ltnp | grep -E '(:80[[:space:]])' \
  || echo "端口 80 已释放"
```

只有确认端口释放后，才能运行 standalone Certbot。

> 禁止未经确认直接执行 `kill -9` 或批量 `pkill nginx`。应优先向明确的 master PID 发送 `QUIT`，让 Nginx 优雅退出并处理已有连接。

续期完成后恢复服务：

```bash
sudo systemctl start nginx
sudo systemctl status nginx --no-pager -l
```

如果启动失败：

```bash
sudo nginx -t
sudo journalctl -u nginx -n 100 --no-pager
sudo ss -lntp | grep ':80'
```

---

## 七、完整恢复流程

推荐按以下顺序操作，避免重复续期和不必要停机。

### 步骤 1：检查磁盘证书

```bash
sudo openssl x509 \
  -in /etc/letsencrypt/live/secure3.cn/fullchain.pem \
  -noout -dates -serial
```

### 步骤 2：仅在磁盘证书过期或即将过期时续期

```bash
sudo certbot renew
```

如果 standalone 因 80 端口冲突失败，先按第五、六节确认并处理占用者。

### 步骤 3：恢复 Nginx

```bash
sudo systemctl start nginx
sudo systemctl is-active nginx
```

### 步骤 4：让 Xray 加载新证书

```bash
sudo systemctl restart xray
sudo systemctl is-active xray
```

如果 Xray 启动失败：

```bash
sudo systemctl status xray --no-pager -l
sudo journalctl -u xray -n 100 --no-pager
sudo tail -n 100 /var/log/xray/error.log
```

重点检查：

- `certificateFile`、`keyFile` 路径是否正确；
- Xray 运行用户是否有权读取证书和私钥；
- 证书与私钥是否匹配；
- `8088` 是否被其他进程占用；
- Xray 配置文件语法是否有效。

---

## 八、恢复后的验证清单

### 1. 服务状态

```bash
sudo systemctl status nginx xray --no-pager -l
```

### 2. 监听端口

```bash
sudo ss -lntp | grep -E ':(80|8088)[[:space:]]'
```

### 3. 磁盘证书有效期

```bash
sudo openssl x509 \
  -in /etc/letsencrypt/live/secure3.cn/fullchain.pem \
  -noout -dates -serial -subject
```

### 4. Xray 实际提供的证书

```bash
openssl s_client \
  -connect 127.0.0.1:8088 \
  -servername secure3.cn \
  -tls1_3 </dev/null 2>/dev/null |
openssl x509 -noout -dates -serial -subject
```

磁盘证书和 Xray 实际提供证书的 `serial`、`notAfter` 应一致。

### 5. 从外部网络验证

在另一台机器执行：

```bash
openssl s_client \
  -connect secure3.cn:8088 \
  -servername secure3.cn \
  -tls1_3 -brief </dev/null
```

确认不再出现：

```text
certificate has expired
```

最后在 v2rayN 中重新测试节点延迟，并进行实际 HTTPS 访问验证。

---

## 九、配置自动续期和 Xray 自动重启

### 1. 检查 Certbot timer

```bash
sudo systemctl enable --now certbot.timer
sudo systemctl status certbot.timer --no-pager
sudo systemctl list-timers --all | grep certbot
```

### 2. 添加 deploy hook

Certbot 只有在证书真正续期成功后才执行 deploy hook。创建 Xray 重启脚本：

```bash
sudo install -d -m 755 /etc/letsencrypt/renewal-hooks/deploy

cat <<'EOF' | sudo tee \
  /etc/letsencrypt/renewal-hooks/deploy/restart-xray.sh >/dev/null
#!/bin/sh
set -eu
systemctl restart xray
systemctl is-active --quiet xray
EOF

sudo chmod 755 \
  /etc/letsencrypt/renewal-hooks/deploy/restart-xray.sh
```

### 3. 测试自动续期

```bash
sudo certbot renew --dry-run
```

如果还在使用 standalone，且 Nginx 长期占用 80 端口，dry-run 仍会失败。不要把“长期停止 Nginx”作为自动续期方案，应改用 Nginx 插件、webroot 或 DNS 验证。

如需在 dry-run 时同时测试 deploy hook，并可接受测试期间重启一次 Xray：

```bash
sudo certbot renew --dry-run --run-deploy-hooks
```

---

## 十、长期解决 80 端口冲突

### 方案 A：使用 Nginx 插件

适用于 `secure3.cn` 已由本机 Nginx 正常接收 80/443 请求的环境：

```bash
sudo certbot plugins
sudo certbot certonly \
  --nginx \
  --cert-name secure3.cn \
  -d secure3.cn
```

执行前应备份 Nginx 配置，并用 `sudo nginx -t` 验证。插件可能修改 Nginx 配置，生产环境操作前需评估。

### 方案 B：使用 webroot

适用于已经明确 Nginx 站点根目录的环境：

```bash
sudo certbot certonly \
  --webroot \
  -w /var/www/html \
  --cert-name secure3.cn \
  -d secure3.cn
```

`/var/www/html` 必须替换为 `secure3.cn` 实际站点根目录，并确保公网能够访问：

```text
http://secure3.cn/.well-known/acme-challenge/<token>
```

### 方案 C：DNS-01

适用于不能开放 80 端口、使用 CDN，或服务端不方便运行 Web 服务的环境。应优先使用 DNS 服务商官方 Certbot 插件实现自动化，不建议长期依赖手工 DNS TXT 验证。

---

## 十一、本次故障的根因链路

本次事件不是单一故障，而是以下问题叠加：

1. Xray 运行中的 TLS 证书已过期，导致客户端测速返回 `-1`；
2. Certbot 证书文件后来已更新至新的有效期，但 Xray 没有重启，仍使用内存中的旧证书；
3. 证书使用 standalone 验证方式，与长期监听 80 端口的 Nginx 冲突；
4. Nginx 曾出现 systemd 状态、残留进程与 `/run/nginx.pid` 不一致，增加了临时释放端口的难度；
5. 缺少 Certbot deploy hook，证书续期后不能自动触发 Xray 重载。

最终恢复关键不是反复强制续期，而是确认磁盘证书已经有效，然后重启 Xray，让它加载新证书。

---

## 十二、日常检查建议

建议定期执行：

```bash
# 磁盘证书到期时间
sudo openssl x509 \
  -in /etc/letsencrypt/live/secure3.cn/fullchain.pem \
  -noout -enddate

# Xray 实际提供的证书到期时间
openssl s_client \
  -connect 127.0.0.1:8088 \
  -servername secure3.cn </dev/null 2>/dev/null |
openssl x509 -noout -enddate

# 自动续期状态
sudo systemctl list-timers --all | grep certbot
sudo journalctl -u certbot.service -n 50 --no-pager
```

磁盘证书与运行中证书的到期时间应保持一致。发现不一致时，优先检查 Xray 是否在证书更新后完成重启。

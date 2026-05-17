# HTTPS 证书申请与配置全流程文档

## 环境信息

- 服务器：Ubuntu（VM-0-14-ubuntu）
- 域名：secure3.cn
- Web 服务器：Nginx
- 证书颁发机构：Let's Encrypt（免费，90天自动续期）

---

## 前提条件

1. 拥有一个域名（本例：secure3.cn）
2. 域名 DNS 已解析到服务器公网 IP
3. 服务器 80 / 443 端口已对外开放
4. 已安装 Nginx

---

## 一、安装 Certbot

```bash
sudo apt update
sudo apt install certbot python3-certbot-nginx -y
```

---

## 二、申请证书

使用 nginx 插件模式申请（不需要停止 nginx）：

```bash
sudo LC_ALL=C.UTF-8 certbot --nginx -d secure3.cn
```

过程中需要：
1. 输入邮箱地址（用于接收到期提醒）
2. 同意服务条款，输入 `Y`
3. 是否接收 EFF 邮件，按需选择 `Y` 或 `N`

申请成功后输出：
```
Certificate is saved at: /etc/letsencrypt/live/secure3.cn/fullchain.pem
Key is saved at:         /etc/letsencrypt/live/secure3.cn/privkey.pem
```

---

## 三、证书文件路径

| 文件 | 路径 |
|------|------|
| 证书（含中间链） | `/etc/letsencrypt/live/secure3.cn/fullchain.pem` |
| 私钥 | `/etc/letsencrypt/live/secure3.cn/privkey.pem` |
| 续期配置 | `/etc/letsencrypt/renewal/secure3.cn.conf` |

---

## 四、Nginx 配置

certbot 自动写入 `/etc/nginx/sites-enabled/default`，内容如下：

```nginx
# HTTP 自动跳转 HTTPS
server {
    listen 80;
    server_name secure3.cn;
    return 301 https://$host$request_uri;
}

# HTTPS 主配置
server {
    listen 443 ssl;
    server_name secure3.cn;

    ssl_certificate /etc/letsencrypt/live/secure3.cn/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/secure3.cn/privkey.pem;

    # 反向代理到后端服务
    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

修改配置后重载 nginx：
```bash
sudo nginx -t          # 检查配置语法
sudo systemctl reload nginx
```

---

## 五、修复自动续期配置

certbot 默认用 standalone 模式续期，会与 nginx 冲突（80 端口被占用）。
需改为 nginx 模式：

```bash
sudo sed -i 's/authenticator = standalone/authenticator = nginx/' /etc/letsencrypt/renewal/secure3.cn.conf
```

---

## 六、验证自动续期

```bash
sudo certbot renew --dry-run
```

输出 `Congratulations, all simulated renewals succeeded` 表示正常。

---

## 七、常用命令

```bash
# 查看已申请的证书列表
sudo certbot certificates

# 手动续期
sudo certbot renew --nginx

# 查看证书到期时间
sudo certbot certificates | grep Expiry

# 重载 nginx
sudo systemctl reload nginx
```

---

## 注意事项

- 证书有效期 **90 天**，certbot 安装后会自动注册 systemd timer 定时续期，无需手动操作
- 续期必须使用 `--nginx` 模式，否则因 80 端口被 nginx 占用会失败
- Let's Encrypt 同一域名每周最多申请 **5 次**，测试时注意
- 证书只支持域名，不支持裸 IP

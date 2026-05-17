# Ubuntu 24.04 安装 Docker

## 环境
- Ubuntu 24.04 LTS (Noble Numbat)
- 使用阿里云镜像源（国内服务器）

## 安装步骤

### 1. 安装依赖

```bash
sudo apt update
sudo apt install -y ca-certificates curl gnupg
```

### 2. 添加 GPG key

```bash
sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://mirrors.aliyun.com/docker-ce/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
sudo chmod a+r /etc/apt/keyrings/docker.gpg
```

### 3. 添加仓库

```bash
sudo tee /etc/apt/sources.list.d/docker.list > /dev/null <<'EOF'
deb [arch=amd64 signed-by=/etc/apt/keyrings/docker.gpg] https://mirrors.aliyun.com/docker-ce/linux/ubuntu noble stable
EOF
```

### 4. 安装 Docker

```bash
sudo apt update
sudo apt install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
```

### 5. 启动并设置开机自启

```bash
sudo systemctl enable docker --now
```

### 6. 允许当前用户免 sudo 使用 Docker（需重新登录生效）

```bash
sudo usermod -aG docker $USER
```

### 7. 验证安装

```bash
sudo docker run hello-world
```

## 配置镜像加速（可选）

```bash
sudo tee /etc/docker/daemon.json <<EOF
{
  "registry-mirrors": [
    "https://mirror.ccs.tencentyun.com",
    "https://registry.cn-hangzhou.aliyuncs.com"
  ]
}
EOF

sudo systemctl restart docker
```

## 常见问题

**E: Package 'docker-ce' has no installation candidate**
- 原因：仓库未添加成功，通常是 sources.list 格式错误或 GPG key 缺失
- 解决：删除 `/etc/apt/sources.list.d/docker.list` 和 `/etc/apt/keyrings/docker.gpg`，重新执行步骤 2~4

**E: Malformed entry in list file**
- 原因：sources.list 写入时产生了多余换行
- 解决：使用 `tee + heredoc` 方式写入（见步骤3），避免 echo 换行问题

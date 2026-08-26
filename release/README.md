# FunAuth 用户中心 · Release

## 产物清单

| 文件 | 大小 | 说明 |
|---|---|---|
| `funauth-linux-amd64` | ~25 MB | 单一二进制（含 Bunker 控制台 + Phoenix + 用户中心 + 配置向导） |
| `start.sh` | 启动脚本 | 一键启动 |
| `ocr_resources/` | OCR 模型 | 4399 验证码识别用（运行时需要） |

## 快速开始

### 1. 首次启动（配置向导）

```bash
cd release
./start.sh
# 或直接运行
./funauth-linux-amd64 serve
```

浏览器访问 `http://localhost:8090/`，按向导完成三步配置：

1. **数据库**：填 MySQL 主机/端口/用户/密码/库名（库不存在会自动创建）
2. **管理员账号**：设置超级管理员用户名 + 密码（自动加入"管理员"组）
3. **网站信息**：站点名、副标题、Logo、页脚文案

点击「完成配置」后，向导会自动：
- 写入 `config.json`
- 创建 18 张数据库表
- 创建管理员账号（bcrypt 加密）
- 创建默认身份组（管理员 / 普通用户）
- 保存网站信息到 `system_settings`

### 2. 重启进入正式服务

```bash
./funauth-linux-amd64 serve
```

看到日志输出：
```
[serve] listening on :8090 (db=true phoenix=true webui=true)
[serve] WebUI: http://localhost:8090/ui/
[serve] 用户中心: http://localhost:8090/uc/
```

### 3. 访问入口

| 入口 | 地址 | 用途 |
|---|---|---|
| 用户中心 SPA | http://localhost:8090/uc/ | Vue3 用户中心（登录/仪表盘/卡槽/商店/兑换/流水/公告/管理后台） |
| Bunker 控制台 | http://localhost:8090/ui/ | 原 webui（账号管理/批量注册） |
| 健康检查 | http://localhost:8090/api/usercenter/health | API 自检（含站点信息） |
| 配置向导 | http://localhost:8090/setup | 仅在 `--setup` 模式或未配置时可用 |

## 命令行参数

```bash
./funauth-linux-amd64 serve [选项]

选项：
  --addr :8090     监听地址（或 FUNAUTH_ADDR 环境变量）
  --no-db          跳过 DB 初始化
  --no-webui       不挂载 Bunker 控制台
  --no-phoenix     不挂载 Phoenix 验证端点
  --setup          强制进入 Web 配置向导模式
  --with-webui     挂载 WebUI 控制台（默认 true）
```

## 环境变量

| 变量 | 默认 | 说明 |
|---|---|---|
| `FUNAUTH_ADDR` | `:8090` | 监听地址 |
| `FUNAUTH_MYSQL_DSN` | 空 | MySQL DSN（设置后跳过 config.json 和向导） |
| `FUNAUTH_JWT_SECRET` | 随机 | JWT 密钥（生产必须固定，否则重启后所有用户被踢下线） |
| `GIN_MODE` | `debug` | 设为 `release` 关闭调试日志 |

## 单二进制包含的能力

- **Bunker 控制台**（`/ui/`）：原有 webui，账号管理、批量注册
- **Phoenix 验证端点**（`/api/phoenix/*`）：4399 协议验证
- **用户中心 API**（`/api/usercenter/*`）：认证/资料/钱包/卡槽/商店/兑换码/公告/API Key/管理员
- **用户中心 SPA**（`/uc/`）：Vue3 + Element Plus 前端
- **配置向导**（`/setup`）：首次启动自动进入，支持 Web 表单配置

## 数据库表

共 18 张表（首次启动自动创建）：
- 复用表：`users` `slots` `account` `idcode` `sauth`
- 用户中心：`groups` `user_groups` `wallets` `wallet_transactions` `products` `categories` `orders` `redemption_codes` `api_keys` `announcements` `system_settings` `quota_transactions` `times_transactions`

## 生产部署建议

```bash
# 固定 JWT 密钥
export FUNAUTH_JWT_SECRET="$(openssl rand -hex 32)"

# 关闭调试
export GIN_MODE=release

# 用 systemd 守护
cat > /etc/systemd/system/funauth.service <<'EOF'
[Unit]
Description=FunAuth User Center
After=network.target mysql.service

[Service]
Type=simple
WorkingDirectory=/opt/funauth
ExecStart=/opt/funauth/funauth-linux-amd64 serve
Environment=FUNAUTH_JWT_SECRET=你的固定密钥
Environment=GIN_MODE=release
Restart=always

[Install]
WantedBy=multi-user.target
EOF
systemctl enable --now funauth
```

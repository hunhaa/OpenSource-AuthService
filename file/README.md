# FunAuth - 一体化 G79 / Phoenix / 租赁服验证工具集 (Linux amd64)

FunAuth Unified CLI 把原来散落的 10+ 个二进制整合成 1~2 个核心可执行文件，同时保留全部 test / register 工具作为独立程序。

## 一、文件清单

| 文件名 | 说明 | 等价旧入口 |
| --- | --- | --- |
| `funauth-linux-amd64` | **核心二进制，推荐只用这个**。默认 `serve` 启动：WebUI 控制台 + Phoenix 验证服务器 + DB（可选） + 所有子命令 | `funauth-linux-amd64` + `webui-linux-amd64` + `com4399register-linux-amd64` |
| `webui-linux-amd64` | 只跑 WebUI 控制台的独立二进制（默认进入 `webui` 子命令） | `webui-linux-amd64` |
| `com4399register-linux-amd64` | 独立版 4399 批量注册工具 | `com4399register-linux-amd64` |
| `test-bruteforce-linux-amd64` | 暴力枚举 PeAuth sp/tr 参数的测试工具 | `test-bruteforce-linux-amd64` |
| `test-com4399-linux-amd64` | 4399 注册流程冒烟测试 | `test-com4399-linux-amd64` |
| `test-com4399login-linux-amd64` | 4399 登录拿 SAuth 的冒烟测试 | `test-com4399login-linux-amd64` |
| `test-proxy-linux-amd64` | 代理池可用性测试 | `test-proxy-linux-amd64` |
| `test-runtest-linux-amd64` | 综合跑测入口 | `test-runtest-linux-amd64` |
| `test-test_mcp-linux-amd64` | MCPK 解包工具测试 | `test-test_mcp-linux-amd64` |
| `test-test_skin-linux-amd64` | 皮肤/资源相关冒烟测试 | `test-test_skin-linux-amd64` |

> **版本**：基于 G79 Engine `3.x` + Patch `3.9.23.298289`（运行 `./funauth-linux-amd64 version` 查看）。

---

## 二、快速上手（3 步）

```bash
# 1. 给二进制加执行权限（如果从 zip 解压出来没有）
chmod +x funauth-linux-amd64

# 2. 直接运行，默认进入 serve：WebUI + Phoenix + DB 初始化引导（首次需要填 MySQL）
./funauth-linux-amd64

# 3. 浏览器打开控制台
#   http://你的服务器IP:8090/ui/
```

如果不想启动 DB 或没有 MySQL，跳过数据库（只保留 WebUI + Phoenix 端点）：

```bash
./funauth-linux-amd64 serve --no-db
```

---

## 三、WebUI 使用流程（租赁服 AuthV2 标准步骤）

WebUI 修复了两个关键问题：
- 解决了 `/api/new` 路由重复注册导致的 panic；
- 修复了 AuthV2 的 `user-token` 被错误 hex 编码导致的 `status=401 body=""`。

### 步骤 1：登录 G79
在 WebUI `登录` 选项卡：
- **方式 A（推荐）**：把 4399 登录拿到的 SAuth Cookie 整串粘进去 → 点 `登录`
- **方式 B**：已有 FBToken 的选 FBToken 标签
成功后右上角会显示你的昵称、UID、等级。

### 步骤 2：启动 Link 连接（关键！必须在 AuthV2 之前做）
切到 `Link` 选项卡 → 点 **启动 Link**：
- 等待日志显示 `Link已建立，GameStart已发送`（通常 1~3 秒）。
- 如果这里失败：检查账号是否被封禁 / 网络是否能访问 G79 Link 服务器 / UserToken 是否过期（可重新登录拿新 Token）。

### 步骤 3：搜索 & 进入租赁服
`搜索` 选项卡输入租赁服名字 → 记下 `entity_id`（就是 ServerID），或者直接在 `租赁服` 选项卡填 ServerID：
1. 填入 ServerID 和密码（若有）
2. 先点 **Enter 租赁服**（可选，用来确认 ServerID/密码正确）
3. 再点 **获取 AuthV2**

结果会显示 `chain_info_b64`、`chain_info_hex`、`chain_info_len`，这就是 NeoOmega / ToolDelta / Bunker 等客户端需要的认证材料。

### 常见报错对照表

| 报错 | 原因 | 解决 |
| --- | --- | --- |
| `panic: handlers are already registered for path '/api/new'` | 用了**旧**二进制（路由注册冲突） | 用本 zip 里的 `funauth-linux-amd64`（已修复） |
| `AuthV2请求失败: AuthV2响应异常 status=401 body=""` | **旧版 bug**：user-token 多做了一层 hex encode | 用本 zip 里的二进制（已去掉多余编码） |
| `未启动Link连接，请先调用 /link/start 建立连接并完成GameStart` | 没启动 Link，或启动后 Link 已断开 | 重新点 `启动 Link`，看到成功日志再继续 |
| `会话不存在或已过期` | WebUI 登录 token 过期（默认 1 小时） | 重新登录一次 |
| `EnterRentalServerWorld失败` / `进入租赁服失败 code != 0` | ServerID 错、密码错、租赁服未开服、账号无权限 | 核对 ServerID（从搜索结果里拿 `entity_id`），确认服开着 |

---

## 四、`funauth-linux-amd64` 全部子命令

```
用法:
  funauth-linux-amd64 [子命令] [选项...]
```

### 4.1 `serve` — 一体化 HTTP 服务（默认，不带参数就进这个）

```bash
# 默认：DB + Phoenix + WebUI 全部启动，首次会交互引导填 MySQL
./funauth-linux-amd64

# 只跑 Phoenix/NeoOmega 验证服务器（不挂 WebUI，当纯后端验证）
./funauth-linux-amd64 serve --no-webui

# 只跑 Bunker-Web 风格 WebUI（不挂 Phoenix 端点）
./funauth-linux-amd64 serve --no-phoenix

# 跳过 DB，无 MySQL 也能跑
./funauth-linux-amd64 serve --no-db

# 改监听地址/端口
./funauth-linux-amd64 serve --addr :9000
# 或用环境变量：FUNAUTH_ADDR=:9000 ./funauth-linux-amd64
```

### 4.2 `webui` — 只跑 WebUI 控制台

```bash
./funauth-linux-amd64 webui
# 或者直接用独立二进制：./webui-linux-amd64
```

### 4.3 `register` — 4399 账号注册

```bash
# 批量注册（和旧版 com4399register-linux-amd64 完全一致）
./funauth-linux-amd64 register

# 注册 1 个，直接输出账号密码
./funauth-linux-amd64 register oneshot
#   username=xxxxx
#   password=xxxxx

# 注册 1 个 + 登录 + 打印完整 SAuth Cookie（可直接粘贴 WebUI 登录）
./funauth-linux-amd64 register sauth
```

### 4.4 `sauth` — 用已有账号密码拿 SAuth

```bash
./funauth-linux-amd64 sauth <用户名> <密码>
```

### 4.5 `nickname` — 辅助账号昵称处理

```bash
# 如果昵称为空，自动改成 nklm_XXXXX 格式
./funauth-linux-amd64 nickname <用户> <密码>

# 也可以显式改成指定名字
./funauth-linux-amd64 nickname <用户> <密码> "我的昵称"
```

### 4.6 `test <子名>` — 调用各测试工具

```bash
./funauth-linux-amd64 test bruteforce      # 原 test-bruteforce-linux-amd64
./funauth-linux-amd64 test com4399         # 原 test-com4399-linux-amd64
./funauth-linux-amd64 test com4399login    # 原 test-com4399login-linux-amd64
./funauth-linux-amd64 test proxy           # 原 test-proxy-linux-amd64
./funauth-linux-amd64 test runtest         # 原 test-runtest-linux-amd64
./funauth-linux-amd64 test mcp             # 原 test-test_mcp-linux-amd64
./funauth-linux-amd64 test skin            # 原 test-test_skin-linux-amd64
```
也可以直接跑对应独立二进制：`./test-com4399-linux-amd64`。

### 4.7 `version` / `help`

```bash
./funauth-linux-amd64 version    # 查看版本
./funauth-linux-amd64 help       # 查看完整帮助
```

---

## 五、与 NeoOmega / ToolDelta / Bunker 对接

AuthV2 成功后拿到的字段：
- `chain_info_b64`：Base64 编码的 chain_info（大多数工具直接用这个）
- `chain_info_hex`：Hex 编码的 chain_info
- `chain_info_len`：字节长度，可用来判断是否为空（正常值几百 ~ 几千字节）

典型 Bunker 风格 `/api/new` 接口也在，路径：
```
POST http://<host>:8090/api/new
```

---

## 六、常见运维

### 1. 端口被占用怎么处理？
二进制启动前会自动 `SIGTERM/SIGKILL` 监听目标端口的旧进程（Linux 下），一般直接重启即可。也可以手动：
```bash
ss -lntp | grep 8090
kill -9 <pid>
```

### 2. DB 配置文件在哪？
默认当前目录的 `config.json`：
```json
{
  "db": {
    "host": "127.0.0.1",
    "port": 3306,
    "user": "root",
    "password": "",
    "name": "funauth"
  }
}
```
不想用 DB 就加 `--no-db`，不需要这个文件。

### 3. 怎么验证所有端点活着？
```bash
curl -s http://127.0.0.1:8090/api/new        # 应该返回 JSON（不 panic）
curl -s http://127.0.0.1:8090/ui/             # 应该返回 WebUI HTML
```

---

## 七、架构说明（本次更新修复了什么）

1. **路由冲突修复**：原来 `cmd/funauth/internal/router` 会同时注册 handlers 和 webui 的两套 `/api/new`，导致 gin panic。现在统一入口：`with-webui` 模式只由 `webui.RegisterRoutes` 注册一次，Phoenix 的 `tan_lobby_*` 等非重叠路由单独追加（`RegisterPhoenixNonOverlapRoutes`），不重复占用路径。
2. **AuthV2 401 修复**：`modules/g79client/auth.go` 中 `SendAuthV2Request` 的 `user-token` 被错误做了 `hex.EncodeToString([]byte(token))`，服务端收到的是 ASCII hex 后的乱码，验签必失败。已改为与其他所有 HTTP API 一致：直接 `req.Header.Set("user-token", token)`，不再 hex encode。
3. **Unified CLI**：把所有子命令（serve / webui / register / sauth / nickname / test）放进 `internal/unified/cli.go`，`funauth-linux-amd64` 和 `webui-linux-amd64` 只是套一层不同 flavor 的默认子命令，减少重复代码。
4. **构建产物同步**：`file/`（根）、`file/new/`、`file/unified/` 三份目录保持完全一致，方便你直接替换 `/Funauth/` 下的旧二进制。

Have fun! 🎮

# HutaomuChat Server

> 安全、简洁的即时通讯应用 — 服务端

HutaomuChat 的后端服务。**Go 1.24+ 实现，零第三方依赖**，仅使用标准库，
离线环境下也能直接编译出可执行文件。配套 Android 客户端。

支持私聊、群聊、朋友圈、表情包、文件传输、系统推送与网页管理后台。

---

## 特性一览

### 实时通信
- WebSocket 长连接，消息毫秒级送达
- **多端同时在线**：同一账号可在手机、平板、电脑同时登录，消息投递到全部设备
- 离线消息队列，重新上线后自动补投
- 消息撤回，撤回事件同样支持离线同步
- 输入中状态实时广播
- @全体成员（仅群主与管理员，每日限额，绕过免打扰）

### 社交功能
- 好友系统：搜索、推荐、申请、免验证自动通过
- 群聊：创建、入群申请、群公告、管理员任免、@提及、免打扰、置顶、群头像、群文件
- 朋友圈与用户圈：发布动态、点赞、评论、多图、可见范围隔离
- 表情包：自定义上传、云端同步
- 个人资料：头像、背景图、签名、性别、地区

### 管理后台 (`/admin`)
- 首次部署初始化向导（管理员账号、JWT 密钥、个推推送配置）
- 仪表盘：8 张统计卡、14 天消息趋势面积图、消息类型环形图
- 12 个功能页面：仪表盘 / 在线用户 / 用户管理 / 会话与群组 / 消息记录 /
  朋友圈动态 / 群公告 / 入群申请 / 运行日志 / 操作审计 / 系统状态 / 服务设置
- 在线用户实时列表、封禁解封、密码重置、全服广播通知
- 所有管理员动作写入审计日志
- 深色 / 浅色双主题，响应式布局
- 通过 WebSocket 实时推送在线人数与服务端事件

### 可靠性与安全
- 密码使用 PBKDF2-HMAC-SHA256（21 万轮，每用户独立随机盐）
- JWT 采用 HMAC-SHA256 签名，访问令牌短时效 + 刷新令牌长时效自动续期
- 数据快照原子写入（临时文件 + rename），并保留备份副本，文件损坏自动回退
- 上传文件基于引用清理：头像、背景图等仍被引用的文件永不被删除
- 上传接口使用 MIME + 扩展名双白名单，阻断可执行文件上传
- 下载接口做路径穿越防护
- 全量输入校验，panic 不会导致进程崩溃

---

## 快速开始

### 编译

```bash
# 需要 Go 1.24 或更高版本，无需联网拉取任何依赖
go build -o hutaomuchat-server .
```

跨平台编译：

```bash
GOOS=windows GOARCH=amd64 go build -o hutaomuchat-server.exe .
GOOS=linux   GOARCH=amd64 go build -o hutaomuchat-server-linux-amd64 .
```

也可以用 Makefile：

```bash
make build          # 当前平台
make build-windows  # Windows
make build-linux    # Linux
make test           # 运行单元测试
```

### 运行

```bash
./hutaomuchat-server
```

首次启动后打开 `http://<host>:3000/admin/setup` 完成初始化向导，
设置管理员账号、监听端口与个推推送配置。

| 入口 | 地址 |
|------|------|
| 管理控制台 | `http://<host>:3000/admin/` |
| 初始化向导 | `http://<host>:3000/admin/setup` |
| REST API | `http://<host>:3000/api/...` |
| WebSocket | `ws://<host>:3000/ws` |

### 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `PORT` | 监听端口，优先于配置文件 | 配置文件中的值 |
| `LOG_LEVEL` | 日志级别：`DEBUG` / `INFO` / `WARN` / `ERROR` | `INFO` |
| `HUTAOMUCHAT_DATA_DIR` | 数据根目录 | 可执行文件所在目录 |
| `ACCESS_TOKEN_TTL` | 访问令牌有效期（天） | `7` |
| `REFRESH_TOKEN_TTL` | 刷新令牌有效期（天） | `90` |

---

## 目录结构

```
.
├── main.go                  # 入口：依赖装配、优雅退出、重启钩子
├── internal/
│   ├── api/                 # HTTP 路由与处理器
│   │   ├── router.go        # 路由表（Go 1.22+ ServeMux）
│   │   ├── middleware.go    # 认证、初始化守卫
│   │   ├── auth.go          # 注册 / 登录 / 资料 / 令牌刷新
│   │   ├── users.go         # 搜索 / 推荐 / 详情
│   │   ├── friends.go       # 好友申请与关系
│   │   ├── chats.go         # 会话 / 群组 / 公告 / 管理员
│   │   ├── messages.go      # 消息历史 / 广播 / 推送
│   │   ├── moments.go       # 朋友圈
│   │   ├── stickers.go      # 表情包
│   │   ├── upload.go        # 文件上传与下载
│   │   ├── push.go          # 个推令牌注册
│   │   ├── setup.go         # 首次运行向导
│   │   ├── admin.go         # 管理后台 API
│   │   ├── admin_ws.go      # 管理后台实时通道
│   │   ├── cleanup.go       # 基于引用的上传文件清理
│   │   └── ws.go            # 客户端 WebSocket 协议
│   ├── config/              # 服务端配置（管理员密码以哈希存储）
│   ├── logging/             # 系统日志 + 用户操作日志
│   ├── models/              # 领域模型与对外 DTO
│   ├── push/                # 个推 UniPush 2.0 客户端
│   ├── security/            # JWT (HS256) + PBKDF2 密码哈希
│   ├── store/               # 内存数据库 + 索引 + 原子快照
│   ├── ws/                  # RFC 6455 WebSocket 实现 + 连接中心
│   └── httpx/               # JSON 助手与中间件
├── web/                     # 管理控制台（原生 JS，无构建步骤）
│   ├── index.html
│   ├── app.js
│   ├── styles.css
│   └── setup.html
├── uploads/                 # 上传文件目录
└── 服务端开发日志.md          # 服务端完整变更记录
```

运行时数据保存在可执行文件同级目录：

```
data/     server.config.json、getui.config.json、store.json（+ .bak 备份）
uploads/  上传的图片与文件
logs/     system-YYYY-MM-DD.log、user-YYYY-MM-DD.log
```

---

## 技术选型说明

旧版基于 Node.js + Express + ws，需要 7 个第三方依赖。本版全部用标准库实现，
因此 `go build` 不需要联网，也不存在依赖漏洞：

| 旧版依赖 | 本版实现 | 说明 |
|---------|---------|------|
| Express | `net/http` + Go 1.22+ `ServeMux` | 方法级路由 `GET /api/users/{id}` |
| ws | `internal/ws`（自实现 RFC 6455） | 握手、分片重组、ping/pong、关闭握手 |
| jsonwebtoken | `internal/security/jwt.go` | HS256 + `crypto/hmac` 常量时间比较 |
| bcryptjs | `internal/security/password.go` | PBKDF2-HMAC-SHA256 / 210,000 轮 |
| multer | `mime/multipart` | 标准库解析 multipart 表单 |
| uuid | `crypto/rand` | 16 字节随机数 hex 编码 |
| cors | `internal/httpx/middleware.go` | 自定义中间件 |

数据层也从 O(n) 线性扫描改为索引查询：用户按 ID / 手机号 / UID 建索引，
好友关系用双向集合，会话成员与消息按会话分桶。

---

## API 一览

完整的接口列表与 Android 客户端保持兼容。

```
认证      POST /api/auth/register       POST /api/auth/login
          POST /api/auth/refresh        GET  /api/auth/me
          PUT  /api/auth/profile
用户      GET  /api/users/search        GET  /api/users/recommended
          GET  /api/users/{id}
好友      GET  /api/friends             POST /api/friends/request
          GET  /api/friends/requests    POST /api/friends/requests/{id}/respond
          DELETE /api/friends/{friendId}
会话      GET  /api/chats               POST /api/chats/single
          POST /api/chats/group         GET  /api/chats/{id}
          POST /api/chats/{id}/leave    POST /api/chats/{id}/announcement
          GET  /api/chats/{id}/announcements
          DELETE /api/chats/{id}/announcements/{announcementId}
          PUT  /api/chats/{id}/avatar   POST /api/chats/{id}/mute
          POST /api/chats/{id}/pin      POST /api/chats/{id}/admins
          DELETE /api/chats/{id}/admins/{userId}
          GET  /api/chats/{id}/files    POST /api/chats/{id}/files/{fileId}/permanent
          DELETE /api/chats/{id}/files/{fileId}
          POST /api/chats/{id}/join     GET  /api/chats/{id}/join-requests
          GET  /api/chats/managed-join-requests
          POST /api/chats/join-requests/{requestId}/respond
          GET  /api/chats/search        GET  /api/chats/global
消息      GET  /api/messages/{chatId}   POST /api/messages/{chatId}
动态      GET  /api/moments             GET  /api/moments/friends
          POST /api/moments             POST /api/moments/{id}/like
          DELETE /api/moments/{id}/like POST /api/moments/{id}/comment
          DELETE /api/moments/{id}
其他      GET  /api/stickers            POST /api/stickers
          DELETE /api/stickers/{id}     POST /api/upload
          POST /api/upload/multiple     POST /api/push/token
          DELETE /api/push/token        GET  /api/health
```

管理接口统一位于 `/api/admin/` 下，需要管理员令牌（`role=admin`）。

---

## WebSocket 协议

客户端连接 `ws://<host>/ws` 后先发送认证帧：

```json
{"type": "authenticate", "token": "<用户 JWT>"}
```

服务端回执：

```json
{"type": "authenticated", "userId": "..."}
```

客户端可发送的帧类型：

| type | 说明 |
|------|------|
| `message` | 发送消息，字段 `chatId` / `content` / `msgType` / `messageId` |
| `typing` | 正在输入，字段 `chatId` |
| `RECALL` | 撤回消息，字段 `chatId` / `messageId` |
| `message_ack` | 消息状态回执，字段 `messageId` / `status` |
| `friend_request` | 通知好友，字段 `toUserId` |
| `ping` | 心跳 |

服务端下发的事件类型：`message`、`pending_messages`、`RECALL`、`typing`、
`online_status`、`at_all_notification`、`at_all_error`、`at_all_info`、
`force_disconnect`、`member_left`、`member_joined`、`friend_request`、
`friend_request_accepted`、`friend_request_rejected`、`JOIN_REQUEST`、
`JOIN_REQUEST_RESPONSE`、`moment_new`、`moment_like`、`moment_comment`、
`system_broadcast`、`error`、`auth_error`。

管理后台连接 `ws://<host>/ws/admin`，使用管理员令牌认证，
可接收 `stats` 与 `event` 两种帧。

---

## 会话保持与文件保留

### 令牌续期

访问令牌有效期较短（默认 7 天），同时签发一个有效期较长的刷新令牌
（默认 90 天）。客户端在收到 401 时用刷新令牌换取新的访问令牌并自动重放请求，
因此访问令牌到期不会把用户踢回登录页。

```
POST /api/auth/refresh
{ "refresh_token": "<refresh_token>" }

→ { "token": "<新的访问令牌>", "refresh_token": "<轮换后的刷新令牌>", "expires_in": 604800 }
```

刷新令牌每次使用后都会轮换，旧的立即失效。

### 上传文件保留策略

服务端每天清理一次上传目录，但**只删除「无人引用」且「超过 7 天」的文件**。
下列引用会被自动识别并永久保留：

- 用户头像、背景图与背景原图
- 表情包
- 朋友圈图片
- 群头像
- 所有消息内容中出现的 `/uploads/<文件名>`

因此头像与简介图不会因为清理任务而消失。

---

## 测试

```bash
go test ./internal/...
```

覆盖密码哈希与令牌签发校验、数据层各类操作、WebSocket 帧解析与多端连接、
HTTP 层鉴权与越权、令牌刷新流程、上传保留策略、快照损坏回退。

---

## 从 Node.js 版迁移

旧版数据保存在 `data.json`，本版保存在 `data/store.json`，两者格式不同，
**不提供自动迁移**。原因：

- 旧版密码使用 bcrypt 哈希，本版使用 PBKDF2，标准库无法校验 bcrypt，
  强行迁移会导致所有账号无法登录；
- 旧版 `data.json` 中的 `password_hash` 字段曾通过接口返回给前端，
  属于安全缺陷，不建议继续沿用这批数据。

迁移建议：以旧服务端为准导出用户手机号列表，在新服务端重新注册，
或在新服务端增加一次性导入脚本（由管理员临时设置密码）。

---

## 许可

[AGPL-3.0](LICENSE) · Copyright (C) 2026 qiuxi903

服务端以 AGPL-3.0 开源。Android 客户端不开源。

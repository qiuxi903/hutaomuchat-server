# HutaomuChat

> 安全、简洁的即时通讯应用 — 服务端

一个基于 Node.js + Express + WebSocket 的即时通讯服务端，配套 Android 客户端。支持私聊、群聊、朋友圈、表情包、文件传输、系统推送、后台管理等完整功能。

---

## 特性一览

**实时通信**
- WebSocket 长连接，消息毫秒级送达
- 离线消息队列，上线后自动投递
- 消息撤回（2 分钟内，支持离线撤回同步）
- @全体成员（管理员专属，每日限额，绕过免打扰）

**社交功能**
- 好友系统（搜索/推荐/申请/验证）
- 群聊（创建/加入申请/公告/管理员/@提及/免打扰/置顶/群头像/群文件）
- 朋友圈 & 用户圈（发布动态、点赞、评论、图片、可见范围隔离）
- 表情包（自定义上传，云端同步）
- 收藏（文本/图片/视频/文件，含来源信息）

**管理后台** (`/admin`)
- 首次部署 OOBE 向导（设置管理员、JWT 密钥、推送配置）
- 仪表盘（在线人数、消息趋势图、注册统计、消息类型分布）
- 用户管理（搜索/编辑/封禁/强制下线）
- 聊天管理（左右分栏，消息实时自动刷新）
- 朋友圈管理（卡片展示图片/点赞/评论）
- 群公告 & 入群申请管理
- 系统信息（内存/文件/推送状态）
- 亮暗双主题 + 中英文切换 + 全局自动刷新

**推送通知**
- 个推 UniPush 2.0 系统级推送（OPPO/vivo/华为/小米厂商通道）
- 应用被完全杀死后仍可收到消息通知

---

## 技术架构

```
┌─────────────┐         ┌──────────────────────────────────┐
│  Android    │  REST   │  Express (HTTP)                  │
│  Client     │ ──────→ │  /api/auth  /api/users           │
│  Compose    │         │  /api/friends /api/chats         │
│  Material3  │  WS     │  /api/moments /api/upload        │
│             │ ←═════→ │  /api/admin /api/push            │
└─────────────┘         │                                  │
                        │  WebSocket Server (ws)           │
                        │  实时消息 / 在线状态 / typing     │
                        │                                  │
                        │  In-Memory DB + JSON 持久化      │
                        │  (Proxy 拦截 + 防抖写入 + 原子写入)│
                        │                                  │
                        │  GeTui UniPush 2.0               │
                        │  离线 → 厂商推送通道              │
                        └──────────────────────────────────┘
```

**通信协议**：REST API 处理元数据（用户/好友/聊天/群设置/表情包/入群申请），WebSocket 处理实时消息（文本/图片/文件/视频/typing/入群通知/撤回/@全体）。

**数据持久化**：内存数据库 + JSON 文件自动持久化。使用 ES6 Proxy 拦截所有数据变更，1 秒防抖延迟写入，原子写入（先 `.tmp` 后 `rename`），进程退出时强制保存。

---

## 快速开始

### 环境要求

- Node.js >= 16
- npm

### 安装

```bash
# 克隆仓库
git clone https://github.com/qiuxi903/HutaomuChat.git
cd HutaomuChat/securechat-server

# 安装依赖
npm install

# 复制环境变量示例
cp .env.example .env

# 启动服务
npm start
```

### 首次部署

启动后浏览器打开 `http://localhost:3000/admin/setup`，进入 OOBE 向导：

1. **管理员账户** — 设置后台登录的用户名和密码
2. **服务器配置** — 服务器名称、端口、JWT 密钥（可自动生成）
3. **推送配置** — 个推 AppId / AppKey / MasterSecret（可选，稍后配置）
4. **完成** — 自动跳转到管理后台

### 推送配置（可选）

如需系统级推送通知（应用被杀死后仍可收到消息）：

1. 在 [个推开放平台](https://dev.getui.com) 注册应用
2. 创建 `getui.config.json`：
   ```json
   {
     "appId": "你的AppID",
     "appKey": "你的AppKey",
     "masterSecret": "你的MasterSecret"
   }
   ```
3. 重启服务端

---

## 项目结构

```
securechat-server/
├── server.js              # 入口，Express + WebSocket + OOBE 中间件
├── db.js                  # 内存数据库 + JSON 持久化层
├── getui.js               # 个推 REST API v2 推送模块
├── package.json
├── .env.example           # 环境变量示例
├── .gitignore
│
├── middleware/
│   └── auth.js            # JWT 认证中间件 + 封禁检查
│
├── routes/
│   ├── auth.js            # 注册/登录/个人资料
│   ├── users.js           # 用户搜索/详情/推荐
│   ├── friends.js         # 好友请求/列表
│   ├── chats.js           # 聊天/群组管理（公告/头像/禁言/置顶/管理员/入群申请）
│   ├── messages.js        # 消息历史
│   ├── moments.js         # 朋友圈/用户圈动态
│   ├── upload.js          # 文件上传（multer，10MB 限制）
│   ├── stickers.js        # 表情包 CRUD
│   ├── push.js            # 推送 token 注册/移除
│   ├── admin.js           # 管理后台 API
│   └── setup.js           # OOBE 初始化配置
│
├── ws/
│   └── index.js           # WebSocket 管理器（消息转发/在线状态/撤回/离线队列/推送）
│
└── admin/
    ├── index.html         # 管理后台 SPA（亮暗主题/中英文/自动刷新）
    └── setup.html         # OOBE 首次部署向导
```

---

## API 概览

### 认证

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/auth/register` | 注册（手机号+密码+昵称） |
| POST | `/api/auth/login` | 登录，返回 JWT |
| GET | `/api/auth/profile` | 获取个人资料 |
| PUT | `/api/auth/profile` | 更新个人资料 |

### 用户

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/users/search` | 搜索用户 |
| GET | `/api/users/recommended` | 推荐好友 |
| GET | `/api/users/:id` | 用户详情 |

### 好友

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/friends` | 好友列表 |
| POST | `/api/friends/request` | 发送好友请求 |
| PUT | `/api/friends/request/:id` | 接受/拒绝请求 |

### 聊天

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/chats` | 聊天列表 |
| POST | `/api/chats/single` | 创建单聊 |
| POST | `/api/chats/group` | 创建群聊 |
| GET | `/api/chats/search` | 按群聊号搜索 |
| POST | `/api/chats/:id/leave` | 退出群聊 |
| POST | `/api/chats/:id/join` | 申请加入群聊 |
| PUT | `/api/chats/:id/announcement` | 发布公告 |
| PUT | `/api/chats/:id/avatar` | 更新群头像 |
| POST | `/api/chats/:id/mute` | 消息免打扰 |
| POST | `/api/chats/:id/pin` | 置顶聊天 |

### 朋友圈

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/moments` | 用户圈动态（scope=public） |
| GET | `/api/moments/friends` | 朋友圈动态（好友） |
| POST | `/api/moments` | 发布动态 |
| POST | `/api/moments/:id/like` | 点赞 |
| POST | `/api/moments/:id/comment` | 评论 |

### 管理后台 (`/api/admin`)

需要管理员 JWT 认证，访问 `/admin` 进入 Web 管理界面。

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/admin/login` | 管理员登录 |
| GET | `/api/admin/stats` | 仪表盘统计 |
| GET/PUT/DELETE | `/api/admin/users` | 用户管理 |
| GET/DELETE | `/api/admin/chats` | 聊天管理 |
| GET/DELETE | `/api/admin/moments` | 朋友圈管理 |
| GET | `/api/admin/system` | 系统信息 |

---

## WebSocket 消息类型

客户端连接 `ws://host:port`，登录后通过 WebSocket 收发实时消息。

| 类型 | 方向 | 说明 |
|------|------|------|
| `auth` | C→S | 登录认证 |
| `CHAT_MESSAGE` | 双向 | 聊天消息（TEXT/IMAGE/FILE/VIDEO/CARD） |
| `RECALL` | 双向 | 撤回消息 |
| `typing` | C→S | 正在输入 |
| `ONLINE_STATUS` | S→C | 在线状态变更 |
| `pending_messages` | S→C | 离线消息投递 |
| `pending_recalls` | S→C | 离线撤回投递 |
| `at_all_notification` | S→C | @全体通知（绕过免打扰） |
| `member_joined/left` | S→C | 群成员加入/退出 |
| `force_disconnect` | S→C | 强制下线（封禁） |

---

## 客户端

Android 客户端使用 Jetpack Compose + Material3 开发，源码暂不开源。预编译 APK 见 [Releases](../../releases) 页面。

客户端特性：
- 6 种预设主题色 + 亮暗色切换
- 聊天背景图自定义
- 好友资料页背景图下拉放大
- 通知独立控制（声音/震动/免打扰）
- 后台保活（前台服务 + AlarmManager + WorkManager + 个推）

---

## 部署建议

- 推荐使用 Docker 或 PM2 部署
- 配置 Nginx 反向代理，启用 HTTPS 和 WSS
- 数据库目前为 JSON 文件持久化，适合中小规模使用
- 大规模部署建议替换为 MongoDB/PostgreSQL

```nginx
# Nginx 配置示例
server {
    listen 443 ssl;
    server_name chat.example.com;

    ssl_certificate     /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

---

## 许可证

[AGPL-3.0](LICENSE)

Copyright (C) 2026 qiuxi903

本程序是自由软件：您可以根据 GNU Affero 通用公共许可证第3版的条款重新分发和/或修改它。

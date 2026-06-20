# OrbitTerm-Server

OrbitTerm 后端基础框架（Go + Gin + Gorm + PostgreSQL + Argon2id + JWT）。

## 部署指南

- 1Panel 容器化部署（含详细步骤、变量示例、升级回滚）：
  - [docs/DEPLOY_1PANEL.md](docs/DEPLOY_1PANEL.md)
- 管理端 API 与首次初始化指南：
  - [docs/ADMIN_API.md](docs/ADMIN_API.md)
- 管理端小白操作指南：
  - [docs/ADMIN_CONSOLE_GUIDE.md](docs/ADMIN_CONSOLE_GUIDE.md)

## 管理端控制台

后端内置了一个轻量管理端 Web 控制台，部署后可访问：

- `https://你的后端域名/admin-console/`

控制台用于完成首次初始化、管理员登录、仪表盘查看、用户治理、策略配置与审计查看。它复用 `/api/v1/admin/...` 接口和 JWT 权限体系，不会额外保存密钥。

## 一、项目目录结构（符合 Go 后端常见最佳实践）

```text
OrbitTerm-Server
├── api/
│   └── v1/
│       └── auth/
│           └── README.md
├── internal/
│   ├── common/        # 统一响应结构
│   ├── config/        # 配置加载与数据库初始化
│   ├── controller/    # HTTP 控制器
│   ├── model/         # 数据库模型
│   ├── repository/    # 数据访问层
│   ├── router/        # 路由注册
│   ├── service/       # 业务逻辑层
│   └── utils/         # 安全工具（Argon2id/JWT）
├── go.mod
├── go.sum
└── main.go
```

## 二、核心模型设计

- `User`
  - `ID`：主键
  - `Username`：唯一用户名
  - `PasswordHash`：Argon2id 哈希后的密码（不存明文）
  - `CreatedAt`：注册时间

- `ServerConfig`
  - `ID`：主键
  - `UserID`：关联用户 ID
  - `AssetID`：客户端生成的跨端稳定资产 UUID
  - `EncryptedBlob`：加密后的配置数据块（`bytea`）
  - `VectorClock`：版本号（使用字符串存储 JSON 结构，便于多端冲突处理）
  - `State`：`active / deleted / purged`，用于最近删除与防复活墓碑
  - `UpdatedAt`：最后更新时间

- `SystemSetting`
  - `Key`：系统配置键，例如 `security_policy`
  - `Value`：JSON 文本配置，用于保存注册开关、密码强度、恢复边界等管理端策略

## 三、账号密码与主密码恢复边界

- 登录密码：属于账号认证体系，管理员可按权限重置，并会强制旧 Token 失效。
- 主密码：属于客户端零知识加密体系，后端不保存主密码、派生密钥，也无法解密用户资产。
- 忘记主密码：管理员无法找回原主密码；用户只能在客户端执行主密码重设流程。若没有旧主密码或本地可用解密材料，旧加密资产无法被服务端恢复。
- 修改主密码：客户端必须先用旧主密码解密资产，再用新主密码重新加密后同步到云端。

## 四、环境变量

可选环境变量（均有默认值）：

- `SERVER_PORT`（默认 `8080`）
- `DATABASE_URL`（默认本地 PostgreSQL 连接）
- `JWT_SECRET`（默认演示值，生产必须替换）
- `JWT_ISSUER`（默认 `orbitterm-server`）
- `JWT_ACCESS_EXPIRE_MINUTES`（默认 `15`）
- `JWT_REFRESH_EXPIRE_DAYS`（默认 `30`）
- `JWT_EXPIRE_HOURS`（兼容旧配置，未设置新变量时回退）
- `ADMIN_BOOTSTRAP_TOKEN`（无默认值；用于首次创建 `super_admin`，生产必须配置高强度随机值）
- `DB_LOG_LEVEL`（默认 `warn`；日志参数会以占位符脱敏，生产环境仍不应长期设置为 `info`）
- `TRUSTED_PROXIES`（默认 `127.0.0.1,::1`；使用 1Panel 反向代理时填写其容器 IP/CIDR，不使用代理可设为空）

## 五、接口

- `GET /healthz`
- `POST /api/v1/auth/register`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/refresh`
- `GET /api/v1/auth/recovery-info`
- `POST /api/v1/config/upload`
- `GET /api/v1/config/pull`
- `GET /api/v1/config/trash`
- `POST /api/v1/config/assets/:asset_id/delete`
- `POST /api/v1/config/assets/:asset_id/restore`
- `POST /api/v1/config/assets/:asset_id/purge`
- `GET /api/v1/config/sync/pull?cursor=&limit=`
- `POST /api/v1/config/sync/ack`
- `GET /api/v1/config/identity-match?fingerprint=`
- `GET /api/v1/admin/dashboard/overview`
- `GET /api/v1/admin/system/runtime`
- `GET /api/v1/admin/system/audit-policy`
- `PUT /api/v1/admin/system/audit-policy`
- `GET /api/v1/admin/system/security-policy`
- `PUT /api/v1/admin/system/security-policy`
- `GET /api/v1/admin/system/recovery-policy`
- `PUT /api/v1/admin/system/recovery-policy`
- `GET /api/v1/admin/system/backup-readiness`
- `GET /api/v1/admin/system/diagnostics`
- `GET /api/v1/admin/audit-logs?action=&admin_user_id=&target_user_id=&limit=&offset=`
- `POST /api/v1/admin/audit-logs/cleanup`
- `POST /api/v1/admin/users/managed`
- `POST /api/v1/admin/users/:id/role`
- `POST /api/v1/admin/users/expired-bans/scan`
- `POST /api/v1/admin/users/force-logout-regular`

后端还内置到期封禁自动解封任务，可通过 `ADMIN_AUTO_UNBAN_ENABLED`、`ADMIN_AUTO_UNBAN_INTERVAL_MINUTES` 与 `ADMIN_AUTO_UNBAN_BATCH_LIMIT` 控制。

后端同时内置最近删除自动清理任务：`ASSET_TRASH_CLEANUP_INTERVAL_MINUTES` 控制扫描间隔，保留周期、批量上限与自动清理开关由管理端动态配置。默认可恢复密文保留 90 天，最小防复活墓碑永久保留。

管理端高危操作要求：

- 封禁、重置登录密码、强制下线、软删除、过期封禁扫描必须传入 `reason`。
- 上述高危操作还必须传入 `confirmation: "CONFIRM"`，用于后端二次确认。

### 注册示例

```bash
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"username":"alice","password":"StrongPass123"}'
```

### 登录示例

```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"alice","password":"StrongPass123"}'
```

## 六、运行

```bash
go run .
```

启动时会自动执行用户、密文配置、同步修订、设备确认水位、审计日志与系统策略的兼容迁移。

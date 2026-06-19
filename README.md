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
  - `EncryptedBlob`：加密后的配置数据块（`bytea`）
  - `VectorClock`：版本号（使用字符串存储 JSON 结构，便于多端冲突处理）
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

## 五、接口

- `POST /api/v1/auth/register`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/refresh`
- `GET /api/v1/auth/recovery-info`
- `GET /api/v1/admin/dashboard/overview`
- `GET /api/v1/admin/system/security-policy`
- `PUT /api/v1/admin/system/security-policy`
- `GET /api/v1/admin/system/recovery-policy`
- `PUT /api/v1/admin/system/recovery-policy`
- `GET /api/v1/admin/system/backup-readiness`
- `GET /api/v1/admin/system/diagnostics`
- `GET /api/v1/admin/audit-logs?action=&admin_user_id=&target_user_id=&limit=&offset=`
- `POST /api/v1/admin/users/managed`
- `POST /api/v1/admin/users/:id/role`
- `POST /api/v1/admin/users/expired-bans/scan`

后端还内置到期封禁自动解封任务，可通过 `ADMIN_AUTO_UNBAN_ENABLED`、`ADMIN_AUTO_UNBAN_INTERVAL_MINUTES` 与 `ADMIN_AUTO_UNBAN_BATCH_LIMIT` 控制。

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

启动时会自动执行 `User`、`ServerConfig`、`AdminAuditLog` 与 `SystemSetting` 的表迁移。

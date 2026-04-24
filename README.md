# OrbitTerm-Server

OrbitTerm 后端基础框架（Go + Gin + Gorm + PostgreSQL + Argon2id + JWT）。

## 部署指南

- 1Panel 容器化部署（含详细步骤、变量示例、升级回滚）：
  - [docs/DEPLOY_1PANEL.md](docs/DEPLOY_1PANEL.md)

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

## 三、环境变量

可选环境变量（均有默认值）：

- `SERVER_PORT`（默认 `8080`）
- `DATABASE_URL`（默认本地 PostgreSQL 连接）
- `JWT_SECRET`（默认演示值，生产必须替换）
- `JWT_ISSUER`（默认 `orbitterm-server`）
- `JWT_EXPIRE_HOURS`（默认 `24`）

## 四、接口

- `POST /api/v1/auth/register`
- `POST /api/v1/auth/login`

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

## 五、运行

```bash
go run .
```

启动时会自动执行 `User` 与 `ServerConfig` 的表迁移。

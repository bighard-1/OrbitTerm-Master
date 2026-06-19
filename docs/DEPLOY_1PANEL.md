# OrbitTerm-Server 1Panel 部署指南

本文档用于将 OrbitTerm 后端部署到 1Panel 的容器环境中，包含：

- 前置准备
- GHCR 私有仓库认证
- 1Panel 编排（Compose）部署步骤
- 环境变量示例与说明
- 反向代理/HTTPS
- 升级与回滚
- 自检清单

## 1. 前置准备

1. 已安装并可访问 1Panel。
2. 服务器已安装 Docker（1Panel 通常会自动管理）。
3. 后端镜像已推送到 GHCR：
   - `ghcr.io/bighard-1/orbitterm-server:latest`
4. 放通端口：
   - `80/443`（生产推荐）
   - `8080`（仅调试时可临时放通）

生成 JWT 强密钥示例（建议至少 32 字符）：

```bash
openssl rand -base64 48
```

## 2. 在 1Panel 添加 GHCR 仓库认证

如果镜像是私有仓库，必须先配置仓库认证。

1. 进入 `容器 -> 仓库`。
2. 点击“添加仓库”。
3. 填写：
   - 仓库地址：`ghcr.io`
   - 用户名：`bighard-1`
   - 密码：GitHub PAT（建议 classic token，包含 `read:packages`，私有仓库常见还需 `repo`）
4. 保存后执行拉取测试。

## 3. 使用 1Panel 编排部署（推荐）

路径：`容器 -> 编排 -> 创建编排 -> 编辑`

- 编排名称建议：`orbitterm-prod`
- 将下方 Compose 粘贴到编辑器并创建启动

```yaml
services:
  orbit-db:
    image: postgres:16-alpine
    container_name: orbit-db
    restart: unless-stopped
    environment:
      POSTGRES_DB: orbitterm
      POSTGRES_USER: orbitterm
      POSTGRES_PASSWORD: "ReplaceWithStrongDBPassword"
      TZ: UTC
    volumes:
      - orbit_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U orbitterm -d orbitterm"]
      interval: 5s
      timeout: 3s
      retries: 20

  orbit-api:
    image: ghcr.io/bighard-1/orbitterm-server:latest
    container_name: orbit-api
    restart: unless-stopped
    depends_on:
      orbit-db:
        condition: service_healthy
    environment:
      SERVER_PORT: "8080"
      DATABASE_URL: "host=orbit-db user=orbitterm password=ReplaceWithStrongDBPassword dbname=orbitterm port=5432 sslmode=disable TimeZone=UTC"
      JWT_SECRET: "ReplaceWithLongRandomJwtSecret"
      JWT_ISSUER: "orbitterm-server"
      JWT_ACCESS_EXPIRE_MINUTES: "15"
      JWT_REFRESH_EXPIRE_DAYS: "30"
      ADMIN_BOOTSTRAP_TOKEN: "ReplaceWithLongRandomAdminBootstrapToken"
      ADMIN_AUTO_UNBAN_ENABLED: "true"
      ADMIN_AUTO_UNBAN_INTERVAL_MINUTES: "10"
      ADMIN_AUTO_UNBAN_BATCH_LIMIT: "100"
    ports:
      - "127.0.0.1:8080:8080"

volumes:
  orbit_db_data:
```

启动后检查：

1. `orbit-db` 状态为 `healthy`。
2. `orbit-api` 日志出现“启动成功，监听端口: 8080”。
3. 使用管理端首次初始化接口创建 `super_admin` 后，建议轮换或清空 `ADMIN_BOOTSTRAP_TOKEN` 并重启后端。
4. 管理端初始化、登录与用户治理示例见：[ADMIN_API.md](ADMIN_API.md)。
5. 若已配置域名反向代理，可访问 `https://你的后端域名/admin-console/` 打开内置管理控制台。

## 4. 环境变量说明（与代码保持一致）

以下变量由后端直接读取（见 `internal/config/config.go`）：

1. `SERVER_PORT`
   - 示例：`8080`
   - 说明：Gin 服务监听端口。

2. `DATABASE_URL`
   - 示例：
     - `host=orbit-db user=orbitterm password=ReplaceWithStrongDBPassword dbname=orbitterm port=5432 sslmode=disable TimeZone=UTC`
   - 说明：PostgreSQL DSN。
   - 注意：编排内必须使用服务名 `orbit-db` 作为主机名。

3. `JWT_SECRET`
   - 示例：`ReplaceWithLongRandomJwtSecret`
   - 说明：JWT 签名密钥，必须高强度随机值。

4. `JWT_ISSUER`
   - 示例：`orbitterm-server`
   - 说明：JWT 签发者标识。

5. `JWT_ACCESS_EXPIRE_MINUTES`
   - 示例：`15`
   - 说明：Access Token 过期时间（分钟）。

6. `JWT_REFRESH_EXPIRE_DAYS`
   - 示例：`30`
   - 说明：Refresh Token 过期时间（天）。

7. `ADMIN_BOOTSTRAP_TOKEN`
   - 示例：`ReplaceWithLongRandomAdminBootstrapToken`
   - 说明：首次创建管理端 `super_admin` 时必须在请求头 `X-Orbit-Admin-Bootstrap-Token` 中携带该值。
   - 注意：这不是管理员登录密码，只是一次性初始化防护令牌；创建首个管理员后仍建议轮换或清空该环境变量并重启后端。

8. `ADMIN_AUTO_UNBAN_ENABLED`
   - 示例：`true`
   - 说明：是否启用限时封禁到期后的后台自动解封任务。

9. `ADMIN_AUTO_UNBAN_INTERVAL_MINUTES`
   - 示例：`10`
   - 说明：自动解封扫描间隔，建议不低于 1 分钟。

10. `ADMIN_AUTO_UNBAN_BATCH_LIMIT`
   - 示例：`100`
   - 说明：每次自动扫描最多处理的到期封禁用户数，建议不超过 500。

数据库容器变量（`orbit-db`）：

1. `POSTGRES_DB`
2. `POSTGRES_USER`
3. `POSTGRES_PASSWORD`

注意：`POSTGRES_PASSWORD` 必须与 `DATABASE_URL` 中 password 保持一致。

## 5. 账号恢复与主密码边界

OrbitTerm 后端采用零知识同步模型：

1. 管理员可以重置用户“登录密码”，但不能查看、找回或替用户重置“主密码”。
2. 云端保存的是客户端加密后的资产密文，后端只负责存储与同步，不具备解密能力。
3. 用户修改主密码时，必须由客户端先用旧主密码解密资产，再用新主密码重新加密并上传。
4. 如果用户忘记主密码且没有任何本地可用解密材料，服务端无法恢复旧资产，只能重新建立资产配置。

## 6. 生产部署建议（1Panel）

1. 不建议公网直接暴露 `8080`。
2. 建议使用 1Panel 网站反向代理到：
   - `http://127.0.0.1:8080`
3. 在 1Panel 中为域名申请 HTTPS 证书（如 Let's Encrypt）。
4. 定期执行备份，建议至少包含：
   - PostgreSQL 数据库快照（优先使用 1Panel 数据库备份或容器内 `pg_dump`）。
   - 脱敏环境变量快照：记录 `JWT_SECRET`、`DATABASE_URL`、`JWT_ISSUER` 等变量是否配置，但不要把密钥原文写入普通文档。
   - 后端镜像版本号/tag/digest。
   - 1Panel 反向代理、域名、HTTPS 证书配置。
5. 管理端可调用 `GET /api/v1/admin/system/backup-readiness` 查看备份就绪状态和环境变量脱敏检查结果，也可调用 `GET /api/v1/admin/system/diagnostics` 导出脱敏诊断包。

## 7. 升级与回滚

建议使用固定版本标签，不长期依赖 `latest`。

升级步骤：

1. 推送新镜像（例如）：`ghcr.io/bighard-1/orbitterm-server:v0.1.1`
2. 进入 1Panel 编排，将 `orbit-api.image` 改为新 tag。
3. 重建并启动 `orbit-api`。
4. 验证登录与配置同步接口。

回滚步骤：

1. 将 `orbit-api.image` 改回旧 tag。
2. 重建 `orbit-api`。
3. 再次验证核心接口。

## 8. 自检清单

1. `orbit-db` healthy。
2. `orbit-api` 无数据库连接错误。
3. `POST /api/v1/auth/register` 正常。
4. `POST /api/v1/auth/login` 可返回 JWT。
5. 带 Bearer Token 调用 `/api/v1/config/upload` 正常。
6. 管理端调用 `/api/v1/admin/system/backup-readiness` 可返回脱敏环境检查结果；排障时可调用 `/api/v1/admin/system/diagnostics` 导出脱敏诊断包。
7. 管理端高危操作必须填写原因，并传入 `confirmation=CONFIRM`，否则后端会拒绝执行。

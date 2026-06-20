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

## 七、生产备份、恢复与冗余容灾（1Panel 小白版）

本章适用于使用 1Panel“容器”功能部署 OrbitTerm 的管理员。建议首次上线前完整阅读，并至少完成一次测试恢复。

### 7.1 先理解三个容易混淆的概念

| 能力 | 解决的问题 | 当前项目状态 |
| --- | --- | --- |
| 数据备份 | 误删除、数据库损坏、升级失败后恢复数据 | 已支持备份自检，但需要管理员配置实际备份任务 |
| 冗余部署 | 一台 API 服务器断网时，由另一台继续提供服务 | 后端为无状态 JWT 架构，可扩展部署；不是开箱即用的自动集群 |
| 高可用数据库 | PostgreSQL 所在服务器损坏后自动切换 | 需要托管高可用 PostgreSQL 或额外搭建主从集群 |

重要结论：

1. “每天有备份”不等于“服务器故障后不停机”。备份只能在故障后恢复。
2. “运行两个 API 容器”也不等于完整高可用。如果两者仍依赖同一台 PostgreSQL，数据库服务器故障后服务仍会中断。
3. OrbitTerm 用户、管理员、加密资产、同步版本、最近删除墓碑、系统策略和审计日志均保存在 PostgreSQL。
4. `orbit-api` 本身不保存用户资产明文；重新创建 API 容器不会丢失资产，但必须恢复正确的数据库和环境变量。

### 7.2 推荐目标：3-2-1 备份原则

生产环境至少保留：

1. 三份数据：生产数据库一份、服务器本地备份一份、异地备份一份。
2. 两种介质：例如服务器磁盘和对象存储/另一台服务器。
3. 一份异地：不能只存放在运行 OrbitTerm 的同一台物理服务器上。

建议保留周期：

| 类型 | 建议频率 | 建议保留 |
| --- | --- | --- |
| PostgreSQL 全量备份 | 每天一次 | 最近 7 天 |
| 每周备份 | 每周一次 | 最近 4 周 |
| 每月备份 | 每月一次 | 最近 12 个月 |
| 升级前备份 | 每次升级前 | 至少保留到新版本稳定运行 7 天 |
| 环境与密钥加密备份 | 每次变量变化后 | 保留当前版本和上一版本 |

可以根据业务量缩短周期。资产变化频繁时，建议使用托管 PostgreSQL 的连续归档/PITR（按时间点恢复），将可能丢失的数据窗口缩短到分钟级。

### 7.3 必须备份的内容

#### A. PostgreSQL 数据库（必须）

数据库备份包含：

- 用户和管理员账号。
- Argon2id 登录密码哈希。
- 客户端上传的加密资产 Blob。
- 同步修订、游标、设备确认水位和向量钟。
- 最近删除记录、墓碑和恢复状态。
- 系统安全、审计、恢复与清理策略。
- 管理员审计日志。

后端不掌握用户主密码，因此数据库备份中仍然只有密文。

#### B. 生产环境变量和密钥（必须加密保存）

至少包括：

- `JWT_SECRET`
- `JWT_ISSUER`
- `DATABASE_URL` 或数据库账号、密码、地址和库名
- `ADMIN_BOOTSTRAP_TOKEN`
- JWT 有效期、可信代理和后台任务配置
- 实际部署的镜像标签和 digest

这些内容不能提交到 GitHub、普通网盘、工单、聊天记录或明文 README。推荐保存在密码管理器或加密文件中，并限制为两名以内的授权管理员可访问。

恢复时应使用原来的 `JWT_SECRET` 和 `JWT_ISSUER`。如果丢失或主动更换，数据库数据不会损坏，但所有现有 Access Token 和 Refresh Token 都会失效，用户必须重新登录。

#### C. 1Panel 与入口配置

至少记录：

- `server.orbitterm.com` 的 DNS 记录。
- 1Panel 网站反向代理目标。
- HTTPS 证书来源和自动续期状态。
- API 容器端口、网络和重启策略。
- PostgreSQL 容器名、数据卷名和数据库版本。
- 防火墙、安全组和负载均衡健康检查规则。

证书通常可以重新签发，但域名、反向代理和防火墙配置仍应留档。

#### D. 镜像版本

生产环境不要只记录 `latest`，应同时记录固定版本或镜像 digest，例如：

```text
ghcr.io/bighard-1/orbitterm-server:具体版本
ghcr.io/bighard-1/orbitterm-server@sha256:实际摘要
```

镜像可以重新拉取，不需要备份 `orbit-api` 容器文件系统。

### 7.4 第一次配置前的准备

1. 在 1Panel 打开“容器”，确认 API 和数据库容器的实际名称。
2. 以下示例默认名称是：
   - API：`orbit-api`
   - 数据库：`orbit-db`
   - 数据库名：`orbitterm`
   - 数据库用户：`orbitterm`
3. 如果你的名称不同，必须替换示例中的对应值。
4. 通过 SSH 登录服务器后先执行：

```bash
docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}'
```

5. 创建只允许 root 访问的备份目录：

```bash
sudo install -d -m 700 /opt/orbitterm/backups/postgres
sudo install -d -m 700 /opt/orbitterm/backups/config
sudo install -d -m 700 /opt/orbitterm/scripts
```

### 7.5 手动执行一次 PostgreSQL 全量备份

执行：

```bash
sudo -i
BACKUP_DIR=/opt/orbitterm/backups/postgres
STAMP=$(date -u +%Y%m%dT%H%M%SZ)
FILE="orbitterm_${STAMP}.dump"

docker exec orbit-db pg_dump \
  -U orbitterm \
  -d orbitterm \
  -Fc > "${BACKUP_DIR}/${FILE}"

test -s "${BACKUP_DIR}/${FILE}"
sha256sum "${BACKUP_DIR}/${FILE}" > "${BACKUP_DIR}/${FILE}.sha256"
chmod 600 "${BACKUP_DIR}/${FILE}" "${BACKUP_DIR}/${FILE}.sha256"
ls -lh "${BACKUP_DIR}/${FILE}" "${BACKUP_DIR}/${FILE}.sha256"
```

说明：

- `-Fc` 使用 PostgreSQL 自定义归档格式，恢复时更灵活。
- `test -s` 用于确认文件不是空文件。
- `.sha256` 用于发现上传、下载或磁盘损坏导致的文件变化。
- 命令成功不等于备份一定能恢复，必须继续完成下一节的校验和恢复演练。

如果出现 `No such container: orbit-db`，说明容器名不一致，请回到 1Panel 容器列表查询真实名称。

### 7.6 校验备份是否可读取

先验证校验和：

```bash
cd /opt/orbitterm/backups/postgres
sha256sum -c orbitterm_你的时间.dump.sha256
```

再检查归档目录：

```bash
docker run --rm \
  -v /opt/orbitterm/backups/postgres:/backup:ro \
  postgres:16-alpine \
  pg_restore --list /backup/orbitterm_你的时间.dump >/dev/null

echo $?
```

输出 `0` 表示归档结构可以读取。非 `0` 表示备份无效，不能删除旧的可用备份。

### 7.7 配置 1Panel 自动备份任务

创建脚本 `/opt/orbitterm/scripts/backup_postgres.sh`：

```bash
sudo tee /opt/orbitterm/scripts/backup_postgres.sh >/dev/null <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail

CONTAINER="orbit-db"
DB_USER="orbitterm"
DB_NAME="orbitterm"
BACKUP_DIR="/opt/orbitterm/backups/postgres"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
FILE="orbitterm_${STAMP}.dump"
TMP="${BACKUP_DIR}/.${FILE}.tmp"
FINAL="${BACKUP_DIR}/${FILE}"

install -d -m 700 "${BACKUP_DIR}"
trap 'rm -f "${TMP}"' EXIT

docker exec "${CONTAINER}" pg_dump \
  -U "${DB_USER}" \
  -d "${DB_NAME}" \
  -Fc > "${TMP}"

test -s "${TMP}"
mv "${TMP}" "${FINAL}"
sha256sum "${FINAL}" > "${FINAL}.sha256"
chmod 600 "${FINAL}" "${FINAL}.sha256"

# 只清理 14 天前的“每日副本”。长期周备份/月备份应由异地存储单独保留。
find "${BACKUP_DIR}" -type f \
  \( -name 'orbitterm_*.dump' -o -name 'orbitterm_*.dump.sha256' \) \
  -mtime +14 -delete

echo "OrbitTerm PostgreSQL backup completed: ${FINAL}"
EOF

sudo chmod 700 /opt/orbitterm/scripts/backup_postgres.sh
sudo /opt/orbitterm/scripts/backup_postgres.sh
```

然后在 1Panel 中：

1. 打开“计划任务”。
2. 新建“Shell 脚本”任务。
3. 名称填写 `OrbitTerm PostgreSQL 每日备份`。
4. 执行周期建议设为每天业务低峰期一次。
5. 脚本填写：

```bash
/opt/orbitterm/scripts/backup_postgres.sh
```

6. 手动执行一次，查看任务日志并确认出现 `backup completed`。
7. 到 `/opt/orbitterm/backups/postgres` 检查新生成的 `.dump` 和 `.sha256` 文件。
8. 为任务失败配置通知；没有失败通知的自动备份不够可靠。

### 7.8 将备份复制到异地

本机备份无法应对硬盘损坏、机房断电、账号封禁或服务器被入侵。至少选择一种异地位置：

- S3 兼容对象存储。
- 另一云厂商的对象存储。
- 不同物理服务器上的加密备份目录。
- 企业 NAS，并确保 NAS 不与生产服务器共用同一块磁盘。

要求：

1. 开启服务端加密或客户端加密。
2. 备份存储账号只授予必要的上传、读取权限。
3. 最好开启版本控制或不可变保留，避免攻击者同时删除生产数据和备份。
4. 上传后重新下载一个样本并执行 `sha256sum -c`。
5. 不要把异地存储的永久管理密钥写入备份脚本；使用 1Panel 安全变量或权限受限的凭据文件。

### 7.9 环境变量密钥备份

最推荐的方式是把生产密钥保存到可信密码管理器，并记录变量名、更新时间和用途。

如果必须保存为文件：

1. 创建明文文件时执行 `umask 077`。
2. 文件权限必须是 `600`。
3. 立即使用 `age`、GPG 或企业密钥系统加密。
4. 加密完成并验证后，安全删除临时明文。
5. 解密密码不能与加密文件存放在同一台服务器。

禁止直接备份整个 `docker inspect` 输出到普通目录，因为其中可能包含数据库密码、JWT 密钥和管理初始化令牌。

环境变量变化后应立即重新备份，尤其是：

- 更换 `JWT_SECRET`。
- 更换数据库密码或地址。
- 更换 `ADMIN_BOOTSTRAP_TOKEN`。
- 调整域名、可信代理和负载均衡网络。

### 7.10 无破坏恢复演练（每月至少一次）

恢复演练不要直接覆盖生产数据库。可以在现有 PostgreSQL 容器中创建一个临时数据库：

```bash
BACKUP=/opt/orbitterm/backups/postgres/orbitterm_你的时间.dump

docker exec orbit-db dropdb \
  -U orbitterm \
  --if-exists \
  --force orbitterm_restore_test

docker exec orbit-db createdb \
  -U orbitterm \
  -O orbitterm \
  orbitterm_restore_test

docker exec -i orbit-db pg_restore \
  -U orbitterm \
  -d orbitterm_restore_test \
  --no-owner \
  --no-privileges < "${BACKUP}"
```

验证主要表是否存在：

```bash
docker exec orbit-db psql \
  -U orbitterm \
  -d orbitterm_restore_test \
  -c '\dt'
```

演练完成后删除测试库：

```bash
docker exec orbit-db dropdb \
  -U orbitterm \
  --if-exists \
  --force orbitterm_restore_test
```

恢复演练记录应包含：备份文件名、校验结果、恢复开始/完成时间、执行人和异常情况，但不能记录密钥原文。

### 7.11 生产数据库完整恢复步骤

只在数据库损坏、误操作且确认需要回滚时执行。恢复会让数据库回到备份时间点，备份之后产生的数据可能丢失。

1. 在 1Panel 停止 `orbit-api`，防止恢复过程中继续写入。
2. 对当前数据库再做一次“事故现场备份”，即使当前数据可能有问题也不要直接丢弃。
3. 校验目标备份的 SHA-256 和 `pg_restore --list`。
4. 确认数据库容器运行正常。
5. 执行：

```bash
BACKUP=/opt/orbitterm/backups/postgres/orbitterm_你的时间.dump

docker exec orbit-db dropdb \
  -U orbitterm \
  --if-exists \
  --force orbitterm

docker exec orbit-db createdb \
  -U orbitterm \
  -O orbitterm \
  orbitterm

docker exec -i orbit-db pg_restore \
  -U orbitterm \
  -d orbitterm \
  --no-owner \
  --no-privileges < "${BACKUP}"
```

6. 在 1Panel 启动 `orbit-api`。后端会执行兼容迁移。
7. 验证：

```bash
curl -fsS https://server.orbitterm.com/healthz
```

8. 登录管理端并执行“备份自检”。
9. 使用测试账号验证登录、拉取资产、上传修改和删除同步。
10. 如果服务端数据从旧备份恢复，客户端同步接口可能返回 `reset_required=true`，客户端应清空旧游标并重新对账，这是防止错误静默合并的安全机制。

不要在没有事故现场备份的情况下反复执行恢复命令。

### 7.12 使用管理端检查备份准备状态

管理员可以访问：

```text
https://你的后端域名/admin-console/
```

进入“备份自检”，检查：

- PostgreSQL 是否可连接。
- 关键数据表是否存在。
- JWT、数据库和管理初始化变量是否仍是默认值或存在风险。
- 建议备份项是否完整。

也可以调用：

```text
GET /api/v1/admin/system/backup-readiness
GET /api/v1/admin/system/diagnostics
```

诊断包会脱敏，不包含 `JWT_SECRET`、Refresh Token、数据库密码和 `ADMIN_BOOTSTRAP_TOKEN` 原文。诊断包不是数据备份，不能代替 `pg_dump`。

### 7.13 单机故障后的冷恢复流程

适用于只有一台生产服务器但已经有异地备份的情况：

1. 准备一台新的 Linux + 1Panel 服务器。
2. 安装容器运行环境。
3. 创建 PostgreSQL 16 容器，数据库名、用户和编码与原环境一致。
4. 从异地存储下载最近一个通过校验的 `.dump` 和 `.sha256`。
5. 校验 SHA-256。
6. 按 7.11 节恢复 PostgreSQL。
7. 拉取记录的固定版本 OrbitTerm 镜像。
8. 从安全备份恢复环境变量。
9. 创建并启动 `orbit-api` 容器。
10. 配置 1Panel 反向代理和 HTTPS。
11. 将域名 DNS 切换到新服务器。
12. 验证健康检查、管理员登录和普通客户端同步。

这种方案成本最低，但恢复通常需要 15–60 分钟，实际时间取决于数据库大小、DNS TTL 和管理员熟练程度。

### 7.14 推荐冗余架构

```text
                       server.orbitterm.com
                                │
                    云负载均衡 / 故障健康检查
                         GET /healthz
                         ┌──────┴──────┐
                         │             │
                 API 服务器 A     API 服务器 B
                   orbit-api       orbit-api
                         │             │
                         └──────┬──────┘
                                │
                    高可用 PostgreSQL 集群
                  主节点 + 同步/异步备用节点
                                │
                       WAL / 快照异地备份
```

两台 API 必须使用：

- 完全相同的固定版本镜像。
- 完全相同的 `JWT_SECRET` 和 `JWT_ISSUER`。
- 指向同一套高可用 PostgreSQL 的 `DATABASE_URL`。
- 一致的系统时间和 UTC 时区。
- 一致的可信代理、HTTPS 和安全策略。

如果两个 API 使用不同 JWT 密钥，用户请求落到不同节点时会随机出现 `401 Token 无效或已过期`。

#### 当前版本的多实例注意事项

当前 `orbit-api` 启动时会同时启动到期封禁扫描和最近删除清理后台任务。多个 API 实例会各自启动任务。业务处理已尽量按数据库状态保持幂等，但当前版本尚未提供 PostgreSQL 分布式任务锁，因此：

1. 小白生产环境优先采用“单活 API + 温备用 API”，故障时启动备用节点并切换流量。
2. 如果采用双活 API，应先完成并验证分布式任务锁，或把周期任务拆分为单独的唯一 Worker 实例。
3. `ADMIN_AUTO_UNBAN_ENABLED=false` 可以关闭某个实例的自动解封任务；最近删除清理由数据库系统策略控制，当前不能按 API 实例单独关闭。
4. 限流器目前是进程内状态。双活部署应在云负载均衡、WAF、Nginx 或 API 网关增加统一限流。

因此，当前版本可以水平扩展 API 请求，但不能把“启动两个相同容器”直接视为已经完成企业级高可用。

### 7.15 小白推荐的分阶段方案

#### 阶段一：先把备份做可靠

- 单台 1Panel 服务器运行 API 和 PostgreSQL。
- 每日执行 `pg_dump`。
- 每日上传到异地对象存储。
- 每月执行恢复演练。
- 升级前额外备份。

这是最低成本、最应该先完成的方案。

#### 阶段二：增加温备用服务器

- 第二台服务器预装 1Panel 和容器环境。
- 预先配置 API 镜像、HTTPS 和安全组，但平时不启动写入服务。
- 定期确认可以访问异地备份和密钥管理器。
- 主服务器故障后，恢复最新数据库、启动备用 API 并切换 DNS。

此方案不能做到零停机，但比临时购买服务器重建可靠得多。

#### 阶段三：生产高可用

- 使用两台不同故障域的 API 服务器。
- 使用云厂商托管高可用 PostgreSQL，优先于小白自行维护 Patroni/etcd 主从集群。
- 使用负载均衡健康检查 `/healthz` 自动摘除故障 API。
- 配置 PostgreSQL PITR、跨区域备份和恢复演练。
- 在启用 API 双活前增加分布式 Worker 锁与网关统一限流。

### 7.16 故障切换检查清单

主服务器断网或硬件故障时：

1. 确认故障来自 API、PostgreSQL、网络、DNS 还是证书，不要盲目恢复数据库。
2. 如果只有 API 节点故障，切换到连接同一数据库的备用 API，不需要恢复数据库。
3. 如果数据库节点故障，先由托管数据库/主从集群完成主库切换。
4. 如果只能从备份恢复，记录备份时间，明确可能丢失的数据时间窗口。
5. 确保备用 API 使用同一套 JWT 和数据库配置。
6. 检查 `/healthz`。
7. 检查管理端运行状态、自动任务和备份自检。
8. 使用测试账号完成登录、拉取、上传、删除、恢复操作。
9. 故障结束后生成复盘记录并补做完整备份。

### 7.17 建议恢复目标

| 部署方式 | RPO（最多可能丢失的数据） | RTO（大致恢复时间） |
| --- | --- | --- |
| 每日异地全量备份 | 最多约 24 小时 | 15–60 分钟或更长 |
| 每小时备份 | 最多约 1 小时 | 15–60 分钟 |
| PostgreSQL PITR/WAL 归档 | 通常数分钟 | 10–30 分钟，取决于平台 |
| 双 API + 托管高可用 PostgreSQL | 通常接近 0，取决于复制方式 | 数十秒到数分钟 |

RPO 是允许最多丢失多少时间的数据，RTO 是服务允许中断多久。管理员应根据用户量、成本和业务重要性选择目标。

### 7.18 上线前最终检查

- [ ] 已生成一次非空 `.dump`。
- [ ] 已通过 SHA-256 校验。
- [ ] 已通过 `pg_restore --list`。
- [ ] 已成功恢复到测试数据库。
- [ ] 已配置 1Panel 每日计划任务和失败通知。
- [ ] 已把备份复制到异地位置。
- [ ] 已加密保存环境变量和密钥。
- [ ] 已记录固定镜像版本和 digest。
- [ ] 已记录 DNS、反向代理、证书和端口配置。
- [ ] 已确认备份负责人和故障联系人。
- [ ] 已在管理端通过“备份自检”。
- [ ] 已制定至少每月一次的恢复演练计划。

只有以上检查均完成，才能认为备份方案真正可用。备份文件存在但从未验证恢复，不应视为可靠备份。

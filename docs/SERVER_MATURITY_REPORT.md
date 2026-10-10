# 服务端分阶段整改报告

基线：`codex/canonical-asset-uuid` 的 `04f47db`。本工作区位于独立分支 `codex/server-maturity`；主目录 `main` 上的管理台 logo 与 `.DS_Store` 改动未触碰。只改服务端、服务端自带管理控制台的文档与部署模板，没有改五端客户端、线上数据库或线上部署。

## P0：生产配置

- 发现：默认演示 JWT 密钥、PostgreSQL 密码可启动；Compose 直接写了示例口令并公开数据库端口。
- 修改：`internal/config/config.go`、`main.go`、`docker-compose.yml`、`docs/DEPLOY_1PANEL.md`、`README.md` 与 `internal/config/production_test.go`。
- 取舍：默认运行模式改为 `production`，启动连接数据库前校验 JWT、数据库密码和可选初始化令牌；开发机须显式设 `APP_ENV=development`。Compose 用必填环境变量，数据库无宿主端口映射，API 仅绑定宿主回环地址。
- 验证：配置单测、全量 Go 测试及 `docker compose config --quiet` 通过。
- 剩余风险：静态强度检查无法证明真实随机性；生产密钥仍须由安全凭据系统生成并托管。
- 下一步：在预发布用正式注入方式试启动，并检查反向代理可达性。

## P1：密码与强制改密

- 发现：管理员引导、建号和重置使用比注册更弱的密码门槛；`MustChangePassword` 只是字段；恢复开关没有控制重置。
- 修改：`internal/service/admin_auth_service.go`、`admin_user_service.go`、`internal/middleware/jwt_auth.go`、相关控制器及测试。
- 取舍：统一使用当前注册密码强度；恢复策略关闭时拒绝管理端重置。已标记账号登录后可访问 `/api/v1/auth/password`、`/api/v1/auth/logout` 和 `/api/v1/auth/logout-all`，其他受保护路由返回 403、`PASSWORD_CHANGE_REQUIRED`。高风险管理员用户变更与审计同事务提交，审计失败回滚变更。
- 验证：密码规则单测、路由门禁测试和 PostgreSQL 审计故障回滚测试通过。
- 剩余风险：五端需处理 403 改密路径；旧管理台令牌须重新登录才能执行受保护管理操作。
- 下一步：五端联调首次登录、重置后登录、改密后令牌更新；检查用户文案。

## P1：可撤销会话

- 发现：原刷新 JWT 没有持久状态，所谓轮换可被重放；不能按设备退出。
- 修改：`internal/model/refresh_session.go`、`internal/repository/refresh_session_repository.go`、`internal/utils/jwt.go`、鉴权服务、路由、迁移与测试。
- 取舍：新刷新 JWT 带随机一次性 ID，数据库只存 ID 摘要；同一设备沿用随机 family ID，重放会撤销整个 family。当前设备退出撤销 family，全部设备退出原子增加用户 token version。`auth_time` 在刷新时保留，迁移包与重要管理操作要求 10 分钟内重新验密。`ALLOW_LEGACY_REFRESH_TOKENS=true` 默认过渡，旧 JWT 首次刷新升级为有状态会话。
- 验证：JWT 单测及 PostgreSQL 轮换、重放撤销集成测试通过。
- 剩余风险：过渡期开启时，旧版无会话 ID 的刷新 JWT 无法检测重放；旧 access token 仍可用到原到期时间。旧会话调用当前设备退出返回 `LEGACY_SESSION`，可用全部设备退出。
- 下一步：五端升级后等待旧刷新令牌最长有效期，再在预发布与生产依次关闭 legacy 开关。

## P1：资源保护

- 发现：上传、向量钟和整批轮换/迁移没有明确大小上限；进程内 IP 限流桶可能无界增长。
- 修改：`internal/middleware/body_limit.go`、`rate_limit.go`、配置控制器与三个业务服务、测试。
- 取舍：单份密文最大 8 MiB、向量钟 64 KiB、批次 256 项；单次上传 JSON 16 MiB、批次 JSON 64 MiB；已知超限和解析超限返回 413。限流桶最多 10000 个，超容量拒绝新来源。
- 验证：HTTP 413 和业务大小上限单测通过。
- 剩余风险：跨实例限流仍需在网关统一实现；大规模合法数据的客户端分批策略需联调。
- 下一步：用五端最大资产样本测试上传与轮换边界，并调整客户端分批提示。

## P1：备份、恢复与运行

- 发现：备份就绪只看数据库可读；整库导出跨表不一致；旧包可回滚数据；部分审计错误被忽略；HTTP 缺少显式超时与优雅退出。
- 修改：`internal/service/backup_readiness_service.go`、`migration_bundle_service.go`、审计/策略/邀请码服务、`internal/workerstatus`、Worker、`main.go`、迁移控制器及集成测试。
- 取舍：备份就绪要求外部提供 24 小时内备份成功和 30 天内恢复演练的时间证据；这只是外部证据输入，不代替实际检查备份介质。导出使用 PostgreSQL repeatable-read 只读事务；恢复在锁表事务内预检时间倒退，显式 `allow_rollback=ALLOW_ROLLBACK` 才允许旧包覆盖。恢复清理刷新会话并提高用户 token version，恢复审计与数据同事务提交。管理员用户、策略、邀请码、资产清理的主要写操作在部署实例中与审计同事务执行；审计失败也记录关键日志与运行时失败计数。HTTP 设置读写/空闲超时及 30 秒优雅退出；Worker 的运行和最近成功/失败状态可从管理端 runtime 查看。
- 验证：PostgreSQL 恢复防回滚、会话清理、审计故障回滚集成测试通过；全量 Go 测试与 `go vet ./...` 通过。
- 剩余风险：备份证据由外部作业提供，无法在应用内证明真实可恢复；恢复仍需停写、独立数据库备份与人工操作演练。现有同步墓碑最小记录继续默认永久保留，未启用有限期物理回收。
- 下一步：预发布演练导出、校验、恢复和手工回滚；检查恢复时长与多实例 Worker 调度。

## 验证命令与范围

在本机临时 PostgreSQL 16 容器的独立测试数据库运行：

```bash
ORBITTERM_TEST_DATABASE_URL='<isolated PostgreSQL DSN>' go test -count=1 ./...
go vet ./...
POSTGRES_PASSWORD='<test value>' DATABASE_URL='<test URL>' JWT_SECRET='<test secret>' docker compose config --quiet
git diff --check
```

上述命令全部通过。PostgreSQL 测试环境是本机临时容器，不是线上服务。发布前仍需五端联合回归、预发布恢复演练和生产凭据注入验证。

## 第二轮发布前复核（基于 `60a9fa2`）

第二轮在隔离工作树修复了以下可复现问题：并发刷新同一旧令牌会撤销成功请求的会话；改密、过期封禁清理及管理员登录会用旧用户快照覆盖并发全部登出的令牌版本；管理员批量过期封禁扫描未锁住读取行；恢复防回滚检查未覆盖同步变更、设备确认与后续审计，且导出时间晚于快照；覆盖恢复会删除备份之后的审计记录。相应修复位于认证、仓储与迁移包服务，新增 PostgreSQL 并发及恢复用例。管理台已增加强制改密入口，并通过本机 Chrome 的桌面和窄屏实际交互验证。

五端只读核对范围为 macOS、iPhone、iPad（共用 Swift 网络代码）、Android、Windows，未改客户端仓库。Apple 端的 `RefreshCoordinator` 能合并进程内刷新；Android 前台与同步 Worker 存在并发刷新和旧令牌回写风险；Windows 授权请求没有刷新串行化。三套客户端均未识别 `PASSWORD_CHANGE_REQUIRED`、`REFRESH_IN_PROGRESS`、`AUTH_STATE_CHANGED`，本地退出也未调用新的服务端退出接口。HTTP 413 目前只会呈现通用错误。五端须完成这些行为和大样本上传边界联调，再进入发布候选。

复核后，隔离 PostgreSQL 上的 `go test ./... -count=1`、`go vet ./...` 和 `git diff --check` 均通过；这不代替预发布数据恢复演练及五端联调。分支仍未推送、未创建 PR、未部署，线上数据未访问。

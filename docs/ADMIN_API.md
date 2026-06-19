# OrbitTerm 管理端 API 与初始化指南

本文档面向没有后端经验的部署者，用于完成管理端首次初始化、登录、用户治理、系统策略、备份自检与审计查询。

示例默认后端地址：

```bash
export ORBIT_API="https://server.orbitterm.com"
```

如果你在服务器本机调试，也可以临时使用：

```bash
export ORBIT_API="http://127.0.0.1:8080"
```

## 1. 首次初始化 super_admin

### 1.1 确认是否需要初始化

```bash
curl -s "$ORBIT_API/api/v1/admin/bootstrap/status"
```

返回示例：

```json
{
  "success": true,
  "data": {
    "needs_setup": true,
    "admin_count": 0
  }
}
```

说明：

- `needs_setup=true`：当前还没有管理员，可以创建首个 `super_admin`。
- `needs_setup=false`：管理端已经初始化，不能重复创建首个管理员。

### 1.2 创建首个 super_admin

前提：后端环境变量必须配置 `ADMIN_BOOTSTRAP_TOKEN`。

```bash
export ADMIN_BOOTSTRAP_TOKEN="替换为你在 1Panel 环境变量里配置的 ADMIN_BOOTSTRAP_TOKEN"

curl -s -X POST "$ORBIT_API/api/v1/admin/bootstrap/super-admin" \
  -H "Content-Type: application/json" \
  -H "X-Orbit-Admin-Bootstrap-Token: $ADMIN_BOOTSTRAP_TOKEN" \
  -d '{
    "username": "admin@example.com",
    "password": "ReplaceWithStrongAdminPassword123"
  }'
```

安全建议：

1. 创建成功后，建议清空或轮换 `ADMIN_BOOTSTRAP_TOKEN` 并重启后端。
2. `ADMIN_BOOTSTRAP_TOKEN` 不是管理员登录密码，只是首次初始化防护令牌。
3. 管理员密码至少 12 位，建议使用密码管理器生成。

## 2. 管理员登录

```bash
curl -s -X POST "$ORBIT_API/api/v1/admin/auth/login" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "admin@example.com",
    "password": "ReplaceWithStrongAdminPassword123"
  }'
```

返回后保存 `access_token`：

```bash
export ADMIN_TOKEN="粘贴返回的 access_token"
```

后续管理端接口统一携带：

```bash
-H "Authorization: Bearer $ADMIN_TOKEN"
```

## 2.1 公共健康检查

该接口无需登录，适合 1Panel、反向代理或外部探活使用。

```bash
curl -s "$ORBIT_API/healthz"
```

返回 `status=ok` 表示后端和数据库连通；若数据库不可达会返回 HTTP 503，状态为 `degraded`。

## 3. 管理端首页概览

```bash
curl -s "$ORBIT_API/api/v1/admin/dashboard/overview" \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

用途：管理端首页聚合展示。

包含：

- 用户总数、正常/风险/封禁/注销数量
- 管理员数量
- 云端配置数量
- 备份就绪摘要
- 最近审计日志

### 3.1 管理端运行状态

```bash
curl -s "$ORBIT_API/api/v1/admin/system/runtime" \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

用途：

- 查看服务运行时长。
- 查看数据库连通性与连接池摘要。
- 查看 JWT Access/Refresh 周期和密钥强度状态。
- 查看自动解封任务是否启用、实际扫描间隔和批量上限。

## 4. 用户治理接口

### 4.0 创建受管用户/管理员

仅 `super_admin` 可调用。创建后用户必须修改初始登录密码。

```bash
curl -s -X POST "$ORBIT_API/api/v1/admin/users/managed" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "ops-admin@example.com",
    "password": "ReplaceWithStrongInitialPassword123",
    "role": "admin",
    "reason": "新增运维管理员",
    "confirmation": "CONFIRM"
  }'
```

### 4.0.1 调整用户角色

仅 `super_admin` 可调用。禁止管理员调整自己的角色，避免自我降权后无法恢复。

```bash
curl -s -X POST "$ORBIT_API/api/v1/admin/users/用户ID/role" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "role": "support",
    "reason": "临时降级为支持角色",
    "confirmation": "CONFIRM"
  }'
```

### 4.1 用户列表

```bash
curl -s "$ORBIT_API/api/v1/admin/users?limit=50&offset=0" \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

可选筛选：

- `q`：用户名关键词
- `role`：`user`、`support`、`admin`、`super_admin`
- `status`：`normal`、`risk`、`banned`、`deleted`

### 4.2 用户详情

```bash
curl -s "$ORBIT_API/api/v1/admin/users/用户ID" \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

### 4.3 封禁用户

高危操作，必须传入 `reason` 与 `confirmation: "CONFIRM"`。

```bash
curl -s -X POST "$ORBIT_API/api/v1/admin/users/用户ID/ban" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "duration_minutes": 1440,
    "reason": "异常登录行为，临时封禁 24 小时",
    "confirmation": "CONFIRM"
  }'
```

说明：

- `duration_minutes` 大于 0 表示限时封禁。
- 不传或小于等于 0 表示永久封禁。

### 4.4 解封用户

```bash
curl -s -X POST "$ORBIT_API/api/v1/admin/users/用户ID/unban" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "reason": "风险解除，恢复访问"
  }'
```

### 4.5 重置登录密码

高危操作，必须传入 `reason` 与 `confirmation: "CONFIRM"`。

```bash
curl -s -X POST "$ORBIT_API/api/v1/admin/users/用户ID/reset-password" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "new_password": "ReplaceWithNewUserLoginPassword123",
    "reason": "用户申请重置登录密码",
    "confirmation": "CONFIRM"
  }'
```

注意：

- 该操作只重置“登录密码”。
- 不会、也不能重置或找回用户“主密码”。
- 用户旧 Token 会失效。

### 4.6 强制下线

```bash
curl -s -X POST "$ORBIT_API/api/v1/admin/users/用户ID/force-logout" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "reason": "安全检查，强制旧 Token 失效",
    "confirmation": "CONFIRM"
  }'
```

### 4.7 软删除/注销用户

```bash
curl -s -X POST "$ORBIT_API/api/v1/admin/users/用户ID/soft-delete" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "reason": "用户注销申请",
    "confirmation": "CONFIRM"
  }'
```

### 4.8 恢复用户

```bash
curl -s -X POST "$ORBIT_API/api/v1/admin/users/用户ID/restore" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "reason": "误删恢复"
  }'
```

### 4.9 扫描并自动解封到期封禁

```bash
curl -s -X POST "$ORBIT_API/api/v1/admin/users/expired-bans/scan" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "limit": 100,
    "reason": "周期性扫描到期封禁",
    "confirmation": "CONFIRM"
  }'
```

### 4.10 后台自动解封任务

后端启动后可自动周期性扫描限时封禁用户，到期后自动解封并写入审计日志。审计中的 `admin_user_id=0` 表示系统后台任务。

可通过以下环境变量控制：

- `ADMIN_AUTO_UNBAN_ENABLED`：是否启用，默认 `true`。
- `ADMIN_AUTO_UNBAN_INTERVAL_MINUTES`：扫描间隔，默认 `10`。
- `ADMIN_AUTO_UNBAN_BATCH_LIMIT`：单次扫描上限，默认 `100`，建议不超过 `500`。

## 5. 系统策略

### 5.1 查看安全策略

```bash
curl -s "$ORBIT_API/api/v1/admin/system/security-policy" \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

### 5.2 更新安全策略

```bash
curl -s -X PUT "$ORBIT_API/api/v1/admin/system/security-policy" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "registration_enabled": false,
    "registration_disabled_reason": "当前为内测阶段，暂不开放注册",
    "min_password_length": 12,
    "default_user_status": "normal",
    "reason": "内测期间关闭公开注册"
  }'
```

安全边界：

- 普通注册永远只能创建 `user` 角色。
- 不能通过安全策略把新用户默认设置为管理员。

## 6. 主密码与恢复策略

### 6.1 用户端公开恢复说明

无需登录：

```bash
curl -s "$ORBIT_API/api/v1/auth/recovery-info"
```

### 6.2 管理端查看恢复策略

```bash
curl -s "$ORBIT_API/api/v1/admin/system/recovery-policy" \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

### 6.3 更新恢复策略展示文案

```bash
curl -s -X PUT "$ORBIT_API/api/v1/admin/system/recovery-policy" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "support_contact": "support@example.com",
    "user_facing_message": "管理员可以重置登录密码，但无法找回主密码。请妥善保存主密码。",
    "require_user_acknowledgement": true,
    "reason": "更新用户端恢复提示文案"
  }'
```

强制安全边界：

- 后端不保存主密码。
- 后端不保存主密码派生密钥。
- 后端不能解密用户资产。
- 忘记主密码时，服务端无法恢复旧加密资产。

## 7. 备份自检

```bash
curl -s "$ORBIT_API/api/v1/admin/system/backup-readiness" \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

用途：

- 检查数据库是否可读。
- 统计关键表数量。
- 检查关键环境变量是否配置或仍为默认值。
- 返回脱敏后的环境检查结果。

注意：

- 该接口不会返回 `JWT_SECRET`、数据库密码等明文。
- 真正的数据库备份建议使用 1Panel PostgreSQL 备份或容器内 `pg_dump`。

## 8. 诊断包导出

```bash
curl -s "$ORBIT_API/api/v1/admin/system/diagnostics" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -o orbitterm-diagnostics.json
```

返回内容包含：

- `runtime`：Go 版本、系统架构、Gin 模式。
- `backup_readiness`：数据库可读性、关键表计数、脱敏环境变量检查。
- `recent_audit_summary`：最近 20 条审计摘要。
- `redaction_policy`：诊断包脱敏规则说明。

该接口会写入 `system_diagnostics_export` 审计记录。

安全说明：

- 不返回 `JWT_SECRET`、数据库密码、`ADMIN_BOOTSTRAP_TOKEN` 原文。
- 不返回用户主密码、主密码派生密钥、服务器密码、私钥或资产明文。
- 审计摘要只标记快照是否存在，不导出快照正文。

## 9. 审计日志

```bash
curl -s "$ORBIT_API/api/v1/admin/audit-logs?limit=50&offset=0" \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

可选筛选：

- `action`
- `resource_type`
- `admin_user_id`
- `target_user_id`

示例：查看某个用户相关审计：

```bash
curl -s "$ORBIT_API/api/v1/admin/audit-logs?target_user_id=用户ID&limit=50" \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

## 10. 角色权限速查

| 角色 | 可查看 | 可操作 |
| --- | --- | --- |
| `super_admin` | 所有管理端数据 | 所有管理端操作、创建管理员、调整角色 |
| `admin` | 所有管理端数据 | 用户治理、策略修改、备份自检；不能创建管理员或调整角色 |
| `support` | 用户列表、审计、策略查看 | 不可执行高危操作 |
| `user` | 普通客户端接口 | 无管理端权限 |

## 11. 常见错误

### 管理端已初始化

说明：数据库中已经存在管理员角色，不能重复创建首个 `super_admin`。

### 管理员初始化令牌未配置

说明：后端没有配置 `ADMIN_BOOTSTRAP_TOKEN`，无法创建首个管理员。

### 高危操作需要二次确认

说明：请求体缺少：

```json
{
  "confirmation": "CONFIRM"
}
```

### 管理操作原因必填

说明：请求体缺少 `reason`，或 `reason` 太短。

### 该账号无管理端权限

说明：普通用户不能登录管理端，需要 `support`、`admin` 或 `super_admin` 角色。

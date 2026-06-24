# 管理平台整改与成熟度路线

## 已完成的本轮整改

- 独立管理员登录/初始化界面，内容区在鉴权前不可见。
- 修复管理员登录 Token wire format，所有受保护请求稳定携带 Bearer Token。
- 策略改为预设选项为主、邮箱后缀等场景允许填写；保存与取消行为明确。
- 普通注册强制邀请码、邮箱域白名单和 12 位以上复杂密码。
- 邀请码支持次数、有效期、撤销、摘要存储和审计。
- 加密全量迁移包覆盖全部当前业务表与运行参数快照；数据库覆盖恢复使用单事务。
- 统一 UTC 存储，浏览器本地化显示。

## 跳板机 / ProxyJump 设计结论

该能力有真实跨区域价值，但不能通过在远端拼接 `ssh target` 命令实现。安全实现必须新增 checked ProxyJump 主线：

1. 跳板机和最终目标分别建立 typed base session。
2. 两跳分别执行 Known Hosts 校验；任一 Unknown 都单独挑战，Changed/Revoked 都阻断。
3. 最终目标认证流量通过受限的 direct-tcpip channel 转发，不把目标凭据落到跳板机磁盘。
4. 策略限制最大跳数、允许的目标网段、超时和并发，并写入管理员审计。
5. Swift 只接收 typed jump/target binding；Release 不允许回退 legacy 或 shell exec。

在上述 checked ABI、UI 和 OpenSSH 集成测试完成前，ProxyJump 不进入公开 Release。

## 专业化成熟度差距

### P0 运维门禁

- 为迁移恢复增加独立维护模式、恢复前自动事故快照和双管理员审批。
- 为管理员启用 MFA/WebAuthn、会话列表、设备撤销和异常登录告警。
- 将加密迁移包恢复纳入真实 PostgreSQL 集成测试与季度演练。

### P1 企业治理

- RBAC 权限矩阵细化到只读审计、用户支持、安全管理员和备份管理员。
- 邀请码批量导出、使用人关联、组织/团队归属和注册速率异常检测。
- 审计日志不可变外送、Webhook/SIEM、告警规则和长期归档。
- Known Hosts 管理 UI、证书轮换流程和双人复核。

### P2 连接能力与体验

- checked ProxyJump、代理、Agent Forwarding 的独立威胁模型和最小权限实现。
- Docker rename/update typed ABI、Batch 多目标 Host Key challenge 聚合。
- 备份计划调度、对象存储、保留策略、校验和与恢复演练报告。
- 国际化、细粒度时区显示偏好、无障碍与操作引导。

## 不应采取的捷径

- 不在浏览器或数据库中保存迁移口令明文。
- 不让在线进程直接改写容器环境变量。
- 不通过 Trust All、Changed accept anyway 或 legacy fallback 简化跳板机。
- 不把“数据库文件存在”视为备份可恢复；必须有校验和与恢复演练。

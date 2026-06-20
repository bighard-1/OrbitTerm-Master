# 资产删除、墓碑与最近删除同步规范

本文定义 OrbitTerm 多端资产删除的安全不变量。服务端只保存密文和不含资产内容的同步元数据，不具备解密能力。

## 生命周期

- `active`：正常同步的资产。
- `deleted`：位于最近删除中，保留加密 Blob，允许用户在保留期内恢复。
- `purged`：加密 Blob 已清除，仅保留 AssetID、Vector Clock 与删除时间等最小墓碑。

默认最近删除保留 90 天。最小墓碑默认永久保留，用于防止长期离线设备重新上传旧副本。

## 冲突不变量

1. 删除必须通过稳定的 `AssetID` 识别资产，不能仅依赖数据库自增 ID。
2. 旧版本更新不得覆盖较新的删除墓碑。
3. 并发删除与修改采用 Delete-Wins；用户主动恢复必须生成严格更新的 Vector Clock。
4. 删除、恢复和永久删除必须携带唯一 Operation ID，并支持幂等重试。
5. 服务端不得记录资产名称、主机地址、凭据、主密码或解密后的身份指纹。
6. 历史配置的 AssetID 只能由已解锁客户端解密后回填，服务端不得尝试推导。

## 兼容发布顺序

1. 先部署仅增加字段与默认策略的服务端版本。
2. 发布支持 AssetID 和墓碑协议的客户端，逐步回填历史记录。
3. 确认活跃客户端完成迁移后，再启用云端墓碑删除接口。
4. 最后启用最近删除清理任务和管理端策略配置。

旧客户端在迁移阶段继续使用原有接口，不会因新增字段而停止同步。

## 当前墓碑 API

以下接口均要求 Bearer Token：

- `GET /api/v1/config/trash?limit=100&offset=0`：分页读取仍可恢复的记录。
- `POST /api/v1/config/assets/:asset_id/delete`：移入最近删除。
- `POST /api/v1/config/assets/:asset_id/restore`：恢复资产。
- `POST /api/v1/config/assets/:asset_id/purge`：清除密文并保留最小墓碑。

删除和恢复请求体：

```json
{
  "device_id": "客户端稳定设备 UUID",
  "operation_id": "每次操作新生成的 UUID",
  "vector_clock": "{\"device-id\":2}"
}
```

永久删除还必须附加 `"confirmation": "CONFIRM"`。

同一 `operation_id` 可安全重试。服务端会在事务内锁定目标资产，重复删除不会延长最近删除期限。

旧版 `DELETE /api/v1/config/:id` 暂时保留物理删除语义，仅供迁移期客户端使用；新版客户端不得继续调用该接口。

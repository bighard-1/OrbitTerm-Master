package worker

import (
	"context"
	"log"
	"time"

	"orbitterm-server/internal/config"
	"orbitterm-server/internal/service"
)

const (
	defaultAssetTrashCleanupInterval = time.Hour
	minimumAssetTrashCleanupInterval = 5 * time.Minute
)

type AssetTrashCleaner interface {
	CleanupExpiredAssetsBySystem() (service.AssetTrashCleanupResult, error)
}

// StartAssetTrashCleanupWorker 定期执行动态删除策略。
// 是否启用、批量上限及保留周期每轮都从数据库读取，管理员修改后无需重启服务。
func StartAssetTrashCleanupWorker(ctx context.Context, cfg config.Config, cleaner AssetTrashCleaner) {
	if cleaner == nil {
		log.Println("资产最近删除清理任务缺少执行器，已跳过启动")
		return
	}
	interval := time.Duration(cfg.AssetTrashCleanupIntervalMinutes) * time.Minute
	if interval < minimumAssetTrashCleanupInterval {
		interval = defaultAssetTrashCleanupInterval
	}

	go func() {
		log.Printf("资产最近删除清理任务已启动，检查间隔: %s", interval)
		timer := time.NewTimer(15 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Println("资产最近删除清理任务已停止")
				return
			case <-timer.C:
				runAssetTrashCleanup(cleaner)
				timer.Reset(interval)
			}
		}
	}()
}

func runAssetTrashCleanup(cleaner AssetTrashCleaner) {
	result, err := cleaner.CleanupExpiredAssetsBySystem()
	if err != nil {
		log.Printf("资产最近删除清理任务执行失败: %v", err)
		return
	}
	if result.PurgedCount > 0 || result.TombstonesDeleted > 0 || result.FailedCount > 0 {
		log.Printf(
			"资产最近删除清理完成: 扫描 %d, 清除密文 %d, 墓碑回收 %d, 延期 %d, 失败 %d",
			result.ScannedCount, result.PurgedCount, result.TombstonesDeleted,
			result.TombstonesDeferred, result.FailedCount,
		)
	}
}

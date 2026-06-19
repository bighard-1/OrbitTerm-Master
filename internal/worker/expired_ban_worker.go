package worker

import (
	"context"
	"log"
	"time"

	"orbitterm-server/internal/config"
	"orbitterm-server/internal/service"
)

const (
	defaultAutoUnbanInterval = 10 * time.Minute
	minimumAutoUnbanInterval = time.Minute
	defaultAutoUnbanLimit    = 100
)

type ExpiredBanScanner interface {
	ScanExpiredBansBySystem(limit int, reason string) (*service.AdminExpiredBanScanResult, error)
}

// StartExpiredBanWorker 周期性扫描并自动解封已到期的限时封禁。
// 该 Worker 只复用 AdminUserService 的领域逻辑，不直接操作数据库，避免出现第二套解封规则。
func StartExpiredBanWorker(ctx context.Context, cfg config.Config, scanner ExpiredBanScanner) {
	if !cfg.AdminAutoUnbanEnabled {
		log.Println("管理端自动解封任务未启用")
		return
	}
	if scanner == nil {
		log.Println("管理端自动解封任务缺少扫描器，已跳过启动")
		return
	}

	interval := time.Duration(cfg.AdminAutoUnbanIntervalMinutes) * time.Minute
	if interval < minimumAutoUnbanInterval {
		interval = defaultAutoUnbanInterval
	}
	limit := cfg.AdminAutoUnbanBatchLimit
	if limit <= 0 || limit > 500 {
		limit = defaultAutoUnbanLimit
	}

	go func() {
		log.Printf("管理端自动解封任务已启动，间隔: %s, 批量上限: %d", interval, limit)
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Println("管理端自动解封任务已停止")
				return
			case <-timer.C:
				runExpiredBanScan(scanner, limit)
				timer.Reset(interval)
			}
		}
	}()
}

func runExpiredBanScan(scanner ExpiredBanScanner, limit int) {
	result, err := scanner.ScanExpiredBansBySystem(limit, "系统周期性自动解封到期封禁")
	if err != nil {
		log.Printf("管理端自动解封任务执行失败: %v", err)
		return
	}
	if result.UnbannedCount > 0 {
		log.Printf("管理端自动解封任务完成: 扫描 %d 个，到期解封 %d 个", result.ScannedCount, result.UnbannedCount)
	}
}

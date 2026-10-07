package worker

import (
	"context"
	"log"
	"orbitterm-server/internal/workerstatus"
	"time"
)

type ExpiredSessionPruner interface {
	PruneExpired(limit int) (int64, error)
}

func StartExpiredSessionWorker(ctx context.Context, pruner ExpiredSessionPruner) {
	if pruner == nil {
		return
	}
	go func() {
		workerstatus.Started("refresh_session_prune")
		defer workerstatus.Stopped("refresh_session_prune")
		log.Print("过期刷新会话清理任务已启动，间隔: 1h")
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Print("过期刷新会话清理任务已停止")
				return
			case <-ticker.C:
				count, err := pruner.PruneExpired(500)
				workerstatus.Completed("refresh_session_prune", err)
				if err != nil {
					log.Printf("过期刷新会话清理失败: %v", err)
				} else if count > 0 {
					log.Printf("过期刷新会话清理完成: %d", count)
				}
			}
		}
	}()
}

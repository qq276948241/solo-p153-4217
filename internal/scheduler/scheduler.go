package scheduler

import (
	"log"
	"time"

	"github.com/robfig/cron/v3"
	"groupbuy/internal/db"
	"groupbuy/internal/handler"
	"groupbuy/internal/model"
)

func Start() {
	c := cron.New()
	_, err := c.AddFunc("0 22 * * *", func() {
		log.Println("[定时任务] 开始执行截单...")
		count, err := handler.RunCutoff()
		if err != nil {
			log.Printf("[定时任务] 截单失败: %v\n", err)
			return
		}
		log.Printf("[定时任务] 截单完成，关闭了 %d 个团期\n", count)
	})
	if err != nil {
		log.Printf("注册截单定时任务失败: %v\n", err)
	}
	_, err = c.AddFunc("0 1 28 * *", func() {
		log.Println("[定时任务] 开始计算月度佣金...")
		month := time.Now().AddDate(0, -1, 0).Format("2006-01")
		var leaders []model.Leader
		db.DB.Find(&leaders)
		for _, l := range leaders {
			if err := handler.CalculateLeaderCommission(l.ID, month); err != nil {
				log.Printf("[定时任务] 佣金计算失败 leader_id=%d: %v\n", l.ID, err)
			}
		}
		log.Printf("[定时任务] 月度佣金计算完成 month=%s\n", month)
	})
	if err != nil {
		log.Printf("注册佣金定时任务失败: %v\n", err)
	}
	c.Start()
}

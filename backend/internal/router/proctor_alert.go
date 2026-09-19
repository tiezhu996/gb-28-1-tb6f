package router

import (
	"github.com/gin-gonic/gin"

	"github.com/onlineexam/onlineexam/internal/constants"
	"github.com/onlineexam/onlineexam/internal/handler"
	"github.com/onlineexam/onlineexam/internal/middleware"
)

// RegisterProctorAlertRoutes 监考告警模块路由。
func RegisterProctorAlertRoutes(g *gin.RouterGroup, h *handler.ProctorAlertHandler) {
	// 学生：答题时上报切屏/粘贴事件（留痕 + 阈值告警）
	g.POST("/exam-records/:id/proctor-events", middleware.RequireRoles(constants.RoleStudent), h.ReportEvent)

	// 教师：只能用自己的账号查询与处理告警（处理人取自 JWT）
	teacher := g.Group("/proctor-alerts", middleware.RequireRoles(constants.RoleTeacher))
	{
		teacher.GET("", h.List)
		teacher.GET("/:id", h.Detail)
		teacher.POST("/:id/handle", h.Handle)
	}
}

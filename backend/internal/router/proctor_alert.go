package router

import (
	"github.com/gin-gonic/gin"

	"github.com/onlineexam/onlineexam/internal/constants"
	"github.com/onlineexam/onlineexam/internal/handler"
	"github.com/onlineexam/onlineexam/internal/middleware"
)

// RegisterProctorAlertRoutes 监考告警模块路由。
func RegisterProctorAlertRoutes(g *gin.RouterGroup, h *handler.ProctorAlertHandler) {
	// 学生：答题时上报监考事件（切屏/粘贴留痕）
	g.POST("/exam-records/:id/proctor-events", middleware.RequireRoles(constants.RoleStudent), h.ReportEvent)

	// 教师/管理员：告警查询与受理/驳回（处理人取自登录账号）
	teacher := g.Group("/proctor-alerts", middleware.RequireRoles(constants.RoleTeacher, constants.RoleAdmin))
	{
		teacher.GET("", h.List)
		teacher.GET("/:id", h.Get)
		teacher.POST("/:id/handle", h.Handle)
	}
}

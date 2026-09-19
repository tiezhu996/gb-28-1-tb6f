package handler

import (
	"fmt"
	"log/slog"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/onlineexam/onlineexam/internal/constants"
	"github.com/onlineexam/onlineexam/internal/dto"
	"github.com/onlineexam/onlineexam/internal/middleware"
	"github.com/onlineexam/onlineexam/internal/service"
	"github.com/onlineexam/onlineexam/internal/util"
)

// ProctorAlertHandler 监考告警 HTTP 处理器。
type ProctorAlertHandler struct {
	svc    *service.ProctorAlertService
	logger *slog.Logger
}

// NewProctorAlertHandler 构造监考告警处理器。
func NewProctorAlertHandler(svc *service.ProctorAlertService, logger *slog.Logger) *ProctorAlertHandler {
	return &ProctorAlertHandler{svc: svc, logger: logger}
}

// ReportEvent 学生答题时上报切屏/粘贴事件（每次单独留痕，连续同类只累计次数）。
func (h *ProctorAlertHandler) ReportEvent(c *gin.Context) {
	recordID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		Error(c, util.NewAppError(constants.CodeBadRequest, "监考告警模块：recordId 参数非法"))
		return
	}
	var req dto.ReportProctorEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Error(c, util.WrapAppError(constants.CodeValidationFailed, fmt.Sprintf("监考告警模块：上报事件参数校验失败（字段 type）（%s）", err.Error()), err))
		return
	}
	result, err := h.svc.ReportEvent(c.Request.Context(), recordID, middleware.GetUserID(c), req.Type, req.Detail)
	if err != nil {
		Error(c, err)
		return
	}
	SuccessMessage(c, constants.MsgProctorEventRecorded, result)
}

// List 教师分页查询告警。
func (h *ProctorAlertHandler) List(c *gin.Context) {
	filter := bson.M{}
	if status := c.Query("status"); status != "" {
		if !constants.IsValidAlertStatus(status) {
			Error(c, util.NewAppError(constants.CodeBadRequest, fmt.Sprintf("监考告警模块：状态字段 %s 非法", status)))
			return
		}
		filter["status"] = status
	}
	if examID := c.Query("exam_id"); examID != "" {
		oid, err := primitive.ObjectIDFromHex(examID)
		if err != nil {
			Error(c, util.NewAppError(constants.CodeBadRequest, "监考告警模块：exam_id 参数非法"))
			return
		}
		filter["exam_id"] = oid
	}
	page := util.GetPageParams(c, 20)
	items, total, err := h.svc.ListAlerts(c.Request.Context(), filter, page.Page, page.PageSize)
	if err != nil {
		Error(c, err)
		return
	}
	PageResult(c, items, total, page.Page, page.PageSize)
}

// Detail 告警详情：告警 + 该答卷全部事件留痕。
func (h *ProctorAlertHandler) Detail(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		Error(c, util.NewAppError(constants.CodeBadRequest, "监考告警模块：id 参数非法"))
		return
	}
	detail, err := h.svc.GetAlertDetail(c.Request.Context(), id)
	if err != nil {
		Error(c, err)
		return
	}
	Success(c, detail)
}

// Handle 教师用自己的账号受理/驳回告警（处理人取自 JWT，处理意见必填，并发只成功一次）。
func (h *ProctorAlertHandler) Handle(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		Error(c, util.NewAppError(constants.CodeBadRequest, "监考告警模块：id 参数非法"))
		return
	}
	var req dto.HandleProctorAlertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Error(c, util.WrapAppError(constants.CodeValidationFailed, fmt.Sprintf("监考告警模块：处理参数校验失败（字段 action/opinion）（%s）", err.Error()), err))
		return
	}
	claims := middleware.GetClaims(c)
	teacherName := ""
	if claims != nil {
		teacherName = claims.Name
	}
	alert, err := h.svc.Handle(c.Request.Context(), id, middleware.GetUserID(c), teacherName, req.Action, req.Opinion)
	if err != nil {
		Error(c, err)
		return
	}
	SuccessMessage(c, constants.MsgAlertHandleSuccess, dto.ToProctorAlertResponse(alert, 0, 0))
}

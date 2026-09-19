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

// ReportEvent 学生答题时上报监考事件（切屏/粘贴等，每次事件单独调用）。
func (h *ProctorAlertHandler) ReportEvent(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		Error(c, util.NewAppError(constants.CodeBadRequest, "监考告警模块：id 参数非法"))
		return
	}
	var req dto.ReportProctorEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Error(c, util.WrapAppError(constants.CodeValidationFailed, fmt.Sprintf("监考告警模块：上报监考事件参数校验失败（字段 type）（%s）", err.Error()), err))
		return
	}
	rec, alertCreated, err := h.svc.ReportEvent(c.Request.Context(), id, middleware.GetUserID(c), req.Type, req.Detail)
	if err != nil {
		Error(c, err)
		return
	}
	SuccessMessage(c, constants.MsgProctorEventRecorded, dto.ProctorStateResponse{
		RecordID:     rec.ID.Hex(),
		ProctorStats: rec.ProctorStats,
		AlertStatus:  rec.AlertStatus,
		AlertCreated: alertCreated,
	})
}

// List 教师分页查询监考告警。
func (h *ProctorAlertHandler) List(c *gin.Context) {
	filter := bson.M{}
	if status := c.Query("status"); status != "" {
		if !constants.IsValidAlertStatus(status) {
			Error(c, util.NewAppError(constants.CodeAlertStatusErr, fmt.Sprintf(constants.MsgAlertStatusInvalid, status)))
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
	list, total, err := h.svc.List(c.Request.Context(), filter, page.Page, page.PageSize)
	if err != nil {
		Error(c, err)
		return
	}
	items := make([]dto.AlertResponse, 0, len(list))
	for _, a := range list {
		items = append(items, dto.ToAlertResponse(a))
	}
	PageResult(c, items, total, page.Page, page.PageSize)
}

// Get 查询单条监考告警。
func (h *ProctorAlertHandler) Get(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		Error(c, util.NewAppError(constants.CodeBadRequest, "监考告警模块：id 参数非法"))
		return
	}
	alert, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		Error(c, err)
		return
	}
	Success(c, dto.ToAlertResponse(alert))
}

// Handle 教师受理/驳回告警（处理人取自当前登录账号，处理意见必填）。
func (h *ProctorAlertHandler) Handle(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		Error(c, util.NewAppError(constants.CodeBadRequest, "监考告警模块：id 参数非法"))
		return
	}
	var req dto.HandleAlertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Error(c, util.WrapAppError(constants.CodeValidationFailed, fmt.Sprintf("监考告警模块：处理告警参数校验失败（字段 action/note，处理意见必填）（%s）", err.Error()), err))
		return
	}
	claims := middleware.GetClaims(c)
	handlerName := middleware.GetEmail(c)
	if claims != nil && claims.Name != "" {
		handlerName = claims.Name
	}
	alert, err := h.svc.Handle(c.Request.Context(), id, req.Action, req.Note, middleware.GetUserID(c).Hex(), handlerName)
	if err != nil {
		Error(c, err)
		return
	}
	SuccessMessage(c, constants.MsgAlertHandleSuccess, dto.ToAlertResponse(alert))
}

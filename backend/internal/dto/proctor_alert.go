package dto

import (
	"time"

	"github.com/onlineexam/onlineexam/internal/model"
)

// ReportProctorEventRequest 学生上报监考事件请求（每次切屏/粘贴单独上报一条）。
type ReportProctorEventRequest struct {
	Type   string `json:"type" binding:"required"`
	Detail string `json:"detail" binding:"omitempty,max=500"`
}

// HandleAlertRequest 教师处理监考告警请求（处理意见必填）。
type HandleAlertRequest struct {
	Action string `json:"action" binding:"required,oneof=accept reject"` // accept=受理 / reject=驳回
	Note   string `json:"note" binding:"required,min=1,max=500"`         // 处理意见（必填）
}

// AlertQuery 监考告警查询参数。
type AlertQuery struct {
	Status   string `form:"status"`
	ExamID   string `form:"exam_id"`
	Page     int64  `form:"page"`
	PageSize int64  `form:"page_size"`
}

// ProctorStateResponse 上报监考事件后的答卷监考状态回执。
type ProctorStateResponse struct {
	RecordID     string         `json:"record_id"`
	ProctorStats map[string]int `json:"proctor_stats"` // 各类型事件累计次数
	AlertStatus  string         `json:"alert_status"`  // 告警状态（空=无告警）
	AlertCreated bool           `json:"alert_created"` // 本次上报是否触发了新告警
}

// AlertResponse 监考告警响应。
type AlertResponse struct {
	ID           string         `json:"id"`
	RecordID     string         `json:"record_id"`
	ExamID       string         `json:"exam_id"`
	ExamTitle    string         `json:"exam_title"`
	StudentID    string         `json:"student_id"`
	StudentName  string         `json:"student_name"`
	TriggerType  string         `json:"trigger_type"`
	TriggerCount int            `json:"trigger_count"`
	TypeStats    map[string]int `json:"type_stats"`
	Status       string         `json:"status"`
	HandlerID    string         `json:"handler_id"`
	HandlerName  string         `json:"handler_name"`
	HandleNote   string         `json:"handle_note"`
	HandledAt    *time.Time     `json:"handled_at"`
	CreatedAt    time.Time      `json:"created_at"`
}

// ToAlertResponse 模型转响应。
func ToAlertResponse(a *model.ProctorAlert) AlertResponse {
	return AlertResponse{
		ID:           a.ID.Hex(),
		RecordID:     a.RecordID.Hex(),
		ExamID:       a.ExamID.Hex(),
		ExamTitle:    a.ExamTitle,
		StudentID:    a.StudentID.Hex(),
		StudentName:  a.StudentName,
		TriggerType:  a.TriggerType,
		TriggerCount: a.TriggerCount,
		TypeStats:    a.TypeStats,
		Status:       a.Status,
		HandlerID:    a.HandlerID,
		HandlerName:  a.HandlerName,
		HandleNote:   a.HandleNote,
		HandledAt:    a.HandledAt,
		CreatedAt:    a.CreatedAt,
	}
}

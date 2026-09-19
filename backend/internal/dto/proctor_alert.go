package dto

import (
	"time"

	"github.com/onlineexam/onlineexam/internal/model"
)

// ReportProctorEventRequest 学生上报监考事件请求（切屏/粘贴）。
type ReportProctorEventRequest struct {
	Type   string `json:"type" binding:"required,oneof=switch_tab paste"`
	Detail string `json:"detail" binding:"omitempty,max=500"`
}

// ReportProctorEventResponse 上报结果：返回当前累计次数与告警状态（已结束答卷 ignored=true）。
type ReportProctorEventResponse struct {
	Ignored     bool   `json:"ignored"` // 答卷已结束，事件被忽略且不再产生新告警
	SwitchCount int    `json:"switch_count"`
	PasteCount  int    `json:"paste_count"`
	AlertStatus string `json:"alert_status"` // none/pending/confirmed/rejected
}

// HandleProctorAlertRequest 教师处理告警请求：受理（confirm）或驳回（reject），处理意见必填。
type HandleProctorAlertRequest struct {
	Action  string `json:"action" binding:"required,oneof=confirm reject"`
	Opinion string `json:"opinion" binding:"required,min=1,max=500"`
}

// ProctorEventResponse 监考事件留痕响应。
type ProctorEventResponse struct {
	ID      string    `json:"id"`
	Type    string    `json:"type"`
	Count   int       `json:"count"`
	Detail  string    `json:"detail"`
	FirstAt time.Time `json:"first_at"`
	LastAt  time.Time `json:"last_at"`
}

// ProctorAlertResponse 监考告警响应（列表/详情复用）。
type ProctorAlertResponse struct {
	ID          string     `json:"id"`
	RecordID    string     `json:"record_id"`
	ExamID      string     `json:"exam_id"`
	ExamTitle   string     `json:"exam_title"`
	StudentID   string     `json:"student_id"`
	StudentName string     `json:"student_name"`
	TriggerType string     `json:"trigger_type"`
	Status      string     `json:"status"`
	Opinion     string     `json:"opinion"`
	HandlerName string     `json:"handler_name"`
	HandledAt   *time.Time `json:"handled_at"`
	SwitchCount int        `json:"switch_count"`
	PasteCount  int        `json:"paste_count"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// ProctorAlertDetailResponse 告警详情：告警 + 该答卷全部事件留痕。
type ProctorAlertDetailResponse struct {
	Alert  ProctorAlertResponse   `json:"alert"`
	Events []ProctorEventResponse `json:"events"`
}

// ProctorSummary 答卷监考摘要：嵌入考试记录响应（学生成绩页/教师记录页展示最终告警状态与次数）。
type ProctorSummary struct {
	AlertID     string `json:"alert_id,omitempty"`
	AlertStatus string `json:"alert_status"` // none/pending/confirmed/rejected
	SwitchCount int    `json:"switch_count"`
	PasteCount  int    `json:"paste_count"`
}

// ToProctorEventResponse 事件模型转响应。
func ToProctorEventResponse(e *model.ProctorEvent) ProctorEventResponse {
	return ProctorEventResponse{
		ID:      e.ID.Hex(),
		Type:    e.Type,
		Count:   e.Count,
		Detail:  e.Detail,
		FirstAt: e.FirstAt,
		LastAt:  e.LastAt,
	}
}

// ToProctorAlertResponse 告警模型转响应（次数由调用方按事件留痕聚合填充）。
func ToProctorAlertResponse(a *model.ProctorAlert, switchCount, pasteCount int) ProctorAlertResponse {
	return ProctorAlertResponse{
		ID:          a.ID.Hex(),
		RecordID:    a.RecordID.Hex(),
		ExamID:      a.ExamID.Hex(),
		ExamTitle:   a.ExamTitle,
		StudentID:   a.StudentID.Hex(),
		StudentName: a.StudentName,
		TriggerType: a.TriggerType,
		Status:      a.Status,
		Opinion:     a.Opinion,
		HandlerName: a.HandlerName,
		HandledAt:   a.HandledAt,
		SwitchCount: switchCount,
		PasteCount:  pasteCount,
		CreatedAt:   a.CreatedAt,
		UpdatedAt:   a.UpdatedAt,
	}
}

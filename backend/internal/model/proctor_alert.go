package model

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ProctorAlert 监考告警实体，集合 proctor_alerts。
// 状态枚举：pending（待处理）/ accepted（已受理）/ rejected（已驳回）。
// 一份答卷最多一条告警（record_id 唯一索引），重复事件不得新增告警；
// 处理意见必填，处理时间与结论（状态）在处理时落库，并发处理只能成功一次。
type ProctorAlert struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	RecordID     primitive.ObjectID `bson:"record_id" json:"record_id"` // 答卷 ID（唯一）
	ExamID       primitive.ObjectID `bson:"exam_id" json:"exam_id"`
	ExamTitle    string             `bson:"exam_title" json:"exam_title"`
	StudentID    primitive.ObjectID `bson:"student_id" json:"student_id"`
	StudentName  string             `bson:"student_name" json:"student_name"`
	TriggerType  string             `bson:"trigger_type" json:"trigger_type"`           // 触发告警的事件类型（switch_tab/copy_paste/blur）
	TriggerCount int                `bson:"trigger_count" json:"trigger_count"`         // 触发时该类型累计次数
	TypeStats    map[string]int     `bson:"type_stats" json:"type_stats"`               // 触发时各类型累计次数快照
	Status       string             `bson:"status" json:"status"`                       // pending / accepted / rejected
	HandlerID    string             `bson:"handler_id,omitempty" json:"handler_id"`     // 处理教师账号 ID（取自登录态）
	HandlerName  string             `bson:"handler_name,omitempty" json:"handler_name"` // 处理教师姓名
	HandleNote   string             `bson:"handle_note,omitempty" json:"handle_note"`   // 处理意见（必填）
	HandledAt    *time.Time         `bson:"handled_at,omitempty" json:"handled_at"`     // 处理时间
	CreatedAt    time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt    time.Time          `bson:"updated_at" json:"updated_at"`
}

package model

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ProctorEvent 监考事件留痕，集合 proctor_events。
// 学生答题时每次切屏/粘贴单独留痕；同一类事件连续重复时只在最新一条上累计 Count。
type ProctorEvent struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	RecordID  primitive.ObjectID `bson:"record_id" json:"record_id"`   // 关联答卷 exam_records._id
	ExamID    primitive.ObjectID `bson:"exam_id" json:"exam_id"`       // 冗余：便于按考试排查
	StudentID primitive.ObjectID `bson:"student_id" json:"student_id"` // 冗余：便于按学生排查
	Type      string             `bson:"type" json:"type"`             // switch_tab / paste
	Count     int                `bson:"count" json:"count"`           // 连续同类事件累计次数
	Detail    string             `bson:"detail" json:"detail"`
	FirstAt   time.Time          `bson:"first_at" json:"first_at"` // 该段连续事件首次发生时间
	LastAt    time.Time          `bson:"last_at" json:"last_at"`   // 最近一次发生时间
}

// ProctorAlert 监考告警，集合 proctor_alerts。
// 每份答卷最多一条（record_id 唯一索引），任一事件类型累计达到阈值后生成；
// 状态机：pending → confirmed/rejected（处理意见必填，记录处理时间与结论）。
type ProctorAlert struct {
	ID          primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	RecordID    primitive.ObjectID  `bson:"record_id" json:"record_id"` // 关联答卷，唯一索引保证不重复告警
	ExamID      primitive.ObjectID  `bson:"exam_id" json:"exam_id"`
	ExamTitle   string              `bson:"exam_title" json:"exam_title"`
	StudentID   primitive.ObjectID  `bson:"student_id" json:"student_id"`
	StudentName string              `bson:"student_name" json:"student_name"`
	TriggerType string              `bson:"trigger_type" json:"trigger_type"` // 触发告警的事件类型 switch_tab/paste
	Status      string              `bson:"status" json:"status"`             // pending/confirmed/rejected
	Opinion     string              `bson:"opinion" json:"opinion"`           // 处理意见（处理时必填）
	HandlerID   primitive.ObjectID  `bson:"handler_id,omitempty" json:"handler_id"`     // 处理教师账号（取自 JWT）
	HandlerName string              `bson:"handler_name" json:"handler_name"`           // 处理教师姓名
	HandledAt   *time.Time          `bson:"handled_at,omitempty" json:"handled_at"`     // 处理时间
	CreatedAt   time.Time           `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time           `bson:"updated_at" json:"updated_at"`
}

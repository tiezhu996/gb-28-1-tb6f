package model

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// AttemptOption 考试快照中的选项（已随机打乱）。
type AttemptOption struct {
	Key  string `bson:"key" json:"key"`
	Text string `bson:"text" json:"text"`
}

// AttemptQuestion 考试快照中的题目。
type AttemptQuestion struct {
	QuestionID    primitive.ObjectID `bson:"question_id" json:"question_id"`
	Type          string             `bson:"type" json:"type"`
	Subject       string             `bson:"subject" json:"subject"`
	KnowledgePoints []string         `bson:"knowledge_points" json:"knowledge_points"`
	Content       string             `bson:"content" json:"content"`
	Options       []AttemptOption    `bson:"options,omitempty" json:"options"`
	Score         float64            `bson:"score" json:"score"`
	CorrectAnswer string             `bson:"correct_answer" json:"correct_answer"` // 客观题标准答案快照
	UserAnswer    string             `bson:"user_answer" json:"user_answer"`       // 学生作答
	Result        string             `bson:"result" json:"result"`                 // correct/wrong/partial/unmarked
	GotScore      float64            `bson:"got_score" json:"got_score"`           // 客观题自动得分
	SubjectiveScore float64          `bson:"subjective_score,omitempty" json:"subjective_score"`
	Comment       string             `bson:"comment,omitempty" json:"comment"`
	Marked        bool               `bson:"marked" json:"marked"` // 教师是否已批改
}

// CheatEvent 切屏/防作弊事件。
type CheatEvent struct {
	Type      string    `bson:"type" json:"type"` // switch_tab / copy_paste / blur
	Detail    string    `bson:"detail" json:"detail"`
	OccurredAt time.Time `bson:"occurred_at" json:"occurred_at"`
}

// ProctorEvent 监考事件留痕：每次切屏/粘贴单独上报一条；
// 同一类事件连续重复时不再新增条目，只累计 Count 并刷新 LastAt。
type ProctorEvent struct {
	Type       string    `bson:"type" json:"type"`             // switch_tab / copy_paste / blur
	Detail     string    `bson:"detail" json:"detail"`         // 事件描述（如“第3次切屏”）
	Count      int       `bson:"count" json:"count"`           // 连续重复累计次数
	OccurredAt time.Time `bson:"occurred_at" json:"occurred_at"` // 首次发生时间
	LastAt     time.Time `bson:"last_at" json:"last_at"`       // 最近一次发生时间
}

// ExamRecord 考试记录/答卷实体，集合 exam_records。
// 状态枚举：in_progress / submitted / graded；结果枚举：correct / wrong / partial / unmarked。
type ExamRecord struct {
	ID            primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	ExamID        primitive.ObjectID `bson:"exam_id" json:"exam_id"`
	ExamTitle     string             `bson:"exam_title" json:"exam_title"`
	StudentID     primitive.ObjectID `bson:"student_id" json:"student_id"`
	StudentName   string             `bson:"student_name" json:"student_name"`
	Questions     []AttemptQuestion  `bson:"questions" json:"questions"`
	Status        string             `bson:"status" json:"status"`
	StartedAt     time.Time          `bson:"started_at" json:"started_at"`
	SubmittedAt   *time.Time         `bson:"submitted_at,omitempty" json:"submitted_at"`
	ObjectiveScore float64           `bson:"objective_score" json:"objective_score"` // 客观题自动得分
	SubjectiveScore float64          `bson:"subjective_score" json:"subjective_score"`
	FinalScore    float64            `bson:"final_score" json:"final_score"` // 最终总分
	CheatCount    int                `bson:"cheat_count" json:"cheat_count"`
	CheatEvents   []CheatEvent       `bson:"cheat_events,omitempty" json:"cheat_events"`
	ProctorEvents []ProctorEvent     `bson:"proctor_events,omitempty" json:"proctor_events"` // 监考事件留痕（连续同类合并计数）
	ProctorStats  map[string]int     `bson:"proctor_stats,omitempty" json:"proctor_stats"`   // 各类型事件累计次数
	AlertStatus   string             `bson:"alert_status,omitempty" json:"alert_status"`     // 告警状态快照：pending/accepted/rejected（空=无告警）
	AutoSubmitted bool               `bson:"auto_submitted" json:"auto_submitted"`
	CreatedAt     time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt     time.Time          `bson:"updated_at" json:"updated_at"`
}

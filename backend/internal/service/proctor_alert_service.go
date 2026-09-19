package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/onlineexam/onlineexam/internal/constants"
	"github.com/onlineexam/onlineexam/internal/dto"
	"github.com/onlineexam/onlineexam/internal/model"
	"github.com/onlineexam/onlineexam/internal/repository"
	"github.com/onlineexam/onlineexam/internal/util"
)

// ProctorAlertService 监考告警服务：事件留痕、阈值告警、教师处理闭环。
type ProctorAlertService struct {
	repo    repository.ProctorAlertRepository
	records repository.ExamRecordRepository // 复用考试记录仓储校验答卷归属与状态
	logger  *slog.Logger
}

// NewProctorAlertService 构造监考告警服务。
func NewProctorAlertService(repo repository.ProctorAlertRepository, records repository.ExamRecordRepository, logger *slog.Logger) *ProctorAlertService {
	return &ProctorAlertService{repo: repo, records: records, logger: logger}
}

// ReportEvent 学生答题时上报切屏/粘贴事件：
// 每次事件单独留痕；同一类事件连续重复只在最新一条上累计次数；
// 任一类型累计达到阈值后为当前答卷生成一条待处理告警（record_id 唯一，重复事件不再新增）；
// 已结束（非答题中）答卷直接忽略，不再产生新告警。
func (s *ProctorAlertService) ReportEvent(ctx context.Context, recordID, studentID primitive.ObjectID, eventType, detail string) (*dto.ReportProctorEventResponse, error) {
	if !constants.IsValidProctorEventType(eventType) {
		return nil, util.NewAppError(constants.CodeProctorEventInvalid, fmt.Sprintf(constants.MsgProctorEventInvalid, eventType))
	}
	rec, err := s.records.FindByID(ctx, recordID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, util.NewAppError(constants.CodeRecordNotFound, fmt.Sprintf(constants.MsgRecordNotFound, recordID.Hex()))
		}
		return nil, fmt.Errorf("proctor alert service report find record: %w", err)
	}
	if rec.StudentID != studentID {
		return nil, util.NewAppError(constants.CodeForbidden, fmt.Sprintf("监考告警模块：学生角色只能上报本人答卷的监考事件（record_id=%s）", recordID.Hex()))
	}
	// 已结束答卷不再留痕、不再产生新告警（幂等返回，避免干扰交卷流程）
	if rec.Status != constants.RecordStatusInProgress {
		s.logger.Info(constants.LogProctorEventIgnored, "record_id", recordID.Hex(), "event_type", eventType, "status", rec.Status)
		return &dto.ReportProctorEventResponse{Ignored: true, AlertStatus: s.alertStatusOf(ctx, recordID)}, nil
	}

	now := time.Now()
	latest, err := s.repo.FindLatestEvent(ctx, recordID)
	switch {
	case err == nil && latest.Type == eventType:
		// 同一类事件连续重复：只累计次数，不新增留痕
		if err := s.repo.IncrementEvent(ctx, latest.ID, now); err != nil {
			return nil, fmt.Errorf("proctor alert service increment event: %w", err)
		}
		s.logger.Info(constants.LogProctorEventMerged, "record_id", recordID.Hex(), "event_type", eventType, "count", latest.Count+1)
	case err != nil && !errors.Is(err, repository.ErrNotFound):
		return nil, fmt.Errorf("proctor alert service find latest event: %w", err)
	default:
		// 每次事件单独留痕
		e := &model.ProctorEvent{
			ID:        primitive.NewObjectID(),
			RecordID:  rec.ID,
			ExamID:    rec.ExamID,
			StudentID: rec.StudentID,
			Type:      eventType,
			Count:     1,
			Detail:    detail,
			FirstAt:   now,
			LastAt:    now,
		}
		if err := s.repo.CreateEvent(ctx, e); err != nil {
			return nil, fmt.Errorf("proctor alert service create event: %w", err)
		}
		s.logger.Info(constants.LogProctorEventRecorded, "record_id", recordID.Hex(), "event_type", eventType, "count", 1, "student", rec.StudentName)
	}

	counts, err := s.repo.CountEventsByRecordIDs(ctx, []primitive.ObjectID{recordID})
	if err != nil {
		return nil, fmt.Errorf("proctor alert service count events: %w", err)
	}
	switchCount := counts[recordID][constants.ProctorEventSwitchTab]
	pasteCount := counts[recordID][constants.ProctorEventPaste]

	// 任一类达到三次后为当前答卷生成一条待处理告警
	if switchCount >= constants.ProctorAlertThreshold || pasteCount >= constants.ProctorAlertThreshold {
		trigger := eventType
		if counts[recordID][eventType] < constants.ProctorAlertThreshold {
			if switchCount >= constants.ProctorAlertThreshold {
				trigger = constants.ProctorEventSwitchTab
			} else {
				trigger = constants.ProctorEventPaste
			}
		}
		alert := &model.ProctorAlert{
			ID:          primitive.NewObjectID(),
			RecordID:    rec.ID,
			ExamID:      rec.ExamID,
			ExamTitle:   rec.ExamTitle,
			StudentID:   rec.StudentID,
			StudentName: rec.StudentName,
			TriggerType: trigger,
			Status:      constants.AlertStatusPending,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := s.repo.CreateAlert(ctx, alert); err != nil {
			if !errors.Is(err, repository.ErrConflict) {
				return nil, fmt.Errorf("proctor alert service create alert: %w", err)
			}
			// 唯一索引命中：该答卷已有告警，重复事件不得新增告警
		} else {
			s.logger.Info(constants.LogProctorAlertCreated, "alert_id", alert.ID.Hex(), "record_id", rec.ID.Hex(), "trigger_type", trigger, "student", rec.StudentName)
		}
	}

	return &dto.ReportProctorEventResponse{
		Ignored:     false,
		SwitchCount: switchCount,
		PasteCount:  pasteCount,
		AlertStatus: s.alertStatusOf(ctx, recordID),
	}, nil
}

// alertStatusOf 查询答卷当前告警状态（无告警返回 none）。
func (s *ProctorAlertService) alertStatusOf(ctx context.Context, recordID primitive.ObjectID) string {
	alert, err := s.repo.FindAlertByRecord(ctx, recordID)
	if err != nil {
		return constants.AlertStatusNone
	}
	return alert.Status
}

// SummarizeByRecordIDs 批量汇总答卷的最终告警状态与各类型事件次数
// （考试记录列表/详情接口复用：学生成绩页与教师记录页展示）。
func (s *ProctorAlertService) SummarizeByRecordIDs(ctx context.Context, recordIDs []primitive.ObjectID) (map[primitive.ObjectID]dto.ProctorSummary, error) {
	out := make(map[primitive.ObjectID]dto.ProctorSummary, len(recordIDs))
	if len(recordIDs) == 0 {
		return out, nil
	}
	alerts, err := s.repo.ListAlertsByRecordIDs(ctx, recordIDs)
	if err != nil {
		return nil, fmt.Errorf("proctor alert service summarize alerts: %w", err)
	}
	counts, err := s.repo.CountEventsByRecordIDs(ctx, recordIDs)
	if err != nil {
		return nil, fmt.Errorf("proctor alert service summarize counts: %w", err)
	}
	for _, id := range recordIDs {
		summary := dto.ProctorSummary{
			AlertStatus: constants.AlertStatusNone,
			SwitchCount: counts[id][constants.ProctorEventSwitchTab],
			PasteCount:  counts[id][constants.ProctorEventPaste],
		}
		if alert, ok := alerts[id]; ok {
			summary.AlertID = alert.ID.Hex()
			summary.AlertStatus = alert.Status
		}
		out[id] = summary
	}
	return out, nil
}

// ListAlerts 教师分页查询告警（告警处理页）。
func (s *ProctorAlertService) ListAlerts(ctx context.Context, filter bson.M, page, pageSize int64) ([]dto.ProctorAlertResponse, int64, error) {
	list, total, err := s.repo.ListAlerts(ctx, filter, page, pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("proctor alert service list alerts: %w", err)
	}
	ids := make([]primitive.ObjectID, 0, len(list))
	for _, a := range list {
		ids = append(ids, a.RecordID)
	}
	counts, err := s.repo.CountEventsByRecordIDs(ctx, ids)
	if err != nil {
		return nil, 0, fmt.Errorf("proctor alert service list count events: %w", err)
	}
	items := make([]dto.ProctorAlertResponse, 0, len(list))
	for _, a := range list {
		items = append(items, dto.ToProctorAlertResponse(a, counts[a.RecordID][constants.ProctorEventSwitchTab], counts[a.RecordID][constants.ProctorEventPaste]))
	}
	return items, total, nil
}

// GetAlertDetail 告警详情：告警 + 该答卷全部事件留痕。
func (s *ProctorAlertService) GetAlertDetail(ctx context.Context, id primitive.ObjectID) (*dto.ProctorAlertDetailResponse, error) {
	alert, err := s.repo.FindAlertByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, util.NewAppError(constants.CodeAlertNotFound, fmt.Sprintf(constants.MsgAlertNotFound, id.Hex()))
		}
		return nil, fmt.Errorf("proctor alert service get alert: %w", err)
	}
	events, err := s.repo.ListEventsByRecord(ctx, alert.RecordID)
	if err != nil {
		return nil, fmt.Errorf("proctor alert service list events: %w", err)
	}
	counts, err := s.repo.CountEventsByRecordIDs(ctx, []primitive.ObjectID{alert.RecordID})
	if err != nil {
		return nil, fmt.Errorf("proctor alert service detail count events: %w", err)
	}
	detail := &dto.ProctorAlertDetailResponse{
		Alert:  dto.ToProctorAlertResponse(alert, counts[alert.RecordID][constants.ProctorEventSwitchTab], counts[alert.RecordID][constants.ProctorEventPaste]),
		Events: make([]dto.ProctorEventResponse, 0, len(events)),
	}
	for _, e := range events {
		detail.Events = append(detail.Events, dto.ToProctorEventResponse(e))
	}
	return detail, nil
}

// Handle 教师用自己的账号受理/驳回告警：处理意见必填，记录处理时间与结论；
// 原子条件更新保证并发处理只能成功一次。
func (s *ProctorAlertService) Handle(ctx context.Context, alertID, teacherID primitive.ObjectID, teacherName, action, opinion string) (*model.ProctorAlert, error) {
	opinion = strings.TrimSpace(opinion)
	if opinion == "" {
		return nil, util.NewAppError(constants.CodeValidationFailed, constants.MsgAlertOpinionEmpty)
	}
	var to string
	switch action {
	case "confirm":
		to = constants.AlertStatusConfirmed
	case "reject":
		to = constants.AlertStatusRejected
	default:
		return nil, util.NewAppError(constants.CodeValidationFailed, fmt.Sprintf(constants.MsgAlertActionInvalid, action))
	}
	alert, err := s.repo.FindAlertByID(ctx, alertID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, util.NewAppError(constants.CodeAlertNotFound, fmt.Sprintf(constants.MsgAlertNotFound, alertID.Hex()))
		}
		return nil, fmt.Errorf("proctor alert service handle find: %w", err)
	}
	if !constants.CanTransition(alert.Status, to, constants.AlertStatusTransitions) {
		return nil, util.NewAppError(constants.CodeAlertAlreadyHandled, fmt.Sprintf(constants.MsgAlertAlreadyHandled, alertID.Hex(), alert.Status))
	}
	ok, err := s.repo.HandleAlert(ctx, alertID, to, opinion, teacherID, teacherName, time.Now())
	if err != nil {
		return nil, fmt.Errorf("proctor alert service handle: %w", err)
	}
	if !ok {
		// 并发处理：状态已被其他请求抢先流转，仅允许一次成功
		s.logger.Warn(constants.LogProctorAlertConflict, "alert_id", alertID.Hex(), "teacher", teacherName)
		return nil, util.NewAppError(constants.CodeAlertAlreadyHandled, fmt.Sprintf(constants.MsgAlertAlreadyHandled, alertID.Hex(), constants.AlertStatusPending))
	}
	s.logger.Info(constants.LogProctorAlertHandled, "alert_id", alertID.Hex(), "record_id", alert.RecordID.Hex(), "conclusion", to, "teacher", teacherName)
	return s.repo.FindAlertByID(ctx, alertID)
}

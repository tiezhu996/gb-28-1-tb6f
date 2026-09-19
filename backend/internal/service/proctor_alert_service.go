package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/onlineexam/onlineexam/internal/constants"
	"github.com/onlineexam/onlineexam/internal/model"
	"github.com/onlineexam/onlineexam/internal/repository"
	"github.com/onlineexam/onlineexam/internal/util"
)

// ProctorAlertService 监考告警服务：事件留痕、阈值告警、教师受理/驳回闭环。
type ProctorAlertService struct {
	alerts  repository.ProctorAlertRepository
	records repository.ExamRecordRepository
	logger  *slog.Logger
}

// NewProctorAlertService 构造监考告警服务。
func NewProctorAlertService(alerts repository.ProctorAlertRepository, records repository.ExamRecordRepository, logger *slog.Logger) *ProctorAlertService {
	return &ProctorAlertService{alerts: alerts, records: records, logger: logger}
}

// ReportEvent 学生答题时上报监考事件（切屏/粘贴/失焦）：
//  1. 每次事件单独留痕；同一类事件连续重复只累计次数；
//  2. 任一类事件累计达到阈值（constants.ProctorAlertThreshold）后为当前答卷生成一条待处理告警；
//  3. 重复事件不得新增告警（record_id 唯一索引 + 状态快照双重保证）；
//  4. 已结束（已提交/已批改）答卷不再留痕也不再产生新告警。
//
// 返回更新后的答卷与本次是否触发了新告警。
func (s *ProctorAlertService) ReportEvent(ctx context.Context, recordID, studentID primitive.ObjectID, eventType, detail string) (*model.ExamRecord, bool, error) {
	if !constants.IsValidProctorEventType(eventType) {
		return nil, false, util.NewAppError(constants.CodeProctorEventInvalid, fmt.Sprintf(constants.MsgProctorEventInvalid, eventType))
	}
	rec, err := s.records.FindByID(ctx, recordID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, false, util.NewAppError(constants.CodeRecordNotFound, fmt.Sprintf(constants.MsgRecordNotFound, recordID.Hex()))
		}
		return nil, false, fmt.Errorf("proctor alert service report find record: %w", err)
	}
	// 学生只能给自己的答卷上报监考事件
	if rec.StudentID != studentID {
		return nil, false, util.NewAppError(constants.CodeForbidden, fmt.Sprintf("监考告警模块：学生角色只能上报本人答卷的监考事件（record_id=%s）", recordID.Hex()))
	}
	// 已结束答卷不再产生新告警（也不再留痕），直接返回当前状态保证幂等
	if rec.Status != constants.RecordStatusInProgress {
		s.logger.Info(constants.LogProctorEventIgnored, "record_id", rec.ID.Hex(), "status", rec.Status, "event_type", eventType)
		return rec, false, nil
	}

	now := time.Now()
	// 留痕：同一类事件连续重复只累计次数，否则新增一条留痕
	if n := len(rec.ProctorEvents); n > 0 && rec.ProctorEvents[n-1].Type == eventType {
		rec.ProctorEvents[n-1].Count++
		rec.ProctorEvents[n-1].LastAt = now
		if detail != "" {
			rec.ProctorEvents[n-1].Detail = detail
		}
	} else {
		rec.ProctorEvents = append(rec.ProctorEvents, model.ProctorEvent{
			Type:       eventType,
			Detail:     detail,
			Count:      1,
			OccurredAt: now,
			LastAt:     now,
		})
	}
	if rec.ProctorStats == nil {
		rec.ProctorStats = make(map[string]int)
	}
	rec.ProctorStats[eventType]++
	rec.UpdatedAt = now

	// 任一类达到阈值后为当前答卷生成一条待处理告警；已存在告警时重复事件不得新增
	alertCreated := false
	if rec.AlertStatus == "" && rec.ProctorStats[eventType] >= constants.ProctorAlertThreshold {
		alert := &model.ProctorAlert{
			ID:           primitive.NewObjectID(),
			RecordID:     rec.ID,
			ExamID:       rec.ExamID,
			ExamTitle:    rec.ExamTitle,
			StudentID:    rec.StudentID,
			StudentName:  rec.StudentName,
			TriggerType:  eventType,
			TriggerCount: rec.ProctorStats[eventType],
			TypeStats:    copyStats(rec.ProctorStats),
			Status:       constants.AlertStatusPending,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if err := s.alerts.Create(ctx, alert); err != nil {
			if !errors.Is(err, repository.ErrConflict) {
				return nil, false, fmt.Errorf("proctor alert service create alert: %w", err)
			}
			// 并发场景下告警已被其他请求创建：同步已有告警状态，不新增
			if existing, ferr := s.alerts.FindByRecordID(ctx, rec.ID); ferr == nil {
				rec.AlertStatus = existing.Status
			}
		} else {
			alertCreated = true
			rec.AlertStatus = constants.AlertStatusPending
			s.logger.Info(constants.LogProctorAlertCreated,
				"alert_id", alert.ID.Hex(), "record_id", rec.ID.Hex(),
				"trigger_type", eventType, "trigger_count", rec.ProctorStats[eventType], "student", rec.StudentName)
		}
	}

	if err := s.records.Update(ctx, rec); err != nil {
		return nil, false, fmt.Errorf("proctor alert service report update record: %w", err)
	}
	s.logger.Info(constants.LogProctorEventRecorded, "record_id", rec.ID.Hex(), "event_type", eventType, "type_count", rec.ProctorStats[eventType], "student", rec.StudentName)
	return rec, alertCreated, nil
}

// Handle 教师受理/驳回告警：处理意见必填，记录处理时间与结论；并发处理只能成功一次。
func (s *ProctorAlertService) Handle(ctx context.Context, alertID primitive.ObjectID, action, note, handlerID, handlerName string) (*model.ProctorAlert, error) {
	if !constants.IsValidAlertAction(action) {
		return nil, util.NewAppError(constants.CodeAlertStatusErr, fmt.Sprintf(constants.MsgAlertStatusInvalid, action))
	}
	if note == "" {
		return nil, util.NewAppError(constants.CodeValidationFailed, constants.MsgAlertNoteRequired)
	}
	to := constants.AlertStatusFromAction(action)
	// 状态机校验：pending → accepted/rejected
	if !constants.CanTransition(constants.AlertStatusPending, to, constants.AlertStatusTransitions) {
		return nil, util.NewAppError(constants.CodeAlertStatusErr, fmt.Sprintf(constants.MsgAlertStatusInvalid, action))
	}
	// 先确认告警存在（区分 404 与并发冲突）
	alert, err := s.alerts.FindByID(ctx, alertID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, util.NewAppError(constants.CodeAlertNotFound, fmt.Sprintf(constants.MsgAlertNotFound, alertID.Hex()))
		}
		return nil, fmt.Errorf("proctor alert service handle find: %w", err)
	}
	// 原子条件更新：仅 pending 可处理，并发处理只能成功一次
	updated, err := s.alerts.HandleIfPending(ctx, alertID, to, handlerID, handlerName, note, time.Now())
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, util.NewAppError(constants.CodeAlertAlreadyHandled, fmt.Sprintf(constants.MsgAlertAlreadyHandled, alertID.Hex(), alert.HandlerName))
		}
		return nil, fmt.Errorf("proctor alert service handle: %w", err)
	}
	// 同步答卷上的告警状态快照（学生成绩页/教师记录页展示最终告警状态），失败不影响处理结果
	if rec, rerr := s.records.FindByID(ctx, updated.RecordID); rerr == nil {
		rec.AlertStatus = updated.Status
		rec.UpdatedAt = time.Now()
		if uerr := s.records.Update(ctx, rec); uerr != nil {
			s.logger.Warn("同步答卷告警状态快照失败", "record_id", updated.RecordID.Hex(), "error", uerr.Error())
		}
	}
	s.logger.Info(constants.LogProctorAlertHandled, "alert_id", updated.ID.Hex(), "record_id", updated.RecordID.Hex(), "action", action, "teacher", handlerName)
	return updated, nil
}

// List 教师分页查询监考告警（可按状态/试卷过滤）。
func (s *ProctorAlertService) List(ctx context.Context, filter bson.M, page, pageSize int64) ([]*model.ProctorAlert, int64, error) {
	list, total, err := s.alerts.List(ctx, filter, page, pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("proctor alert service list: %w", err)
	}
	return list, total, nil
}

// GetByID 查询单条监考告警。
func (s *ProctorAlertService) GetByID(ctx context.Context, id primitive.ObjectID) (*model.ProctorAlert, error) {
	alert, err := s.alerts.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, util.NewAppError(constants.CodeAlertNotFound, fmt.Sprintf(constants.MsgAlertNotFound, id.Hex()))
		}
		return nil, fmt.Errorf("proctor alert service get: %w", err)
	}
	return alert, nil
}

// GetByRecordID 按答卷查询监考告警（不存在时返回 nil, nil）。
func (s *ProctorAlertService) GetByRecordID(ctx context.Context, recordID primitive.ObjectID) (*model.ProctorAlert, error) {
	alert, err := s.alerts.FindByRecordID(ctx, recordID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("proctor alert service get by record: %w", err)
	}
	return alert, nil
}

func copyStats(src map[string]int) map[string]int {
	dst := make(map[string]int, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

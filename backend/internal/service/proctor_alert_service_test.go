package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/onlineexam/onlineexam/internal/constants"
	"github.com/onlineexam/onlineexam/internal/model"
	"github.com/onlineexam/onlineexam/internal/repository"
	"github.com/onlineexam/onlineexam/internal/util"
)

// fakeAlertRepo 内存版监考告警仓储（模拟唯一索引与原子条件更新语义）。
type fakeAlertRepo struct {
	alerts   map[string]*model.ProctorAlert
	byRecord map[string]string // record_id -> alert_id（模拟唯一索引）
}

func newFakeAlertRepo() *fakeAlertRepo {
	return &fakeAlertRepo{alerts: make(map[string]*model.ProctorAlert), byRecord: make(map[string]string)}
}

func (f *fakeAlertRepo) Create(_ context.Context, a *model.ProctorAlert) error {
	if _, dup := f.byRecord[a.RecordID.Hex()]; dup {
		return repository.ErrConflict
	}
	f.alerts[a.ID.Hex()] = a
	f.byRecord[a.RecordID.Hex()] = a.ID.Hex()
	return nil
}

func (f *fakeAlertRepo) FindByID(_ context.Context, id primitive.ObjectID) (*model.ProctorAlert, error) {
	if a, ok := f.alerts[id.Hex()]; ok {
		return a, nil
	}
	return nil, repository.ErrNotFound
}

func (f *fakeAlertRepo) FindByRecordID(_ context.Context, recordID primitive.ObjectID) (*model.ProctorAlert, error) {
	if id, ok := f.byRecord[recordID.Hex()]; ok {
		return f.alerts[id], nil
	}
	return nil, repository.ErrNotFound
}

func (f *fakeAlertRepo) List(_ context.Context, _ bson.M, _, _ int64) ([]*model.ProctorAlert, int64, error) {
	var out []*model.ProctorAlert
	for _, a := range f.alerts {
		out = append(out, a)
	}
	return out, int64(len(out)), nil
}

func (f *fakeAlertRepo) HandleIfPending(_ context.Context, id primitive.ObjectID, status, handlerID, handlerName, note string, handledAt time.Time) (*model.ProctorAlert, error) {
	a, ok := f.alerts[id.Hex()]
	if !ok || a.Status != constants.AlertStatusPending {
		return nil, repository.ErrNotFound
	}
	a.Status = status
	a.HandlerID = handlerID
	a.HandlerName = handlerName
	a.HandleNote = note
	a.HandledAt = &handledAt
	return a, nil
}

func newTestProctorSvc() (*ProctorAlertService, *fakeRecordRepo, *fakeAlertRepo) {
	recordRepo := newFakeRecordRepo()
	alertRepo := newFakeAlertRepo()
	svc := NewProctorAlertService(alertRepo, recordRepo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return svc, recordRepo, alertRepo
}

// newInProgressRecord 构造一条答题中的答卷。
func newInProgressRecord(recordRepo *fakeRecordRepo) *model.ExamRecord {
	rec := &model.ExamRecord{
		ID:          primitive.NewObjectID(),
		ExamID:      primitive.NewObjectID(),
		ExamTitle:   "监考测试卷",
		StudentID:   primitive.NewObjectID(),
		StudentName: "李同学",
		Status:      constants.RecordStatusInProgress,
		StartedAt:   time.Now(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	_ = recordRepo.Create(context.Background(), rec)
	return rec
}

func TestProctorEventThresholdAndDedup(t *testing.T) {
	svc, recordRepo, alertRepo := newTestProctorSvc()
	rec := newInProgressRecord(recordRepo)
	ctx := context.Background()

	// 前两次切屏：只留痕与累计，不产生告警
	for i := 0; i < 2; i++ {
		_, created, err := svc.ReportEvent(ctx, rec.ID, rec.StudentID, constants.ProctorEventSwitchTab, "切屏")
		if err != nil {
			t.Fatalf("ReportEvent() error = %v", err)
		}
		if created {
			t.Fatal("未达阈值不应生成告警")
		}
	}
	// 第三次切屏：达到阈值，生成一条待处理告警
	updated, created, err := svc.ReportEvent(ctx, rec.ID, rec.StudentID, constants.ProctorEventSwitchTab, "切屏")
	if err != nil {
		t.Fatalf("ReportEvent() error = %v", err)
	}
	if !created {
		t.Fatal("达到阈值应生成告警")
	}
	if updated.AlertStatus != constants.AlertStatusPending {
		t.Fatalf("alert status = %s, want pending", updated.AlertStatus)
	}
	if updated.ProctorStats[constants.ProctorEventSwitchTab] != 3 {
		t.Fatalf("switch_tab count = %d, want 3", updated.ProctorStats[constants.ProctorEventSwitchTab])
	}
	// 连续同类事件只累计次数：3 次切屏只有 1 条留痕
	if len(updated.ProctorEvents) != 1 || updated.ProctorEvents[0].Count != 3 {
		t.Fatalf("proctor events = %+v, want 1 entry with count 3", updated.ProctorEvents)
	}

	// 继续上报（含粘贴达到阈值）：重复事件不得新增告警
	for i := 0; i < 4; i++ {
		if _, _, err := svc.ReportEvent(ctx, rec.ID, rec.StudentID, constants.ProctorEventCopyPaste, "粘贴"); err != nil {
			t.Fatalf("ReportEvent() error = %v", err)
		}
	}
	if _, created, err := svc.ReportEvent(ctx, rec.ID, rec.StudentID, constants.ProctorEventSwitchTab, "切屏"); err != nil || created {
		t.Fatalf("已有告警后不得新增, created = %v, err = %v", created, err)
	}
	if len(alertRepo.alerts) != 1 {
		t.Fatalf("alert count = %d, want 1（重复事件不得新增告警）", len(alertRepo.alerts))
	}
	final, _ := recordRepo.FindByID(ctx, rec.ID)
	if final.ProctorStats[constants.ProctorEventCopyPaste] != 4 {
		t.Fatalf("copy_paste count = %d, want 4", final.ProctorStats[constants.ProctorEventCopyPaste])
	}
	// 交替类型产生多条留痕：switch_tab(3) → copy_paste(4) → switch_tab(1)
	if len(final.ProctorEvents) != 3 {
		t.Fatalf("proctor events len = %d, want 3", len(final.ProctorEvents))
	}
	if final.ProctorEvents[2].Type != constants.ProctorEventSwitchTab || final.ProctorEvents[2].Count != 1 {
		t.Fatalf("last event = %+v, want switch_tab count 1", final.ProctorEvents[2])
	}
}

func TestProctorFinishedRecordNoAlert(t *testing.T) {
	svc, recordRepo, alertRepo := newTestProctorSvc()
	rec := newInProgressRecord(recordRepo)
	ctx := context.Background()

	// 答卷已结束（已提交）
	rec.Status = constants.RecordStatusSubmitted
	_ = recordRepo.Update(ctx, rec)

	for i := 0; i < 5; i++ {
		updated, created, err := svc.ReportEvent(ctx, rec.ID, rec.StudentID, constants.ProctorEventSwitchTab, "切屏")
		if err != nil {
			t.Fatalf("ReportEvent() error = %v", err)
		}
		if created {
			t.Fatal("已结束答卷不得产生新告警")
		}
		if updated.AlertStatus != "" {
			t.Fatalf("alert status = %s, want empty", updated.AlertStatus)
		}
	}
	if len(alertRepo.alerts) != 0 {
		t.Fatalf("alert count = %d, want 0", len(alertRepo.alerts))
	}
	updated, _ := recordRepo.FindByID(ctx, rec.ID)
	if len(updated.ProctorEvents) != 0 || len(updated.ProctorStats) != 0 {
		t.Fatal("已结束答卷不应再留痕")
	}
}

func TestProctorEventPermissionAndType(t *testing.T) {
	svc, recordRepo, _ := newTestProctorSvc()
	rec := newInProgressRecord(recordRepo)
	ctx := context.Background()

	// 非法事件类型
	if _, _, err := svc.ReportEvent(ctx, rec.ID, rec.StudentID, "hack", ""); err == nil {
		t.Fatal("非法事件类型应报错")
	} else {
		var appErr *util.AppError
		if !errors.As(err, &appErr) || appErr.Code != constants.CodeProctorEventInvalid {
			t.Fatalf("error = %v, want CodeProctorEventInvalid", err)
		}
	}
	// 他人答卷禁止上报
	if _, _, err := svc.ReportEvent(ctx, rec.ID, primitive.NewObjectID(), constants.ProctorEventSwitchTab, ""); err == nil {
		t.Fatal("他人答卷上报应报错")
	} else {
		var appErr *util.AppError
		if !errors.As(err, &appErr) || appErr.Code != constants.CodeForbidden {
			t.Fatalf("error = %v, want CodeForbidden", err)
		}
	}
}

func TestAlertHandleOnlyOnce(t *testing.T) {
	svc, recordRepo, alertRepo := newTestProctorSvc()
	rec := newInProgressRecord(recordRepo)
	ctx := context.Background()

	// 触发告警
	for i := 0; i < 3; i++ {
		_, _, _ = svc.ReportEvent(ctx, rec.ID, rec.StudentID, constants.ProctorEventSwitchTab, "切屏")
	}
	var alert *model.ProctorAlert
	for _, a := range alertRepo.alerts {
		alert = a
	}
	if alert == nil {
		t.Fatal("应已生成告警")
	}

	// 处理意见必填
	if _, err := svc.Handle(ctx, alert.ID, constants.AlertActionAccept, "", "tid", "王老师"); err == nil {
		t.Fatal("处理意见必填")
	}
	// 非法动作
	if _, err := svc.Handle(ctx, alert.ID, "close", "意见", "tid", "王老师"); err == nil {
		t.Fatal("非法处理动作应报错")
	}

	// 教师受理：记录处理人、意见、时间与结论
	handled, err := svc.Handle(ctx, alert.ID, constants.AlertActionAccept, "确认违规，成绩作废", "tid-1", "王老师")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if handled.Status != constants.AlertStatusAccepted {
		t.Fatalf("status = %s, want accepted", handled.Status)
	}
	if handled.HandleNote != "确认违规，成绩作废" || handled.HandlerName != "王老师" || handled.HandledAt == nil {
		t.Fatalf("handled = %+v, 处理人/意见/时间缺失", handled)
	}
	// 答卷快照同步为最终告警状态
	recAfter, _ := recordRepo.FindByID(ctx, rec.ID)
	if recAfter.AlertStatus != constants.AlertStatusAccepted {
		t.Fatalf("record alert status = %s, want accepted", recAfter.AlertStatus)
	}

	// 并发/重复处理只能成功一次
	if _, err := svc.Handle(ctx, alert.ID, constants.AlertActionReject, "误报", "tid-2", "李老师"); err == nil {
		t.Fatal("重复处理应失败（并发只能成功一次）")
	} else {
		var appErr *util.AppError
		if !errors.As(err, &appErr) || appErr.Code != constants.CodeAlertAlreadyHandled {
			t.Fatalf("error = %v, want CodeAlertAlreadyHandled", err)
		}
	}
	if alertRepo.alerts[alert.ID.Hex()].HandlerName != "王老师" {
		t.Fatal("并发处理不得覆盖首次处理结果")
	}
}

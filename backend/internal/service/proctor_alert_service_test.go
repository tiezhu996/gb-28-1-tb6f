package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/onlineexam/onlineexam/internal/constants"
	"github.com/onlineexam/onlineexam/internal/model"
	"github.com/onlineexam/onlineexam/internal/repository"
	"github.com/onlineexam/onlineexam/internal/util"
)

// fakeProctorRepo 内存版监考告警仓储（带锁，模拟原子处理语义）。
type fakeProctorRepo struct {
	mu     sync.Mutex
	events []*model.ProctorEvent
	alerts map[string]*model.ProctorAlert // key: record_id hex（模拟唯一索引）
}

func newFakeProctorRepo() *fakeProctorRepo {
	return &fakeProctorRepo{alerts: make(map[string]*model.ProctorAlert)}
}

func (f *fakeProctorRepo) CreateEvent(_ context.Context, e *model.ProctorEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, e)
	return nil
}

func (f *fakeProctorRepo) FindLatestEvent(_ context.Context, recordID primitive.ObjectID) (*model.ProctorEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var latest *model.ProctorEvent
	for _, e := range f.events {
		if e.RecordID == recordID && (latest == nil || e.LastAt.After(latest.LastAt)) {
			latest = e
		}
	}
	if latest == nil {
		return nil, repository.ErrNotFound
	}
	return latest, nil
}

func (f *fakeProctorRepo) IncrementEvent(_ context.Context, id primitive.ObjectID, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, e := range f.events {
		if e.ID == id {
			e.Count++
			e.LastAt = at
			return nil
		}
	}
	return repository.ErrNotFound
}

func (f *fakeProctorRepo) ListEventsByRecord(_ context.Context, recordID primitive.ObjectID) ([]*model.ProctorEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*model.ProctorEvent
	for _, e := range f.events {
		if e.RecordID == recordID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeProctorRepo) CountEventsByRecordIDs(_ context.Context, ids []primitive.ObjectID) (map[primitive.ObjectID]map[string]int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[primitive.ObjectID]map[string]int)
	for _, e := range f.events {
		for _, id := range ids {
			if e.RecordID == id {
				m, ok := out[id]
				if !ok {
					m = make(map[string]int)
					out[id] = m
				}
				m[e.Type] += e.Count
			}
		}
	}
	return out, nil
}

func (f *fakeProctorRepo) CreateAlert(_ context.Context, a *model.ProctorAlert) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.alerts[a.RecordID.Hex()]; ok {
		return repository.ErrConflict // 唯一索引：同一答卷仅一条告警
	}
	f.alerts[a.RecordID.Hex()] = a
	return nil
}

func (f *fakeProctorRepo) FindAlertByRecord(_ context.Context, recordID primitive.ObjectID) (*model.ProctorAlert, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if a, ok := f.alerts[recordID.Hex()]; ok {
		return a, nil
	}
	return nil, repository.ErrNotFound
}

func (f *fakeProctorRepo) FindAlertByID(_ context.Context, id primitive.ObjectID) (*model.ProctorAlert, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, a := range f.alerts {
		if a.ID == id {
			return a, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (f *fakeProctorRepo) ListAlerts(_ context.Context, filter bson.M, _, _ int64) ([]*model.ProctorAlert, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*model.ProctorAlert
	for _, a := range f.alerts {
		if status, ok := filter["status"]; ok && a.Status != status {
			continue
		}
		out = append(out, a)
	}
	return out, int64(len(out)), nil
}

func (f *fakeProctorRepo) ListAlertsByRecordIDs(_ context.Context, ids []primitive.ObjectID) (map[primitive.ObjectID]*model.ProctorAlert, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[primitive.ObjectID]*model.ProctorAlert)
	for _, id := range ids {
		if a, ok := f.alerts[id.Hex()]; ok {
			out[id] = a
		}
	}
	return out, nil
}

// HandleAlert 模拟原子条件更新：锁内校验 pending，并发只成功一次。
func (f *fakeProctorRepo) HandleAlert(_ context.Context, id primitive.ObjectID, status, opinion string, handlerID primitive.ObjectID, handlerName string, handledAt time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, a := range f.alerts {
		if a.ID == id {
			if a.Status != constants.AlertStatusPending {
				return false, nil
			}
			a.Status = status
			a.Opinion = opinion
			a.HandlerID = handlerID
			a.HandlerName = handlerName
			a.HandledAt = &handledAt
			return true, nil
		}
	}
	return false, nil
}

func newTestProctorSvc() (*ProctorAlertService, *fakeProctorRepo, *fakeRecordRepo) {
	proctorRepo := newFakeProctorRepo()
	recordRepo := newFakeRecordRepo()
	svc := NewProctorAlertService(proctorRepo, recordRepo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return svc, proctorRepo, recordRepo
}

func newInProgressRecord(recordRepo *fakeRecordRepo, studentID primitive.ObjectID) *model.ExamRecord {
	now := time.Now()
	rec := &model.ExamRecord{
		ID:          primitive.NewObjectID(),
		ExamID:      primitive.NewObjectID(),
		ExamTitle:   "期中考试",
		StudentID:   studentID,
		StudentName: "张三",
		Status:      constants.RecordStatusInProgress,
		StartedAt:   now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_ = recordRepo.Create(context.Background(), rec)
	return rec
}

// TestProctorEventTraceAndAlert 留痕/连续合并/阈值告警/重复事件不新增告警。
func TestProctorEventTraceAndAlert(t *testing.T) {
	svc, proctorRepo, recordRepo := newTestProctorSvc()
	student := primitive.NewObjectID()
	rec := newInProgressRecord(recordRepo, student)
	ctx := context.Background()

	// 切屏、切屏（连续同类合并）、粘贴、切屏
	seq := []string{
		constants.ProctorEventSwitchTab,
		constants.ProctorEventSwitchTab,
		constants.ProctorEventPaste,
		constants.ProctorEventSwitchTab,
	}
	for _, typ := range seq {
		if _, err := svc.ReportEvent(ctx, rec.ID, student, typ, ""); err != nil {
			t.Fatalf("ReportEvent(%s) error = %v", typ, err)
		}
	}

	// 留痕：3 条（switch x2 合并为一条、paste x1、switch x1）
	events, err := proctorRepo.ListEventsByRecord(ctx, rec.ID)
	if err != nil {
		t.Fatalf("ListEventsByRecord() error = %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("event traces = %d, want 3（连续同类应合并）", len(events))
	}
	if events[0].Type != constants.ProctorEventSwitchTab || events[0].Count != 2 {
		t.Fatalf("first trace = %+v, want switch_tab count=2", events[0])
	}

	// 切屏累计 3 次 → 已生成一条待处理告警
	alert, err := proctorRepo.FindAlertByRecord(ctx, rec.ID)
	if err != nil {
		t.Fatalf("alert should exist after 3 switch events: %v", err)
	}
	if alert.Status != constants.AlertStatusPending {
		t.Fatalf("alert status = %s, want pending", alert.Status)
	}
	if alert.TriggerType != constants.ProctorEventSwitchTab {
		t.Fatalf("trigger type = %s, want switch_tab", alert.TriggerType)
	}

	// 继续上报（含粘贴达到 3 次）：重复事件不得新增告警
	for i := 0; i < 3; i++ {
		res, err := svc.ReportEvent(ctx, rec.ID, student, constants.ProctorEventPaste, "")
		if err != nil {
			t.Fatalf("ReportEvent(paste) error = %v", err)
		}
		if i == 2 && res.PasteCount != 4 {
			t.Fatalf("paste count = %d, want 4", res.PasteCount)
		}
	}
	if len(proctorRepo.alerts) != 1 {
		t.Fatalf("alerts = %d, want 1（重复事件不得新增告警）", len(proctorRepo.alerts))
	}

	// 摘要：最终告警状态与次数
	summaries, err := svc.SummarizeByRecordIDs(ctx, []primitive.ObjectID{rec.ID})
	if err != nil {
		t.Fatalf("SummarizeByRecordIDs() error = %v", err)
	}
	sum := summaries[rec.ID]
	if sum.AlertStatus != constants.AlertStatusPending || sum.SwitchCount != 3 || sum.PasteCount != 4 {
		t.Fatalf("summary = %+v, want pending/switch=3/paste=4", sum)
	}
}

// TestProctorEventIgnoredWhenFinished 已结束答卷不再留痕、不再产生新告警。
func TestProctorEventIgnoredWhenFinished(t *testing.T) {
	svc, proctorRepo, recordRepo := newTestProctorSvc()
	student := primitive.NewObjectID()
	rec := newInProgressRecord(recordRepo, student)
	rec.Status = constants.RecordStatusSubmitted
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		res, err := svc.ReportEvent(ctx, rec.ID, student, constants.ProctorEventSwitchTab, "")
		if err != nil {
			t.Fatalf("ReportEvent() error = %v", err)
		}
		if !res.Ignored {
			t.Fatal("已结束答卷上报事件应被忽略")
		}
	}
	if len(proctorRepo.events) != 0 {
		t.Fatalf("events = %d, want 0（已结束答卷不再留痕）", len(proctorRepo.events))
	}
	if len(proctorRepo.alerts) != 0 {
		t.Fatalf("alerts = %d, want 0（已结束答卷不再产生新告警）", len(proctorRepo.alerts))
	}
}

// TestProctorReportValidation 非法事件类型 / 他人答卷 校验。
func TestProctorReportValidation(t *testing.T) {
	svc, _, recordRepo := newTestProctorSvc()
	student := primitive.NewObjectID()
	rec := newInProgressRecord(recordRepo, student)
	ctx := context.Background()

	if _, err := svc.ReportEvent(ctx, rec.ID, student, "blur", ""); err == nil {
		t.Fatal("非法事件类型应报错")
	}
	if _, err := svc.ReportEvent(ctx, rec.ID, primitive.NewObjectID(), constants.ProctorEventSwitchTab, ""); err == nil {
		t.Fatal("上报他人答卷应报错")
	}
	if _, err := svc.ReportEvent(ctx, primitive.NewObjectID(), student, constants.ProctorEventSwitchTab, ""); err == nil {
		t.Fatal("答卷不存在应报错")
	}
}

// TestProctorHandleFlow 教师处理闭环：意见必填、记录时间与结论、状态机。
func TestProctorHandleFlow(t *testing.T) {
	svc, _, recordRepo := newTestProctorSvc()
	student := primitive.NewObjectID()
	rec := newInProgressRecord(recordRepo, student)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := svc.ReportEvent(ctx, rec.ID, student, constants.ProctorEventPaste, ""); err != nil {
			t.Fatalf("ReportEvent() error = %v", err)
		}
	}
	alert, err := svc.repo.FindAlertByRecord(ctx, rec.ID)
	if err != nil {
		t.Fatalf("alert should exist: %v", err)
	}
	teacher := primitive.NewObjectID()

	// 处理意见必填
	if _, err := svc.Handle(ctx, alert.ID, teacher, "王老师", "confirm", "  "); err == nil {
		t.Fatal("处理意见为空应报错")
	}
	// 非法动作
	if _, err := svc.Handle(ctx, alert.ID, teacher, "王老师", "close", "意见"); err == nil {
		t.Fatal("非法处理动作应报错")
	}
	// 受理
	handled, err := svc.Handle(ctx, alert.ID, teacher, "王老师", "confirm", "确认违规，成绩作废")
	if err != nil {
		t.Fatalf("Handle(confirm) error = %v", err)
	}
	if handled.Status != constants.AlertStatusConfirmed {
		t.Fatalf("status = %s, want confirmed", handled.Status)
	}
	if handled.Opinion != "确认违规，成绩作废" || handled.HandlerName != "王老师" || handled.HandledAt == nil {
		t.Fatalf("handled = %+v, want opinion/handler/handled_at recorded", handled)
	}
	// 重复处理（含驳回）应失败：并发处理只能成功一次
	if _, err := svc.Handle(ctx, alert.ID, teacher, "王老师", "reject", "改判"); err == nil {
		t.Fatal("重复处理应失败")
	} else {
		var appErr *util.AppError
		if !errors.As(err, &appErr) || appErr.Code != constants.CodeAlertAlreadyHandled {
			t.Fatalf("error = %v, want CodeAlertAlreadyHandled", err)
		}
	}
	// 不存在的告警
	if _, err := svc.Handle(ctx, primitive.NewObjectID(), teacher, "王老师", "confirm", "意见"); err == nil {
		t.Fatal("告警不存在应报错")
	}
}

// TestProctorHandleConcurrent 并发处理同一条告警只能成功一次。
func TestProctorHandleConcurrent(t *testing.T) {
	svc, _, recordRepo := newTestProctorSvc()
	student := primitive.NewObjectID()
	rec := newInProgressRecord(recordRepo, student)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := svc.ReportEvent(ctx, rec.ID, student, constants.ProctorEventSwitchTab, ""); err != nil {
			t.Fatalf("ReportEvent() error = %v", err)
		}
	}
	alert, err := svc.repo.FindAlertByRecord(ctx, rec.ID)
	if err != nil {
		t.Fatalf("alert should exist: %v", err)
	}

	const workers = 16
	var wg sync.WaitGroup
	var mu sync.Mutex
	success := 0
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			action := "confirm"
			if i%2 == 0 {
				action = "reject"
			}
			_, err := svc.Handle(ctx, alert.ID, primitive.NewObjectID(), "王老师", action, "并发处理意见")
			if err == nil {
				mu.Lock()
				success++
				mu.Unlock()
				return
			}
			var appErr *util.AppError
			if !errors.As(err, &appErr) || appErr.Code != constants.CodeAlertAlreadyHandled {
				t.Errorf("unexpected error = %v", err)
			}
		}(i)
	}
	wg.Wait()
	if success != 1 {
		t.Fatalf("concurrent handle success = %d, want 1", success)
	}
	final, err := svc.repo.FindAlertByID(ctx, alert.ID)
	if err != nil {
		t.Fatalf("FindAlertByID() error = %v", err)
	}
	if final.Status != constants.AlertStatusConfirmed && final.Status != constants.AlertStatusRejected {
		t.Fatalf("final status = %s, want confirmed/rejected", final.Status)
	}
	if final.HandledAt == nil || final.Opinion == "" {
		t.Fatal("处理时间与处理意见必须记录")
	}
}

package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/onlineexam/onlineexam/internal/model"
)

// ProctorAlertRepository 监考告警仓储接口（事件留痕 + 告警处理）。
type ProctorAlertRepository interface {
	// 事件留痕
	CreateEvent(ctx context.Context, e *model.ProctorEvent) error
	FindLatestEvent(ctx context.Context, recordID primitive.ObjectID) (*model.ProctorEvent, error)
	IncrementEvent(ctx context.Context, id primitive.ObjectID, at time.Time) error
	ListEventsByRecord(ctx context.Context, recordID primitive.ObjectID) ([]*model.ProctorEvent, error)
	CountEventsByRecordIDs(ctx context.Context, ids []primitive.ObjectID) (map[primitive.ObjectID]map[string]int, error)
	// 告警
	CreateAlert(ctx context.Context, a *model.ProctorAlert) error
	FindAlertByRecord(ctx context.Context, recordID primitive.ObjectID) (*model.ProctorAlert, error)
	FindAlertByID(ctx context.Context, id primitive.ObjectID) (*model.ProctorAlert, error)
	ListAlerts(ctx context.Context, filter bson.M, page, pageSize int64) ([]*model.ProctorAlert, int64, error)
	ListAlertsByRecordIDs(ctx context.Context, ids []primitive.ObjectID) (map[primitive.ObjectID]*model.ProctorAlert, error)
	// HandleAlert 原子处理：仅当状态仍为 pending 时更新成功，返回是否处理成功（并发只成功一次）。
	HandleAlert(ctx context.Context, id primitive.ObjectID, status, opinion string, handlerID primitive.ObjectID, handlerName string, handledAt time.Time) (bool, error)
}

// MongoProctorAlertRepository MongoDB 监考告警仓储实现。
type MongoProctorAlertRepository struct {
	events *mongo.Collection
	alerts *mongo.Collection
}

// NewMongoProctorAlertRepository 构造监考告警仓储。
func NewMongoProctorAlertRepository(db *mongo.Database) *MongoProctorAlertRepository {
	return &MongoProctorAlertRepository{
		events: db.Collection("proctor_events"),
		alerts: db.Collection("proctor_alerts"),
	}
}

func (r *MongoProctorAlertRepository) CreateEvent(ctx context.Context, e *model.ProctorEvent) error {
	if _, err := r.events.InsertOne(ctx, e); err != nil {
		return fmt.Errorf("create proctor event: %w", err)
	}
	return nil
}

func (r *MongoProctorAlertRepository) FindLatestEvent(ctx context.Context, recordID primitive.ObjectID) (*model.ProctorEvent, error) {
	var e model.ProctorEvent
	opts := options.FindOne().SetSort(bson.D{{Key: "last_at", Value: -1}, {Key: "_id", Value: -1}})
	if err := r.events.FindOne(ctx, bson.M{"record_id": recordID}, opts).Decode(&e); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, fmt.Errorf("find latest proctor event: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("find latest proctor event: %w", err)
	}
	return &e, nil
}

// IncrementEvent 连续同类事件只累计次数：count+1 并刷新 last_at。
func (r *MongoProctorAlertRepository) IncrementEvent(ctx context.Context, id primitive.ObjectID, at time.Time) error {
	res, err := r.events.UpdateOne(ctx, bson.M{"_id": id}, bson.M{
		"$inc": bson.M{"count": 1},
		"$set": bson.M{"last_at": at},
	})
	if err != nil {
		return fmt.Errorf("increment proctor event: %w", err)
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("increment proctor event: %w", ErrNotFound)
	}
	return nil
}

func (r *MongoProctorAlertRepository) ListEventsByRecord(ctx context.Context, recordID primitive.ObjectID) ([]*model.ProctorEvent, error) {
	opts := options.Find().SetSort(bson.D{{Key: "first_at", Value: 1}, {Key: "_id", Value: 1}})
	cur, err := r.events.Find(ctx, bson.M{"record_id": recordID}, opts)
	if err != nil {
		return nil, fmt.Errorf("list proctor events: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }()
	var list []*model.ProctorEvent
	if err := cur.All(ctx, &list); err != nil {
		return nil, fmt.Errorf("decode proctor events: %w", err)
	}
	return list, nil
}

// CountEventsByRecordIDs 按答卷聚合各类型事件累计次数（学生成绩页/教师记录页/告警列表共用）。
func (r *MongoProctorAlertRepository) CountEventsByRecordIDs(ctx context.Context, ids []primitive.ObjectID) (map[primitive.ObjectID]map[string]int, error) {
	out := make(map[primitive.ObjectID]map[string]int, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"record_id": bson.M{"$in": ids}}}},
		{{Key: "$group", Value: bson.M{
			"_id":   bson.M{"record_id": "$record_id", "type": "$type"},
			"total": bson.M{"$sum": "$count"},
		}}},
	}
	cur, err := r.events.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("count proctor events: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }()
	var rows []struct {
		ID struct {
			RecordID primitive.ObjectID `bson:"record_id"`
			Type     string             `bson:"type"`
		} `bson:"_id"`
		Total int `bson:"total"`
	}
	if err := cur.All(ctx, &rows); err != nil {
		return nil, fmt.Errorf("decode proctor event counts: %w", err)
	}
	for _, row := range rows {
		m, ok := out[row.ID.RecordID]
		if !ok {
			m = make(map[string]int)
			out[row.ID.RecordID] = m
		}
		m[row.ID.Type] = row.Total
	}
	return out, nil
}

// CreateAlert 创建告警；proctor_alerts.record_id 唯一索引保证同一答卷仅一条，
// 重复事件触发时返回 ErrConflict，由 service 忽略（重复事件不得新增告警）。
func (r *MongoProctorAlertRepository) CreateAlert(ctx context.Context, a *model.ProctorAlert) error {
	if _, err := r.alerts.InsertOne(ctx, a); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("create proctor alert: %w", ErrConflict)
		}
		return fmt.Errorf("create proctor alert: %w", err)
	}
	return nil
}

func (r *MongoProctorAlertRepository) FindAlertByRecord(ctx context.Context, recordID primitive.ObjectID) (*model.ProctorAlert, error) {
	var a model.ProctorAlert
	if err := r.alerts.FindOne(ctx, bson.M{"record_id": recordID}).Decode(&a); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, fmt.Errorf("find proctor alert by record: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("find proctor alert by record: %w", err)
	}
	return &a, nil
}

func (r *MongoProctorAlertRepository) FindAlertByID(ctx context.Context, id primitive.ObjectID) (*model.ProctorAlert, error) {
	var a model.ProctorAlert
	if err := r.alerts.FindOne(ctx, bson.M{"_id": id}).Decode(&a); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, fmt.Errorf("find proctor alert by id: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("find proctor alert by id: %w", err)
	}
	return &a, nil
}

func (r *MongoProctorAlertRepository) ListAlerts(ctx context.Context, filter bson.M, page, pageSize int64) ([]*model.ProctorAlert, int64, error) {
	total, err := r.alerts.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("count proctor alerts: %w", err)
	}
	opts := options.Find().
		SetSkip((page - 1) * pageSize).
		SetLimit(pageSize).
		SetSort(bson.D{{Key: "created_at", Value: -1}})
	cur, err := r.alerts.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("list proctor alerts: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }()
	var list []*model.ProctorAlert
	if err := cur.All(ctx, &list); err != nil {
		return nil, 0, fmt.Errorf("decode proctor alerts: %w", err)
	}
	return list, total, nil
}

func (r *MongoProctorAlertRepository) ListAlertsByRecordIDs(ctx context.Context, ids []primitive.ObjectID) (map[primitive.ObjectID]*model.ProctorAlert, error) {
	out := make(map[primitive.ObjectID]*model.ProctorAlert, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	cur, err := r.alerts.Find(ctx, bson.M{"record_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, fmt.Errorf("list proctor alerts by records: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }()
	var list []*model.ProctorAlert
	if err := cur.All(ctx, &list); err != nil {
		return nil, fmt.Errorf("decode proctor alerts by records: %w", err)
	}
	for _, a := range list {
		out[a.RecordID] = a
	}
	return out, nil
}

// HandleAlert 原子状态流转：filter 锁定 status=pending，并发处理只有一个请求能匹配成功。
func (r *MongoProctorAlertRepository) HandleAlert(ctx context.Context, id primitive.ObjectID, status, opinion string, handlerID primitive.ObjectID, handlerName string, handledAt time.Time) (bool, error) {
	filter := bson.M{"_id": id, "status": alertStatusPending()}
	update := bson.M{"$set": bson.M{
		"status":       status,
		"opinion":      opinion,
		"handler_id":   handlerID,
		"handler_name": handlerName,
		"handled_at":   handledAt,
		"updated_at":   handledAt,
	}}
	res, err := r.alerts.UpdateOne(ctx, filter, update)
	if err != nil {
		return false, fmt.Errorf("handle proctor alert: %w", err)
	}
	return res.ModifiedCount > 0, nil
}

// alertStatusPending 避免 repository 直接依赖 constants 的循环，这里硬编码状态值。
// 注意：该状态枚举同时在 constants/enums.go、service 状态机、formatters、前端 constants 中出现。
func alertStatusPending() string {
	return "pending"
}

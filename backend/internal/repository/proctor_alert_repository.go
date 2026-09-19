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

// ProctorAlertRepository 监考告警仓储接口。
type ProctorAlertRepository interface {
	Create(ctx context.Context, a *model.ProctorAlert) error
	FindByID(ctx context.Context, id primitive.ObjectID) (*model.ProctorAlert, error)
	FindByRecordID(ctx context.Context, recordID primitive.ObjectID) (*model.ProctorAlert, error)
	List(ctx context.Context, filter bson.M, page, pageSize int64) ([]*model.ProctorAlert, int64, error)
	// HandleIfPending 原子条件更新：仅当告警仍处于 pending 时写入处理结论（并发处理只能成功一次）。
	HandleIfPending(ctx context.Context, id primitive.ObjectID, status, handlerID, handlerName, note string, handledAt time.Time) (*model.ProctorAlert, error)
}

// MongoProctorAlertRepository MongoDB 监考告警仓储实现。
type MongoProctorAlertRepository struct {
	coll *mongo.Collection
}

// NewMongoProctorAlertRepository 构造监考告警仓储。
func NewMongoProctorAlertRepository(db *mongo.Database) *MongoProctorAlertRepository {
	return &MongoProctorAlertRepository{coll: db.Collection("proctor_alerts")}
}

func (r *MongoProctorAlertRepository) Create(ctx context.Context, a *model.ProctorAlert) error {
	_, err := r.coll.InsertOne(ctx, a)
	if err != nil {
		// record_id 唯一索引冲突：该答卷已存在告警，重复事件不得新增告警
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("create proctor alert: %w", ErrConflict)
		}
		return fmt.Errorf("create proctor alert: %w", err)
	}
	return nil
}

func (r *MongoProctorAlertRepository) FindByID(ctx context.Context, id primitive.ObjectID) (*model.ProctorAlert, error) {
	var alert model.ProctorAlert
	if err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&alert); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, fmt.Errorf("find proctor alert by id: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("find proctor alert by id: %w", err)
	}
	return &alert, nil
}

func (r *MongoProctorAlertRepository) FindByRecordID(ctx context.Context, recordID primitive.ObjectID) (*model.ProctorAlert, error) {
	var alert model.ProctorAlert
	if err := r.coll.FindOne(ctx, bson.M{"record_id": recordID}).Decode(&alert); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, fmt.Errorf("find proctor alert by record: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("find proctor alert by record: %w", err)
	}
	return &alert, nil
}

func (r *MongoProctorAlertRepository) List(ctx context.Context, filter bson.M, page, pageSize int64) ([]*model.ProctorAlert, int64, error) {
	total, err := r.coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("count proctor alerts: %w", err)
	}
	opts := options.Find().
		SetSkip((page - 1) * pageSize).
		SetLimit(pageSize).
		SetSort(bson.M{"created_at": -1})
	cur, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("list proctor alerts: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }()
	var alerts []*model.ProctorAlert
	if err := cur.All(ctx, &alerts); err != nil {
		return nil, 0, fmt.Errorf("decode proctor alerts: %w", err)
	}
	return alerts, total, nil
}

func (r *MongoProctorAlertRepository) HandleIfPending(ctx context.Context, id primitive.ObjectID, status, handlerID, handlerName, note string, handledAt time.Time) (*model.ProctorAlert, error) {
	// 原子条件更新：filter 中锁定 status=pending，两个并发请求只有一个能匹配成功。
	filter := bson.M{"_id": id, "status": alertStatusPending()}
	update := bson.M{"$set": bson.M{
		"status":       status,
		"handler_id":   handlerID,
		"handler_name": handlerName,
		"handle_note":  note,
		"handled_at":   handledAt,
		"updated_at":   handledAt,
	}}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var alert model.ProctorAlert
	if err := r.coll.FindOneAndUpdate(ctx, filter, update, opts).Decode(&alert); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			// 告警不存在或已被他人处理（状态已非 pending）
			return nil, fmt.Errorf("handle proctor alert if pending: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("handle proctor alert if pending: %w", err)
	}
	return &alert, nil
}

// alertStatusPending 避免 repository 直接依赖 constants 的循环，这里硬编码状态值。
// 注意：该状态枚举同时在 constants/enums.go、service 状态机、formatters、前端 constants 中出现。
func alertStatusPending() string {
	return "pending"
}

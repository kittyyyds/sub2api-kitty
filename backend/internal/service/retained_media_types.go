package service

import (
	"context"
	"errors"
	"time"
)

var (
	ErrStorageExpired     = errors.New("素材已到期，不能再购买延期")
	ErrStorageChanged     = errors.New("素材期限或订单已变化，请刷新后重试")
	ErrStorageUnavailable = errors.New("云素材存储暂不可用")
	ErrStorageInvalid     = errors.New("素材或延期参数不正确")
)

type RetainedMedia struct {
	ID        string    `json:"id"`
	UserID    int64     `json:"-"`
	SourceKey string    `json:"-"`
	SHA256    string    `json:"-"`
	Kind      string    `json:"kind"`
	Name      string    `json:"name"`
	MimeType  string    `json:"mime_type"`
	Bytes     int64     `json:"bytes"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}
type StorageExtension struct {
	RequestID      string    `json:"request_id"`
	Days           int       `json:"days"`
	ExpectedExpiry time.Time `json:"expected_expiry"`
}
type StorageReceipt struct {
	OrderID   string    `json:"order_id"`
	AssetID   string    `json:"asset_id"`
	Days      int       `json:"days"`
	Amount    string    `json:"amount_usd"`
	Before    time.Time `json:"before_expiry"`
	After     time.Time `json:"after_expiry"`
	CreatedAt time.Time `json:"created_at"`
}
type RetainedMediaRepository interface {
	Create(context.Context, *RetainedMedia) (*RetainedMedia, error)
	Get(context.Context, int64, string) (*RetainedMedia, error)
	BySource(context.Context, int64, string) (*RetainedMedia, error)
	List(context.Context, int64, int) ([]RetainedMedia, error)
	Extend(context.Context, int64, string, StorageExtension) (*StorageReceipt, error)
	Orders(context.Context, int64, int) ([]StorageReceipt, error)
	Cleanup(context.Context, func(string) error) (int, error)
}

package domain

import (
	"net/netip"
	"time"

	"github.com/google/uuid"
)

// User は、ユーザー。
type User struct {
	ID           uuid.UUID
	Username     string
	DisplayName  string
	Role         string
	PasswordHash *string
	DisabledAt   *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// AuditEvent は、監査ログの 1 件。要約は JSON のまま持つ。
type AuditEvent struct {
	ID          uuid.UUID
	OccurredAt  time.Time
	ActorType   string
	ActorUserID *uuid.UUID
	ActorRole   *string
	Action      string
	TargetType  string
	TargetID    *string
	Summary     []byte
	SourceIP    *netip.Addr
	UserAgent   *string
	ActorClient *string
}

// TunerServerStatus は、チューナーサーバーの状態。
type TunerServerStatus struct {
	DegradedSince *time.Time
	LastError     string
	ClockSkewMs   *int64
	CheckedAt     time.Time
}

// StoragePool は、保存先プールの設定と状態。
type StoragePool struct {
	ID           string
	Priority     int32
	MinFreeBytes int64
	LowSince     *time.Time
	UpdatedAt    time.Time
	// TotalBytes、FreeBytes、MeasuredAt は、容量の管理が最後に測った容量と空き容量と、測った日時 (#957)。測っていなければ nil。
	TotalBytes *int64
	FreeBytes  *int64
	MeasuredAt *time.Time
}

// Instance は、動いているロールのインスタンス。
type Instance struct {
	ID          uuid.UUID
	Role        string
	Address     string
	StartedAt   time.Time
	HeartbeatAt time.Time
}

// Lease は、リーダー選出のリース。
type Lease struct {
	Name       string
	Holder     uuid.UUID
	Generation int64
	AcquiredAt time.Time
	RenewedAt  time.Time
	ExpiresAt  time.Time
}

// Tuner は、チューナーの構成と状態。
type Tuner struct {
	TunerIndex  int64
	Name        string
	Types       []string
	IsAvailable bool
	IsRemote    bool
	IsFault     bool
}

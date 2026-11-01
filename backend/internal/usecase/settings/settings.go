// Package settings は、画面から変える全体の設定を扱う (設計書 11.2)。
//
// 起動に要るものや秘密情報は YAML と環境変数に置き、家族が日常的に変えたいものを DB に置く。
// ここでは、録画設定の全体の既定値、過去の番組と通知の保持期間、チャンネル種別の表示を扱う。
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/netip"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/reserve"
)

const (
	// DefaultArchiveRetentionDays は、過去の番組の保持期間の既定値 (付録 A)。
	DefaultArchiveRetentionDays = 90
	// DefaultNotificationRetentionDays は、配り終えた通知の保持期間の既定値 (付録 A、#551)。
	DefaultNotificationRetentionDays = 90
	// maxArchiveRetentionDays は、過去の番組と通知の保持期間の上限。無期限にするなら 0 にする。
	maxArchiveRetentionDays = 3650
	// auditTarget は、設定の監査ログの対象の種類。
	auditTarget = "system_settings"
	// maxDuplicateWindowDays は、重複を避ける期間の上限 (10 年)。ルールと同じ。
	maxDuplicateWindowDays = 3650
)

// ErrInvalidSettings は、設定の値が正しくないことを表す。
var ErrInvalidSettings = errors.New("invalid system settings")

// System は、画面から変える全体の設定。
type System struct {
	// Recording は、録画設定の全体の既定値 (設計書 4.7)。Go の既定値に、画面で変えた項目を重ねたもの。
	Recording *domain.RecordingSettings
	// RecordingOverrides は、画面で変えた項目。変えていない項目は nil。
	RecordingOverrides *domain.RecordingOverrides
	// ArchiveRetentionDays は、過去の番組の保持期間 (日)。0 なら無期限。
	ArchiveRetentionDays int
	// NotificationRetentionDays は、配り終えた通知の保持期間 (日)。既読か未読かにかかわらず消す。0 なら無期限。
	NotificationRetentionDays int
	// Rule は、ルールを作るときの既定値 (#470)。
	Rule *RuleDefaults
	// MCPEnabled は、MCP のクライアントに、認可サーバーと MCP サーバーを使わせるかどうか (#963)。既定は false。
	MCPEnabled bool
	// DataBroadcastIPEnabled は、データ放送の文書の双方向の通信を、tenban が中継するかどうか (#501)。既定は false。
	// 中継すると、文書が送る受信機の情報 (郵便番号など) が、放送局などの外部のサーバーに届く。
	DataBroadcastIPEnabled bool
	// AnimeLinkingEnabled は、しょぼいカレンダー、Annict、AniList との連携 (アニメの番組と作品の結びつき) を使うかどうか (設計書 3.8)。
	// 外部へ問い合わせるので、既定は false (設計書 0.3)。
	AnimeLinkingEnabled bool
	// DRCSReplacements は、字幕の外字 (DRCS) を文字に置き換える表 (#825)。外字の画像の MD5 (小文字の 16 進) から文字へ。
	DRCSReplacements map[string]string
	// UpdatedAt は、最後に変えた日時。一度も変えていなければ nil。
	UpdatedAt *time.Time
}

// ArchiveRetention は、過去の番組の保持期間を返す。無期限なら負の値を返す。
func (s *System) ArchiveRetention() time.Duration {
	if s.ArchiveRetentionDays == 0 {
		return -1
	}

	return time.Duration(s.ArchiveRetentionDays) * 24 * time.Hour
}

// NotificationRetention は、配り終えた通知の保持期間を返す。無期限なら負の値を返す。
func (s *System) NotificationRetention() time.Duration {
	if s.NotificationRetentionDays == 0 {
		return -1
	}

	return time.Duration(s.NotificationRetentionDays) * 24 * time.Hour
}

// RuleDefaults は、ルールを作るときの既定値 (#470)。ルールは家族で共有するので、全体の設定に持つ。
type RuleDefaults struct {
	// AvoidDuplicates は、同じ番組名を録画済みなら予約しないかどうか (設計書 4.4)。既定は false。
	AvoidDuplicates bool
	// DuplicateWindowDays は、重複を避ける期間 (日)。nil なら期間を区切らない。
	DuplicateWindowDays *int
	// FollowEventRelay は、イベントリレーで続く番組も予約するかどうか。既定は true。
	FollowEventRelay bool
}

// ruleDefaultsJSON は、DB に保存する、ルールの既定値のうち変えた項目。変えていない項目は nil。
type ruleDefaultsJSON struct {
	AvoidDuplicates     *bool `json:"avoidDuplicates,omitempty"`
	DuplicateWindowDays *int  `json:"duplicateWindowDays,omitempty"`
	FollowEventRelay    *bool `json:"followEventRelay,omitempty"`
}

// RuleChanges は、ルールの既定値の変える項目。nil の項目は変えない。
type RuleChanges struct {
	AvoidDuplicates *bool
	// DuplicateWindowDays は、重複を避ける期間 (1〜3650 日)。ClearDuplicateWindowDays なら、期間を区切らないようにする。
	DuplicateWindowDays      *int
	ClearDuplicateWindowDays bool
	FollowEventRelay         *bool
}

// defaultRuleDefaults は、ルールの既定値の既定値。ルールの入力の既定値 (設計書 4.2) と同じ。
func defaultRuleDefaults() *RuleDefaults {
	return &RuleDefaults{AvoidDuplicates: false, DuplicateWindowDays: nil, FollowEventRelay: true}
}

// Changes は、変える項目。nil の項目は変えない。
type Changes struct {
	// Recording は、録画設定の既定値の変える項目。項目を既定値に戻すときは、既定値と同じ値を渡す。
	Recording                 *domain.RecordingOverrides
	ArchiveRetentionDays      *int
	NotificationRetentionDays *int
	Rule                      *RuleChanges
	MCPEnabled                *bool
	DataBroadcastIPEnabled    *bool
	AnimeLinkingEnabled       *bool
	// DRCSReplacements は、外字の置き換えの表の変更 (#825)。MD5 から、置き換える文字 (nil なら消す) へ。書いていない外字は変えない。
	DRCSReplacements map[string]*string
}

// Load は、全体の設定を返す。一度も変えていなければ、既定値を返す。
func Load(ctx context.Context, db sqlc.DBTX) (*System, error) {
	row, err := sqlc.New(db).GetSystemSettings(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return defaults(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the system settings: %w", err)
	}

	return systemOf(&row)
}

// RecordingDefaults は、録画設定の全体の既定値を返す (assign.DefaultsFunc)。画面から変えられる (設計書 11.2) ので、使うたびに引く。
func RecordingDefaults(ctx context.Context, db sqlc.DBTX) (*domain.RecordingSettings, error) {
	system, err := Load(ctx, db)
	if err != nil {
		return nil, err
	}

	return system.Recording, nil
}

// Store は、全体の設定を変える。変えたことは監査ログに残す。
type Store struct {
	clock    clock.Clock
	recorder audit.Recorder
}

// NewStore は Store を返す。
func NewStore(clk clock.Clock, recorder audit.Recorder) *Store {
	return &Store{clock: clk, recorder: recorder}
}

// Database は、読み出しと、トランザクションでの書き込みに使う DB。*pgxpool.Pool が満たす。
type Database interface {
	sqlc.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Update は、changes の項目を変え、変えた後の設定を返す。
func (s *Store) Update(ctx context.Context, db Database, actor *audit.Actor, changes *Changes) (*System, error) {
	if changes == nil {
		return nil, fmt.Errorf("%w: changes is nil", ErrInvalidSettings)
	}
	if days := changes.ArchiveRetentionDays; days != nil {
		if _, err := retentionDays("archiveRetentionDays", *days); err != nil {
			return nil, err
		}
	}
	if days := changes.NotificationRetentionDays; days != nil {
		if _, err := retentionDays("notificationRetentionDays", *days); err != nil {
			return nil, err
		}
	}
	if err := validateRuleChanges(changes.Rule); err != nil {
		return nil, err
	}

	var updated *System
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		now := s.clock.Now()
		row, err := q.LockSystemSettings(ctx, sqlc.LockSystemSettingsParams{ArchiveRetentionDays: DefaultArchiveRetentionDays, Now: now})
		if err != nil {
			return fmt.Errorf("failed to lock the system settings: %w", err)
		}
		current, err := systemOf(&row)
		if err != nil {
			return err
		}
		overrides := merge(current.RecordingOverrides, changes.Recording)
		encoded, err := reserve.EncodeOverrides(overrides)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidSettings, err)
		}
		archiveDays, err := retentionDays("archiveRetentionDays", valueOr(changes.ArchiveRetentionDays, current.ArchiveRetentionDays))
		if err != nil {
			return err
		}
		notificationDays, err := retentionDays(
			"notificationRetentionDays", valueOr(changes.NotificationRetentionDays, current.NotificationRetentionDays))
		if err != nil {
			return err
		}
		ruleDefaults, err := decodeRuleDefaults(row.RuleDefaults)
		if err != nil {
			return err
		}
		ruleEncoded, err := json.Marshal(mergeRule(ruleDefaults, changes.Rule))
		if err != nil {
			return fmt.Errorf("failed to encode the rule defaults: %w", err)
		}
		replacementsEncoded, err := encodeDRCSReplacements(current.DRCSReplacements, changes.DRCSReplacements)
		if err != nil {
			return err
		}
		saved, err := q.UpdateSystemSettings(ctx, sqlc.UpdateSystemSettingsParams{
			RecordingDefaults: encoded, ArchiveRetentionDays: archiveDays, NotificationRetentionDays: notificationDays,
			RuleDefaults: ruleEncoded, MCPEnabled: valueOr(changes.MCPEnabled, current.MCPEnabled),
			DataBroadcastIPEnabled: valueOr(changes.DataBroadcastIPEnabled, current.DataBroadcastIPEnabled),
			AnimeLinkingEnabled:    valueOr(changes.AnimeLinkingEnabled, current.AnimeLinkingEnabled),
			DrcsReplacements:       replacementsEncoded, Now: now,
		})
		if err != nil {
			return fmt.Errorf("failed to update the system settings: %w", err)
		}
		if updated, err = systemOf(&saved); err != nil {
			return err
		}

		return s.recorder.Record(ctx, tx, &audit.Event{
			Actor:      actor,
			Action:     "settings.updated",
			TargetType: auditTarget,
			TargetID:   "system",
			Summary: map[string]any{
				"recording":                 change(current.RecordingOverrides, updated.RecordingOverrides),
				"archiveRetentionDays":      change(current.ArchiveRetentionDays, updated.ArchiveRetentionDays),
				"notificationRetentionDays": change(current.NotificationRetentionDays, updated.NotificationRetentionDays),
				"rule":                      change(current.Rule, updated.Rule),
				"mcpEnabled":                change(current.MCPEnabled, updated.MCPEnabled),
				"dataBroadcastIpEnabled":    change(current.DataBroadcastIPEnabled, updated.DataBroadcastIPEnabled),
				"animeLinkingEnabled":       change(current.AnimeLinkingEnabled, updated.AnimeLinkingEnabled),
				// 表は大きくなりうるので、監査ログには、変えた外字だけを残す。
				"drcsReplacements": drcsChanges(current.DRCSReplacements, updated.DRCSReplacements, changes.DRCSReplacements),
			},
			// 送信元は、操作者 (actor) に持たせる。
			SourceIP:  netip.Addr{},
			UserAgent: "",
		})
	})
	if err != nil {
		return nil, err
	}

	return updated, nil
}

// change は、監査ログに残す、変える前と後の値。
func change(from, to any) map[string]any {
	return map[string]any{"from": from, "to": to}
}

// retentionDays は、保持期間 name の日数が範囲にあるかを確かめ、DB に保存する形にする。
func retentionDays(name string, days int) (int32, error) {
	if days < 0 || days > maxArchiveRetentionDays {
		return 0, fmt.Errorf("%w: %s must be between 0 and %d", ErrInvalidSettings, name, maxArchiveRetentionDays)
	}

	return int32(days), nil
}

// valueOr は、変える値 change があればそれを、なければ今の値 current を返す。
func valueOr[T any](change *T, current T) T {
	if change != nil {
		return *change
	}

	return current
}

// merge は、base に changes の nil でない項目を重ねる。
func merge(base, changes *domain.RecordingOverrides) *domain.RecordingOverrides {
	merged := *base
	if changes == nil {
		return &merged
	}
	if changes.PoolID != nil {
		merged.PoolID = changes.PoolID
	}
	if changes.FileNameTemplate != nil {
		merged.FileNameTemplate = changes.FileNameTemplate
	}
	if changes.StartMarginSeconds != nil {
		merged.StartMarginSeconds = changes.StartMarginSeconds
	}
	if changes.EndMarginSeconds != nil {
		merged.EndMarginSeconds = changes.EndMarginSeconds
	}
	if changes.Priority != nil {
		merged.Priority = changes.Priority
	}
	if changes.JoinSegments != nil {
		merged.JoinSegments = changes.JoinSegments
	}

	return &merged
}

// validateRuleChanges は、ルールの既定値の変える項目が正しいかを確かめる。
func validateRuleChanges(changes *RuleChanges) error {
	if changes == nil {
		return nil
	}
	if changes.ClearDuplicateWindowDays && changes.DuplicateWindowDays != nil {
		return fmt.Errorf("%w: duplicateWindowDays and clearDuplicateWindowDays cannot be used together", ErrInvalidSettings)
	}
	if days := changes.DuplicateWindowDays; days != nil && (*days < 1 || *days > maxDuplicateWindowDays) {
		return fmt.Errorf("%w: duplicateWindowDays must be between 1 and %d", ErrInvalidSettings, maxDuplicateWindowDays)
	}

	return nil
}

// mergeRule は、保存しているルールの既定値の変えた項目に、changes を重ねる。
func mergeRule(base *ruleDefaultsJSON, changes *RuleChanges) *ruleDefaultsJSON {
	merged := *base
	if changes == nil {
		return &merged
	}
	if changes.AvoidDuplicates != nil {
		merged.AvoidDuplicates = changes.AvoidDuplicates
	}
	if changes.DuplicateWindowDays != nil {
		merged.DuplicateWindowDays = changes.DuplicateWindowDays
	}
	if changes.ClearDuplicateWindowDays {
		merged.DuplicateWindowDays = nil
	}
	if changes.FollowEventRelay != nil {
		merged.FollowEventRelay = changes.FollowEventRelay
	}

	return &merged
}

// decodeRuleDefaults は、DB に保存したルールの既定値を読む。
func decodeRuleDefaults(raw []byte) (*ruleDefaultsJSON, error) {
	var stored ruleDefaultsJSON
	if err := json.Unmarshal(raw, &stored); err != nil {
		return nil, fmt.Errorf("failed to decode the rule defaults: %w", err)
	}

	return &stored, nil
}

// resolveRule は、ルールの既定値の既定値に、保存した項目を重ねる。
func resolveRule(stored *ruleDefaultsJSON) *RuleDefaults {
	resolved := defaultRuleDefaults()
	if stored.AvoidDuplicates != nil {
		resolved.AvoidDuplicates = *stored.AvoidDuplicates
	}
	resolved.DuplicateWindowDays = stored.DuplicateWindowDays
	if stored.FollowEventRelay != nil {
		resolved.FollowEventRelay = *stored.FollowEventRelay
	}

	return resolved
}

func defaults() *System {
	return &System{
		Recording:                 domain.DefaultRecordingSettings(),
		RecordingOverrides:        new(domain.RecordingOverrides),
		ArchiveRetentionDays:      DefaultArchiveRetentionDays,
		NotificationRetentionDays: DefaultNotificationRetentionDays,
		Rule:                      defaultRuleDefaults(),
		MCPEnabled:                false,
		DataBroadcastIPEnabled:    false,
		AnimeLinkingEnabled:       false,
		DRCSReplacements:          map[string]string{},
		UpdatedAt:                 nil,
	}
}

func systemOf(row *sqlc.SystemSetting) (*System, error) {
	overrides, err := domain.DecodeOverrides(row.RecordingDefaults)
	if err != nil {
		return nil, err
	}
	rule, err := decodeRuleDefaults(row.RuleDefaults)
	if err != nil {
		return nil, err
	}

	replacements := map[string]string{}
	if err := json.Unmarshal(row.DrcsReplacements, &replacements); err != nil {
		return nil, fmt.Errorf("failed to decode the DRCS replacements: %w", err)
	}

	return &System{
		DRCSReplacements:          replacements,
		Recording:                 domain.DefaultRecordingSettings().Resolve(overrides),
		RecordingOverrides:        overrides,
		ArchiveRetentionDays:      int(row.ArchiveRetentionDays),
		NotificationRetentionDays: int(row.NotificationRetentionDays),
		Rule:                      resolveRule(rule),
		MCPEnabled:                row.MCPEnabled,
		DataBroadcastIPEnabled:    row.DataBroadcastIPEnabled,
		AnimeLinkingEnabled:       row.AnimeLinkingEnabled,
		UpdatedAt:                 &row.UpdatedAt,
	}, nil
}

const (
	// maxDRCSReplacements は、外字の置き換えの表に書ける数の上限 (#825)。
	maxDRCSReplacements = 2000
	// maxDRCSReplacementLength は、外字を置き換える文字の長さ (文字数) の上限。aribb24.js は、置き換えた文字を外字の 1 文字の枠に縮めて描くので、長いと読めない。
	maxDRCSReplacementLength = 4
)

// encodeDRCSReplacements は、外字の置き換えの表に changes を当てたあとの、保存する JSON を返す。
// changes は、MD5 から、置き換える文字 (nil なら表から消す) への表。書いていない外字は変えない。
// 表を丸ごと送らせないのは、別の画面や管理者が同時に登録したものを、古い表で上書きして消さないためである。
func encodeDRCSReplacements(current map[string]string, changes map[string]*string) ([]byte, error) {
	replacements := maps.Clone(current)
	if replacements == nil {
		replacements = map[string]string{}
	}
	for md5, text := range changes {
		key := strings.ToLower(md5)
		if len(key) != 32 || strings.Trim(key, "0123456789abcdef") != "" {
			return nil, fmt.Errorf("%w: %q is not an MD5 hash", ErrInvalidSettings, md5)
		}
		if text == nil {
			delete(replacements, key)

			continue
		}
		if n := utf8.RuneCountInString(*text); n == 0 || n > maxDRCSReplacementLength {
			return nil, fmt.Errorf("%w: the replacement of %s must be 1 to %d characters", ErrInvalidSettings, key, maxDRCSReplacementLength)
		}
		replacements[key] = *text
	}
	if len(replacements) > maxDRCSReplacements {
		return nil, fmt.Errorf("%w: drcsReplacements must have at most %d entries", ErrInvalidSettings, maxDRCSReplacements)
	}
	encoded, err := json.Marshal(replacements)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the DRCS replacements: %w", err)
	}

	return encoded, nil
}

// drcsChanges は、監査ログに残す、変えた外字の、変える前と後の文字。消した外字は、後を空にする。
func drcsChanges(current, updated map[string]string, changes map[string]*string) map[string]any {
	result := make(map[string]any, len(changes))
	for md5 := range changes {
		key := strings.ToLower(md5)
		result[key] = change(current[key], updated[key])
	}

	return result
}

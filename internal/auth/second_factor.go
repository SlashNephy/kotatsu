package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/secretbox"
	"github.com/SlashNephy/kotatsu/internal/totp"
)

const (
	// TOTPPurpose は、TOTP の秘密キーを暗号化する鍵を、アプリの秘密キーから導出するときの用途。
	TOTPPurpose = "kotatsu/totp/v1"
	// totpIssuer は、認証アプリに出す発行者の名前。
	totpIssuer = "Kotatsu"
	// enrollmentTTL は、TOTP の登録を始めてから、コードで確かめるまでに許す時間 (付録 A)。
	enrollmentTTL = 10 * time.Minute
	// RecoveryCodeCount は、一度に作るリカバリーコードの数 (付録 A)。
	RecoveryCodeCount = 10
	// recoveryCodeBytes は、リカバリーコードの乱数の長さ。Base32 の 10 文字 (50 ビット) になる。
	recoveryCodeBytes = 7
	recoveryCodeChars = 10
)

// SecondFactor は、第二要素の種類。
type SecondFactor string

const (
	SecondFactorTOTP         SecondFactor = "totp"
	SecondFactorRecoveryCode SecondFactor = "recovery_code"
)

var (
	// ErrInvalidSecondFactor は、TOTP のコードやリカバリーコードが違う、使用済み、のどれかを表す。
	ErrInvalidSecondFactor = errors.New("invalid second factor")
	// ErrNoTOTPEnrollment は、TOTP の登録を始めていないか、期限が切れたことを表す。
	ErrNoTOTPEnrollment = errors.New("no pending TOTP enrollment")
	// ErrNoTOTP は、TOTP を登録していないことを表す。
	ErrNoTOTP = errors.New("TOTP is not enabled")
)

// TOTPEnrollment は、登録中の TOTP。秘密キーは、ここでしか見せない。
type TOTPEnrollment struct {
	// Secret は、認証アプリに手で入力する形の秘密キー。
	Secret string
	// URI は、認証アプリが QR コードから読む otpauth URI。
	URI       string
	ExpiresAt time.Time
}

// SecondFactors は、第二要素 (TOTP とリカバリーコード) を扱う (設計書 9.3)。パスキーは Passkeys が扱う。
type SecondFactors struct {
	clock    clock.Clock
	recorder audit.Recorder
	// box は、TOTP の秘密キーを暗号化する。アプリの秘密キーから導出した鍵を使う (設計書 8.4)。
	box *secretbox.Box
}

// NewSecondFactors は、アプリの秘密キー appSecret で TOTP の秘密キーを暗号化する SecondFactors を返す。
func NewSecondFactors(clk clock.Clock, recorder audit.Recorder, appSecret string) (*SecondFactors, error) {
	box, err := secretbox.New(appSecret, TOTPPurpose)
	if err != nil {
		return nil, err
	}

	return &SecondFactors{clock: clk, recorder: recorder, box: box}, nil
}

// Methods は、ユーザーが使える第二要素の種類を返す。登録していなければ空を返す。
func (s *SecondFactors) Methods(ctx context.Context, db sqlc.DBTX, userID uuid.UUID) ([]SecondFactor, error) {
	q := sqlc.New(db)
	methods := []SecondFactor{}
	hasTOTP, err := q.HasUserTOTP(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get the TOTP: %w", err)
	}
	if hasTOTP {
		methods = append(methods, SecondFactorTOTP)
	}
	passkeys, err := q.CountUserPasskeys(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to count passkeys: %w", err)
	}
	if passkeys > 0 {
		methods = append(methods, SecondFactorPasskey)
	}
	codes, err := q.CountUnusedRecoveryCodes(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to count recovery codes: %w", err)
	}
	if codes > 0 {
		methods = append(methods, SecondFactorRecoveryCode)
	}

	return methods, nil
}

// Status は、ユーザーが TOTP を登録しているかと、使っていないリカバリーコードの数を返す。
func (s *SecondFactors) Status(ctx context.Context, db sqlc.DBTX, userID uuid.UUID) (bool, int, error) {
	q := sqlc.New(db)
	hasTOTP, err := q.HasUserTOTP(ctx, userID)
	if err != nil {
		return false, 0, fmt.Errorf("failed to get the TOTP: %w", err)
	}
	codes, err := q.CountUnusedRecoveryCodes(ctx, userID)
	if err != nil {
		return false, 0, fmt.Errorf("failed to count recovery codes: %w", err)
	}

	return hasTOTP, int(codes), nil
}

// BeginTOTP は、TOTP の登録を始め、新しい秘密キーを返す。ConfirmTOTP でコードを確かめるまで、有効にはならない。
// 登録済みの TOTP があっても、確かめるまでは古い TOTP がそのまま使える。
func (s *SecondFactors) BeginTOTP(ctx context.Context, db sqlc.DBTX, userID uuid.UUID, account string) (*TOTPEnrollment, error) {
	secret, err := totp.NewSecret()
	if err != nil {
		return nil, err
	}
	sealed, err := s.box.Seal(secret)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	expiresAt := now.Add(enrollmentTTL)
	err = sqlc.New(db).UpsertTOTPEnrollment(ctx, sqlc.UpsertTOTPEnrollmentParams{
		UserID: userID, SecretEncrypted: sealed, Now: now, ExpiresAt: expiresAt,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start the TOTP enrollment: %w", err)
	}

	return &TOTPEnrollment{Secret: totp.Encode(secret), URI: totp.URI(totpIssuer, account, secret), ExpiresAt: expiresAt}, nil
}

// ConfirmTOTP は、登録中の TOTP のコードを確かめて有効にし、新しいリカバリーコードを返す。
// 既に TOTP があれば置き換え、古いリカバリーコードは使えなくする。tx の中で呼ぶ。
func (s *SecondFactors) ConfirmTOTP(ctx context.Context, tx pgx.Tx, actor *audit.Actor, userID uuid.UUID, code string) ([]string, error) {
	q := sqlc.New(tx)
	now := s.clock.Now()
	enrollment, err := q.GetTOTPEnrollmentForUpdate(ctx, sqlc.GetTOTPEnrollmentForUpdateParams{UserID: userID, Now: now})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoTOTPEnrollment
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the TOTP enrollment: %w", err)
	}
	secret, err := s.box.Open(enrollment.SecretEncrypted)
	if err != nil {
		return nil, err
	}
	step, ok, err := totp.Verify(secret, code, now, 0)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrInvalidSecondFactor
	}

	err = q.UpsertUserTOTP(ctx, sqlc.UpsertUserTOTPParams{UserID: userID, SecretEncrypted: enrollment.SecretEncrypted, LastUsedStep: step, Now: now})
	if err != nil {
		return nil, fmt.Errorf("failed to enable the TOTP: %w", err)
	}
	if err := q.DeleteTOTPEnrollment(ctx, userID); err != nil {
		return nil, fmt.Errorf("failed to finish the TOTP enrollment: %w", err)
	}
	codes, err := s.replaceRecoveryCodes(ctx, q, userID, now)
	if err != nil {
		return nil, err
	}

	return codes, s.record(ctx, tx, actor, "user.totp_enabled", userID)
}

// VerifyTOTP は、TOTP のコードを確かめる。一度受け付けたコードは、二度と受け付けない。tx の中で呼ぶ。
func (s *SecondFactors) VerifyTOTP(ctx context.Context, tx pgx.Tx, userID uuid.UUID, code string) error {
	q := sqlc.New(tx)
	row, err := q.GetUserTOTPForUpdate(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidSecondFactor
	}
	if err != nil {
		return fmt.Errorf("failed to get the TOTP: %w", err)
	}
	secret, err := s.box.Open(row.SecretEncrypted)
	if err != nil {
		return err
	}
	step, ok, err := totp.Verify(secret, code, s.clock.Now(), row.LastUsedStep)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidSecondFactor
	}
	if err := q.SetTOTPLastUsedStep(ctx, sqlc.SetTOTPLastUsedStepParams{UserID: userID, LastUsedStep: step}); err != nil {
		return fmt.Errorf("failed to record the TOTP use: %w", err)
	}

	return nil
}

// UseRecoveryCode は、リカバリーコードを確かめ、使用済みにする。どのコードも 1 回だけ使える。
func (s *SecondFactors) UseRecoveryCode(ctx context.Context, db sqlc.DBTX, userID uuid.UUID, code string) error {
	n, err := sqlc.New(db).UseRecoveryCode(ctx, sqlc.UseRecoveryCodeParams{
		UserID: userID, CodeHash: hashRecoveryCode(code), Now: new(s.clock.Now()),
	})
	if err != nil {
		return fmt.Errorf("failed to use the recovery code: %w", err)
	}
	if n == 0 {
		return ErrInvalidSecondFactor
	}

	return nil
}

// DeleteTOTP は、TOTP を消す。リカバリーコードは TOTP の予備なので、あわせて消す。
//
// 第二要素がなくなると、次にパスワードでログインしたときに、TOTP を登録し直すことになる (設計書 9.3)。
func (s *SecondFactors) DeleteTOTP(ctx context.Context, tx pgx.Tx, actor *audit.Actor, userID uuid.UUID) error {
	q := sqlc.New(tx)
	n, err := q.DeleteUserTOTP(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to delete the TOTP: %w", err)
	}
	if n == 0 {
		return ErrNoTOTP
	}
	if err := q.DeleteRecoveryCodes(ctx, userID); err != nil {
		return fmt.Errorf("failed to delete recovery codes: %w", err)
	}

	return s.record(ctx, tx, actor, "user.totp_disabled", userID)
}

// RegenerateRecoveryCodes は、リカバリーコードを作り直す。古いコードは使えなくなる。TOTP を登録していなければ作らない。
func (s *SecondFactors) RegenerateRecoveryCodes(ctx context.Context, tx pgx.Tx, actor *audit.Actor, userID uuid.UUID) ([]string, error) {
	q := sqlc.New(tx)
	// TOTP の行をロックし、同時に TOTP を消されて、TOTP のないリカバリーコードが残らないようにする。
	_, err := q.GetUserTOTPForUpdate(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoTOTP
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the TOTP: %w", err)
	}
	codes, err := s.replaceRecoveryCodes(ctx, q, userID, s.clock.Now())
	if err != nil {
		return nil, err
	}

	return codes, s.record(ctx, tx, actor, "user.recovery_codes_regenerated", userID)
}

func (s *SecondFactors) replaceRecoveryCodes(ctx context.Context, q *sqlc.Queries, userID uuid.UUID, now time.Time) ([]string, error) {
	if err := q.DeleteRecoveryCodes(ctx, userID); err != nil {
		return nil, fmt.Errorf("failed to delete recovery codes: %w", err)
	}
	codes := make([]string, 0, RecoveryCodeCount)
	for range RecoveryCodeCount {
		code, err := newRecoveryCode()
		if err != nil {
			return nil, err
		}
		err = q.InsertRecoveryCode(ctx, sqlc.InsertRecoveryCodeParams{ID: id.New(), UserID: userID, CodeHash: hashRecoveryCode(code), Now: now})
		if err != nil {
			return nil, fmt.Errorf("failed to save a recovery code: %w", err)
		}
		codes = append(codes, code)
	}

	return codes, nil
}

func (s *SecondFactors) record(ctx context.Context, tx pgx.Tx, actor *audit.Actor, action string, userID uuid.UUID) error {
	return s.recorder.Record(ctx, tx, &audit.Event{
		Actor:      actor,
		Action:     action,
		TargetType: loginAuditTarget,
		TargetID:   userID.String(),
		Summary:    map[string]any{},
		// 送信元は、操作者 (actor) に持たせる。
		SourceIP:  netip.Addr{},
		UserAgent: "",
	})
}

// newRecoveryCode は、xxxxx-xxxxx の形のリカバリーコードを作る。
func newRecoveryCode() (string, error) {
	raw := make([]byte, recoveryCodeBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("failed to generate a recovery code: %w", err)
	}
	code := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw))[:recoveryCodeChars]

	return code[:recoveryCodeChars/2] + "-" + code[recoveryCodeChars/2:], nil
}

// hashRecoveryCode は、リカバリーコードのハッシュを返す。区切りの記号と大文字と小文字は区別しない。
func hashRecoveryCode(code string) []byte {
	normalized := strings.ToLower(strings.NewReplacer("-", "", " ", "").Replace(code))
	sum := sha256.Sum256([]byte(normalized))

	return sum[:]
}

package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/lib/password"
)

const (
	// ChallengeCookieName は、第二要素の確認を待つログインのトークンを入れる Cookie の名前。
	ChallengeCookieName = "kotatsu_login"
	// challengeCookiePath は、第二要素の確認を待つログインの Cookie を送るパス。/auth/* にだけ送る。
	challengeCookiePath = "/auth/"
	// DefaultChallengeTTL は、パスワードの確認から第二要素の確認までに許す時間 (付録 A)。
	DefaultChallengeTTL = 5 * time.Minute

	// ログインの試行回数の制限 (設計書 9.5、付録 A)。
	// 失敗が free 回を超えると、throttleBase から倍々に待たせ、throttleMax で頭打ちにする。永久にはロックしない。
	// IP 単位は、家族が同じ回線から入力を間違えることもあるので、アカウント単位より多く許す。
	accountFreeFailures = 5
	ipFreeFailures      = 20
	throttleBase        = time.Second
	// concurrentAttemptWait は、同じアカウントか IP の試行を確かめている間に来た試行を、待たせる時間。
	concurrentAttemptWait = time.Second
	throttleMax           = 15 * time.Minute
	// throttleReset は、最後の失敗からこれだけ経つと、失敗の回数を数え直す時間。
	throttleReset = 24 * time.Hour

	// loginAuditTarget は、ログインの監査ログの対象の種類。
	loginAuditTarget = "user"
	// summaryMethod は、ログインの監査ログの要約で、認証に使った方式を入れる項目。
	summaryMethod = "method"
	// summaryUsername は、監査ログの要約で、ユーザー名を入れる項目。
	summaryUsername = "username"
	// summaryRole は、監査ログの要約で、ロールを入れる項目。
	summaryRole = "role"
	// actionLoginFailed は、ログインに失敗したことを表す監査ログの操作。
	actionLoginFailed = "user.login_failed"
)

var (
	// ErrInvalidCredentials は、ユーザー名かパスワードが違うことを表す。
	// ユーザーがいない、無効になった、パスワードを持たない、パスワードが違う、のどれかを区別しない。
	ErrInvalidCredentials = errors.New("invalid username or password")
	// ErrThrottled は、試行回数の制限で、ログインを受け付けないことを表す。errors.As で *ThrottledError を取り出せる。
	ErrThrottled = errors.New("too many login attempts")
)

var _ error = new(ThrottledError)

// ThrottledError は、次に試せるまでの時間を持つ、試行回数の制限のエラー。
type ThrottledError struct {
	RetryAfter time.Duration
}

func (e *ThrottledError) Error() string {
	return fmt.Sprintf("%s: retry after %s", ErrThrottled, e.RetryAfter)
}

func (e *ThrottledError) Unwrap() error {
	return ErrThrottled
}

// throttleDelay は、失敗を failures 回重ねたときの待ち時間を返す。free 回までは待たせない。
func throttleDelay(failures, free int) time.Duration {
	if failures <= free {
		return 0
	}
	delay := throttleBase
	for range failures - free - 1 {
		delay *= 2
		if delay >= throttleMax {
			return throttleMax
		}
	}

	return delay
}

// Challenge は、パスワードの確認を終え、第二要素の確認を待っているログイン。
type Challenge struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	ExpiresAt time.Time
}

// LoginsOptions は Logins の設定。
type LoginsOptions struct {
	Clock    clock.Clock
	Recorder audit.Recorder
	Logger   *slog.Logger
	// Sessions は、第二要素まで確かめたログインのセッションを作る。
	Sessions *Sessions
	// SecondFactors は、第二要素を確かめる。
	SecondFactors *SecondFactors
	// ChallengeTTL は、パスワードの確認から第二要素の確認までに許す時間。0 なら DefaultChallengeTTL にする。
	ChallengeTTL time.Duration
	// MaxConcurrentVerifications は、パスワードを同時に確かめる数の上限。0 なら CPU の数にする。
	MaxConcurrentVerifications int
	// VerificationWait は、確かめる順番を待つ時間の上限。過ぎたら試行を待たせる。0 なら DefaultVerificationWait にする。
	VerificationWait time.Duration
}

// DefaultVerificationWait は、パスワードを確かめる順番を待つ時間の上限の既定値。
const DefaultVerificationWait = 3 * time.Second

// Logins は、パスワードでのログインを扱う (設計書 9.3、9.5)。
type Logins struct {
	clock        clock.Clock
	recorder     audit.Recorder
	logger       *slog.Logger
	sessions     *Sessions
	factors      *SecondFactors
	challengeTTL time.Duration
	// verifications は、パスワードを確かめている数。Argon2id は 1 回で約 19 MiB のメモリと CPU を使うので、上限を設ける (#542)。
	verifications    chan struct{}
	verificationWait time.Duration
}

// NewLogins は Logins を返す。
func NewLogins(opts *LoginsOptions) *Logins {
	limit := opts.MaxConcurrentVerifications
	if limit <= 0 {
		limit = runtime.NumCPU()
	}
	l := &Logins{
		clock: opts.Clock, recorder: opts.Recorder, logger: opts.Logger,
		sessions: opts.Sessions, factors: opts.SecondFactors, challengeTTL: opts.ChallengeTTL,
		verifications: make(chan struct{}, limit), verificationWait: opts.VerificationWait,
	}
	if l.challengeTTL <= 0 {
		l.challengeTTL = DefaultChallengeTTL
	}
	if l.verificationWait <= 0 {
		l.verificationWait = DefaultVerificationWait
	}

	return l
}

// Beginner はトランザクションを始められるもの。*pgxpool.Pool が満たす。
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// throttleKeys は、試行回数を数える単位を返す。存在しないユーザー名も、名前ごとに数える。
func throttleKeys(username string, sourceIP netip.Addr) []string {
	keys := []string{"account:" + strings.ToLower(username)}
	if sourceIP.IsValid() {
		keys = append(keys, "ip:"+sourceIP.String())
	}

	return keys
}

// Password は、ユーザー名とパスワードを確かめる。
//
// 確かめられても、セッションは作らない。パスワードでのログインには第二要素が必須なので (設計書 9.3)、
// 第二要素の確認を待つ Challenge を作り、そのトークンを返す。第二要素を確かめるとセッションになる。
//
// 失敗は、アカウント単位と IP 単位で数え、続くと待ち時間を指数的に延ばす。
// ユーザーがいなくても、パスワードの検証と同じだけ時間をかけ、ユーザー名が存在するかを推測させない。
func (l *Logins) Password(ctx context.Context, db Database, username, secret string, client *ClientInfo) (string, *Challenge, error) {
	now := l.clock.Now()
	keys := throttleKeys(username, client.SourceIP)
	var (
		token     string
		challenge *Challenge
		invalid   bool
	)
	// 制限のキーのロック、待ち時間の確認、パスワードの検証、失敗の記録を、1 つのトランザクションで行う (#542)。
	// 検証の間はロックを持つので、同じアカウントか IP の次の試行は、この試行の失敗を見てから判定される。
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		if err := lockThrottleKeys(ctx, tx, keys); err != nil {
			return err
		}
		if err := checkThrottleIn(ctx, tx, keys, now); err != nil {
			return err
		}

		user, err := sqlc.New(tx).GetUserByUsername(ctx, username)
		usable := err == nil && user.DisabledAt == nil && user.PasswordHash != nil
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("failed to get the user: %w", err)
		}
		var hash *string
		if usable {
			hash = user.PasswordHash
		}
		verification, err := l.verifyPassword(ctx, secret, hash)
		if err != nil {
			return err
		}

		if !verification.OK {
			var userID *uuid.UUID
			if usable {
				userID = &user.ID
			}
			invalid = true

			// 失敗は、ロールバックせずに数えて残す。
			return l.recordFailure(ctx, tx, keys, userID, username, client, now, actionLoginFailed, "password")
		}

		var rehashed *string
		if verification.NeedsRehash {
			encoded, err := password.Hash(secret)
			if err != nil {
				return err
			}
			rehashed = &encoded
		}
		token, challenge, err = l.succeed(ctx, tx, &user, rehashed, client, now)

		return err
	})
	if err != nil {
		return "", nil, err
	}
	if invalid {
		return "", nil, ErrInvalidCredentials
	}

	return token, challenge, nil
}

// verifyPassword は、パスワードがハッシュに一致するかを確かめる。encoded が nil なら、同じだけ時間をかけて一致しないとする。
// 同時に確かめる数を抑え、順番を待ちきれなければ ThrottledError を返す (#542)。
func (l *Logins) verifyPassword(ctx context.Context, secret string, encoded *string) (*password.Verification, error) {
	// 空きがあると、select は取り消した要求でも順番を取りうるので、先に確かめる。
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("failed to wait for a password verification: %w", err)
	}
	timer := time.NewTimer(l.verificationWait)
	defer timer.Stop()
	select {
	case l.verifications <- struct{}{}:
	case <-timer.C:
		return nil, &ThrottledError{RetryAfter: concurrentAttemptWait}
	case <-ctx.Done():
		return nil, fmt.Errorf("failed to wait for a password verification: %w", ctx.Err())
	}
	defer func() { <-l.verifications }()

	if encoded == nil {
		if err := password.VerifyDummy(secret); err != nil {
			return nil, err
		}

		return &password.Verification{OK: false, NeedsRehash: false}, nil
	}

	return password.Verify(secret, *encoded)
}

// lockThrottleKeys は、試行回数の制限のキーを、トランザクションの終わりまでロックする。
// ほかの試行がロックしていれば、待たずに ThrottledError を返す。待たせると、同時の試行が DB の接続を抱えたまま並ぶからである。
func lockThrottleKeys(ctx context.Context, tx pgx.Tx, keys []string) error {
	for _, key := range keys {
		locked, err := sqlc.New(tx).TryLockLoginKey(ctx, key)
		if err != nil {
			return fmt.Errorf("failed to lock the login throttle: %w", err)
		}
		if !locked {
			return &ThrottledError{RetryAfter: concurrentAttemptWait}
		}
	}

	return nil
}

// checkThrottleIn は、トランザクション tx の中で、待ち時間の中なら ThrottledError を返す。
func checkThrottleIn(ctx context.Context, tx pgx.Tx, keys []string, now time.Time) error {
	throttles, err := sqlc.New(tx).GetLoginThrottles(ctx, keys)
	if err != nil {
		return fmt.Errorf("failed to get the login throttles: %w", err)
	}
	var wait time.Duration
	for _, throttle := range throttles {
		wait = max(wait, throttle.LockedUntil.Sub(now))
	}
	if wait > 0 {
		return &ThrottledError{RetryAfter: wait}
	}

	return nil
}

// recordFailure は、失敗を数えて待ち時間を決め、action と、試した方式 method を監査ログに残す。
func (l *Logins) recordFailure(
	ctx context.Context,
	tx pgx.Tx,
	keys []string,
	userID *uuid.UUID,
	username string,
	client *ClientInfo,
	now time.Time,
	action, method string,
) error {
	q := sqlc.New(tx)
	if _, err := q.DeleteStaleLoginThrottles(ctx, sqlc.DeleteStaleLoginThrottlesParams{ResetBefore: now.Add(-throttleReset), Now: now}); err != nil {
		return fmt.Errorf("failed to delete stale login throttles: %w", err)
	}
	for _, key := range keys {
		failures, err := q.RecordLoginFailure(ctx, sqlc.RecordLoginFailureParams{Key: key, Now: now, ResetBefore: now.Add(-throttleReset)})
		if err != nil {
			return fmt.Errorf("failed to record the login failure: %w", err)
		}
		free := accountFreeFailures
		if strings.HasPrefix(key, "ip:") {
			free = ipFreeFailures
		}
		delay := throttleDelay(int(failures), free)
		if err := q.SetLoginLockedUntil(ctx, sqlc.SetLoginLockedUntilParams{Key: key, LockedUntil: now.Add(delay)}); err != nil {
			return fmt.Errorf("failed to throttle logins: %w", err)
		}
	}

	// ユーザーがいなければ、操作者は tenban にし、試したユーザー名を要約に残す。
	actor, targetID := audit.System("tenban"), ""
	if userID != nil {
		actor, targetID = audit.User(*userID), userID.String()
	}

	return l.recorder.Record(ctx, tx, &audit.Event{
		Actor:      actor,
		Action:     action,
		TargetType: loginAuditTarget,
		TargetID:   targetID,
		Summary:    map[string]any{summaryUsername: username, summaryMethod: method},
		SourceIP:   client.SourceIP,
		UserAgent:  client.UserAgent,
	})
}

// succeed は、第二要素の確認を待つ Challenge を作る。
// アカウント単位の失敗の回数は、第二要素まで確かめ終えてから消す (finish)。ここで消すと、パスワードの確認から
// やり直すたびに、第二要素を試せる回数が戻り、第二要素のコードを総当たりできてしまう。
// rehashed が nil でなければ、今のパラメーターで作り直したハッシュに置き換える。
func (l *Logins) succeed(
	ctx context.Context,
	tx pgx.Tx,
	user *sqlc.User,
	rehashed *string,
	client *ClientInfo,
	now time.Time,
) (string, *Challenge, error) {
	q := sqlc.New(tx)
	if rehashed != nil {
		if _, err := q.SetUserPassword(ctx, sqlc.SetUserPasswordParams{ID: user.ID, PasswordHash: rehashed, Now: now}); err != nil {
			return "", nil, fmt.Errorf("failed to rehash the password: %w", err)
		}
	}
	if _, err := q.DeleteExpiredLoginChallenges(ctx, now); err != nil {
		return "", nil, fmt.Errorf("failed to delete expired login challenges: %w", err)
	}

	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("failed to generate a login token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	challenge := &Challenge{ID: id.New(), UserID: user.ID, ExpiresAt: now.Add(l.challengeTTL)}
	var sourceIP *netip.Addr
	if client.SourceIP.IsValid() {
		sourceIP = &client.SourceIP
	}
	err := q.InsertLoginChallenge(ctx, sqlc.InsertLoginChallengeParams{
		ID:        challenge.ID,
		TokenHash: hashToken(token),
		UserID:    user.ID,
		CreatedAt: now,
		ExpiresAt: challenge.ExpiresAt,
		UserAgent: client.UserAgent,
		SourceIP:  sourceIP,
	})
	if err != nil {
		return "", nil, fmt.Errorf("failed to create a login challenge: %w", err)
	}

	err = l.recorder.Record(ctx, tx, &audit.Event{
		Actor:      audit.User(user.ID),
		Action:     "user.password_verified",
		TargetType: loginAuditTarget,
		TargetID:   user.ID.String(),
		Summary:    map[string]any{"rehashed": rehashed != nil},
		SourceIP:   client.SourceIP,
		UserAgent:  client.UserAgent,
	})
	if err != nil {
		return "", nil, err
	}

	return token, challenge, nil
}

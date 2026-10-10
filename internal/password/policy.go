package password

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// MinLength は、パスワードの最小の長さ (文字数、付録 A)。
	// パスワードでのログインには第二要素が必須なので、NIST SP 800-63B-4 が第二要素と組み合わせる場合に認める 8 文字にする。
	MinLength = 8
	// MaxLength は、パスワードの最大の長さ (文字数、付録 A)。NIST の求める 64 文字以上を許し、
	// Argon2id に渡す入力が大きくなりすぎないよう上限を設ける。
	MaxLength = 256
	// MinScore は、zxcvbn-ts のスコアの閾値 (付録 A)。
	// 2 は、試行回数を制限したオンラインの攻撃に耐える強さ (推定試行回数 10^6 以上) である。
	// パスワードでのログインには第二要素が必須で、試行回数も制限するので、この強さで足りる。
	// 3 にすると、無作為な 8 文字でもほとんど拒否され、最小の長さを 8 文字とした意図と食い違う。
	MinScore = 2
)

// Reason は、パスワードを拒否した理由。画面は、理由ごとに案内を出し分ける。
type Reason string

const (
	ReasonTooShort    Reason = "too_short"
	ReasonTooLong     Reason = "too_long"
	ReasonBlocklisted Reason = "blocklisted"
	ReasonBreached    Reason = "breached"
	ReasonTooWeak     Reason = "too_weak"
)

// ErrWeakPassword は、パスワードが規定を満たさないことを表す。errors.As で RejectedError を取り出せる。
var ErrWeakPassword = errors.New("the password does not meet the policy")

var _ error = new(RejectedError)

// RejectedError は、パスワードを拒否した理由を持つエラー。
type RejectedError struct {
	Reason Reason
	// Strength は、zxcvbn-ts で拒否したときの判定の結果。ほかの理由では nil。
	Strength *Strength
}

func (e *RejectedError) Error() string {
	return fmt.Sprintf("%s: %s", ErrWeakPassword, e.Reason)
}

func (e *RejectedError) Unwrap() error {
	return ErrWeakPassword
}

// Policy は、パスワードの規定 (設計書 9.3)。
type Policy struct {
	blocklist map[string]struct{}
	estimator StrengthEstimator
	// hibp は、Have I Been Pwned への照会。nil なら照会しない。
	hibp   *HIBP
	logger *slog.Logger
}

// PolicyOptions は Policy の設定。
type PolicyOptions struct {
	// HIBP は、Have I Been Pwned への照会。nil なら照会しない。
	HIBP *HIBP
	// StrengthTimeout は、zxcvbn-ts の 1 回の判定の時間の上限。0 なら DefaultStrengthTimeout にする。
	StrengthTimeout time.Duration
	// Estimator は、強度を判定するもの。nil なら zxcvbn-ts で判定する。
	// zxcvbn-ts は -race で 1 回に数十秒かかるので、強度の判定そのものを確かめないテストで差し替える。
	Estimator StrengthEstimator
	Logger    *slog.Logger
}

// NewPolicy は、埋め込んだブロックリストと zxcvbn-ts を読み込んで Policy を返す。
func NewPolicy(opts *PolicyOptions) (*Policy, error) {
	blocklist, err := loadBlocklist()
	if err != nil {
		return nil, err
	}
	estimator := opts.Estimator
	if estimator == nil {
		estimator = NewEstimator(opts.StrengthTimeout)
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	return &Policy{blocklist: blocklist, estimator: estimator, hibp: opts.HIBP, logger: logger}, nil
}

// Check は、パスワードが規定を満たすかを確かめる。満たさなければ *RejectedError を返す。
// userInputs には、ユーザー名や表示名のように、パスワードに使われると推測しやすい語を渡す。
//
// 判定は、安く済むものから順に行う: 長さ、同梱のブロックリスト、zxcvbn-ts、Have I Been Pwned。
// Have I Been Pwned に照会できなかったときは、パスワードの設定を止めずに、警告を残して通す。
// 照会は任意の追加の確認で、同梱のブロックリストと zxcvbn-ts の判定は済んでいるからである。
func (p *Policy) Check(ctx context.Context, password string, userInputs []string) error {
	length := utf8.RuneCountInString(password)
	if length < MinLength {
		return &RejectedError{Reason: ReasonTooShort, Strength: nil}
	}
	if length > MaxLength {
		return &RejectedError{Reason: ReasonTooLong, Strength: nil}
	}
	// ブロックリストは小文字で持つので、大文字と小文字を区別せずに照らす。
	if _, listed := p.blocklist[strings.ToLower(password)]; listed {
		return &RejectedError{Reason: ReasonBlocklisted, Strength: nil}
	}
	strength, err := p.estimator.Estimate(ctx, password, userInputs)
	if err != nil {
		return err
	}
	if strength.Score < MinScore {
		return &RejectedError{Reason: ReasonTooWeak, Strength: strength}
	}
	if p.hibp == nil {
		return nil
	}
	breached, err := p.hibp.Breached(ctx, password)
	if err != nil {
		p.logger.WarnContext(ctx, "skipped the breached password check", slog.Any("error", err))

		return nil
	}
	if breached {
		return &RejectedError{Reason: ReasonBreached, Strength: nil}
	}

	return nil
}

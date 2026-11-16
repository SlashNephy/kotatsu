package drama

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/secretbox"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
)

const (
	// sealPurpose は、TMDb のトークンを暗号化する用途 (設計書 8.4)。
	sealPurpose = "kotatsu/tmdb-token/v1"
	// tokenAuditTarget は、トークンを変えたことを監査ログに残すときの対象の種類。
	tokenAuditTarget = "tmdb_token"
	// maxTokenLength は、管理者が入れるトークンの長さの上限。TMDb の API の読み取りアクセストークンは、200 文字ほどの JWT。
	maxTokenLength = 2048
	// tokenHintLength は、設定したトークンを見分けるために画面に出す、末尾の文字数。
	tokenHintLength = 4
	// minHintedTokenLength は、末尾を画面に出すトークンの長さの下限。短いトークンは、末尾だけでも大きな手がかりになるので出さない。
	minHintedTokenLength = 32
)

// ErrInvalidToken は、管理者が入れた TMDb のトークンが正しくないことを表す。
var ErrInvalidToken = errors.New("invalid TMDb token")

// TokenStatus は、TMDb のトークンの様子。トークンそのものは持たない。
type TokenStatus struct {
	Configured bool
	// Hint は、設定したトークンの末尾の数文字。設定していないか、トークンが短ければ空。
	Hint      string
	UpdatedAt *time.Time
}

// Tokens は、管理者が連携の画面で入れた TMDb の API の読み取りアクセストークンを、アプリの秘密キーで暗号化して DB に持つ
// (#1651、設計書 3.9、8.4)。tenban が画面からの変更を受け、nekonote の照合のジョブが、照合のたびに読む。
type Tokens struct {
	box      *secretbox.Box
	clock    clock.Clock
	recorder audit.Recorder
}

var _ TokenSource = new(Tokens)

// NewTokens は Tokens を返す。appSecret は、アプリの秘密キー (auth.secret_key)。
func NewTokens(clk clock.Clock, recorder audit.Recorder, appSecret string) (*Tokens, error) {
	box, err := secretbox.New(appSecret, sealPurpose)
	if err != nil {
		return nil, err
	}

	return &Tokens{box: box, clock: clk, recorder: recorder}, nil
}

// Status は、トークンを設定したかどうかと、見分けるための末尾の数文字を返す。
func (t *Tokens) Status(ctx context.Context, db sqlc.DBTX) (*TokenStatus, error) {
	row, err := sqlc.New(db).GetTMDbToken(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return &TokenStatus{Configured: false, Hint: "", UpdatedAt: nil}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the TMDb token: %w", err)
	}
	token, err := t.box.Open(row.TokenEncrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt the TMDb token: %w", err)
	}

	return &TokenStatus{Configured: true, Hint: tokenHint(string(token)), UpdatedAt: new(row.UpdatedAt)}, nil
}

// Token は、設定したトークンを返す。設定していなければ空を返す。
func (t *Tokens) Token(ctx context.Context, db sqlc.DBTX) (string, error) {
	row, err := sqlc.New(db).GetTMDbToken(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get the TMDb token: %w", err)
	}
	token, err := t.box.Open(row.TokenEncrypted)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt the TMDb token: %w", err)
	}

	return string(token), nil
}

// Update は、トークンを token に変える。前後の空白は除き、空にすると消す。TMDb には問い合わせない。
// サーバーが外部へ問い合わせるときに使う秘密情報なので、呼ぶ側で、直近の再認証を確かめる (設計書 9.5)。
// 監査ログには、変えたことだけを残し、値は残さない。
func (t *Tokens) Update(ctx context.Context, db Database, actor *audit.Actor, token string) error {
	token = strings.TrimSpace(token)
	if err := validateToken(token); err != nil {
		return err
	}

	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		action, err := t.save(ctx, sqlc.New(tx), token)
		if err != nil || action == "" {
			return err
		}

		return t.recorder.Record(ctx, tx, &audit.Event{
			Actor: actor, Action: action, TargetType: tokenAuditTarget, TargetID: "tmdb", Summary: map[string]any{},
			// 送信元は、操作者 (actor) に持たせる。
			SourceIP: netip.Addr{}, UserAgent: "",
		})
	})
}

// LogSecrets は、ログの報告から伏せる値として、設定したトークンを返す (#1521)。連携を無効にしていても、保存してある値は返す。
func (t *Tokens) LogSecrets(ctx context.Context, db sqlc.DBTX) ([]string, error) {
	token, err := t.Token(ctx, db)
	if err != nil {
		return nil, err
	}
	if token == "" {
		return []string{}, nil
	}

	return []string{token}, nil
}

// save は、トークンを暗号化して保存するか、空なら消し、監査ログに残す操作の名前を返す。消すものがなければ空を返す。
func (t *Tokens) save(ctx context.Context, q *sqlc.Queries, token string) (string, error) {
	if token == "" {
		deleted, err := q.DeleteTMDbToken(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to delete the TMDb token: %w", err)
		}
		if deleted == 0 {
			return "", nil
		}

		return "drama.tmdb_token_deleted", nil
	}
	encrypted, err := t.box.Seal([]byte(token))
	if err != nil {
		return "", err
	}
	if err := q.UpsertTMDbToken(ctx, sqlc.UpsertTMDbTokenParams{TokenEncrypted: encrypted, Now: t.clock.Now()}); err != nil {
		return "", fmt.Errorf("failed to save the TMDb token: %w", err)
	}

	return "drama.tmdb_token_updated", nil
}

// validateToken は、トークンが長すぎず、Authorization ヘッダーに入れられない文字 (空白や制御文字、ASCII の外の文字) を含まないことを確かめる。
func validateToken(token string) error {
	if len(token) > maxTokenLength {
		return fmt.Errorf("%w: the token must be at most %d characters", ErrInvalidToken, maxTokenLength)
	}
	for _, r := range token {
		if r <= ' ' || r > '~' {
			return fmt.Errorf("%w: the token must consist of visible ASCII characters", ErrInvalidToken)
		}
	}

	return nil
}

// tokenHint は、トークンを見分けるための末尾の数文字を返す。短いトークンは出さない。
func tokenHint(token string) string {
	if len(token) < minHintedTokenLength {
		return ""
	}

	return token[len(token)-tokenHintLength:]
}

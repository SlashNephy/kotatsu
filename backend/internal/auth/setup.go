package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/password"
)

var (
	// ErrSetupDone は、管理者がいるので、初回セットアップは済んでいることを表す。
	ErrSetupDone = errors.New("the setup is already done")
	// ErrInvalidSetupToken は、セットアップトークンが違うことを表す。
	ErrInvalidSetupToken = errors.New("invalid setup token")
)

// Setup は、管理者が 1 人もいない間だけ、最初の管理者を作らせる (設計書 9.2)。
//
// 管理者を作るには、セットアップトークンが要る。トークンがなければ、Kotatsu を公開した状態で、
// 第三者に先に管理者を作られてしまうからである。トークンは、all の構成では起動時にログへ出し、
// マルチレプリカの構成では kotatsu admin setup-token で発行する。
type Setup struct {
	clock    clock.Clock
	recorder audit.Recorder
	policy   *password.Policy
	logger   *slog.Logger
}

// NewSetup は Setup を返す。policy で、管理者のパスワードの規定を確かめる。
func NewSetup(clk clock.Clock, recorder audit.Recorder, policy *password.Policy, logger *slog.Logger) *Setup {
	return &Setup{clock: clk, recorder: recorder, policy: policy, logger: logger}
}

// Needed は、管理者が 1 人もおらず、初回セットアップが要るかどうかを返す。
// 無効にした管理者も数える。管理者を作った後は、初回セットアップを開き直さない。
func (s *Setup) Needed(ctx context.Context, db sqlc.DBTX) (bool, error) {
	hasAdmin, err := sqlc.New(db).HasAdmin(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to find an administrator: %w", err)
	}

	return !hasAdmin, nil
}

// IssueToken は、セットアップトークンを発行する。DB にはハッシュだけを保存する。
// 管理者がいれば、ErrSetupDone を返す。
func (s *Setup) IssueToken(ctx context.Context, db Database) (string, error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("failed to generate a setup token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)

	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		if err := q.LockSetup(ctx); err != nil {
			return fmt.Errorf("failed to lock the setup: %w", err)
		}
		if err := s.requireNoAdmin(ctx, tx); err != nil {
			return err
		}
		if err := q.InsertSetupToken(ctx, sqlc.InsertSetupTokenParams{TokenHash: hashToken(token), Now: s.clock.Now()}); err != nil {
			return fmt.Errorf("failed to save the setup token: %w", err)
		}

		return nil
	})
	if err != nil {
		return "", err
	}

	return token, nil
}

// NewAdmin は、初回セットアップで作る管理者の項目。
type NewAdmin struct {
	Username    string
	DisplayName string
	Password    string
}

// CreateAdmin は、セットアップトークンを確かめて、最初の管理者を作る。作ったら、すべてのセットアップトークンを消す。
// 管理者には、パスワードだけを設定する。第二要素は、最初にログインするときに登録する (設計書 9.3)。
func (s *Setup) CreateAdmin(ctx context.Context, db Database, token string, admin *NewAdmin, client *ClientInfo) (*User, error) {
	// パスワードの規定の確認とハッシュ化は時間がかかるので、トランザクションの外で行う。
	// その前に、セットアップが済んでいないか、トークンが正しいかを安く確かめ、正しいトークンを持つ人にだけ時間を使う。
	// パスワードの規定に当たったかどうかも、トークンを持たない人には見せない。
	if err := s.requireNoAdmin(ctx, db); err != nil {
		return nil, err
	}
	if err := s.requireToken(ctx, db, token); err != nil {
		return nil, err
	}
	if err := s.policy.Check(ctx, admin.Password, []string{admin.Username, admin.DisplayName}); err != nil {
		return nil, err
	}
	encoded, err := password.Hash(admin.Password)
	if err != nil {
		return nil, err
	}

	var created *User
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		if err := q.LockSetup(ctx); err != nil {
			return fmt.Errorf("failed to lock the setup: %w", err)
		}
		// ロックを取ってから、もう一度確かめる。同時に作られないようにするためである。
		if err := s.requireNoAdmin(ctx, tx); err != nil {
			return err
		}
		if err := s.requireToken(ctx, tx, token); err != nil {
			return err
		}
		created, err = NewUsers(s.clock).Create(ctx, tx, &NewUser{
			Username: admin.Username, DisplayName: admin.DisplayName, Role: RoleAdmin, PasswordHash: &encoded,
		})
		if err != nil {
			return err
		}
		if err := q.DeleteSetupTokens(ctx); err != nil {
			return fmt.Errorf("failed to delete the setup tokens: %w", err)
		}

		return s.recorder.Record(ctx, tx, &audit.Event{
			Actor:      audit.User(created.ID),
			Action:     "user.setup",
			TargetType: loginAuditTarget,
			TargetID:   created.ID.String(),
			Summary:    map[string]any{summaryUsername: created.Username},
			SourceIP:   client.SourceIP,
			UserAgent:  client.UserAgent,
		})
	})
	if err != nil {
		return nil, err
	}

	return created, nil
}

type setupStatusResponse struct {
	// Required は、初回セットアップが要るかどうか。画面は、これを見て /setup に案内する。
	Required bool `json:"required"`
}

type setupRequest struct {
	Token       string `json:"token"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Password    string `json:"password"`
}

type setupResponse struct {
	Username string `json:"username"`
	// Next は、次に行うこと。作った管理者でログインし、第二要素を登録する。
	Next string `json:"next"`
}

type weakPasswordResponse struct {
	Error weakPasswordBody `json:"error"`
}

type weakPasswordBody struct {
	Code   string          `json:"code"`
	Reason password.Reason `json:"reason"`
}

// StatusHandler は、初回セットアップが要るかを返すハンドラーを返す (GET /auth/setup)。
func (s *Setup) StatusHandler(db sqlc.DBTX) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		needed, err := s.Needed(r.Context(), db)
		if err != nil {
			writeInternalError(w, r, s.logger, "failed to set up", err)

			return
		}
		writeJSON(w, http.StatusOK, setupStatusResponse{Required: needed})
	})
}

// CreateHandler は、セットアップトークンを確かめて、最初の管理者を作るハンドラーを返す (POST /auth/setup)。
func (s *Setup) CreateHandler(db Database, client func(*http.Request) *ClientInfo) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req setupRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Token == "" {
			writeLoginError(w, http.StatusUnauthorized, "INVALID_SETUP_TOKEN", "the setup token is required")

			return
		}

		created, err := s.CreateAdmin(r.Context(), db, req.Token, &NewAdmin{
			Username: req.Username, DisplayName: req.DisplayName, Password: req.Password,
		}, client(r))
		if rejected, ok := errors.AsType[*password.RejectedError](err); ok {
			writeJSON(w, http.StatusBadRequest, weakPasswordResponse{Error: weakPasswordBody{Code: "WEAK_PASSWORD", Reason: rejected.Reason}})

			return
		}
		switch {
		case err == nil:
			writeJSON(w, http.StatusCreated, setupResponse{Username: created.Username, Next: "login"})
		case errors.Is(err, ErrSetupDone):
			writeLoginError(w, http.StatusConflict, "SETUP_DONE", "the setup is already done")
		case errors.Is(err, ErrInvalidSetupToken):
			writeLoginError(w, http.StatusUnauthorized, "INVALID_SETUP_TOKEN", "invalid setup token")
		case errors.Is(err, ErrInvalidUser), errors.Is(err, ErrUsernameTaken):
			writeLoginError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		default:
			writeInternalError(w, r, s.logger, "failed to set up", err)
		}
	})
}

func (s *Setup) requireNoAdmin(ctx context.Context, db sqlc.DBTX) error {
	needed, err := s.Needed(ctx, db)
	if err != nil {
		return err
	}
	if !needed {
		return ErrSetupDone
	}

	return nil
}

func (s *Setup) requireToken(ctx context.Context, db sqlc.DBTX, token string) error {
	exists, err := sqlc.New(db).SetupTokenExists(ctx, hashToken(token))
	if err != nil {
		return fmt.Errorf("failed to find the setup token: %w", err)
	}
	if !exists {
		// 管理者を作ると、トークンはすべて消える。管理者の有無を確かめてから、トークンを確かめるまでの間に、
		// 別の人のセットアップが終わった場合は、トークンの誤りではなく、セットアップが済んだことを返す。
		if err := s.requireNoAdmin(ctx, db); err != nil {
			return err
		}

		return ErrInvalidSetupToken
	}

	return nil
}

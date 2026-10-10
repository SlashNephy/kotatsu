package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
)

// BeginPasskeyLogin は、パスキーだけでのログインを始める (設計書 9.3)。
//
// どのユーザーかは、認証器が答える。パスキーは、端末を持っていることと、生体認証か PIN の両方を確かめるので、
// 本人の確認 (user verification) を必須にし、それだけで二要素の認証とみなす。
func (l *Logins) BeginPasskeyLogin(ctx context.Context, db Database, client *ClientInfo) (*PasskeyAssertion, error) {
	rp, err := relyingParty(client)
	if err != nil {
		return nil, err
	}
	options, session, err := rp.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		return nil, fmt.Errorf("failed to begin the passkey login: %w", err)
	}
	var assertion *PasskeyAssertion
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		token, expiresAt, err := saveCeremony(ctx, tx, l.clock.Now(), ceremonyLogin, nil, session)
		if err != nil {
			return err
		}
		assertion = &PasskeyAssertion{Options: &options.Response, Token: token, ExpiresAt: expiresAt}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return assertion, nil
}

// CompleteWithPasskey は、認証器の応答 (PublicKeyCredential の JSON) を確かめ、パスキーだけでのログインを完了させる。
// 手続きは、確かめられなくても使用済みにする。確かめられなければ ErrInvalidPasskey を返す。
func (l *Logins) CompleteWithPasskey(ctx context.Context, db Database, token string, response []byte, client *ClientInfo) (*LoginResult, error) {
	rp, err := relyingParty(client)
	if err != nil {
		return nil, err
	}
	var (
		result    *LoginResult
		verifyErr error
	)
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		now := l.clock.Now()
		_, session, err := takeCeremony(ctx, tx, ceremonyLogin, token, now)
		if err != nil {
			return err
		}
		var user *webauthnUser
		user, verifyErr = l.verifyDiscoverablePasskey(ctx, tx, rp, session, response, client)
		if errors.Is(verifyErr, ErrInvalidPasskey) {
			// 失敗と、手続きを使用済みにしたことは、ロールバックせずに残す。
			return nil
		}
		if verifyErr != nil {
			return verifyErr
		}
		result, err = l.startSession(ctx, tx, user.id, SecondFactorPasskey, client)

		return err
	})
	if err != nil {
		return nil, err
	}
	if verifyErr != nil {
		return nil, verifyErr
	}

	return result, nil
}

// verifyDiscoverablePasskey は、認証器が答えたパスキーを引き、応答を確かめる。
func (l *Logins) verifyDiscoverablePasskey(
	ctx context.Context,
	tx pgx.Tx,
	rp *webauthn.WebAuthn,
	session *webauthn.SessionData,
	response []byte,
	client *ClientInfo,
) (*webauthnUser, error) {
	parsed, err := protocol.ParseCredentialRequestResponseBytes(response)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidPasskey, err)
	}
	row, err := sqlc.New(tx).GetPasskeyByCredentialID(ctx, parsed.RawID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: unknown credential", ErrInvalidPasskey)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find the passkey: %w", err)
	}
	user, err := loadWebAuthnUser(ctx, tx, row.UserID)
	if err != nil {
		return nil, err
	}

	verifyErr := func() error {
		if user.disabled {
			return fmt.Errorf("%w: the user is disabled", ErrInvalidPasskey)
		}
		// 認証器が答えたユーザーハンドルが、パスキーの持ち主と同じかは、go-webauthn が確かめる。
		_, credential, err := rp.ValidatePasskeyLogin(func(_, _ []byte) (webauthn.User, error) {
			return user, nil
		}, *session, parsed)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidPasskey, err)
		}

		return l.acceptPasskey(ctx, tx, user, credential)
	}()
	if errors.Is(verifyErr, ErrInvalidPasskey) {
		if err := l.recordPasskeyFailure(ctx, tx, user, client); err != nil {
			return nil, err
		}
	}
	if verifyErr != nil {
		return nil, verifyErr
	}

	return user, nil
}

// BeginPasskeySecondFactor は、パスワードを確かめた後の第二要素として、パスキーでの確認を始める。
// 手続きの状態は、待っているログインに持たせる。
func (l *Logins) BeginPasskeySecondFactor(ctx context.Context, db Database, token string, client *ClientInfo) (*PasskeyAssertion, error) {
	rp, err := relyingParty(client)
	if err != nil {
		return nil, err
	}
	var assertion *PasskeyAssertion
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		row, err := l.challenge(ctx, tx, token)
		if err != nil {
			return err
		}
		user, err := loadWebAuthnUser(ctx, tx, row.UserID)
		if err != nil {
			return err
		}
		if len(user.credentials) == 0 {
			return ErrNoPasskey
		}
		options, session, err := rp.BeginLogin(user, webauthn.WithUserVerification(protocol.VerificationRequired))
		if err != nil {
			return fmt.Errorf("failed to begin the passkey login: %w", err)
		}
		if err := setChallengeSession(ctx, tx, row, session); err != nil {
			return err
		}
		assertion = &PasskeyAssertion{Options: &options.Response, Token: "", ExpiresAt: row.ExpiresAt}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return assertion, nil
}

// CompleteWithPasskeySecondFactor は、パスキーの応答を確かめて、パスワードでのログインを完了させる。
// 失敗は、ほかの第二要素と同じく数え、上限に達したらパスワードの確認からやり直させる。
func (l *Logins) CompleteWithPasskeySecondFactor(ctx context.Context, db Database, token string, response []byte, client *ClientInfo) (*LoginResult, error) {
	rp, err := relyingParty(client)
	if err != nil {
		return nil, err
	}

	return l.complete(ctx, db, token, SecondFactorPasskey, client, func(tx pgx.Tx, row *sqlc.LoginChallenge) error {
		if row.WebauthnSession == nil {
			return fmt.Errorf("%w: the passkey ceremony was not started", ErrInvalidSecondFactor)
		}
		session, err := decodeSession(row.WebauthnSession)
		if err != nil {
			return err
		}
		// チャレンジは 1 回しか使わせない。失敗しても、次は確認を始め直させる。
		if err := setChallengeSession(ctx, tx, row, nil); err != nil {
			return err
		}
		parsed, err := protocol.ParseCredentialRequestResponseBytes(response)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidSecondFactor, err)
		}
		user, err := loadWebAuthnUser(ctx, tx, row.UserID)
		if err != nil {
			return err
		}
		credential, err := rp.ValidateLogin(user, *session, parsed)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidSecondFactor, err)
		}
		if err := l.acceptPasskey(ctx, tx, user, credential); err != nil {
			if errors.Is(err, ErrInvalidPasskey) {
				return fmt.Errorf("%w: %w", ErrInvalidSecondFactor, err)
			}

			return err
		}

		return nil
	})
}

// acceptPasskey は、確かめたパスキーの署名の回数を見て、複製の疑いがなければ、使ったことを残す。
//
// 同期するパスキーは署名の回数を 0 のままにするので、0 が続くことは疑わない。go-webauthn は、0 でない回数が
// 増えていないときだけ、複製の疑いを立てる。
func (l *Logins) acceptPasskey(ctx context.Context, tx pgx.Tx, user *webauthnUser, credential *webauthn.Credential) error {
	if credential.Authenticator.CloneWarning {
		l.logger.WarnContext(ctx, "the passkey may be cloned", "userId", user.id.String())

		return fmt.Errorf("%w: the signature counter did not increase", ErrInvalidPasskey)
	}

	return recordPasskeyUse(ctx, tx, user, credential, l.clock.Now())
}

func (l *Logins) recordPasskeyFailure(ctx context.Context, tx pgx.Tx, user *webauthnUser, client *ClientInfo) error {
	return l.recorder.Record(ctx, tx, &audit.Event{
		Actor:      audit.User(user.id),
		Action:     actionLoginFailed,
		TargetType: loginAuditTarget,
		TargetID:   user.id.String(),
		Summary:    map[string]any{summaryUsername: user.name, summaryMethod: string(SecondFactorPasskey)},
		SourceIP:   client.SourceIP,
		UserAgent:  client.UserAgent,
	})
}

// setChallengeSession は、待っているログインに、パスキーの手続きの状態を持たせる。nil なら消す。
func setChallengeSession(ctx context.Context, tx pgx.Tx, row *sqlc.LoginChallenge, session *webauthn.SessionData) error {
	var encoded []byte
	if session != nil {
		var err error
		if encoded, err = json.Marshal(session); err != nil {
			return fmt.Errorf("failed to encode the passkey ceremony: %w", err)
		}
	}
	if err := sqlc.New(tx).SetLoginChallengeWebAuthnSession(ctx, sqlc.SetLoginChallengeWebAuthnSessionParams{ID: row.ID, WebauthnSession: encoded}); err != nil {
		return fmt.Errorf("failed to save the passkey ceremony: %w", err)
	}

	return nil
}

package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/password"
)

const (
	ceremonyReauthentication = "reauthentication"
	// actionReauthenticated は、ログイン中のセッションのまま、もう一度認証したことを表す監査ログの操作。
	actionReauthenticated = "user.reauthenticated"
	// actionReauthenticationFailed は、再認証に失敗したことを表す監査ログの操作。
	actionReauthenticationFailed = "user.reauthentication_failed"
)

// ErrUnsupportedSecondFactor は、パスワードと組み合わせる第二要素として、使えない種類を指定したことを表す。
var ErrUnsupportedSecondFactor = errors.New("unsupported second factor; use totp or recovery_code, or a passkey alone")

// Reauthenticate は、ログイン中のセッションのまま、パスワードと第二要素 (TOTP かリカバリーコード) を確かめ、
// セッションの認証の日時を更新する (設計書 9.5)。更新した日時を返す。
//
// 失敗は、パスワードでのログインと同じく、アカウント単位と IP 単位で数えて待たせる。
// パスワードが合っていても、第二要素を総当たりで当てさせないためである。
func (l *Logins) Reauthenticate(
	ctx context.Context,
	db Database,
	principal *Principal,
	secret string,
	method SecondFactor,
	code string,
	client *ClientInfo,
) (time.Time, error) {
	if method != SecondFactorTOTP && method != SecondFactorRecoveryCode {
		return time.Time{}, ErrUnsupportedSecondFactor
	}
	user, err := sqlc.New(db).GetUser(ctx, principal.UserID)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get the user: %w", err)
	}
	now := l.clock.Now()
	keys := throttleKeys(user.Username, client.SourceIP)

	var verifyErr error
	// パスワードでのログインと同じく、制限のキーをロックしてから、待ち時間を確かめて検証する (#542)。
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		if err := lockThrottleKeys(ctx, tx, keys); err != nil {
			return err
		}
		if err := checkThrottleIn(ctx, tx, keys, now); err != nil {
			return err
		}
		verification := &password.Verification{OK: false, NeedsRehash: false}
		if user.DisabledAt == nil && user.PasswordHash != nil {
			if verification, err = password.Verify(secret, *user.PasswordHash); err != nil {
				return err
			}
		} else if err := password.VerifyDummy(secret); err != nil {
			return err
		}

		failed := "password"
		switch {
		case !verification.OK:
			verifyErr = ErrInvalidCredentials
		case method == SecondFactorTOTP:
			verifyErr, failed = l.factors.VerifyTOTP(ctx, tx, user.ID, code), string(method)
		default:
			verifyErr, failed = l.factors.UseRecoveryCode(ctx, tx, user.ID, code), string(method)
		}
		if errors.Is(verifyErr, ErrInvalidCredentials) || errors.Is(verifyErr, ErrInvalidSecondFactor) {
			// 失敗は、ロールバックせずに数えて残す。
			return l.recordFailure(ctx, tx, keys, &user.ID, user.Username, client, now, actionReauthenticationFailed, failed)
		}
		if verifyErr != nil {
			return verifyErr
		}
		if err := sqlc.New(tx).ResetLoginThrottle(ctx, keys[0]); err != nil {
			return fmt.Errorf("failed to reset the login throttle: %w", err)
		}

		return l.markReauthenticated(ctx, tx, principal, method, client, now)
	})
	if err != nil {
		return time.Time{}, err
	}
	if verifyErr != nil {
		return time.Time{}, verifyErr
	}

	return now, nil
}

// BeginPasskeyReauthentication は、ログイン中のユーザーに、パスキーでの再認証を始めさせる。
// パスキーは、本人の確認 (user verification) を必須にするので、それだけで二要素の認証とみなす (設計書 9.3)。
func (l *Logins) BeginPasskeyReauthentication(ctx context.Context, db Database, principal *Principal, client *ClientInfo) (*PasskeyAssertion, error) {
	rp, err := relyingParty(client)
	if err != nil {
		return nil, err
	}
	var assertion *PasskeyAssertion
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		user, err := loadWebAuthnUser(ctx, tx, principal.UserID, rp.Config.RPID)
		if err != nil {
			return err
		}
		if len(user.credentials) == 0 {
			return ErrNoPasskey
		}
		options, session, err := rp.BeginLogin(user, webauthn.WithUserVerification(protocol.VerificationRequired))
		if err != nil {
			return fmt.Errorf("failed to begin the passkey reauthentication: %w", err)
		}
		token, expiresAt, err := saveCeremony(ctx, tx, l.clock.Now(), ceremonyReauthentication, &principal.UserID, session)
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

// CompletePasskeyReauthentication は、パスキーの応答を確かめ、セッションの認証の日時を更新する。更新した日時を返す。
// 手続きは、確かめられなくても使用済みにする。
func (l *Logins) CompletePasskeyReauthentication(
	ctx context.Context,
	db Database,
	principal *Principal,
	token string,
	response []byte,
	client *ClientInfo,
) (time.Time, error) {
	rp, err := relyingParty(client)
	if err != nil {
		return time.Time{}, err
	}
	now := l.clock.Now()
	var verifyErr error
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		ceremony, session, err := takeCeremony(ctx, tx, ceremonyReauthentication, token, now)
		if err != nil {
			return err
		}
		if ceremony.UserID == nil || *ceremony.UserID != principal.UserID {
			return ErrWebAuthnCeremonyInvalid
		}
		user, err := loadWebAuthnUser(ctx, tx, principal.UserID, rp.Config.RPID)
		if err != nil {
			return err
		}
		verifyErr = l.verifyPasskeyOf(ctx, tx, rp, user, session, response)
		if errors.Is(verifyErr, ErrInvalidPasskey) {
			// 失敗と、手続きを使用済みにしたことは、ロールバックせずに残す。
			return l.recordPasskeyFailure(ctx, tx, user, client, actionReauthenticationFailed)
		}
		if verifyErr != nil {
			return verifyErr
		}

		return l.markReauthenticated(ctx, tx, principal, SecondFactorPasskey, client, now)
	})
	if err != nil {
		return time.Time{}, err
	}
	if verifyErr != nil {
		return time.Time{}, verifyErr
	}

	return now, nil
}

// verifyPasskeyOf は、user のパスキーで署名した応答を確かめる。確かめられなければ ErrInvalidPasskey を返す。
func (l *Logins) verifyPasskeyOf(
	ctx context.Context,
	tx pgx.Tx,
	rp *webauthn.WebAuthn,
	user *webauthnUser,
	session *webauthn.SessionData,
	response []byte,
) error {
	if user.disabled {
		return fmt.Errorf("%w: the user is disabled", ErrInvalidPasskey)
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(response)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidPasskey, err)
	}
	credential, err := rp.ValidateLogin(user, *session, parsed)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidPasskey, err)
	}

	return l.acceptPasskey(ctx, tx, user, credential)
}

// markReauthenticated は、セッションの認証の日時を now にし、監査ログに残す。
func (l *Logins) markReauthenticated(ctx context.Context, tx pgx.Tx, principal *Principal, method SecondFactor, client *ClientInfo, now time.Time) error {
	return markSessionReauthenticated(ctx, tx, l.recorder, principal, method, client, now)
}

// markSessionReauthenticated は、セッションの認証の日時を now にし、method で再認証したことを監査ログに残す。
func markSessionReauthenticated(
	ctx context.Context, tx pgx.Tx, recorder audit.Recorder, principal *Principal, method SecondFactor, client *ClientInfo, now time.Time,
) error {
	n, err := sqlc.New(tx).ReauthenticateSession(ctx, sqlc.ReauthenticateSessionParams{Now: now, ID: principal.SessionID, UserID: principal.UserID})
	if err != nil {
		return fmt.Errorf("failed to update the session: %w", err)
	}
	// 確かめている間に、セッションを失効させられた。
	if n == 0 {
		return ErrUnauthenticated
	}

	return recorder.Record(ctx, tx, &audit.Event{
		Actor:      audit.UserFrom(principal.UserID, client.SourceIP, client.UserAgent),
		Action:     actionReauthenticated,
		TargetType: loginAuditTarget,
		TargetID:   principal.UserID.String(),
		Summary:    map[string]any{summaryMethod: string(method), "sessionId": principal.SessionID.String()},
		SourceIP:   client.SourceIP,
		UserAgent:  client.UserAgent,
	})
}

package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/usersettings"
)

const (
	// SecondFactorPasskey は、第二要素としてのパスキー。
	SecondFactorPasskey SecondFactor = "passkey"
	// WebAuthnCookieName は、パスキーの登録と、パスキーだけでのログインの手続きのトークンを入れる Cookie の名前。
	WebAuthnCookieName = "kotatsu_webauthn"
	// MaxPasskeys は、1 人が登録できるパスキーの数 (付録 A)。
	MaxPasskeys = 20
	// PasskeyCeremonyTTL は、パスキーの手続きを始めてから、認証器の応答を受け取るまでに許す時間 (付録 A)。
	PasskeyCeremonyTTL = 5 * time.Minute
	// maxPasskeyNameLength は、パスキーの名前の長さの上限 (文字数)。
	maxPasskeyNameLength = 64
	// rpDisplayName は、認証器に出す、Kotatsu の名前。
	rpDisplayName = "Kotatsu"

	ceremonyRegistration = "registration"
	ceremonyLogin        = "login"
)

var (
	// ErrPasskeyUnavailable は、Secure Context (HTTPS か localhost) でないか、IP アドレスで接続しているので、
	// パスキーを使えないことを表す。WebAuthn は、ドメイン名の Secure Context でしか使えない。
	ErrPasskeyUnavailable = errors.New("passkeys require HTTPS (or localhost) and a domain name, not an IP address")
	// ErrInvalidPasskey は、認証器の応答を確かめられなかったことを表す。
	ErrInvalidPasskey = errors.New("invalid passkey")
	// ErrWebAuthnCeremonyInvalid は、パスキーの手続きを始めていないか、期限が切れたか、使用済みであることを表す。
	ErrWebAuthnCeremonyInvalid = errors.New("the passkey ceremony has expired or was not started")
	// ErrTooManyPasskeys は、登録できるパスキーの数の上限に達したことを表す。
	ErrTooManyPasskeys = errors.New("too many passkeys")
	// ErrInvalidPasskeyName は、パスキーの名前が空か、長すぎることを表す。
	ErrInvalidPasskeyName = errors.New("invalid passkey name")
	// ErrPasskeyNotFound は、パスキーがないことを表す。
	ErrPasskeyNotFound = errors.New("passkey not found")
	// ErrLastPasskey は、パスワードを持たないユーザーの最後のパスキーなので、消せないことを表す。
	// 消すと、ログインする手段がなくなるからである。
	ErrLastPasskey = errors.New("the last passkey of a user without a password cannot be deleted")
	// ErrNoPasskey は、パスキーを登録していないことを表す。
	ErrNoPasskey = errors.New("no passkey is registered")
)

// Passkey は、登録したパスキー。
type Passkey struct {
	ID   uuid.UUID
	Name string
	// RPID は、登録したときに接続していたホスト名。パスキーは、このホスト名でしか使えない。
	RPID       string
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

// PasskeyCreation は、パスキーの登録の開始。Options をブラウザーの navigator.credentials.create に渡す。
type PasskeyCreation struct {
	Options *protocol.PublicKeyCredentialCreationOptions
	// Token は、手続きを続けるためのトークン。Cookie (kotatsu_webauthn) に入れる。
	Token     string
	ExpiresAt time.Time
}

// PasskeyAssertion は、パスキーでのログインの開始。Options をブラウザーの navigator.credentials.get に渡す。
type PasskeyAssertion struct {
	Options *protocol.PublicKeyCredentialRequestOptions
	// Token は、パスキーだけでのログインを続けるためのトークン。第二要素としての確認では、待っているログインの
	// トークンをそのまま使うので空。
	Token     string
	ExpiresAt time.Time
}

// PasskeysOptions は Passkeys の設定。
type PasskeysOptions struct {
	Clock    clock.Clock
	Recorder audit.Recorder
	Logger   *slog.Logger
	// Sessions は、手続きのトークンを入れる Cookie の属性を、セッションの Cookie とそろえる。
	Sessions *Sessions
}

// Passkeys は、パスキーの登録と管理を扱う (設計書 9.3)。ログインでの確認は Logins が扱う。
type Passkeys struct {
	clock    clock.Clock
	recorder audit.Recorder
	logger   *slog.Logger
	sessions *Sessions
}

// NewPasskeys は Passkeys を返す。
func NewPasskeys(opts *PasskeysOptions) *Passkeys {
	return &Passkeys{clock: opts.Clock, recorder: opts.Recorder, logger: opts.Logger, sessions: opts.Sessions}
}

// BeginRegistration は、ログイン中のユーザーにパスキーの登録を始めさせる。
//
// パスキーだけでもログインできるよう、認証器に保存するパスキー (discoverable credential) を求め、
// 生体認証か PIN での本人の確認 (user verification) を必須にする。
func (p *Passkeys) BeginRegistration(ctx context.Context, db Database, userID uuid.UUID, client *ClientInfo) (*PasskeyCreation, error) {
	rp, err := relyingParty(client)
	if err != nil {
		return nil, err
	}
	var creation *PasskeyCreation
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		user, err := loadWebAuthnUser(ctx, tx, userID, rp.Config.RPID)
		if err != nil {
			return err
		}
		if user.total >= MaxPasskeys {
			return ErrTooManyPasskeys
		}
		// 登録済みの認証器に、同じユーザーのパスキーを重ねて作らせない。
		exclusions := make([]protocol.CredentialDescriptor, 0, len(user.credentials))
		for i := range user.credentials {
			exclusions = append(exclusions, user.credentials[i].Descriptor())
		}
		options, session, err := rp.BeginRegistration(user, webauthn.WithExclusions(exclusions))
		if err != nil {
			return fmt.Errorf("failed to begin the passkey registration: %w", err)
		}
		token, expiresAt, err := saveCeremony(ctx, tx, p.clock.Now(), ceremonyRegistration, &userID, session)
		if err != nil {
			return err
		}
		creation = &PasskeyCreation{Options: &options.Response, Token: token, ExpiresAt: expiresAt}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return creation, nil
}

// FinishRegistration は、認証器の応答 (PublicKeyCredential の JSON) を確かめ、パスキーを name の名前で登録する。
// 手続きは、確かめられなくても使用済みにする。
func (p *Passkeys) FinishRegistration(
	ctx context.Context,
	db Database,
	actor *audit.Actor,
	userID uuid.UUID,
	token, name string,
	response []byte,
	client *ClientInfo,
) (*Passkey, error) {
	name, err := normalizePasskeyName(name)
	if err != nil {
		return nil, err
	}
	rp, err := relyingParty(client)
	if err != nil {
		return nil, err
	}
	var (
		created   *Passkey
		verifyErr error
	)
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		now := p.clock.Now()
		ceremony, session, err := takeCeremony(ctx, tx, ceremonyRegistration, token, now)
		if err != nil {
			return err
		}
		if ceremony.UserID == nil || *ceremony.UserID != userID {
			return ErrWebAuthnCeremonyInvalid
		}
		// 同じユーザーの登録を同時に完了させず、上限を超えさせない。
		if _, err := sqlc.New(tx).LockUser(ctx, userID); err != nil {
			return fmt.Errorf("failed to lock the user: %w", err)
		}
		created, verifyErr = p.register(ctx, tx, rp, actor, userID, name, session, response, now)
		if verifyErr != nil && !errors.Is(verifyErr, ErrInvalidPasskey) && !errors.Is(verifyErr, ErrTooManyPasskeys) {
			return verifyErr
		}

		// 確かめられなくても、手続きを使用済みにしたことは残す。
		return nil
	})
	if err != nil {
		return nil, err
	}
	if verifyErr != nil {
		return nil, verifyErr
	}

	return created, nil
}

// List は、ユーザーのパスキーを、登録した順に返す。
func (p *Passkeys) List(ctx context.Context, db sqlc.DBTX, userID uuid.UUID) ([]*Passkey, error) {
	rows, err := sqlc.New(db).ListUserPasskeys(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list passkeys: %w", err)
	}
	passkeys := make([]*Passkey, 0, len(rows))
	for i := range rows {
		passkeys = append(passkeys, passkeyOf(&rows[i]))
	}

	return passkeys, nil
}

// Rename は、パスキーの名前を変える。
func (p *Passkeys) Rename(ctx context.Context, tx pgx.Tx, actor *audit.Actor, userID, passkeyID uuid.UUID, name string) error {
	name, err := normalizePasskeyName(name)
	if err != nil {
		return err
	}
	n, err := sqlc.New(tx).RenameUserPasskey(ctx, sqlc.RenameUserPasskeyParams{ID: passkeyID, UserID: userID, Name: name})
	if err != nil {
		return fmt.Errorf("failed to rename the passkey: %w", err)
	}
	if n == 0 {
		return ErrPasskeyNotFound
	}

	return p.record(ctx, tx, actor, "user.passkey_renamed", userID, &Passkey{ID: passkeyID, Name: name, RPID: "", CreatedAt: time.Time{}, LastUsedAt: nil})
}

// Delete は、パスキーを消す。パスワードを持たないユーザーの最後のパスキーは、ログインする手段がなくなるので消さない。
func (p *Passkeys) Delete(ctx context.Context, tx pgx.Tx, actor *audit.Actor, userID, passkeyID uuid.UUID) error {
	q := sqlc.New(tx)
	// 同じユーザーのパスキーを同時に消して、最後の 1 つまで消してしまわないようにする。
	if _, err := q.LockUser(ctx, userID); err != nil {
		return fmt.Errorf("failed to lock the user: %w", err)
	}
	user, err := q.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get the user: %w", err)
	}
	rows, err := q.ListUserPasskeys(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to list passkeys: %w", err)
	}
	var target *sqlc.UserPasskey
	for i := range rows {
		if rows[i].ID == passkeyID {
			target = &rows[i]
		}
	}
	if target == nil {
		return ErrPasskeyNotFound
	}
	if user.PasswordHash == nil && len(rows) == 1 {
		return ErrLastPasskey
	}
	if _, err := q.DeleteUserPasskey(ctx, sqlc.DeleteUserPasskeyParams{ID: passkeyID, UserID: userID}); err != nil {
		return fmt.Errorf("failed to delete the passkey: %w", err)
	}

	return p.record(ctx, tx, actor, "user.passkey_deleted", userID, passkeyOf(target))
}

func (p *Passkeys) register(
	ctx context.Context,
	tx pgx.Tx,
	rp *webauthn.WebAuthn,
	actor *audit.Actor,
	userID uuid.UUID,
	name string,
	session *webauthn.SessionData,
	response []byte,
	now time.Time,
) (*Passkey, error) {
	user, err := loadWebAuthnUser(ctx, tx, userID, rp.Config.RPID)
	if err != nil {
		return nil, err
	}
	if user.total >= MaxPasskeys {
		return nil, ErrTooManyPasskeys
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(response)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidPasskey, err)
	}
	credential, err := rp.CreateCredential(user, *session, parsed)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidPasskey, err)
	}

	q := sqlc.New(tx)
	// 別のユーザーが登録した認証器の ID と重なれば、受け付けない。
	if _, err := q.GetPasskeyByCredentialID(ctx, credential.ID); err == nil {
		return nil, fmt.Errorf("%w: the credential is already registered", ErrInvalidPasskey)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("failed to find the passkey: %w", err)
	}
	encoded, err := json.Marshal(credential)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the passkey: %w", err)
	}
	passkey := &Passkey{ID: id.New(), Name: name, RPID: session.GetRelyingPartyID(rp.Config.RPID), CreatedAt: now, LastUsedAt: nil}
	err = q.InsertUserPasskey(ctx, sqlc.InsertUserPasskeyParams{
		ID: passkey.ID, UserID: userID, CredentialID: credential.ID, Credential: encoded, RpID: passkey.RPID, Name: name, Now: now,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save the passkey: %w", err)
	}

	return passkey, p.record(ctx, tx, actor, "user.passkey_registered", userID, passkey)
}

// record は、パスキーの変更を監査ログに残し、同じユーザーのほかの端末に、設定が変わったことを知らせる。
func (p *Passkeys) record(ctx context.Context, tx pgx.Tx, actor *audit.Actor, action string, userID uuid.UUID, passkey *Passkey) error {
	if err := usersettings.Touch(ctx, tx, p.clock, userID); err != nil {
		return err
	}

	return p.recorder.Record(ctx, tx, &audit.Event{
		Actor:      actor,
		Action:     action,
		TargetType: loginAuditTarget,
		TargetID:   userID.String(),
		Summary:    map[string]any{"passkeyId": passkey.ID.String(), "name": passkey.Name},
		// 送信元は、操作者 (actor) に持たせる。
		SourceIP:  netip.Addr{},
		UserAgent: "",
	})
}

func passkeyOf(row *sqlc.UserPasskey) *Passkey {
	return &Passkey{ID: row.ID, Name: row.Name, RPID: row.RpID, CreatedAt: row.CreatedAt, LastUsedAt: row.LastUsedAt}
}

// normalizePasskeyName は、前後の空白を除いた名前を返す。空か、長すぎれば ErrInvalidPasskeyName を返す。
func normalizePasskeyName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxPasskeyNameLength {
		return "", ErrInvalidPasskeyName
	}

	return name, nil
}

// relyingParty は、送信元が接続したホスト名を RP ID とする WebAuthn の設定を返す。
//
// Kotatsu は、LAN の名前や、リバースプロキシのドメイン名など、決まったホスト名を持たないので、リクエストごとに決める。
// パスキーは、登録したときのホスト名でしか使えない。
func relyingParty(client *ClientInfo) (*webauthn.WebAuthn, error) {
	hostname := client.Host
	if host, _, err := net.SplitHostPort(hostname); err == nil {
		hostname = host
	}
	hostname = strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(hostname, "["), "]"))
	// WebAuthn の RP ID は、ドメイン名でなければならない。
	if _, err := netip.ParseAddr(hostname); err == nil || hostname == "" {
		return nil, ErrPasskeyUnavailable
	}
	// ブラウザーは、HTTPS か localhost でしかパスキーを使わせない。
	if !client.Secure && hostname != "localhost" && !strings.HasSuffix(hostname, ".localhost") {
		return nil, ErrPasskeyUnavailable
	}
	rp, err := webauthn.New(&webauthn.Config{
		RPID:                  hostname,
		RPDisplayName:         rpDisplayName,
		RPOrigins:             []string{client.Origin()},
		AttestationPreference: protocol.PreferNoAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:        protocol.ResidentKeyRequirementRequired,
			RequireResidentKey: new(true),
			UserVerification:   protocol.VerificationRequired,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPasskeyUnavailable, err)
	}

	return rp, nil
}

var _ webauthn.User = new(webauthnUser)

// webauthnUser は、go-webauthn に渡すユーザー。ユーザーハンドルには、個人を表さないユーザーの ID を使う。
type webauthnUser struct {
	id          uuid.UUID
	name        string
	displayName string
	disabled    bool
	hasPassword bool
	// total は、どのホスト名で登録したかを問わない、パスキーの数。
	total int
	// passkeys と credentials は、今の RP ID で登録したパスキーだけを持つ。
	passkeys    []sqlc.UserPasskey
	credentials []webauthn.Credential
}

func (u *webauthnUser) WebAuthnID() []byte {
	return u.id[:]
}

func (u *webauthnUser) WebAuthnName() string {
	return u.name
}

func (u *webauthnUser) WebAuthnDisplayName() string {
	return u.displayName
}

func (u *webauthnUser) WebAuthnCredentials() []webauthn.Credential {
	return u.credentials
}

// loadWebAuthnUser は、ユーザーと、RP ID が rpID のパスキーを引く。
//
// ほかのホスト名で登録したパスキーは、ブラウザーが使わせないので、ログインの候補にも登録の除外にも入れない。
// 認証器が答えても、候補にないパスキーは go-webauthn が受け付けない。
func loadWebAuthnUser(ctx context.Context, db sqlc.DBTX, userID uuid.UUID, rpID string) (*webauthnUser, error) {
	q := sqlc.New(db)
	user, err := q.GetUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get the user: %w", err)
	}
	rows, err := q.ListUserPasskeys(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list passkeys: %w", err)
	}
	passkeys := make([]sqlc.UserPasskey, 0, len(rows))
	credentials := make([]webauthn.Credential, 0, len(rows))
	for i := range rows {
		if rows[i].RpID != rpID {
			continue
		}
		var credential webauthn.Credential
		if err := json.Unmarshal(rows[i].Credential, &credential); err != nil {
			return nil, fmt.Errorf("failed to decode the passkey %s: %w", rows[i].ID, err)
		}
		passkeys = append(passkeys, rows[i])
		credentials = append(credentials, credential)
	}

	return &webauthnUser{
		id:          user.ID,
		name:        user.Username,
		displayName: user.DisplayName,
		disabled:    user.DisabledAt != nil,
		hasPassword: user.PasswordHash != nil,
		total:       len(rows),
		passkeys:    passkeys,
		credentials: credentials,
	}, nil
}

// recordPasskeyUse は、ログインに使ったパスキーの、署名の回数とフラグを更新し、使った日時を残す。
func recordPasskeyUse(ctx context.Context, tx pgx.Tx, user *webauthnUser, credential *webauthn.Credential, now time.Time) error {
	for i := range user.passkeys {
		if !bytes.Equal(user.passkeys[i].CredentialID, credential.ID) {
			continue
		}
		encoded, err := json.Marshal(credential)
		if err != nil {
			return fmt.Errorf("failed to encode the passkey: %w", err)
		}
		if err := sqlc.New(tx).RecordPasskeyUse(ctx, sqlc.RecordPasskeyUseParams{ID: user.passkeys[i].ID, Credential: encoded, Now: new(now)}); err != nil {
			return fmt.Errorf("failed to record the passkey use: %w", err)
		}

		return nil
	}

	return fmt.Errorf("%w: the credential is not registered", ErrInvalidPasskey)
}

// saveCeremony は、パスキーの手続きの途中の状態を保存し、続けるためのトークンを返す。
func saveCeremony(ctx context.Context, tx pgx.Tx, now time.Time, kind string, userID *uuid.UUID, session *webauthn.SessionData) (string, time.Time, error) {
	q := sqlc.New(tx)
	if _, err := q.DeleteExpiredWebAuthnCeremonies(ctx, now); err != nil {
		return "", time.Time{}, fmt.Errorf("failed to delete expired passkey ceremonies: %w", err)
	}
	encoded, err := json.Marshal(session)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to encode the passkey ceremony: %w", err)
	}
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, fmt.Errorf("failed to generate a passkey ceremony token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expiresAt := now.Add(PasskeyCeremonyTTL)
	err = q.InsertWebAuthnCeremony(ctx, sqlc.InsertWebAuthnCeremonyParams{
		ID: id.New(), TokenHash: hashToken(token), Kind: kind, UserID: userID, Session: encoded, Now: now, ExpiresAt: expiresAt,
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to save the passkey ceremony: %w", err)
	}

	return token, expiresAt, nil
}

// takeCeremony は、トークンの指すパスキーの手続きを引き、使用済みにする。
func takeCeremony(ctx context.Context, tx pgx.Tx, kind, token string, now time.Time) (*sqlc.WebauthnCeremony, *webauthn.SessionData, error) {
	row, err := sqlc.New(tx).TakeWebAuthnCeremony(ctx, sqlc.TakeWebAuthnCeremonyParams{TokenHash: hashToken(token), Kind: kind, Now: now})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, ErrWebAuthnCeremonyInvalid
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the passkey ceremony: %w", err)
	}
	session, err := decodeSession(row.Session)
	if err != nil {
		return nil, nil, err
	}

	return &row, session, nil
}

func decodeSession(encoded []byte) (*webauthn.SessionData, error) {
	var session webauthn.SessionData
	if err := json.Unmarshal(encoded, &session); err != nil {
		return nil, fmt.Errorf("failed to decode the passkey ceremony: %w", err)
	}

	return &session, nil
}

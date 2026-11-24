package google

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/googledrive"
	"github.com/SlashNephy/kotatsu/internal/infra/medialib"
)

const (
	// connectionTTL は、DB から読んだ接続を使い回す長さ。接続し直したり外したりしたら、この間に気づく。
	connectionTTL = 30 * time.Second
	// expiryMargin は、アクセストークンの期限のどれだけ前に作り直すか。
	expiryMargin = time.Minute
)

// DriveTokens は、Google Drive のアクセストークンを、DB に置いたリフレッシュトークンから作り、手元で使い回す。
// tenban、kyusu、nekonote のそれぞれが持ち、アクセストークンは DB に置かない。
type DriveTokens struct {
	service *Service
	db      sqlc.DBTX

	mu sync.Mutex
	// row は、最後に DB から読んだ接続。接続していなければ nil。folders は、同じときに読んだ、見せるフォルダ。
	// checkedAt は、読んだ日時。
	row       *sqlc.GoogleDriveConnection
	folders   []*medialib.DriveFolder
	checkedAt time.Time
	// token は、手元のアクセストークン。tokenFor は、それを作ったリフレッシュトークン (暗号化したもの)。expiry は、その期限。
	token    *oauth2.Token
	tokenFor []byte
	expiry   time.Time
}

var (
	_ googledrive.TokenSource = new(DriveTokens)
	_ medialib.DriveScope     = new(DriveTokens)
)

// DriveTokens は、DB db の接続から、Google Drive のアクセストークンを作る DriveTokens を返す。
func (s *Service) DriveTokens(db sqlc.DBTX) *DriveTokens {
	tokens := &DriveTokens{
		service: s, db: db, mu: sync.Mutex{}, row: nil, folders: []*medialib.DriveFolder{}, checkedAt: time.Time{},
		token: nil, tokenFor: nil, expiry: time.Time{},
	}
	s.tokensMu.Lock()
	defer s.tokensMu.Unlock()
	s.tokens = append(s.tokens, tokens)

	return tokens
}

// DriveClient は、接続した Google Drive を読む Drive API のクライアントを返す。
func (s *Service) DriveClient(tokens *DriveTokens) *googledrive.Client {
	return googledrive.NewClient(s.http, s.apiURL, tokens)
}

// Connected は、Google Drive を接続していて、リフレッシュトークンが使えなくなっていないかどうかを返す。
func (t *DriveTokens) Connected(ctx context.Context) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	row, err := t.connection(ctx)
	if err != nil {
		return false, err
	}

	return row != nil && row.RevokedAt == nil, nil
}

// Folders は、管理者がライブラリとして見せると選んだフォルダを、選んだ順に返す。
func (t *DriveTokens) Folders(ctx context.Context) ([]*medialib.DriveFolder, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, err := t.connection(ctx); err != nil {
		return nil, err
	}

	return slices.Clone(t.folders), nil
}

// Token は、アクセストークンを返す。期限が近ければ、リフレッシュトークンで作り直す。リフレッシュトークンが使えなくなっていれば
// (取り消された、同意の画面が「テスト中」で 7 日たったなど)、接続にマークを付け、ErrNotConnected を返す。管理者は接続し直す。
func (t *DriveTokens) Token(ctx context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	row, err := t.connection(ctx)
	if err != nil {
		return "", err
	}
	if row == nil || row.RevokedAt != nil {
		return "", ErrNotConnected
	}
	now := t.service.clock.Now()
	if t.token != nil && slices.Equal(t.tokenFor, row.RefreshTokenEncrypted) && t.expiry.Add(-expiryMargin).After(now) {
		return t.token.AccessToken, nil
	}
	refresh, err := t.service.driveBox.Open(row.RefreshTokenEncrypted)
	if err != nil {
		return "", err
	}
	config, err := t.service.oauthConfig(ctx, t.db, "")
	if errors.Is(err, ErrUnavailable) {
		return "", ErrNotConnected
	}
	if err != nil {
		return "", err
	}
	token, err := config.TokenSource(t.service.httpContext(ctx), &oauth2.Token{RefreshToken: string(refresh)}).Token()
	if err != nil {
		if retrieve, ok := errors.AsType[*oauth2.RetrieveError](err); ok && retrieve.ErrorCode == "invalid_grant" {
			t.markRevoked(ctx, row)

			return "", fmt.Errorf("%w: the refresh token was rejected", ErrNotConnected)
		}

		return "", fmt.Errorf("failed to refresh the Google Drive token: %w", tokenError(err))
	}
	// 期限は、時計をそろえるため、Google が返した残りの長さから、この部品の時計で決める。
	expiry := token.Expiry
	if token.ExpiresIn > 0 {
		expiry = now.Add(time.Duration(token.ExpiresIn) * time.Second)
	}
	t.token, t.tokenFor, t.expiry = token, row.RefreshTokenEncrypted, expiry

	return token.AccessToken, nil
}

// Invalidate は、手元のアクセストークンを破棄する。Drive API が 401 を返したときに呼ばれる。
func (t *DriveTokens) Invalidate() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.token, t.tokenFor = nil, nil
}

// forget は、キャッシュした接続を破棄し、次に DB から読み直させる。
func (t *DriveTokens) forget() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.checkedAt = time.Time{}
}

// connection は、接続を返す。少し前に読んだものがあれば、DB を読まずに返す。接続していなければ nil を返す。t.mu を持って呼ぶ。
func (t *DriveTokens) connection(ctx context.Context) (*sqlc.GoogleDriveConnection, error) {
	now := t.service.clock.Now()
	if !t.checkedAt.IsZero() && now.Sub(t.checkedAt) < connectionTTL {
		return t.row, nil
	}
	q := sqlc.New(t.db)
	row, err := q.GetGoogleDriveConnection(ctx)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		t.row = nil
	case err != nil:
		return nil, fmt.Errorf("failed to get the Google Drive connection: %w", err)
	default:
		t.row = &row
	}
	rows, err := q.ListGoogleDriveFolders(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list the Google Drive folders: %w", err)
	}
	t.folders = make([]*medialib.DriveFolder, 0, len(rows))
	for _, folder := range rows {
		t.folders = append(t.folders, &medialib.DriveFolder{ID: folder.FolderID, DriveID: folder.DriveID, Library: folder.LibraryName})
	}
	t.checkedAt = now

	return t.row, nil
}

// markRevoked は、リフレッシュトークンが使えなくなったことを、接続に記録する。接続し直した後の行には付けない。
func (t *DriveTokens) markRevoked(ctx context.Context, row *sqlc.GoogleDriveConnection) {
	now := t.service.clock.Now()
	if _, err := sqlc.New(t.db).MarkGoogleDriveConnectionRevoked(ctx, sqlc.MarkGoogleDriveConnectionRevokedParams{
		Now: &now, RefreshTokenEncrypted: row.RefreshTokenEncrypted,
	}); err != nil {
		t.service.logger.WarnContext(ctx, "failed to mark the Google Drive connection as revoked", slog.Any("error", err))
	}
	t.service.logger.WarnContext(ctx, "the Google Drive refresh token was rejected; reconnect Google Drive on the integrations page")
	t.row = &sqlc.GoogleDriveConnection{
		ID: row.ID, RefreshTokenEncrypted: row.RefreshTokenEncrypted, AccountName: row.AccountName, ConnectedAt: row.ConnectedAt, RevokedAt: &now,
	}
	t.token, t.tokenFor = nil, nil
}

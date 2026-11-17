package graph

import (
	"context"
	"errors"
	"strings"
	"uuid"

	"github.com/SlashNephy/kotatsu/internal/lib/secretbox"
	"github.com/SlashNephy/kotatsu/internal/usecase/dramalibrary"
	"github.com/SlashNephy/kotatsu/internal/usecase/dramarecord"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// errNoDramaRecords は、見たドラマの記録を扱う部品を渡していないことを表す。
var errNoDramaRecords = errors.New("drama records are not configured")

// dramaRecordService は、見たドラマの記録を扱う部品を返す (#1649)。渡していなければ、内部のエラーにする。
func (r *Resolver) dramaRecordService() (*dramarecord.Service, error) {
	if r.dramaRecords == nil {
		return nil, errNoDramaRecords
	}

	return r.dramaRecords, nil
}

// dramaDestinationName は、GraphQL のドラマの記録先を、記録先の名前 (DB の destination) にする。
func dramaDestinationName(destination model.DramaRecordDestination) string {
	return strings.ToLower(string(destination))
}

// dramaDestinationOf は、記録先の名前を、GraphQL のドラマの記録先にする。
func dramaDestinationOf(name string) model.DramaRecordDestination {
	return model.DramaRecordDestination(strings.ToUpper(name))
}

// dramaRecordApps は、ドラマの記録先ごとに、管理者が登録したアプリを返す。
func (r *Resolver) dramaRecordApps(ctx context.Context) ([]*model.DramaRecordApp, error) {
	service, err := r.dramaRecordService()
	if err != nil {
		return nil, err
	}
	apps, err := service.Apps(ctx, r.db)
	if err != nil {
		return nil, err
	}
	origin := requestOrigin(ctx)
	result := make([]*model.DramaRecordApp, 0, len(apps))
	for _, app := range apps {
		result = append(result, &model.DramaRecordApp{
			Destination: dramaDestinationOf(app.Destination), ClientID: nonEmpty(app.ClientID), Configured: app.Configured,
			RedirectURI: dramarecord.RedirectURI(origin, app.Destination), UpdatedAt: app.UpdatedAt,
		})
	}

	return result, nil
}

// dramaRecordSettings は、ユーザー userID の、ドラマの記録先ごとの連携を返す。
func (r *Resolver) dramaRecordSettings(ctx context.Context, userID uuid.UUID) (*model.DramaRecordSettings, error) {
	service, err := r.dramaRecordService()
	if err != nil {
		return nil, err
	}
	availability, err := service.Available(ctx, r.db)
	if err != nil {
		return nil, err
	}
	connections, err := service.Connections(ctx, r.db, userID)
	if err != nil {
		return nil, err
	}
	connected := make(map[string]*dramarecord.Connection, len(connections))
	for _, connection := range connections {
		connected[connection.Destination] = connection
	}
	destinations := []*model.DramaRecordDestinationStatus{}
	for _, name := range service.Destinations() {
		status := &model.DramaRecordDestinationStatus{
			Destination: dramaDestinationOf(name), Available: availability.LinkingEnabled && availability.Configured[name], Connection: nil,
		}
		if c, ok := connected[name]; ok {
			status.Connection = &model.DramaRecordConnection{
				AccountName: c.AccountName, ConnectedAt: c.ConnectedAt, ReauthorizationRequired: c.ReauthorizationRequired,
				WatchlistCheckedAt: c.WatchlistCheckedAt, WatchlistImportedAt: c.WatchlistImportedAt, WatchlistError: c.WatchlistError,
			}
		}
		destinations = append(destinations, status)
	}

	percent, err := dramalibrary.ThresholdPercent(ctx, r.db, userID)
	if err != nil {
		return nil, err
	}

	return &model.DramaRecordSettings{Enabled: availability.LinkingEnabled, ThresholdPercent: percent, Destinations: destinations}, nil
}

// dramaRecordError は、ドラマの記録のエラーを、エラーコードを持つエラーにする。
func dramaRecordError(err error) error {
	switch {
	case errors.Is(err, dramarecord.ErrInvalidApp), errors.Is(err, dramarecord.ErrUnknownDestination):
		return NewError(CodeBadUserInput, err.Error())
	case errors.Is(err, dramarecord.ErrUnavailable):
		return NewError(CodeConflict, "drama records are not available; ask an administrator to enable them")
	case errors.Is(err, dramarecord.ErrSessionRequired):
		return NewError(CodeForbidden, "a browser session is required to connect")
	case errors.Is(err, dramarecord.ErrNotConnected):
		return NewError(CodeNotFound, "the destination is not connected")
	case errors.Is(err, secretbox.ErrDecrypt):
		// アプリの秘密キーを変えると、保存したシークレットやトークンを読めなくなる。中身は返さない。
		return NewError(CodeConflict, "the saved secret cannot be decrypted; register it again")
	default:
		return err
	}
}

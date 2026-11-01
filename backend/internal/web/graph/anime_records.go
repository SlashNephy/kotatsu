package graph

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/lib/clientinfo"
	"github.com/SlashNephy/kotatsu/internal/lib/secretbox"
	"github.com/SlashNephy/kotatsu/internal/usecase/animerecord"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// errNoAnimeRecords は、見たアニメの記録を扱う部品を渡していないことを表す。
var errNoAnimeRecords = errors.New("anime records are not configured")

// animeRecordService は、見たアニメの記録を扱う部品を返す。渡していなければ、内部のエラーにする。
func (r *Resolver) animeRecordService() (*animerecord.Service, error) {
	if r.animeRecords == nil {
		return nil, errNoAnimeRecords
	}

	return r.animeRecords, nil
}

// destinationName は、GraphQL の記録先を、記録先の名前 (DB の destination) にする。
func destinationName(destination model.AnimeRecordDestination) string {
	return strings.ToLower(string(destination))
}

// destinationOf は、記録先の名前を、GraphQL の記録先にする。
func destinationOf(name string) model.AnimeRecordDestination {
	return model.AnimeRecordDestination(strings.ToUpper(name))
}

// requestOrigin は、いま接続しているオリジンを返す。リダイレクト URI に使う。
func requestOrigin(ctx context.Context) string {
	if client := clientinfo.From(ctx); client != nil {
		return client.Origin()
	}

	return ""
}

// animeRecordApps は、記録先ごとに、管理者が登録したアプリを返す。
func (r *Resolver) animeRecordApps(ctx context.Context) ([]*model.AnimeRecordApp, error) {
	service, err := r.animeRecordService()
	if err != nil {
		return nil, err
	}
	apps, err := service.Apps(ctx, r.db)
	if err != nil {
		return nil, err
	}
	origin := requestOrigin(ctx)
	result := make([]*model.AnimeRecordApp, 0, len(apps))
	for _, app := range apps {
		result = append(result, &model.AnimeRecordApp{
			Destination: destinationOf(app.Destination), ClientID: nonEmpty(app.ClientID), Configured: app.Configured,
			RedirectURI: animerecord.RedirectURI(origin, app.Destination), UpdatedAt: app.UpdatedAt,
		})
	}

	return result, nil
}

// animeRecordSettings は、ユーザー userID の記録の設定と、記録先ごとの連携を返す。
func (r *Resolver) animeRecordSettings(ctx context.Context, userID uuid.UUID) (*model.AnimeRecordSettings, error) {
	service, err := r.animeRecordService()
	if err != nil {
		return nil, err
	}
	availability, err := service.Available(ctx, r.db)
	if err != nil {
		return nil, err
	}
	userSettings, err := animerecord.LoadSettings(ctx, r.db, userID)
	if err != nil {
		return nil, err
	}
	connections, err := service.Connections(ctx, r.db, userID)
	if err != nil {
		return nil, err
	}
	connected := make(map[string]*animerecord.Connection, len(connections))
	for _, connection := range connections {
		connected[connection.Destination] = connection
	}
	destinations := []*model.AnimeRecordDestinationStatus{}
	for _, name := range service.Destinations() {
		status := &model.AnimeRecordDestinationStatus{
			Destination: destinationOf(name), Available: availability.LinkingEnabled && availability.Configured[name], Connection: nil,
		}
		if connection, ok := connected[name]; ok {
			status.Connection = &model.AnimeRecordConnection{AccountName: connection.AccountName, ConnectedAt: connection.ConnectedAt}
		}
		destinations = append(destinations, status)
	}

	return &model.AnimeRecordSettings{
		Enabled: availability.LinkingEnabled, ThresholdPercent: userSettings.ThresholdPercent,
		WatchingOnFirst: userSettings.Rules.WatchingOnFirst, WatchingOnEpisode: userSettings.Rules.WatchingOnEpisode,
		WatchedOnLast: userSettings.Rules.WatchedOnLast, Destinations: destinations,
	}, nil
}

// toPlaybackTarget は、プレイヤーの報告の対象を、usecase の形にする。サービスか録画の、どちらか 1 つだけを受け付ける。
func toPlaybackTarget(serviceID, recordingID *string) (*animerecord.Target, error) {
	if (serviceID == nil) == (recordingID == nil) {
		return nil, NewError(CodeBadUserInput, "exactly one of serviceId and recordingId is required")
	}
	if serviceID != nil {
		id, err := parseServiceGlobalID(*serviceID)
		if err != nil {
			return nil, err
		}

		return &animerecord.Target{ServiceID: new(id), RecordingID: nil}, nil
	}
	id, err := parseRecordingID(*recordingID)
	if err != nil {
		return nil, err
	}

	return &animerecord.Target{ServiceID: nil, RecordingID: new(id)}, nil
}

// toIntervals は、プレイヤーが送った区間を、usecase の形にする。
func toIntervals(input []*model.PlaybackIntervalInput) ([]*animerecord.Interval, error) {
	if len(input) > animerecord.MaxReportedIntervals {
		return nil, NewError(CodeBadUserInput, "too many intervals")
	}
	intervals := make([]*animerecord.Interval, 0, len(input))
	for _, interval := range input {
		intervals = append(intervals, &animerecord.Interval{From: interval.From, To: interval.To})
	}

	return intervals, nil
}

// animeRecordError は、記録のエラーを、エラーコードを持つエラーにする。
func animeRecordError(err error) error {
	switch {
	case errors.Is(err, animerecord.ErrInvalidApp), errors.Is(err, animerecord.ErrInvalidSettings),
		errors.Is(err, animerecord.ErrInvalidReport), errors.Is(err, animerecord.ErrUnknownDestination):
		return NewError(CodeBadUserInput, err.Error())
	case errors.Is(err, animerecord.ErrUnavailable):
		return NewError(CodeConflict, "anime records are not available; ask an administrator to enable them")
	case errors.Is(err, animerecord.ErrSessionRequired):
		return NewError(CodeForbidden, "a browser session is required to connect")
	case errors.Is(err, animerecord.ErrNotConnected):
		return NewError(CodeNotFound, "the destination is not connected")
	case errors.Is(err, secretbox.ErrDecrypt):
		// アプリの秘密キーを変えると、保存したシークレットやトークンを読めなくなる。中身は返さない。
		return NewError(CodeConflict, "the saved secret cannot be decrypted; register it again")
	default:
		return err
	}
}

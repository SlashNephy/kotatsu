package graph

import (
	"context"
	"errors"
	"math"
	"strings"
	"unicode/utf8"

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
		WatchedOnLast: userSettings.Rules.WatchedOnLast, NotifyWishlist: userSettings.NotifyWishlist, Destinations: destinations,
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

// maxRecordTitleRunes は、手で記録するときに受け取る作品名の長さの上限。
const maxRecordTitleRunes = 200

// recordAnimeEpisodes は、ログイン中のユーザーが手で選んだ話を、連携した記録先ごとに記録するジョブを入れる (#1232)。
func (r *Resolver) recordAnimeEpisodes(ctx context.Context, input *model.RecordAnimeEpisodesInput) (bool, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return false, err
	}
	target, err := r.manualTarget(ctx, input)
	if err != nil {
		return false, err
	}
	if r.animeRecords == nil {
		return false, NewError(CodeConflict, "anime records are not available")
	}
	if err := r.animeRecords.RecordManually(ctx, r.db, r.jobs, principal.UserID, target); err != nil {
		switch {
		case errors.Is(err, animerecord.ErrNothingToRecord):
			return false, NewError(CodeBadUserInput, err.Error())
		case errors.Is(err, animerecord.ErrAlreadyRecorded):
			return false, NewError(CodeConflict, err.Error())
		default:
			return false, animeRecordError(err)
		}
	}

	return true, nil
}

// manualTarget は、手で記録する話の入力を、usecase の形にする。番組、録画、作品のどれか 1 つを受け付ける。
func (r *Resolver) manualTarget(ctx context.Context, input *model.RecordAnimeEpisodesInput) (*animerecord.ManualTarget, error) {
	given := 0
	for _, set := range []bool{input.Program != nil, input.Recording != nil, input.Tid != nil} {
		if set {
			given++
		}
	}
	if given != 1 {
		return nil, NewError(CodeBadUserInput, "exactly one of program, recording and tid is required")
	}
	switch {
	case input.Program != nil:
		node, err := (&queryResolver{r}).Node(ctx, *input.Program)
		if err != nil {
			return nil, err
		}
		program, ok := node.(*model.Program)
		if !ok {
			return nil, NewError(CodeBadUserInput, "program must be a program ID")
		}

		return &animerecord.ManualTarget{
			Program: &animerecord.ManualProgram{ServiceID: program.ServiceKey, StartAt: program.StartAt, Name: program.Name}, RecordingID: nil, Work: nil,
		}, nil
	case input.Recording != nil:
		id, err := parseRecordingID(*input.Recording)
		if err != nil {
			return nil, err
		}

		return &animerecord.ManualTarget{Program: nil, RecordingID: &id, Work: nil}, nil
	default:
		tid := *input.Tid
		if tid <= 0 || tid > math.MaxInt32 || input.Episode == nil {
			return nil, NewError(CodeBadUserInput, "tid and episode are required")
		}
		title := ""
		if input.Title != nil {
			title = strings.TrimSpace(*input.Title)
		}
		if utf8.RuneCountInString(title) > maxRecordTitleRunes {
			return nil, NewError(CodeBadUserInput, "title is too long")
		}

		return &animerecord.ManualTarget{
			Program: nil, RecordingID: nil, Work: &animerecord.ManualWork{TID: int32(tid), Number: *input.Episode, Title: title},
		}, nil
	}
}

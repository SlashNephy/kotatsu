package graph

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/registry"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/query"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/kyusu"
	"github.com/SlashNephy/kotatsu/internal/usecase/scramble"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
	"github.com/SlashNephy/kotatsu/internal/usecase/stream"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// createStreamURL は、サービスか録画の、署名付きの映像の URL を発行する (設計書 7.6、9.6)。
// 映像を見ることは誰にでも許されているので、管理者が外部プレイヤー用の URL を有効にしていれば (#1755)、ログインしている人に発行する。
func (r *Resolver) createStreamURL(ctx context.Context, target string) (*model.StreamURL, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	if r.streamSigner == nil {
		return nil, errors.New("stream URL signer is not configured")
	}
	system, err := settings.Load(ctx, r.db)
	if err != nil {
		return nil, err
	}
	if !system.ExternalPlayerURLsEnabled {
		return nil, NewError(CodeForbidden, "external player URLs are disabled by the administrator")
	}
	resource, err := r.streamResource(ctx, target)
	if err != nil {
		return nil, err
	}
	// URL ごとに記録を残し、署名に記録の ID を含める。管理者は、記録を消して、この URL だけを取り消せる (#1757)。
	urlID := id.New()
	signed := r.streamSigner.Sign(resource.path, principal.UserID, urlID, r.clock.Now().Add(resource.lifetime))
	err = r.streamURLs.Create(ctx, actorOf(ctx, principal), &auth.NewStreamURL{
		ID: urlID, UserID: principal.UserID, Resource: resource.path, TargetName: resource.name, ExpiresAt: signed.ExpiresAt,
	})
	if err != nil {
		return nil, err
	}

	return &model.StreamURL{URL: signed.URL, ExpiresAt: signed.ExpiresAt}, nil
}

// streamTarget は、署名付き URL を発行する対象。
type streamTarget struct {
	// path は、映像のパス。
	path string
	// name は、URL の記録に残す、サービス名か録画の名前 (#1757)。
	name string
	// lifetime は、署名付き URL の有効期限の長さ。
	lifetime time.Duration
}

// streamResource は、Service か Recording の ID から、署名付き URL を発行する対象を返す。ないものの URL は発行しない。
// 期限は、ライブなら一律の長さ、録画なら録画の長さに少し足した長さにする (#1756)。録画中の録画も、予定の長さで決める。
func (r *Resolver) streamResource(ctx context.Context, target string) (*streamTarget, error) {
	typeName, _, err := ParseGlobalID(target)
	if err != nil {
		return nil, NewError(CodeBadUserInput, "invalid target ID")
	}
	q := r.queries
	switch typeName {
	case serviceType:
		serviceID, err := parseServiceGlobalID(target)
		if err != nil {
			return nil, err
		}
		services, err := q.ListGuideServices(ctx, query.ListGuideServicesParams{
			IncludeHidden: true, MediaServiceTypes: []int64{}, IDs: []int64{serviceID},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to look up the service: %w", err)
		}
		if len(services) == 0 {
			return nil, NewError(CodeNotFound, "service not found")
		}

		return &streamTarget{path: stream.ServicePath(serviceID), name: services[0].Name, lifetime: stream.LiveURLLifetime}, nil
	case recordingType:
		recordingID, err := parseRecordingID(target)
		if err != nil {
			return nil, err
		}
		rows, err := q.ListRecordingsByIDs(ctx, []uuid.UUID{recordingID})
		if err != nil {
			return nil, fmt.Errorf("failed to look up the recording: %w", err)
		}
		if len(rows) == 0 {
			return nil, NewError(CodeNotFound, "recording not found")
		}

		return &streamTarget{
			path: stream.RecordingPath(recordingID), name: rows[0].Name,
			lifetime: stream.RecordingURLLifetime(rows[0].EndAt.Sub(rows[0].StartAt)),
		}, nil
	default:
		return nil, NewError(CodeBadUserInput, "target must be a Service or Recording ID")
	}
}

// transcoding は、サーバー側でのトランスコードを使えるかと、選べる画質を返す (設計書 7.1)。
// kyusu は、トランスコードできるときだけ登録簿に載るので、生きている kyusu がいれば使えるとみなす。
func (r *Resolver) transcoding(ctx context.Context) (*model.Transcoding, error) {
	if _, err := viewer(ctx); err != nil {
		return nil, err
	}
	// 登録簿を読むだけなので、このインスタンスの ID は使わない。
	instances, err := registry.New(r.db, r.clock, uuid.Nil()).ListAlive(ctx, kyusu.Role)
	if err != nil {
		return nil, err
	}
	presets := make([]*model.StreamPreset, 0, len(kyusu.Presets()))
	for _, preset := range kyusu.Presets() {
		presets = append(presets, &model.StreamPreset{
			Name: preset.Name, Height: preset.Height, VideoBitrate: preset.VideoBitrate, Codec: preset.Codec, Passthrough: preset.Passthrough,
		})
	}

	return &model.Transcoding{Available: len(instances) > 0, Presets: presets}, nil
}

// bs4kChannelType は、BS4K のチャンネル種別。Mahiron が返す (設計書 3.1)。
const bs4kChannelType = "BS4K"

// videoFormat は、映像の解像度 (分からなければ nil) とチャンネル種別から、映像の種類を決める (#806)。
// BS4K のチャンネルは、番組の解像度が分からなくても、2K と書かれていても、MPEG-2 ではないので UHD にする。
func videoFormat(resolution *string, channelType string) model.VideoFormat {
	switch {
	case resolution != nil && *resolution == "4320p":
		return model.VideoFormatShv
	case resolution != nil && *resolution == "2160p", channelType == bs4kChannelType:
		return model.VideoFormatUhd
	default:
		return model.VideoFormatHd
	}
}

// serviceVideoFormat は、サービスで放送中の番組の映像の種類を返す。放送中の番組がなければ、チャンネル種別から決める。
func (r *Resolver) serviceVideoFormat(ctx context.Context, obj *model.Service) (model.VideoFormat, error) {
	resolution, err := r.queries.GetPresentVideoResolution(ctx, query.GetPresentVideoResolutionParams{
		ServiceID: obj.MirakurunID(), Now: r.clock.Now(),
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("failed to get the video resolution of the present program: %w", err)
	}

	return videoFormat(resolution, obj.ChannelTypeKey), nil
}

// recordingVideoFormat は、録画の映像の種類を、録画を始めたときの番組の解像度と、サービスのチャンネル種別から返す。
func (r *Resolver) recordingVideoFormat(ctx context.Context, obj *model.Recording) (model.VideoFormat, error) {
	service, err := loadersFrom(ctx).Services.Load(ctx, obj.ServiceKey)()
	if err != nil {
		return "", err
	}

	return videoFormat(obj.VideoResolution, service.ChannelType), nil
}

// errLiveRatesUnavailable は、内訳を数えられない (チューナーサーバーを設定していない) ときのエラー。
var errLiveRatesUnavailable = errors.New("live stream rates are unavailable")

// subscribeLiveStreamRates は、ライブのサービス service の、PID の種類ごとのビットレートを送る (#812)。
func (r *Resolver) subscribeLiveStreamRates(ctx context.Context, service string) (<-chan *model.LiveStreamRates, error) {
	if _, err := viewer(ctx); err != nil {
		return nil, err
	}
	serviceID, err := parseServiceGlobalID(service)
	if err != nil {
		return nil, err
	}
	if r.liveRates == nil {
		return nil, errLiveRatesUnavailable
	}
	rates, err := r.liveRates.Watch(ctx, serviceID)
	switch {
	case errors.Is(err, stream.ErrNoTuner):
		return nil, errLiveRatesUnavailable
	case err != nil:
		r.logger.WarnContext(ctx, "failed to watch the live stream rates", slog.Int64("service_id", serviceID), slog.Any("error", err))

		return nil, errLiveRatesUnavailable
	}
	out := make(chan *model.LiveStreamRates, 1)
	go func() {
		defer close(out)
		for rate := range rates {
			select {
			case out <- liveStreamRatesOf(rate):
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}

// subscribeLiveStreamScrambled は、ライブのサービスが、スクランブルされたまま届くと決めたら送る (設計書 7.1、#1517)。viewerEvents に載せる。
// 購読を始めたときにサービスごとに最後に決めた日時を控え、それより後に決めたものだけを送る。前に見た人の判定で、今の視聴を止めないためである。
// 変更の通知が設定されていなければ、何も送らない。
func (r *Resolver) subscribeLiveStreamScrambled(ctx context.Context) (<-chan *model.LiveStreamScrambled, error) {
	out := make(chan *model.LiveStreamScrambled, 1)
	if r.changes == nil || r.db == nil {
		return out, nil
	}
	// 決めた日時を読む前に購読し、読んだ後に決めたものを取りこぼさないようにする。
	wake, unsubscribe := r.changes.Subscribe()
	channelVersion := r.changes.Version(scramble.NotifyChannel)
	last, err := r.liveScramble.Verdicts(ctx)
	if err != nil {
		unsubscribe()

		return nil, err
	}
	go func() {
		defer close(out)
		defer unsubscribe()
		for {
			select {
			case <-ctx.Done():
				return
			case <-wake:
			}
			latest := r.changes.Version(scramble.NotifyChannel)
			if latest <= channelVersion {
				continue
			}
			channelVersion = latest
			verdicts, err := r.liveScramble.Verdicts(ctx)
			if err != nil {
				// 閉じれば、クライアントがつなぎ直す。
				return
			}
			for _, serviceID := range slices.Sorted(maps.Keys(verdicts)) {
				decidedAt := verdicts[serviceID]
				if previous, ok := last[serviceID]; ok && !decidedAt.After(previous) {
					continue
				}
				last[serviceID] = decidedAt
				event := &model.LiveStreamScrambled{Service: GlobalID(serviceType, strconv.FormatInt(serviceID, 10)), DecidedAt: decidedAt}
				select {
				case <-ctx.Done():
					return
				case out <- event:
				}
			}
		}
	}()

	return out, nil
}

// liveStreamRatesOf は、ライブのサービスの内訳を、GraphQL の型にする。
func liveStreamRatesOf(rate *stream.LiveRates) *model.LiveStreamRates {
	rates := &model.LiveStreamRates{
		VideoBps:       int(rate.BPS.Video),
		AudioBps:       int(rate.BPS.Audio),
		OtherBps:       int(rate.BPS.Other),
		PsiBps:         int(rate.BPS.PSI),
		UnknownBps:     int(rate.BPS.Unknown),
		TotalBps:       int(rate.TotalBPS),
		Drops:          rate.Drops,
		Errors:         rate.Errors,
		Scrambled:      rate.Scrambled,
		BroadcastClock: nil,
		ServerTime:     rate.ServerTime,
	}
	if rate.Clock != nil {
		rates.BroadcastClock = &model.BroadcastClock{Pcr: rate.Clock.PCR, ReceivedAt: rate.Clock.ReceivedAt}
	}

	return rates
}

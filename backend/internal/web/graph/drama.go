package graph

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"

	"github.com/SlashNephy/kotatsu/internal/usecase/drama"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// tmdbWebURL は、TMDb の作品のページのオリジン。
const tmdbWebURL = "https://www.themoviedb.org"

// dramaEpisodeOf は、結びついた TMDb の作品と話を GraphQL の形にする。
func dramaEpisodeOf(episode *drama.LinkedEpisode) *model.DramaEpisode {
	seriesURL := fmt.Sprintf("%s/tv/%d", tmdbWebURL, episode.SeriesID)
	result := &model.DramaEpisode{
		Series: &model.DramaSeries{
			TmdbID: int(episode.SeriesID), Name: episode.SeriesName, OriginalName: episode.SeriesOriginalName, URL: seriesURL,
			WatchStatus: nil,
		},
		SeriesSource:  model.DramaSeriesSourceSearch,
		SeasonNumber:  intOf(episode.SeasonNumber),
		EpisodeNumber: intOf(episode.EpisodeNumber),
		EpisodeName:   episode.EpisodeName,
		EpisodeSource: nil,
		URL:           seriesURL,
		Watched:       false,
	}
	if episode.SeriesSource == drama.SeriesSourceOverride {
		result.SeriesSource = model.DramaSeriesSourceOverride
	}
	switch episode.EpisodeSource {
	case drama.EpisodeSourceAirDate:
		result.EpisodeSource = new(model.DramaEpisodeSourceAirDate)
	case drama.EpisodeSourceTitle:
		result.EpisodeSource = new(model.DramaEpisodeSourceTitle)
	}
	if episode.SeasonNumber != nil && episode.EpisodeNumber != nil {
		result.URL = fmt.Sprintf("%s/season/%d/episode/%d", seriesURL, *episode.SeasonNumber, *episode.EpisodeNumber)
	}

	return result
}

// dramaTitleOverrides は、作品名ごとの上書きを、すべて返す。
func (r *Resolver) dramaTitleOverrides(ctx context.Context) ([]*model.DramaTitleOverride, error) {
	overrides, err := drama.ListOverrides(ctx, r.db)
	if err != nil {
		return nil, err
	}
	result := make([]*model.DramaTitleOverride, 0, len(overrides))
	for _, override := range overrides {
		result = append(result, &model.DramaTitleOverride{
			TitleKey: override.TitleKey, TmdbSeriesID: intOf(override.SeriesID), UpdatedAt: override.UpdatedAt,
		})
	}

	return result, nil
}

// tmdbSeriesIDOf は、GraphQL の Int を、TMDb の作品の ID にする。1 以上で、int32 に収まらなければ BAD_USER_INPUT を返す。
func tmdbSeriesIDOf(value int) (int32, error) {
	if value <= 0 || value > math.MaxInt32 {
		return 0, NewError(CodeBadUserInput, "tmdbSeriesId must be a positive TMDb series ID")
	}

	return int32(value), nil
}

// dramaTitleKeyOf は、上書きの変更の入力から、作品名の鍵を返す。番組名と鍵のどちらか 1 つだけを受け付ける。
func dramaTitleKeyOf(input *model.DramaTitleOverrideInput) (string, error) {
	switch {
	case input.ProgramName != nil && input.TitleKey == nil:
		return drama.TitleKeyOf(*input.ProgramName), nil
	case input.ProgramName == nil && input.TitleKey != nil:
		// 一覧の鍵はもう鍵の形なので、そのまま同じ鍵になる。空白や記号だけの値は空になり、BAD_USER_INPUT にする。
		return drama.TitleKey(*input.TitleKey), nil
	default:
		return "", NewError(CodeBadUserInput, "pass exactly one of programName and titleKey")
	}
}

// dramaError は、上書きのエラーを、エラーコードを持つエラーにする。
func dramaError(err error) error {
	if errors.Is(err, drama.ErrInvalidOverride) {
		return NewError(CodeBadUserInput, "pass exactly one of tmdbSeriesId, exclude and reset with a program name that has a title")
	}
	if errors.Is(err, drama.ErrInvalidToken) {
		return NewError(CodeBadUserInput, err.Error())
	}

	return err
}

// errNoDramaTokens は、TMDb のトークンを扱う部品を渡していないことを表す。
var errNoDramaTokens = errors.New("the TMDb token store is not configured")

// dramaTokenStore は、TMDb のトークンを扱う部品を返す。渡していなければ内部のエラーを返す。
func (r *Resolver) dramaTokenStore() (*drama.Tokens, error) {
	if r.dramaTokens == nil {
		return nil, errNoDramaTokens
	}

	return r.dramaTokens, nil
}

// dramaLinking は、TMDb との連携の様子を返す。トークンそのものは返さない。
func (r *Resolver) dramaLinking(ctx context.Context) (*model.DramaLinkingStatus, error) {
	tokens, err := r.dramaTokenStore()
	if err != nil {
		return nil, err
	}
	status, err := tokens.Status(ctx, r.db)
	if err != nil {
		return nil, err
	}
	result := &model.DramaLinkingStatus{TokenConfigured: status.Configured, TokenHint: nil, TokenUpdatedAt: status.UpdatedAt}
	if status.Hint != "" {
		result.TokenHint = new(status.Hint)
	}

	return result, nil
}

// startDramaLinking は、これから 1 週間の番組を照合するジョブを入れる。入れられなくても、6 時間ごとの定期ジョブが照合するので、
// 設定の変更は失敗にしない。
func (r *Resolver) startDramaLinking(ctx context.Context) {
	if r.jobs == nil {
		return
	}
	if err := drama.EnqueueStart(ctx, r.db, r.jobs); err != nil {
		r.logger.WarnContext(ctx, "failed to enqueue the drama match", slog.Any("error", err))
	}
}

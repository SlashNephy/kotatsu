package graph

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"strconv"
	"strings"

	"github.com/SlashNephy/kotatsu/internal/usecase/anime"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// animeChannel は、サービス serviceID のチャンネルの対応を返す。
func (r *Resolver) animeChannel(ctx context.Context, serviceID int64) (*model.AnimeChannel, error) {
	channels, err := anime.ListChannels(ctx, r.db)
	if err != nil {
		return nil, err
	}
	for _, channel := range channels {
		if channel.ServiceID == serviceID {
			return animeChannelOf(channel), nil
		}
	}

	return nil, NewError(CodeNotFound, "service not found")
}

// animeChannelOf は、チャンネルの対応を GraphQL の形にする。
func animeChannelOf(channel *anime.Channel) *model.AnimeChannel {
	return &model.AnimeChannel{
		ServiceID:                 serviceGlobalID(channel.ServiceID),
		DefinitionName:            channel.DefinitionName,
		ImportedSyobocalChannelID: intOf(channel.ImportedSyobocalChannelID),
		ImportedAnnictChannelID:   intOf(channel.ImportedAnnictChannelID),
		SyobocalChannelIDOverride: intOf(channel.OverrideSyobocalChannelID),
		AnnictChannelIDOverride:   intOf(channel.OverrideAnnictChannelID),
		SyobocalChannelID:         intOf(channel.SyobocalChannelID),
		AnnictChannelID:           intOf(channel.AnnictChannelID),
	}
}

// animeImports は、連携先の情報を取り込んだ様子を返す。
func (r *Resolver) animeImports(ctx context.Context) ([]*model.AnimeImport, error) {
	imports, err := anime.ListImports(ctx, r.db)
	if err != nil {
		return nil, err
	}
	result := make([]*model.AnimeImport, 0, len(imports))
	for _, status := range imports {
		result = append(result, &model.AnimeImport{
			Source:     model.AnimeImportSource(strings.ToUpper(status.Source)),
			CheckedAt:  status.CheckedAt,
			ImportedAt: status.ImportedAt,
			ItemCount:  status.ItemCount,
			Error:      status.Error,
		})
	}

	return result, nil
}

// toAnimeChannelChanges は、チャンネルの対応の上書きの入力を、変える項目にする。
func toAnimeChannelChanges(input *model.AnimeChannelInput) (*anime.ChannelChanges, error) {
	changes := &anime.ChannelChanges{
		SyobocalChannelID: nil, ResetSyobocalChannelID: input.ResetSyobocalChannelID,
		AnnictChannelID: nil, ResetAnnictChannelID: input.ResetAnnictChannelID,
	}
	if err := setInt32("syobocalChannelId", input.SyobocalChannelID, &changes.SyobocalChannelID); err != nil {
		return nil, err
	}
	if err := setInt32("annictChannelId", input.AnnictChannelID, &changes.AnnictChannelID); err != nil {
		return nil, err
	}

	return changes, nil
}

// setInt32 は、GraphQL の Int を int32 にして target に入れる。value が nil なら何もしない。範囲を超えていれば BAD_USER_INPUT を返す。
func setInt32(name string, value *int, target **int32) error {
	if value == nil {
		return nil
	}
	if *value < math.MinInt32 || *value > math.MaxInt32 {
		return NewError(CodeBadUserInput, name+" is out of range")
	}
	*target = new(int32(*value))

	return nil
}

// intOf は、DB の int32 を GraphQL の Int にする。
func intOf(value *int32) *int {
	if value == nil {
		return nil
	}

	return new(int(*value))
}

// animeError は、チャンネルの対応のエラーを、エラーコードを持つエラーにする。
func animeError(err error) error {
	if errors.Is(err, anime.ErrInvalidChannel) {
		return NewError(CodeBadUserInput, err.Error())
	}
	if errors.Is(err, anime.ErrServiceNotFound) {
		return NewError(CodeNotFound, "service not found")
	}

	return err
}

// startAnimeLinking は、連携先の情報を取り込むジョブを入れる。入れられなくても、1 時間ごとの定期ジョブが取り込むので、設定の変更は失敗にしない。
func (r *Resolver) startAnimeLinking(ctx context.Context) {
	if r.jobs == nil {
		return
	}
	if err := anime.EnqueueStart(ctx, r.db, r.jobs); err != nil {
		r.logger.WarnContext(ctx, "failed to enqueue the anime import", slog.Any("error", err))
	}
}

// animeEpisodesOf は、結びついた話を GraphQL の形にする。
func animeEpisodesOf(episodes []*anime.LinkedEpisode) []*model.AnimeEpisode {
	result := make([]*model.AnimeEpisode, 0, len(episodes))
	for _, episode := range episodes {
		works := make([]*model.AnimeWork, 0, len(episode.Works))
		for _, work := range episode.Works {
			works = append(works, &model.AnimeWork{
				AnnictWorkID: intOf(work.AnnictWorkID), AnilistID: intOf(work.AniListID), MalID: intOf(work.MALID),
				AnnictURL: workURL("https://annict.com/works/", work.AnnictWorkID), AnilistURL: workURL("https://anilist.co/anime/", work.AniListID),
				MalURL: workURL("https://myanimelist.net/anime/", work.MALID),
			})
		}
		source := model.AnimeEpisodeSourceSyobocal
		if episode.Source == anime.SourceTitle {
			source = model.AnimeEpisodeSourceTitle
		}
		result = append(result, &model.AnimeEpisode{
			Source: source, SyobocalTid: intOf(episode.TID), SyobocalURL: workURL("https://cal.syoboi.jp/tid/", episode.TID),
			CountStart: episode.CountStart, CountEnd: episode.CountEnd, Subtitle: episode.Subtitle,
			First: episode.First, Last: episode.Last, Rerun: episode.Rerun, Works: works,
		})
	}

	return result
}

// workURL は、作品の ID のページを返す。ID がなければ nil を返す。
func workURL(base string, id *int32) *string {
	if id == nil {
		return nil
	}

	return new(base + strconv.FormatInt(int64(*id), 10))
}

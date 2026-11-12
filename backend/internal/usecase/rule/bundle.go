package rule

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/domain/episode"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// RecordedEpisodeWindow は、自分で録った録画を「録った回」として見る期間 (付録 A、#1498)。
// 同じ回の再放送が来るまでの間隔の分布を、Kotatsu の番組表と録画で測る前に置いた初期値である。
// アニメやドラマの再放送は、数か月から 1 年ほど後に来ることが多いと見込んで 1 年にする。短すぎると、少し間を空けた再放送をもう一度録る。
// 長すぎると、録画が増えるほど照合が重くなる。ほかの録画ソフトから引き継いだ履歴は、この期間で切らない。
const RecordedEpisodeWindow = 365 * day

// terrestrialChannelType は、地上波のチャンネル種別。これ以外の局の放送を、束から 1 本を選ぶときの「衛星」とする。
const terrestrialChannelType = "GR"

// recordedEpisode は、「録った回」の録画。束ねた放送を録画済みとして外した理由に添える。
type recordedEpisode struct {
	serviceID int64
	name      string
	endAt     time.Time
}

// bundleSameEpisodes は、同じ回を束ねるルールに一致した放送を、局をまたいで同じ回で束ね、束ごとに 1 本だけを残す (設計書 4.4、#1498)。
// 束から外した放送からは、同じ回を束ねるルールだけを外す。どのルールにも一致しなくなった放送は、予約すべき集合から除き、
// skipped が nil でなければ、外したルールと理由と一緒に入れる。手動の予約は、比べる相手に入れない。
// 別のルールが残った放送は、外したルールと理由を partial に入れる。後で前の回を録らない (#1499) 設定で外れたときに、
// 束ねた理由を優先して出すためである。
func bundleSameEpisodes(
	ctx context.Context, tx pgx.Tx, now time.Time, matched map[programKey]*desiredProgram,
	skipped, partial map[programKey]*skippedProgram,
) error {
	keys := []programKey{}
	for key, d := range matched {
		if slices.ContainsFunc(d.rules, func(r *compiledRule) bool { return r.BundleSameEpisodes }) {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	// 人が取り消した (無効にした) 予約の番組は、束ねる前に外す。選ぶと、無効の予約のまま、ほかの放送も外してしまう。
	cancelled, err := cancelledPrograms(ctx, tx, now)
	if err != nil {
		return err
	}
	keys = slices.DeleteFunc(keys, func(key programKey) bool { return cancelled[key] })
	if len(keys) == 0 {
		return nil
	}
	slices.SortFunc(keys, compareKeys)

	broadcasts, err := listBundledBroadcasts(ctx, tx, now, matched, keys)
	if err != nil {
		return err
	}
	recorded, refs, err := listRecordedEpisodes(ctx, tx, now)
	if err != nil {
		return err
	}
	index := make(map[*episode.Broadcast]programKey, len(broadcasts))
	for i, b := range broadcasts {
		index[b] = keys[i]
	}

	for i, decision := range episode.Resolve(broadcasts, recorded) {
		if decision.Recorded == nil && decision.Instead == nil {
			continue
		}
		key := keys[i]
		d := matched[key]
		removed := []*compiledRule{}
		d.rules = slices.DeleteFunc(d.rules, func(r *compiledRule) bool {
			if r.BundleSameEpisodes {
				removed = append(removed, r)
			}

			return r.BundleSameEpisodes
		})
		s := &skippedProgram{
			row: d.row, rules: removed, reason: SkipReasonOtherBroadcast, lastRecorded: nil, recorded: nil, instead: nil,
			laterEpisode: 0, byRecording: false,
		}
		if decision.Recorded != nil {
			s.reason = SkipReasonRecordedEpisode
			s.recorded = refs[decision.Recorded]
			s.lastRecorded = new(s.recorded.endAt)
		} else {
			s.instead = new(index[decision.Instead])
		}
		if len(d.rules) > 0 {
			// この設定を入れていない別のルールも一致していれば、そのルールが受け持って予約する。
			partial[key] = s

			continue
		}
		delete(matched, key)
		if skipped != nil {
			skipped[key] = s
		}
	}

	return nil
}

// cancelledPrograms は、人が取り消した (無効にした) 予約のある、まだ終わっていない番組を返す (#1504)。
func cancelledPrograms(ctx context.Context, tx pgx.Tx, now time.Time) (map[programKey]bool, error) {
	rows, err := sqlc.New(tx).ListCancelledBroadcasts(ctx, now)
	if err != nil {
		return nil, fmt.Errorf("failed to list cancelled broadcasts: %w", err)
	}
	cancelled := make(map[programKey]bool, len(rows))
	for _, row := range rows {
		cancelled[programKey{serviceID: row.ServiceID, eventID: *row.EventID}] = true
	}

	return cancelled, nil
}

// listBundledBroadcasts は、keys の番組を、束ねる候補の放送にする。返す値は keys と同じ並び。
func listBundledBroadcasts(
	ctx context.Context, tx pgx.Tx, now time.Time, matched map[programKey]*desiredProgram, keys []programKey,
) ([]*episode.Broadcast, error) {
	q := sqlc.New(tx)
	serviceIDs := make([]int64, 0, len(keys))
	startAts := make([]time.Time, 0, len(keys))
	for _, key := range keys {
		serviceIDs = append(serviceIDs, key.serviceID)
		startAts = append(startAts, matched[key].row.StartAt)
	}
	animeRows, err := q.ListProgramAnimeEpisodesForBundling(ctx, sqlc.ListProgramAnimeEpisodesForBundlingParams{
		ServiceIds: serviceIDs, StartAts: startAts,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list the anime episodes of the programs: %w", err)
	}
	type animeKey struct {
		serviceID int64
		startAt   int64
	}
	anime := make(map[animeKey]*episode.Anime, len(animeRows))
	for _, row := range animeRows {
		anime[animeKey{serviceID: row.ServiceID, startAt: row.StartAt.UnixNano()}] = &episode.Anime{TID: row.SyobocalTid, Count: row.Count}
	}
	rejected, err := conflictingPrograms(ctx, tx, now)
	if err != nil {
		return nil, err
	}

	broadcasts := make([]*episode.Broadcast, 0, len(keys))
	for _, key := range keys {
		row := matched[key].row
		src := programSource(&row)
		src.Anime = anime[animeKey{serviceID: key.serviceID, startAt: row.StartAt.UnixNano()}]
		broadcasts = append(broadcasts, &episode.Broadcast{
			Episode:   episode.Parse(src),
			ServiceID: key.serviceID,
			EventID:   key.eventID,
			StartAt:   row.StartAt,
			EndAt:     row.EndAt,
			Captioned: episode.Captioned(row.Name),
			Satellite: row.ChannelType != terrestrialChannelType,
			Rejected:  rejected[key],
		})
	}

	return broadcasts, nil
}

// programSource は、照合する番組から、回を読む元を作る。しょぼいカレンダーの話は入れない。
func programSource(row *sqlc.ListMatchableProgramsRow) *episode.Source {
	extended := ""
	if row.ExtendedNormalized != nil {
		extended = *row.ExtendedNormalized
	}

	return &episode.Source{
		Name: row.Name, Description: row.DescriptionNormalized, Extended: extended, SeriesEpisode: row.SeriesEpisode, Anime: nil,
	}
}

// conflictingPrograms は、割り当ての結果が競合の、これから放送する番組の予約を返す。録れない見込みの放送として扱う (#1498、#1499)。
func conflictingPrograms(ctx context.Context, tx pgx.Tx, now time.Time) (map[programKey]bool, error) {
	conflicts, err := sqlc.New(tx).ListConflictingReservedPrograms(ctx, now)
	if err != nil {
		return nil, fmt.Errorf("failed to list the conflicting reservations: %w", err)
	}
	rejected := make(map[programKey]bool, len(conflicts))
	for _, row := range conflicts {
		rejected[programKey{serviceID: row.ServiceID, eventID: row.EventID}] = true
	}

	return rejected, nil
}

// listRecordedEpisodes は、「録った回」を読む。録画履歴 (録った回を見る期間の内に始まったものと、引き継いだもの) と、録画中の録画。
func listRecordedEpisodes(ctx context.Context, tx pgx.Tx, now time.Time) ([]*episode.Recorded, map[*episode.Recorded]*recordedEpisode, error) {
	rows, err := sqlc.New(tx).ListRecordedEpisodes(ctx, now.Add(-RecordedEpisodeWindow))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list the recorded episodes: %w", err)
	}
	recorded := make([]*episode.Recorded, 0, len(rows))
	refs := make(map[*episode.Recorded]*recordedEpisode, len(rows))
	for _, row := range rows {
		var anime *episode.Anime
		if row.SyobocalTid != nil && row.AnimeCount != nil {
			anime = &episode.Anime{TID: *row.SyobocalTid, Count: *row.AnimeCount}
		}
		src := &episode.Source{Name: row.Name, Description: row.Description, Extended: "", SeriesEpisode: row.SeriesEpisode, Anime: anime}
		r := &episode.Recorded{Episode: episode.Parse(src)}
		recorded = append(recorded, r)
		refs[r] = &recordedEpisode{serviceID: row.ServiceID, name: row.Name, endAt: row.EndAt.UTC()}
	}

	return recorded, refs, nil
}

// SkipReason は、ルールに一致した番組を予約しなかった理由。
type SkipReason string

const (
	// SkipReasonDuplicateTitle は、同じ番組名を期間内に録画済みなので、重複の回避で外したこと (設計書 4.4)。
	SkipReasonDuplicateTitle SkipReason = "duplicate_title"
	// SkipReasonRecordedEpisode は、同じ回を録画済み (録画中を含む) なので外したこと (#1498)。
	SkipReasonRecordedEpisode SkipReason = "recorded_episode"
	// SkipReasonOtherBroadcast は、同じ回を別の放送で録るので外したこと (#1498)。
	SkipReasonOtherBroadcast SkipReason = "other_broadcast"
	// SkipReasonEarlierEpisode は、追っているシリーズの前の回なので外したこと (#1499)。
	SkipReasonEarlierEpisode SkipReason = "earlier_episode"
)

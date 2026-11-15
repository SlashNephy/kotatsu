// Package nextepisode は、録画のシリーズの続きの放送 (#1564、設計書 4.4) を探す。
package nextepisode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/domain/episode"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

const (
	// Horizon は、続きの放送を探す先の長さ。番組表の、これから 1 週間の放送から探す (付録 A)。
	Horizon = 7 * 24 * time.Hour
	// MaxBroadcasts は、返す続きの放送の最大の数 (付録 A)。毎日の再放送が並んでも、画面を埋めないよう抑える。
	MaxBroadcasts = 10
	// maxCandidates は、DB から読む候補の最大の数。予約のある放送と、同じイベントを共有する放送を外した後で MaxBroadcasts を残せるよう、多めに読む。
	maxCandidates = 100
)

// Broadcast は、続きの放送。
type Broadcast struct {
	ServiceID int64
	EventID   int64
	StartAt   time.Time
	// Number は、放送の話数。シリーズ名で探したときは番組名などから読んだ話数、TID で探したときはしょぼいカレンダーの話数。
	Number float64
}

// Key は、続きの放送を探す鍵。強い順に、しょぼいカレンダーの TID と話数、番組名から読んだシリーズ名と話数を使う。
type Key struct {
	// TID と Count は、録画が 1 つの話に結びついたときの、しょぼいカレンダーの作品と話数。結びついていなければ TID は 0。
	TID   int32
	Count float64
	// Series と Number は、回の読み方 (設計書 4.4) で読んだシリーズ名と話数。
	Series string
	Number int
}

// KeyOf は、録画の回を読む元 src から、続きの放送を探す鍵を返す。シリーズも話数も読めなければ (ニュースや帯番組など) nil を返す。
//
// 録画がしょぼいカレンダーの 1 つの話に結びついていれば、TID と話数で探す。番組名の話数と数え方が違うことがあるので、
// しょぼいカレンダーの話数は、しょぼいカレンダーの話数とだけ比べる。結びついていなければ、シリーズ名の完全一致と話数で探す。
// シリーズ名は、前の回を録らない設定 (#1499) と同じく、SameSeries のように緩めない。局ごとの飾りの違いで、別の期や編の回を出さないためである。
func KeyOf(src *episode.Source) *Key {
	if src.Anime != nil && src.Anime.TID > 0 && src.Anime.Count > 0 {
		return &Key{TID: src.Anime.TID, Count: src.Anime.Count, Series: "", Number: 0}
	}
	ep := episode.Parse(src)
	if ep.Series == "" || ep.Number <= 0 {
		return nil
	}

	return &Key{TID: 0, Count: 0, Series: ep.Series, Number: ep.Number}
}

// Candidate は、続きの放送の候補。DB から読んだ番組。
type Candidate struct {
	Broadcast

	// RelatedItems は、上流の番組の relatedItems の JSON。同じイベントを共有する放送を見分けるのに使う。
	RelatedItems []byte
}

// ProgramKey は、番組 (サービス, eventId)。
type ProgramKey struct {
	ServiceID int64
	EventID   int64
}

// Pick は、放送の早い順に並んだ候補 candidates から、続きの放送を選ぶ。DB は使わない。
//
//   - 予約のある番組と、予約のある番組と同じイベントを共有する番組は外す。録るかどうかが、もう決まっているためである。
//     無効にした (取り消した) 予約も含む。人が録らないと決めた放送を、また薦めないためである。
//   - recorded にある話数の放送は、同じ回を録画済みとして外す。
//   - 同じイベントを共有する番組 (メインチャンネルとサブチャンネルなど) が並んだら、ルールの照合と同じく、
//     サービス ID の最も小さい 1 つだけを残す。
//   - MaxBroadcasts までを返す。
func Pick(candidates []*Candidate, reserved map[ProgramKey]bool, recorded map[float64]bool) []*Broadcast {
	type entry struct {
		candidate *Candidate
		shared    []*domain.SharedProgram
	}
	entries := make([]*entry, 0, len(candidates))
	present := map[ProgramKey]bool{}
	for _, c := range candidates {
		shared, err := domain.SharedPrograms(c.ServiceID, c.RelatedItems)
		if err != nil {
			shared = []*domain.SharedProgram{}
		}
		taken := reserved[ProgramKey{ServiceID: c.ServiceID, EventID: c.EventID}] ||
			slices.ContainsFunc(shared, func(p *domain.SharedProgram) bool {
				return reserved[ProgramKey{ServiceID: p.ServiceID, EventID: p.EventID}]
			})
		if taken || recorded[c.Number] {
			continue
		}
		entries = append(entries, &entry{candidate: c, shared: shared})
		present[ProgramKey{ServiceID: c.ServiceID, EventID: c.EventID}] = true
	}
	broadcasts := []*Broadcast{}
	for _, e := range entries {
		// 同じイベントを、サービス ID のより小さいサービスでも放送していれば、そちらを残す。
		if slices.ContainsFunc(e.shared, func(p *domain.SharedProgram) bool {
			return p.ServiceID < e.candidate.ServiceID && present[ProgramKey{ServiceID: p.ServiceID, EventID: p.EventID}]
		}) {
			continue
		}
		broadcasts = append(broadcasts, new(e.candidate.Broadcast))
		if len(broadcasts) == MaxBroadcasts {
			break
		}
	}

	return broadcasts
}

// Find は、録画 recordingID のシリーズの、now から Horizon までに始まる続きの放送を、放送の早い順に返す (#1564)。
// 録画が無いか、シリーズも話数も読めない録画では、空を返す。
func Find(ctx context.Context, db sqlc.DBTX, recordingID uuid.UUID, now time.Time) ([]*Broadcast, error) {
	q := sqlc.New(db)
	row, err := q.GetRecordingNextEpisodeSource(ctx, recordingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return []*Broadcast{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the recording: %w", err)
	}
	key := KeyOf(sourceOf(&row))
	if key == nil {
		return []*Broadcast{}, nil
	}
	candidates, recorded, err := listCandidates(ctx, q, key, now)
	if err != nil {
		return nil, err
	}
	reserved, err := reservedPrograms(ctx, q, candidates)
	if err != nil {
		return nil, err
	}

	return Pick(candidates, reserved, recorded), nil
}

// sourceOf は、録画の回を読む元を作る。記述子の話数は、番組表の番組を先に、なければアーカイブの番組を使う。
func sourceOf(row *sqlc.GetRecordingNextEpisodeSourceRow) *episode.Source {
	extended := map[string]string{}
	// 詳細を読めなくても、番組名と概要から回を読む。
	if err := json.Unmarshal(row.Extended, &extended); err != nil {
		extended = map[string]string{}
	}
	src := &episode.Source{
		Name: row.Name, Description: row.Description, Extended: domain.NormalizeExtended(extended),
		SeriesEpisode: row.ProgramSeriesEpisode, Anime: nil,
	}
	if src.SeriesEpisode == nil {
		src.SeriesEpisode = row.ArchivedSeriesEpisode
	}
	if row.SyobocalTid != nil && row.CountStart != nil {
		src.Anime = &episode.Anime{TID: *row.SyobocalTid, Count: *row.CountStart}
	}

	return src
}

// listCandidates は、鍵 key の続きの放送の候補と、録画履歴にある録った回の話数を返す。
func listCandidates(ctx context.Context, q *sqlc.Queries, key *Key, now time.Time) ([]*Candidate, map[float64]bool, error) {
	candidates := []*Candidate{}
	recorded := map[float64]bool{}
	if key.TID > 0 {
		rows, err := q.ListAnimeNextPrograms(ctx, sqlc.ListAnimeNextProgramsParams{
			SyobocalTid: key.TID, Count: key.Count, Since: now, Until: now.Add(Horizon), MaxRows: maxCandidates,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list the next programs of the anime: %w", err)
		}
		for i := range rows {
			candidates = append(candidates, &Candidate{
				ServiceID: rows[i].ServiceID, EventID: rows[i].EventID, StartAt: rows[i].StartAt, Number: rows[i].EpisodeNumber,
				RelatedItems: rows[i].RelatedItems,
			})
		}
		counts, err := q.ListRecordedAnimeCounts(ctx, key.TID)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list the recorded episodes of the anime: %w", err)
		}
		for _, count := range counts {
			recorded[count] = true
		}

		return candidates, recorded, nil
	}

	rows, err := q.ListSeriesNextPrograms(ctx, sqlc.ListSeriesNextProgramsParams{
		Series: key.Series, Number: int64(key.Number), Since: now, Until: now.Add(Horizon), MaxRows: maxCandidates,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list the next programs of the series: %w", err)
	}
	for i := range rows {
		candidates = append(candidates, &Candidate{
			ServiceID: rows[i].ServiceID, EventID: rows[i].EventID, StartAt: rows[i].StartAt, Number: rows[i].EpisodeNumber,
			RelatedItems: rows[i].RelatedItems,
		})
	}
	history, err := q.ListRecordedSeriesEpisodes(ctx, key.Series)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list the recorded episodes of the series: %w", err)
	}
	for i := range history {
		ep := episode.Parse(&episode.Source{
			Name: history[i].Name, Description: history[i].Description, Extended: "", SeriesEpisode: history[i].SeriesEpisode, Anime: nil,
		})
		if ep.Series == key.Series && ep.Number > 0 {
			recorded[float64(ep.Number)] = true
		}
	}

	return candidates, recorded, nil
}

// reservedPrograms は、候補の番組と、同じイベントを共有する番組のうち、予約のあるものを返す。無効にした予約も含める。
func reservedPrograms(ctx context.Context, q *sqlc.Queries, candidates []*Candidate) (map[ProgramKey]bool, error) {
	reserved := map[ProgramKey]bool{}
	serviceIDs, eventIDs := []int64{}, []int64{}
	for _, c := range candidates {
		serviceIDs, eventIDs = append(serviceIDs, c.ServiceID), append(eventIDs, c.EventID)
		shared, err := domain.SharedPrograms(c.ServiceID, c.RelatedItems)
		if err != nil {
			continue
		}
		for _, other := range shared {
			serviceIDs, eventIDs = append(serviceIDs, other.ServiceID), append(eventIDs, other.EventID)
		}
	}
	if len(serviceIDs) == 0 {
		return reserved, nil
	}
	rows, err := q.ListReservationsByPrograms(ctx, sqlc.ListReservationsByProgramsParams{ServiceIds: serviceIDs, EventIds: eventIDs})
	if err != nil {
		return nil, fmt.Errorf("failed to list the reservations of the next programs: %w", err)
	}
	for i := range rows {
		if rows[i].EventID != nil {
			reserved[ProgramKey{ServiceID: rows[i].ServiceID, EventID: *rows[i].EventID}] = true
		}
	}

	return reserved, nil
}

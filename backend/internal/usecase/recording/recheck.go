package recording

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/domain/episode"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/notify"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/usecase/epg"
	"github.com/SlashNephy/kotatsu/internal/usecase/reserve"
	"github.com/SlashNephy/kotatsu/internal/usecase/rule"
)

// EpisodeRecheckTimeout は、録画の最初のデータが届いてから、予約のイベント ID が EIT[p/f] に出るまで待つ上限 (付録 A、#1500)。
// 2026 年 11 月に実放送 (Mahiron) の無料の局で 120 回測ると、最初のデータから次の番組として出るまでは、地上波で最長 0.95 秒、
// BS で最長 3.04 秒、BS4K で最長 0.71 秒だった。ARIB TR-B14 の送出間隔 (2 秒) の 5 倍で、測った最長の 3 倍を超え、
// 録画の開始マージンの既定 (15 秒) より短い 10 秒にする。
const EpisodeRecheckTimeout = 10 * time.Second

// terrestrialChannelType は、地上波のチャンネル種別。これ以外の局の放送を、同じ回の束ねでいう「衛星」とする。
const terrestrialChannelType = "GR"

// episodeRecheck は、ルール予約の録画の録り始めに、EIT[p/f] で番組情報を書き直し、同じ回をもう録画済みかを確かめ直す状態 (#1500)。
// 予定の開始まで読み捨てる分も Write で受け取り、EIT[p/f] を読む。
type episodeRecheck struct {
	recorder      *Recorder
	tracker       *endTracker
	reservationID uuid.UUID
	serviceName   string
	channelType   string
	serviceID16   uint16
	eventID16     uint16
	// early は、予定の開始まで読み捨てる分の解析器。
	early *ts.Analyzer

	mu sync.Mutex
	// firstData は、録画の最初のデータが届いた時刻。まだ届いていなければゼロ値。
	firstData time.Time
	// info は、最後に読んで書き直した、予約した番組の EIT[p/f] の情報。まだ読んでいなければ nil。
	info *ts.EventInfo
	// done は、確かめ終えたかどうか。matched は、同じ回を録画済みと分かったかどうかと、その録画のサービス (分からなければ nil)。
	done           bool
	matched        bool
	matchedService *int64
	// notified は、録画の開始を通知したかどうか。
	notified bool
}

var _ interface{ Write(p []byte) (int, error) } = new(episodeRecheck)

// newEpisodeRecheck は、予約 reservation の録画を、録り始めに同じ回かを確かめ直す対象なら、その状態を返す。対象でなければ nil を返す。
// イベント ID の無い録画 (時刻予約など) と、DB に届かずに対象かを決められない録画は、確かめずに録る。
func (r *Recorder) newEpisodeRecheck(ctx context.Context, logger *slog.Logger, reservation *sqlc.GetReservationForRecordingRow) *episodeRecheck {
	sid, ok := ts.BroadcastServiceID(reservation.ServiceID)
	if reservation.EventID == nil || !ok || *reservation.EventID < 0 || *reservation.EventID > 0xFFFF {
		return nil
	}
	checkCtx, cancel := context.WithTimeout(ctx, r.opts.DatabaseTimeout)
	defer cancel()
	target, err := sqlc.New(r.pool).IsEpisodeRecheckTarget(checkCtx, reservation.ID)
	if err != nil {
		logger.WarnContext(ctx, "failed to decide whether to recheck the episode; recording without rechecking", slog.Any("error", err))

		return nil
	}
	if !target {
		return nil
	}

	return &episodeRecheck{
		recorder: r, tracker: nil, reservationID: reservation.ID, serviceName: reservation.ServiceName, channelType: reservation.ChannelType,
		serviceID16: sid, eventID16: uint16(*reservation.EventID), early: ts.NewAnalyzer(nil),
		mu: sync.Mutex{}, firstData: time.Time{}, info: nil, done: false, matched: false, matchedService: nil, notified: false,
	}
}

// Write は、予定の開始まで読み捨てる分を受け取り、EIT[p/f] を読む。最初に受け取った時刻から、待つ上限を数える。
func (c *episodeRecheck) Write(p []byte) (int, error) {
	c.observeData(c.recorder.clock.Now())

	return c.early.Write(p)
}

// observeData は、録画のデータが at に届いたことを覚える。最初に届いた時刻だけを残す。c が nil なら何もしない。
func (c *episodeRecheck) observeData(at time.Time) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.firstData.IsZero() {
		c.firstData = at
	}
}

// eventInfo は、予約した番組の、EIT[p/f] の放送中か次の番組として読んだ情報を返す。書いているセグメントの解析器で読んだものを先に使う。
func (c *episodeRecheck) eventInfo() *ts.EventInfo {
	for _, analyzer := range []*ts.Analyzer{c.tracker.analyzer.Load(), c.early} {
		if analyzer == nil {
			continue
		}
		if info := analyzer.EventInfo(c.serviceID16, c.eventID16); info != nil {
			return info
		}
	}

	return nil
}

// check は、録画をやめるかを確かめる。同じ回を録画済みと分かったら true を返す。c が nil か、確かめ終えていれば false を返す。
//   - 予約のイベント ID が EIT[p/f] の放送中か次の番組に出たら、番組表の番組と録画を書き直し、同じ回を録画済みかを確かめる。
//   - 録画済みでなくても、待つ上限まで確かめ直す。同時に録り始めた先の録画が、後から番組情報を書き直すことがあるからである。
//   - 待つ上限が来たら、確かめ終え、録画の開始を通知する。EIT[p/f] が届かなければ、番組表の情報のまま録り続ける。
func (c *episodeRecheck) check(ctx context.Context, now time.Time) bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	first, done := c.firstData, c.done
	c.mu.Unlock()
	if done {
		// やめると決めたら、録画を終えるまで、そう返し続ける。セグメントを閉じられずに開き直しても、録画をやめる。
		return c.decided()
	}
	if first.IsZero() {
		return false
	}
	logger := c.recorder.logger.With(slog.String("recording_id", c.tracker.recordingID.String()))
	expired := !now.Before(first.Add(EpisodeRecheckTimeout))
	info := c.eventInfo()
	changed := info != nil && !sameEventInfo(info, c.info)
	if changed {
		c.rewrite(ctx, logger, info, now)
		c.info = info
	}
	if c.info != nil && (changed || expired) {
		matched, service, err := c.judge(ctx)
		switch {
		case err != nil:
			// 確かめられなければ、迷ったら録る。
			logger.WarnContext(ctx, "failed to recheck the episode; keep recording", slog.Any("error", err))
		case matched:
			logger.InfoContext(ctx, "the same episode has already been recorded; stopping the recording")
			c.mu.Lock()
			c.done, c.matched, c.matchedService = true, true, service
			c.mu.Unlock()

			return true
		default:
			logger.DebugContext(ctx, "rechecked the episode; it has not been recorded")
		}
	}
	if !expired {
		return false
	}
	if c.info == nil {
		logger.InfoContext(ctx, "the program did not appear in EIT[p/f] in time; recording with the EPG information",
			slog.Duration("timeout", EpisodeRecheckTimeout))
	}
	c.mu.Lock()
	c.done = true
	c.mu.Unlock()
	if err := c.notifyStarted(ctx); err != nil {
		// 録画を終えるときに通知し直す。
		logger.WarnContext(ctx, "failed to notify that the recording started", slog.Any("error", err))
	}

	return false
}

// decided は、同じ回を録画済みと分かって、録画をやめると決めたかどうかを返す。
func (c *episodeRecheck) decided() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.matched
}

// sameEventInfo は、2 つの EIT[p/f] の情報が同じかどうかを返す。b は nil でもよい。
func sameEventInfo(a, b *ts.EventInfo) bool {
	if b == nil || a.Name != b.Name || a.Description != b.Description || (a.Series == nil) != (b.Series == nil) {
		return false
	}
	if a.Series != nil && *a.Series != *b.Series {
		return false
	}

	return slices.EqualFunc(a.Extended, b.Extended, func(x, y *ts.ExtendedItem) bool { return *x == *y })
}

// rewrite は、EIT[p/f] で読んだ番組名、概要、詳細、シリーズの記述子で、番組表の番組と録画を書き直す (#1500)。
// 空の値では書き直さず、前の値を残す。番組表で読めていた話数を消さないためである。正規化して今の値と同じなら、書き直さない。
// チューナーサーバーと文字の書き方 (〜 と ～ など) が違うだけの値で、番組表と録画を書き換えないためである。書き直せなくても、録画は続ける。
func (c *episodeRecheck) rewrite(ctx context.Context, logger *slog.Logger, info *ts.EventInfo, now time.Time) {
	r := c.recorder
	t := c.tracker
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		recording, err := q.GetRecording(ctx, t.recordingID)
		if err != nil {
			return fmt.Errorf("failed to get the recording: %w", err)
		}
		recordingParams := sqlc.RewriteRecordingProgramInfoParams{
			Name: nil, Description: nil, Extended: nil, NameNormalized: nil, DescriptionNormalized: nil, Now: now, ID: t.recordingID,
		}
		recordingParams.Name, recordingParams.NameNormalized = changedText(info.Name, recording.NameNormalized)
		recordingParams.Description, recordingParams.DescriptionNormalized = changedText(info.Description, recording.DescriptionNormalized)
		recordingParams.Extended, err = changedExtended(info.Extended, recording.Extended)
		if err != nil {
			return err
		}
		if recordingParams.Name != nil || recordingParams.Description != nil || recordingParams.Extended != nil {
			if err := q.RewriteRecordingProgramInfo(ctx, recordingParams); err != nil {
				return fmt.Errorf("failed to rewrite the recording: %w", err)
			}
			if _, err := notify.Publish(ctx, tx, r.clock, notify.RecordingsChannel); err != nil {
				return err
			}
		}

		return rewriteProgram(ctx, tx, r, info, t.serviceID, *t.eventID, now)
	})
	if err != nil {
		logger.WarnContext(ctx, "failed to rewrite the program information from EIT[p/f]", slog.Any("error", err))

		return
	}
	logger.InfoContext(ctx, "read the program information from EIT[p/f]", slog.String("name", info.Name))
}

// changedText は、EIT[p/f] の値 value が空でなく、正規化したものが今の正規化した値 current と違えば、value と正規化したものを返す。
// 書き直さなくてよければ、どちらも nil を返す。
func changedText(value string, current *string) (*string, *string) {
	if value == "" {
		return nil, nil
	}
	normalized := domain.Normalize(value)
	if current != nil && foldWaveDash(*current) == foldWaveDash(normalized) {
		return nil, nil
	}

	return &value, &normalized
}

// foldWaveDash は、波ダッシュ (U+301C) を、NFKC で全角のチルダ (U+FF5E) を寄せた ~ にそろえる。
// 同じ 0x2141 を、チューナーサーバーは局によってどちらにもするので (BS の一部の局と BS4K)、書き方の違いだけで書き直さないためである。
// 書き直すと、録画履歴の番組名のキーが変わり、重複の回避で同じ番組名と分からなくなる。
func foldWaveDash(s string) string {
	return strings.ReplaceAll(s, "〜", "~")
}

// rewriteProgram は、番組表の番組 (serviceID, eventID) を、EIT[p/f] の値で書き直す。変わったら、EPG の変更として記録して通知し、
// ルールの照合をやり直させる。後から来る別の局の同じ回を、この録画と突き合わせられる。番組表に番組がなければ、何もしない。
func rewriteProgram(ctx context.Context, tx pgx.Tx, r *Recorder, info *ts.EventInfo, serviceID, eventID int64, now time.Time) error {
	q := sqlc.New(tx)
	current, err := q.GetProgramForRecheck(ctx, sqlc.GetProgramForRecheckParams{ServiceID: serviceID, EventID: eventID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get the program: %w", err)
	}
	params := sqlc.RewriteProgramInfoParams{
		Name: nil, Description: nil, NameNormalized: nil, DescriptionNormalized: nil, Extended: nil, ExtendedNormalized: nil,
		SeriesSet: false, SeriesEpisode: nil, SeriesRepeat: pgtype.Bool{Bool: false, Valid: false}, Now: now, ServiceID: serviceID, EventID: eventID,
	}
	params.Name, params.NameNormalized = changedText(info.Name, &current.NameNormalized)
	params.Description, params.DescriptionNormalized = changedText(info.Description, &current.DescriptionNormalized)
	extended, extendedNormalized, err := encodeExtended(info.Extended)
	if err != nil {
		return fmt.Errorf("failed to encode the extended information: %w", err)
	}
	if extendedNormalized != nil && (current.ExtendedNormalized == nil || foldWaveDash(*current.ExtendedNormalized) != foldWaveDash(*extendedNormalized)) {
		params.Extended, params.ExtendedNormalized = extended, extendedNormalized
	}
	if info.Series != nil {
		// 話数が 0 なら、記述子を送っても値を入れない局なので、どちらもないものとする (設計書 3.3)。
		if info.Series.Episode > 0 {
			params.SeriesEpisode = new(int32(info.Series.Episode))
			params.SeriesRepeat = pgtype.Bool{Bool: info.Series.Repeat != 0, Valid: true}
		}
		params.SeriesSet = !equalPointers(params.SeriesEpisode, current.SeriesEpisode) || params.SeriesRepeat != current.SeriesRepeat
	}
	if params.Name == nil && params.Description == nil && params.Extended == nil && !params.SeriesSet {
		return nil
	}
	if _, err := q.RewriteProgramInfo(ctx, params); err != nil {
		return fmt.Errorf("failed to rewrite the program: %w", err)
	}

	return publishProgramChange(ctx, tx, r, serviceID, eventID, now)
}

// equalPointers は、2 つのポインタの指す値が同じか、どちらも nil かを返す。
func equalPointers[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}

	return *a == *b
}

// publishProgramChange は、番組 (serviceID, eventID) が変わったことを、EPG の変更として記録して通知する (設計書 3.3)。
func publishProgramChange(ctx context.Context, tx pgx.Tx, r *Recorder, serviceID, eventID int64, now time.Time) error {
	q := sqlc.New(tx)
	program, err := q.GetProgramForReservation(ctx, sqlc.GetProgramForReservationParams{ServiceID: serviceID, EventID: eventID})
	if err != nil {
		return fmt.Errorf("failed to read the rewritten program: %w", err)
	}
	version, err := notify.Publish(ctx, tx, r.clock, epg.NotifyChannel)
	if err != nil {
		return err
	}
	err = q.InsertEPGChange(ctx, sqlc.InsertEPGChangeParams{
		Version: version, ServiceID: serviceID, StartAt: program.StartAt, EndAt: program.EndAt, CreatedAt: now,
	})
	if err != nil {
		return fmt.Errorf("failed to record an EPG change: %w", err)
	}

	return nil
}

// encodeExtended は、EIT[p/f] の詳細を、番組表の extended (見出しから本文への対応の JSON) と、正規化したものにする。
// 詳細がなければ、どちらも nil にして、前の値を残す。同じ見出しが続けば、本文をつなぐ。
func encodeExtended(items []*ts.ExtendedItem) ([]byte, *string, error) {
	if len(items) == 0 {
		return nil, nil, nil
	}
	extended := make(map[string]string, len(items))
	for _, item := range items {
		extended[item.Heading] += item.Text
	}
	encoded, err := json.Marshal(extended)
	if err != nil {
		return nil, nil, err
	}

	return encoded, new(domain.NormalizeExtended(extended)), nil
}

// changedExtended は、EIT[p/f] の詳細 items が、録画の今の詳細 current と正規化して違えば、書き直す JSON を返す (#1646)。
// 詳細がないか、同じなら nil を返して、前の値を残す。
func changedExtended(items []*ts.ExtendedItem, current []byte) ([]byte, error) {
	extended, normalized, err := encodeExtended(items)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the extended information: %w", err)
	}
	if normalized == nil {
		return nil, nil
	}
	currentMap := map[string]string{}
	if err := json.Unmarshal(current, &currentMap); err != nil {
		return nil, fmt.Errorf("failed to decode the extended information of the recording: %w", err)
	}
	if foldWaveDash(domain.NormalizeExtended(currentMap)) == foldWaveDash(*normalized) {
		return nil, nil
	}

	return extended, nil
}

// judge は、この録画を、同じ回を束ねる判定 (#1498) の候補 1 本として、先に録り始めた録画と、録画履歴と比べる。
// 同じ回を録画済みなら true と、その録画のサービスを返す。
func (c *episodeRecheck) judge(ctx context.Context) (bool, *int64, error) {
	r := c.recorder
	t := c.tracker
	q := sqlc.New(r.pool)
	recording, err := q.GetRecording(ctx, t.recordingID)
	if err != nil {
		return false, nil, fmt.Errorf("failed to get the recording: %w", err)
	}
	source, err := q.GetRecordingEpisodeSource(ctx, t.recordingID)
	if err != nil {
		return false, nil, fmt.Errorf("failed to get the episode of the recording: %w", err)
	}
	rows, err := q.ListEarlierRecordedEpisodes(ctx, sqlc.ListEarlierRecordedEpisodesParams{
		Since: recording.StartAt.Add(-rule.RecordedEpisodeWindow), ServiceID: recording.ServiceID, EventID: recording.EventID, APIID: recording.APIID,
	})
	if err != nil {
		return false, nil, fmt.Errorf("failed to list the recorded episodes: %w", err)
	}
	recorded := make([]*episode.Recorded, 0, len(rows))
	services := make(map[*episode.Recorded]int64, len(rows))
	for _, row := range rows {
		var anime *episode.Anime
		if row.SyobocalTid != nil && row.AnimeCount != nil {
			anime = &episode.Anime{TID: *row.SyobocalTid, Count: *row.AnimeCount}
		}
		e := &episode.Recorded{Episode: episode.Parse(&episode.Source{
			Name: row.Name, Description: row.Description, Extended: "", SeriesEpisode: row.SeriesEpisode, Anime: anime,
		})}
		recorded = append(recorded, e)
		services[e] = row.ServiceID
	}

	// 回を読む項目は、EIT[p/f] の値が空でなければ、それを使う。書き直せなかったときも、読んだ値で比べる。
	name, description := recording.Name, recording.Description
	if c.info.Name != "" {
		name = c.info.Name
	}
	if c.info.Description != "" {
		description = c.info.Description
	}
	extended := ""
	if _, normalized, err := encodeExtended(c.info.Extended); err == nil && normalized != nil {
		extended = *normalized
	}
	seriesEpisode := source.SeriesEpisode
	if c.info.Series != nil {
		seriesEpisode = nil
		if c.info.Series.Episode > 0 {
			seriesEpisode = new(int32(c.info.Series.Episode))
		}
	}
	var anime *episode.Anime
	if source.SyobocalTid != nil && source.AnimeCount != nil {
		anime = &episode.Anime{TID: *source.SyobocalTid, Count: *source.AnimeCount}
	}
	candidate := &episode.Broadcast{
		Episode: episode.Parse(&episode.Source{
			Name: name, Description: domain.Normalize(description), Extended: extended, SeriesEpisode: seriesEpisode, Anime: anime,
		}),
		ServiceID: recording.ServiceID, EventID: valueOr(recording.EventID), StartAt: recording.StartAt, EndAt: recording.EndAt,
		Captioned: episode.Captioned(name), Satellite: source.ChannelType != terrestrialChannelType, Rejected: false,
	}
	decision := episode.Resolve([]*episode.Broadcast{candidate}, recorded)[0]
	if decision.Recorded == nil {
		return false, nil, nil
	}
	service, ok := services[decision.Recorded]
	if !ok {
		return true, nil, nil
	}

	return true, &service, nil
}

// notifyStarted は、確かめ直した録画の開始を、まだ通知していなければ通知する (設計書 6.2)。c が nil か、録画をやめたなら何もしない。
func (c *episodeRecheck) notifyStarted(ctx context.Context) error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	skip := c.notified || c.matched
	c.mu.Unlock()
	if skip {
		return nil
	}
	r := c.recorder
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		row, err := sqlc.New(tx).GetRecordingWithService(ctx, c.tracker.recordingID)
		if err != nil {
			return fmt.Errorf("failed to get the recording: %w", err)
		}

		return r.emitStarted(ctx, tx, &row.Recording, c.reservationID, row.ServiceName)
	})
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.notified = true
	c.mu.Unlock()

	return nil
}

// abandon は、同じ回を録画済みと分かった録画をやめる (#1500)。書いたファイルと録画の記録を消し、予約は理由を付けて無効にする。
// 録らないと決めたものが、録画の一覧に失敗として並ばないよう、失敗にはしない。予約を有効のまま残すと、スケジューラがまた録り始める。
// 割り振りは、録画の行がないので、Worker が消す (有効に戻せば、また書ける)。
func (r *Recorder) abandon(ctx context.Context, logger *slog.Logger, assignment *Assignment, recording *sqlc.Recording, tracker *endTracker) error {
	tracker.backfill.stop()
	tracker.backfill = nil
	var files []sqlc.RecordingFile
	err := r.untilDatabase(ctx, logger, "abandon the recording", func(ctx context.Context) error {
		// 記録できていないセグメントがあれば、ファイルを消せるよう、先に記録する。
		if err := tracker.pending.save(ctx, r, tracker); err != nil {
			return err
		}

		return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
			q := sqlc.New(tx)
			// 録画をやめるのは、今の世代の mikan だけにする。
			generation, err := q.LockRecordingAssignmentGeneration(ctx, assignment.ID)
			switch {
			case errors.Is(err, pgx.ErrNoRows):
			case err != nil:
				return fmt.Errorf("failed to lock the recording assignment: %w", err)
			case generation != assignment.Generation:
				return errSuperseded
			}
			if files, err = q.ListRecordingFiles(ctx, recording.ID); err != nil {
				return fmt.Errorf("failed to list the recording files: %w", err)
			}
			if _, err := q.DeleteRecording(ctx, recording.ID); err != nil {
				return fmt.Errorf("failed to delete the recording: %w", err)
			}
			_, err = q.DisableReservationForRecordedEpisode(ctx, sqlc.DisableReservationForRecordedEpisodeParams{
				ServiceID: tracker.recheck.matchedService, Now: r.clock.Now(), ID: assignment.ReservationID,
			})
			if err != nil {
				return fmt.Errorf("failed to disable the reservation: %w", err)
			}
			if _, err := notify.Publish(ctx, tx, r.clock, notify.RecordingsChannel); err != nil {
				return err
			}
			_, err = notify.Publish(ctx, tx, r.clock, reserve.NotifyChannel)

			return err
		})
	})
	if errors.Is(err, errSuperseded) {
		logger.InfoContext(ctx, "another mikan took over the recording; leaving it")

		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to abandon the recording: %w", err)
	}
	for i := range files {
		if files[i].Kind != fileKindSegment {
			continue
		}
		// ファイルを消せなくても、記録は消したので、録画には含まれない。残ったファイルは、DB にないファイルとして見つかる (設計書 5.6)。
		if err := r.store.Remove(files[i].PoolID, files[i].Path, files[i].ID); err != nil {
			logger.WarnContext(ctx, "failed to remove a segment of the abandoned recording", slog.String("path", files[i].Path), slog.Any("error", err))
		}
	}
	logger.InfoContext(ctx, "abandoned the recording of the episode already recorded", slog.Int("segments", len(files)))

	return nil
}

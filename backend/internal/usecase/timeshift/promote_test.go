package timeshift_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/usecase/assign"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/recording"
	"github.com/SlashNephy/kotatsu/internal/usecase/timeshift"
)

// fakeInserter は、入れたジョブを覚える Inserter。
type fakeInserter struct {
	params []river.InsertManyParams
}

func (f *fakeInserter) InsertManyTx(_ context.Context, _ pgx.Tx, params []river.InsertManyParams) ([]*rivertype.JobInsertResult, error) {
	f.params = append(f.params, params...)
	results := make([]*rivertype.JobInsertResult, 0, len(params))
	for range params {
		results = append(results, &rivertype.JobInsertResult{Job: new(rivertype.JobRow), UniqueSkippedAsDuplicate: false})
	}

	return results, nil
}

// promoteEnv は、地上波の 0 番目のサービスをタイムシフトで録り、11:20 から今 (12:10) までのバッファに、
// 11:30 に始まって 12:00 に終わった番組 (イベント ID 2) と、12:00 に始まった放送中の番組 (イベント ID 3) を記録した環境。
// mikan は、番組の替わりに 4 秒遅れて気づいたものとする。
type promoteEnv struct {
	*env

	userID   uuid.UUID
	inserter *fakeInserter
	promoter *timeshift.Promoter
}

const noticeDelay = 4 * time.Second

func newPromoteEnv(t *testing.T) *promoteEnv {
	t.Helper()

	e := newEnv(t)
	e.configure(t, 0)
	ctx := t.Context()
	base := time.Date(2026, 10, 10, 11, 0, 0, 0, time.UTC)
	chunkStart := base.Add(20 * time.Minute)
	for _, c := range []struct {
		start time.Time
		end   *time.Time
	}{{start: chunkStart, end: new(base.Add(65 * time.Minute))}, {start: base.Add(65 * time.Minute), end: nil}} {
		chunkID := id.New()
		require.NoError(t, e.q.InsertTimeshiftChunk(ctx, sqlc.InsertTimeshiftChunkParams{
			ID: chunkID, ServiceID: grService(0), Path: chunkPath(grService(0), c.start), Size: 0, StartedAt: c.start, Now: c.start,
		}))
		if c.end != nil {
			_, err := e.q.CloseTimeshiftChunk(ctx, sqlc.CloseTimeshiftChunkParams{ID: chunkID, Size: 0, EndedAt: c.end})
			require.NoError(t, err)
		}
	}
	for _, p := range []struct {
		eventID int64
		start   time.Time
		end     *time.Time
	}{
		{eventID: 1, start: chunkStart, end: new(base.Add(30*time.Minute + noticeDelay))},
		{eventID: 2, start: base.Add(30*time.Minute + noticeDelay), end: new(base.Add(60*time.Minute + noticeDelay))},
		{eventID: 3, start: base.Add(60*time.Minute + noticeDelay), end: nil},
	} {
		require.NoError(t, e.q.StartTimeshiftProgram(ctx, sqlc.StartTimeshiftProgramParams{ServiceID: grService(0), StartedAt: p.start, EventID: p.eventID, Name: ""}))
		if p.end != nil {
			_, err := e.q.EndTimeshiftProgram(ctx, sqlc.EndTimeshiftProgramParams{ServiceID: grService(0), EndedAt: p.end})
			require.NoError(t, err)
		}
	}
	userID := id.New()
	_, err := e.pool.Exec(ctx, `INSERT INTO users (id, username, display_name, role, created_at, updated_at) VALUES ($1, 'member', 'member', 'member', $2, $2)`,
		userID, e.clk.Now())
	require.NoError(t, err)
	inserter := new(fakeInserter)
	promoter := timeshift.NewPromoter(e.clk, audit.NewRecorder(e.clk), e.reservations, assign.FixedDefaults(domain.DefaultRecordingSettings()), inserter)

	return &promoteEnv{env: e, userID: userID, inserter: inserter, promoter: promoter}
}

// auditActions は、監査ログに残した、対象の種類と操作の種類の組を、残した順に返す。
func (e *promoteEnv) auditActions(t *testing.T) [][2]string {
	t.Helper()

	rows, err := e.pool.Query(t.Context(), `SELECT target_type, action FROM audit_events ORDER BY occurred_at, action`)
	require.NoError(t, err)
	defer rows.Close()
	actions := [][2]string{}
	for rows.Next() {
		var target, action string
		require.NoError(t, rows.Scan(&target, &action))
		actions = append(actions, [2]string{target, action})
	}
	require.NoError(t, rows.Err())

	return actions
}

// TestPromoteEndedProgram は、放送の終わった番組を昇格すると、予約のない録画を録画中として作り、番組表の時間と mikan が気づいた時間を
// 合わせた範囲に前後のマージンを足した範囲を書き出すジョブを入れ、監査ログに残すことを確かめる (#1057、設計書 5.7)。
func TestPromoteEndedProgram(t *testing.T) {
	t.Parallel()

	e := newPromoteEnv(t)
	ctx := t.Context()
	programStart := time.Date(2026, 10, 10, 11, 30, 0, 0, time.UTC)
	promotion, err := e.promoter.Promote(ctx, e.pool, audit.User(e.userID), grService(0), programStart.Add(noticeDelay))
	require.NoError(t, err)
	require.NotNil(t, promotion.RecordingID)
	assert.Nil(t, promotion.Reservation)

	got, err := e.q.GetTimeshiftPromotion(ctx, *promotion.RecordingID)
	require.NoError(t, err)
	margin := domain.DefaultMarginSeconds * time.Second
	assert.Equal(t, programStart.Add(-margin), got.TimeshiftPromotion.FromAt.UTC())
	assert.Equal(t, programStart.Add(30*time.Minute+noticeDelay+margin), got.TimeshiftPromotion.ToAt.UTC())
	assert.Equal(t, &e.userID, got.TimeshiftPromotion.CreatedBy)
	rec := got.Recording
	assert.Equal(t, string(recording.StateRecording), rec.State)
	assert.Nil(t, rec.ReservationID)
	assert.Nil(t, rec.RuleID)
	assert.Equal(t, new(int64(2)), rec.EventID)
	assert.Equal(t, "こたつテスト1 テスト番組 2", rec.Name)
	assert.Equal(t, programStart, rec.StartAt.UTC())
	assert.Equal(t, programStart.Add(30*time.Minute), rec.EndAt.UTC())

	require.Len(t, e.inserter.params, 1)
	assert.Equal(t, recording.PromoteArgs{RecordingID: *promotion.RecordingID}, e.inserter.params[0].Args)
	// 終わりのマージンは今より前に過ぎているので、すぐに書き出す。
	assert.True(t, e.inserter.params[0].InsertOpts.ScheduledAt.IsZero())
	assert.Equal(t, [][2]string{{"recording", "timeshift.promote"}}, e.auditActions(t))

	programs, err := timeshift.PlayablePrograms(ctx, e.pool, grService(0), e.clk.Now())
	require.NoError(t, err)
	promoting := map[int64]bool{}
	for _, p := range programs {
		promoting[p.EventID] = p.Promoting
	}
	assert.Equal(t, map[int64]bool{1: false, 2: true, 3: false}, promoting)

	// 同じ番組を書き出している途中なら、重ねて昇格しない。マージンを変えて書き出す範囲が変わっても同じ。
	_, err = e.promoter.Promote(ctx, e.pool, audit.User(e.userID), grService(0), programStart.Add(noticeDelay))
	require.ErrorIs(t, err, timeshift.ErrAlreadyPromoting)
	settings := domain.DefaultRecordingSettings()
	settings.StartMarginSeconds = 60
	changed := timeshift.NewPromoter(e.clk, audit.NewRecorder(e.clk), e.reservations, assign.FixedDefaults(settings), e.inserter)
	_, err = changed.Promote(ctx, e.pool, audit.User(e.userID), grService(0), programStart.Add(noticeDelay))
	require.ErrorIs(t, err, timeshift.ErrAlreadyPromoting)
	assert.Len(t, e.inserter.params, 1)
}

// TestPromoteOnAirProgram は、放送中の番組を昇格すると、番組予約を作り、監査ログに予約の作成と昇格を残すことを確かめる。
// 番組の頭からの分は、録画を始めた mikan がバッファから写す (#826)。
func TestPromoteOnAirProgram(t *testing.T) {
	t.Parallel()

	e := newPromoteEnv(t)
	start := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	promotion, err := e.promoter.Promote(t.Context(), e.pool, audit.User(e.userID), grService(0), start.Add(noticeDelay))
	require.NoError(t, err)
	assert.Nil(t, promotion.RecordingID)
	require.NotNil(t, promotion.Reservation)
	assert.Equal(t, domain.ReservationKindProgram, promotion.Reservation.Kind)
	assert.Equal(t, new(int64(3)), promotion.Reservation.EventID)
	assert.Empty(t, e.inserter.params)
	assert.Equal(t, [][2]string{{"reservation", "reservation.create"}, {"reservation", "timeshift.promote"}}, e.auditActions(t))
}

// TestPromoteUnknownProgram は、バッファにない番組は昇格できないことを確かめる。
func TestPromoteUnknownProgram(t *testing.T) {
	t.Parallel()

	e := newPromoteEnv(t)
	_, err := e.promoter.Promote(t.Context(), e.pool, audit.User(e.userID), grService(0), time.Date(2026, 10, 10, 11, 31, 0, 0, time.UTC))
	require.ErrorIs(t, err, timeshift.ErrProgramNotFound)
}

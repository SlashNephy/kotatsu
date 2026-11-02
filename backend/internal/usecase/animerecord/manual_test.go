package animerecord_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/animerecord"
)

// TestRecordManually は、手で選んだ話の記録を、番組、録画、作品の話のどれからも作り、同じ話を二重に記録しないことを確かめる (#1232)。
func TestRecordManually(t *testing.T) {
	t.Parallel()

	program := &animerecord.ManualTarget{
		Program: &animerecord.ManualProgram{ServiceID: serviceID(), StartAt: at(12, 0), Name: titleA}, RecordingID: nil, Work: nil,
	}
	work := func(number float64) *animerecord.ManualTarget {
		return &animerecord.ManualTarget{Program: nil, RecordingID: nil, Work: &animerecord.ManualWork{TID: testTID, Number: number, Title: titleA}}
	}
	tests := []struct {
		name string
		// targets は、順に記録を頼むもの。wantErrs は、それぞれの結果 (nil なら成功)。
		targets  []*animerecord.ManualTarget
		wantErrs []error
		want     []string
		wantJobs int
	}{
		{
			name: "番組に結びついた話をすべて記録し、二度は記録しない", targets: []*animerecord.ManualTarget{program, program},
			wantErrs: []error{nil, animerecord.ErrAlreadyRecorded}, want: []string{annictThirdPending, annictFourthPending}, wantJobs: 1,
		},
		{
			name: "作品の話を記録し、同じ話は二度記録しない", targets: []*animerecord.ManualTarget{work(5), work(5), work(6)},
			wantErrs: []error{nil, animerecord.ErrAlreadyRecorded, nil}, want: []string{"annict:5:pending", "annict:6:pending"}, wantJobs: 2,
		},
		{
			name: "作品の話を手で記録したら、その話の番組を見ても記録しない", targets: []*animerecord.ManualTarget{work(3), program},
			// 番組の第 4 話だけを記録する。
			wantErrs: []error{nil, nil}, want: []string{annictThirdPending, annictFourthPending}, wantJobs: 2,
		},
		{
			name: "番組で記録した話は、作品の話としても記録しない", targets: []*animerecord.ManualTarget{program, work(4)},
			wantErrs: []error{nil, animerecord.ErrAlreadyRecorded}, want: []string{annictThirdPending, annictFourthPending}, wantJobs: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newRecordFixture(t)
			f.connect(t, f.alice, animerecord.DestinationAnnict)
			for i, target := range tt.targets {
				err := f.service.RecordManually(t.Context(), f.pool, f.inserter, f.alice.UserID, target)
				if tt.wantErrs[i] == nil {
					require.NoError(t, err, i)
				} else {
					require.ErrorIs(t, err, tt.wantErrs[i], i)
				}
			}
			assert.Equal(t, tt.want, pendingRecords(t, f, f.alice.UserID))
			assert.Len(t, f.inserter.taken(), tt.wantJobs)
		})
	}
}

// TestRecordManuallyRejects は、連携が無効なとき、連携していないとき、記録できる話がないときに、記録を作らないことを確かめる。
func TestRecordManuallyRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		prepare func(t *testing.T, f *recordFixture)
		target  *animerecord.ManualTarget
		wantErr error
	}{
		{
			name: "連携していない", prepare: func(*testing.T, *recordFixture) {},
			target:  &animerecord.ManualTarget{Program: nil, RecordingID: nil, Work: &animerecord.ManualWork{TID: testTID, Number: 1, Title: ""}},
			wantErr: animerecord.ErrNotConnected,
		},
		{
			name: "話に結びついていない番組", prepare: func(t *testing.T, f *recordFixture) {
				t.Helper()
				f.connect(t, f.alice, animerecord.DestinationAnnict)
			},
			target: &animerecord.ManualTarget{
				Program: &animerecord.ManualProgram{ServiceID: serviceID(), StartAt: at(11, 0), Name: "ニュース"}, RecordingID: nil, Work: nil,
			},
			wantErr: animerecord.ErrNothingToRecord,
		},
		{
			name: "連携が無効", prepare: func(t *testing.T, f *recordFixture) {
				t.Helper()
				f.connect(t, f.alice, animerecord.DestinationAnnict)
				setLinking(t, f, false)
			},
			target:  &animerecord.ManualTarget{Program: nil, RecordingID: nil, Work: &animerecord.ManualWork{TID: testTID, Number: 1, Title: ""}},
			wantErr: animerecord.ErrUnavailable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newRecordFixture(t)
			tt.prepare(t, f)
			require.ErrorIs(t, f.service.RecordManually(t.Context(), f.pool, f.inserter, f.alice.UserID, tt.target), tt.wantErr)
			assert.Empty(t, pendingRecords(t, f, f.alice.UserID))
		})
	}
}

// TestRecordManuallySends は、作品の話を手で記録したら、プレイヤーで見たときと同じジョブで記録先に送ることを確かめる (#1232)。
func TestRecordManuallySends(t *testing.T) {
	t.Parallel()
	f := newRecordFixture(t)
	f.connect(t, f.alice, animerecord.DestinationAnnict)
	target := &animerecord.ManualTarget{Program: nil, RecordingID: nil, Work: &animerecord.ManualWork{TID: testTID, Number: 3, Title: titleA}}
	require.NoError(t, f.service.RecordManually(t.Context(), f.pool, f.inserter, f.alice.UserID, target))
	jobs := f.inserter.taken()
	require.Len(t, jobs, 1)
	assert.Equal(t, int64(0), jobs[0].ServiceID, "a manual record is not tied to a program")

	require.NoError(t, f.service.Record(t.Context(), f.pool, &jobs[0], false))
	records, _, _ := f.annict.snapshot()
	assert.Equal(t, []string{"E3"}, records)
	assert.Equal(t, []string{annictThirdRecorded}, pendingRecords(t, f, f.alice.UserID))
}

// TestRecordManuallyConcurrently は、同じ作品の話を同時に記録しても、1 回だけ記録することを確かめる (#1232)。
func TestRecordManuallyConcurrently(t *testing.T) {
	t.Parallel()
	f := newRecordFixture(t)
	f.connect(t, f.alice, animerecord.DestinationAnnict)
	target := &animerecord.ManualTarget{Program: nil, RecordingID: nil, Work: &animerecord.ManualWork{TID: testTID, Number: 7, Title: titleA}}
	const n = 5
	errs := make(chan error, n)
	start := make(chan struct{})
	for range n {
		go func() {
			<-start
			// 頼んだ日時が重ならないよう、時計を進めてから頼む。
			f.clock.Advance(time.Millisecond)
			errs <- f.service.RecordManually(t.Context(), f.pool, f.inserter, f.alice.UserID, target)
		}()
	}
	close(start)
	succeeded := 0
	for range n {
		if err := <-errs; err == nil {
			succeeded++
		} else {
			require.ErrorIs(t, err, animerecord.ErrAlreadyRecorded)
		}
	}
	assert.Equal(t, 1, succeeded)
	assert.Equal(t, []string{"annict:7:pending"}, pendingRecords(t, f, f.alice.UserID))
}

// TestRecordManuallyRecording は、録画から、録画に写した結びつきの話を記録することを確かめる (#1232)。
func TestRecordManuallyRecording(t *testing.T) {
	t.Parallel()
	f := newRecordFixture(t)
	f.connect(t, f.alice, animerecord.DestinationAnnict)
	recordingID := uuid.MustParse("0192f000-0000-7000-8000-000000000001")
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, service_id, name, description, start_at, end_at, state, recorded_from, created_at, updated_at)
		VALUES ($1, $2, 'テスト', '', $3, $4, 'completed', $3, now(), now())`, recordingID, serviceID(), at(12, 0), at(12, 30))
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), `INSERT INTO recording_anime_matches (recording_id, matched_at) VALUES ($1, now())`, recordingID)
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), `INSERT INTO recording_anime_episodes (recording_id, position, source, syobocal_tid, count_start, count_end, is_first, is_last)
		VALUES ($1, 0, 'syobocal', 6776, 3, 3, false, false)`, recordingID)
	require.NoError(t, err)

	target := &animerecord.ManualTarget{Program: nil, RecordingID: &recordingID, Work: nil}
	require.NoError(t, f.service.RecordManually(t.Context(), f.pool, f.inserter, f.alice.UserID, target))
	assert.Equal(t, []string{annictThirdPending}, pendingRecords(t, f, f.alice.UserID))
}

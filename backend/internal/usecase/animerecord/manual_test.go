package animerecord_test

import (
	"testing"

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

package timeshift_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/usecase/timeshift"
)

// TestPlayablePrograms は、バッファの番組ごとに、再生できる範囲の始まりと、チャンクの間の途切れた区間を求めることを確かめる (設計書 7.2)。
func TestPlayablePrograms(t *testing.T) {
	t.Parallel()

	// 時間は、今の 1 時間前からの時間で表す。end が 0 なら、書いている途中のチャンクか、放送中の番組。
	type period struct{ start, end time.Duration }
	type want struct {
		start        time.Duration
		playableFrom time.Duration
		gaps         []period
		fromStart    bool
	}
	tests := []struct {
		name       string
		configured bool
		chunks     []period
		programs   []period
		want       []want
	}{
		{
			name: "区切ったチャンクの境目は途切れない", configured: true,
			chunks:   []period{{0, 5 * time.Minute}, {5 * time.Minute, 10 * time.Minute}},
			programs: []period{{2 * time.Minute, 8 * time.Minute}},
			want:     []want{{start: 2 * time.Minute, playableFrom: 2 * time.Minute, gaps: []period{}}},
		},
		{
			name: "番組の頭のチャンクがなければ、残っているチャンクの始まりから再生する", configured: true,
			chunks:   []period{{10 * time.Minute, 15 * time.Minute}},
			programs: []period{{5 * time.Minute, 12 * time.Minute}},
			want:     []want{{start: 5 * time.Minute, playableFrom: 10 * time.Minute, gaps: []period{}}},
		},
		{
			name: "チャンクの間の穴は途切れた区間にする", configured: true,
			chunks:   []period{{0, 5 * time.Minute}, {7 * time.Minute, 10 * time.Minute}, {10 * time.Minute, 0}},
			programs: []period{{time.Minute, 0}},
			want:     []want{{start: time.Minute, playableFrom: time.Minute, gaps: []period{{5 * time.Minute, 7 * time.Minute}}}},
		},
		{
			name: "引き継ぎで重なったチャンクは途切れない", configured: true,
			chunks:   []period{{0, 5 * time.Minute}, {4 * time.Minute, 0}},
			programs: []period{{time.Minute, 0}},
			want:     []want{{start: time.Minute, playableFrom: time.Minute, gaps: []period{}}},
		},
		{
			name: "チャンクと重ならない番組は除く", configured: true,
			chunks:   []period{{20 * time.Minute, 25 * time.Minute}},
			programs: []period{{0, 10 * time.Minute}, {10 * time.Minute, 30 * time.Minute}},
			want:     []want{{start: 10 * time.Minute, playableFrom: 20 * time.Minute, gaps: []period{}}},
		},
		{
			name: "前の番組の終わりに始まった番組だけを、頭から録れたものとする", configured: true,
			chunks:   []period{{0, 0}},
			programs: []period{{0, 5 * time.Minute}, {5 * time.Minute, 0}},
			// 録り始めたときの番組は、途中から録っている。
			want: []want{
				{start: 0, playableFrom: 0, gaps: []period{}, fromStart: false},
				{start: 5 * time.Minute, playableFrom: 5 * time.Minute, gaps: []period{}, fromStart: true},
			},
		},
		{
			name: "録り直した後の番組は、頭から録れていない", configured: true,
			chunks:   []period{{0, 5 * time.Minute}, {7 * time.Minute, 0}},
			programs: []period{{0, 5 * time.Minute}, {7 * time.Minute, 0}},
			want: []want{
				{start: 0, playableFrom: 0, gaps: []period{}, fromStart: false},
				{start: 7 * time.Minute, playableFrom: 7 * time.Minute, gaps: []period{}, fromStart: false},
			},
		},
		{
			name: "タイムシフトから外したサービスは空", configured: false,
			chunks:   []period{{0, 5 * time.Minute}},
			programs: []period{{time.Minute, 4 * time.Minute}},
			want:     []want{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := newEnv(t)
			if tt.configured {
				e.configure(t, 0)
			}
			serviceID := grService(0)
			base := e.clk.Now().Add(-time.Hour)
			for _, c := range tt.chunks {
				chunkID := id.New()
				started := base.Add(c.start)
				require.NoError(t, e.q.InsertTimeshiftChunk(t.Context(), sqlc.InsertTimeshiftChunkParams{
					ID: chunkID, ServiceID: serviceID, Path: chunkPath(serviceID, started), Size: 0, StartedAt: started, Now: started,
				}))
				if c.end > 0 {
					_, err := e.q.CloseTimeshiftChunk(t.Context(), sqlc.CloseTimeshiftChunkParams{ID: chunkID, Size: 0, EndedAt: new(base.Add(c.end))})
					require.NoError(t, err)
				}
			}
			for i, p := range tt.programs {
				require.NoError(t, e.q.StartTimeshiftProgram(t.Context(), sqlc.StartTimeshiftProgramParams{
					ServiceID: serviceID, StartedAt: base.Add(p.start), EventID: int64(i + 1), Name: "番組",
				}))
				if p.end > 0 {
					_, err := e.q.EndTimeshiftProgram(t.Context(), sqlc.EndTimeshiftProgramParams{ServiceID: serviceID, EndedAt: new(base.Add(p.end))})
					require.NoError(t, err)
				}
			}

			got, err := timeshift.PlayablePrograms(t.Context(), e.pool, serviceID, e.clk.Now())
			require.NoError(t, err)
			wants := make([]want, 0, len(got))
			for _, p := range got {
				gaps := make([]period, 0, len(p.Gaps))
				for _, gap := range p.Gaps {
					gaps = append(gaps, period{start: gap.From.Sub(base), end: gap.To.Sub(base)})
				}
				wants = append(wants, want{start: p.StartedAt.Sub(base), playableFrom: p.PlayableFrom.Sub(base), gaps: gaps, fromStart: p.FromStart})
			}
			assert.Equal(t, tt.want, wants)
		})
	}
}

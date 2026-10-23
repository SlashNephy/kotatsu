package epgstation_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/epgstation"
)

// mainPool は、テストで対応付ける Kotatsu のプール ID。
const mainPool = "main"

func TestNewPlan(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 10, 21, 0, 0, 0, time.UTC)
	file := func(dir string, fileType epgstation.VideoFileType) *epgstation.VideoFile {
		return &epgstation.VideoFile{ID: 1, ParentDirectoryName: dir, FilePath: "a", Type: fileType, Name: "TS", Size: 1}
	}
	snapshot := &epgstation.Snapshot{
		Rules: []*epgstation.Rule{
			{ID: 1, ParentDirectoryName: ""},
			{ID: 2, ParentDirectoryName: animeDirectory},
			{ID: 3, ParentDirectoryName: recordedDirectory},
		},
		ManualReserves: []*epgstation.Reserve{
			{ID: 1, IsTimeSpecified: false, StartAt: now.Add(time.Hour), ParentDirectoryName: animeDirectory},
			{ID: 2, IsTimeSpecified: true, StartAt: now.Add(time.Hour)},
			// 開始済みの予約は、ディレクトリが対応付けられていなくても数えない。
			{ID: 3, IsTimeSpecified: false, StartAt: now, ParentDirectoryName: "old"},
		},
		Recordings: []*epgstation.Recording{
			{
				ID:    1,
				Files: []*epgstation.VideoFile{file(recordedDirectory, epgstation.VideoFileTypeTS), file(encodedDirectory, epgstation.VideoFileTypeEncoded)},
				Drop:  &epgstation.DropLog{ErrorCount: 0, DropCount: 0, ScramblingCount: 0},
			},
			{
				ID:    2,
				Files: []*epgstation.VideoFile{file(recordedDirectory, epgstation.VideoFileTypeEncoded), file(animeDirectory, epgstation.VideoFileTypeTS)},
			},
			// 録画中の録画は、ファイルもドロップの集計も数えない。
			{
				ID:          3,
				IsRecording: true,
				Files:       []*epgstation.VideoFile{file("live", epgstation.VideoFileTypeTS)},
				Drop:        &epgstation.DropLog{ErrorCount: 0, DropCount: 0, ScramblingCount: 0},
			},
		},
		Histories: []*epgstation.RecordedHistory{{ID: 1}, {ID: 2}},
		Tags:      []*epgstation.Tag{{ID: 1}},
	}

	tests := []struct {
		name  string
		pools map[string]string
		want  *epgstation.Plan
	}{
		{
			name:  "every directory is mapped",
			pools: map[string]string{recordedDirectory: mainPool, encodedDirectory: mainPool, animeDirectory: "sub"},
			want: &epgstation.Plan{
				Rules:                3,
				ProgramReserves:      1,
				TimeReserves:         1,
				Recordings:           2,
				TSFiles:              2,
				OtherFiles:           2,
				DropLogs:             1,
				Histories:            2,
				Tags:                 1,
				StartedReserves:      1,
				RecordingsInProgress: 1,
				UnmappedDirectories:  []*epgstation.UnmappedDirectory{},
			},
		},
		{
			name:  "lists unmapped directories in name order",
			pools: map[string]string{recordedDirectory: mainPool},
			want: &epgstation.Plan{
				Rules:                3,
				ProgramReserves:      1,
				TimeReserves:         1,
				Recordings:           2,
				TSFiles:              1,
				OtherFiles:           1,
				DropLogs:             1,
				Histories:            2,
				Tags:                 1,
				StartedReserves:      1,
				RecordingsInProgress: 1,
				UnmappedDirectories: []*epgstation.UnmappedDirectory{
					{Name: animeDirectory, Files: 1, Rules: 1, Reserves: 1},
					{Name: encodedDirectory, Files: 1, Rules: 0, Reserves: 0},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := epgstation.NewPlan(snapshot, &epgstation.PlanOptions{Now: now, Pools: tt.pools})
			assert.Equal(t, tt.want, got)
		})
	}
}

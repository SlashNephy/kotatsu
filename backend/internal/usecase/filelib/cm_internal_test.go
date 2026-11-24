package filelib

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
)

// fakeCMDetector は、FFmpeg の代わりに、決めた区間を返す。受け取った入力と、Open で読んだ TS の大きさを保存する。
type fakeCMDetector struct {
	sections []cmdetect.Section
	err      error
	input    *cmdetect.Input
	read     int64
}

var _ cmdetect.Detector = new(fakeCMDetector)

func (*fakeCMDetector) Name() string { return "fake" }

func (*fakeCMDetector) Version() int32 { return 7 }

func (d *fakeCMDetector) Detect(_ context.Context, input *cmdetect.Input) ([]cmdetect.Section, error) {
	d.input = input
	file, err := input.Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()
	n, err := io.Copy(io.Discard, file)
	if err != nil {
		return nil, err
	}
	d.read = n

	return d.sections, d.err
}

// TestCMWorker は、インデックスを作り終えたライブラリのファイルの CM の区間を、ライブラリの口でファイルを読んで見分け、
// TS から読んだサービスと番組の時刻を渡し、今のファイルを見分けた結果だけを返すことを確認する (#1734)。
func TestCMWorker(t *testing.T) {
	t.Parallel()

	f := newImagesFixture(t)
	file := f.open(t)
	inserter := new(fakeInserter)
	require.NoError(t, EnqueueCM(t.Context(), inserter, f.reader.Indexer(), file))
	require.Len(t, inserter.args, 1, "見分けていなければ、見分けるジョブを入れる")
	args, ok := inserter.args[0].(CMArgs)
	require.True(t, ok)
	assert.Equal(t, CMArgs{Library: internalLibrary, Path: internalShowTS, FileID: file.ID()}, args)

	detector := &fakeCMDetector{
		sections: []cmdetect.Section{{Start: 2 * time.Second, End: 3 * time.Second}}, err: nil, input: nil, read: 0,
	}
	worker := newCMWorker(f.worker.libraries, f.reader.Indexer(), func(context.Context) (cmdetect.Detector, error) { return detector, nil })
	require.NoError(t, worker.Work(t.Context(), &river.Job[CMArgs]{JobRow: nil, Args: args}))
	assert.Equal(t, file.Entry.Size, detector.read, "ライブラリの口でファイルを読み通す")
	require.NotNil(t, detector.input)
	assert.Equal(t, uint16(0x0400), detector.input.ServiceID, "TS から読んだサービスの映像と音声を見分ける")
	_, duration, _ := file.Timeline()
	assert.Equal(t, duration, detector.input.Duration)
	result, ok := f.reader.Indexer().CM(f.ref, file.ID())
	require.True(t, ok)
	assert.Equal(t, &CMResult{
		FileID: file.ID(), Detector: "fake", Version: 7, Sections: []*CMSection{{Start: 2 * time.Second, End: 3 * time.Second}}, Note: "",
	}, result)
	inserter.args = nil
	require.NoError(t, EnqueueCM(t.Context(), inserter, f.reader.Indexer(), file))
	assert.Empty(t, inserter.args, "見分けた結果があれば、入れない")

	// 映像のないファイルは、見分けるものがないので、区間のない結果を残し、画面を待たせない。
	noVideo := &fakeCMDetector{sections: nil, err: cmdetect.ErrNoVideo, input: nil, read: 0}
	worker = newCMWorker(f.worker.libraries, f.reader.Indexer(), func(context.Context) (cmdetect.Detector, error) { return noVideo, nil })
	require.NoError(t, worker.Work(t.Context(), &river.Job[CMArgs]{JobRow: nil, Args: args}))
	result, ok = f.reader.Indexer().CM(f.ref, file.ID())
	require.True(t, ok)
	assert.Empty(t, result.Sections)

	// ファイルを書き換えたら、前のファイルの結果は返さず、前のファイルのジョブは取りやめる。
	later := file.Entry.ModifiedAt.Add(time.Minute)
	require.NoError(t, os.Chtimes(filepath.Join(f.dir, internalShowTS), later, later))
	_, ok = f.reader.Indexer().CM(f.ref, f.open(t).ID())
	assert.False(t, ok)
	err := worker.Work(t.Context(), &river.Job[CMArgs]{JobRow: nil, Args: args})
	var cancel *rivertype.JobCancelError
	require.ErrorAs(t, err, &cancel)
	assert.ErrorIs(t, err, errFileChanged)
}

// TestCMProgram は、番組のウィンドウに渡す番組の時刻を、録り始め (再生の 0 秒) からの時間にし、分からなければ渡さないことを確認する。
func TestCMProgram(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, 11, 1, 6, 9, 30, 0, time.UTC)
	start := from.Add(30 * time.Second)
	tests := []struct {
		name      string
		broadcast *Broadcast
		want      *cmdetect.Program
	}{
		{
			name: "known",
			broadcast: &Broadcast{
				NetworkID: nil, ServiceID: 1, ServiceName: "", RecordedFrom: &from, RecordedUntil: nil,
				Program: &Program{EventID: 1, Name: "", Description: "", Extended: nil, Start: &start, Duration: new(43 * time.Minute)},
			},
			want: &cmdetect.Program{Start: 30 * time.Second, Duration: 43 * time.Minute},
		},
		{
			name: "no recorded time",
			broadcast: &Broadcast{
				NetworkID: nil, ServiceID: 1, ServiceName: "", RecordedFrom: nil, RecordedUntil: nil,
				Program: &Program{EventID: 1, Name: "", Description: "", Extended: nil, Start: &start, Duration: new(43 * time.Minute)},
			},
			want: nil,
		},
		{
			name: "no duration",
			broadcast: &Broadcast{
				NetworkID: nil, ServiceID: 1, ServiceName: "", RecordedFrom: &from, RecordedUntil: nil,
				Program: &Program{EventID: 1, Name: "", Description: "", Extended: nil, Start: &start, Duration: nil},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, cmProgram(tt.broadcast))
		})
	}
}

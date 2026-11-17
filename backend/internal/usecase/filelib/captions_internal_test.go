package filelib

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/usecase/autocaption"
)

// fakeTranscriber は、whisper-cli の代わりに、決めた字幕を返す。
type fakeTranscriber struct {
	cues []autocaption.Cue
}

var _ autocaption.Transcriber = new(fakeTranscriber)

func (*fakeTranscriber) Model() string { return "fake-model" }

func (t *fakeTranscriber) Transcribe(_ context.Context, wav string, _ func(ratio float64)) ([]autocaption.Cue, error) {
	if _, err := os.Stat(wav); err != nil {
		return nil, err
	}

	return t.cues, nil
}

// fakeAudio は、FFmpeg の代わりに、読んだ TS の大きさを数え、空の WAV を書き、決めた位置を返す。
type fakeAudio struct {
	offset    time.Duration
	err       error
	read      int64
	serviceID uint16
}

func (a *fakeAudio) extract(_ context.Context, r io.Reader, serviceID uint16, wav string) (time.Duration, error) {
	n, err := io.Copy(io.Discard, r)
	if err != nil {
		return 0, err
	}
	a.read, a.serviceID = n, serviceID
	if a.err != nil {
		return 0, a.err
	}

	return a.offset, os.WriteFile(wav, []byte("RIFF"), 0o600)
}

// TestCaptionsWorker は、ライブラリのファイルの音声を、ライブラリの口で読んで取り出して書き起こし、再生の 0 秒からの時間にそろえて置き、
// 今のファイルから作った字幕だけを返すことを確かめる (#1734)。
func TestCaptionsWorker(t *testing.T) {
	t.Parallel()

	f := newImagesFixture(t)
	file := f.open(t)
	inserter := new(fakeInserter)
	_, err := EnqueueCaptions(t.Context(), inserter, file)
	require.NoError(t, err)
	require.Len(t, inserter.args, 1)
	args, ok := inserter.args[0].(CaptionsArgs)
	require.True(t, ok)
	assert.Equal(t, CaptionsArgs{Library: internalLibrary, Path: internalShowTS, FileID: file.ID()}, args)

	// 音声は、最初の I フレームより 1 秒前から始まる。0.5 秒で終わる字幕は、再生に入らないので捨てる。
	audio := &fakeAudio{offset: -time.Second, err: nil, read: 0, serviceID: 0}
	transcriber := &fakeTranscriber{cues: []autocaption.Cue{
		{Start: 0, End: 500 * time.Millisecond, Text: "前の番組"},
		{Start: 2 * time.Second, End: 3 * time.Second, Text: "こんにちは"},
	}}
	now := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	worker := NewCaptionsWorker(f.worker.libraries, f.reader.Indexer(), clocktest.NewFake(now),
		func() (autocaption.Transcriber, error) { return transcriber, nil },
		func() (autocaption.ReaderExtractor, error) { return audio.extract, nil }, t.TempDir())
	require.NoError(t, worker.Work(t.Context(), &river.Job[CaptionsArgs]{JobRow: nil, Args: args}))
	assert.Equal(t, file.Entry.Size, audio.read, "ライブラリの口でファイルを読み通す")
	assert.Equal(t, uint16(0x0400), audio.serviceID, "TS から読んだサービスの音声を取り出す")
	result, ok := f.reader.Indexer().Captions(f.ref, file.ID())
	require.True(t, ok)
	assert.Equal(t, &CaptionsResult{
		FileID: file.ID(), Model: "fake-model", GeneratedAt: now,
		Cues: []*CaptionCue{{Start: time.Second, End: 2 * time.Second, Text: "こんにちは"}},
	}, result)

	// 音声のないファイルは、字幕のない結果を残す。
	audio.err = autocaption.ErrNoAudio
	require.NoError(t, worker.Work(t.Context(), &river.Job[CaptionsArgs]{JobRow: nil, Args: args}))
	result, ok = f.reader.Indexer().Captions(f.ref, file.ID())
	require.True(t, ok)
	assert.Empty(t, result.Cues)

	// ファイルを書き換えたら、前のファイルの字幕は返さず、前のファイルのジョブは取りやめる。
	later := file.Entry.ModifiedAt.Add(time.Minute)
	require.NoError(t, os.Chtimes(filepath.Join(f.dir, internalShowTS), later, later))
	_, ok = f.reader.Indexer().Captions(f.ref, f.open(t).ID())
	assert.False(t, ok)
	err = worker.Work(t.Context(), &river.Job[CaptionsArgs]{JobRow: nil, Args: args})
	var cancel *rivertype.JobCancelError
	require.ErrorAs(t, err, &cancel)
	assert.ErrorIs(t, err, errFileChanged)
}

// TestCaptionsWorkerWriting は、書いている途中のファイルは、音声を取り出さずに取りやめることを確かめる。
func TestCaptionsWorkerWriting(t *testing.T) {
	t.Parallel()

	f := newImagesFixture(t)
	// 索引の時計 (今の 1 時間後) から見て、書いたばかりのファイルにする。
	now := time.Now().Add(time.Hour)
	require.NoError(t, os.Chtimes(filepath.Join(f.dir, internalShowTS), now, now))
	file := f.open(t)
	require.True(t, file.Writing)
	audio := &fakeAudio{offset: 0, err: nil, read: 0, serviceID: 0}
	worker := NewCaptionsWorker(f.worker.libraries, f.reader.Indexer(), clocktest.NewFake(now),
		func() (autocaption.Transcriber, error) { return &fakeTranscriber{cues: nil}, nil },
		func() (autocaption.ReaderExtractor, error) { return audio.extract, nil }, t.TempDir())
	args := CaptionsArgs{Library: internalLibrary, Path: internalShowTS, FileID: file.ID()}
	err := worker.Work(t.Context(), &river.Job[CaptionsArgs]{JobRow: nil, Args: args})
	var cancel *rivertype.JobCancelError
	require.ErrorAs(t, err, &cancel)
	require.ErrorIs(t, err, errFileWriting)
	assert.Zero(t, audio.read, "音声を取り出さない")
}

// TestCaptionsWorkerNotConfigured は、whisper-cli かモデルがなければ、やり直さずに取りやめ、理由を残すことを確かめる。
func TestCaptionsWorkerNotConfigured(t *testing.T) {
	t.Parallel()

	f := newImagesFixture(t)
	worker := NewCaptionsWorker(f.worker.libraries, f.reader.Indexer(), clocktest.NewFake(time.Now()),
		func() (autocaption.Transcriber, error) { return nil, autocaption.ErrNotConfigured },
		func() (autocaption.ReaderExtractor, error) { return nil, errors.New("unused") }, t.TempDir())
	args := CaptionsArgs{Library: internalLibrary, Path: internalShowTS, FileID: f.open(t).ID()}
	err := worker.Work(t.Context(), &river.Job[CaptionsArgs]{JobRow: nil, Args: args})
	var cancel *rivertype.JobCancelError
	require.ErrorAs(t, err, &cancel)
	assert.ErrorIs(t, err, autocaption.ErrNotConfigured)
}

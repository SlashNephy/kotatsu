package filelib

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/infra/medialib"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
)

// writeArtifact は、データディレクトリ dataDir の name に、size バイトのファイルを置く。
func writeArtifact(t *testing.T, dataDir, name string, size int) {
	t.Helper()

	path := filepath.Join(dataDir, filepath.FromSlash(name))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, make([]byte, size), 0o600))
}

// TestClearArtifacts は、libraries/ の下と、前に消しきれずに残った置き場だけを数えて消し、データディレクトリのほかのもの
// (録画のセグメントの生成物、キャプチャ、字幕のモデル) を残すことを確かめる (#1896)。
func TestClearArtifacts(t *testing.T) {
	t.Parallel()

	errRecord := errors.New("record failed")
	// outside は、libraries/ の外の、消さないもの。
	outside := []string{"0199f0a0-0000-7000-8000-00000000beef/index", "captures/user/capture.png", "whisper/ggml-model.bin", "librariesx/keep"}
	tests := []struct {
		name string
		// artifacts は、置く生成物の名前と大きさ。
		artifacts map[string]int
		// dirs は、置く空のディレクトリ。
		dirs       []string
		recordErr  error
		wantUsage  *Usage
		wantRecord bool
		wantErr    error
		// wantRemain は、消した後に libraries/ が残るかどうか。
		wantRemain bool
	}{
		{
			name: "clear",
			artifacts: map[string]int{
				"libraries/a/index": 100, "libraries/a/source.json": 10, "libraries/b/thumbnail.jpg": 1000,
				// 前に片付けたときに消しきれずに残った置き場も、数えて消す。
				".libraries-1.removing/c/index": 5,
			},
			wantUsage: &Usage{Size: 1115, Files: 4}, wantRecord: true,
		},
		{
			name: "only detached", artifacts: map[string]int{".libraries-1.removing/c/index": 5},
			wantUsage: &Usage{Size: 5, Files: 1}, wantRecord: true,
		},
		{name: "empty file", artifacts: map[string]int{"libraries/.keep": 0}, wantUsage: &Usage{Size: 0, Files: 1}, wantRecord: true},
		// 空のディレクトリだけなら、消すものはないので、監査ログに残さない。PurgeDetached が、空の置き場を消す。
		{name: "empty directories", dirs: []string{"libraries/a", ".libraries-1.removing"}, wantUsage: &Usage{Size: 0, Files: 0}, wantRemain: true},
		// 消すものがなければ、監査ログに残さない。
		{name: "nothing", artifacts: map[string]int{}, wantUsage: &Usage{Size: 0, Files: 0}, wantRecord: false},
		// 監査ログを書けなければ、消さない。
		{
			name: "record failed", artifacts: map[string]int{"libraries/a/index": 100}, recordErr: errRecord,
			wantRecord: true, wantErr: errRecord, wantRemain: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dataDir := t.TempDir()
			for name, size := range tt.artifacts {
				writeArtifact(t, dataDir, name, size)
			}
			for _, dir := range tt.dirs {
				require.NoError(t, os.MkdirAll(filepath.Join(dataDir, filepath.FromSlash(dir)), 0o755))
			}
			for _, name := range outside {
				writeArtifact(t, dataDir, name, 1)
			}
			indexer := NewIndexer(dataDir, medialib.NewSet(map[string]string{}, []string{}, map[string]string{}),
				clocktest.NewFake(time.Now()), slog.New(slog.DiscardHandler), true)
			before, err := indexer.Artifacts()
			require.NoError(t, err)

			var recorded *Usage
			usage, err := indexer.ClearArtifacts(t.Context(), func(u *Usage) error {
				recorded = u

				return tt.recordErr
			})
			require.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.wantRecord, recorded != nil)
			if tt.wantErr == nil {
				assert.Equal(t, tt.wantUsage, usage)
				assert.Equal(t, before, usage, "測った量と同じだけ消す")
			}
			require.NoError(t, indexer.PurgeDetached())
			if tt.wantRemain {
				assert.DirExists(t, filepath.Join(dataDir, kago.LibraryIndexDir))
			} else {
				assert.NoDirExists(t, filepath.Join(dataDir, kago.LibraryIndexDir))
			}
			detached, err := filepath.Glob(filepath.Join(dataDir, ".libraries-*"))
			require.NoError(t, err)
			assert.Empty(t, detached)
			for _, name := range outside {
				assert.FileExists(t, filepath.Join(dataDir, filepath.FromSlash(name)))
			}
		})
	}
}

// TestClearArtifactsWithoutDataDir は、データディレクトリを設定していなければ、量を測らず、何も消さないことを確かめる。
// 空のパスで消すと、作業ディレクトリの libraries/ を消してしまうためである。
func TestClearArtifactsWithoutDataDir(t *testing.T) {
	t.Parallel()

	indexer := NewIndexer("", medialib.NewSet(map[string]string{}, []string{}, map[string]string{}),
		clocktest.NewFake(time.Now()), slog.New(slog.DiscardHandler), true)
	_, err := indexer.Artifacts()
	require.ErrorIs(t, err, ErrNoDataDir)
	_, err = indexer.ClearArtifacts(t.Context(), func(*Usage) error { return nil })
	require.ErrorIs(t, err, ErrNoDataDir)
	require.ErrorIs(t, indexer.PurgeDetached(), ErrNoDataDir)
}

// TestClearArtifactsRebuilds は、作り終えた索引を消すと、読んだときに索引がないものとして返し、開けば作り直すことを確かめる。
func TestClearArtifactsRebuilds(t *testing.T) {
	t.Parallel()

	f := newIndexFixture(t)
	f.indexer.Ensure(t.Context(), f.ref, f.entry)
	require.Eventually(t, func() bool {
		index, err := f.indexer.Read(f.ref, f.entry)
		require.NoError(t, err)

		return index.Complete
	}, 10*time.Second, 10*time.Millisecond)

	_, err := f.indexer.ClearArtifacts(t.Context(), func(*Usage) error { return nil })
	require.NoError(t, err)
	require.NoError(t, f.indexer.PurgeDetached())
	index, err := f.indexer.Read(f.ref, f.entry)
	require.NoError(t, err)
	assert.False(t, index.Complete)
	assert.Empty(t, index.Entries)

	f.indexer.Ensure(t.Context(), f.ref, f.entry)
	require.Eventually(t, func() bool {
		index, err := f.indexer.Read(f.ref, f.entry)
		require.NoError(t, err)

		return index.Complete && index.Broadcast != nil
	}, 10*time.Second, 10*time.Millisecond)
}

// TestClearArtifactsStopsBuilds は、作っている途中の索引を取りやめて止まるのを待ってから消し、片付けている間は作り始めず、
// 片付けた後に開けば作り直すことを確かめる。取りやめずに消すと、作っていた処理がファイルを読み終えるまで、作り直しが始まらない。
func TestClearArtifactsStopsBuilds(t *testing.T) {
	t.Parallel()

	f := newIndexFixture(t)
	// 作る枠を埋めて、作り始めた処理を待たせておく。
	for range maxBuilds {
		f.indexer.slots <- struct{}{}
	}
	f.indexer.Ensure(t.Context(), f.ref, f.entry)
	f.indexer.mu.Lock()
	require.NotNil(t, f.indexer.building[indexKey(f.ref)])
	f.indexer.mu.Unlock()
	// 作り始める前に置かれていた索引の置き場 (前のファイルのもの) を、消す対象にする。
	writeArtifact(t, f.indexer.dataDir, kago.LibraryIndexDir+"/old/index", 10)

	_, err := f.indexer.ClearArtifacts(t.Context(), func(*Usage) error {
		f.indexer.mu.Lock()
		building := len(f.indexer.building)
		f.indexer.mu.Unlock()
		assert.Zero(t, building, "作っていた処理が止まってから消す")
		// 片付けている間は、作り始めない。
		f.indexer.Ensure(t.Context(), f.ref, f.entry)
		f.indexer.mu.Lock()
		assert.Nil(t, f.indexer.building[indexKey(f.ref)])
		f.indexer.mu.Unlock()
		// ほかの片付けとは重ねない。
		_, err := f.indexer.ClearArtifacts(t.Context(), func(*Usage) error { return nil })
		assert.ErrorIs(t, err, ErrClearing)

		return nil
	})
	require.NoError(t, err)
	require.NoError(t, f.indexer.PurgeDetached())
	for range maxBuilds {
		<-f.indexer.slots
	}

	f.indexer.Ensure(t.Context(), f.ref, f.entry)
	require.Eventually(t, func() bool {
		index, err := f.indexer.Read(f.ref, f.entry)
		require.NoError(t, err)

		return index.Complete
	}, 10*time.Second, 10*time.Millisecond)
}

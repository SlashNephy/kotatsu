package kago_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"text/template"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/kago"
)

const (
	title    = "番組名"
	untitled = "untitled"
	date     = "20261009-2100"
	channel  = "こたつテスト1"
)

func TestSanitizeName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain", input: "【新】番組名 #1「サブタイトル」", want: "【新】番組名 #1「サブタイトル」"},
		{name: "path separators", input: "A/B\\C", want: "ABC"},
		{name: "characters Windows cannot use", input: `番組: "A" <B> | C? *`, want: "番組 A B  C"},
		{name: "control characters", input: "番組\x00名\t\n\x7f", want: title},
		{name: "invalid UTF-8", input: "番組\xff\xfe名", want: title},
		{name: "leading dots and spaces", input: " ..hidden", want: "hidden"},
		{name: "trailing dots and ideographic spaces", input: "番組名.　", want: title},
		{name: "parent directory", input: "..", want: untitled},
		{name: "empty", input: "", want: untitled},
		{name: "only separators", input: "///", want: untitled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, kago.SanitizeName(tt.input))
		})
	}
}

func TestRenderName(t *testing.T) {
	t.Parallel()

	tmpl := template.Must(template.New("name").Option("missingkey=error").Parse("{{.Date}} {{.Channel}} {{.Title}}"))
	type data struct {
		Date, Channel, Title string
	}

	tests := []struct {
		name   string
		data   data
		want   string
		maxLen int
	}{
		{
			name: "plain",
			data: data{Date: date, Channel: channel, Title: title},
			want: "20261009-2100 こたつテスト1 番組名.m2ts",
		},
		{
			// 値に入っている / を取り除くのは、呼ぶ側 (reserve.RenderFileName) の役目。
			name: "slashes create directories",
			data: data{Date: date, Channel: channel, Title: "前編/後編"},
			want: "20261009-2100 こたつテスト1 前編/後編.m2ts",
		},
		{
			name: "empty, dot and dot-dot elements are dropped",
			data: data{Date: date, Channel: channel, Title: "/../a//./b/../番組"},
			want: "20261009-2100 こたつテスト1/a/b/番組.m2ts",
		},
		{
			name: "a directory named untitled is kept",
			data: data{Date: date, Channel: channel, Title: "untitled/番組"},
			want: "20261009-2100 こたつテスト1 untitled/番組.m2ts",
		},
		{
			name: "characters Windows cannot use are removed from directories too",
			data: data{Date: date, Channel: channel, Title: "A: B?/番組"},
			want: "20261009-2100 こたつテスト1 A B/番組.m2ts",
		},
		{
			name: "directories deeper than the limit are dropped",
			data: data{Date: date, Channel: channel, Title: "1/2/3/4/5/6/7/8/9/10/番組"},
			want: "20261009-2100 こたつテスト1 1/2/3/4/5/6/7/8/番組.m2ts",
		},
		{
			// 3 バイトの文字を 200 文字並べ、600 バイトにする。
			name:   "a long title is cut at a character boundary",
			data:   data{Date: date, Channel: channel, Title: strings.Repeat("あ", 200)},
			maxLen: 255,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := kago.RenderName(tmpl, tt.data, ".m2ts")
			require.NoError(t, err)
			if tt.want != "" {
				assert.Equal(t, tt.want, got)
			}
			assert.True(t, utf8.ValidString(got))
			assert.True(t, strings.HasSuffix(got, ".m2ts"))
			if tt.maxLen > 0 {
				assert.LessOrEqual(t, len(got), tt.maxLen)
				assert.Greater(t, len(got), tt.maxLen-3, "only the bytes of a partial character may be left over")
			}
		})
	}

	t.Run("invalid extension", func(t *testing.T) {
		t.Parallel()

		for _, ext := range []string{"m2ts", "./m2ts", "." + strings.Repeat("x", 20)} {
			_, err := kago.RenderName(tmpl, data{Date: date, Channel: channel, Title: title}, ext)
			require.ErrorIs(t, err, kago.ErrInvalidPath, ext)
		}
	})

	t.Run("a name cut at an ideographic space can be created", func(t *testing.T) {
		t.Parallel()

		// 切る位置の直前に全角空白が来るようにする。名前の先頭 (日付とチャンネルと空白) は 34 バイトで、
		// 上限の 255 バイトから ".m2ts" を引いた 250 バイトまでに入れる。
		// 34 + 3×71 + 3 (全角空白) = 250 バイトで、次の文字を足すと 253 バイトになるので、全角空白の直後で切れる。
		long := strings.Repeat("あ", 71) + "\u3000" + strings.Repeat("い", 20)
		name, err := kago.RenderName(tmpl, data{Date: date, Channel: channel, Title: long}, ".m2ts")
		require.NoError(t, err)
		assert.NotContains(t, name, "\u3000.m2ts", "the trailing ideographic space must be trimmed")

		store := kago.New(map[string]string{poolMain: t.TempDir()}, nil)
		f, _, err := store.Create(poolMain, "", name)
		require.NoError(t, err)
		require.NoError(t, f.Close())
	})

	t.Run("template error", func(t *testing.T) {
		t.Parallel()

		_, err := kago.RenderName(tmpl, map[string]string{}, ".m2ts")
		require.Error(t, err)
	})
}

func TestCreate(t *testing.T) {
	t.Parallel()

	longName := strings.Repeat("あ", 83) + ".m2ts"
	require.LessOrEqual(t, len(longName), 255)

	tests := []struct {
		name  string
		dir   string
		file  string
		count int
		// existingDir は、dir を先に作っておくかどうか。
		existingDir bool
		wantRel     []string
	}{
		{
			name:    "sequence numbers for the same name",
			file:    fileName,
			count:   3,
			wantRel: []string{fileName, "番組 (2).m2ts", "番組 (3).m2ts"},
		},
		{
			name:        "in a subdirectory",
			dir:         "anime",
			file:        fileName,
			count:       2,
			existingDir: true,
			wantRel:     []string{"anime/番組.m2ts", "anime/番組 (2).m2ts"},
		},
		{
			name:    "creates missing directories",
			dir:     "GR/2026-10",
			file:    fileName,
			count:   1,
			wantRel: []string{"GR/2026-10/番組.m2ts"},
		},
		{
			name:  "a long name is cut to make room for the sequence number",
			file:  longName,
			count: 2,
			wantRel: []string{
				longName,
				// 255 バイトから " (2)" と ".m2ts" の 9 バイトを引いた 246 バイトに、3 バイトの文字が 82 文字入る。
				strings.Repeat("あ", 82) + " (2).m2ts",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mount := t.TempDir()
			if tt.existingDir {
				require.NoError(t, os.Mkdir(filepath.Join(mount, tt.dir), 0o755))
			}
			store := kago.New(map[string]string{poolMain: mount}, nil)

			rels := []string{}
			for range tt.count {
				f, rel, err := store.Create(poolMain, tt.dir, tt.file)
				require.NoError(t, err)
				assert.Equal(t, filepath.Join(mount, rel), f.Name())
				require.NoError(t, f.Close())
				rels = append(rels, rel)
			}
			assert.Equal(t, tt.wantRel, rels)
			for _, rel := range rels {
				assert.LessOrEqual(t, len(filepath.Base(rel)), 255)
			}
		})
	}
}

func TestCreateConcurrently(t *testing.T) {
	t.Parallel()

	store := kago.New(map[string]string{poolMain: t.TempDir()}, nil)

	const n = 20
	var wg sync.WaitGroup
	rels := make(chan string, n)
	errs := make(chan error, n)
	for range n {
		wg.Go(func() {
			f, rel, err := store.Create(poolMain, "", fileName)
			if err != nil {
				errs <- err

				return
			}
			errs <- f.Close()
			rels <- rel
		})
	}
	wg.Wait()
	close(rels)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	seen := map[string]bool{}
	for rel := range rels {
		assert.False(t, seen[rel], "%s was created twice", rel)
		seen[rel] = true
	}
	assert.Len(t, seen, n)
}

func TestCreateRejectsUnsanitizedName(t *testing.T) {
	t.Parallel()

	store := kago.New(map[string]string{poolMain: t.TempDir()}, nil)

	tests := []struct {
		name string
		file string
	}{
		{name: "separator", file: "../番組.m2ts"},
		{name: "control character", file: "番組\n.m2ts"},
		{name: "too long", file: strings.Repeat("a", 251) + ".m2ts"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := store.Create(poolMain, "", tt.file)
			require.ErrorIs(t, err, kago.ErrInvalidPath)
		})
	}
}

// TestCreateDoesNotEscapeThePool は、プールの中のシンボリックリンクをたどって、プールの外にディレクトリを作らないことを確かめる。
func TestCreateDoesNotEscapeThePool(t *testing.T) {
	t.Parallel()

	mount, outside := t.TempDir(), t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(mount, "link")))
	store := kago.New(map[string]string{poolMain: mount}, nil)

	_, _, err := store.Create(poolMain, "link/sub", fileName)
	require.Error(t, err)
	_, statErr := os.Stat(filepath.Join(outside, "sub"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

// TestRemoveDeletesEmptyDirectories は、録画を消して空になったディレクトリを、プールの直下まで遡って消し、
// 空でないディレクトリは残すことを確かめる。
func TestRemoveDeletesEmptyDirectories(t *testing.T) {
	t.Parallel()

	mount := t.TempDir()
	store := kago.New(map[string]string{poolMain: mount}, nil)
	create := func(dir string) string {
		f, rel, err := store.Create(poolMain, dir, fileName)
		require.NoError(t, err)
		require.NoError(t, f.Close())

		return rel
	}
	removed := create("GR/2026-10")
	kept := create("GR/2026-11")

	require.NoError(t, store.Remove(poolMain, removed, uuid.Nil))
	_, err := os.Stat(filepath.Join(mount, "GR/2026-10"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(mount, kept))
	require.NoError(t, err)

	require.NoError(t, store.Remove(poolMain, kept, uuid.Nil))
	entries, err := os.ReadDir(mount)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// TestRemoveKeepsDirectoriesInTheSegmentDir は、生成物のディレクトリの中にディレクトリがあれば、
// 生成物のディレクトリではないとみなし、中身ごと消さないことを確かめる。
func TestRemoveKeepsDirectoriesInTheSegmentDir(t *testing.T) {
	t.Parallel()

	mount, dataRoot := t.TempDir(), t.TempDir()
	store := kago.New(map[string]string{poolMain: mount}, nil).WithDataDir(dataRoot)
	f, rel, err := store.Create(poolMain, "", fileName)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	fileID := uuid.New()
	other := filepath.Join(dataRoot, fileID.String(), "sub", "other.m2ts")
	require.NoError(t, os.MkdirAll(filepath.Dir(other), 0o755))
	require.NoError(t, os.WriteFile(other, []byte("x"), 0o600))

	require.ErrorIs(t, store.Remove(poolMain, rel, fileID), kago.ErrInvalidPath)
	_, err = os.Stat(other)
	assert.NoError(t, err)
}

// TestRemoveDoesNotFollowSymlinksInTheSegmentDir は、生成物のディレクトリの中のシンボリックリンクをたどらず、リンクだけを消すことを確かめる。
func TestRemoveDoesNotFollowSymlinksInTheSegmentDir(t *testing.T) {
	t.Parallel()

	mount, dataRoot, outside := t.TempDir(), t.TempDir(), t.TempDir()
	target := filepath.Join(outside, "keep.txt")
	require.NoError(t, os.WriteFile(target, []byte("x"), 0o600))
	store := kago.New(map[string]string{poolMain: mount}, nil).WithDataDir(dataRoot)
	f, rel, err := store.Create(poolMain, "", fileName)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	fileID := uuid.New()
	segmentDir, err := store.CreateSegmentDir(fileID)
	require.NoError(t, err)
	require.NoError(t, os.Symlink(target, filepath.Join(dataRoot, segmentDir.Path, "link")))

	require.NoError(t, store.Remove(poolMain, rel, fileID))
	_, err = os.Stat(target)
	require.NoError(t, err)
	_, err = os.Lstat(filepath.Join(dataRoot, segmentDir.Path))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

// TestRename は、録画ファイルを移し、移す先に同じ名前があれば連番を付け、元のディレクトリが空になれば消すことを確かめる。
func TestRename(t *testing.T) {
	t.Parallel()

	mount := t.TempDir()
	store := kago.New(map[string]string{poolMain: mount}, nil)
	create := func(dir, name string) string {
		f, rel, err := store.Create(poolMain, dir, name)
		require.NoError(t, err)
		require.NoError(t, f.Close())

		return rel
	}
	from := create("old", fileName)
	create("GR", "新しい名前.m2ts")

	got, err := store.Rename(poolMain, from, "GR", "新しい名前.m2ts", 1)
	require.NoError(t, err)
	assert.Equal(t, "GR/新しい名前 (2).m2ts", got)
	_, err = os.Stat(filepath.Join(mount, "old"))
	require.ErrorIs(t, err, os.ErrNotExist, "the emptied directory is removed")

	same, err := store.Rename(poolMain, got, "GR", "新しい名前.m2ts", 2)
	require.NoError(t, err)
	assert.Equal(t, got, same, "renaming to the same name keeps the file")

	_, err = store.Rename(poolMain, got, "../outside", fileName, 1)
	require.ErrorIs(t, err, kago.ErrInvalidPath)
}

// TestRestore は、Rename で移したファイルを、元のパスへ戻すことを確かめる (#917)。
// 元の名前は、今の整え方で使えない文字を含むことがあるので、整え直さずに戻す。
func TestRestore(t *testing.T) {
	t.Parallel()

	const (
		// unsanitized は、整え方を変える前に付いた名前。今の SanitizeName は半角の : を取り除く。
		unsanitized = "old/古い:名前.m2ts"
		moved       = "GR/新しい名前.m2ts"
	)
	tests := []struct {
		name string
		// occupied は、戻す前に、元のパスに別のファイルを置くかどうか。
		occupied bool
		wantErr  bool
	}{
		{name: "今の整え方で使えない文字を含む元の名前に戻し、消えた元のディレクトリを作り直す", occupied: false, wantErr: false},
		{name: "元のパスに別のファイルがあれば、上書きせずに失敗する", occupied: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mount := t.TempDir()
			store := kago.New(map[string]string{poolMain: mount}, nil)
			require.NoError(t, os.MkdirAll(filepath.Join(mount, "GR"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(mount, moved), []byte("recording"), 0o600))
			if tt.occupied {
				require.NoError(t, os.MkdirAll(filepath.Join(mount, "old"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(mount, unsanitized), []byte("other"), 0o600))
			}

			err := store.Restore(poolMain, moved, unsanitized)
			if tt.wantErr {
				require.Error(t, err)
				got, err := os.ReadFile(filepath.Join(mount, unsanitized))
				require.NoError(t, err)
				assert.Equal(t, "other", string(got), "the existing file is not overwritten")
				_, err = os.Stat(filepath.Join(mount, moved))
				require.NoError(t, err, "the moved file stays")

				return
			}
			require.NoError(t, err)
			got, err := os.ReadFile(filepath.Join(mount, unsanitized))
			require.NoError(t, err)
			assert.Equal(t, "recording", string(got))
			_, err = os.Stat(filepath.Join(mount, "GR"))
			require.ErrorIs(t, err, os.ErrNotExist, "the emptied directory is removed")
		})
	}

	mount := t.TempDir()
	store := kago.New(map[string]string{poolMain: mount}, nil)
	require.ErrorIs(t, store.Restore(poolMain, moved, "../outside.m2ts"), kago.ErrInvalidPath)

	// 元と同じパスなら、何もしない。
	require.NoError(t, os.WriteFile(filepath.Join(mount, "same.m2ts"), []byte("recording"), 0o600))
	require.NoError(t, store.Restore(poolMain, "same.m2ts", "./same.m2ts"))
	_, err := os.Stat(filepath.Join(mount, "same.m2ts"))
	require.NoError(t, err)
}

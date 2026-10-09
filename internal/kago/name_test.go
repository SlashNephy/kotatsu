package kago_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"text/template"
	"unicode/utf8"

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
			name: "separators in a title do not create directories",
			data: data{Date: date, Channel: channel, Title: "前編/後編"},
			want: "20261009-2100 こたつテスト1 前編後編.m2ts",
		},
		{
			// 3 バイトの文字を 200 文字並べ、600 バイトにする。
			name:   "a long title is cut at a character boundary",
			data:   data{Date: date, Channel: channel, Title: strings.Repeat("あ", 200)},
			maxLen: 255 - len(".kotatsu"),
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
				// 付属ファイル用のディレクトリの名前も、255 バイトに収まる。
				assert.LessOrEqual(t, len(got+".kotatsu"), 255)
			}
		})
	}

	t.Run("template error", func(t *testing.T) {
		t.Parallel()

		_, err := kago.RenderName(tmpl, map[string]string{}, ".m2ts")
		require.Error(t, err)
	})
}

func TestCreate(t *testing.T) {
	t.Parallel()

	longName := strings.Repeat("あ", 80) + ".m2ts"
	require.LessOrEqual(t, len(longName), 255-len(".kotatsu"))

	tests := []struct {
		name    string
		dir     string
		file    string
		count   int
		wantRel []string
	}{
		{
			name:    "sequence numbers for the same name",
			file:    fileName,
			count:   3,
			wantRel: []string{fileName, "番組 (2).m2ts", "番組 (3).m2ts"},
		},
		{
			name:    "in a subdirectory",
			dir:     "anime",
			file:    fileName,
			count:   2,
			wantRel: []string{"anime/番組.m2ts", "anime/番組 (2).m2ts"},
		},
		{
			name:  "a long name is cut to make room for the sequence number",
			file:  longName,
			count: 2,
			wantRel: []string{
				longName,
				// 247 バイトから " (2)" と ".m2ts" の 9 バイトを引いた 238 バイトに、3 バイトの文字が 79 文字入る。
				strings.Repeat("あ", 79) + " (2).m2ts",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mount := t.TempDir()
			if tt.dir != "" {
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
				assert.LessOrEqual(t, len(filepath.Base(rel)), 255-len(".kotatsu"))
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
		{name: "too long", file: strings.Repeat("a", 250) + ".m2ts"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := store.Create(poolMain, "", tt.file)
			require.ErrorIs(t, err, kago.ErrInvalidPath)
		})
	}
}

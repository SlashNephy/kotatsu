package licenses_test

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/web/licenses"
)

const (
	listPath    = "dist/go.json"
	linuxModule = "example.com/linux"
)

const listJSON = `{"modules":[
  {"path":"std","version":"go1.27.3","files":[{"name":"LICENSE","text":"Go"}]},
  {"path":"example.com/linux","version":"v1.0.0","files":[{"name":"LICENSE","text":"linux"}]},
  {"path":"example.com/darwin","version":"v1.0.0","files":[{"name":"LICENSE","text":"darwin"}]}
]}`

// TestLoad は、一覧を、動いているバイナリに組み込まれたモジュールと標準ライブラリに絞ることと、
// 一覧を書き出さずにビルドしたときは、一覧がないことを返すことを確かめる (#1604)。
func TestLoad(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fsys      fstest.MapFS
		linked    map[string]bool
		wantPaths []string
		wantErr   error
		// wantAnyErr は、エラーの種類を問わず失敗することを表す。
		wantAnyErr bool
	}{
		{
			name:      "組み込まれたものと標準ライブラリだけ",
			fsys:      fstest.MapFS{listPath: {Data: []byte(listJSON)}},
			linked:    map[string]bool{linuxModule: true},
			wantPaths: []string{"std", linuxModule},
		},
		{
			name:      "組み込まれたものが分からなければ絞らない",
			fsys:      fstest.MapFS{listPath: {Data: []byte(listJSON)}},
			linked:    nil,
			wantPaths: []string{"std", linuxModule, "example.com/darwin"},
		},
		{
			name:    "一覧を書き出さずにビルドした",
			fsys:    fstest.MapFS{"dist/.gitkeep": {Data: nil}},
			wantErr: licenses.ErrNotGenerated,
		},
		{
			name:       "読めない一覧",
			fsys:       fstest.MapFS{listPath: {Data: []byte("{")}},
			wantAnyErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := licenses.LoadFrom(tt.fsys, tt.linked)
			if tt.wantAnyErr {
				require.Error(t, err)

				return
			}
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			paths := make([]string, 0, len(got.Modules))
			for _, m := range got.Modules {
				paths = append(paths, m.Path)
			}
			assert.Equal(t, tt.wantPaths, paths)
		})
	}
}

package drcstable_test

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/drcstable"
)

const tablePath = "dist/drcs.json"

// TestLoad は、作ったテーブルを読むことと、作らずにビルドしたときや壊れたテーブルを、置き換えのないテーブルとして黙って
// 返さないことを確認する。
func TestLoad(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		fsys    fstest.MapFS
		want    map[string]string
		wantErr error
		// wantAnyErr は、エラーの種類を問わず失敗することを表す。
		wantAnyErr bool
	}{
		{
			name: "作ったテーブル",
			fsys: fstest.MapFS{tablePath: {Data: []byte(
				`{"commit":"f0037120","replacements":{"012665f146f9c4dfbfe65c41c8b30c19":"芦","016669fa94786f9581342d47f317c02c":"『"}}`,
			)}},
			want: map[string]string{"012665f146f9c4dfbfe65c41c8b30c19": "芦", "016669fa94786f9581342d47f317c02c": "『"},
		},
		{
			name:    "作らずにビルドした",
			fsys:    fstest.MapFS{"dist/.gitkeep": {}},
			wantErr: drcstable.ErrNotGenerated,
		},
		{
			name:       "JSON として読めない",
			fsys:       fstest.MapFS{tablePath: {Data: []byte(`{"commit":`)}},
			wantAnyErr: true,
		},
		{
			name:       "置き換えが空",
			fsys:       fstest.MapFS{tablePath: {Data: []byte(`{"commit":"f0037120","replacements":{}}`)}},
			wantAnyErr: true,
		},
		{
			name:       "commit がない",
			fsys:       fstest.MapFS{tablePath: {Data: []byte(`{"replacements":{"012665f146f9c4dfbfe65c41c8b30c19":"芦"}}`)}},
			wantAnyErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := drcstable.LoadFrom(tt.fsys)
			switch {
			case tt.wantErr != nil:
				require.ErrorIs(t, err, tt.wantErr)
			case tt.wantAnyErr:
				require.Error(t, err)
				require.NotErrorIs(t, err, drcstable.ErrNotGenerated)
			default:
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// TestMerge は、管理者の登録を既定のテーブルより優先し、どちらにもある外字は登録の文字にすることを確認する。
func TestMerge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		defaults   map[string]string
		registered map[string]string
		want       map[string]string
	}{
		{
			name:       "登録を既定のテーブルに足す",
			defaults:   map[string]string{"aa": "芦"},
			registered: map[string]string{"bb": "辻"},
			want:       map[string]string{"aa": "芦", "bb": "辻"},
		},
		{
			name:       "同じ外字は登録を優先する",
			defaults:   map[string]string{"aa": "芦", "bb": "『"},
			registered: map[string]string{"aa": "葦"},
			want:       map[string]string{"aa": "葦", "bb": "『"},
		},
		{
			name:       "既定のテーブルがないビルドでも登録は使う",
			defaults:   nil,
			registered: map[string]string{"aa": "葦"},
			want:       map[string]string{"aa": "葦"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, drcstable.Merge(tt.defaults, tt.registered))
		})
	}
}

// TestMergeDoesNotChangeInputs は、重ねても、渡した既定のテーブルを書き換えないことを確認する。読んだ既定のテーブルを
// 使い回しても、ほかの呼び出しに登録が混ざらないようにするため。
func TestMergeDoesNotChangeInputs(t *testing.T) {
	t.Parallel()

	defaults := map[string]string{"aa": "芦"}
	drcstable.Merge(defaults, map[string]string{"aa": "葦", "bb": "辻"})
	assert.Equal(t, map[string]string{"aa": "芦"}, defaults)
}

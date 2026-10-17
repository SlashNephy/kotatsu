package kago_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/kago"
)

// TestSidecarOwner は、付属ファイルのディレクトリから、持ち主の録画ファイルの場所を求められることを確かめる。
func TestSidecarOwner(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		poolID  string
		dir     string
		want    *kago.Location
		wantErr bool
	}{
		{name: "録画ファイルの横", poolID: poolMain, dir: "番組/録画.m2ts.kotatsu", want: &kago.Location{PoolID: poolMain, Path: "番組/録画.m2ts"}},
		{name: "まとめて置くディレクトリ", poolID: kago.SidecarPoolID, dir: "main/番組/録画.m2ts.kotatsu", want: &kago.Location{PoolID: poolMain, Path: "番組/録画.m2ts"}},
		{name: "まとめて置くディレクトリの直下はプールが分からない", poolID: kago.SidecarPoolID, dir: "録画.m2ts.kotatsu", wantErr: true},
		{name: "付属ファイルのディレクトリでない", poolID: poolMain, dir: "録画.m2ts", wantErr: true},
		{name: "名前が接尾辞だけ", poolID: poolMain, dir: "番組/.kotatsu", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := kago.SidecarOwner(tt.poolID, tt.dir)
			if tt.wantErr {
				require.ErrorIs(t, err, kago.ErrInvalidPath)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

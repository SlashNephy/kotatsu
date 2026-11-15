package cmdetect_test

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect/logo"
)

// TestMaterialsRoundTrip は、材料を書いて読み戻すと、時刻はマイクロ秒、点数は 1e-4、明るさと肌理は 0.01 の刻みで一致することを確かめる (#1507)。
func TestMaterialsRoundTrip(t *testing.T) {
	t.Parallel()

	want := []cmdetect.SegmentMaterial{
		{Path: "main/a.m2ts#100", Material: &cmdetect.Material{
			Fps: 30000.0 / 1001, Duration: 42*time.Second + 500*time.Millisecond, Corner: logo.TopLeft, Box: logo.Box{X: 10, Y: 20, W: 30, H: 40},
			Times: []float64{0.567233, 0.600600, 0.633966, 2.1},
			Measures: []logo.Measure{
				{Score: 0.41234, Brightness: 200.5, Texture: 3.25},
				{Score: -0.2, Brightness: 0, Texture: 0},
				{Score: 1, Brightness: 255, Texture: 80.125},
				{Score: 0, Brightness: 12.345, Texture: 0.01},
			},
		}},
		{Path: "main/b.m2ts#200", Material: &cmdetect.Material{
			Fps: 59.94, Duration: 0, Corner: logo.BottomRight, Box: logo.Box{X: 1, Y: 2, W: 3, H: 4},
			Times: []float64{0}, Measures: []logo.Measure{{Score: 0.3, Brightness: 100, Texture: 5}},
		}},
	}
	// 1 つ目のセグメントには、今の見分け方の境目の候補がある (#1506)。2 つ目は、材料を読めなかった回とする。
	cmdetect.SetMaterialBounds(want[0].Material, []float64{12.1, 72.7, 216.9}, 1817.3, -0.418)
	data, err := cmdetect.EncodeMaterials(want)
	require.NoError(t, err)
	got, err := cmdetect.DecodeMaterials(data)
	require.NoError(t, err)
	ats, duration, offset := cmdetect.MaterialBounds(got[0].Material)
	assert.Equal(t, []float64{12.1, 72.7, 216.9}, ats)
	assert.InDelta(t, 1817.3, duration, 1e-9)
	assert.InDelta(t, -0.418, offset, 1e-9)
	ats, _, _ = cmdetect.MaterialBounds(got[1].Material)
	assert.Nil(t, ats)

	require.Len(t, got, len(want))
	for i := range want {
		w, g := want[i].Material, got[i].Material
		assert.Equal(t, want[i].Path, got[i].Path)
		assert.InDelta(t, w.Fps, g.Fps, 1e-9)
		assert.Equal(t, w.Duration, g.Duration)
		assert.Equal(t, w.Corner, g.Corner)
		assert.Equal(t, w.Box, g.Box)
		require.Len(t, g.Times, len(w.Times))
		require.Len(t, g.Measures, len(w.Measures))
		for j := range w.Times {
			assert.InDelta(t, w.Times[j], g.Times[j], 1e-6)
			assert.InDelta(t, w.Measures[j].Score, g.Measures[j].Score, 0.5e-4)
			assert.InDelta(t, w.Measures[j].Brightness, g.Measures[j].Brightness, 0.005)
			assert.InDelta(t, w.Measures[j].Texture, g.Measures[j].Texture, 0.005)
		}
	}
}

// TestDecodeMaterialsRejects は、壊れた材料と、途中で切れた材料を、読み戻さずに読み直しに回すことを確かめる (#1507)。
func TestDecodeMaterialsRejects(t *testing.T) {
	t.Parallel()

	data, err := cmdetect.EncodeMaterials([]cmdetect.SegmentMaterial{{Path: "main/a.m2ts#1", Material: &cmdetect.Material{
		Fps: 30, Duration: time.Second, Corner: logo.TopRight, Box: logo.Box{X: 0, Y: 0, W: 1, H: 1},
		Times: []float64{0, 1.0 / 30}, Measures: []logo.Measure{{Score: 0, Brightness: 0, Texture: 0}, {Score: 0, Brightness: 0, Texture: 0}},
	}}})
	require.NoError(t, err)

	tests := []struct {
		name string
		data []byte
	}{
		{name: "not gzip", data: []byte("not a material")},
		{name: "truncated", data: data[:len(data)/2]},
		{name: "empty", data: []byte{}},
		{name: "a frame count that overflows", data: rawMaterial(t, `{"version":1,"segments":[{"path":"a","fps":30,"frames":922337203685477581}]}`)},
		{name: "another format version", data: rawMaterial(t, `{"version":0,"segments":[]}`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := cmdetect.DecodeMaterials(tt.data)
			assert.Error(t, err)
		})
	}
}

// rawMaterial は、header の JSON だけを持つ材料を、EncodeMaterials と同じ包み方で作る。
func rawMaterial(t *testing.T, header string) []byte {
	t.Helper()

	var out bytes.Buffer
	zw := gzip.NewWriter(&out)
	size, err := binary.Append(nil, binary.LittleEndian, int32(float64(len(header))))
	require.NoError(t, err)
	_, err = zw.Write(append(size, header...))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	return out.Bytes()
}

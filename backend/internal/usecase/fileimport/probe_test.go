package fileimport_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/usecase/fileimport"
)

// jst は、TS の時刻の時間帯を返す。
func jst() *time.Location {
	return time.FixedZone("JST", 9*60*60)
}

// loadTestTS は、テストの TS を読む。SDT と EIT は、地上波の「こたつテスト１」(network_id 0x7FE0、service_id 0x0400) に揃えている。
func loadTestTS(t *testing.T) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "ts", "test.ts"))
	require.NoError(t, err)

	return data
}

// bdav は、TS を、パケットの前に 4 バイトの時刻を付けた Blu-ray (BDAV) の形にする。
func bdav(data []byte) []byte {
	out := make([]byte, 0, len(data)/ts.PacketSize*192)
	for offset := 0; offset+ts.PacketSize <= len(data); offset += ts.PacketSize {
		out = append(out, 0x00, 0x00, 0x00, 0x00)
		out = append(out, data[offset:offset+ts.PacketSize]...)
	}

	return out
}

// TestReadProbe は、TS の先頭と終わりから、サービス、番組、録った時刻を読み、BDAV と TS でないファイルを見分けることを確認する (#1734)。
func TestReadProbe(t *testing.T) {
	t.Parallel()

	data := loadTestTS(t)
	// 読み始めがパケットの途中でも、188 バイトごとの sync byte を見つける。
	shifted := append([]byte{0x00, 0x01, 0x02}, data...)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, jst())
	want := &fileimport.Probe{
		NetworkID: new(uint16(0x7FE0)),
		Services: []*fileimport.Service{{
			ServiceID: 0x0400,
			Name:      "こたつテスト１",
			Events: []*fileimport.Event{{
				EventID: 1, Name: "テスト番組", Description: "合成した映像と音声です。", Extended: []*ts.ExtendedItem{},
				Start: &start, Duration: new(30 * time.Minute),
			}},
		}},
		RecordedFrom:  &start,
		RecordedUntil: &start,
	}
	tests := []struct {
		name    string
		data    []byte
		want    *fileimport.Probe
		wantErr error
	}{
		{name: "188 バイトの TS", data: data, want: want, wantErr: nil},
		{name: "先頭がパケットの途中", data: shifted, want: want, wantErr: nil},
		{name: "192 バイトの BDAV", data: bdav(data), want: nil, wantErr: fileimport.ErrBDAV},
		{name: "TS でない", data: bytes.Repeat([]byte("kotatsu"), 1000), want: nil, wantErr: fileimport.ErrNotTS},
		{name: "短すぎる", data: data[:ts.PacketSize*2], want: nil, wantErr: fileimport.ErrNotTS},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := fileimport.ReadProbe(bytes.NewReader(tt.data), int64(len(tt.data)))
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			// 時刻は時間帯の表し方が違っても同じなら等しいとみなすため、文字列にして比べる。
			assert.Equal(t, tt.want.NetworkID, got.NetworkID)
			assert.Equal(t, tt.want.RecordedFrom.String(), got.RecordedFrom.String())
			assert.Equal(t, tt.want.RecordedUntil.String(), got.RecordedUntil.String())
			assert.Equal(t, tt.want.Services, got.Services)
		})
	}
}

// TestServiceProgram は、録った範囲に最も長く重なる番組を選ぶことを確認する (#1734)。
// 録画は番組の前にマージンを取って始まるので、頭で放送中の番組は、前の番組であることが多い。
func TestServiceProgram(t *testing.T) {
	t.Parallel()

	at := func(hour, minute, second int) *time.Time {
		return new(time.Date(2026, 11, 9, hour, minute, second, 0, jst()))
	}
	previous := &fileimport.Event{EventID: 1, Name: "前の番組", Description: "", Extended: []*ts.ExtendedItem{}, Start: at(23, 0, 0), Duration: new(45 * time.Minute)}
	target := &fileimport.Event{EventID: 2, Name: "録った番組", Description: "", Extended: []*ts.ExtendedItem{}, Start: at(23, 45, 0), Duration: new(30 * time.Minute)}
	next := &fileimport.Event{
		EventID: 3, Name: "次の番組", Description: "", Extended: []*ts.ExtendedItem{}, Start: new(target.Start.Add(30 * time.Minute)), Duration: new(30 * time.Minute),
	}
	undecided := &fileimport.Event{EventID: 4, Name: "時刻が未定", Description: "", Extended: []*ts.ExtendedItem{}, Start: nil, Duration: nil}
	tests := []struct {
		name     string
		events   []*fileimport.Event
		from, to *time.Time
		want     *fileimport.Event
	}{
		{name: "前後のマージンを含めて録った", events: []*fileimport.Event{previous, target, next}, from: at(23, 44, 50), to: new(next.Start.Add(10 * time.Second)), want: target},
		{name: "範囲が分からなければ、頭で放送中の番組", events: []*fileimport.Event{previous, target}, from: nil, to: nil, want: previous},
		{name: "範囲が一瞬なら、その時の番組", events: []*fileimport.Event{previous, target}, from: at(23, 50, 0), to: at(23, 50, 0), want: target},
		{name: "どれとも重ならない", events: []*fileimport.Event{previous}, from: at(23, 50, 0), to: at(23, 55, 0), want: nil},
		{name: "時刻が未定の番組は選ばない", events: []*fileimport.Event{undecided}, from: at(23, 50, 0), to: at(23, 55, 0), want: nil},
		{name: "番組がない", events: []*fileimport.Event{}, from: nil, to: nil, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			service := &fileimport.Service{ServiceID: 1, Name: "", Events: tt.events}
			assert.Equal(t, tt.want, service.Program(tt.from, tt.to))
		})
	}
}

// TestReadProbeTail は、頭で読み切れない大きなファイルでは、終わりも読み、同じ番組を 1 つにまとめることを確認する (#1734)。
func TestReadProbeTail(t *testing.T) {
	t.Parallel()

	data := loadTestTS(t)
	// 頭で読む上限 (32 MiB) を超えるよう、null パケットを挟む。
	null := make([]byte, ts.PacketSize)
	null[0], null[1], null[2], null[3] = 0x47, 0x1F, 0xFF, 0x10
	file := append(bytes.Clone(data), bytes.Repeat(null, (40<<20)/ts.PacketSize)...)
	file = append(file, data...)

	got, err := fileimport.ReadProbe(bytes.NewReader(file), int64(len(file)))
	require.NoError(t, err)
	require.Len(t, got.Services, 1)
	require.Len(t, got.Services[0].Events, 1)
	assert.Equal(t, "テスト番組", got.Services[0].Events[0].Name)
	require.NotNil(t, got.RecordedUntil)
}

// TestMergeEvent は、頭で読めなかった番組の項目を、終わりで読んだ同じ番組の項目で埋め、読めた項目は変えないことを確認する (#1734)。
func TestMergeEvent(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 11, 9, 23, 45, 0, 0, jst())
	full := &fileimport.Event{
		EventID: 2, Name: "録った番組", Description: "概要", Extended: []*ts.ExtendedItem{{Heading: "出演者", Text: "こたつ"}},
		Start: &start, Duration: new(30 * time.Minute),
	}
	tests := []struct {
		name string
		dst  *fileimport.Event
		want *fileimport.Event
	}{
		{
			name: "頭で次の番組として読み、名前と時刻がなかった",
			dst:  &fileimport.Event{EventID: 2, Name: "", Description: "", Extended: []*ts.ExtendedItem{}, Start: nil, Duration: nil},
			want: full,
		},
		{
			name: "頭で読めた項目は残す",
			dst:  &fileimport.Event{EventID: 2, Name: "先の名前", Description: "", Extended: []*ts.ExtendedItem{}, Start: nil, Duration: nil},
			want: &fileimport.Event{EventID: 2, Name: "先の名前", Description: full.Description, Extended: full.Extended, Start: full.Start, Duration: full.Duration},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fileimport.MergeEvent(tt.dst, full)
			assert.Equal(t, tt.want, tt.dst)
		})
	}
}

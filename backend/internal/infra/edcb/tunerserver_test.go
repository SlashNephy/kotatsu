package edcb_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/encoding/japanese"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/infra/edcb"
)

func TestParseExtended(t *testing.T) {
	t.Parallel()

	// EDCB が書く形 (EpgDBUtil.cpp の AddExtEvent): 「- 項目名」の行、本文の行を CRLF で並べる。
	// 項目名と本文の行頭の「-…- 」には「-」を 1 つ足す。
	tests := []struct {
		name string
		in   string
		want map[string]string
	}{
		{name: "empty", in: "", want: map[string]string{}},
		{
			name: "items",
			in:   "- 番組内容\r\n今日の特集は鍋料理。\r\n- 出演者\r\n山田太郎\r\n鈴木花子\r\n",
			want: map[string]string{"番組内容": "今日の特集は鍋料理。", "出演者": "山田太郎\n鈴木花子"},
		},
		{
			// 本文の行頭の「- 」は「-- 」に、「-- 」は「--- 」にエスケープされている。
			name: "escaped hyphens",
			in:   "- 内容\r\n-- 一つ目\r\n--- 二つ目\r\n-まだ項目でない\r\n",
			want: map[string]string{"内容": "- 一つ目\n-- 二つ目\n-まだ項目でない"},
		},
		{
			name: "escaped item name",
			in:   "- -- 注意\r\n本文\r\n",
			want: map[string]string{"- 注意": "本文"},
		},
		{name: "text before the first item", in: "前書き\r\n- 項目\r\n本文\r\n", want: map[string]string{"項目": "本文"}},
		{name: "item without body", in: "- 項目\r\n", want: map[string]string{"項目": ""}},
		// 本文がもともと改行で終わるときは、区切りの改行が続き、空の行になる。その改行は残す。
		{name: "body ending with a newline", in: "- 一\r\n本文\r\n\r\n- 二\r\n本文\r\n\r\n", want: map[string]string{"一": "本文\n", "二": "本文\n"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, edcb.ParseExtended(tt.in))
		})
	}
}

func TestParseChannelSettings(t *testing.T) {
	t.Parallel()

	sjis, err := japanese.ShiftJIS.NewEncoder().Bytes([]byte("ＮＨＫ総合\t関東広域０\t32736\t32736\t1024\t1\t0\t1\t1\r\n"))
	require.NoError(t, err)
	nhk := &edcb.ChannelEntry{
		ONID: 32736, TSID: 32736, SID: 1024, Name: "ＮＨＫ総合", NetworkName: "関東広域０",
		Type: 1, PartialReception: false, CaptureEPG: true, Search: true,
	}
	tests := []struct {
		name    string
		in      []byte
		want    []*edcb.ChannelEntry
		wantErr bool
	}{
		{name: "utf-8 with bom", in: []byte("\xEF\xBB\xBFＮＨＫ総合\t関東広域０\t32736\t32736\t1024\t1\t0\t1\t1\r\n"), want: []*edcb.ChannelEntry{nhk}},
		// 古い Windows の EDCB は、Shift_JIS で書く。
		{name: "shift_jis", in: sjis, want: []*edcb.ChannelEntry{nhk}},
		{
			name: "later line wins and lines without tabs are skipped",
			in:   []byte("comment\nold\t\t32736\t32736\t1024\t1\t0\t0\t0\nＮＨＫ総合\t関東広域０\t32736\t32736\t1024\t1\t0\t1\t1\n\n"),
			want: []*edcb.ChannelEntry{nhk},
		},
		{name: "missing columns are zero", in: []byte("A\tB\t1\t2\t3\n"), want: []*edcb.ChannelEntry{{
			ONID: 1, TSID: 2, SID: 3, Name: "A", NetworkName: "B", Type: 0, PartialReception: false, CaptureEPG: false, Search: false,
		}}},
		{name: "id out of range", in: []byte("A\tB\t70000\t2\t3\t1\t0\t1\t1\n"), wantErr: true},
		{name: "not a number", in: []byte("A\tB\tx\t2\t3\t1\t0\t1\t1\n"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := edcb.ParseChannelSettings(tt.in)
			if tt.wantErr {
				require.ErrorIs(t, err, edcb.ErrMalformed)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestChannelType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		onid uint16
		want string
	}{
		{onid: 0x7FE0, want: "GR"},
		{onid: 0x7880, want: "GR"},
		{onid: 0x7FE8, want: "GR"},
		{onid: 0x0004, want: "BS"},
		{onid: 0x0006, want: "CS"},
		{onid: 0x0007, want: "CS"},
		{onid: 0x000B, want: "BS4K"},
		{onid: 0x0001, want: "SKY"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, edcb.ChannelType(tt.onid), "%#x", tt.onid)
	}
}

// edcbServer は、実際の EDCB から取った ChSet5、サービス、番組の応答を返す偽の EpgTimerSrv。
func edcbServer(t *testing.T) *fakeServer {
	t.Helper()

	return newFakeServer(t, map[uint32]reply{
		1060: {status: 1, payload: fixture(t, "file_copy_chset5.bin"), stream: nil, hold: false, drop: false},
		1021: {status: 1, payload: fixture(t, "enum_service.bin"), stream: nil, hold: false, drop: false},
		1029: {status: 1, payload: fixture(t, "enum_pg_info_ex.bin"), stream: nil, hold: false, drop: false},
	})
}

func TestTunerServerServices(t *testing.T) {
	t.Parallel()

	services, err := edcb.NewTunerServer(edcbServer(t).client(t), nil).Services(t.Context())
	require.NoError(t, err)

	// ENUM_SERVICE は EPG を読み込んだ 31 サービスだけを返すが、一覧はチャンネルスキャンで拾った 39 サービスにする。
	require.Len(t, services, 39)
	byID := map[int64]*tunerserver.Service{}
	for _, s := range services {
		byID[s.ID] = s
	}
	assert.Equal(t, &tunerserver.Service{
		ID: 3273601024, ServiceID: 1024, NetworkID: 32736, Name: "ＮＨＫ総合１・東京", Type: 1, LogoID: nil,
		// リモコンキー ID は、ENUM_SERVICE から補う。
		RemoteControlKeyID: new(1),
		Channel:            &tunerserver.ChannelRef{Type: "GR", Channel: "32736"},
	}, byID[3273601024])
}

func TestTunerServerServicesWhenBusy(t *testing.T) {
	t.Parallel()

	// EPG のデータを読み込み終えていなければ、空の一覧にせず、ErrBusy を返す。空の一覧として扱うと、DB のサービスを消してしまう。
	s := newFakeServer(t, map[uint32]reply{
		1060: {status: 1, payload: fixture(t, "file_copy_chset5.bin"), stream: nil, hold: false, drop: false},
		1021: {status: 208, payload: nil, stream: nil, hold: false, drop: false},
	})
	_, err := edcb.NewTunerServer(s.client(t), nil).Services(t.Context())
	require.ErrorIs(t, err, edcb.ErrBusy)
}

func TestTunerServerPrograms(t *testing.T) {
	t.Parallel()

	s := edcbServer(t)
	programs, err := edcb.NewTunerServer(s.client(t), nil).Programs(t.Context(), nil)
	require.NoError(t, err)

	require.Len(t, programs, 5)
	p := programs[0]
	// 番組名と概要は、テストの材料では伏せ字になっている。
	assert.Equal(t, int64(327360102413578), p.ID)
	assert.Equal(t, 13578, p.EventID)
	assert.Equal(t, 1024, p.ServiceID)
	assert.Equal(t, 32736, p.NetworkID)
	assert.Equal(t, time.Date(2026, 11, 13, 14, 0, 0, 0, time.UTC).UnixMilli(), p.StartAt)
	assert.Equal(t, (30 * time.Minute).Milliseconds(), p.Duration)
	assert.True(t, p.IsFree)
	assert.NotEmpty(t, p.Name)
	assert.Equal(t, []*domain.Genre{{Lv1: 5, Lv2: 15, Un1: 15, Un2: 15}, {Lv1: 8, Lv2: 2, Un1: 15, Un2: 15}, {Lv1: 8, Lv2: 4, Un1: 15, Un2: 15}}, p.Genres)
	assert.Equal(t, []*domain.Audio{{ComponentType: 3, ComponentTag: new(0x10), IsMain: true, SamplingRate: 48000, Langs: []string{}}}, p.Audios)
	assert.Equal(t, &tunerserver.Video{Resolution: "1080i"}, p.Video)
	// 共有のイベントグループは、Mirakurun と同じく、ネットワーク ID を持たない。
	assert.Equal(t, []*domain.RelatedItem{
		{Type: "shared", NetworkID: 0, ServiceID: 1024, EventID: 13578},
		{Type: "shared", NetworkID: 0, ServiceID: 1025, EventID: 13578},
	}, p.RelatedItems)
	assert.Nil(t, p.Series)
	// 全件を引くときは、すべてのサービスを引く要求を送る。
	assert.Equal(t, []uint32{1029}, commands(s.received()))
}

func TestTunerServerProgramsOfService(t *testing.T) {
	t.Parallel()

	s := edcbServer(t)
	programs, err := edcb.NewTunerServer(s.client(t), nil).Programs(t.Context(), &tunerserver.ProgramsQuery{NetworkID: 32736, ServiceID: 1024, EventID: 13583})
	require.NoError(t, err)

	// 番組 ID は、要求ではなく、受け取った後で絞る。
	require.Len(t, programs, 1)
	assert.Equal(t, 13583, programs[0].EventID)
	// サービスを絞るときは、チャンネルの設定から TSID を引いて、そのサービスだけを引く。
	received := s.received()
	assert.Equal(t, []uint32{1060, 1029}, commands(received))
	assert.Equal(t, fromHex(t, "28000000040000000000000000000000"+"0004e07fe07f0000"+"0000000000000000"+"ffffffffffffff7f"), received[1].body)
}

func TestTunerServerProgramsOfUnknownService(t *testing.T) {
	t.Parallel()

	s := edcbServer(t)
	programs, err := edcb.NewTunerServer(s.client(t), nil).Programs(t.Context(), &tunerserver.ProgramsQuery{NetworkID: 1, ServiceID: 2, EventID: 0})
	require.NoError(t, err)
	assert.Empty(t, programs)
	assert.Equal(t, []uint32{1060}, commands(s.received()))
}

func commands(requests []request) []uint32 {
	out := make([]uint32, 0, len(requests))
	for _, r := range requests {
		out = append(out, r.command)
	}

	return out
}

// event は、EPGDB_EVENT_INFO を組む。name が空なら、番組名などを持たない (サブチャンネルの番組)。
// shared は、共有のイベントグループで指す番組の SID と番組 ID の組を並べたもの。
func event(sid, eid uint16, name string, shared ...uint16) payload {
	body := payload{}.u16(32736).u16(32736).u16(sid).u16(eid).u8(1)
	for _, v := range []uint16{2026, 11, 5, 13, 21, 0, 0, 0} {
		body = body.u16(v)
	}
	body = body.u8(1).u32(1800)
	short, genres := sized(nil), sized(nil)
	if name != "" {
		short = sized(payload{}.str(name).str("概要"))
		genres = sized(vector(sized(payload{}.u8(7).u8(0).u8(0xF).u8(0xF))))
	}
	body = body.append(short).append(sized(nil)).append(genres).append(sized(nil)).append(sized(nil))
	group := sized(nil)
	if len(shared) > 0 {
		refs := []payload{}
		for i := 0; i+1 < len(shared); i += 2 {
			refs = append(refs, sized(payload{}.u16(32736).u16(32736).u16(shared[i]).u16(shared[i+1])))
		}
		group = sized(payload{}.u8(1).append(vector(refs...)))
	}

	return sized(body.append(group).append(sized(nil)).u8(0))
}

// serviceEvents は、EPGDB_SERVICE_EVENT_INFO を組む。
func serviceEvents(sid uint16, events ...payload) payload {
	info := sized(payload{}.u16(32736).u16(32736).u16(sid).u8(1).u8(0).str("").str("").str("").str("").u8(1))

	return sized(info.append(vector(events...)))
}

func TestTunerServerProgramsShareDetails(t *testing.T) {
	t.Parallel()

	// サブチャンネル (1025) の番組 100 は、メインのチャンネル (1024) の番組 100 を指す共有のイベントグループだけを持つ。
	// 番組 200 は、何も指さない。
	main := serviceEvents(1024, event(1024, 100, "ニュース", 1024, 100, 1025, 100))
	sub := serviceEvents(1025, event(1025, 100, "", 1024, 100, 1025, 100), event(1025, 200, ""))
	chSet := []byte("総合\t関東\t32736\t32736\t1024\t1\t0\t1\t1\n総合２\t関東\t32736\t32736\t1025\t1\t0\t1\t1\n")
	tests := []struct {
		name   string
		query  *tunerserver.ProgramsQuery
		events []reply
		want   []uint32
	}{
		{
			name:   "all services",
			query:  nil,
			events: []reply{{status: 1, payload: vector(main, sub), stream: nil, hold: false, drop: false}},
			want:   []uint32{1029},
		},
		{
			// サブチャンネルだけを引いたときは、指す先のメインのチャンネルの番組も引く。
			name:  "sub-channel only",
			query: &tunerserver.ProgramsQuery{NetworkID: 32736, ServiceID: 1025, EventID: 0},
			events: []reply{
				{status: 1, payload: vector(sub), stream: nil, hold: false, drop: false},
				{status: 1, payload: vector(main), stream: nil, hold: false, drop: false},
			},
			want: []uint32{1060, 1029, 1029},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := newFakeServer(t, map[uint32]reply{1060: {status: 1, payload: chSet, stream: nil, hold: false, drop: false}})
			s.queued[1029] = tt.events
			programs, err := edcb.NewTunerServer(s.client(t), nil).Programs(t.Context(), tt.query)
			require.NoError(t, err)

			byID := map[int64]*tunerserver.Program{}
			for _, p := range programs {
				byID[p.ID] = p
			}
			shared := byID[(32736*100000+1025)*100000+100]
			require.NotNil(t, shared)
			assert.Equal(t, "ニュース", shared.Name)
			assert.Equal(t, "概要", shared.Description)
			assert.Equal(t, []*domain.Genre{{Lv1: 7, Lv2: 0, Un1: 15, Un2: 15}}, shared.Genres)
			// 指す先のない番組は、文を持たないまま返す。
			alone := byID[(32736*100000+1025)*100000+200]
			require.NotNil(t, alone)
			assert.Empty(t, alone.Name)
			assert.Nil(t, alone.Genres)
			assert.Equal(t, tt.want, commands(s.received()))
		})
	}
}

func TestTunerServerProgramsRestoresText(t *testing.T) {
	t.Parallel()

	// EDCB は、追加記号を角括弧の形に、〜と—を CP932 の文字に置き換え、改行を CRLF で返す。
	// 同じ形が複数の記号に当たる「[三]」は、戻さない。
	list := vector(serviceEvents(1024, event(1024, 100, "ニュース[字][三]～―\r\n続き")))
	s := newFakeServer(t, map[uint32]reply{1029: {status: 1, payload: list, stream: nil, hold: false, drop: false}})
	programs, err := edcb.NewTunerServer(s.client(t), nil).Programs(t.Context(), nil)
	require.NoError(t, err)
	require.Len(t, programs, 1)
	assert.Equal(t, "ニュース🈑[三]〜—\n続き", programs[0].Name)
}

func TestTunerServerOnAirPrograms(t *testing.T) {
	t.Parallel()

	s := newFakeServer(t, map[uint32]reply{})
	programs, err := edcb.NewTunerServer(s.client(t), nil).OnAirPrograms(t.Context())
	require.NoError(t, err)
	assert.Empty(t, programs)
	assert.Empty(t, s.received())
}

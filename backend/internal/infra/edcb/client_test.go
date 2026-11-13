package edcb_test

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/edcb"
)

// testWait と testTick は、偽の EpgTimerSrv が要求を受け取るのを待つ上限と間隔。
const (
	testWait = 5 * time.Second
	testTick = 10 * time.Millisecond
)

// reply は、偽の EpgTimerSrv が 1 つのコマンドに返す応答。
type reply struct {
	status  uint32
	payload []byte
	// stream は、応答の後に続けて送るバイト列 (RELAY_VIEW_STREAM の TS)。
	stream []byte
	// hold は、応答を返さずに、接続が閉じられるまで待つかどうか (保留した GET_STATUS_NOTIFY2)。
	hold bool
}

// request は、偽の EpgTimerSrv が受け取った要求。
type request struct {
	command uint32
	body    []byte
}

// fakeServer は、CtrlCmd を話す偽の EpgTimerSrv。コマンドごとに、決めた応答を返す。
type fakeServer struct {
	listener net.Listener
	replies  map[uint32]reply
	// queued は、replies より先に、届いた順に返す応答。使い切ったら replies を返す。
	queued map[uint32][]reply

	mu       sync.Mutex
	requests []request
}

func newFakeServer(t *testing.T, replies map[uint32]reply) *fakeServer {
	t.Helper()

	var config net.ListenConfig
	listener, err := config.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	s := &fakeServer{listener: listener, replies: replies, queued: map[uint32][]reply{}, mu: sync.Mutex{}, requests: []request{}}
	t.Cleanup(func() { _ = listener.Close() })
	go s.serve()

	return s
}

func (s *fakeServer) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *fakeServer) handle(conn net.Conn) {
	defer conn.Close()
	head := make([]byte, 8)
	if _, err := io.ReadFull(conn, head); err != nil {
		return
	}
	command := binary.LittleEndian.Uint32(head)
	body := make([]byte, binary.LittleEndian.Uint32(head[4:]))
	if _, err := io.ReadFull(conn, body); err != nil {
		return
	}
	s.mu.Lock()
	s.requests = append(s.requests, request{command: command, body: body})
	r, ok := s.replies[command]
	if queue := s.queued[command]; len(queue) > 0 {
		r, ok, s.queued[command] = queue[0], true, queue[1:]
	}
	s.mu.Unlock()

	if !ok {
		r = reply{status: 203, payload: nil, stream: nil, hold: false}
	}
	if r.hold {
		_, _ = io.Copy(io.Discard, conn)

		return
	}
	res := binary.LittleEndian.AppendUint32(nil, r.status)
	res = binary.LittleEndian.AppendUint32(res, dword(len(r.payload)))
	res = append(res, r.payload...)
	res = append(res, r.stream...)
	_, _ = conn.Write(res)
}

func (s *fakeServer) received() []request {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]request{}, s.requests...)
}

func (s *fakeServer) client(t *testing.T) *edcb.Client {
	t.Helper()

	client, err := edcb.New(&edcb.Options{Address: s.listener.Addr().String(), DialTimeout: 0, RequestTimeout: 0, Dial: nil})
	require.NoError(t, err)

	return client
}

// fixture は、実際の EDCB (xtne6f/EDCB e6e3c61、Linux) が返した応答の本文を読む。
func fixture(t *testing.T, name string) []byte {
	t.Helper()

	b, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)

	return b
}

// ipv6Address は、IPv6 の接続先の書き方。
const ipv6Address = "[::1]:4510"

func TestNewNormalizesAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "192.168.1.10", want: "192.168.1.10:4510"},
		{in: "edcb.local:14510", want: "edcb.local:14510"},
		{in: ipv6Address, want: ipv6Address},
		{in: "::1", want: ipv6Address},
		{in: "", wantErr: true},
		{in: "edcb.local:99999", wantErr: true},
		{in: ":4510", wantErr: true},
		{in: "edcb.local::4510", wantErr: true},
		{in: "192.168.1.10:4510:extra", wantErr: true},
		{in: "edcb.local:abc", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()

			client, err := edcb.New(&edcb.Options{Address: tt.in, DialTimeout: 0, RequestTimeout: 0, Dial: nil})
			if tt.wantErr {
				require.Error(t, err)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, client.Address())
		})
	}
}

func TestStatusErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status uint32
		want   error
	}{
		{name: "busy", status: 208, want: edcb.ErrBusy},
		{name: "not supported", status: 203, want: edcb.ErrNotSupported},
		{name: "error", status: 0, want: edcb.ErrFailed},
		{name: "invalid argument", status: 204, want: edcb.ErrFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := newFakeServer(t, map[uint32]reply{1021: {status: tt.status, payload: nil, stream: nil, hold: false}})
			_, err := s.client(t).Services(t.Context())
			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestWaitNotificationStopsWithContext(t *testing.T) {
	t.Parallel()

	// 新しい通知がなければ、EpgTimerSrv は応答を保留する。待つのは ctx が終わるまで。
	s := newFakeServer(t, map[uint32]reply{2200: {status: 0, payload: nil, stream: nil, hold: true}})
	ctx, cancel := context.WithCancel(t.Context())
	client := s.client(t)
	done := make(chan error, 1)
	go func() {
		_, err := client.WaitNotification(ctx, 3)
		done <- err
	}()
	require.Eventually(t, func() bool { return len(s.received()) == 1 }, testWait, testTick)
	cancel()
	err := <-done
	require.ErrorIs(t, err, context.Canceled)
	assert.NotErrorIs(t, err, edcb.ErrMalformed)
}

// fromHex は、16 進の文字列をバイト列にする。
func fromHex(t *testing.T, s string) []byte {
	t.Helper()

	b, err := hex.DecodeString(s)
	require.NoError(t, err)

	return b
}

// 以下の要求の本文は、テストの材料を取ったときに、実際の EDCB に送ったもの。クライアントが同じバイト列を組むことを確かめる。
const (
	// eventsRequest は、NHK 総合 (32736, 32736, 1024) の、2026-11-13 22:00 から 24:00 (日本標準時) に始まる番組を引く要求。
	eventsRequest = "280000000400000000000000000000000004e07fe07f000000f0ea53ba73dd0100c07317cb73dd01"
	// openRequest は、NetworkTV の ID 900 で NHK Eテレ (32737, 32737, 1032) を開き、TS を TCP で送らせる要求。
	openRequest = "1a00000001000000e17fe17f0804010000008403000002000000"
	// checkRequest は、NetworkTV の ID 901 が開いているかを確かめる要求 (useSID=0)。
	checkRequest = "1a00000000000000000000000000010000008503000002000000"
	// filesRequest は、LogoData.ini と ChSet5.txt をまとめて求める要求。
	filesRequest = "050040000000020000001e0000004c006f0067006f0044006100740061002e0069006e00690000001a0000004300680053006500740035002e007400780074000000"
	// chSetRequest は、ChSet5.txt を求める要求。
	chSetRequest = "1a0000004300680053006500740035002e007400780074000000"
)

func TestServicesFromRealEDCB(t *testing.T) {
	t.Parallel()

	s := newFakeServer(t, map[uint32]reply{1021: {status: 1, payload: fixture(t, "enum_service.bin"), stream: nil, hold: false}})
	services, err := s.client(t).Services(t.Context())
	require.NoError(t, err)

	require.Len(t, services, 31)
	assert.Equal(t, &edcb.Service{
		ONID: 32391, TSID: 32391, SID: 23608, Type: 1, PartialReception: false,
		ProviderName: "", Name: "ＴＯＫＹＯ　ＭＸ１", NetworkName: "東京７", TSName: "ＴＯＫＹＯ　ＭＸ", RemoteControlKeyID: 9,
	}, services[0])
	// ワンセグのサービスは、部分受信の印が付く。
	assert.Equal(t, &edcb.Service{
		ONID: 32742, TSID: 32742, SID: 1456, Type: 0xC0, PartialReception: true,
		ProviderName: "", Name: "テレ東携帯", NetworkName: "関東広域６", TSName: "テレビ東京", RemoteControlKeyID: 7,
	}, services[30])
	assert.Equal(t, []request{{command: 1021, body: []byte{}}}, s.received())
}

func TestEventsFromRealEDCB(t *testing.T) {
	t.Parallel()

	// 番組名、概要、詳細は、公開リポジトリに放送局の文章を置かないよう、同じ長さの伏せ字 (〇) に置き換えてある。
	s := newFakeServer(t, map[uint32]reply{1029: {status: 1, payload: fixture(t, "enum_pg_info_ex.bin"), stream: nil, hold: false}})
	jst := time.FixedZone("JST", 9*60*60)
	nhk := edcb.ServiceKey{ONID: 32736, TSID: 32736, SID: 1024}
	got, err := s.client(t).Events(t.Context(), &edcb.EventsQuery{
		Services: []edcb.ServiceKey{nhk},
		From:     time.Date(2026, 11, 13, 22, 0, 0, 0, jst),
		To:       time.Date(2026, 11, 14, 0, 0, 0, 0, jst),
	})
	require.NoError(t, err)

	assert.Equal(t, []request{{command: 1029, body: fromHex(t, eventsRequest)}}, s.received())
	require.Len(t, got, 1)
	assert.Equal(t, nhk, got[0].Service.ServiceKey)
	assert.Equal(t, "ＮＨＫ総合１・東京", got[0].Service.Name)
	require.Len(t, got[0].Events, 5)
	e := got[0].Events[0]
	assert.Equal(t, uint16(13578), e.EventID)
	assert.Equal(t, nhk, e.ServiceKey)
	assert.True(t, e.StartKnown)
	assert.True(t, time.Date(2026, 11, 13, 23, 0, 0, 0, jst).Equal(e.Start), "%v", e.Start)
	assert.True(t, e.DurationKnown)
	assert.Equal(t, 30*time.Minute, e.Duration)
	require.NotNil(t, e.Short)
	assert.NotEmpty(t, e.Short.Name)
	assert.NotNil(t, e.Extended)
	assert.Equal(t, []edcb.Genre{{Level1: 5, Level2: 15, User1: 15, User2: 15}, {Level1: 8, Level2: 2, User1: 15, User2: 15}, {Level1: 8, Level2: 4, User1: 15, User2: 15}}, e.Genres)
	assert.Equal(t, &edcb.Component{StreamContent: 1, ComponentType: 0xB3, ComponentTag: 0, Text: ""}, e.Video)
	assert.Equal(t, []edcb.AudioComponent{{
		StreamContent: 2, ComponentType: 3, ComponentTag: 0x10, StreamType: 0x0F, SimulcastGroupTag: 0xFF,
		ESMultiLingual: false, MainComponent: true, QualityIndicator: 2, SamplingRate: 7, Text: "",
	}}, e.Audios)
	// 総合のサブチャンネル (1025) と同じ番組を流す、共有のイベントグループ。
	assert.Equal(t, &edcb.EventGroup{Type: 1, Events: []edcb.EventRef{
		{ONID: 32736, TSID: 32736, SID: 1024, EventID: 13578},
		{ONID: 32736, TSID: 32736, SID: 1025, EventID: 13578},
	}}, e.Group)
	assert.Nil(t, e.Relay)
	assert.False(t, e.Scrambled)
	// 詳細のない番組は、Extended が nil になる。
	assert.Nil(t, got[0].Events[2].Extended)
}

func TestEventsRequest(t *testing.T) {
	t.Parallel()

	jst := time.FixedZone("JST", 9*60*60)
	all := func(from, to int64) string {
		b := binary.LittleEndian.AppendUint32(nil, 8+4*8)
		b = binary.LittleEndian.AppendUint32(b, 4)
		for _, v := range []int64{0xFFFF_FFFF_FFFF, 0xFFFF_FFFF_FFFF, from, to} {
			b, _ = binary.Append(b, binary.LittleEndian, v)
		}

		return hex.EncodeToString(b)
	}
	tests := []struct {
		name  string
		query *edcb.EventsQuery
		want  string
	}{
		// 範囲を決めなければ、開始時刻が未定の番組も含めて、すべてを引く (EpgTimerSrv は 0 と最大の値を特別に扱う)。
		{name: "everything", query: &edcb.EventsQuery{Services: nil, From: time.Time{}, To: time.Time{}}, want: all(0, 1<<63-1)},
		{name: "nil query", query: nil, want: all(0, 1<<63-1)},
		// 終わりだけを決めたときは、未定の番組を含めないよう、始まりを 1 にする。
		{
			name:  "until",
			query: &edcb.EventsQuery{Services: nil, From: time.Time{}, To: time.Date(2026, 11, 14, 0, 0, 0, 0, jst)},
			want:  all(1, 0x01dd73cb1773c000),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := newFakeServer(t, map[uint32]reply{1029: {status: 1, payload: fromHex(t, "0800000000000000"), stream: nil, hold: false}})
			got, err := s.client(t).Events(t.Context(), tt.query)
			require.NoError(t, err)
			assert.Empty(t, got)
			assert.Equal(t, []request{{command: 1029, body: fromHex(t, tt.want)}}, s.received())
		})
	}
}

func TestNetworkTVFromRealEDCB(t *testing.T) {
	t.Parallel()

	s := newFakeServer(t, map[uint32]reply{1073: {status: 1, payload: fixture(t, "nwtv_open.bin"), stream: nil, hold: false}})
	pid, err := s.client(t).OpenNetworkTV(t.Context(), 900, edcb.ServiceKey{ONID: 32737, TSID: 32737, SID: 1032})
	require.NoError(t, err)
	assert.Equal(t, int32(27), pid)
	assert.Equal(t, []request{{command: 1073, body: fromHex(t, openRequest)}}, s.received())
}

func TestNetworkTVProcess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		reply    reply
		wantPID  int32
		wantOpen bool
	}{
		{name: "open", reply: reply{status: 1, payload: fixture(t, "nwtv_check_open.bin"), stream: nil, hold: false}, wantPID: 27, wantOpen: true},
		// 開いていなければ (EDCB の予約にチューナーを取られたときも)、EpgTimerSrv は CMD_ERR (0) を返す。
		{name: "closed", reply: reply{status: 0, payload: nil, stream: nil, hold: false}, wantPID: 0, wantOpen: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := newFakeServer(t, map[uint32]reply{1073: tt.reply})
			pid, open, err := s.client(t).NetworkTVProcess(t.Context(), 901)
			require.NoError(t, err)
			assert.Equal(t, tt.wantPID, pid)
			assert.Equal(t, tt.wantOpen, open)
			assert.Equal(t, []request{{command: 1073, body: fromHex(t, checkRequest)}}, s.received())
		})
	}
}

func TestCloseNetworkTV(t *testing.T) {
	t.Parallel()

	s := newFakeServer(t, map[uint32]reply{1074: {status: 1, payload: nil, stream: nil, hold: false}})
	require.NoError(t, s.client(t).CloseNetworkTV(t.Context(), 900))
	assert.Equal(t, []request{{command: 1074, body: fromHex(t, "84030000")}}, s.received())
}

func TestRelayStream(t *testing.T) {
	t.Parallel()

	// 応答の後に、TS がそのまま続く。偽の TS は、同期バイトだけを置いた 2 パケット。
	ts := make([]byte, 2*188)
	ts[0], ts[188] = 0x47, 0x47
	s := newFakeServer(t, map[uint32]reply{301: {status: 1, payload: nil, stream: ts, hold: false}})
	stream, err := s.client(t).RelayStream(t.Context(), 27)
	require.NoError(t, err)
	got, err := io.ReadAll(stream)
	require.NoError(t, err)
	require.NoError(t, stream.Close())
	assert.Equal(t, ts, got)
	assert.Equal(t, []request{{command: 301, body: fromHex(t, "1b000000")}}, s.received())
}

func TestRelayStreamFails(t *testing.T) {
	t.Parallel()

	// 1 つのプロセスの TS を受け取れるのは 2 つまでで、3 つ目には CMD_ERR が返る。
	s := newFakeServer(t, map[uint32]reply{301: {status: 0, payload: nil, stream: nil, hold: false}})
	_, err := s.client(t).RelayStream(t.Context(), 27)
	require.ErrorIs(t, err, edcb.ErrFailed)
}

func TestTunersFromRealEDCB(t *testing.T) {
	t.Parallel()

	s := newFakeServer(t, map[uint32]reply{
		1016: {status: 1, payload: fixture(t, "enum_tuner_reserve.bin"), stream: nil, hold: false},
		1066: {status: 1, payload: fixture(t, "enum_tuner_process_open.bin"), stream: nil, hold: false},
	})
	client := s.client(t)

	reserves, err := client.TunerReserves(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []*edcb.TunerReserve{
		{TunerID: 1, TunerName: "BonDriver_LinuxMirakurun.so", ReserveIDs: []uint32{}},
		{TunerID: 2, TunerName: "BonDriver_LinuxMirakurun.so", ReserveIDs: []uint32{}},
		// 最後は、チューナーが足りない予約を集めたもの。
		{TunerID: edcb.UnassignedTunerID, TunerName: "チューナー不足", ReserveIDs: []uint32{}},
	}, reserves)

	processes, err := client.TunerProcesses(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []*edcb.TunerProcess{{
		TunerID: 2, ProcessID: 27, Drop: 0, Scramble: 0, SignalLevel: 0, Space: -1, Channel: -1, ONID: -1, TSID: -1,
		Recording: false, CapturingEPG: false, ExtraFlags: 0,
	}}, processes)
}

func TestTunerProcessesWhenIdle(t *testing.T) {
	t.Parallel()

	// 起動しているチューナーがなければ、空の配列が返る。
	s := newFakeServer(t, map[uint32]reply{1066: {status: 1, payload: fixture(t, "enum_tuner_process_idle.bin"), stream: nil, hold: false}})
	processes, err := s.client(t).TunerProcesses(t.Context())
	require.NoError(t, err)
	assert.Empty(t, processes)
}

func TestNotificationsFromRealEDCB(t *testing.T) {
	t.Parallel()

	jst := time.FixedZone("JST", 9*60*60)
	tests := []struct {
		name    string
		fixture string
		count   uint32
		want    *edcb.Notification
	}{
		{
			// count が 0 なら、今の動作の状態 (2: EPG の取得中) を、最後の通知の巡回カウンタと一緒に返す。
			name: "status", fixture: "notify_status.bin", count: 0,
			want: &edcb.Notification{
				ID: edcb.NotifyUpdateSrvStatus, Time: time.Date(2026, 11, 13, 22, 6, 59, 650*int(time.Millisecond), jst),
				Param1: 2, Param2: 0, Count: 9, Param4: "", Param5: "", Param6: "",
			},
		},
		{
			name: "epg capture end", fixture: "notify_epgcap_end.bin", count: 12,
			want: &edcb.Notification{
				ID: edcb.NotifyEPGCapEnd, Time: time.Date(2026, 11, 13, 22, 12, 53, 345*int(time.Millisecond), jst),
				Param1: 0, Param2: 0, Count: 13, Param4: "", Param5: "", Param6: "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := newFakeServer(t, map[uint32]reply{2200: {status: 1, payload: fixture(t, tt.fixture), stream: nil, hold: false}})
			got, err := s.client(t).WaitNotification(t.Context(), tt.count)
			require.NoError(t, err)
			assert.True(t, tt.want.Time.Equal(got.Time), "%v", got.Time)
			got.Time = tt.want.Time
			assert.Equal(t, tt.want, got)
			body := binary.LittleEndian.AppendUint16(nil, 5)
			body = binary.LittleEndian.AppendUint32(body, tt.count)
			assert.Equal(t, []request{{command: 2200, body: body}}, s.received())
		})
	}
}

func TestFilesFromRealEDCB(t *testing.T) {
	t.Parallel()

	chSet := fixture(t, "file_copy_chset5.bin")
	s := newFakeServer(t, map[uint32]reply{
		2060: {status: 1, payload: fixture(t, "file_copy2.bin"), stream: nil, hold: false},
		1060: {status: 1, payload: chSet, stream: nil, hold: false},
	})
	client := s.client(t)

	files, err := client.Files(t.Context(), []string{"LogoData.ini", "ChSet5.txt"})
	require.NoError(t, err)
	require.Len(t, files, 2)
	// ロゴをまだ受け取っていなければ、LogoData.ini は空で返る。
	assert.Equal(t, &edcb.File{Name: "LogoData.ini", Data: []byte{}}, files[0])
	assert.Equal(t, &edcb.File{Name: "ChSet5.txt", Data: chSet}, files[1])

	got, err := client.ChannelSettings(t.Context())
	require.NoError(t, err)
	assert.Equal(t, chSet, got)
	assert.Equal(t, []request{
		{command: 2060, body: fromHex(t, filesRequest)},
		{command: 1060, body: fromHex(t, chSetRequest)},
	}, s.received())
}

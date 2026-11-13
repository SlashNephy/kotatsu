package edcb_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/infra/edcb"
)

// labBonDriver は、応答を取った EDCB の BonDriver の名前。
const labBonDriver = "BonDriver_LinuxMirakurun.so"

// tunerServer は、実際の EDCB の、チューナーが 2 つ (2 つ目を視聴中) で、地上波だけのチャンネルの設定を返す偽のサーバー。
func tunerServer(t *testing.T) *fakeServer {
	t.Helper()

	return newFakeServer(t, map[uint32]reply{
		1016: {status: 1, payload: fixture(t, "enum_tuner_reserve.bin"), stream: nil, hold: false, drop: false},
		1066: {status: 1, payload: fixture(t, "enum_tuner_process_open.bin"), stream: nil, hold: false, drop: false},
		1060: {status: 1, payload: fixture(t, "file_copy_chset5.bin"), stream: nil, hold: false, drop: false},
	})
}

func TestTunerServerTuners(t *testing.T) {
	t.Parallel()

	tuner := func(index int, available, using bool) *tunerserver.Tuner {
		return &tunerserver.Tuner{
			Index: index, Name: labBonDriver, Types: []string{"GR"}, Users: []*tunerserver.TunerUser{},
			IsAvailable: available, IsRemote: false, IsFree: !using, IsUsing: using, IsFault: false,
		}
	}
	tests := []struct {
		name       string
		bonDrivers []string
		want       []*tunerserver.Tuner
	}{
		// チューナーが足りない予約を集めた行は、チューナーにしない。
		{name: "no BonDrivers configured", bonDrivers: nil, want: []*tunerserver.Tuner{tuner(1, true, false), tuner(2, true, true)}},
		// BonDriver の名前は、Windows のファイル名なので、大文字と小文字を区別しない。
		{name: "listed BonDriver", bonDrivers: []string{"bondriver_linuxmirakurun.so"}, want: []*tunerserver.Tuner{tuner(1, true, false), tuner(2, true, true)}},
		{name: "unlisted BonDriver", bonDrivers: []string{"BonDriver_mirakc.dll"}, want: []*tunerserver.Tuner{tuner(1, false, false), tuner(2, false, true)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := edcb.NewTunerServer(tunerServer(t).client(t), &edcb.TunerServerOptions{BonDrivers: tt.bonDrivers})
			tuners, err := server.Tuners(t.Context())
			require.NoError(t, err)
			assert.Equal(t, tt.want, tuners)
		})
	}
}

func TestTunerServerTunersWhenFailed(t *testing.T) {
	t.Parallel()

	// どの問い合わせが失敗しても、空の一覧ではなくエラーを返す。空の一覧を記録すると、チューナーがないことになってしまう。
	for _, command := range []uint32{1016, 1066, 1060} {
		t.Run(strconv.FormatUint(uint64(command), 10), func(t *testing.T) {
			t.Parallel()

			s := tunerServer(t)
			s.queued[command] = []reply{{status: 208, payload: nil, stream: nil, hold: false, drop: false}}
			_, err := edcb.NewTunerServer(s.client(t), nil).Tuners(t.Context())
			require.ErrorIs(t, err, edcb.ErrBusy)
		})
	}
}

func TestTunerServerClockAndIdentity(t *testing.T) {
	t.Parallel()

	s := newFakeServer(t, map[uint32]reply{})
	server := edcb.NewTunerServer(s.client(t), nil)
	date, err := server.ServerDate(t.Context())
	require.ErrorIs(t, err, tunerserver.ErrClockUnavailable)
	assert.Equal(t, time.Time{}, date)
	identity, err := server.Identify(t.Context())
	require.NoError(t, err)
	assert.Equal(t, &tunerserver.Identity{Kind: tunerserver.KindEdcb, Version: ""}, identity)
	// どちらも EpgTimerSrv に問い合わせない。
	assert.Empty(t, s.received())
}

func TestTunerServerChannels(t *testing.T) {
	t.Parallel()

	// チャンネルの設定のサービスを、TS (TSID) ごとにまとめる。
	s := edcbServer(t)
	server := edcb.NewTunerServer(s.client(t), nil)
	channels, err := server.Channels(t.Context())
	require.NoError(t, err)
	services, err := server.Services(t.Context())
	require.NoError(t, err)
	total := 0
	for _, channel := range channels {
		assert.Equal(t, "GR", channel.Type)
		assert.NotEmpty(t, channel.Name)
		for _, service := range channel.Services {
			assert.Equal(t, tunerserver.ChannelRef{Type: channel.Type, Channel: channel.Channel}, *service.Channel)
		}
		total += len(channel.Services)
	}
	assert.Equal(t, len(services), total)
	// NHK 総合 (東京) の TS には、NHK 総合 1 のサービスが入る。
	ids := map[string][]int64{}
	for _, channel := range channels {
		for _, service := range channel.Services {
			ids[channel.Channel] = append(ids[channel.Channel], service.ID)
		}
	}
	assert.Contains(t, ids["32736"], int64(3273601024))
	assert.NotContains(t, ids["32736"], int64(3273701032))
}

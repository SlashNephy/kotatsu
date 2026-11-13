package edcb_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/infra/edcb"
)

func TestParseLogoMap(t *testing.T) {
	t.Parallel()

	data := []byte("\xEF\xBB\xBF[Other]\r\n7FE00400=9\r\n[LogoIDMap]\r\n7FE00400=0\r\n7fe10408 = 8\r\nbroken=1\r\n7FE00401=x\r\n")
	assert.Equal(t, map[edcb.ServiceKey]int{
		{ONID: 0x7FE0, TSID: 0, SID: 0x0400}: 0,
		{ONID: 0x7FE1, TSID: 0, SID: 0x0408}: 8,
	}, edcb.ParseLogoMap(data))
}

// logoMapFile は、EDCB がサービスとロゴの ID の対応を書くファイル。
const logoMapFile = "LogoData.ini"

// files は、FILE_COPY2 の応答 (版と FILE_DATA の配列) を組む。
func files(items ...[2]string) reply {
	list := make([]payload, 0, len(items))
	for _, item := range items {
		list = append(list, sized(payload{}.str(item[0]).u32(dword(len(item[1]))).u32(0).append(payload(item[1]))))
	}

	return reply{status: 1, payload: payload{}.u16(5).append(vector(list...)), stream: nil, hold: false, drop: false}
}

func TestTunerServerServiceLogo(t *testing.T) {
	t.Parallel()

	logoMap := [2]string{logoMapFile, "[LogoIDMap]\r\n7FE00400=3\r\n"}
	tests := []struct {
		name    string
		replies []reply
		id      int64
		want    []byte
		wantErr error
	}{
		{
			// 種類 2 と 0 しかなければ、望ましい順で先の 2 を返す。
			name: "preferred type",
			replies: []reply{files(logoMap), files(
				[2]string{`LogoData\7fe0_003_000_05.png`, ""}, [2]string{`LogoData\7fe0_003_000_02.png`, "png2"},
				[2]string{`LogoData\7fe0_003_000_04.png`, ""}, [2]string{`LogoData\7fe0_003_000_01.png`, ""},
				[2]string{`LogoData\7fe0_003_000_03.png`, ""}, [2]string{`LogoData\7fe0_003_000_00.png`, "png0"},
			)},
			id: 3273601024, want: []byte("png2"), wantErr: nil,
		},
		{name: "unknown service", replies: []reply{files(logoMap)}, id: 3273601025, want: nil, wantErr: tunerserver.ErrLogoUnavailable},
		// EDCB がロゴを集めていなければ、LogoData.ini は空になる。
		{name: "no logo map", replies: []reply{files([2]string{logoMapFile, ""})}, id: 3273601024, want: nil, wantErr: tunerserver.ErrLogoUnavailable},
		{name: "no image", replies: []reply{files(logoMap), files()}, id: 3273601024, want: nil, wantErr: tunerserver.ErrLogoUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := newFakeServer(t, map[uint32]reply{})
			s.queued[2060] = tt.replies
			got, err := edcb.NewTunerServer(s.client(t), nil).ServiceLogo(t.Context(), tt.id)
			require.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTunerServerServiceLogoCachesMap(t *testing.T) {
	t.Parallel()

	// ロゴの ID の対応は、読み直す間隔の間、読み直さない。
	s := newFakeServer(t, map[uint32]reply{})
	s.queued[2060] = []reply{files([2]string{logoMapFile, ""})}
	server := edcb.NewTunerServer(s.client(t), nil)
	for range 3 {
		_, err := server.ServiceLogo(t.Context(), 3273601024)
		require.ErrorIs(t, err, tunerserver.ErrLogoUnavailable)
	}
	assert.Equal(t, []uint32{2060}, commands(s.received()))
}

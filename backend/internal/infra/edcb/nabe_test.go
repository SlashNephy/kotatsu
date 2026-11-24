package edcb_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/infra/registry"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/nabe"
	"github.com/SlashNephy/kotatsu/internal/web/mirakurunapi"
)

// nabeKeyMaterial は、テストのアプリの秘密キー。
const nabeKeyMaterial = "a-key-material-that-is-long-enough-for-tests"

// unusedOpener は、nabe から受け取れずに自分で開こうとしたら失敗させる。
type unusedOpener struct{}

var errLocalOpen = errors.New("opened the NetworkTV without nabe")

func (unusedOpener) OpenServiceStream(context.Context, int64, *tunerserver.StreamOptions) (io.ReadCloser, error) {
	return nil, errLocalOpen
}

func (unusedOpener) OpenChannelStream(context.Context, string, string, *tunerserver.StreamOptions) (io.ReadCloser, error) {
	return nil, errLocalOpen
}

// nabeInstances は、1 台の nabe を返す登録簿。
type nabeInstances struct {
	address string
}

func (i nabeInstances) ListAlive(context.Context, string) ([]*registry.Instance, error) {
	return []*registry.Instance{{ID: uuid.Nil(), Role: nabe.Role, Address: i.address, StartedAt: time.Time{}, HeartbeatAt: time.Time{}}}, nil
}

// TestNabeSharesNetworkTVAcrossRoles は、ほかのロール (別のプロセス) が nabe から受け取るとき、
// 同じサービスの受け手が何人いても、nabe が NetworkTV を 1 本だけ開くことを確認する (#1535)。
func TestNabeSharesNetworkTVAcrossRoles(t *testing.T) {
	t.Parallel()

	s, server, ids, feed := feedServer(t, 0, 0)
	token, err := nabe.NewToken(nabeKeyMaterial)
	require.NoError(t, err)
	hub := httptest.NewServer(mirakurunapi.NabeHandler(server, token, "test", slog.New(slog.DiscardHandler)))
	t.Cleanup(hub.Close)
	instances := nabeInstances{address: strings.TrimPrefix(hub.URL, "http://")}

	// 録画の mikan とライブ視聴の tenban を、別々の Streams で表す。
	streams := make([]io.ReadCloser, 0, 2)
	for range 2 {
		roleStreams := nabe.NewStreams(instances, token, unusedOpener{}, new(clock.Real), slog.New(slog.DiscardHandler))
		stream, err := roleStreams.OpenServiceStream(t.Context(), nhkSougou, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = stream.Close() })
		streams = append(streams, stream)
	}
	packets := append(patPacket(32736, 1024), dataPacket()...)
	feed <- packets
	for _, stream := range streams {
		assert.Equal(t, packets, readPackets(t, stream, 2))
	}
	assert.Equal(t, 1, countCommand(s, 1073))

	// EDCB の予約にチューナーを取られたら、どちらの受け手も、チューナーを取られたと分かる。
	feed <- append(patPacket(0x4010, 101), dataPacket()...)
	for _, stream := range streams {
		_, err := io.ReadAll(stream)
		require.ErrorIs(t, err, tunerserver.ErrTunerPreempted)
	}
	assert.Equal(t, 1, countCommand(s, 1074))
	_, released := ids.state()
	assert.Equal(t, []int32{1000}, released)
}

// TestNabeClosesNetworkTVWhenRolesLeave は、ほかのロールの受け手が全員去ったら、nabe が NetworkTV を閉じることを確認する。
func TestNabeClosesNetworkTVWhenRolesLeave(t *testing.T) {
	t.Parallel()

	s, server, ids, feed := feedServer(t, 0, 0)
	token, err := nabe.NewToken(nabeKeyMaterial)
	require.NoError(t, err)
	hub := httptest.NewServer(mirakurunapi.NabeHandler(server, token, "test", slog.New(slog.DiscardHandler)))
	t.Cleanup(hub.Close)
	roleStreams := nabe.NewStreams(nabeInstances{address: strings.TrimPrefix(hub.URL, "http://")}, token, unusedOpener{},
		new(clock.Real), slog.New(slog.DiscardHandler))
	streams := make([]io.ReadCloser, 0, 2)
	for range 2 {
		stream, err := roleStreams.OpenServiceStream(t.Context(), nhkSougou, nil)
		require.NoError(t, err)
		streams = append(streams, stream)
	}
	feed <- patPacket(32736, 1024)
	for _, stream := range streams {
		assert.Equal(t, patPacket(32736, 1024), readPackets(t, stream, 1))
	}

	require.NoError(t, streams[0].Close())
	feed <- dataPacket()
	assert.Equal(t, dataPacket(), readPackets(t, streams[1], 1))
	assert.Zero(t, countCommand(s, 1074))
	require.NoError(t, streams[1].Close())
	// HTTP の接続が切れたことに nabe が気づくのは、少し後になる。
	require.Eventually(t, func() bool {
		_, released := ids.state()

		return slices.Equal(released, []int32{1000})
	}, testWait, testTick)
	assert.Equal(t, 1, countCommand(s, 1074))
}

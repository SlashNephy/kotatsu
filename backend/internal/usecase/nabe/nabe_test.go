package nabe_test

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/infra/registry"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/nabe"
)

// keyMaterial は、テストのアプリの秘密キー。
const keyMaterial = "a-key-material-that-is-long-enough-for-tests"

// fromRemote と fromLocal は、nabe から受け取ったか、自分で開いたかを見分ける本文。
const (
	fromRemote = "remote"
	fromLocal  = "local"
)

// fakeInstances は、決めた nabe のインスタンスを返す登録簿。
type fakeInstances struct {
	instances []*registry.Instance
	err       error
}

func (f *fakeInstances) ListAlive(_ context.Context, role string) ([]*registry.Instance, error) {
	if role != nabe.Role {
		return nil, errors.New("unexpected role " + role)
	}

	return f.instances, f.err
}

// fakeOpener は、開いたストリームを数え、決めた本文か、決めたエラーを返す。
type fakeOpener struct {
	tunerserver.Client

	body []byte
	err  error

	mu     sync.Mutex
	opened int
}

func (f *fakeOpener) OpenServiceStream(context.Context, int64, *tunerserver.StreamOptions) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opened++
	if f.err != nil {
		return nil, f.err
	}

	return io.NopCloser(bytes.NewReader(f.body)), nil
}

func (f *fakeOpener) OpenChannelStream(ctx context.Context, _, _ string, opts *tunerserver.StreamOptions) (io.ReadCloser, error) {
	return f.OpenServiceStream(ctx, 0, opts)
}

func (f *fakeOpener) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.opened
}

// startNabe は、remote を nabe として公開し、そのアドレスを返す。secret は nabe のアプリの秘密キー。
func startNabe(t *testing.T, remote tunerserver.Client, secret string) string {
	t.Helper()

	token, err := nabe.NewToken(secret)
	require.NoError(t, err)
	server := httptest.NewServer(nabe.Handler(remote, token, "test", slog.New(slog.DiscardHandler)))
	t.Cleanup(server.Close)

	return strings.TrimPrefix(server.URL, "http://")
}

// closedAddress は、待ち受けていないアドレスを返す。
func closedAddress(t *testing.T) string {
	t.Helper()

	listener, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())

	return address
}

func TestStreams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// nabe は、nabe がいるかどうか。unreachable なら、登録簿にはいるが届かない。nabeKey は nabe のアプリの秘密キー。
		nabe        bool
		unreachable bool
		// address は、登録簿に載せるアドレス。空なら nabe のアドレスにする。
		address   string
		nabeKey   string
		listErr   error
		remoteErr error
		// want は、受け取る本文。wantErr なら、そのエラーで開けない。
		want       string
		wantErr    error
		wantRemote int
		wantLocal  int
	}{
		{name: "from nabe", nabe: true, want: fromRemote, wantRemote: 1, wantLocal: 0},
		{name: "no nabe", nabe: false, want: fromLocal, wantRemote: 0, wantLocal: 1},
		// 登録簿を引けなければ、録画を止めないよう、自分で開く。
		{name: "registry unavailable", nabe: true, listErr: errors.New("database is down"), want: fromLocal, wantRemote: 0, wantLocal: 1},
		{name: "nabe unreachable", nabe: true, unreachable: true, want: fromLocal, wantRemote: 0, wantLocal: 1},
		{name: "invalid address", nabe: true, address: "[::1", want: fromLocal, wantRemote: 0, wantLocal: 1},
		// 秘密キーが違って断られても、自分で開く。
		{name: "wrong secret", nabe: true, nabeKey: keyMaterial + "-other", want: fromLocal, wantRemote: 0, wantLocal: 1},
		// チューナーが足りないと断られたら、自分で開いても同じなので、そのまま返す。
		{name: "no tuner on nabe", nabe: true, remoteErr: tunerserver.ErrTunerUnavailable, wantErr: tunerserver.ErrTunerUnavailable, wantRemote: 1, wantLocal: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			remote := &fakeOpener{body: []byte(fromRemote), err: tt.remoteErr}
			local := &fakeOpener{body: []byte(fromLocal)}
			instances := &fakeInstances{instances: []*registry.Instance{}, err: tt.listErr}
			if tt.nabe {
				address := closedAddress(t)
				if !tt.unreachable {
					address = startNabe(t, remote, cmp.Or(tt.nabeKey, keyMaterial))
				}
				address = cmp.Or(tt.address, address)
				instances.instances = append(instances.instances, &registry.Instance{
					ID: uuid.New(), Role: nabe.Role, Address: address, StartedAt: time.Time{}, HeartbeatAt: time.Time{},
				})
			}

			token, err := nabe.NewToken(keyMaterial)
			require.NoError(t, err)
			streams := nabe.NewStreams(instances, token, local, new(clock.Real), slog.New(slog.DiscardHandler))

			for _, open := range []func() (io.ReadCloser, error){
				func() (io.ReadCloser, error) { return streams.OpenServiceStream(t.Context(), 3273601024, nil) },
				func() (io.ReadCloser, error) { return streams.OpenChannelStream(t.Context(), "GR", "32736", nil) },
			} {
				stream, err := open()
				if tt.wantErr != nil {
					require.ErrorIs(t, err, tt.wantErr)

					continue
				}
				require.NoError(t, err)
				got, err := io.ReadAll(stream)
				require.NoError(t, err)
				require.NoError(t, stream.Close())
				assert.Equal(t, tt.want, string(got))
			}
			assert.Equal(t, 2*tt.wantRemote, remote.count())
			assert.Equal(t, 2*tt.wantLocal, local.count())
		})
	}
}

func TestStreamsPicksSameNabeForService(t *testing.T) {
	t.Parallel()

	// nabe が何台いても、同じサービスは同じ nabe から受け取る。
	remotes := []*fakeOpener{{body: []byte("a")}, {body: []byte("b")}, {body: []byte("c")}}
	instances := &fakeInstances{instances: []*registry.Instance{}, err: nil}
	for _, remote := range remotes {
		instances.instances = append(instances.instances, &registry.Instance{
			ID: uuid.New(), Role: nabe.Role, Address: startNabe(t, remote, keyMaterial), StartedAt: time.Time{}, HeartbeatAt: time.Time{},
		})
	}
	token, err := nabe.NewToken(keyMaterial)
	require.NoError(t, err)
	streams := nabe.NewStreams(instances, token, &fakeOpener{body: []byte(fromLocal)}, new(clock.Real), slog.New(slog.DiscardHandler))
	var first string
	for i := range 5 {
		// 登録簿の並びが変わっても、同じ nabe を選ぶ。
		instances.instances = append(instances.instances[1:], instances.instances[0])
		stream, err := streams.OpenServiceStream(t.Context(), 3273601024, nil)
		require.NoError(t, err)
		got, err := io.ReadAll(stream)
		require.NoError(t, err)
		require.NoError(t, stream.Close())
		if i == 0 {
			first = string(got)
		}
		assert.Equal(t, first, string(got))
	}
}

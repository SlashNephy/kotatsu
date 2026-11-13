package edcb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
)

const (
	// relayRetryInterval と relayRetryTimeout は、NetworkTV を開いてから TS を受け取れるようになるまで、
	// RELAY_VIEW_STREAM をやり直す間隔と上限。開いた直後は、TS を送る FIFO (Windows では名前付きパイプ) ができていない。
	// 2026年11月に Linux の EDCB で測ったときは、0.2 秒で受け取れた。
	relayRetryInterval = 100 * time.Millisecond
	relayRetryTimeout  = 10 * time.Second
	// closeTimeout は、ストリームを閉じるときに、NWTV_ID_CLOSE と ID の返却に掛ける時間の上限。
	closeTimeout = 10 * time.Second
)

// ErrNoNetworkTVID は、設定した範囲の NetworkTV の ID が、すべて使われていることを表す。
var ErrNoNetworkTVID = errors.New("no NetworkTV ID is free")

// NetworkTVIDs は、NetworkTV の ID を配る (#1533)。ID は EpgTimerSrv 全体で 1 つの名前空間なので、
// Kotatsu のどのインスタンスとも重ならないように、DB で持ち主を記録する。
type NetworkTVIDs interface {
	// Acquire は、空いている ID を取る。空いていなければ ErrNoNetworkTVID を返す。
	Acquire(ctx context.Context, service ServiceKey) (int32, error)
	// Release は、取った ID を返す。
	Release(ctx context.Context, id int32) error
}

// OpenServiceStream は、サービスのストリームを、NetworkTV で開く (#1533、設計書 5.1)。
//   - NetworkTV の ID を取り、NWTV_ID_SET_CH で開き、TS を受け取れるようになるまで RELAY_VIEW_STREAM をやり直す。
//   - 返すストリームを閉じたら、NWTV_ID_CLOSE を送り、ID を返す。EDCB は、中継の接続が切れても NetworkTV を閉じない。
//   - EDCB が送る TS は、サービスで絞ったものになる。スクランブルを解くかどうかは、EDCB の BonDriver と設定で決まるので、
//     opts.Decode は使わない。優先度もないので、opts.Priority も使わない。
//   - サービスがチャンネルの設定 (ChSet5.txt) になければ tunerserver.ErrNotFound を、ID が足りないか、
//     EDCB がチューナーを開けなければ tunerserver.ErrTunerUnavailable を包んで返す。
func (s *TunerServer) OpenServiceStream(ctx context.Context, id int64, _ *tunerserver.StreamOptions) (io.ReadCloser, error) {
	entries, err := s.channelEntries(ctx)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if serviceID(entry.ServiceKey) == id {
			return s.openNetworkTV(ctx, entry.ServiceKey)
		}
	}

	return nil, fmt.Errorf("%w: service %d", tunerserver.ErrNotFound, id)
}

// OpenChannelStream は、TSID が channel の TS のサービスのうち、チャンネルの設定で最初のものを開く (#1533)。
// EDCB の NetworkTV はサービスで絞った TS しか送らないので、ほかのサービスは含まない。
func (s *TunerServer) OpenChannelStream(ctx context.Context, channelType, channel string, _ *tunerserver.StreamOptions) (io.ReadCloser, error) {
	entries, err := s.channelEntries(ctx)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if ChannelType(entry.ONID) == channelType && strconv.Itoa(int(entry.TSID)) == channel {
			return s.openNetworkTV(ctx, entry.ServiceKey)
		}
	}

	return nil, fmt.Errorf("%w: channel %s %s", tunerserver.ErrNotFound, channelType, channel)
}

func (s *TunerServer) openNetworkTV(ctx context.Context, service ServiceKey) (io.ReadCloser, error) {
	if s.ids == nil {
		return nil, errors.New("NetworkTV IDs are not configured")
	}
	id, err := s.ids.Acquire(ctx, service)
	if errors.Is(err, ErrNoNetworkTVID) {
		return nil, fmt.Errorf("%w: %w", tunerserver.ErrTunerUnavailable, err)
	}
	if err != nil {
		return nil, err
	}
	stream := &networkTVStream{ReadCloser: nil, server: s, id: id, once: sync.Once{}, closeErr: nil}
	processID, err := s.client.OpenNetworkTV(ctx, id, service)
	if err != nil {
		_ = stream.release(context.WithoutCancel(ctx))
		if errors.Is(err, ErrFailed) {
			// EDCB は、使えるチューナーがないときも、サービスを受信できないときも、同じ失敗を返す。
			return nil, fmt.Errorf("%w: %w", tunerserver.ErrTunerUnavailable, err)
		}

		return nil, err
	}
	relay, err := s.relay(ctx, processID)
	if err != nil {
		_ = stream.release(context.WithoutCancel(ctx))

		return nil, err
	}
	stream.ReadCloser = relay

	return stream, nil
}

// relay は、TS を受け取れるようになるまで RELAY_VIEW_STREAM をやり直す。返すストリームは、ctx が終わると閉じる。
func (s *TunerServer) relay(ctx context.Context, processID int32) (io.ReadCloser, error) {
	deadline := time.NewTimer(relayRetryTimeout)
	defer deadline.Stop()
	for {
		relay, err := s.client.RelayStream(ctx, processID)
		if err == nil {
			return relay, nil
		}
		if !errors.Is(err, ErrFailed) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return nil, fmt.Errorf("the NetworkTV stream did not become ready: %w", err)
		case <-time.After(relayRetryInterval):
		}
	}
}

// networkTVStream は、NetworkTV の TS を受け取っている接続。閉じたら、NetworkTV を閉じて ID を返す。
type networkTVStream struct {
	io.ReadCloser

	server   *TunerServer
	id       int32
	once     sync.Once
	closeErr error
}

var _ io.ReadCloser = new(networkTVStream)

// Close は、中継の接続を切り、NetworkTV を閉じて ID を返す。何度呼んでもよい。
func (n *networkTVStream) Close() error {
	n.once.Do(func() {
		var errs []error
		if n.ReadCloser != nil {
			errs = append(errs, n.ReadCloser.Close())
		}
		n.closeErr = errors.Join(append(errs, n.release(context.Background()))...)
	})

	return n.closeErr
}

// release は、NetworkTV を閉じて ID を返す。閉じられなくても ID は返す。開いている ID に NWTV_ID_SET_CH を送れば、
// EDCB は選局し直すか開き直すので、次に使う人が困らない。返さないと、このインスタンスが動いている間、ID が減ったままになる。
// ctx が止められていても閉じられるよう、呼び出し側は止められない ctx を渡す。
func (n *networkTVStream) release(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, closeTimeout)
	defer cancel()
	var errs []error
	if err := n.server.client.CloseNetworkTV(ctx, n.id); err != nil {
		errs = append(errs, fmt.Errorf("failed to close NetworkTV %d: %w", n.id, err))
	}
	if err := n.server.ids.Release(ctx, n.id); err != nil {
		errs = append(errs, fmt.Errorf("failed to release NetworkTV ID %d: %w", n.id, err))
	}

	return errors.Join(errs...)
}

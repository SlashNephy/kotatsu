package edcb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

const (
	// relayRetryInterval と relayRetryTimeout は、NetworkTV を開いてから TS を受け取れるようになるまで、
	// RELAY_VIEW_STREAM をやり直す間隔と上限。開いた直後は、TS を送る FIFO (Windows では名前付きパイプ) ができていない。
	// 2026年11月に Linux の EDCB で計測したときは、0.2 秒で受け取れた。
	relayRetryInterval = 100 * time.Millisecond
	relayRetryTimeout  = 10 * time.Second
	// sharedRelayTimeout は、ほかのインスタンスが開いている NetworkTV に加わったときに、RELAY_VIEW_STREAM をやり直す上限 (#1850)。
	// TS を送る FIFO はできているはずなので、これを過ぎても受け取れなければ、中継の空きがない (EDCB は 3 つ目の中継を断る) とみなす。
	// 開いた直後に加わったときに、FIFO ができるのを待てるよう、計測した時間 (0.2 秒) より長くする。
	sharedRelayTimeout = 2 * time.Second
	// DefaultReleaseRetryInterval は、ID の持ち主から外れられなかったときに、外れ直す既定の間隔 (#1850)。
	DefaultReleaseRetryInterval = 10 * time.Second
	// DefaultHandoverGrace は、止まるインスタンスが最後の持ち主だった NetworkTV を、ほかのインスタンスが引き継ぐのを待つ既定の時間 (#1850)。
	// 受け手は、切れてから 2 秒 (録画の RetryInterval) ほどで、ほかの nabe につなぎ直す。加わらなければ閉じる。
	DefaultHandoverGrace = 5 * time.Second
	// closeTimeout は、ストリームを閉じるときに、NWTV_ID_CLOSE と ID の返却に掛ける時間の上限。
	closeTimeout = 10 * time.Second
	// defaultPreemptionCheckInterval は、NetworkTV がまだ開いているかを問い合わせる既定の間隔 (#1534)。
	// EDCB の予約にチューナーを取られると NetworkTV は閉じるので、TS の PAT が変わるより先に気づけることがある。
	defaultPreemptionCheckInterval = 5 * time.Second
)

// ErrNoNetworkTVID は、設定した範囲の NetworkTV の ID が、すべて使われていることを表す。
var ErrNoNetworkTVID = errors.New("no NetworkTV ID is free")

// errRelayBusy は、ほかのインスタンスが開いている NetworkTV に加わったのに、中継に空きがなかったことを表す (#1850)。
var errRelayBusy = errors.New("the shared NetworkTV has no free relay")

// NetworkTVIDs は、NetworkTV の ID を配信する (#1533)。ID は EpgTimerSrv 全体で 1 つの名前空間なので、
// Kotatsu のどのインスタンスとも重ならないように、DB で持ち主を記録する。
// 同じサービスを開くインスタンスは、2 つまで同じ ID を持ち、同じ TS を受け取る (#1850)。
type NetworkTVIDs interface {
	// Acquire は、service の ID を取る。exclusive でなく、ほかのインスタンスが同じサービスの ID を持っていて、持ち主に空きがあれば、
	// その ID に加わり、shared に true を返す。加われなければ、空いている ID を取る。空いていなければ ErrNoNetworkTVID を返す。
	Acquire(ctx context.Context, service ServiceKey, exclusive bool) (id int32, shared bool, err error)
	// Release は、ID の持ち主から外れ、ほかに生きている持ち主がいなければ last に true を返す。
	// keep でなければ、そのときに閉じ始めたことを記録するので、呼び出し側は NWTV_ID_CLOSE を送ってから Forget する。
	// keep なら、ほかのインスタンスが加われるよう残すので、呼び出し側は待ってから CloseIfUnheld を呼ぶ。
	Release(ctx context.Context, id int32, keep bool) (last bool, err error)
	// CloseIfUnheld は、keep で残した ID に、生きている持ち主がいなければ閉じ始めたことを記録し、true を返す。
	CloseIfUnheld(ctx context.Context, id int32) (bool, error)
	// Forget は、閉じた ID を消す。
	Forget(ctx context.Context, id int32) error
}

// OpenServiceStream は、サービスのストリームを、NetworkTV で開く (#1533、設計書 5.1)。
//   - NetworkTV の ID を取り、NWTV_ID_SET_CH で開き、TS を受け取れるようになるまで RELAY_VIEW_STREAM をやり直す。
//   - 返すストリームを閉じたら、NWTV_ID_CLOSE を送り、ID を返す。EDCB は、中継の接続が切れても NetworkTV を閉じない。
//   - ほかのインスタンスが同じサービスを開いていれば、その ID に加わり、同じ TS を受け取る。最後の持ち主だけが閉じる (#1850)。
//   - EDCB が送る TS は、サービスで絞ったものになる。スクランブルを解くかどうかは、EDCB の BonDriver と設定で決まるので、
//     opts.Decode は使わない。優先度もないので、opts.Priority も使わない。
//   - サービスがチャンネルの設定 (ChSet5.txt) になければ tunerserver.ErrNotFound を、ID が足りないか、
//     EDCB がチューナーを開けなければ tunerserver.ErrTunerUnavailable を包んで返す。
//   - 同じサービスをすでに開いていれば、新しく開かずに、その TS を分け合う (#1535)。最後の受け手が閉じたら、NetworkTV を閉じる。
//     読むのが遅く、バッファできる上限を超えた受け手は、ErrSubscriberTooSlow で切る。
func (s *TunerServer) OpenServiceStream(ctx context.Context, id int64, opts *tunerserver.StreamOptions) (io.ReadCloser, error) {
	entries, err := s.channelEntries(ctx)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if serviceID(entry.ServiceKey) == id {
			return s.openShared(ctx, entry.ServiceKey, opts)
		}
	}

	return nil, fmt.Errorf("%w: service %d", tunerserver.ErrNotFound, id)
}

// OpenChannelStream は、TSID が channel の TS のサービスのうち、チャンネルの設定で最初のものを開く (#1533)。
// EDCB の NetworkTV はサービスで絞った TS しか送らないので、ほかのサービスは含まない。そのサービスのストリームと分け合う (#1535)。
func (s *TunerServer) OpenChannelStream(ctx context.Context, channelType, channel string, opts *tunerserver.StreamOptions) (io.ReadCloser, error) {
	entries, err := s.channelEntries(ctx)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if ChannelType(entry.ONID) == channelType && strconv.Itoa(int(entry.TSID)) == channel {
			return s.openShared(ctx, entry.ServiceKey, opts)
		}
	}

	return nil, fmt.Errorf("%w: channel %s %s", tunerserver.ErrNotFound, channelType, channel)
}

// openShared は、service のストリームを、分け合う NetworkTV から受け取る。
// ライブ視聴、kyusu、タイムシフトが新しく NetworkTV を開くときは、EPG の取得のためにチューナーを空けておくかを先に確認する (#1536)。
// すでに開いている NetworkTV に加わるだけなら、チューナーを増やさないので確認しない。
func (s *TunerServer) openShared(ctx context.Context, service ServiceKey, opts *tunerserver.StreamOptions) (io.ReadCloser, error) {
	if viewing(opts) && !s.share.active(service) && s.keepsTunerForEPG(ctx) {
		return nil, errEPGCapture
	}

	return s.share.open(ctx, service)
}

// openNetworkTV は、service の NetworkTV を開く。ほかのインスタンスが開いている NetworkTV に加わったのに、中継に空きがなければ
// (止まったはずの持ち主の中継が切れていないなど)、自分で別の ID を取って開き直す (#1850)。
func (s *TunerServer) openNetworkTV(ctx context.Context, service ServiceKey) (io.ReadCloser, error) {
	if s.ids == nil {
		return nil, errors.New("NetworkTV IDs are not configured")
	}
	stream, err := s.openNetworkTVID(ctx, service, false)
	if errors.Is(err, errRelayBusy) {
		return s.openNetworkTVID(ctx, service, true)
	}

	return stream, err
}

func (s *TunerServer) openNetworkTVID(ctx context.Context, service ServiceKey, exclusive bool) (io.ReadCloser, error) {
	id, shared, err := s.ids.Acquire(ctx, service, exclusive)
	if errors.Is(err, ErrNoNetworkTVID) {
		return nil, fmt.Errorf("%w: %w", tunerserver.ErrTunerUnavailable, err)
	}
	if err != nil {
		return nil, err
	}
	stream := &networkTVStream{
		ReadCloser: nil, server: s, id: id, once: sync.Once{}, closeErr: nil,
		pat: nil, preempted: atomic.Bool{}, stopCheck: func() {}, checkDone: nil,
	}
	// 開いている ID に、同じサービスで NWTV_ID_SET_CH を送ると、EDCB は選局し直さずに、同じプロセスを返す。
	processID, err := s.client.OpenNetworkTV(ctx, id, service)
	if err != nil {
		_ = stream.release(context.WithoutCancel(ctx), false)
		if errors.Is(err, ErrFailed) {
			// EDCB は、使えるチューナーがないときも、サービスを受信できないときも、同じ失敗を返す。
			return nil, fmt.Errorf("%w: %w", tunerserver.ErrTunerUnavailable, err)
		}

		return nil, err
	}
	timeout := relayRetryTimeout
	if shared {
		timeout = sharedRelayTimeout
	}
	relay, err := s.relay(ctx, processID, timeout)
	if err != nil {
		_ = stream.release(context.WithoutCancel(ctx), false)
		if shared && errors.Is(err, ErrFailed) {
			return nil, fmt.Errorf("%w: NetworkTV %d: %w", errRelayBusy, id, err)
		}

		return nil, err
	}
	stream.ReadCloser = relay
	stream.watch(ctx, service, processID)
	s.streams.add()

	return stream, nil
}

// relay は、TS を受け取れるようになるまで RELAY_VIEW_STREAM をやり直す。返すストリームは、ctx が終わると閉じる。
func (s *TunerServer) relay(ctx context.Context, processID int32, timeout time.Duration) (io.ReadCloser, error) {
	deadline := time.NewTimer(timeout)
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

// stopping は、プロセスが止まり始めたかを返す。
func (s *TunerServer) stopping() bool {
	if s.stopped == nil {
		return false
	}
	select {
	case <-s.stopped:
		return true
	default:
		return false
	}
}

// WaitClosed は、開いた NetworkTV のストリームが、すべて閉じ終えるまで待つ (#1850)。ctx が終わったら待つのをやめ、ctx のエラーを返す。
// プロセスが止まるときは、最後の持ち主だった NetworkTV を、ほかのインスタンスが引き継ぐのを待ってから閉じるので、
// その前にプロセスが終わって、NetworkTV を開いたまま残さないよう、終わる前に呼ぶ。
func (s *TunerServer) WaitClosed(ctx context.Context) error {
	return s.streams.wait(ctx)
}

// openStreams は、開いていて、まだ閉じ終えていない NetworkTV のストリームを数える。
type openStreams struct {
	mu    sync.Mutex
	count int
	// idle は、count が 0 になったら閉じる。
	idle chan struct{}
}

func newOpenStreams() *openStreams {
	idle := make(chan struct{})
	close(idle)

	return &openStreams{mu: sync.Mutex{}, count: 0, idle: idle}
}

func (o *openStreams) add() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.count == 0 {
		o.idle = make(chan struct{})
	}
	o.count++
}

func (o *openStreams) done() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.count--
	if o.count == 0 {
		close(o.idle)
	}
}

func (o *openStreams) wait(ctx context.Context) error {
	o.mu.Lock()
	idle := o.idle
	o.mu.Unlock()
	select {
	case <-idle:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// networkTVStream は、NetworkTV の TS を受け取っている接続。閉じたら、NetworkTV を閉じて ID を返す。
type networkTVStream struct {
	io.ReadCloser

	server   *TunerServer
	id       int32
	once     sync.Once
	closeErr error

	// pat は、受け取った TS の PAT を見張る。TSID かサービスが、開いたときから変わったら、preempted を立てる (#1534)。
	pat *ts.PATWatcher
	// preempted は、EDCB の予約にチューナーを取られたと分かったかどうか。立ったら、Read は ErrTunerPreempted を返す。
	preempted atomic.Bool
	// stopCheck と checkDone は、NetworkTV がまだ開いているかを問い合わせる goroutine を止めて待つ。
	stopCheck func()
	checkDone chan struct{}
}

var _ io.ReadCloser = new(networkTVStream)

// Read は、TS を読む。EDCB の予約にチューナーを取られたと分かったら、tunerserver.ErrTunerPreempted を返す。
// 取られた後に届いた別の局の TS は返さない。1 回に読んだ中で局が変わったときは、変わったと分かったパケットより前の分を返す。
// 分け合うストリーム (#1535) は大きく読むので、読んだ分をまとめて破棄すると、自分の局の TS まで落とすためである。
func (n *networkTVStream) Read(p []byte) (int, error) {
	if n.preempted.Load() {
		return 0, n.preemption()
	}
	read, err := n.ReadCloser.Read(p)
	// 問い合わせで取られたと分かると、中継の接続を切るので、読むのは接続を切ったエラーで終わる。
	if err != nil && n.preempted.Load() {
		return 0, n.preemption()
	}
	if n.pat == nil {
		return read, err
	}
	for off := 0; off < read; off += tsPacketSize {
		_, _ = n.pat.Write(p[off:min(off+tsPacketSize, read)])
		if n.preempted.Load() {
			if off > 0 {
				return off, nil
			}

			return 0, n.preemption()
		}
	}

	return read, err
}

// Close は、中継の接続を切り、NetworkTV を閉じて ID を返す。何度呼んでもよい。
// ほかのインスタンスも同じ ID を持っていれば、閉じずに持ち主から外れる。プロセスが止まるときは、ほかのインスタンスが
// 引き継げるよう、最後の持ち主でも HandoverGrace だけ閉じずに待つ (#1850)。
func (n *networkTVStream) Close() error {
	n.once.Do(func() {
		n.stopCheck()
		if n.checkDone != nil {
			<-n.checkDone
		}
		var errs []error
		if n.ReadCloser != nil {
			errs = append(errs, n.ReadCloser.Close())
		}
		n.closeErr = errors.Join(append(errs, n.release(context.Background(), n.server.stopping()))...)
		n.server.streams.done()
	})

	return n.closeErr
}

// release は、ID の持ち主から外れ、最後の持ち主なら NetworkTV を閉じて ID を消す。閉じられなくても ID は消す。
// 開いている ID に NWTV_ID_SET_CH を送れば、EDCB は選局し直すか開き直すので、次に使う人が困らない。
// 消さないと、このインスタンスが動いている間、ID が減ったままになる。
// 持ち主から外れられなかったとき (DB に届かないなど) は、ほかの持ち主の TS を切らないよう閉じず、裏で外れ直す (retryRelease)。
// keep なら、最後の持ち主でも、ほかのインスタンスが加わるのを HandoverGrace だけ待ち、加わらなければ閉じる。
// ctx が止められていても閉じられるよう、呼び出し側は止められない ctx を渡す。
func (n *networkTVStream) release(ctx context.Context, keep bool) error {
	failed, err := n.tryRelease(ctx, keep)
	if failed && !n.server.stopping() {
		go n.retryRelease(context.WithoutCancel(ctx))
	}

	return err
}

// retryRelease は、持ち主から外れられなかった ID から、外れられるまで ReleaseRetryInterval ごとにやり直す。
// 外れないままだと、このインスタンスが動いている間、NetworkTV を開いたままにし、チューナーを使い続ける。
// プロセスが止まり始めたらやめる。持ち主の行は、インスタンスが止まった後に neko が消し、NetworkTV を閉じる。
func (n *networkTVStream) retryRelease(ctx context.Context) {
	ticker := time.NewTicker(n.server.releaseRetryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-n.server.stopped:
			return
		case <-ticker.C:
		}
		if failed, _ := n.tryRelease(ctx, false); !failed {
			return
		}
	}
}

// tryRelease は、ID の持ち主から外れ、最後の持ち主なら閉じる。持ち主から外れられなかったら failed に true を返す。
func (n *networkTVStream) tryRelease(ctx context.Context, keep bool) (bool, error) {
	releaseCtx, cancel := context.WithTimeout(ctx, closeTimeout)
	last, err := n.server.ids.Release(releaseCtx, n.id, keep)
	cancel()
	if err != nil {
		return true, err
	}
	if !last {
		return false, nil
	}
	if keep {
		timer := time.NewTimer(n.server.handoverGrace)
		<-timer.C
	}
	ctx, cancel = context.WithTimeout(ctx, closeTimeout)
	defer cancel()
	if keep {
		closing, err := n.server.ids.CloseIfUnheld(ctx, n.id)
		if err != nil || !closing {
			return false, err
		}
	}
	var errs []error
	if err := n.server.client.CloseNetworkTV(ctx, n.id); err != nil {
		errs = append(errs, fmt.Errorf("failed to close NetworkTV %d: %w", n.id, err))
	}
	if err := n.server.ids.Forget(ctx, n.id); err != nil {
		errs = append(errs, err)
	}

	return false, errors.Join(errs...)
}

func (n *networkTVStream) preemption() error {
	return fmt.Errorf("%w: NetworkTV %d", tunerserver.ErrTunerPreempted, n.id)
}

// watch は、EDCB の予約にチューナーを取られたことに気づけるようにする (#1534)。
//   - 受け取った TS の PAT の TSID かサービスが、開いたときから変わったら、取られたとみなす。EDCB は、取られても接続を切らずに、
//     別の局の TS を送り続ける。プロセスも同じままなので、プロセスの ID では気づけない。
//   - PreemptionCheckInterval ごとに、NetworkTV がまだ開いているかを問い合わせ (NWTV_ID_SET_CH を useSID=0 で送る)、
//     閉じていたら取られたとみなし、中継の接続を切って Read を止める。
//
// 問い合わせは、ストリームと同じく ctx が終わるまで続ける。
func (n *networkTVStream) watch(ctx context.Context, service ServiceKey, processID int32) {
	// EDCB は、NetworkTV を開いた直後に、サービスを載せず NIT だけを載せた PAT を送ることがある (2026年11月に、Linux の EDCB で
	// 20 回開いて 8 回)。サービスが PAT から消えたことで取られたとみなすのは、サービスを載せた PAT を受け取った後にする。
	// 局が変われば TSID も変わるので、TSID は最初の PAT から見る。PAT は Read から順に届くので、seen は守らなくてよい。
	seen := false
	n.pat = ts.NewPATWatcher(func(pat *ts.PAT) {
		switch {
		case pat.TransportStreamID != service.TSID:
			n.preempted.Store(true)
		case slices.Contains(pat.Programs, service.SID):
			seen = true
		case seen:
			n.preempted.Store(true)
		}
	})
	ctx, cancel := context.WithCancel(ctx)
	n.stopCheck, n.checkDone = cancel, make(chan struct{})
	go func() {
		defer close(n.checkDone)
		ticker := time.NewTicker(n.server.preemptionCheckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			current, open, err := n.server.client.NetworkTVProcess(ctx, n.id)
			// 問い合わせに失敗しただけなら、TS の PAT で気づくのに任せる。
			if err != nil || (open && current == processID) {
				continue
			}
			n.preempted.Store(true)
			_ = n.ReadCloser.Close()

			return
		}
	}()
}

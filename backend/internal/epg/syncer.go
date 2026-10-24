package epg

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"maps"
	"slices"
	"time"

	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

const (
	// DefaultFullSyncInterval は、全量を突き合わせる間隔 (設計書の付録 A の初期値)。
	DefaultFullSyncInterval = 30 * time.Minute
	// 以下は、設計書 3.3 の「下限」「バックオフ」「1〜2 秒分をまとめる」を具体化した値で、付録 A には無い。
	// DefaultMinFullSyncInterval は、全量の突き合わせの間隔の下限。再接続が続いても、これより短い間隔では突き合わせない。
	DefaultMinFullSyncInterval = time.Minute
	// DefaultBatchWindow は、イベントをまとめて反映するまでに待つ時間。
	DefaultBatchWindow = 1500 * time.Millisecond
	// DefaultBackoffInitial と DefaultBackoffMax は、失敗したときに待つ時間の最初と上限。失敗が続くと倍にしていく。
	DefaultBackoffInitial = time.Second
	DefaultBackoffMax     = 5 * time.Minute
	// DefaultStableAfter は、イベントストリームがこの時間より長く続いたら、接続が安定したとみなしてバックオフを戻す時間。
	DefaultStableAfter = time.Minute
)

// EventSource は、イベントストリームを開くチューナーサーバーの API。*mirakurun.Client が満たす。
type EventSource interface {
	OpenEvents(ctx context.Context, query *mirakurun.EventsQuery) (mirakurun.EventStream, error)
}

var _ EventSource = new(mirakurun.Client)

// SyncerOptions は Syncer の設定。0 の項目は既定値にする。
type SyncerOptions struct {
	FullSyncInterval    time.Duration
	MinFullSyncInterval time.Duration
	BatchWindow         time.Duration
	BackoffInitial      time.Duration
	BackoffMax          time.Duration
	StableAfter         time.Duration
	// OnTunerEvent は、チューナーのイベントが届いたときに呼ぶ。チューナーの構成を取得し直させるのに使う。nil なら呼ばない。
	OnTunerEvent func()
	// OnFullSync は、全量の突き合わせを終えたときに、終えた時刻を渡して呼ぶ。/metrics で同期の遅れを示すのに使う。nil なら呼ばない。
	OnFullSync func(at time.Time)
}

// Syncer は、全量の突き合わせとイベントの購読を組み合わせて、EPG を同期し続ける (設計書 3.3)。
//
// イベントは「変化があった」という合図として扱い、変化のあったサービスの番組を取得し直して突き合わせる。
// Mirakurun のイベントは取りこぼしや遅れがあり、番組の削除も届かないことがあるので、
// データの正しさは、接続したときと定期的に行う全量の突き合わせで保証する。
type Syncer struct {
	reconciler *Reconciler
	events     EventSource
	clock      clock.Clock
	logger     *slog.Logger
	opts       SyncerOptions

	lastFullSync time.Time
}

// NewSyncer は Syncer を作る。opts は nil でもよい。
func NewSyncer(reconciler *Reconciler, events EventSource, clk clock.Clock, logger *slog.Logger, opts *SyncerOptions) *Syncer {
	o := SyncerOptions{
		FullSyncInterval:    DefaultFullSyncInterval,
		MinFullSyncInterval: DefaultMinFullSyncInterval,
		BatchWindow:         DefaultBatchWindow,
		BackoffInitial:      DefaultBackoffInitial,
		BackoffMax:          DefaultBackoffMax,
		StableAfter:         DefaultStableAfter,
		OnTunerEvent:        nil,
		OnFullSync:          nil,
	}
	if opts != nil {
		o.OnTunerEvent = opts.OnTunerEvent
		o.OnFullSync = opts.OnFullSync
		o.FullSyncInterval = cmpOr(opts.FullSyncInterval, o.FullSyncInterval)
		o.MinFullSyncInterval = cmpOr(opts.MinFullSyncInterval, o.MinFullSyncInterval)
		o.BatchWindow = cmpOr(opts.BatchWindow, o.BatchWindow)
		o.BackoffInitial = cmpOr(opts.BackoffInitial, o.BackoffInitial)
		o.BackoffMax = cmpOr(opts.BackoffMax, o.BackoffMax)
		o.StableAfter = cmpOr(opts.StableAfter, o.StableAfter)
	}

	return &Syncer{reconciler: reconciler, events: events, clock: clk, logger: logger, opts: o, lastFullSync: time.Time{}}
}

func cmpOr(v, fallback time.Duration) time.Duration {
	if v == 0 {
		return fallback
	}

	return v
}

// Run は、ctx が終わるまで同期を続ける。
// 接続したときに全量を突き合わせ、その後はイベントを反映しつつ、定期的に全量を突き合わせる。
// 失敗したら待ってから接続し直し、失敗が続くと待つ時間を延ばす。
func (s *Syncer) Run(ctx context.Context) error {
	backoff := s.opts.BackoffInitial
	for {
		stable, err := s.connect(ctx)
		// 止められて切れたのなら、失敗ではない。
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if stable {
			backoff = s.opts.BackoffInitial
		}
		s.logger.WarnContext(ctx, "EPG sync is interrupted; reconnecting", slog.Any("error", err), slog.Duration("backoff", backoff))

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, s.opts.BackoffMax)
	}
}

// connect は、全量を突き合わせてからイベントを購読し、ストリームが切れるまで反映する。
// ストリームが StableAfter より長く続いたら stable を true にして返す。
func (s *Syncer) connect(ctx context.Context) (bool, error) {
	if err := s.syncAllIfDue(ctx); err != nil {
		return false, err
	}

	stream, err := s.events.OpenEvents(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = stream.Close()
	}()
	opened := s.clock.Now()
	err = s.consume(ctx, stream)

	return s.clock.Now().Sub(opened) >= s.opts.StableAfter, err
}

// syncAllIfDue は、前回から MinFullSyncInterval 以上たっていれば全量を突き合わせる。
func (s *Syncer) syncAllIfDue(ctx context.Context) error {
	if !s.lastFullSync.IsZero() && s.clock.Now().Sub(s.lastFullSync) < s.opts.MinFullSyncInterval {
		return nil
	}

	return s.syncAll(ctx)
}

func (s *Syncer) syncAll(ctx context.Context) error {
	result, err := s.reconciler.SyncAll(ctx)
	if err != nil {
		return err
	}
	s.lastFullSync = s.clock.Now()
	s.logResult(ctx, "synced the whole EPG", result)
	if s.opts.OnFullSync != nil {
		s.opts.OnFullSync(s.lastFullSync)
	}

	return nil
}

// consume は、ストリームのイベントを BatchWindow ごとにまとめて反映する。全量の突き合わせも定期的に行う。
func (s *Syncer) consume(ctx context.Context, stream mirakurun.EventStream) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	events := make(chan *mirakurun.Event)
	readErr := make(chan error, 1)
	go func() {
		readErr <- readEvents(ctx, stream, events)
	}()

	fullSync := time.NewTicker(s.opts.FullSyncInterval)
	defer fullSync.Stop()
	var flush <-chan time.Time
	batch := newEventBatch()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-readErr:
			// 切れる前に届いていたイベントは反映しておく。
			return errors.Join(err, s.apply(ctx, batch))
		case event := <-events:
			batch.add(event)
			if flush == nil {
				flush = time.After(s.opts.BatchWindow)
			}
		case <-flush:
			flush = nil
			if err := s.apply(ctx, batch); err != nil {
				return err
			}
			batch = newEventBatch()
		case <-fullSync.C:
			if err := s.syncAll(ctx); err != nil {
				return err
			}
			// 溜まっていたイベントの変化は、今の全量の突き合わせで反映済みなので捨てる。
			// チューナーの構成は全量の突き合わせの対象ではないので、チューナーのイベントだけは知らせる。
			if batch.tuners && s.opts.OnTunerEvent != nil {
				s.opts.OnTunerEvent()
			}
			flush = nil
			batch = newEventBatch()
		}
	}
}

// apply は、まとめたイベントを反映する。サービスが変わったときは全量を、番組が変わったときはそのサービスを突き合わせる。
func (s *Syncer) apply(ctx context.Context, batch *eventBatch) error {
	if batch.tuners && s.opts.OnTunerEvent != nil {
		s.opts.OnTunerEvent()
	}
	if batch.services {
		return s.syncAll(ctx)
	}
	if len(batch.serviceIDs) == 0 {
		return nil
	}

	result, err := s.reconciler.SyncServices(ctx, slices.Sorted(maps.Keys(batch.serviceIDs)))
	if err != nil {
		return err
	}
	s.logResult(ctx, "applied EPG events", result)

	return nil
}

func (s *Syncer) logResult(ctx context.Context, msg string, result *Result) {
	s.logger.InfoContext(ctx, msg,
		slog.Int("services_upserted", result.ServicesUpserted),
		slog.Int("programs_upserted", result.ProgramsUpserted),
		slog.Int("programs_deleted", result.ProgramsDeleted),
		slog.Int64("version", result.Version))
}

// readEvents は、ストリームのイベントを events に送る。ストリームが終わったら、その理由を返す。
func readEvents(ctx context.Context, stream mirakurun.EventStream, events chan<- *mirakurun.Event) error {
	for {
		event, err := stream.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return errors.New("the event stream is closed")
			}

			return err
		}
		select {
		case events <- event:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// eventBatch は、反映を待つイベントをまとめたもの。
type eventBatch struct {
	// serviceIDs は、番組が変わったサービス。
	serviceIDs map[int64]struct{}
	// services は、サービスが変わったかどうか。
	services bool
	// tuners は、チューナーのイベントが届いたかどうか。
	tuners bool
}

func newEventBatch() *eventBatch {
	return &eventBatch{serviceIDs: map[int64]struct{}{}, services: false, tuners: false}
}

// add は、イベントから変化のあったものを読み取る。
// Mirakurun と Mahiron の program のイベントは、create と update では番組を、remove では番組の ID だけを送る。
// mirakc の epg.programs-updated と onair.program-changed は、番組が変わったサービスの ID を送る。
func (b *eventBatch) add(event *mirakurun.Event) {
	switch {
	case event.Resource == "program":
		var data struct {
			ID        int64 `json:"id"`
			NetworkID int64 `json:"networkId"`
			ServiceID int64 `json:"serviceId"`
		}
		if err := json.Unmarshal(event.Data, &data); err != nil {
			// 形の分からないイベントは捨てる。取りこぼしは全量の突き合わせで埋まる。
			return
		}
		// サービスの番号は networkId と組でしか一意にならないので、片方しか無ければ番組 ID から求める。
		if data.NetworkID != 0 && data.ServiceID != 0 {
			b.serviceIDs[data.NetworkID*serviceIDBase+data.ServiceID] = struct{}{}
		} else if data.ID != 0 {
			// 番組 ID は (networkId × 100000 + serviceId) × 100000 + eventId なので、下 5 桁を除くとサービス ID になる。
			b.serviceIDs[data.ID/serviceIDBase] = struct{}{}
		}
	// mirakc の onair.program-changed は、放送中か次の番組 (EIT[p/f]) が変わったサービスの ID を送る (設計書 5.1)。
	case event.Resource == "epg" && event.Type == "programs-updated",
		event.Resource == "onair" && event.Type == "program-changed":
		var data struct {
			ServiceID int64 `json:"serviceId"`
		}
		if err := json.Unmarshal(event.Data, &data); err == nil && data.ServiceID != 0 {
			b.serviceIDs[data.ServiceID] = struct{}{}
		}
	case event.Resource == "service":
		b.services = true
	case event.Resource == "tuner":
		b.tuners = true
	}
}

package edcb

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
)

// OpenEvents は、EpgTimerSrv の通知 (GET_STATUS_NOTIFY2) を、チューナーサーバーのイベントに読み替えるストリームを開く (#1531)。
// EPG のデータを読み込み直した通知 (NotifyUpdateEPGData) は、どのサービスが変わったかを持たないので、
// サービスが変わったイベントにして、全量を突き合わせさせる。ほかの通知は読み飛ばす。
func (s *TunerServer) OpenEvents(ctx context.Context) (tunerserver.EventStream, error) {
	// 巡回カウンタが 0 の問い合わせは、待たずに、最後の通知の巡回カウンタを返す。それより後の通知から読む。
	current, err := s.client.WaitNotification(ctx, 0)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	stream := &eventStream{
		events: make(chan *tunerserver.Event), err: make(chan error, 1), cancel: cancel, done: make(chan struct{}),
	}
	go stream.wait(ctx, s.client, current.Count)

	return stream, nil
}

// eventStream は、EpgTimerSrv の通知を、別の goroutine で順に待つ。Close で、保留している問い合わせを切る。
type eventStream struct {
	events chan *tunerserver.Event
	// err は、通知を待てなくなった理由。待つ goroutine は、これを送って終わる。
	err    chan error
	cancel context.CancelFunc
	done   chan struct{}
}

var _ tunerserver.EventStream = (*eventStream)(nil)

// Next は、次のイベントを返す。通知を待てなくなったら、その理由を返す。
func (e *eventStream) Next() (*tunerserver.Event, error) {
	select {
	case event := <-e.events:
		return event, nil
	case err := <-e.err:
		// 続けて呼ばれても同じ理由を返す。
		e.err <- err

		return nil, err
	}
}

// Close は、通知を待つのをやめ、待つ goroutine が終わるまで待つ。
func (e *eventStream) Close() error {
	e.cancel()
	<-e.done

	return nil
}

func (e *eventStream) wait(ctx context.Context, client *Client, count uint32) {
	defer close(e.done)
	for {
		notification, err := client.WaitNotification(ctx, count)
		if err != nil {
			e.err <- err

			return
		}
		count = notification.Count
		if notification.ID != NotifyUpdateEPGData {
			continue
		}
		select {
		case e.events <- &tunerserver.Event{Type: tunerserver.EventTypeServiceChanged, ServiceID: 0}:
		case <-ctx.Done():
			e.err <- ctx.Err()

			return
		}
	}
}

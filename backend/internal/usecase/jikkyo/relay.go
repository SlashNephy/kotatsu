package jikkyo

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/infra/jikkyoclient"
)

const (
	// relayGrace は、最後の視聴者が抜けてから NX-Jikkyo との接続を閉じるまでの時間。局を行き来しても、つなぎ直さずに済ませる。
	relayGrace = 10 * time.Second
	// subscriberBuffer は、視聴者ごとに溜めておくイベントの数。これを超えたら、遅い視聴者の分だけ捨てる。
	subscriberBuffer = 256
)

// Event は、視聴者に届ける実況のイベント。Status と Comment のどちらかを入れる。
type Event struct {
	Status  *jikkyoclient.LiveStatus
	Comment *jikkyoclient.LiveComment
}

// Relay は、NX-Jikkyo のライブのコメントを、チャンネルごとに 1 本の接続で受け取り、視聴者に配る (設計書 7.4)。
// tenban のプロセスごとに持ち、複数の tenban の間では調整しない。
type Relay struct {
	client *jikkyoclient.Client
	grace  time.Duration

	mu   sync.Mutex
	hubs map[string]*hub
}

type hub struct {
	session     *jikkyoclient.LiveSession
	subscribers map[*Subscription]struct{}
	// status は、最後の接続の状態。あとから来た視聴者にも知らせる。
	status *jikkyoclient.LiveStatus
	timer  *time.Timer
}

// Subscription は、1 人の視聴者の購読。
type Subscription struct {
	// Events は、届いたイベント。
	Events <-chan *Event

	events    chan *Event
	relay     *Relay
	channelID string
	once      sync.Once
}

// NewRelay は Relay を返す。
func NewRelay(client *jikkyoclient.Client) *Relay {
	return NewRelayWithGrace(client, relayGrace)
}

// NewRelayWithGrace は、最後の視聴者が抜けてから接続を閉じるまでの時間を指定して Relay を返す。
func NewRelayWithGrace(client *jikkyoclient.Client, grace time.Duration) *Relay {
	return &Relay{client: client, grace: grace, mu: sync.Mutex{}, hubs: map[string]*hub{}}
}

// Subscribe は、チャンネル channelID のコメントの購読を始める。Close で終える。
// NX-Jikkyo との接続はほかの視聴者と共有するので、ctx の取り消しでは閉じない。
func (r *Relay) Subscribe(ctx context.Context, channelID string) *Subscription {
	events := make(chan *Event, subscriberBuffer)
	subscription := &Subscription{Events: events, events: events, relay: r, channelID: channelID, once: sync.Once{}}

	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.hubs[channelID]
	if !ok {
		current = &hub{session: nil, subscribers: map[*Subscription]struct{}{}, status: nil, timer: nil}
		r.hubs[channelID] = current
		current.session = r.client.OpenLive(context.WithoutCancel(ctx), &jikkyoclient.LiveOptions{
			ChannelID: channelID, Cookie: "", Receive: true,
			OnComment: func(comment *jikkyoclient.LiveComment) { r.publish(current, &Event{Status: nil, Comment: comment}) },
			OnStatus:  func(status *jikkyoclient.LiveStatus) { r.publishStatus(current, status) },
		})
	}
	if current.timer != nil {
		current.timer.Stop()
		current.timer = nil
	}
	current.subscribers[subscription] = struct{}{}
	if current.status != nil {
		events <- &Event{Status: current.status, Comment: nil}
	}

	return subscription
}

// Close は、購読を終える。最後の視聴者が抜けたら、少し待ってから NX-Jikkyo との接続を閉じる。
func (s *Subscription) Close() {
	s.once.Do(func() { s.relay.unsubscribe(s) })
}

func (r *Relay) unsubscribe(subscription *Subscription) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.hubs[subscription.channelID]
	if !ok {
		return
	}
	delete(current.subscribers, subscription)
	if len(current.subscribers) > 0 {
		return
	}
	current.timer = time.AfterFunc(r.grace, func() { r.closeIfIdle(subscription.channelID, current) })
}

func (r *Relay) closeIfIdle(channelID string, target *hub) {
	r.mu.Lock()
	if r.hubs[channelID] != target || len(target.subscribers) > 0 {
		r.mu.Unlock()

		return
	}
	delete(r.hubs, channelID)
	r.mu.Unlock()
	target.session.Close()
}

func (r *Relay) publishStatus(target *hub, status *jikkyoclient.LiveStatus) {
	r.mu.Lock()
	target.status = status
	r.mu.Unlock()
	r.publish(target, &Event{Status: status, Comment: nil})
}

func (r *Relay) publish(target *hub, event *Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for subscription := range target.subscribers {
		select {
		case subscription.events <- event:
		default:
			// 読むのが遅い視聴者のために、ほかの視聴者を待たせない。
		}
	}
}

// PosterCookie は、ユーザーの投稿を NX-Jikkyo で見分けるための値を返す。
// NX-Jikkyo はこの値からユーザーの ID を作るので、ID でのミュートをユーザーごとに効かせられる。
// アプリの秘密キーで作るので、NX-Jikkyo には Kotatsu のユーザー ID を渡さない。
func PosterCookie(secret string, userID uuid.UUID) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("jikkyo-poster:" + userID.String()))

	return hex.EncodeToString(mac.Sum(nil))
}

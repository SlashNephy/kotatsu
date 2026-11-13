// Package tunerserver は、チューナーサーバーの種類に依らない型と interface を表現する (設計書 3.1)。
//
// usecase と web は、この型でチューナーサーバーを扱う。Mirakurun 互換のサーバーの API の形と、イベントやエラーの解釈は、
// 実装 (infra/mirakurun) が受け持ち、この型に変換して返す。
//
// サービスは、Mirakurun のサービス ID (networkId × 100000 + serviceId) で識別する (設計書 3.2)。
// Mirakurun 互換でない実装も、この形の ID を作って返す。
package tunerserver

import (
	"context"
	"io"
	"time"

	"github.com/SlashNephy/kotatsu/internal/domain"
)

// Client は、Kotatsu が使うチューナーサーバーの API。複数の goroutine から使ってよい。
// 使う側は、要るメソッドだけの interface を自分で持ち、この interface を満たす値を受け取る。
type Client interface {
	Channels(ctx context.Context) ([]*Channel, error)
	Services(ctx context.Context) ([]*Service, error)
	// Programs は番組を返す。query が nil なら全件を返す。
	Programs(ctx context.Context, query *ProgramsQuery) ([]*Program, error)
	// OnAirPrograms は、放送中と次の番組 (EIT[p/f]) を返す。持たないサーバーでは空を返す。
	OnAirPrograms(ctx context.Context) ([]*OnAirProgram, error)
	Tuners(ctx context.Context) ([]*Tuner, error)
	// ServerDate は、チューナーサーバーの時計の時刻を返す (設計書 3.7)。
	ServerDate(ctx context.Context) (time.Time, error)
	// Identify は、チューナーサーバーの種類と版を返す (#1538)。
	Identify(ctx context.Context) (*Identity, error)
	// OpenEvents は、チューナーサーバーの変化を知らせるイベントのストリームを開く。
	OpenEvents(ctx context.Context) (EventStream, error)
	// OpenServiceStream は、サービスのストリームを開く。返す io.ReadCloser は、読み終えたら必ず閉じる。
	// opts が nil なら、スクランブルを解除せず、優先度 0 で開く。
	// チューナーを確保できなければ ErrTunerUnavailable を、サービスがなければ ErrNotFound を包んだエラーを返す。
	OpenServiceStream(ctx context.Context, serviceID int64, opts *StreamOptions) (io.ReadCloser, error)
	// OpenChannelStream は、チャンネル種別 channelType のチャンネル channel のストリームを、すべてのサービスを含んだまま開く。
	// エラーは OpenServiceStream と同じく返す。
	OpenChannelStream(ctx context.Context, channelType, channel string, opts *StreamOptions) (io.ReadCloser, error)
	// ServiceLogo は、サービスのロゴの PNG を返す。
	// サービスがなければ ErrNotFound を、ロゴをまだ受信していなければ ErrLogoUnavailable を包んだエラーを返す。
	ServiceLogo(ctx context.Context, serviceID int64) ([]byte, error)
}

// Channel は、チューナーサーバーのチャンネル。
type Channel struct {
	// Type はチャンネル種別。GR、BS、CS、SKY のほか、Mahiron は BS4K や EXT1 のような任意の文字列を返すので、
	// 列挙型にせず文字列のまま持つ (設計書 3.1)。
	Type     string
	Channel  string
	Name     string
	Services []*Service
}

// ChannelRef は、Service が属するチャンネル。
type ChannelRef struct {
	Type    string
	Channel string
}

// Service は、チューナーサーバーのサービス。
type Service struct {
	// ID はサービス ID (networkId × 100000 + serviceId)。Kotatsu はこの値でサービスを識別する (設計書 3.2)。
	ID        int64
	ServiceID int
	NetworkID int
	Name      string
	Type      int
	// LogoID と RemoteControlKeyID は、サーバーが返さなければ nil。
	LogoID             *int
	RemoteControlKeyID *int
	// Channel は、サーバーが返さなければ nil。
	Channel *ChannelRef
}

// Program は、チューナーサーバーの番組。
type Program struct {
	// ID は Mirakurun の番組 ID ((networkId × 100000 + serviceId) × 100000 + eventId)。
	// EID は再利用されうるので、番組の同一性の判定には使わない (設計書 3.2)。
	ID        int64
	EventID   int
	ServiceID int
	NetworkID int
	// StartAt は開始時刻 (UNIX 時間のミリ秒)。
	StartAt int64
	// Duration は長さ (ミリ秒)。終了時刻が未定のときは 0 か 1 にする (Mirakurun は 1、Mahiron は 0 を返す)。
	Duration    int64
	IsFree      bool
	Name        string
	Description string
	Genres      []*domain.Genre
	Extended    map[string]string
	// RelatedItems は、イベント共有 (shared) とイベントリレー (relay) で関連づけられた番組 (設計書 4.4、4.6)。
	RelatedItems []*domain.RelatedItem
	// Audios は、番組の音声の構成。音声コンポーネント記述子ごとに 1 つある。
	Audios []*domain.Audio
	// Video は、番組の映像。EIT のコンポーネント記述子から作られる。サーバーが返さなければ nil。
	Video *Video
	// Series は、EIT のシリーズの記述子 (#1311)。サーバーが返さなければ nil。
	Series *Series
}

// Start は開始時刻を返す。
func (p *Program) Start() time.Time {
	return time.UnixMilli(p.StartAt).UTC()
}

// Series は、番組のシリーズの記述子 (ARIB STD-B10 の series_descriptor)。
type Series struct {
	ID int
	// Repeat は、再放送の番号 (repeat_label)。0 なら初回の放送。
	Repeat int
	// Episode は話数、LastEpisode は最終話の話数。分からなければ 0。
	Episode     int
	LastEpisode int
	Name        string
}

// Video は、番組の映像。
type Video struct {
	// Resolution は、映像の解像度 (480i、1080i、2160p、4320p など)。
	// Mahiron は BS4K の番組の type を mpeg2 と返すので、映像の種類は Resolution で見分ける (#806)。
	Resolution string
}

// ProgramsQuery は、番組の絞り込みの条件。0 の項目では絞り込まない。
type ProgramsQuery struct {
	NetworkID int
	ServiceID int
	EventID   int
}

// OnAirProgram は、サービスで放送中の番組と次の番組。
// 番組表 (EIT[schedule]) ではなく EIT[p/f] から作るので、延長や繰り上げがすぐに反映される (設計書 5.1)。
type OnAirProgram struct {
	// ServiceID はサービス ID (networkId × 100000 + serviceId)。
	ServiceID int64
	// Current は放送中の番組。放送していなければ nil。
	Current *Program
	// Next は次の番組。なければ nil。
	Next *Program
}

// Tuner は、チューナーサーバーのチューナー。
type Tuner struct {
	Index int
	Name  string
	// Types は、このチューナーが受信できるチャンネル種別。1 本で複数の種別を受信できる機器もある (設計書 3.6)。
	Types []string
	// Users は、チューナーを使っている利用者。利用者を返さないサーバーでは空にする。
	Users       []*TunerUser
	IsAvailable bool
	IsRemote    bool
	IsFree      bool
	IsUsing     bool
	IsFault     bool
}

// TunerUser は、チューナーを使っている利用者。
type TunerUser struct {
	ID       string
	Priority int
	Agent    string
}

// TunerUserIdentifier は、チューナーの利用者の ID を返せるストリーム (#1467)。
// OpenServiceStream が返すストリームが満たしていれば、Tuner の Users と突き合わせて、受信しているチューナーを見つけられる。
// 利用者の ID を返さないサーバーのストリームは、これを満たさないか、空を返してよい。
type TunerUserIdentifier interface {
	TunerUserID() string
}

// TunerOf は、利用者の ID が userID のチューナーを返す。見つからなければ nil (#1467)。
func TunerOf(tuners []*Tuner, userID string) *Tuner {
	if userID == "" {
		return nil
	}
	for _, tuner := range tuners {
		if tuner == nil {
			continue
		}
		for _, user := range tuner.Users {
			if user != nil && user.ID == userID {
				return tuner
			}
		}
	}

	return nil
}

// StreamOptions は、ストリームを開くときの設定。
type StreamOptions struct {
	// Decode が true なら、チューナーサーバーにスクランブルを解除させる。
	Decode bool
	// Priority はチューナーの優先度 (Mirakurun の priority の値)。チューナーが足りないとき、優先度の低い利用者からチューナーを奪う。
	// 負の値なら、他の利用者からチューナーを奪わない。予約ごとの設定をそのまま渡すので、用途の列挙にはしない。
	// 優先度を持たないサーバーは無視してよい。
	Priority int
}

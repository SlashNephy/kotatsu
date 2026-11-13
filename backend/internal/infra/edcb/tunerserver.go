package edcb

import (
	"context"
	"maps"
	"strconv"
	"strings"
	"time"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

// TunerServer は、EDCB の EpgTimerSrv を、Kotatsu のチューナーサーバーの型で読む (#1531、設計書 3.1)。
// 今は、EPG の同期に使うサービスと番組と、チューナーの一覧を持つ。値は、Mirakurun が同じ放送から作る値にそろえる。
type TunerServer struct {
	client *Client
	// text は、EDCB が書いた番組とサービスの文を、Mirakurun と同じ形に戻す。
	text *strings.Replacer
	// bonDrivers は、Kotatsu がライブと録画に使う BonDriver の名前。空なら、すべてのチューナーを使えるとみなす (#1532)。
	bonDrivers map[string]bool
	// ids は、NetworkTV の ID を配る (#1533)。nil なら、ストリームを開けない。
	ids NetworkTVIDs
	// preemptionCheckInterval は、NetworkTV がまだ開いているかを問い合わせる間隔 (#1534)。
	preemptionCheckInterval time.Duration
}

// TunerServerOptions は TunerServer の設定。
type TunerServerOptions struct {
	// BonDrivers は、Kotatsu がライブと録画に使う BonDriver の名前。EpgTimerSrv.ini の [TVTEST] に並べたものと同じにする。
	// 空なら、すべてのチューナーを使えるとみなす (#1532)。
	BonDrivers []string
	// NetworkTVIDs は、ストリームを開くときに NetworkTV の ID を配る (#1533)。nil なら、ストリームを開けない。
	NetworkTVIDs NetworkTVIDs
	// PreemptionCheckInterval は、NetworkTV がまだ開いているかを問い合わせる間隔 (#1534)。0 なら 5 秒。
	PreemptionCheckInterval time.Duration
}

// NewTunerServer は TunerServer を返す。opts は nil でもよい。
func NewTunerServer(client *Client, opts *TunerServerOptions) *TunerServer {
	bonDrivers := map[string]bool{}
	var ids NetworkTVIDs
	interval := defaultPreemptionCheckInterval
	if opts != nil {
		if opts.PreemptionCheckInterval > 0 {
			interval = opts.PreemptionCheckInterval
		}
		for _, name := range opts.BonDrivers {
			bonDrivers[strings.ToLower(name)] = true
		}
		ids = opts.NetworkTVIDs
	}

	return &TunerServer{client: client, text: newTextReplacer(), bonDrivers: bonDrivers, ids: ids, preemptionCheckInterval: interval}
}

// newTextReplacer は、EDCB の文を Mirakurun と同じ形に戻す置き換えを作る。
//   - 改行は、CRLF を LF にする。
//   - EDCB は、JIS X 0208 の一部の文字を、CP932 の流儀の Unicode にする。Mirakurun と同じ JIS の流儀に戻す
//     (1 区 33 点の「〜」(U+301C) が「～」(U+FF5E) に、1 区 29 点の「—」(U+2014) が「―」(U+2015) になる)。
//     NFKC では「～」が「~」になり、「〜」とそろわないので、ルールのキーワードが一致しなくなる。
//   - 追加記号の代わりの文字列 (「[字]」など) のうち、戻せるものを記号に戻す (gaijiFallbacks)。
//
// 中型の英数を半角にするかどうかや、いくつもの記号が同じ文字列になるものは、EDCB が書いた時点で分からなくなるので、戻さない。
// 照合は NFKC で幅をそろえ、括弧の印も囲み文字と同じに扱うので、照合には効かない (設計書 3.5)。
func newTextReplacer() *strings.Replacer {
	pairs := append([]string{"\r\n", "\n", "～", "〜", "―", "—"}, gaijiFallbacks()...)

	return strings.NewReplacer(pairs...)
}

// Services は、サービスを返す。一覧は、チャンネルスキャンで拾った全サービス (ChSet5.txt) から作る。
// ENUM_SERVICE は EPG を読み込んだサービスしか返さないので、リモコンキー ID を補うのにだけ使う。
// EpgTimerSrv が EPG のデータを読み込み終えていなければ ErrBusy を返す。空の一覧として扱うと、DB のサービスを消してしまう。
func (s *TunerServer) Services(ctx context.Context) ([]*tunerserver.Service, error) {
	entries, err := s.channelEntries(ctx)
	if err != nil {
		return nil, err
	}
	loaded, err := s.client.Services(ctx)
	if err != nil {
		return nil, err
	}
	remoteKeys := make(map[ServiceKey]uint8, len(loaded))
	for _, service := range loaded {
		remoteKeys[service.ServiceKey] = service.RemoteControlKeyID
	}
	services := make([]*tunerserver.Service, 0, len(entries))
	for _, entry := range entries {
		services = append(services, s.toService(entry, remoteKeys[entry.ServiceKey]))
	}

	return services, nil
}

// Programs は、番組を返す。query が nil なら全件を返す。開始時刻が未定の番組は返さない (Mirakurun も返さない)。
// EDCB はサービスを TSID まで含めて見分けるので、サービスを絞るときは、チャンネルの設定から TSID を引く。
func (s *TunerServer) Programs(ctx context.Context, query *tunerserver.ProgramsQuery) ([]*tunerserver.Program, error) {
	events := &EventsQuery{Services: nil, From: time.Time{}, To: time.Time{}}
	if query != nil && query.NetworkID != 0 && query.ServiceID != 0 {
		entries, err := s.channelEntries(ctx)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if int(entry.ONID) == query.NetworkID && int(entry.SID) == query.ServiceID {
				events.Services = append(events.Services, entry.ServiceKey)
			}
		}
		if len(events.Services) == 0 {
			return []*tunerserver.Program{}, nil
		}
	}
	list, err := s.client.Events(ctx, events)
	if err != nil {
		return nil, err
	}
	index := indexEvents(list)
	// サービスを絞ったときは、共有の番組が指す先のサービスの番組も引く。
	if missing := missingSharedServices(list, index); len(missing) > 0 {
		more, err := s.client.Events(ctx, &EventsQuery{Services: missing, From: time.Time{}, To: time.Time{}})
		if err != nil {
			return nil, err
		}
		maps.Copy(index, indexEvents(more))
	}
	programs := []*tunerserver.Program{}
	for _, item := range list {
		for _, event := range item.Events {
			if !event.StartKnown || !matchProgram(query, event) {
				continue
			}
			programs = append(programs, s.toProgram(withSharedDetails(event, index)))
		}
	}

	return programs, nil
}

// eventKey は、EDCB で番組を見分ける組。
type eventKey struct {
	ServiceKey

	EventID uint16
}

func indexEvents(list []*ServiceEvents) map[eventKey]*Event {
	index := map[eventKey]*Event{}
	for _, item := range list {
		for _, event := range item.Events {
			index[eventKey{ServiceKey: event.ServiceKey, EventID: event.EventID}] = event
		}
	}

	return index
}

// sharedSource は、文を持たない番組が、共有のイベントグループ (group_type 1) で指す、文を持つ番組を返す。なければ nil。
// サブチャンネルの番組は、放送でメインのチャンネルの番組を指すイベントグループだけを持ち、番組名などを持たない。
func sharedSource(event *Event, index map[eventKey]*Event) *Event {
	if event.Short != nil || event.Group == nil || event.Group.Type != 1 {
		return nil
	}
	for _, ref := range event.Group.Events {
		source := index[eventKey(ref)]
		if source != nil && source != event && source.Short != nil {
			return source
		}
	}

	return nil
}

// missingSharedServices は、文を持たない番組が共有のイベントグループで指す番組のうち、まだ引いていないもののサービスを返す。
func missingSharedServices(list []*ServiceEvents, index map[eventKey]*Event) []ServiceKey {
	seen := map[ServiceKey]bool{}
	missing := []ServiceKey{}
	for _, item := range list {
		for _, event := range item.Events {
			if event.Short != nil || event.Group == nil || event.Group.Type != 1 || sharedSource(event, index) != nil {
				continue
			}
			for _, ref := range event.Group.Events {
				if _, ok := index[eventKey(ref)]; !ok && !seen[ref.ServiceKey] {
					seen[ref.ServiceKey] = true
					missing = append(missing, ref.ServiceKey)
				}
			}
		}
	}

	return missing
}

// withSharedDetails は、文を持たない番組に、共有のイベントグループが指す番組の文、ジャンル、映像、音声、詳細を写した番組を返す。
// Mirakurun も、サブチャンネルの番組に、メインのチャンネルの番組の情報を写して返す。スクランブルの有無は、番組自身のものを使う。
func withSharedDetails(event *Event, index map[eventKey]*Event) *Event {
	source := sharedSource(event, index)
	if source == nil {
		return event
	}
	shared := *event
	shared.Short, shared.Extended, shared.Genres, shared.Video, shared.Audios = source.Short, source.Extended, source.Genres, source.Video, source.Audios

	return &shared
}

func matchProgram(query *tunerserver.ProgramsQuery, event *Event) bool {
	if query == nil {
		return true
	}

	return (query.NetworkID == 0 || query.NetworkID == int(event.ONID)) &&
		(query.ServiceID == 0 || query.ServiceID == int(event.SID)) &&
		(query.EventID == 0 || query.EventID == int(event.EventID))
}

// OnAirPrograms は、放送中と次の番組を返す。EDCB には、EIT[p/f] を返すコマンドがないので、空を返す。
// 放送中の番組の延長と繰り下げは、録画している TS の EIT[p/f] で追う (設計書 5.1)。
func (s *TunerServer) OnAirPrograms(context.Context) ([]*tunerserver.OnAirProgram, error) {
	return []*tunerserver.OnAirProgram{}, nil
}

func (s *TunerServer) channelEntries(ctx context.Context) ([]*ChannelEntry, error) {
	data, err := s.client.ChannelSettings(ctx)
	if err != nil {
		return nil, err
	}

	return ParseChannelSettings(data)
}

// serviceID は、Kotatsu のサービス ID (networkId × 100000 + serviceId、設計書 3.2)。TSID は含めない。
func serviceID(key ServiceKey) int64 {
	return int64(key.ONID)*100000 + int64(key.SID)
}

func (s *TunerServer) toService(entry *ChannelEntry, remoteKey uint8) *tunerserver.Service {
	var remote *int
	// EDCB は、リモコンキー ID の「無い」を 0 で表す。
	if remoteKey != 0 {
		remote = new(int(remoteKey))
	}

	return &tunerserver.Service{
		ID:                 serviceID(entry.ServiceKey),
		ServiceID:          int(entry.SID),
		NetworkID:          int(entry.ONID),
		Name:               s.text.Replace(entry.Name),
		Type:               entry.Type,
		LogoID:             nil,
		RemoteControlKeyID: remote,
		Channel:            &tunerserver.ChannelRef{Type: ChannelType(entry.ONID), Channel: strconv.Itoa(int(entry.TSID))},
	}
}

// ChannelType は、ONID から、チャンネル種別 (GR、BS、CS、BS4K、SKY) を決める。EDCB は種別を持たない。
// 地上波は、ネットワーク ID が 0x7880 から 0x7FE8 の範囲にある (ARIB TR-B14)。
// チャンネルは、TS (TSID) ごとに 1 つとする。EDCB は物理チャンネルを返さないので、同じ TS のサービスを束ねるのに TSID を使う。
func ChannelType(onid uint16) string {
	switch {
	case onid >= 0x7880 && onid <= 0x7FE8:
		return "GR"
	case onid == 0x0004:
		return "BS"
	case onid == 0x0006 || onid == 0x0007:
		return "CS"
	case onid == 0x000B:
		return "BS4K"
	default:
		return "SKY"
	}
}

func (s *TunerServer) toProgram(event *Event) *tunerserver.Program {
	p := &tunerserver.Program{
		ID:           serviceID(event.ServiceKey)*100000 + int64(event.EventID),
		EventID:      int(event.EventID),
		ServiceID:    int(event.SID),
		NetworkID:    int(event.ONID),
		StartAt:      event.Start.UnixMilli(),
		Duration:     1, // 終了時刻が未定のときは、Mirakurun と同じく 1 にする。
		IsFree:       !event.Scrambled,
		Name:         "",
		Description:  "",
		Genres:       nil, // ジャンルがなければ、Mirakurun と同じく nil にする。
		Extended:     map[string]string{},
		RelatedItems: []*domain.RelatedItem{},
		Audios:       make([]*domain.Audio, 0, len(event.Audios)),
		Video:        nil,
		Series:       nil, // EDCB は、シリーズの記述子を持たない。
	}
	if event.DurationKnown {
		p.Duration = event.Duration.Milliseconds()
	}
	if event.Short != nil {
		p.Name, p.Description = s.text.Replace(event.Short.Name), s.text.Replace(event.Short.Text)
	}
	if event.Extended != nil {
		for key, value := range ParseExtended(*event.Extended) {
			p.Extended[s.text.Replace(key)] = s.text.Replace(value)
		}
	}
	for _, g := range event.Genres {
		p.Genres = append(p.Genres, &domain.Genre{Lv1: int(g.Level1), Lv2: int(g.Level2), Un1: int(g.User1), Un2: int(g.User2)})
	}
	for _, group := range []*EventGroup{event.Group, event.Relay} {
		p.RelatedItems = append(p.RelatedItems, relatedItems(group)...)
	}
	for _, a := range event.Audios {
		p.Audios = append(p.Audios, &domain.Audio{
			ComponentType: int(a.ComponentType),
			ComponentTag:  new(int(a.ComponentTag)),
			IsMain:        a.MainComponent,
			SamplingRate:  ts.SamplingRate(a.SamplingRate),
			// EDCB は、音声の言語コードを持たない。
			Langs: []string{},
		})
	}
	if event.Video != nil {
		if resolution, ok := videoResolution(event.Video.ComponentType); ok {
			p.Video = &tunerserver.Video{Resolution: resolution}
		}
	}

	return p
}

// relatedItems は、イベントグループを、Mirakurun と同じ形の関連する番組にする。
// group_type が 4 より小さい記述子は、同じネットワークの番組を指し、ネットワーク ID を持たない。
func relatedItems(group *EventGroup) []*domain.RelatedItem {
	if group == nil {
		return nil
	}
	kind := "movement"
	switch group.Type {
	case 1:
		kind = "shared"
	case 2, 4:
		kind = "relay"
	}
	items := make([]*domain.RelatedItem, 0, len(group.Events))
	for _, ref := range group.Events {
		item := &domain.RelatedItem{Type: kind, NetworkID: 0, ServiceID: int(ref.SID), EventID: int(ref.EventID)}
		if group.Type >= 4 {
			item.NetworkID = int(ref.ONID)
		}
		items = append(items, item)
	}

	return items
}

// videoResolution は、映像のコンポーネント記述子の component_type から、解像度を引く。Mirakurun の表と同じ。
func videoResolution(componentType uint8) (string, bool) {
	switch componentType {
	case 0x01, 0x02, 0x03, 0x04:
		return "480i", true
	case 0x83:
		return "4320p", true
	case 0x91, 0x92, 0x93, 0x94:
		return "2160p", true
	case 0xA1, 0xA2, 0xA3, 0xA4:
		return "480p", true
	case 0xB1, 0xB2, 0xB3, 0xB4:
		return "1080i", true
	case 0xC1, 0xC2, 0xC3, 0xC4:
		return "720p", true
	case 0xD1, 0xD2, 0xD3, 0xD4:
		return "240p", true
	case 0xE1, 0xE2, 0xE3, 0xE4:
		return "1080p", true
	case 0xF1, 0xF2, 0xF3, 0xF4:
		return "180p", true
	default:
		return "", false
	}
}

// ParseExtended は、EDCB が 1 本にまとめた詳細 (拡張形式イベント記述子) を、項目名から本文への対応に戻す。
// EDCB は、項目ごとに「- 項目名」の行と本文の行を、CRLF で並べる。項目名と本文の行頭の「-…- 」には、
// 項目名の行と見分けられるよう、「-」を 1 つ足している (EDCB の EpgDataCap3/EpgDBUtil.cpp の EscapeHyphenSpaceAndAppend)。
// 本文の改行は、Mirakurun と同じく LF にする。最初の項目名より前の行は読み捨てる。
func ParseExtended(text string) map[string]string {
	result := map[string]string{}
	var key string
	var body []string
	started := false
	flush := func() {
		if started {
			result[key] = strings.Join(body, "\n")
		}
	}
	// 項目ごとに、本文の後に区切りの改行が 1 つ続く。最後の項目の区切りを外せば、ほかの項目の区切りは、
	// 次の項目名の行の手前の改行になる。本文がもともと改行で終わるときは、その改行を残す (Mirakurun と同じ)。
	text = strings.TrimSuffix(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for line := range strings.SplitSeq(text, "\n") {
		if strings.HasPrefix(line, "- ") {
			flush()
			key, body, started = unescapeHyphen(line[2:]), nil, true

			continue
		}
		body = append(body, unescapeHyphen(line))
	}
	flush()

	return result
}

// unescapeHyphen は、行頭の「-」が 2 つ以上続いて空白が来るとき、足した「-」を 1 つ外す。
func unescapeHyphen(line string) string {
	n := len(line) - len(strings.TrimLeft(line, "-"))
	if n >= 2 && len(line) > n && line[n] == ' ' {
		return line[1:]
	}

	return line
}

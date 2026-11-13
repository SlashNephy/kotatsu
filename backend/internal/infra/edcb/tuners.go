package edcb

import (
	"cmp"
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
)

// Tuners は、EpgTimerSrv のチューナーの一覧を返す (#1532、設計書 3.6)。
//   - 一覧は ENUM_TUNER_RESERVE から作る。チューナーが足りない予約を集めた行 (UnassignedTunerID) は除く。
//   - 受信できる種別は、チャンネルの設定 (ChSet5.txt) のネットワーク ID から決め、すべてのチューナーで同じとみなす。
//     BonDriver ごとに受信できるチャンネル (ChSet4) を返すコマンドはない。
//   - 使っているかどうかは、ENUM_TUNER_PROCESS (起動しているチューナーのプロセス) から決める。使っている人の概念はない。
//   - Kotatsu が使えるのは、設定で並べた BonDriver のチューナーだけとする。NetworkTV は、EpgTimerSrv.ini の [TVTEST] に
//     並べた BonDriver しか使わないためである。設定がなければ、すべてのチューナーを使えるとみなす。
func (s *TunerServer) Tuners(ctx context.Context) ([]*tunerserver.Tuner, error) {
	reserves, err := s.client.TunerReserves(ctx)
	if err != nil {
		return nil, err
	}
	processes, err := s.client.TunerProcesses(ctx)
	if err != nil {
		return nil, err
	}
	entries, err := s.channelEntries(ctx)
	if err != nil {
		return nil, err
	}
	using := map[uint32]bool{}
	for _, process := range processes {
		using[process.TunerID] = true
	}
	types := channelTypes(entries)
	tuners := []*tunerserver.Tuner{}
	for _, reserve := range reserves {
		if reserve.TunerID == UnassignedTunerID {
			continue
		}
		tuners = append(tuners, &tunerserver.Tuner{
			Index:       int(reserve.TunerID),
			Name:        reserve.TunerName,
			Types:       slices.Clone(types),
			Users:       []*tunerserver.TunerUser{},
			IsAvailable: s.usable(reserve.TunerName),
			IsRemote:    false,
			IsFree:      !using[reserve.TunerID],
			IsUsing:     using[reserve.TunerID],
			IsFault:     false,
		})
	}
	slices.SortFunc(tuners, func(a, b *tunerserver.Tuner) int { return cmp.Compare(a.Index, b.Index) })

	return tuners, nil
}

// usable は、Kotatsu が BonDriver name のチューナーを使えるかを返す。Windows のファイル名なので、大文字と小文字を区別しない。
func (s *TunerServer) usable(name string) bool {
	return len(s.bonDrivers) == 0 || s.bonDrivers[strings.ToLower(name)]
}

// channelTypes は、チャンネルの設定のサービスの種別を、GR、BS、CS、BS4K、SKY の順に並べて返す。
func channelTypes(entries []*ChannelEntry) []string {
	seen := map[string]bool{}
	for _, entry := range entries {
		seen[ChannelType(entry.ONID)] = true
	}
	types := []string{}
	for _, t := range []string{"GR", "BS", "CS", "BS4K", "SKY"} {
		if seen[t] {
			types = append(types, t)
		}
	}

	return types
}

// ServerDate は、EDCB の時計を読めないので、常に tunerserver.ErrClockUnavailable を返す (#1532)。
// 時計のずれは、録画の TS の TOT との比較 (設計書 3.7) だけで見る。
func (s *TunerServer) ServerDate(context.Context) (time.Time, error) {
	return time.Time{}, tunerserver.ErrClockUnavailable
}

// Identify は、種類として EDCB を返す。EpgTimerSrv は版を名乗らないので、版は空にする。
func (s *TunerServer) Identify(context.Context) (*tunerserver.Identity, error) {
	return &tunerserver.Identity{Kind: tunerserver.KindEdcb, Version: ""}, nil
}

// Channels は、チャンネルの設定 (ChSet5.txt) のサービスを、TS ごとにまとめて返す (#1533)。
// EDCB は物理チャンネルを返さないので、チャンネルには TSID を入れる。名前は、TS の最初のサービスのネットワーク名にする。
func (s *TunerServer) Channels(ctx context.Context) ([]*tunerserver.Channel, error) {
	services, err := s.Services(ctx)
	if err != nil {
		return nil, err
	}
	entries, err := s.channelEntries(ctx)
	if err != nil {
		return nil, err
	}
	networkNames := map[tunerserver.ChannelRef]string{}
	for _, entry := range entries {
		ref := tunerserver.ChannelRef{Type: ChannelType(entry.ONID), Channel: strconv.Itoa(int(entry.TSID))}
		if _, ok := networkNames[ref]; !ok {
			networkNames[ref] = entry.NetworkName
		}
	}
	channels := []*tunerserver.Channel{}
	index := map[tunerserver.ChannelRef]*tunerserver.Channel{}
	for _, service := range services {
		ref := *service.Channel
		channel, ok := index[ref]
		if !ok {
			channel = &tunerserver.Channel{Type: ref.Type, Channel: ref.Channel, Name: s.text.Replace(networkNames[ref]), Services: []*tunerserver.Service{}}
			index[ref] = channel
			channels = append(channels, channel)
		}
		channel.Services = append(channel.Services, service)
	}

	return channels, nil
}

// ServiceLogo は、EDCB のロゴをまだ読まないので、常に tunerserver.ErrLogoUnavailable を返す。
// tenban は、録画とタイムシフトの TS から集めたロゴを返す (#1308)。EDCB のロゴは #1537 で扱う。
func (s *TunerServer) ServiceLogo(context.Context, int64) ([]byte, error) {
	return nil, tunerserver.ErrLogoUnavailable
}

var _ tunerserver.Client = new(TunerServer)

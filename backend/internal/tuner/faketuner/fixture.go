package faketuner

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"time"
)

const (
	// 地上波の network_id は 0x7FE0 から始まる。ここでは実在しない値を使う。
	grNetworkID = 0x7FE0
	// BS の network_id は 4 と決まっている。
	bsNetworkID = 4

	programDuration = 30 * time.Minute
	programsBefore  = 2
	programsAfter   = 12
)

// DefaultOptions は、地上波 3 チャンネルと BS 1 チャンネル、地上波のチューナー 2 本と BS/CS のチューナー 1 本の構成を返す。
// 地上波のチャンネルをチューナーより多くしているので、チューナーの奪い合いを再現できる。
// 番組は、各サービスに 30 分の番組を now の前後に並べる。サービス名と番組名は架空のものである。
//
// ロゴは、GR 27 の「こたつテスト1」と BS に持たせる。GR 26 は、ロゴの番号だけあって、画像はまだ受信していない状況にする。
//
// testdata/ts/test.ts の SDT と EIT は、GR 27 の「こたつテスト1」(service_id 0x0400) に揃えている。
func DefaultOptions(now time.Time, ts []byte) *Options {
	channels := []*Channel{
		{
			Type:    "GR",
			Channel: "27",
			Name:    "こたつテスト1",
			Services: []*Service{
				withLogo(withRemoteControlKey(newService(grNetworkID, 0x0400, "こたつテスト1"), 1), 0, color.RGBA{R: 0xE8, G: 0x5A, B: 0x2C, A: 0xFF}),
				newService(grNetworkID, 0x0401, "こたつテスト1 サブ"),
			},
		},
		{
			Type:     "GR",
			Channel:  "26",
			Name:     "こたつテスト2",
			Services: []*Service{withLogo(newService(grNetworkID+1, 0x0408, "こたつテスト2"), 1, nil)},
		},
		{
			Type:     "GR",
			Channel:  "25",
			Name:     "こたつテスト3",
			Services: []*Service{newService(grNetworkID+2, 0x0410, "こたつテスト3")},
		},
		{
			Type:     "BS",
			Channel:  "BS15_0",
			Name:     "こたつ BS",
			Services: []*Service{withLogo(newService(bsNetworkID, 101, "こたつ BS"), 0, color.RGBA{R: 0x2C, G: 0x6E, B: 0xE8, A: 0xFF})},
		},
	}

	programs := []*Program{}
	start := now.Truncate(programDuration).Add(-programsBefore * programDuration)
	for _, channel := range channels {
		for _, service := range channel.Services {
			for i := range programsBefore + programsAfter {
				eventID := i + 1
				programs = append(programs, &Program{
					ID:           ProgramID(service.NetworkID, service.ServiceID, eventID),
					EventID:      eventID,
					ServiceID:    service.ServiceID,
					NetworkID:    service.NetworkID,
					StartAt:      start.Add(time.Duration(i) * programDuration).UnixMilli(),
					Duration:     programDuration.Milliseconds(),
					IsFree:       true,
					Name:         fmt.Sprintf("%s テスト番組 %d", service.Name, eventID),
					Description:  "偽のチューナーサーバーが合成した番組です。",
					RelatedItems: nil,
					// 実機で最も多い、日本語のステレオ 1 つにする。
					Audios: []*Audio{{ComponentType: 3, ComponentTag: new(16), IsMain: true, SamplingRate: 48000, Langs: []string{"jpn"}}},
					// testdata/ts/test.ts に合わせ、MPEG-2 の映像にする。
					Video: &Video{Type: "mpeg2", Resolution: "1080i"},
				})
			}
		}
	}

	return &Options{
		Channels: channels,
		Programs: programs,
		Tuners: []*TunerConfig{
			{Name: "GR0", Types: []string{"GR"}},
			{Name: "GR1", Types: []string{"GR"}},
			{Name: "BSCS0", Types: []string{"BS", "CS"}},
		},
		TS:    ts,
		Clock: nil,
	}
}

func withRemoteControlKey(service *Service, key int) *Service {
	service.RemoteControlKeyID = key

	return service
}

// withLogo は、サービスにロゴの番号を持たせる。fill が nil でなければ、その色で塗った 64×36 の PNG を、ロゴの画像として持たせる。
func withLogo(service *Service, logoID int, fill color.Color) *Service {
	service.LogoID = new(logoID)
	if fill != nil {
		service.Logo = solidPNG(fill)
	}

	return service
}

// solidPNG は、地上波のロゴと同じ 64×36 の、1 色で塗った PNG を返す。
func solidPNG(fill color.Color) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 64, 36))
	for y := range 36 {
		for x := range 64 {
			img.Set(x, y, fill)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		// メモリ上の画像の符号化は失敗しない。
		panic(err)
	}

	return buf.Bytes()
}

func newService(networkID, serviceID int, name string) *Service {
	return &Service{
		ID:        ServiceItemID(networkID, serviceID),
		ServiceID: serviceID,
		NetworkID: networkID,
		Name:      name,
		// 0x01 はデジタル TV サービス。
		Type:               0x01,
		RemoteControlKeyID: 0,
		LogoID:             nil,
		Channel:            nil,
		Logo:               nil,
	}
}

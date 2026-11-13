package edcb

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
)

const (
	// logoMapFile は、EDCB がサービスとロゴの ID の対応を書くファイル (BonCtrl/BonCtrl.cpp の LOGO_SAVE_FOLDER L".ini")。
	logoMapFile = "LogoData.ini"
	// logoMapTTL は、ロゴの ID の対応を読み直す間隔。EDCB は、放送の CDT と SDT からロゴを集めたときに書き足すので、
	// 頻繁には変わらない。
	logoMapTTL = 10 * time.Minute
)

// logoTypePreference は、返すロゴの種類の順。録画の TS から集めたロゴ (#1308) と同じく、HD の大きいロゴ (64×36) を先にする。
func logoTypePreference() []int {
	return []int{5, 2, 4, 1, 3, 0}
}

// logoMap は、読んだロゴの ID の対応と、読んだ時刻。
type logoMap struct {
	mu       sync.Mutex
	ids      map[ServiceKey]int
	loadedAt time.Time
}

// ServiceLogo は、EDCB が集めたサービスのロゴの PNG を返す (#1537、設計書 3.1)。
//   - サービスとロゴの ID の対応は、LogoData.ini の [LogoIDMap] から読む (キーは ONID と SID を 4 桁ずつの 16 進、値はロゴの ID)。
//   - 画像は、LogoData\{ONID}_{ロゴの ID}_000_{種類}.png を FILE_COPY2 で取る (BonCtrl/BonCtrl.cpp)。
//   - EDCB がロゴを集めていなければ tunerserver.ErrLogoUnavailable を返す。tenban は、録画とタイムシフトの TS から集めたロゴを返す。
//     EDCB は、ロゴを集める設定を有効にしていないと、LogoData.ini を書かない。
//
// TSID はロゴの ID の対応に含まれないので、サービス ID (ONID × 100000 + SID) から引ける。
func (s *TunerServer) ServiceLogo(ctx context.Context, id int64) ([]byte, error) {
	ids, err := s.logoIDs(ctx)
	if err != nil {
		return nil, err
	}
	var service ServiceKey
	logoID, ok := -1, false
	for key, value := range ids {
		if serviceID(key) == id {
			service, logoID, ok = key, value, true

			break
		}
	}
	if !ok {
		return nil, fmt.Errorf("%w: EDCB has no logo for service %d", tunerserver.ErrLogoUnavailable, id)
	}
	names := []string{}
	for _, logoType := range logoTypePreference() {
		names = append(names, fmt.Sprintf("LogoData\\%04x_%03x_000_%02x.png", service.ONID, logoID, logoType))
	}
	files, err := s.client.Files(ctx, names)
	if err != nil {
		return nil, err
	}
	// FILE_COPY2 は、頼んだ順に返す。ないファイルは空になる。
	for _, file := range files {
		if len(file.Data) > 0 {
			return file.Data, nil
		}
	}

	return nil, fmt.Errorf("%w: EDCB has no logo image for service %d", tunerserver.ErrLogoUnavailable, id)
}

// logoIDs は、サービスとロゴの ID の対応を返す。logoMapTTL の間は、前に読んだものを使う。
func (s *TunerServer) logoIDs(ctx context.Context) (map[ServiceKey]int, error) {
	s.logos.mu.Lock()
	defer s.logos.mu.Unlock()
	if s.logos.ids != nil && time.Since(s.logos.loadedAt) < logoMapTTL {
		return s.logos.ids, nil
	}
	files, err := s.client.Files(ctx, []string{logoMapFile})
	if err != nil {
		return nil, err
	}
	data := []byte{}
	if len(files) > 0 {
		data = files[0].Data
	}
	s.logos.ids, s.logos.loadedAt = ParseLogoMap(data), time.Now()

	return s.logos.ids, nil
}

// ParseLogoMap は、LogoData.ini の [LogoIDMap] を読む。読めない行は読み飛ばす。
// キーは ONID と SID を 4 桁ずつの 16 進で並べたもの (「7FE00400」)、値はロゴの ID (10 進)。
func ParseLogoMap(data []byte) map[ServiceKey]int {
	ids := map[ServiceKey]int{}
	data = bytes.TrimPrefix(data, []byte(utf8BOM))
	inSection := false
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") {
			inSection = strings.EqualFold(line, "[LogoIDMap]")

			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !inSection || !ok || len(strings.TrimSpace(key)) != 8 {
			continue
		}
		key = strings.TrimSpace(key)
		onid, err1 := strconv.ParseUint(key[:4], 16, 16)
		sid, err2 := strconv.ParseUint(key[4:], 16, 16)
		logoID, err3 := strconv.Atoi(strings.TrimSpace(value))
		if err1 != nil || err2 != nil || err3 != nil || logoID < 0 || logoID > 0xFFFF {
			continue
		}
		ids[ServiceKey{ONID: uint16(onid), TSID: 0, SID: uint16(sid)}] = logoID
	}

	return ids
}

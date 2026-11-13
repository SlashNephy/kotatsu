package edcb

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/japanese"
)

// ChannelEntry は、EpgTimerSrv のチャンネルの設定 (ChSet5.txt) の 1 行 (CH_DATA5)。
// チャンネルスキャンで拾ったサービスがすべて並ぶ。ENUM_SERVICE は EPG を読み込んだサービスしか返さないので、
// サービスの一覧には、こちらを使う。
type ChannelEntry struct {
	ServiceKey

	Name        string
	NetworkName string
	Type        int
	// PartialReception は、部分受信 (ワンセグ) のサービスかどうか。
	PartialReception bool
	// CaptureEPG は、EPG を取得する対象かどうか。
	CaptureEPG bool
	// Search は、検索のときの既定の対象かどうか。
	Search bool
}

// utf8BOM は、UTF-8 のバイト順マーク。Linux の EDCB は、ChSet5.txt を BOM 付きの UTF-8 で書く。
const utf8BOM = "\xEF\xBB\xBF"

// ParseChannelSettings は、ChSet5.txt を読む。文字コードは、BOM 付きの UTF-8 か UTF-8 なら UTF-8、
// そうでなければ Shift_JIS (古い Windows の EDCB) とみなす。
// 列は、サービス名、ネットワーク名、ONID、TSID、SID、サービスの種類、部分受信、EPG の取得、検索の対象で、タブで区切る
// (EDCB の Common/ParseTextInstances.cpp の CParseChText5)。タブを含まない行は読み飛ばし、同じサービスは後の行を使う。
func ParseChannelSettings(data []byte) ([]*ChannelEntry, error) {
	data = bytes.TrimPrefix(data, []byte(utf8BOM))
	if !utf8.Valid(data) {
		decoded, err := japanese.ShiftJIS.NewDecoder().Bytes(data)
		if err != nil {
			return nil, fmt.Errorf("failed to decode ChSet5.txt: %w", err)
		}
		data = decoded
	}
	entries := []*ChannelEntry{}
	index := map[ServiceKey]int{}
	for number, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		if !strings.Contains(line, "\t") {
			continue
		}
		entry, err := parseChannelLine(line)
		if err != nil {
			return nil, fmt.Errorf("failed to read line %d of ChSet5.txt: %w", number+1, err)
		}
		if i, ok := index[entry.ServiceKey]; ok {
			entries[i] = entry

			continue
		}
		index[entry.ServiceKey] = len(entries)
		entries = append(entries, entry)
	}

	return entries, nil
}

func parseChannelLine(line string) (*ChannelEntry, error) {
	fields := strings.Split(line, "\t")
	// EDCB は、足りない列を 0 として読む。
	for len(fields) < 9 {
		fields = append(fields, "0")
	}
	ids := make([]uint16, 0, 3)
	for _, field := range fields[2:5] {
		n, err := strconv.ParseUint(strings.TrimSpace(field), 10, 16)
		if err != nil {
			return nil, fmt.Errorf("%w: %q is not a 16-bit number", ErrMalformed, field)
		}
		ids = append(ids, uint16(n))
	}
	flags := make([]int, 0, 4)
	for _, field := range fields[5:9] {
		n, err := strconv.Atoi(strings.TrimSpace(field))
		if err != nil {
			return nil, fmt.Errorf("%w: %q is not a number", ErrMalformed, field)
		}
		flags = append(flags, n)
	}

	return &ChannelEntry{
		ONID:             ids[0],
		TSID:             ids[1],
		SID:              ids[2],
		Name:             fields[0],
		NetworkName:      fields[1],
		Type:             flags[0],
		PartialReception: flags[1] != 0,
		CaptureEPG:       flags[2] != 0,
		Search:           flags[3] != 0,
	}, nil
}

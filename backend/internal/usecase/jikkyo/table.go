// Package jikkyo は、サービスとニコニコ実況のチャンネルの対応と、実況のコメントの取得を扱う (設計書 7.4)。
package jikkyo

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/SlashNephy/kotatsu/internal/domain"
)

//go:embed konomitv/jikkyo_channels.json
var konomiTVChannels []byte

const (
	// terrestrialNetworkID は、KonomiTV の対応表で、地上波のサービスに書かれているネットワーク ID。
	// 地上波のネットワーク ID は地域ごとに違うので、対応表では 15 にそろえ、サービス ID だけで引く。
	terrestrialNetworkID = 15
	// noChannel は、KonomiTV の対応表で、実況のチャンネルがないことを表す ID。
	noChannel = -1
)

// serviceKey は、ネットワーク ID とサービス ID の組。
type serviceKey struct {
	networkID int64
	serviceID int64
}

// Table は、サービスから実況のチャンネルの ID (jk1 など) を引く対応表。
type Table struct {
	// services は、BS と CS のサービスと、設定で足したサービスの対応。
	// 空の値は、実況のチャンネルがないと対応表に書かれていること。
	services map[serviceKey]string
	// terrestrial は、地上波のサービス ID からの対応。空の値の意味は services と同じ。
	terrestrial map[int64]string
}

// konomiTVChannel は、KonomiTV の対応表の 1 行。
type konomiTVChannel struct {
	JikkyoID  int    `json:"jikkyo_id"`
	NetworkID int64  `json:"network_id"`
	ServiceID string `json:"service_id"`
}

// NewTable は、同梱した KonomiTV の対応表に、overrides を重ねた対応表を返す。
// overrides のキーは「ネットワーク ID:サービス ID」、値は実況のチャンネルの ID (jk1 など)。
func NewTable(overrides map[string]string) (*Table, error) {
	var channels []konomiTVChannel
	if err := json.Unmarshal(konomiTVChannels, &channels); err != nil {
		return nil, fmt.Errorf("failed to parse the jikkyo channel table: %w", err)
	}
	t := &Table{services: map[serviceKey]string{}, terrestrial: map[int64]string{}}
	for _, channel := range channels {
		serviceID, err := strconv.ParseInt(channel.ServiceID, 0, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid service ID %q in the jikkyo channel table: %w", channel.ServiceID, err)
		}
		// 実況のチャンネルがないサービスも、サブチャンネルとして前のサービスに寄せないよう、空の値で残す。
		id := ""
		if channel.JikkyoID != noChannel {
			id = "jk" + strconv.Itoa(channel.JikkyoID)
		}
		if channel.NetworkID == terrestrialNetworkID {
			t.terrestrial[serviceID] = id
		} else {
			t.services[serviceKey{networkID: channel.NetworkID, serviceID: serviceID}] = id
		}
	}
	for key, id := range overrides {
		networkID, serviceID, ok := parseServiceKey(key)
		if !ok {
			return nil, fmt.Errorf("invalid jikkyo channel override %q: the key must be <network ID>:<service ID>", key)
		}
		if !strings.HasPrefix(id, "jk") {
			return nil, fmt.Errorf("invalid jikkyo channel override %q: the channel must be like jk1", key)
		}
		t.services[serviceKey{networkID: networkID, serviceID: serviceID}] = id
	}

	return t, nil
}

func parseServiceKey(key string) (int64, int64, bool) {
	network, service, ok := strings.Cut(key, ":")
	if !ok {
		return 0, 0, false
	}
	networkID, err := strconv.ParseInt(network, 0, 64)
	if err != nil {
		return 0, 0, false
	}
	serviceID, err := strconv.ParseInt(service, 0, 64)
	if err != nil {
		return 0, 0, false
	}

	return networkID, serviceID, true
}

// Lookup は、サービスの実況のチャンネルの ID を返す。対応するチャンネルがなければ false を返す。
// 地上波は、サービス ID だけで引く。サブチャンネルは対応表にないので、サービス ID が 1 つ前か 2 つ前のメインチャンネルに寄せる。
func (t *Table) Lookup(networkID, serviceID int64) (string, bool) {
	if id, ok := t.services[serviceKey{networkID: networkID, serviceID: serviceID}]; ok {
		return id, id != ""
	}
	if !domain.IsTerrestrialNetwork(networkID) {
		return "", false
	}
	for _, candidate := range []int64{serviceID, serviceID - 1, serviceID - 2} {
		if id, ok := t.terrestrial[candidate]; ok {
			return id, id != ""
		}
	}

	return "", false
}

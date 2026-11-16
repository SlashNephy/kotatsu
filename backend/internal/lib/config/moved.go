package config

import (
	"slices"
	"strings"

	"go.yaml.in/yaml/v4"
)

// movedSetting は、画面で設定するようになり、設定ファイルと環境変数から読まなくなった項目 (#1869、設計書 11.2)。
type movedSetting struct {
	// keys は、YAML のキーの道筋。
	keys []string
	// screen は、代わりに設定する管理者の画面のパス。
	screen string
}

// 移した項目を設定する、管理者の画面のパス。
const (
	retentionScreen    = "/admin/retention"
	integrationsScreen = "/admin/integrations"
	recordingScreen    = "/admin/recording"
	servicesScreen     = "/admin/services"
)

// movedSettings は、画面に移した項目を返す。書いてあっても起動は止めず、値を読まずに、ログで警告する。
// 設定ファイルを直さないまま更新しても、起動できるようにするためである。
func movedSettings() []movedSetting {
	return []movedSetting{
		{keys: []string{"audit", "retention_days"}, screen: retentionScreen},
		{keys: []string{"audit", "client_info_retention_days"}, screen: retentionScreen},
		{keys: []string{"auth", "hibp", "enabled"}, screen: integrationsScreen},
		{keys: []string{"web_push", "subject"}, screen: integrationsScreen},
		{keys: []string{"thumbnail", "disable_preview"}, screen: recordingScreen},
		{keys: []string{"power", "manage_sleep"}, screen: recordingScreen},
		{keys: []string{"captions", "threads"}, screen: recordingScreen},
		{keys: []string{"jikkyo", "channels"}, screen: servicesScreen},
	}
}

// IgnoredSetting は、画面で設定するようになったので、読まずに無視した項目 (#1869)。
type IgnoredSetting struct {
	// Key は、YAML のキー (audit.retention_days など)。
	Key string
	// Env は、渡されていた環境変数の名前 (KOTATSU_AUDIT_RETENTION_DAYS など)。設定ファイルに書いてあったなら空。
	Env string
	// Screen は、代わりに設定する管理者の画面のパス (/admin/retention など)。
	Screen string
}

// ignoredEnv は、画面に移した項目の環境変数のうち、渡されているものを返す。
func ignoredEnv(lookupEnv LookupEnv) []IgnoredSetting {
	ignored := []IgnoredSetting{}
	for _, moved := range movedSettings() {
		if name := envName(moved.keys); hasEnv(lookupEnv, name) {
			ignored = append(ignored, IgnoredSetting{Key: strings.Join(moved.keys, "."), Env: name, Screen: moved.screen})
		}
	}

	return ignored
}

// hasEnv は、環境変数 name が渡されているかどうかを返す。
func hasEnv(lookupEnv LookupEnv, name string) bool {
	_, ok := lookupEnv(name)

	return ok
}

// stripMovedSettings は、設定ファイルの YAML から、画面に移した項目を取り除き、取り除いた項目と、取り除いた後の YAML を返す。
// 取り除いて空になった節 (audit: など) も取り除く。未知のキーを拒む読み込みで、消した節が誤りにならないようにするためである。
func stripMovedSettings(data []byte) ([]byte, []IgnoredSetting, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, nil, err
	}
	ignored := []IgnoredSetting{}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return data, ignored, nil
	}
	for _, moved := range movedSettings() {
		if removeKey(doc.Content[0], moved.keys) {
			ignored = append(ignored, IgnoredSetting{Key: strings.Join(moved.keys, "."), Env: "", Screen: moved.screen})
		}
	}
	// 項目がなくても、空の節 (audit: だけの行) を取り除いているので、書き出し直す。
	stripped, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, nil, err
	}

	return stripped, ignored, nil
}

// removeKey は、対応のノード mapping から、keys の道筋の項目を取り除き、取り除いたかどうかを返す。
// 途中の節が、取り除いた後に空になったか、もとから空 (null) なら、その節も取り除く。
func removeKey(mapping *yaml.Node, keys []string) bool {
	index := slices.IndexFunc(pairKeys(mapping), func(key *yaml.Node) bool { return key.Value == keys[0] })
	if index < 0 {
		return false
	}
	value := mapping.Content[2*index+1]
	leaf := len(keys) == 1
	removed := leaf
	if !leaf && value.Kind == yaml.MappingNode {
		removed = removeKey(value, keys[1:])
	}
	if leaf || isEmpty(value) {
		mapping.Content = slices.Delete(mapping.Content, 2*index, 2*index+2)
	}

	return removed
}

// pairKeys は、対応のノードのキーを、並んだ順に返す。対応のノードは、キーと値を交互に持つ。
func pairKeys(mapping *yaml.Node) []*yaml.Node {
	keys := make([]*yaml.Node, 0, len(mapping.Content)/2)
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		keys = append(keys, mapping.Content[i])
	}

	return keys
}

// isEmpty は、ノードが空の対応か null かを返す。
func isEmpty(node *yaml.Node) bool {
	return (node.Kind == yaml.MappingNode && len(node.Content) == 0) || (node.Kind == yaml.ScalarNode && node.Tag == "!!null")
}

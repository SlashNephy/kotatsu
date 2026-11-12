package redact

import (
	"bytes"
	"cmp"
	"encoding/json"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const (
	// MinSecretLength は、伏せる秘密の値の最短の長さ (秘密の最短の長さ、設計書 付録 A)。
	// これより短い値は、ふつうの語や番組名の一部と区別が付かないので使わない。
	MinSecretLength = 8
	// SecretMark は、伏せた秘密の値の代わりに置く印。画面で伏せるもの (frontend の admin/logs/redact.ts) と同じ印にする。
	SecretMark = "[REDACTED_SECRET]"
	// truncationMark は、1 行の上限で切った行の末尾の印 (logring.Truncate と同じ)。
	truncationMark = "…"
	// minTailPrefix は、切った行の末尾に残った秘密の前半を伏せる、最短の長さ。これより短い前半は、秘密の手がかりにならない。
	minTailPrefix = 4
)

// Secrets は、秘密の値を伏せる。値は MinSecretLength 以上のものだけを使い、大文字と小文字を区別せずに当てる。
// 短い秘密が長い秘密の一部だと、短い方を先に消したときに長い方が半端に残るので、長いものから順に置き換える。
type Secrets struct {
	patterns []*regexp.Regexp
	// values は、伏せる値を小文字にして、長い順に並べたもの (エスケープした形を含む)。
	values []string
}

// NewSecrets は、values を伏せる Secrets を作る。同じ値は 1 つにまとめる。
func NewSecrets(values []string) *Secrets {
	unique := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		// 印そのものに含まれる値は、伏せた後の印を崩すので使わない。
		if len(value) < MinSecretLength || strings.Contains(strings.ToLower(SecretMark), strings.ToLower(value)) {
			continue
		}
		// ログの整形でエスケープされた形 (JSON、HTML をエスケープする JSON、logfmt の引用符) でも出るので、どの形も探す。
		for _, form := range escapedForms(strings.ToLower(value)) {
			unique[form] = struct{}{}
		}
	}
	sorted := make([]string, 0, len(unique))
	for value := range unique {
		sorted = append(sorted, value)
	}
	slices.SortFunc(sorted, func(a, b string) int {
		return cmp.Or(cmp.Compare(len(b), len(a)), strings.Compare(a, b))
	})
	patterns := make([]*regexp.Regexp, 0, len(sorted))
	for _, value := range sorted {
		patterns = append(patterns, regexp.MustCompile("(?i)"+regexp.QuoteMeta(value)))
	}

	return &Secrets{patterns: patterns, values: sorted}
}

// escapedForms は、value と、ログの整形でエスケープされた value の形を返す。
// slog の JSON は " と \ と制御文字を、encoding/json はさらに < > & を \u003c の形に、logfmt は strconv.Quote の形にする。
func escapedForms(value string) []string {
	forms := []string{value}
	var plain bytes.Buffer
	encoder := json.NewEncoder(&plain)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err == nil {
		forms = append(forms, unquote(strings.TrimSuffix(plain.String(), "\n")))
	}
	if html, err := json.Marshal(value); err == nil {
		forms = append(forms, unquote(string(html)))
	}

	return append(forms, unquote(strconv.Quote(value)))
}

// unquote は、両端の引用符を除く。
func unquote(quoted string) string {
	return strings.TrimSuffix(strings.TrimPrefix(quoted, `"`), `"`)
}

// Len は、伏せる値の数を返す。
func (s *Secrets) Len() int {
	return len(s.patterns)
}

// Apply は、text の中の秘密の値を SecretMark にする。
// 1 行の上限で切った行 (末尾が「…」) は、切り口にまたがった秘密の前半が末尾に残るので、それも伏せる。
func (s *Secrets) Apply(text string) string {
	// 行の数 × 秘密の数だけ正規表現をかけると遅い (8000 行の 2 つの出どころと 50 個の秘密で約 1.8 秒) ので、
	// 小文字にした行に値が含まれるときだけ置き換える。
	lower := strings.ToLower(text)
	for i, pattern := range s.patterns {
		if !strings.Contains(lower, s.values[i]) {
			continue
		}
		text = pattern.ReplaceAllLiteralString(text, SecretMark)
		lower = strings.ToLower(text)
	}
	body, truncated := strings.CutSuffix(text, truncationMark)
	if !truncated {
		return text
	}
	for _, value := range s.values {
		for length := len(value) - 1; length >= minTailPrefix; length-- {
			if len(body) >= length && strings.EqualFold(body[len(body)-length:], value[:length]) {
				return body[:len(body)-length] + SecretMark + truncationMark
			}
		}
	}

	return text
}

// URLSecrets は、URL の文字列 raw のうち、秘密として伏せる部分を返す。
// 資格情報 (ユーザー:パスワード) は、そのままの形と、パーセントエンコードを解いた形の両方を返す。
// whole なら、URL の全体も返す (Webhook の URL のように、URL そのものが秘密になるもの)。
func URLSecrets(raw string, whole bool) []string {
	secrets := []string{}
	if raw == "" {
		return secrets
	}
	if whole {
		secrets = append(secrets, raw)
	}
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return secrets
	}
	if userinfo, _, ok := strings.Cut(strings.TrimPrefix(raw, u.Scheme+"://"), "@"); ok {
		secrets = append(secrets, userinfo)
	}
	if password, ok := u.User.Password(); ok {
		secrets = append(secrets, password, url.QueryEscape(password), url.PathEscape(password))
	}

	return secrets
}

// HeaderSecrets は、「名前=値」をカンマでつないだヘッダーの値 (OTEL_EXPORTER_OTLP_HEADERS の形) から、値を返す。
// 名前と値をつないだものも、全体も返す。
func HeaderSecrets(headers string) []string {
	secrets := []string{}
	if headers == "" {
		return secrets
	}
	secrets = append(secrets, headers)
	for pair := range strings.SplitSeq(headers, ",") {
		_, value, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		secrets = append(secrets, strings.TrimSpace(pair), value)
		if decoded, err := url.QueryUnescape(value); err == nil {
			secrets = append(secrets, decoded)
		}
	}

	return secrets
}

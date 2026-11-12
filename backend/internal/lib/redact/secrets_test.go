package redact_test

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/lib/redact"
)

func TestSecrets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		secrets []string
		text    string
		want    string
	}{
		{
			name:    "秘密の値を伏せる",
			secrets: []string{"s3cr3t-client-value"},
			text:    `oidc failed {"client_secret":"s3cr3t-client-value"}`,
			want:    `oidc failed {"client_secret":"[REDACTED_SECRET]"}`,
		},
		{
			name:    "大文字と小文字を区別しない",
			secrets: []string{"AbCdEfGhIjKl"},
			text:    "token abcdefghijkl and ABCDEFGHIJKL",
			want:    "token [REDACTED_SECRET] and [REDACTED_SECRET]",
		},
		{
			// 短い方を先に消すと、長い方の残り (-extra) が出てしまう。
			name:    "長いものから順に置き換える",
			secrets: []string{"password1", "password1-extra"},
			text:    "url password1-extra and password1",
			want:    "url [REDACTED_SECRET] and [REDACTED_SECRET]",
		},
		{
			name:    "最短の長さに満たない値は使わない",
			secrets: []string{"abc1234"},
			text:    "abc1234 is a normal word",
			want:    "abc1234 is a normal word",
		},
		{
			name:    "空の値は使わない",
			secrets: []string{"", "   "},
			text:    "nothing to hide",
			want:    "nothing to hide",
		},
		{
			name:    "番組名や ID は残す",
			secrets: []string{"webhook-secret-xyz"},
			text:    "recording started title=ニュース7 program_id=327360102400123",
			want:    "recording started title=ニュース7 program_id=327360102400123",
		},
		{
			name:    "印の一部になる値は使わず、2 回かけても変わらない",
			secrets: []string{"REDACTED", "_SECRET]", "hunter2-hunter2"},
			text:    "[REDACTED_SECRET] hunter2-hunter2",
			want:    "[REDACTED_SECRET] [REDACTED_SECRET]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			secrets := redact.NewSecrets(tt.secrets)
			got := secrets.Apply(tt.text)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, got, secrets.Apply(got))
		})
	}
}

func TestURLSecrets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		raw   string
		whole bool
		want  []string
	}{
		{name: "空", raw: "", whole: true, want: []string{}},
		{name: "資格のない URL は何も返さない", raw: "http://mirakurun:40772/", whole: false, want: []string{}},
		{
			name: "資格は、そのままと解いた形の両方",
			raw:  (&url.URL{Scheme: "postgres", User: url.UserPassword("kotatsu", "p@ss/word"), Host: "db:5432", Path: "/kotatsu"}).String(), whole: false,
			want: []string{"kotatsu:p%40ss%2Fword", "p@ss/word", "p%40ss%2Fword", "p@ss%2Fword"},
		},
		{
			name: "URL の全体も返す",
			raw:  "https://discord.com/api/webhooks/1/token", whole: true,
			want: []string{"https://discord.com/api/webhooks/1/token"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, redact.URLSecrets(tt.raw, tt.whole))
		})
	}
}

func TestHeaderSecrets(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{}, redact.HeaderSecrets(""))
	assert.Equal(t,
		[]string{
			"Mackerel-Api-Key=abc%3Ddef,X-Other=zzz",
			"Mackerel-Api-Key=abc%3Ddef", "abc%3Ddef", "abc=def",
			"X-Other=zzz", "zzz", "zzz",
		},
		redact.HeaderSecrets("Mackerel-Api-Key=abc%3Ddef,X-Other=zzz"))
}

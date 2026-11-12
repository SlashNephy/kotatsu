package redact_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/lib/redact"
)

// TestSecretsEscaped は、ログの整形でエスケープされた形の秘密の値も伏せることを確かめる。
// slog の JSON は " と \ と制御文字を、encoding/json はさらに < > & を < の形にする。logfmt は strconv.Quote の形にする。
func TestSecretsEscaped(t *testing.T) {
	t.Parallel()

	const value = "p\"a\\s<s>&w\nord"
	tests := []struct {
		name string
		text string
	}{
		{name: "slog の JSON (HTML はエスケープしない)", text: `{"password":"p\"a\\s<s>&w\nord"}`},
		{name: "encoding/json (HTML もエスケープする)", text: `{"password":"p\"a\\s\u003cs\u003e\u0026w\nord"}`},
		{name: "logfmt (strconv.Quote)", text: `password="p\"a\\s<s>&w\nord"`},
		{name: "そのままの形", text: "password=" + value},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := redact.NewSecrets([]string{value}).Apply(tt.text)
			assert.Contains(t, got, redact.SecretMark)
			for _, part := range []string{"<s>", `\u003cs`, `w\nord`, "w\nord"} {
				assert.NotContains(t, got, part)
			}
		})
	}
}

// TestSecretsTruncated は、1 行の上限で切った行の末尾に、秘密の値の前半が残らないことを確かめる。
func TestSecretsTruncated(t *testing.T) {
	t.Parallel()

	secrets := redact.NewSecrets([]string{"s3cr3t-webhook-token"})
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "切り口にまたがる秘密の前半を伏せる", text: "url=https://x.test/s3cr3t-web…", want: "url=https://x.test/" + redact.SecretMark + "…"},
		{name: "大文字と小文字を区別しない", text: "token S3CR3T-W…", want: "token " + redact.SecretMark + "…"},
		{name: "短すぎる前半は残す", text: "value s3c…", want: "value s3c…"},
		{name: "切っていない行の末尾は触らない", text: "url=s3cr3t-web", want: "url=s3cr3t-web"},
		{name: "切った行でも、秘密の前半でなければ触らない", text: "hello world…", want: "hello world…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := secrets.Apply(tt.text)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, got, secrets.Apply(got))
		})
	}
}

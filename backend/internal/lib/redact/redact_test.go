package redact_test

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/lib/redact"
)

func TestURLError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "パスのトークンを除く",
			err:  &url.Error{Op: "Post", URL: "https://discord.com/api/webhooks/123/secret-token", Err: context.DeadlineExceeded},
			want: `Post "https://discord.com": context deadline exceeded`,
		},
		{
			name: "クエリとユーザー情報を除く",
			err: &url.Error{Op: "Get", URL: (&url.URL{
				Scheme: "https", User: url.UserPassword("user", "pass"), Host: "example.com:8443", Path: "/x", RawQuery: "token=secret",
			}).String(), Err: context.Canceled},
			want: `Get "https://example.com:8443": context canceled`,
		},
		{
			name: "読めない URL は空にする",
			err:  &url.Error{Op: "parse", URL: "mysql://user:pa/ss@host:3306/db", Err: errors.New("invalid port")},
			want: `parse "": invalid port`,
		},
		{
			name: "*url.Error がなければそのまま",
			err:  errors.New("something failed"),
			want: "something failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.EqualError(t, redact.URLError(tt.err), tt.want)
		})
	}
}

// TestURLErrorKeepsCause は、URL を除いても、原因を errors.Is で確かめられることを確かめる。
func TestURLErrorKeepsCause(t *testing.T) {
	t.Parallel()

	err := redact.URLError(&url.Error{Op: "Post", URL: "https://example.com/secret", Err: context.DeadlineExceeded})

	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/domain"
)

func TestCommentSourceAccessFor(t *testing.T) {
	t.Parallel()

	const other = "other"
	tests := []struct {
		name   string
		access domain.CommentSourceAccess
		source string
		want   bool
	}{
		{name: "全体とユーザーの両方でオプトインしていれば使える", access: domain.CommentSourceAccess{System: []string{other}, User: []string{other}}, source: other, want: true},
		{name: "全体でオプトインしていなければ使えない", access: domain.CommentSourceAccess{System: []string{}, User: []string{other}}, source: other, want: false},
		{name: "ユーザーがオプトインしていなければ使えない", access: domain.CommentSourceAccess{System: []string{other}, User: []string{}}, source: other, want: false},
		{name: "別のソースのオプトインは効かない", access: domain.CommentSourceAccess{System: []string{other}, User: []string{other}}, source: domain.CommentSourceNXJikkyo, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.access.For(tt.source).Allowed())
		})
	}
}

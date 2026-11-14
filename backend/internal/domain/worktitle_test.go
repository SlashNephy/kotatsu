package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/domain"
)

func TestWorkTitle(t *testing.T) {
	t.Parallel()

	const work = "作品名"

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain title with episode", in: "豊臣兄弟! (7)", want: "豊臣兄弟!"},
		{name: "enclosed marks and episode", in: "🈑豊臣兄弟!#7「サブタイトル」🈞", want: "豊臣兄弟!"},
		{name: "episode in kanji", in: "作品名 第七話", want: work},
		{name: "slot name with quoted title", in: "土曜ドラマ「作品名」 第3話", want: work},
		{name: "slot name with full-width number", in: "ドラマ２４「作品名」 第4話", want: work},
		{name: "slot name with space", in: "日曜劇場 作品名 #5", want: work},
		{name: "drama prefix", in: "ドラマ「作品名」 第2話", want: work},
		{name: "morning drama", in: "連続テレビ小説 作品名 (12)", want: work},
		{name: "movie slot", in: "金曜ロードショー 映画のタイトル", want: "映画のタイトル"},
		// 話数のない番組名でも、枠の名前の後のかぎ括弧を作品名とする。
		{name: "movie with quoted title", in: "映画「はんてんの約束」", want: "はんてんの約束"},
		{name: "slot name and quote without episode", in: "土曜ドラマ「作品名」", want: work},
		{name: "serial drama", in: "連続ドラマ「縁側の午後」 第12話", want: "縁側の午後"},
		{name: "leading mark before slot name", in: "【新】土曜ドラマ「作品名」#1", want: work},
		{name: "quoted title with suffix", in: "金曜ドラマ「作品名」SP", want: work},
		// 枠の名前に似た作品名は、後ろに空白もかぎ括弧もないので削らない。
		{name: "title starting like a slot name", in: "ドラマチックな恋 #1", want: "ドラマチックな恋"},
		// 枠の名前だけなら、そのまま返す。
		{name: "slot name only", in: "土曜ドラマ", want: "土曜ドラマ"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, domain.WorkTitle(tt.in), tt.name)
	}
}

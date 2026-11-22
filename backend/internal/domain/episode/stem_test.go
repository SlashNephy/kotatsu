package episode_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/domain/episode"
)

func TestStem(t *testing.T) {
	t.Parallel()

	const harbor = "港町の灯"

	tests := []struct {
		name  string
		title string
		want  string
	}{
		{name: "作品名と話数の間の語を除いた作品名を返す", title: "港町の灯　二つの影　＃０２🈑🈓", want: harbor},
		{name: "作品名に空白があっても、最後の語だけを除く", title: "港町の　灯　二つの影　＃０２", want: harbor},
		{name: "話数の前に語が 1 つしかなければ空", title: "港町の灯　＃０５🈞", want: ""},
		{name: "話数の後ろに副題があれば空", title: "港町の灯　二つの影　＃０２　嵐の夜", want: ""},
		{name: "話数がなければ空", title: "港町の灯　二つの影", want: ""},
		{name: "最後の語が数字 (期) なら空", title: "港町の灯　４　＃３", want: ""},
		{name: "最後の語がシーズンの表記なら空", title: "港町の灯　Season2　＃３", want: ""},
		{name: "最後の語がシーズンの略記なら空", title: "港町の灯　S3　＃３", want: ""},
		{name: "最後の語がローマ数字なら空", title: "港町の灯　II　＃３", want: ""},
		{name: "空白で区切ったシーズンの表記なら空", title: "港町の灯　2nd Season　＃３", want: ""},
		{name: "編を読んだ番組名は空", title: "港町の灯　霧の港編　＃３", want: ""},
		{name: "期を読んだ番組名は空", title: "港町の灯　第2期　＃３", want: ""},
		{name: "先頭の枠の名前は作品名に含めない", title: "<深夜アニメ枠>港町の灯　二つの影　＃０２", want: harbor},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, episode.Stem(tt.title))
		})
	}
}

func TestStems(t *testing.T) {
	t.Parallel()

	const harbor = "港町の灯"

	tests := []struct {
		name  string
		title string
		want  []string
	}{
		{name: "副題が 1 語なら Stem と同じ 1 つ", title: "港町の灯　二つの影　＃０２🈑🈓", want: []string{harbor}},
		{name: "副題に空白が入れば、長い順にたどる", title: "港町の灯　嵐の夜　その二　＃０５", want: []string{harbor + "嵐の夜", harbor}},
		{name: "期を表す語の手前で止める", title: "灯台守　４　嵐の夜　＃５", want: []string{"灯台守4"}},
		{name: "話数の前に語が 1 つしかなければ空", title: "港町の灯　＃０５", want: []string{}},
		{name: "話数の後ろに副題があれば空", title: "港町の灯　二つの影　＃０２「霧笛」", want: []string{}},
		{name: "編を読んだ番組名は空", title: "港町の灯　霧の港編　＃０２", want: []string{}},
		{name: "先頭の枠の名前は作品名に含めない", title: "<深夜アニメ枠>港町の灯　二つの影　＃０２", want: []string{harbor}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, episode.Stems(tt.title))
		})
	}
}

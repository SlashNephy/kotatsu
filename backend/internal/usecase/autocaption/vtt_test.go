package autocaption_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/autocaption"
)

func TestParseVTT(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		vtt     string
		want    []autocaption.Cue
		wantErr bool
	}{
		{
			name: "whisper-cli の出力を読む",
			vtt:  "WEBVTT\n\n00:00:00.000 --> 00:00:09.620\n大手うどんチェーンの会見\n\n00:00:09.620 --> 00:01:15.760\n2人ともワクワク\n",
			want: []autocaption.Cue{
				{Start: 0, End: 9620 * time.Millisecond, Text: "大手うどんチェーンの会見"},
				{Start: 9620 * time.Millisecond, End: time.Minute + 15760*time.Millisecond, Text: "2人ともワクワク"},
			},
		},
		{
			name: "時の欄のない時刻と、1 時間を超える時刻を読む",
			vtt:  "WEBVTT\n\n01:02.345 --> 01:03.000\nあ\n\n01:00:00.000 --> 01:00:01.500\nい\n",
			want: []autocaption.Cue{
				{Start: time.Minute + 2345*time.Millisecond, End: time.Minute + 3*time.Second, Text: "あ"},
				{Start: time.Hour, End: time.Hour + 1500*time.Millisecond, Text: "い"},
			},
		},
		{
			name: "複数の行は改行でつなぎ、タグと前後の空白を取り除く",
			vtt:  "WEBVTT\r\n\r\n00:00:01.000 --> 00:00:02.000\r\n <c.yellow>1行目</c> \r\n<00:00:01.500>2行目\r\n",
			want: []autocaption.Cue{{Start: time.Second, End: 2 * time.Second, Text: "1行目\n2行目"}},
		},
		{
			name: "字幕の ID と時刻の後の設定、NOTE の塊を読み飛ばす",
			vtt:  "\uFEFFWEBVTT - whisper\n\nNOTE 自動生成\n\ncue-1\n00:00:01.000 --> 00:00:02.000 align:start position:10%\nう\n",
			want: []autocaption.Cue{{Start: time.Second, End: 2 * time.Second, Text: "う"}},
		},
		{
			name: "本文が空の字幕 (無音の区間) と、長さのない字幕は捨てる",
			vtt:  "WEBVTT\n\n00:01:00.000 --> 00:01:07.520\n\n00:01:08.000 --> 00:01:08.000\nえ\n\n00:01:09.000 --> 00:01:10.000\n <i></i> \n\n00:01:10.000 --> 00:01:11.000\nお\n",
			want: []autocaption.Cue{{Start: 70 * time.Second, End: 71 * time.Second, Text: "お"}},
		},
		{
			name: "字幕がなければ空",
			vtt:  "WEBVTT\n",
			want: []autocaption.Cue{},
		},
		{name: "WEBVTT で始まらなければ誤り", vtt: "1\n00:00:01,000 --> 00:00:02,000\nSRT\n", wantErr: true},
		{name: "空のファイルは誤り", vtt: "", wantErr: true},
		{name: "時刻が読めなければ誤り", vtt: "WEBVTT\n\n00:00:01 --> 00:00:02.000\nか\n", wantErr: true},
		{name: "分が 60 以上なら誤り", vtt: "WEBVTT\n\n00:60:00.000 --> 01:00:02.000\nか\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := autocaption.ParseVTT(strings.NewReader(tt.vtt))
			if tt.wantErr {
				require.Error(t, err)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

package autocaption_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/autocaption"
)

func ms(v int) time.Duration { return time.Duration(v) * time.Millisecond }

func TestFitCues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		cues   []autocaption.Cue
		speech []autocaption.Span
		want   []autocaption.Cue
	}{
		{
			// whisper が無音と音楽まで含めて 15 秒と 18 秒にした台詞は、読む時間 (0.5 + 文字数 × 0.15 秒) で切る。
			name: "VAD がなければ、長すぎる字幕を読む時間で切る",
			cues: []autocaption.Cue{
				{Start: ms(75540), End: ms(90340), Text: "大げさなんですよ"},
				{Start: ms(90340), End: ms(108780), Text: "スペシャルコースのことでしょ"},
			},
			speech: nil,
			want: []autocaption.Cue{
				{Start: ms(75540), End: ms(77240), Text: "大げさなんですよ"},
				{Start: ms(90340), End: ms(92940), Text: "スペシャルコースのことでしょ"},
			},
		},
		{
			name:   "VAD があれば、始まりを重なる最初の声の始まりまで遅らせ、終わりを声の終わりにする",
			cues:   []autocaption.Cue{{Start: ms(116080), End: ms(120080), Text: "やっぱり男が使えるのが確かなんだよな"}},
			speech: []autocaption.Span{{Start: ms(112990), End: ms(114760)}, {Start: ms(118170), End: ms(120100)}},
			want:   []autocaption.Cue{{Start: ms(118170), End: ms(120080), Text: "やっぱり男が使えるのが確かなんだよな"}},
		},
		{
			name:   "声の区間が読む時間より長ければ、声の終わりまで出す",
			cues:   []autocaption.Cue{{Start: ms(122340), End: ms(133040), Text: "私会を寄せません"}},
			speech: []autocaption.Span{{Start: ms(122300), End: ms(124290)}},
			want:   []autocaption.Cue{{Start: ms(122340), End: ms(124290), Text: "私会を寄せません"}},
		},
		{
			name:   "声の区間が長くても、7 秒より長くは出さない",
			cues:   []autocaption.Cue{{Start: 0, End: ms(20000), Text: "はい"}},
			speech: []autocaption.Span{{Start: 0, End: ms(20000)}},
			want:   []autocaption.Cue{{Start: 0, End: ms(7000), Text: "はい"}},
		},
		{
			name: "短すぎる字幕は、次の字幕の始まりを超えない範囲で 1 秒にのばす",
			cues: []autocaption.Cue{
				{Start: ms(116060), End: ms(116080), Text: "ごめんなさい"},
				{Start: ms(116500), End: ms(118000), Text: "次"},
				{Start: ms(130000), End: ms(130020), Text: "動くな"},
			},
			speech: nil,
			want: []autocaption.Cue{
				{Start: ms(116060), End: ms(116500), Text: "ごめんなさい"},
				{Start: ms(116500), End: ms(117500), Text: "次"},
				{Start: ms(130000), End: ms(131000), Text: "動くな"},
			},
		},
		{
			name:   "長さが 0 の字幕も、声の区間に合わせる",
			cues:   []autocaption.Cue{{Start: ms(67160), End: ms(67160), Text: "大事"}},
			speech: []autocaption.Span{{Start: ms(66000), End: ms(68000)}},
			want:   []autocaption.Cue{{Start: ms(67160), End: ms(68160), Text: "大事"}},
		},
		{
			name:   "字幕がなければ空",
			cues:   []autocaption.Cue{},
			speech: []autocaption.Span{{Start: 0, End: ms(1000)}},
			want:   []autocaption.Cue{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, autocaption.FitCues(tt.cues, tt.speech))
		})
	}
}

func TestParseSpeech(t *testing.T) {
	t.Parallel()

	out := strings.Join([]string{
		"whisper_vad_init_from_file_with_params_no_state: loading VAD model",
		"Detected 2 speech segments:",
		"Speech segment 0: start = 0.00, end = 368.00",
		"Speech segment 1: start = 416.00, end = 557.50",
		"",
	}, "\n")
	spans, err := autocaption.ParseSpeech(strings.NewReader(out))
	require.NoError(t, err)
	assert.Equal(t, []autocaption.Span{{Start: 0, End: ms(3680)}, {Start: ms(4160), End: ms(5575)}}, spans)
}

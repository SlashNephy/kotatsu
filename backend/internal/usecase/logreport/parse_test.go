package logreport_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/lib/logring"
	"github.com/SlashNephy/kotatsu/internal/usecase/logreport"
)

func TestParseTunerLog(t *testing.T) {
	t.Parallel()

	jst := time.FixedZone("JST", 9*60*60)
	at := time.Date(2026, 11, 12, 22, 50, 18, 941_000_000, jst).UnixMilli()
	tests := []struct {
		name string
		body string
		want []logreport.Line
	}{
		{
			name: "Mirakurun の形 (時刻 重さ: 本文)",
			body: "2026-11-12T22:50:18.941+09:00 info: listening on http://192.0.2.10:40772\n" +
				"2026-11-12T22:50:18.941+09:00 warning: tuner 1 is busy\n" +
				"2026-11-12T22:50:18.941+09:00 error: TunerDevice#0 failed\n" +
				"2026-11-12T22:50:18.941+09:00 debug: tick\n",
			want: []logreport.Line{
				{Time: at, Level: logring.LevelInfo, Inferred: false, Text: "listening on http://192.0.2.10:40772"},
				{Time: at, Level: logring.LevelWarn, Inferred: false, Text: "tuner 1 is busy"},
				{Time: at, Level: logring.LevelError, Inferred: false, Text: "TunerDevice#0 failed"},
				{Time: at, Level: logring.LevelInfo, Inferred: false, Text: "tick"},
			},
		},
		{
			name: "Mahiron の logfmt (時刻と重さを除いた残りを本文にする)",
			body: `time=2026-11-12T22:50:18.941+09:00 level=WARN msg="network EPG collection failed" networkId=32111` + "\n",
			want: []logreport.Line{
				{Time: at, Level: logring.LevelWarn, Inferred: false, Text: `msg="network EPG collection failed" networkId=32111`},
			},
		},
		{
			name: "JSON の形",
			body: `{"time":"2026-11-12T22:50:18.941+09:00","level":"ERROR","msg":"boom","code":3}` + "\n",
			want: []logreport.Line{
				{Time: at, Level: logring.LevelError, Inferred: false, Text: `boom {"code":3}`},
			},
		},
		{
			name: "重さが無ければ、本文の言葉から推し量る",
			body: "2026-11-12T22:50:18.941+09:00 チューナーを開けませんでした: エラー\n" +
				"2026-11-12T22:50:18.941+09:00 connection failed\n" +
				"2026-11-12T22:50:18.941+09:00 deprecated option, warning\n" +
				"2026-11-12T22:50:18.941+09:00 注意: 時計がずれています\n" +
				"2026-11-12T22:50:18.941+09:00 started\n",
			want: []logreport.Line{
				{Time: at, Level: logring.LevelError, Inferred: true, Text: "チューナーを開けませんでした: エラー"},
				{Time: at, Level: logring.LevelError, Inferred: true, Text: "connection failed"},
				{Time: at, Level: logring.LevelWarn, Inferred: true, Text: "deprecated option, warning"},
				{Time: at, Level: logring.LevelWarn, Inferred: true, Text: "注意: 時計がずれています"},
				{Time: at, Level: logring.LevelInfo, Inferred: true, Text: "started"},
			},
		},
		{
			// 「errors」の一部ではない語 (terror の類) で、error と推し量らない。
			name: "語の一部では推し量らない",
			body: "2026-11-12T22:50:18.941+09:00 terrorist movie scheduled\n",
			want: []logreport.Line{
				{Time: at, Level: logring.LevelInfo, Inferred: true, Text: "terrorist movie scheduled"},
			},
		},
		{
			name: "形式の違う行は捨てる",
			body: "    at Object.<anonymous> (/app/lib/index.js:1:1)\n" +
				"not a log line\n" +
				"\n" +
				`{"broken json` + "\n" +
				"time=not-a-time level=INFO msg=x\n",
			want: []logreport.Line{},
		},
		{
			name: "タイムゾーンの無い時刻は、表示のタイムゾーンで読む",
			body: "2026-11-12 22:50:18.941 info: local time\n",
			want: []logreport.Line{
				{Time: at, Level: logring.LevelInfo, Inferred: false, Text: "local time"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, logreport.ParseTunerLog(tt.body, jst))
		})
	}
}

func TestInferLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		text string
		want logring.Level
	}{
		{text: "Exception in thread", want: logring.LevelError},
		{text: "FATAL: out of memory", want: logring.LevelError},
		{text: "critical temperature", want: logring.LevelError},
		{text: "録画に失敗しました", want: logring.LevelError},
		{text: "例外が発生", want: logring.LevelError},
		{text: "接続できません", want: logring.LevelError},
		{text: "WARN low disk", want: logring.LevelWarn},
		{text: "警告: 空きが少ない", want: logring.LevelWarn},
		{text: "recording started", want: logring.LevelInfo},
		{text: "errorless run", want: logring.LevelInfo},
	}
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, logreport.InferLevel(tt.text))
		})
	}
}

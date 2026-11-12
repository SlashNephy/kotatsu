package logreport_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/SlashNephy/kotatsu/internal/lib/logring"
	"github.com/SlashNephy/kotatsu/internal/usecase/logreport"
)

// BenchmarkReport は、バッファがいっぱい (8000 行) で、チューナーサーバーからも 8000 行を受け取り、
// 50 個の秘密を伏せるときの、画面を開く 1 回の時間を測る。
func BenchmarkReport(b *testing.B) {
	buffer := logring.NewBuffer(logring.DefaultCapacity, logring.DefaultMaxLineLength)
	for i := range logring.DefaultCapacity {
		buffer.Add(logring.Line{Time: int64(i), Level: logring.LevelInfo, Text: fmt.Sprintf(
			`applied EPG events {"role":"neko (scheduler)","component":"epg","events":%d,"service_id":3273601024,"title":"テスト番組 %d"}`, i, i)})
	}
	var body strings.Builder
	for i := range logring.DefaultCapacity {
		fmt.Fprintf(&body, "time=2026-11-12T22:50:18.941+09:00 level=INFO msg=\"HTTP request completed\" path=/api/programs status=200 n=%d\n", i)
	}
	tuner := newTuner(b, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body.String())) })
	secrets := make([]string, 0, 50)
	for i := range 50 {
		secrets = append(secrets, fmt.Sprintf("secret-value-%02d-%s", i, strings.Repeat("x", 30)))
	}
	service := logreport.New(&logreport.Options{
		Buffer: buffer, Tuner: tuner, Hosts: nil, InstanceID: "", Roles: nil,
		Secrets: staticSecrets(secrets...), Timeout: 0, Limit: 0, MaxLineLength: 0, Location: nil,
	})
	b.ResetTimer()
	for b.Loop() {
		if _, err := service.Report(b.Context(), ""); err != nil {
			b.Fatal(err)
		}
	}
}

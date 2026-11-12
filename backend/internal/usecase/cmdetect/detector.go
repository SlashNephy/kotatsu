// Package cmdetect は、録画の中の CM の区間を見分ける (設計書 6.1、#479)。
//
// 見分け方は Detector で差し替えられる。いまは、FFmpeg の無音の検出とシーンの切り替わりに、TS の中の手掛かりを合わせる
// FFmpegDetector だけがある。精度が足りなければ、join_logo_scp のような外部のツールを使う Detector を足す。
// 見分けた区間は、Detector の名前と版と一緒に DB に残すので、Detector を直したら、版を上げて録り直さずに見分け直せる。
package cmdetect

import (
	"context"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/ffprogress"
)

// Section は、CM とみなした区間。Start と End は、ファイルの最初の映像からの時間。
type Section struct {
	Start time.Duration
	End   time.Duration
}

// Input は、Detector に渡す録画のファイル。
type Input struct {
	// Path は、TS のファイルのパス。
	Path string
	// ServiceID は、録画したサービスの ID (program_number)。TS の PAT と PMT から、そのサービスの映像と音声の PID を引くのに使う。
	// 録画は、代表画像と同じく、録画したサービスだけの TS であることを前提にする。
	ServiceID uint16
	// Progress は、FFmpeg の -progress のまとまり b と、それまでにファイルから FFmpeg に流したバイト数 read を受け取る (#1511)。
	// nil なら受け取らない。
	Progress func(b *ffprogress.Block, read int64)
}

// Detector は、録画のファイルから CM の区間を見分ける。
type Detector interface {
	// Name は、DB に残す、見分け方の名前。
	Name() string
	// Version は、見分け方の版。見分け方を変えたら上げる。
	Version() int32
	// Detect は、input のファイルの CM の区間を、始まりの順に返す。見つからなければ空のスライスを返す。
	Detect(ctx context.Context, input *Input) ([]Section, error)
}

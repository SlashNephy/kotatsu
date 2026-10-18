package kyusu

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/SlashNephy/kotatsu/internal/config"
)

const (
	// PlaylistName は、HLS のプレイリストのファイルの名前。
	PlaylistName = "playlist.m3u8"
	// segmentPattern は、HLS のセグメントのファイルの名前の形。
	segmentPattern = "segment%05d.ts"
	// segmentSeconds は、HLS のセグメントの長さ (秒)。
	segmentSeconds = 2
	// playlistSize は、ライブのプレイリストに載せるセグメントの数。古いセグメントは消す。
	playlistSize = 6
	// stopDelay は、プロセスを止めた後、標準入出力の受け渡しが終わるのを待つ時間。過ぎたら、パイプを閉じて待つのをやめる。
	stopDelay = 5 * time.Second
	// stderrLimit は、エラーの説明に添える、プロセスの標準エラー出力の末尾の大きさ。
	stderrLimit = 4096
)

// ErrToolNotFound は、FFmpeg か tsreadex が PATH にないことを表す。
var ErrToolNotFound = errors.New("FFmpeg or tsreadex is not found in PATH")

// Transcoder は、TS を tsreadex と FFmpeg に通して、HLS のプレイリストとセグメントを書く。
type Transcoder struct {
	cfg      *config.Kyusu
	ffmpeg   string
	tsreadex string
}

// NewTranscoder は Transcoder を返す。FFmpeg と tsreadex が PATH になければ、ErrToolNotFound を返す。
func NewTranscoder(cfg *config.Kyusu) (*Transcoder, error) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrToolNotFound, err)
	}
	tsreadex, err := exec.LookPath("tsreadex")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrToolNotFound, err)
	}

	return &Transcoder{cfg: cfg, ffmpeg: ffmpeg, tsreadex: tsreadex}, nil
}

// RunOptions は、録画の仮想 HLS のセグメントを書くときの設定 (設計書 7.3)。
// 仮想 HLS では、録画の始まりからの時間を、Segment の長さで区切ってセグメントの番号を決めておき、
// k 番目のセグメントから書くときは、その時刻の手前の I フレームから入力を流して、エンコーダーを起動し直す。
type RunOptions struct {
	// StartNumber は、最初に書くセグメントの番号 k。
	StartNumber int
	// Trim は、入力の始まり (流し始めた I フレーム) から、k 番目のセグメントの始まりまでの時間。デコードして捨てる。
	Trim time.Duration
}

// VODSegmentDuration は、録画の仮想 HLS のセグメントの長さ。ライブ視聴と同じ 2 秒にして、シークの位置を細かく合わせる。
const VODSegmentDuration = segmentSeconds * time.Second

// vodPlaylistName は、録画のトランスコードで FFmpeg が書くプレイリストの名前。視聴者には、kyusu が作るプレイリストを返す。
const vodPlaylistName = "encoder.m3u8"

// Run は、src を読み終えるか ctx が終わるまで、preset の画質と audio の音声で dir に HLS を書く。dir は作っておく。
// ctx が終わったら、どちらのプロセスも止めて、ctx のエラーを返す。
// どちらかのプロセスが失敗したら、もう一方も止めて、失敗したプロセスの標準エラー出力を添えたエラーを返す。
func (t *Transcoder) Run(ctx context.Context, src io.Reader, preset *Preset, audio Audio, dir string) error {
	return t.run(ctx, src, ffmpegArgs(t.cfg, preset, audio, dir, nil))
}

// RunVOD は、録画の仮想 HLS のセグメントを、opts.StartNumber 番目から書く。src は、k 番目のセグメントの手前の I フレームから流す。
// FFmpeg は書いたセグメントを消さないので、視聴者は巻き戻せる。古いセグメントは VOD が消す。セグメントの時刻は、k 番目のセグメントの始まりを k × セグメントの長さにする。
func (t *Transcoder) RunVOD(ctx context.Context, src io.Reader, preset *Preset, audio Audio, dir string, opts *RunOptions) error {
	return t.run(ctx, src, ffmpegArgs(t.cfg, preset, audio, dir, opts))
}

func (t *Transcoder) run(ctx context.Context, src io.Reader, args []string) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	normalizer := exec.CommandContext(runCtx, t.tsreadex, tsreadexArgs()...)
	encoder := exec.CommandContext(runCtx, t.ffmpeg, args...)
	normalizerErr, encoderErr := newTailBuffer(stderrLimit), newTailBuffer(stderrLimit)
	for _, cmd := range []*exec.Cmd{normalizer, encoder} {
		cmd.WaitDelay = stopDelay
	}
	normalizer.Stdin = src
	normalizer.Stderr = normalizerErr
	encoder.Stderr = encoderErr
	encoder.Stdout = io.Discard
	pipe, err := normalizer.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to connect tsreadex to FFmpeg: %w", err)
	}
	encoder.Stdin = pipe

	if err := encoder.Start(); err != nil {
		_ = pipe.Close()

		return fmt.Errorf("failed to start FFmpeg: %w", err)
	}
	if err := normalizer.Start(); err != nil {
		_ = pipe.Close()
		cancel()
		_ = encoder.Wait()

		return fmt.Errorf("failed to start tsreadex: %w", err)
	}
	normalizerDone, encoderDone := make(chan error, 1), make(chan error, 1)
	go func() {
		normalizerDone <- normalizer.Wait()
	}()
	go func() {
		encoderDone <- encoder.Wait()
	}()

	select {
	case err := <-normalizerDone:
		if err != nil {
			// tsreadex が失敗したら、FFmpeg も止める。
			cancel()
			<-encoderDone
			if ctx.Err() != nil {
				return ctx.Err()
			}

			return fmt.Errorf("tsreadex failed: %w: %s", err, normalizerErr.String())
		}
		// 入力を読み終えたので、FFmpeg が残りを書き終えるのを待つ。
		if err := <-encoderDone; err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			return fmt.Errorf("FFmpeg failed: %w: %s", err, encoderErr.String())
		}

		return nil
	case err := <-encoderDone:
		// FFmpeg が先に止まったら、入力を待っている tsreadex も止める。
		cancel()
		<-normalizerDone
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			return fmt.Errorf("FFmpeg failed: %w: %s", err, encoderErr.String())
		}

		return fmt.Errorf("FFmpeg exited before the input ended: %s", encoderErr.String())
	}
}

// tsreadexArgs は、tsreadex の引数を返す (設計書 7.3)。
//
//   - -x 18/38/39: EIT などの番組情報を取り除く。
//   - -n -1: PAT の最初のサービスだけを選び、PID を固定する。
//   - -a 13: 第 1 音声がなければ無音を補い、モノラルをステレオにし、デュアルモノを 2 つの音声に分ける。
//   - -b 7: 第 2 音声がなければ第 1 音声を写し、モノラルをステレオにする。
//   - -c 5 と -u 1: 字幕と文字スーパーがなくても PMT に載せ、字幕は 5 秒ごとに空のデータを補う。FFmpeg が字幕を待って止まらないようにする。
//   - -d 9: 字幕と文字スーパーを、aribb24.js が読める ID3 timed metadata に変換し、PTS を単調増加にする。
//     +4 (FFmpeg 4.4 のバグの回避) は、FFmpeg 6.1 で直ったので付けない。
func tsreadexArgs() []string {
	return []string{"-x", "18/38/39", "-n", "-1", "-a", "13", "-b", "7", "-c", "5", "-u", "1", "-d", "9", "-"}
}

// ffmpegArgs は、FFmpeg の引数を返す。音声は、audio で選んだ tsreadex の出力の音声を 1 つだけ入れる。映像は、どのエンコーダーでも、CPU の yadif でインターレースを解除してから縮める (設計書 7.2)。
// VAAPI と QSV では、解除した後に GPU へ送って縮める。vod が nil でなければ、録画の仮想 HLS のセグメントを書く。
func ffmpegArgs(cfg *config.Kyusu, preset *Preset, audio Audio, dir string, vod *RunOptions) []string {
	height := strconv.Itoa(preset.Height)
	device := []string{}
	// yadif の mode は、フィールドごとに 1 フレームにするなら 1、フレームごとなら 0。
	yadif := "yadif=0:-1:1"
	if preset.Fields {
		yadif = "yadif=1:-1:1"
	}
	filter := yadif + ",scale=-2:" + height
	var encoder string
	options := []string{}
	switch cfg.Encoder {
	case config.EncoderVAAPI:
		device = []string{"-vaapi_device", cfg.VAAPIDevice}
		filter = yadif + ",format=nv12,hwupload,scale_vaapi=w=-2:h=" + height
		encoder = map[Codec]string{CodecH264: "h264_vaapi", CodecHEVC: "hevc_vaapi"}[preset.Codec]
	case config.EncoderQSV:
		device = []string{"-init_hw_device", "qsv=hw", "-filter_hw_device", "hw"}
		// scale_qsv は、幅に -2 (比を保って偶数にそろえる) を受け付けず、-1 (比を保つ) までしか受け付けない (#509)。
		filter = yadif + ",format=nv12,hwupload=extra_hw_frames=64,scale_qsv=w=-1:h=" + height
		encoder = map[Codec]string{CodecH264: "h264_qsv", CodecHEVC: "hevc_qsv"}[preset.Codec]
	case config.EncoderNVENC:
		encoder = map[Codec]string{CodecH264: "h264_nvenc", CodecHEVC: "hevc_nvenc"}[preset.Codec]
	default:
		// x264 と x265 は、速さを優先する。
		encoder = map[Codec]string{CodecH264: "libx264", CodecHEVC: "libx265"}[preset.Codec]
		options = []string{"-preset", "veryfast"}
	}
	bitrate := preset.VideoBitrate

	args := make([]string, 0, 64)
	args = append(args, "-hide_banner", "-loglevel", "warning", "-nostdin")
	args = append(args, device...)
	args = append(args, "-f", "mpegts", "-i", "pipe:0", "-map", "0:v:0", "-map", "0:a:"+audio.streamIndex(), "-map", "0:d?")
	hls := []string{
		"-hls_list_size", strconv.Itoa(playlistSize),
		// temp_file は、書きかけのプレイリストとセグメントを読ませないようにする。
		"-hls_flags", "delete_segments+independent_segments+temp_file",
		"-hls_segment_filename", filepath.Join(dir, segmentPattern),
		filepath.Join(dir, PlaylistName),
	}
	if vod != nil {
		// 出力側の -ss は、デコードしてから捨てるので、k 番目のセグメントは、フレーム単位で k × セグメントの長さから始まる。
		args = append(args,
			"-ss", seconds(vod.Trim),
			"-output_ts_offset", seconds(time.Duration(vod.StartNumber)*VODSegmentDuration),
		)
		hls = []string{
			"-hls_list_size", "0", "-start_number", strconv.Itoa(vod.StartNumber),
			"-hls_flags", "independent_segments+temp_file",
			"-hls_segment_filename", filepath.Join(dir, segmentPattern),
			filepath.Join(dir, vodPlaylistName),
		}
	}
	args = append(args, "-vf", filter)
	args = append(args, "-c:v", encoder)
	args = append(args, options...)
	args = append(args,
		"-b:v", strconv.Itoa(bitrate)+"k", "-maxrate", strconv.Itoa(bitrate*3/2)+"k", "-bufsize", strconv.Itoa(bitrate*2)+"k",
		// セグメントの長さをそろえるため、I フレームを 2 秒ごとに置く。
		"-force_key_frames", "expr:gte(t,n_forced*"+strconv.Itoa(segmentSeconds)+")",
		"-c:a", "aac", "-b:a", strconv.Itoa(audioBitrate)+"k", "-ac", "2", "-ar", "48000",
		"-c:d", "copy",
		"-f", "hls", "-hls_segment_type", "mpegts", "-hls_time", strconv.Itoa(segmentSeconds),
	)

	return append(args, hls...)
}

// seconds は、FFmpeg の引数に渡す秒を返す。
func seconds(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'f', 6, 64)
}

// tailBuffer は、書かれたものの末尾 limit バイトだけを残す。プロセスの標準エラー出力を、エラーの説明に添えるのに使う。
type tailBuffer struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	limit int
}

var _ io.Writer = new(tailBuffer)

func newTailBuffer(limit int) *tailBuffer {
	return &tailBuffer{mu: sync.Mutex{}, buf: bytes.Buffer{}, limit: limit}
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Write(p)
	if over := b.buf.Len() - b.limit; over > 0 {
		b.buf.Next(over)
	}

	return len(p), nil
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

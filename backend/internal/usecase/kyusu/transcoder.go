package kyusu

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/lib/sharedfile"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
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
	// 映像を変換しないプリセットでは使わない。
	Trim time.Duration
	// Cuts は、映像を変換しないプリセットで、k 番目から後のセグメントを始める I フレームの、出力での PTS (90 kHz) の並び (#806)。
	// FFmpeg は、入力の PTS に Offset を足して書くので、放送の PTS に Offset を足した値にする。
	Cuts []int64
	// Offset は、映像を変換しないプリセットで、入力の PTS に足して出力の PTS にする量 (90 kHz)。9 の倍数にする。
	// FFmpeg はマイクロ秒で受け取るので、9 の倍数なら、刻みの数がそのまま保たれる。
	Offset int64
}

// VODSegmentDuration は、録画の仮想 HLS のセグメントの長さ。ライブ視聴と同じ 2 秒にして、シークの位置を細かく合わせる。
const VODSegmentDuration = segmentSeconds * time.Second

// vodPlaylistName は、録画のトランスコードで FFmpeg が書くプレイリストの名前。視聴者には、kyusu が作るプレイリストを返す。
const vodPlaylistName = "encoder.m3u8"

// Run は、src を読み終えるか ctx が終わるまで、preset の画質と audio の音声で dir に HLS を書く。dir は作っておく。
// ctx が終わったら、どちらのプロセスも止めて、ctx のエラーを返す。
// どちらかのプロセスが失敗したら、もう一方も止めて、失敗したプロセスの標準エラー出力を添えたエラーを返す。
func (t *Transcoder) Run(ctx context.Context, src io.Reader, preset *Preset, audio Audio, dir string) error {
	return t.run(ctx, src, ffmpegArgs(t.cfg, preset, audio, dir, nil), io.Discard)
}

// RunVOD は、録画の仮想 HLS のセグメントを、opts.StartNumber 番目から書く。src は、k 番目のセグメントの手前の I フレームから流す。
// FFmpeg は書いたセグメントを消さないので、視聴者は巻き戻せる。古いセグメントは VOD が消す。セグメントの時刻は、k 番目のセグメントの始まりを k × セグメントの長さにする。
//
// 映像を変換しないプリセットでは、FFmpeg の出力を 1 本の TS で受け取り、opts.Cuts の I フレームの位置で区切って書く (#806)。
// FFmpeg の HLS の muxer は、出力の始まりからの時間で区切るので、起動した位置によって区切りがずれるからである。
// セグメントは、書き終えてから名前を変えて置くので、ファイルがあれば書き終えている。最後のセグメントは、入力を読み終えたときだけ置く。
func (t *Transcoder) RunVOD(ctx context.Context, src io.Reader, preset *Preset, audio Audio, dir string, opts *RunOptions) error {
	if !preset.Passthrough {
		return t.run(ctx, src, ffmpegArgs(t.cfg, preset, audio, dir, opts), io.Discard)
	}
	splitter, err := ts.NewSplitter(opts.Cuts, VODSegmentDuration, func(i int) (ts.Segment, error) {
		return newSegmentFile(filepath.Join(dir, fmt.Sprintf(segmentPattern, opts.StartNumber+i)))
	})
	if err != nil {
		return err
	}
	err = t.run(ctx, src, ffmpegArgs(t.cfg, preset, audio, dir, opts), splitter)
	if err == nil {
		err = splitter.Close()
	}
	// 止めたか失敗したときは、書きかけのセグメントを捨てる。開いたままにすると、ハンドルが漏れ、一時ファイルも残る (#1094)。
	if err != nil {
		if abortErr := splitter.Abort(); abortErr != nil {
			err = errors.Join(err, abortErr)
		}
	}

	return err
}

// segmentFile は、セグメントを一時的な名前で書き、閉じるときに本来の名前に変える。
type segmentFile struct {
	*os.File

	path string
}

var _ ts.Segment = new(segmentFile)

func newSegmentFile(path string) (*segmentFile, error) {
	file, err := os.Create(path + ".tmp")
	if err != nil {
		return nil, fmt.Errorf("failed to create a segment: %w", err)
	}

	return &segmentFile{File: file, path: path}, nil
}

func (f *segmentFile) Close() error {
	if err := f.File.Close(); err != nil {
		return fmt.Errorf("failed to write a segment: %w", err)
	}
	if err := sharedfile.Rename(f.Name(), f.path); err != nil {
		return fmt.Errorf("failed to put a segment: %w", err)
	}

	return nil
}

// Abort は、書きかけのセグメントを閉じ、一時的な名前のファイルを消す。Close で閉じた後でも消す。
func (f *segmentFile) Abort() error {
	// 捨てるので、閉じるときのエラー (閉じ済みも含む) は気にしない。
	_ = f.File.Close()
	if err := os.Remove(f.Name()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to discard a segment: %w", err)
	}

	return nil
}

// run は、tsreadex と FFmpeg を動かす。FFmpeg の標準出力は stdout に書く。
func (t *Transcoder) run(ctx context.Context, src io.Reader, args []string, stdout io.Writer) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	normalizer := exec.CommandContext(runCtx, t.tsreadex, tsreadexArgs()...) //nolint:gosec // G204: パスは PATH から引き、引数は固定の値から取る。
	// FFmpeg のパスは PATH から引く。引数は、設定のエンコーダーと、固定の表の画質と、数値から組み立てる。
	// 利用者の入力 (URL の画質や音声) は、表を引くのに使うだけで、そのまま渡さない。
	// gosec の taint 解析は、入力を受け取った関数の戻り値をまとめて汚染されたものとみなすので、G204 と G702 をここで外す。
	encoder := exec.CommandContext(runCtx, t.ffmpeg, args...) //nolint:gosec // G204 と G702 の誤検知。理由は上のとおり。
	normalizerErr, encoderErr := newTailBuffer(stderrLimit), newTailBuffer(stderrLimit)
	for _, cmd := range []*exec.Cmd{normalizer, encoder} {
		cmd.WaitDelay = stopDelay
	}
	normalizer.Stdin = src
	normalizer.Stderr = normalizerErr
	encoder.Stderr = encoderErr
	encoder.Stdout = stdout
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
// yadif は、インターレースの印のあるフレームだけを解除するので、BS4K のプログレッシブの映像は、そのまま通す。
// 縮めた後は 8 ビットにする。BS4K の 10 ビットの映像 (HEVC Main10) をそのまま渡すと、x264 は High 10 で書き、
// ブラウザで再生できない。NVENC は 10 ビットの入力を受け付けない (#806)。
// VAAPI と QSV では、解除した後に GPU へ送って縮める。NVENC では、映像の復号を GPU (CUDA) でする。BS4K の 2160p の HEVC を CPU で復号すると重いため。
// vod が nil でなければ、録画の仮想 HLS のセグメントを書く。
// 映像を変換しないプリセットは、passthroughArgs で組み立てる。
func ffmpegArgs(cfg *config.Kyusu, preset *Preset, audio Audio, dir string, vod *RunOptions) []string {
	if preset.Passthrough {
		return passthroughArgs(audio, dir, vod)
	}
	height := strconv.Itoa(preset.Height)
	device := []string{}
	// yadif の mode は、フィールドごとに 1 フレームにするなら 1、フレームごとなら 0。
	// フィールドごとにするときは、59.94fps に固定する。プログレッシブの 59.94p の入力 (BS4K) では、yadif がフレームを
	// そのまま通したまま、フレームレートだけを倍 (119.88fps) と記すため。
	yadif := "yadif=0:-1:1"
	if preset.Fields {
		yadif = "yadif=1:-1:1,fps=60000/1001"
	}
	filter := yadif + ",scale=-2:" + height + ",format=yuv420p"
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
		// 復号したフレームは CPU のメモリへ戻し、yadif と縮小は CPU でする。
		device = []string{"-hwaccel", "cuda"}
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

// passthroughArgs は、映像を変換しないプリセットの FFmpeg の引数を返す (#806)。音声と字幕は、ほかのプリセットと同じく扱う。
// ライブ視聴では、HLS の muxer で書く。セグメントは I フレームの位置でしか切れないので、長さは 2 秒ちょうどにならない。
// 録画では、入力の PTS をそのまま使い (-copyts)、vod.Offset を足して、1 本の TS で標準出力に書く。区切るのは RunVOD である。
// mpegts の muxer が PTS を遅らせない (-muxdelay と -muxpreload を 0) ようにして、出力の PTS を、放送の PTS に Offset を足したものにそろえる。
func passthroughArgs(audio Audio, dir string, vod *RunOptions) []string {
	args := make([]string, 0, 48)
	args = append(args, "-hide_banner", "-loglevel", "warning", "-nostdin")
	if vod != nil {
		args = append(args, "-copyts")
	}
	args = append(args, "-f", "mpegts", "-i", "pipe:0", "-map", "0:v:0", "-map", "0:a:"+audio.streamIndex(), "-map", "0:d?",
		"-c:v", "copy",
		"-c:a", "aac", "-b:a", strconv.Itoa(audioBitrate)+"k", "-ac", "2", "-ar", "48000",
		"-c:d", "copy",
	)
	if vod != nil {
		return append(args,
			"-output_ts_offset", ptsSeconds(vod.Offset),
			"-muxdelay", "0", "-muxpreload", "0",
			"-f", "mpegts", "pipe:1",
		)
	}

	return append(args,
		"-f", "hls", "-hls_segment_type", "mpegts", "-hls_time", strconv.Itoa(segmentSeconds),
		"-hls_list_size", strconv.Itoa(playlistSize),
		"-hls_flags", "delete_segments+independent_segments+temp_file",
		"-hls_segment_filename", filepath.Join(dir, segmentPattern),
		filepath.Join(dir, PlaylistName),
	)
}

// ptsSeconds は、9 の倍数の PTS の刻み (90 kHz) を、FFmpeg の引数に渡す秒にする。
// 刻みの 9 つは 100 マイクロ秒なので、マイクロ秒の整数で表せる。ナノ秒を経ないので、長い時間でも int64 を超えない。
func ptsSeconds(ticks int64) string {
	micros := ticks / 9 * 100
	sign := ""
	if micros < 0 {
		sign, micros = "-", -micros
	}

	return fmt.Sprintf("%s%d.%06d", sign, micros/1_000_000, micros%1_000_000)
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

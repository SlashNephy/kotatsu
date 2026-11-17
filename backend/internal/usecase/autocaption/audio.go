package autocaption

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/procprio"
	"github.com/SlashNephy/kotatsu/internal/lib/sharedfile"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

const (
	// ffmpegCommand は、PATH から探す FFmpeg のコマンドの名前。FFmpeg の入ったイメージ (-ffmpeg) には入っている。
	ffmpegCommand = "ffmpeg"
	// audioFilter は、whisper.cpp が読む 16 kHz の音声にするフィルター。async=1 で、ドロップで欠けた所を無音で埋め、後の字幕の時刻をずらさない。
	audioFilter = "aresample=16000:async=1"
	// noStreamMessage は、-map に当たるストリームがないときに FFmpeg が出すエラーの一部。
	noStreamMessage = "matches no streams"
)

var (
	// ErrFFmpegNotFound は、FFmpeg が PATH に見つからないことを表す。
	ErrFFmpegNotFound = errors.New("FFmpeg is not found in PATH")
	// ErrNoAudio は、ファイルに、サービスの音声が見つからないことを表す。
	ErrNoAudio = errors.New("the file has no audio of the service")
)

// Extractor は、録画のセグメントの音声を、16 kHz のモノラルの WAV に取り出す。
// 返す offset は、WAV の始まりの、再生の時間 (セグメントの最初の I フレームを 0 にしたもの、#1811) での位置。
type Extractor func(ctx context.Context, path string, serviceID uint16, wav string) (offset time.Duration, err error)

// FFmpegExtractor は、FFmpeg で音声を取り出す Extractor を返す。FFmpeg がなければ ErrFFmpegNotFound を返す。
// lowerer は、FFmpeg を最低の優先度で動かす。nil なら優先度を変えない。
func FFmpegExtractor(lowerer *procprio.Lowerer) (Extractor, error) {
	if _, err := exec.LookPath(ffmpegCommand); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFFmpegNotFound, err)
	}

	return func(ctx context.Context, path string, serviceID uint16, wav string) (time.Duration, error) {
		file, err := sharedfile.Open(path)
		if err != nil {
			return 0, fmt.Errorf("failed to open the recording file: %w", err)
		}
		defer func() { _ = file.Close() }()

		return extractAudio(ctx, lowerer, file, serviceID, "0:a:0", wav)
	}, nil
}

// ReaderExtractor は、TS を読み出す r から、Extractor と同じく音声を取り出す。ローカルのパスのないファイル (ライブラリのファイル。#1734) で使う。
// ほかのソフトで丸ごと録った、複数のサービスを含む TS もあるので、serviceID の番組 (program_number) の最初の音声を読む。
type ReaderExtractor func(ctx context.Context, r io.Reader, serviceID uint16, wav string) (offset time.Duration, err error)

// FFmpegReaderExtractor は、FFmpeg で音声を取り出す ReaderExtractor を返す。FFmpeg がなければ ErrFFmpegNotFound を返す。
func FFmpegReaderExtractor(lowerer *procprio.Lowerer) (ReaderExtractor, error) {
	if _, err := exec.LookPath(ffmpegCommand); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFFmpegNotFound, err)
	}

	return func(ctx context.Context, r io.Reader, serviceID uint16, wav string) (time.Duration, error) {
		return extractAudio(ctx, lowerer, r, serviceID, fmt.Sprintf("0:p:%d:a:0", serviceID), wav)
	}, nil
}

// extractAudio は、TS を読み出す r の最初の音声を、FFmpeg で wav に書く。FFmpeg に流すのと同じ TS を ts.CueScanner にも読ませ、
// 最初の音声と最初の I フレームの PTS の差から、WAV の始まりの再生の時間での位置を求める (CM の区間と同じ基準、#1811)。
// 読む音声は stream (FFmpeg の -map の指定) で選ぶ。録画は録画したサービスだけの TS なので (CM の区間の見分けと同じく)、最初の音声 (0:a:0) を読む。
// デュアルモノは、2 つの音声を混ぜて 1 つにする。
func extractAudio(ctx context.Context, lowerer *procprio.Lowerer, r io.Reader, serviceID uint16, stream, wav string) (time.Duration, error) {
	scanner := ts.NewCueScanner(serviceID)
	// 時刻を PTS のまま扱わせ (-copyts)、TS の不連続を詰めずに、欠けた所を aresample に無音で埋めさせる。
	cmd := exec.CommandContext(ctx, ffmpegCommand, "-nostdin", "-hide_banner", "-nostats", "-loglevel", "error",
		"-threads", "1", "-copyts", "-f", "mpegts", "-i", "pipe:0",
		"-map", stream, "-vn", "-sn", "-dn", "-af", audioFilter, "-ac", "1", "-c:a", "pcm_s16le", "-f", "wav", "-y", wav)
	cmd.Stdin = io.TeeReader(r, scanner)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return 0, fmt.Errorf("failed to open the FFmpeg stderr: %w", err)
	}
	if err := lowerer.Start(cmd); err != nil {
		return 0, fmt.Errorf("failed to start FFmpeg: %w", err)
	}
	tail := tailLines(stderr)
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return 0, fmt.Errorf("FFmpeg was stopped: %w", ctx.Err())
		}
		// 音声がないと分かるのは、FFmpeg が 0:a:0 に当たるストリームを見つけなかったときだけ。読めなかったなどのほかの失敗を、
		// 音声のないセグメントとして飛ばすと、前の字幕を空で置き換えてしまう。
		if scanner.Cues().FirstAudioPTS == nil && slices.ContainsFunc(tail, func(line string) bool { return strings.Contains(line, noStreamMessage) }) {
			return 0, ErrNoAudio
		}

		return 0, fmt.Errorf("failed to extract the audio with FFmpeg: %w: %s", err, strings.Join(tail, "; "))
	}

	return audioOffset(scanner.Cues())
}

// audioOffset は、最初の音声の PTS の、再生の時間 (最初の I フレームを 0 にしたもの) での位置を返す。
// I フレームの探し方を知らない映像で、最初の I フレームが分からなければ、最初の映像の PES を 0 にする (CM の区間と同じ)。
// 映像がなければ、音声の始まりを 0 にする。PTS が一周していても、差が半周より短ければ正しく求まる。
func audioOffset(cues *ts.Cues) (time.Duration, error) {
	if cues.FirstAudioPTS == nil {
		return 0, ErrNoAudio
	}
	base := *cues.FirstAudioPTS
	switch {
	case cues.FirstKeyframePTS != nil:
		base = *cues.FirstKeyframePTS
	case cues.FirstVideoPTS != nil:
		base = *cues.FirstVideoPTS
	}

	return time.Duration(ts.PTSDiff(base, *cues.FirstAudioPTS)) * time.Second / ts.PTSClock, nil
}

// tailLines は、r を最後まで読み、末尾の数行を返す。
func tailLines(r io.Reader) []string {
	tail := []string{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		tail = append(tail, scanner.Text())
		if len(tail) > maxStderrLines {
			tail = tail[1:]
		}
	}
	_, _ = io.Copy(io.Discard, r)

	return tail
}

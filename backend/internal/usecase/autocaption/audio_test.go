package autocaption_test

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/usecase/autocaption"
)

func TestAudioOffset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		video    *int64
		keyframe *int64
		audio    *int64
		want     time.Duration
		wantErr  error
	}{
		{name: "音声が I フレームより後に始まる", video: new(int64(9000)), keyframe: new(int64(18000)), audio: new(int64(27000)), want: 100 * time.Millisecond},
		{name: "音声が I フレームより前に始まる", video: new(int64(9000)), keyframe: new(int64(18000)), audio: new(int64(9000)), want: -100 * time.Millisecond},
		{name: "I フレームが分からなければ、最初の映像の PES を 0 にする", video: new(int64(9000)), keyframe: nil, audio: new(int64(18000)), want: 100 * time.Millisecond},
		{name: "映像がなければ、音声の始まりを 0 にする", video: nil, keyframe: nil, audio: new(int64(18000)), want: 0},
		// PTS は 33 ビットで一周する。
		{name: "PTS が一周していても差を求める", video: nil, keyframe: new(int64(1<<33 - 4500)), audio: new(int64(4500)), want: 100 * time.Millisecond},
		{name: "音声がなければ誤り", video: new(int64(9000)), keyframe: new(int64(9000)), audio: nil, wantErr: autocaption.ErrNoAudio},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := autocaption.AudioOffset(&ts.Cues{
				FirstVideoPTS: tt.video, LastVideoPTS: tt.video, FirstKeyframePTS: tt.keyframe, FirstAudioPTS: tt.audio, AudioChanges: []int64{},
			})
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestFFmpegExtractor は、録画の音声を 16 kHz のモノラルの WAV に取り出し、WAV の始まりの、最初の I フレームからの位置を返すことを確認する。
// 位置は、ffprobe で読んだ最初の音声のパケットと最初の映像のキーフレームの PTS の差と比較する。FFmpeg か ffprobe がなければ飛ばす。
func TestFFmpegExtractor(t *testing.T) {
	t.Parallel()

	extract, err := autocaption.FFmpegExtractor(nil)
	if errors.Is(err, autocaption.ErrFFmpegNotFound) {
		t.Skip("FFmpeg is not installed")
	}
	require.NoError(t, err)
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("FFmpeg is not installed")
	}
	const path = "../../../testdata/ts/test.ts"
	wav := filepath.Join(t.TempDir(), "0.wav")
	offset, err := extract(t.Context(), path, 0, wav)
	require.NoError(t, err)

	want := firstPacketPTS(t, path, "a:0", false) - firstPacketPTS(t, path, "v:0", true)
	assert.InDelta(t, want, offset.Seconds(), 0.001)
	data, err := os.ReadFile(wav)
	require.NoError(t, err)
	require.Greater(t, len(data), 44)
	assert.Equal(t, "RIFF", string(data[0:4]))
	assert.Equal(t, uint16(1), binary.LittleEndian.Uint16(data[22:24]), "mono")
	assert.Equal(t, uint32(16000), binary.LittleEndian.Uint32(data[24:28]), "16 kHz")
	assert.Equal(t, uint16(16), binary.LittleEndian.Uint16(data[34:36]), "16 bits")
}

// TestFFmpegReaderExtractor は、ローカルのパスのないファイル (ライブラリのファイル。#1734) から、serviceID の番組の音声を取り出し、
// パスで読んだときと同じ位置を返すことを確認する。ほかのサービスの番組を指せば、そのサービスの音声はないので、ほかのサービスの音声を
// 取り出さずに失敗する。
// FFmpeg か ffprobe がなければ飛ばす。
func TestFFmpegReaderExtractor(t *testing.T) {
	t.Parallel()

	extract, err := autocaption.FFmpegReaderExtractor(nil)
	if errors.Is(err, autocaption.ErrFFmpegNotFound) {
		t.Skip("FFmpeg is not installed")
	}
	require.NoError(t, err)
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("FFmpeg is not installed")
	}
	const path = "../../../testdata/ts/test.ts"
	byPath, err := autocaption.FFmpegExtractor(nil)
	require.NoError(t, err)
	want, err := byPath(t.Context(), path, 0x0400, filepath.Join(t.TempDir(), "path.wav"))
	require.NoError(t, err)
	tests := []struct {
		name      string
		serviceID uint16
		wantErr   bool
	}{
		{name: "the service of the file", serviceID: 0x0400, wantErr: false},
		{name: "another service", serviceID: 0x0401, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, err := os.Open(path)
			require.NoError(t, err)
			defer file.Close()
			offset, err := extract(t.Context(), file, tt.serviceID, filepath.Join(t.TempDir(), "0.wav"))
			if tt.wantErr {
				require.Error(t, err)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, want, offset, "パスで読んだときと同じ位置")
		})
	}
}

// TestFFmpegExtractorWithoutAudio は、音声のない録画では ErrNoAudio を返すことを確認する。FFmpeg がなければ飛ばす。
func TestFFmpegExtractorWithoutAudio(t *testing.T) {
	t.Parallel()

	extract, err := autocaption.FFmpegExtractor(nil)
	if errors.Is(err, autocaption.ErrFFmpegNotFound) {
		t.Skip("FFmpeg is not installed")
	}
	require.NoError(t, err)
	_, err = extract(t.Context(), "../../../testdata/ts/test-h264.ts", 0, filepath.Join(t.TempDir(), "0.wav"))
	require.ErrorIs(t, err, autocaption.ErrNoAudio)
}

// TestFFmpegExtractorStopped は、止めたら ErrNoAudio ではなく、止めた理由を返すことを確認する。音声のないセグメントとして飛ばし、
// 残りのセグメントを読み続けないようにする。FFmpeg がなければ飛ばす。
func TestFFmpegExtractorStopped(t *testing.T) {
	t.Parallel()

	extract, err := autocaption.FFmpegExtractor(nil)
	if errors.Is(err, autocaption.ErrFFmpegNotFound) {
		t.Skip("FFmpeg is not installed")
	}
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = extract(ctx, "../../../testdata/ts/test-h264.ts", 0, filepath.Join(t.TempDir(), "0.wav"))
	require.ErrorIs(t, err, context.Canceled)
	require.NotErrorIs(t, err, autocaption.ErrNoAudio)
}

// TestFFmpegExtractorUnreadable は、FFmpeg が読めないファイル (TS でないもの) を、音声のないセグメントとして飛ばさないことを確認する。
// FFmpeg がなければ飛ばす。
func TestFFmpegExtractorUnreadable(t *testing.T) {
	t.Parallel()

	extract, err := autocaption.FFmpegExtractor(nil)
	if errors.Is(err, autocaption.ErrFFmpegNotFound) {
		t.Skip("FFmpeg is not installed")
	}
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "broken.m2ts")
	require.NoError(t, os.WriteFile(path, []byte("not a transport stream"), 0o600))
	_, err = extract(t.Context(), path, 0, filepath.Join(t.TempDir(), "0.wav"))
	require.Error(t, err)
	require.NotErrorIs(t, err, autocaption.ErrNoAudio)
}

// firstPacketPTS は、ffprobe で読んだ、TS の stream の最初のパケット (keyframe なら最初のキーフレームのパケット) の PTS (秒) を返す。
func firstPacketPTS(t *testing.T, path, stream string, keyframe bool) float64 {
	t.Helper()

	out, err := exec.CommandContext(t.Context(), "ffprobe", "-v", "error", "-select_streams", stream,
		"-show_entries", "packet=pts_time,flags", "-of", "csv=p=0", path).Output()
	require.NoError(t, err)
	for line := range strings.Lines(string(out)) {
		pts, flags, _ := strings.Cut(strings.TrimSpace(line), ",")
		if keyframe && !strings.Contains(flags, "K") {
			continue
		}
		value, err := strconv.ParseFloat(pts, 64)
		require.NoError(t, err)

		return value
	}
	require.FailNow(t, "no packet found", stream)

	return 0
}

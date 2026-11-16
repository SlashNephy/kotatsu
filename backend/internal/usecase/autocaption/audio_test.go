package autocaption_test

import (
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

// TestFFmpegExtractor は、録画の音声を 16 kHz のモノラルの WAV に取り出し、WAV の始まりの、最初の I フレームからの位置を返すことを確かめる。
// 位置は、ffprobe で読んだ最初の音声のパケットと最初の映像のキーフレームの PTS の差と比べる。FFmpeg か ffprobe がなければ飛ばす。
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

// TestFFmpegExtractorWithoutAudio は、音声のない録画では ErrNoAudio を返すことを確かめる。FFmpeg がなければ飛ばす。
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

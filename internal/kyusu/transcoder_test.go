package kyusu_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/config"
	"github.com/SlashNephy/kotatsu/internal/kyusu"
	"github.com/SlashNephy/kotatsu/internal/ts"
)

func loadTestTS(t *testing.T) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "ts", "test.ts"))
	require.NoError(t, err)

	return data
}

// newTranscoder は、x264 の Transcoder を返す。FFmpeg か tsreadex がなければ、テストを飛ばす。
func newTranscoder(t *testing.T) *kyusu.Transcoder {
	t.Helper()

	transcoder, err := kyusu.NewTranscoder(&config.Kyusu{Encoder: config.EncoderX264, VAAPIDevice: "", WorkDir: ""})
	if errors.Is(err, kyusu.ErrToolNotFound) {
		t.Skip("FFmpeg or tsreadex is not installed")
	}
	require.NoError(t, err)
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe is not installed")
	}

	return transcoder
}

// probedStream は、ffprobe が報告するストリーム。
type probedStream struct {
	CodecType string `json:"codec_type"`
	CodecName string `json:"codec_name"`
	Height    int    `json:"height"`
}

func probe(t *testing.T, path string) []probedStream {
	t.Helper()

	out, err := exec.CommandContext(t.Context(), "ffprobe", "-v", "error", "-show_streams", "-of", "json", path).Output()
	require.NoError(t, err)
	var probed struct {
		Streams []probedStream `json:"streams"`
	}
	require.NoError(t, json.Unmarshal(out, &probed))

	return probed.Streams
}

// TestTranscoderWritesHLS は、TS を、画質のプリセットどおりの映像と、AAC の音声と、字幕の ID3 timed metadata を持つ
// MPEG-TS のセグメントの HLS にすることを確かめる (設計書 7.3)。
// testdata/ts/test.ts には字幕がないので、ID3 timed metadata は、tsreadex が補う空のデータである。
func TestTranscoderWritesHLS(t *testing.T) {
	t.Parallel()

	transcoder := newTranscoder(t)
	tests := []struct {
		preset     string
		wantCodec  string
		wantHeight int
	}{
		{preset: "360p", wantCodec: "h264", wantHeight: 360},
		{preset: "720p", wantCodec: "h264", wantHeight: 720},
		{preset: "360p-hevc", wantCodec: "hevc", wantHeight: 360},
	}
	for _, tt := range tests {
		t.Run(tt.preset, func(t *testing.T) {
			t.Parallel()

			preset, ok := kyusu.PresetByName(tt.preset)
			require.True(t, ok)
			dir := t.TempDir()
			require.NoError(t, transcoder.Run(t.Context(), bytes.NewReader(loadTestTS(t)), preset, kyusu.AudioFirst, dir))

			playlist, err := os.ReadFile(filepath.Join(dir, kyusu.PlaylistName))
			require.NoError(t, err)
			assert.Contains(t, string(playlist), "#EXT-X-INDEPENDENT-SEGMENTS")
			assert.Contains(t, string(playlist), "#EXTINF:")
			segments, err := filepath.Glob(filepath.Join(dir, "segment*.ts"))
			require.NoError(t, err)
			require.NotEmpty(t, segments)
			sort.Strings(segments)
			assert.Contains(t, string(playlist), filepath.Base(segments[0]))
			data, err := os.ReadFile(segments[0])
			require.NoError(t, err)
			assert.Zero(t, len(data)%ts.PacketSize, "a segment consists of whole TS packets")

			got := map[string]probedStream{}
			for _, s := range probe(t, segments[0]) {
				got[s.CodecType] = s
			}
			assert.Equal(t, tt.wantCodec, got["video"].CodecName)
			assert.Equal(t, tt.wantHeight, got["video"].Height)
			assert.Equal(t, "aac", got["audio"].CodecName)
			assert.Equal(t, "timed_id3", got["data"].CodecName, "subtitles are carried as ID3 timed metadata")
		})
	}
}

// loopReader は、data を繰り返し返す。チューナーのストリームのように、終わらない入力にする。
type loopReader struct {
	data []byte
	pos  int
}

func (r *loopReader) Read(p []byte) (int, error) {
	n := copy(p, r.data[r.pos:])
	r.pos = (r.pos + n) % len(r.data)

	return n, nil
}

var _ io.Reader = new(loopReader)

// TestTranscoderStopsWithContext は、終わらない入力でも、ctx が終わればプロセスを止めて戻ることを確かめる。
func TestTranscoderStopsWithContext(t *testing.T) {
	t.Parallel()

	transcoder := newTranscoder(t)
	preset, ok := kyusu.PresetByName("360p")
	require.True(t, ok)
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	src := &loopReader{data: loadTestTS(t), pos: 0}
	done := make(chan error, 1)
	go func() {
		done <- transcoder.Run(ctx, src, preset, kyusu.AudioFirst, dir)
	}()
	require.Eventually(t, func() bool {
		playlist, err := os.ReadFile(filepath.Join(dir, kyusu.PlaylistName))

		return err == nil && strings.Contains(string(playlist), "#EXTINF:")
	}, 30*time.Second, 100*time.Millisecond, "the playlist is written while the input continues")

	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(10 * time.Second):
		t.Fatal("the transcoder did not stop")
	}
}

// stallingReader は、data を一度返した後、unblock が閉じられるまで止まる。番組の間で途切れたチューナーのストリームに当たる。
type stallingReader struct {
	data    []byte
	unblock <-chan struct{}
}

func (r *stallingReader) Read(p []byte) (int, error) {
	if len(r.data) > 0 {
		n := copy(p, r.data)
		r.data = r.data[n:]

		return n, nil
	}
	<-r.unblock

	return 0, io.EOF
}

// TestTranscoderReportsEncoderFailure は、入力が途切れている間に FFmpeg が失敗しても、待ち続けずに、FFmpeg のエラーを返すことを確かめる。
func TestTranscoderReportsEncoderFailure(t *testing.T) {
	t.Parallel()

	newTranscoder(t)
	// VAAPI のデバイスがないので、FFmpeg は起動してすぐに失敗する。
	transcoder, err := kyusu.NewTranscoder(&config.Kyusu{Encoder: config.EncoderVAAPI, VAAPIDevice: "/nonexistent/renderD128", WorkDir: ""})
	require.NoError(t, err)
	preset, ok := kyusu.PresetByName("360p")
	require.True(t, ok)
	unblock := make(chan struct{})
	t.Cleanup(func() { close(unblock) })
	dir := t.TempDir()
	done := make(chan error, 1)
	src := &stallingReader{data: loadTestTS(t), unblock: unblock}
	go func() {
		done <- transcoder.Run(t.Context(), src, preset, kyusu.AudioFirst, dir)
	}()

	select {
	case err := <-done:
		require.ErrorContains(t, err, "FFmpeg failed")
	case <-time.After(30 * time.Second):
		t.Fatal("the transcoder did not return after FFmpeg failed")
	}
}

// twoAudioTS は、test.ts の映像に、第 1 音声として 1 kHz の音を、第 2 音声として無音を付けた TS を作る。
// tsreadex は、放送と同じく、PMT の stream_identifier_descriptor (component_tag) で第 2 音声を見分けるので、付け足す。
func twoAudioTS(t *testing.T) []byte {
	t.Helper()

	path := filepath.Join(t.TempDir(), "two-audio.ts")
	cmd := exec.CommandContext(t.Context(), "ffmpeg", "-hide_banner", "-loglevel", "error",
		"-i", filepath.Join("..", "..", "testdata", "ts", "test.ts"),
		"-f", "lavfi", "-i", "sine=frequency=1000:sample_rate=48000",
		"-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=48000",
		"-map", "0:v:0", "-map", "1:a", "-map", "2:a", "-c:v", "copy", "-c:a", "aac", "-shortest", "-f", "mpegts", path)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	return tagComponents(t, data)
}

// tagComponents は、FFmpeg が書いた PMT の ES に、順に stream_identifier_descriptor を付ける。
// component_tag は、映像は 0x00、第 1 音声は 0x10、第 2 音声は 0x11 (ARIB TR-B14)。
// FFmpeg の PMT は 1 つのパケットに収まり、ES に記述子を持たないものとして扱う。
func tagComponents(t *testing.T, data []byte) []byte {
	t.Helper()

	componentTags := []byte{0x00, 0x10, 0x11}

	const pmtPID = 0x1000 // FFmpeg の mpegts の PMT の PID の既定値
	out := make([]byte, 0, len(data))
	for i := 0; i+ts.PacketSize <= len(data); i += ts.PacketSize {
		packet := data[i : i+ts.PacketSize]
		pid := int(packet[1]&0x1f)<<8 | int(packet[2])
		if pid != pmtPID || packet[1]&0x40 == 0 {
			out = append(out, packet...)

			continue
		}
		section := packet[5:] // ヘッダーの 4 バイトと pointer_field (0) の後
		length := int(section[1]&0x0f)<<8 | int(section[2])
		programInfoLength := int(section[10]&0x0f)<<8 | int(section[11])
		esStart, esEnd := 12+programInfoLength, 3+length-4
		rebuilt := append([]byte{}, section[:esStart]...)
		// added は、足した記述子のバイト数。section_length に足す。
		var added uint16
		for pos, n := esStart, 0; pos < esEnd; n++ {
			infoLength := int(section[pos+3]&0x0f)<<8 | int(section[pos+4])
			require.Zero(t, infoLength, "FFmpeg writes no ES descriptors")
			require.Less(t, n, len(componentTags))
			rebuilt = append(rebuilt, section[pos], section[pos+1], section[pos+2], 0xf0, 3, 0x52, 1, componentTags[n])
			added += 3
			pos += 5
		}
		binary.BigEndian.PutUint16(rebuilt[1:3], binary.BigEndian.Uint16(rebuilt[1:3])+added)
		rebuilt = binary.BigEndian.AppendUint32(rebuilt, mpegCRC32(rebuilt))
		next := append(append([]byte{}, packet[:5]...), rebuilt...)
		require.LessOrEqual(t, len(next), ts.PacketSize)
		for len(next) < ts.PacketSize {
			next = append(next, 0xff)
		}
		out = append(out, next...)
	}

	return out
}

// mpegCRC32 は、PSI のセクションの CRC (MPEG-2 の CRC-32) を返す。
func mpegCRC32(b []byte) uint32 {
	crc := uint32(0xffffffff)
	for _, v := range b {
		crc ^= uint32(v) << 24
		for range 8 {
			if crc&0x80000000 != 0 {
				crc = crc<<1 ^ 0x04c11db7
			} else {
				crc <<= 1
			}
		}
	}

	return crc
}

// meanVolume は、HLS のセグメントの音声の平均の音量 (dB) を返す。無音なら、およそ -91 dB になる。
func meanVolume(t *testing.T, path string) float64 {
	t.Helper()

	out, err := exec.CommandContext(t.Context(), "ffmpeg", "-hide_banner", "-i", path,
		"-map", "0:a:0", "-af", "volumedetect", "-f", "null", "-").CombinedOutput()
	require.NoError(t, err, string(out))
	match := regexp.MustCompile(`mean_volume: (-?[0-9.]+) dB`).FindSubmatch(out)
	require.NotNil(t, match, string(out))
	volume, err := strconv.ParseFloat(string(match[1]), 64)
	require.NoError(t, err)

	return volume
}

// TestTranscoderSelectsAudio は、選んだ音声だけを HLS に入れることを確かめる (設計書 7.5)。
func TestTranscoderSelectsAudio(t *testing.T) {
	t.Parallel()

	transcoder := newTranscoder(t)
	src := twoAudioTS(t)
	preset, ok := kyusu.PresetByName("360p")
	require.True(t, ok)
	tests := []struct {
		name   string
		audio  kyusu.Audio
		silent bool
	}{
		{name: "第 1 音声", audio: kyusu.AudioFirst, silent: false},
		{name: "第 2 音声", audio: kyusu.AudioSecond, silent: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			require.NoError(t, transcoder.Run(t.Context(), bytes.NewReader(src), preset, tt.audio, dir))
			segments, err := filepath.Glob(filepath.Join(dir, "segment*.ts"))
			require.NoError(t, err)
			require.NotEmpty(t, segments)
			sort.Strings(segments)

			audios := 0
			for _, s := range probe(t, segments[0]) {
				if s.CodecType == "audio" {
					audios++
				}
			}
			assert.Equal(t, 1, audios, "only the selected audio is encoded")
			volume := meanVolume(t, segments[0])
			if tt.silent {
				assert.Less(t, volume, -80.0)
			} else {
				assert.Greater(t, volume, -40.0)
			}
		})
	}
}

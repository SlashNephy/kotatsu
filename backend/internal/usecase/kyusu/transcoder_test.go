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
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/usecase/kyusu"
)

func loadTestTS(t *testing.T) []byte {
	t.Helper()

	return loadTS(t, "test.ts")
}

// loadTS は、testdata/ts の name の TS を返す。
func loadTS(t *testing.T, name string) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "ts", name))
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
	FrameRate string `json:"r_frame_rate"`
	PixFmt    string `json:"pix_fmt"`
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
// テスト用の TS には字幕がないので、ID3 timed metadata は、tsreadex が補う空のデータである。
// BS4K に似せた 10 ビットの HEVC の入力でも、ブラウザで再生できる 8 ビットの映像にすることと、
// 変換しないプリセットでは、HEVC をそのまま送ることを確かめる (#806)。
func TestTranscoderWritesHLS(t *testing.T) {
	t.Parallel()

	transcoder := newTranscoder(t)
	// frames30 と frames60 は、ffprobe が返すフレームレート (29.97fps と 59.94fps)。
	const frames30, frames60 = "30000/1001", "60000/1001"
	// mpeg2 は地上波に似せた MPEG-2 の入力、bs4k は BS4K に似せた 10 ビットの HEVC の入力。
	const mpeg2, bs4k = "test.ts", "test-hevc-main10.ts"
	// yuv420p は、ブラウザが復号できる 8 ビットの画素の形式。
	const yuv420p = "yuv420p"
	tests := []struct {
		// input は、testdata/ts の入力の TS。
		input      string
		preset     string
		wantCodec  string
		wantHeight int
		// wantFrameRate は、映像のフレームレート。test.ts は 29.97fps のインターレースなので、60fps のプリセットでは倍になる。
		// test-hevc-main10.ts は 59.94fps のプログレッシブなので、どのプリセットでもそのまま送る。
		wantFrameRate string
		// wantPixFmt は、映像の画素の形式。変換するプリセットでは、ブラウザが復号できる 8 ビットにする。
		// 変換しないプリセットでは、入力の 10 ビットのまま送る。
		wantPixFmt string
	}{
		{input: mpeg2, preset: "360p", wantCodec: string(kyusu.CodecH264), wantHeight: 360, wantFrameRate: frames30, wantPixFmt: yuv420p},
		{input: mpeg2, preset: "720p", wantCodec: string(kyusu.CodecH264), wantHeight: 720, wantFrameRate: frames30, wantPixFmt: yuv420p},
		{input: mpeg2, preset: "360p-hevc", wantCodec: string(kyusu.CodecHEVC), wantHeight: 360, wantFrameRate: frames30, wantPixFmt: yuv420p},
		{input: mpeg2, preset: "720p60", wantCodec: string(kyusu.CodecH264), wantHeight: 720, wantFrameRate: frames60, wantPixFmt: yuv420p},
		{input: bs4k, preset: "360p", wantCodec: string(kyusu.CodecH264), wantHeight: 360, wantFrameRate: frames60, wantPixFmt: yuv420p},
		{input: bs4k, preset: "360p-hevc", wantCodec: string(kyusu.CodecHEVC), wantHeight: 360, wantFrameRate: frames60, wantPixFmt: yuv420p},
		{input: bs4k, preset: "720p60", wantCodec: string(kyusu.CodecH264), wantHeight: 720, wantFrameRate: frames60, wantPixFmt: yuv420p},
		{input: bs4k, preset: kyusu.PassthroughPreset, wantCodec: string(kyusu.CodecHEVC), wantHeight: 180, wantFrameRate: frames60, wantPixFmt: "yuv420p10le"},
	}
	for _, tt := range tests {
		t.Run(tt.input+"/"+tt.preset, func(t *testing.T) {
			t.Parallel()

			preset, ok := kyusu.PresetByName(tt.preset)
			require.True(t, ok)
			dir := t.TempDir()
			require.NoError(t, transcoder.Run(t.Context(), bytes.NewReader(loadTS(t, tt.input)), preset, kyusu.AudioFirst, dir))

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
			assert.Equal(t, tt.wantFrameRate, got["video"].FrameRate)
			assert.Equal(t, tt.wantPixFmt, got["video"].PixFmt)
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
	//nolint:gosec // G204: 引数は固定の値と、テスト用の一時ディレクトリのパスだけ。
	cmd := exec.CommandContext(t.Context(), "ffmpeg", "-hide_banner", "-loglevel", "error",
		"-i", filepath.Join("..", "..", "..", "testdata", "ts", "test.ts"),
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

// TestTranscoderRunVODDiscardsThePartialSegment は、映像を変換しないプリセットの VOD を ctx で止めたとき、
// 書きかけのセグメントの一時ファイルを残さないことを確かめる (#1094)。
// 入力をそのまま流すスクリプトを、FFmpeg と tsreadex として PATH に置く。PATH はプロセスで共有するので、並行して動かさない。
func TestTranscoderRunVODDiscardsThePartialSegment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake tools are shell scripts")
	}
	bin := t.TempDir()
	for _, tool := range []string{"ffmpeg", "tsreadex"} {
		require.NoError(t, os.WriteFile(filepath.Join(bin, tool), []byte("#!/bin/sh\nexec cat\n"), 0o755)) //nolint:gosec // G306: PATH から実行するスクリプトなので、実行権限が要る。
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	transcoder, err := kyusu.NewTranscoder(&config.Kyusu{Encoder: config.EncoderX264, VAAPIDevice: "", WorkDir: ""})
	require.NoError(t, err)
	preset, ok := kyusu.PresetByName(kyusu.PassthroughPreset)
	require.True(t, ok)
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	src := &loopReader{data: loadTestTS(t), pos: 0}
	done := make(chan error, 1)
	go func() {
		done <- transcoder.RunVOD(ctx, src, preset, kyusu.AudioFirst, dir, &kyusu.RunOptions{StartNumber: 0, Trim: 0, Cuts: []int64{0}})
	}()
	require.Eventually(t, func() bool {
		partial, err := filepath.Glob(filepath.Join(dir, "*.tmp"))

		return err == nil && len(partial) > 0
	}, 30*time.Second, 10*time.Millisecond, "the first segment is being written")

	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(10 * time.Second):
		t.Fatal("the transcoder did not stop")
	}
	// 書き終えたセグメントは置いたまま残る。書きかけのものだけを捨てる。
	partial, err := filepath.Glob(filepath.Join(dir, "*.tmp"))
	require.NoError(t, err)
	assert.Empty(t, partial, "the temporary file of the partial segment is removed")
}

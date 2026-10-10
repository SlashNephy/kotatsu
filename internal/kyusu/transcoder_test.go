package kyusu_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
			require.NoError(t, transcoder.Run(t.Context(), bytes.NewReader(loadTestTS(t)), preset, dir))

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
		done <- transcoder.Run(ctx, src, preset, dir)
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
		done <- transcoder.Run(t.Context(), src, preset, dir)
	}()

	select {
	case err := <-done:
		require.ErrorContains(t, err, "FFmpeg failed")
	case <-time.After(30 * time.Second):
		t.Fatal("the transcoder did not return after FFmpeg failed")
	}
}

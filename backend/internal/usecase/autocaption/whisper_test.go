package autocaption_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/usecase/autocaption"
)

// brokenModel は、偽の whisper-cli が、読めずに失敗するモデルのファイルの名前。
const brokenModel = "broken.bin"

// TestMain は、このテストのバイナリが whisper-cli の引数 (-ovtt) で起動されたら、偽の whisper-cli として動く。
// 偽の whisper-cli は、受け取った引数を -of のディレクトリの args に書き、進み具合を標準エラー出力に出し、-of のパスに WebVTT を書く。
// モデルの名前が brokenModel なら、読めずに失敗する。
func TestMain(m *testing.M) {
	if slices.Contains(os.Args[1:], "-ovtt") {
		os.Exit(fakeWhisper(os.Args[1:]))
	}
	os.Exit(m.Run())
}

func fakeWhisper(args []string) int {
	value := func(flag string) string {
		if i := slices.Index(args, flag); i >= 0 && i+1 < len(args) {
			return args[i+1]
		}

		return ""
	}
	out := value("-of")
	if filepath.Base(value("-m")) == brokenModel {
		fmt.Fprintln(os.Stderr, "whisper_init_from_file_with_params_no_state: failed to load model")

		return 1
	}
	fmt.Fprintln(os.Stderr, "whisper_init_from_file_with_params_no_state: loading model")
	fmt.Fprintln(os.Stderr, "whisper_print_progress_callback: progress =  40%")
	fmt.Fprintln(os.Stderr, "whisper_print_progress_callback: progress = 100%")
	// 受け取った引数は、NOTE の塊に 1 行ずつ書く。WebVTT を読むときは読み飛ばす。
	vtt := "WEBVTT\n\nNOTE\n" + strings.Join(args, "\n") + "\n\n00:00:01.000 --> 00:00:02.500\nこんにちは\n\n00:00:03.000 --> 00:00:04.000\n\n"
	// 書くのは -of のディレクトリの中だけにする。
	root, err := os.OpenRoot(filepath.Dir(out))
	if err != nil {
		return 1
	}
	defer func() { _ = root.Close() }()
	if err := root.WriteFile(filepath.Base(out)+".vtt", []byte(vtt), 0o600); err != nil {
		return 1
	}

	return 0
}

// selfCommand は、偽の whisper-cli として動く、このテストのバイナリのパスを返す。
func selfCommand(t *testing.T) string {
	t.Helper()

	self, err := os.Executable()
	require.NoError(t, err)

	return self
}

// writeModel は、dir の下の rel にモデルの代わりのファイルを書き、パスを返す。
func writeModel(t *testing.T, dir, rel string) string {
	t.Helper()

	path := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte("ggml"), 0o600))

	return path
}

func TestNewWhisperNotConfigured(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	model := writeModel(t, dir, "model.bin")
	command := filepath.Join(dir, "missing-whisper-cli")
	executable, err := os.Executable()
	require.NoError(t, err)
	tests := []struct {
		name    string
		cfg     config.Captions
		dataDir string
	}{
		{name: "whisper-cli が見つからない", cfg: config.Captions{WhisperCommand: command, WhisperModel: model, Threads: 0}, dataDir: ""},
		{name: "モデルが見つからない", cfg: config.Captions{WhisperCommand: executable, WhisperModel: filepath.Join(dir, "none.bin"), Threads: 0}, dataDir: ""},
		{name: "モデルのパスがディレクトリ", cfg: config.Captions{WhisperCommand: executable, WhisperModel: dir, Threads: 0}, dataDir: ""},
		{name: "モデルを省き、データディレクトリにもない", cfg: config.Captions{WhisperCommand: executable, WhisperModel: "", Threads: 0}, dataDir: dir},
		{name: "モデルを省き、データディレクトリもない", cfg: config.Captions{WhisperCommand: executable, WhisperModel: "", Threads: 0}, dataDir: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := autocaption.NewWhisper(&tt.cfg, tt.dataDir, nil)
			require.ErrorIs(t, err, autocaption.ErrNotConfigured)
		})
	}
}

// TestWhisperTranscribe は、whisper-cli に日本語と、設定のモデルとスレッドの数を渡し、出力した WebVTT の字幕と進み具合を返すことを確かめる。
// モデルを省くと、データディレクトリの whisper/ggml-model.bin を使う。
func TestWhisperTranscribe(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		model       string
		threads     int
		wantThreads string
	}{
		{name: "設定のモデルとスレッドの数を使う", model: "custom.bin", threads: 3, wantThreads: "3"},
		{name: "モデルを省くと、データディレクトリのものを使う", model: "", threads: 2, wantThreads: "2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data := t.TempDir()
			command := selfCommand(t)
			wantModel := writeModel(t, data, autocaption.DefaultModelPath)
			if tt.model != "" {
				wantModel = writeModel(t, data, tt.model)
			}
			model := ""
			if tt.model != "" {
				model = wantModel
			}
			whisper, err := autocaption.NewWhisper(&config.Captions{WhisperCommand: command, WhisperModel: model, Threads: tt.threads}, data, nil)
			require.NoError(t, err)
			assert.Equal(t, filepath.Base(wantModel), whisper.Model())

			work := t.TempDir()
			wav := filepath.Join(work, "0.wav")
			require.NoError(t, os.WriteFile(wav, []byte("RIFF"), 0o600))
			progress := []float64{}
			cues, err := whisper.Transcribe(t.Context(), wav, func(ratio float64) { progress = append(progress, ratio) })
			require.NoError(t, err)
			// 本文の空の字幕は捨てる。
			assert.Equal(t, []autocaption.Cue{{Start: time.Second, End: 2500 * time.Millisecond, Text: "こんにちは"}}, cues)
			assert.Equal(t, []float64{0.4, 1}, progress)

			written, err := os.ReadFile(filepath.Join(work, "0.vtt"))
			require.NoError(t, err)
			args, _, _ := strings.Cut(strings.TrimPrefix(string(written), "WEBVTT\n\nNOTE\n"), "\n\n")
			assert.Equal(t, []string{
				"-m", wantModel, "-f", wav, "-l", "ja", "-t", tt.wantThreads, "-sns", "-pp", "-np", "-ovtt", "-of", filepath.Join(work, "0"),
			}, strings.Split(args, "\n"))
		})
	}
}

// TestWhisperTranscribeFails は、whisper-cli が失敗したら、標準エラー出力の末尾を添えて返すことを確かめる。
func TestWhisperTranscribeFails(t *testing.T) {
	t.Parallel()

	model := writeModel(t, t.TempDir(), brokenModel)
	whisper, err := autocaption.NewWhisper(&config.Captions{WhisperCommand: selfCommand(t), WhisperModel: model, Threads: 1}, "", nil)
	require.NoError(t, err)
	wav := filepath.Join(t.TempDir(), "0.wav")
	_, err = whisper.Transcribe(t.Context(), wav, nil)
	require.ErrorContains(t, err, "failed to load model")
}

func TestThreadsOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		configured int
		cpus       int
		want       int
	}{
		{name: "設定した数を使う", configured: 12, cpus: 4, want: 12},
		{name: "省くと CPU の数の半分", configured: 0, cpus: 8, want: 4},
		{name: "CPU が 1 つでも 1 にする", configured: 0, cpus: 1, want: 1},
		{name: "CPU が多くても 8 までにする", configured: 0, cpus: 32, want: 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, autocaption.ThreadsOf(tt.configured, tt.cpus))
		})
	}
}

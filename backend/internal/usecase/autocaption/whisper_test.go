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

// brokenModel は、モックの whisper-cli が、読めずに失敗するモデルのファイルの名前。
const brokenModel = "broken.bin"

// TestMain は、このテストのバイナリが whisper-cli の引数 (-ovtt) で起動されたら、モックの whisper-cli として動く。
// モックの whisper-cli は、受け取った引数を -of のディレクトリの args に書き、進み具合を標準エラー出力に出し、-of のパスに WebVTT を書く。
// モデルの名前が brokenModel なら、読めずに失敗する。
// -vm で起動されたら、モックの whisper-vad-speech-segments として動き、1.5 秒から 2.1 秒を声の区間として書く。
func TestMain(m *testing.M) {
	if slices.Contains(os.Args[1:], "-ovtt") {
		os.Exit(fakeWhisper(os.Args[1:]))
	}
	if slices.Contains(os.Args[1:], "-vm") {
		fmt.Println("Detected 1 speech segments:")
		fmt.Println("Speech segment 0: start = 150.00, end = 210.00")
		os.Exit(0)
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
	// 受け取った引数は、NOTE のチャンクに 1 行ずつ書く。WebVTT を読むときは読み飛ばす。
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

// selfCommand は、モックの whisper-cli として動く、このテストのバイナリのパスを返す。
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
		{name: "whisper-cli が見つからない", cfg: config.Captions{WhisperCommand: command, WhisperModel: model}, dataDir: ""},
		{name: "モデルが見つからない", cfg: config.Captions{WhisperCommand: executable, WhisperModel: filepath.Join(dir, "none.bin")}, dataDir: ""},
		{name: "モデルのパスがディレクトリ", cfg: config.Captions{WhisperCommand: executable, WhisperModel: dir}, dataDir: ""},
		{name: "モデルを省き、データディレクトリにもない", cfg: config.Captions{WhisperCommand: executable, WhisperModel: ""}, dataDir: dir},
		{name: "モデルを省き、データディレクトリもない", cfg: config.Captions{WhisperCommand: executable, WhisperModel: ""}, dataDir: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := autocaption.NewWhisper(&tt.cfg, 0, tt.dataDir, nil)
			require.ErrorIs(t, err, autocaption.ErrNotConfigured)
		})
	}
}

// TestWhisperTranscribe は、whisper-cli に日本語と、設定のモデルとスレッドの数を渡し、出力した WebVTT の字幕と進み具合を返すことを確認する。
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
			whisper, err := autocaption.NewWhisper(&config.Captions{WhisperCommand: command, WhisperModel: model}, tt.threads, data, nil)
			require.NoError(t, err)
			assert.Equal(t, filepath.Base(wantModel), whisper.Model())

			work := t.TempDir()
			wav := filepath.Join(work, "0.wav")
			require.NoError(t, os.WriteFile(wav, []byte("RIFF"), 0o600))
			progress := []float64{}
			cues, err := whisper.Transcribe(t.Context(), wav, func(ratio float64) { progress = append(progress, ratio) })
			require.NoError(t, err)
			// 本文の空の字幕は破棄する。
			// 字幕の終わりは、読む時間の見積もり (0.5 + 5 文字 × 0.15 = 1.25 秒) で切る。
			assert.Equal(t, []autocaption.Cue{{Start: time.Second, End: 2250 * time.Millisecond, Text: "こんにちは"}}, cues)
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

// TestWhisperTranscribeWithVAD は、VAD を置いていれば、字幕の時刻を声の区間に合わせることを確認する。
func TestWhisperTranscribeWithVAD(t *testing.T) {
	t.Parallel()

	data := t.TempDir()
	model := writeModel(t, data, autocaption.DefaultModelPath)
	writeModel(t, data, autocaption.DefaultVADModelPath)
	whisper, err := autocaption.NewWhisper(&config.Captions{
		WhisperCommand: selfCommand(t), WhisperModel: model, VADCommand: selfCommand(t), VADModel: "",
	}, 1, data, nil)
	require.NoError(t, err)
	wav := filepath.Join(t.TempDir(), "0.wav")
	require.NoError(t, os.WriteFile(wav, []byte("RIFF"), 0o600))
	cues, err := whisper.Transcribe(t.Context(), wav, nil)
	require.NoError(t, err)
	// whisper の 1.0〜2.5 秒の字幕を、声の始まり (1.5 秒) から出す。終わりは whisper の終わりを超えない。
	assert.Equal(t, []autocaption.Cue{{Start: 1500 * time.Millisecond, End: 2500 * time.Millisecond, Text: "こんにちは"}}, cues)
}

func TestNewVAD(t *testing.T) {
	t.Parallel()

	data := t.TempDir()
	placed := writeModel(t, data, autocaption.DefaultVADModelPath)
	tests := []struct {
		name    string
		cfg     config.Captions
		dataDir string
		wantErr error
	}{
		{name: "既定の場所にコマンドがなければ使わない", cfg: config.Captions{VADCommand: "", VADModel: ""}, dataDir: data, wantErr: autocaption.ErrVADNotPlaced},
		{name: "既定の場所にモデルがなければ使わない", cfg: config.Captions{VADCommand: "", VADModel: ""}, dataDir: t.TempDir(), wantErr: autocaption.ErrVADNotPlaced},
		{name: "設定に書いたコマンドがなければ設定の誤り", cfg: config.Captions{VADCommand: filepath.Join(data, "missing"), VADModel: placed}, dataDir: data, wantErr: autocaption.ErrNotConfigured},
		{name: "設定に書いたモデルがなければ設定の誤り", cfg: config.Captions{VADCommand: selfCommand(t), VADModel: filepath.Join(data, "missing.bin")}, dataDir: data, wantErr: autocaption.ErrNotConfigured},
		{name: "置いてあれば使う", cfg: config.Captions{VADCommand: selfCommand(t), VADModel: ""}, dataDir: data, wantErr: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			vad, err := autocaption.NewVAD(&tt.cfg, tt.dataDir, nil)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, vad)

				return
			}
			require.NoError(t, err)
			assert.NotNil(t, vad)
		})
	}
}

// TestWhisperTranscribeFails は、whisper-cli が失敗したら、標準エラー出力の末尾を添えて返すことを確認する。
func TestWhisperTranscribeFails(t *testing.T) {
	t.Parallel()

	model := writeModel(t, t.TempDir(), brokenModel)
	whisper, err := autocaption.NewWhisper(&config.Captions{WhisperCommand: selfCommand(t), WhisperModel: model}, 1, "", nil)
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

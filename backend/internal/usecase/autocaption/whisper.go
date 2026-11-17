package autocaption

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/lib/procprio"
)

const (
	// whisperCommand は、設定で whisper-cli のパスを省いたときに、PATH から探すコマンドの名前。
	whisperCommand = "whisper-cli"
	// DefaultModelPath は、設定でモデルのパスを省いたときの、データディレクトリの中のモデルのパス。
	DefaultModelPath = "whisper/ggml-model.bin"
	// language は、音声の言語。日本の放送の録画なので、日本語に決める。自動で見分けさせると、音楽や無音の所で外れることがある。
	language = "ja"
	// maxThreads は、スレッドの数を自動で決めるときの上限。8 を超えても速さはあまり伸びない (#1682 の計測)。
	maxThreads = 8
	// progressPrefix は、whisper-cli の -pp が標準エラー出力に出す進み具合の行の前置き。
	progressPrefix = "whisper_print_progress_callback: progress ="
	// maxStderrLines は、失敗したときに、エラーに含める whisper-cli の標準エラー出力の行の数。
	maxStderrLines = 5
)

// ErrNotConfigured は、字幕の生成に使う whisper-cli かモデルが見つからないことを表す。管理者が設定の captions に置き場所を書くか、
// 既定の場所 (whisper-cli は PATH、モデルはデータディレクトリの whisper/ggml-model.bin) に置く (設計書 6.1)。モデルは画面からダウンロードできる。
// 録画の詳細は、ジョブがこの理由で取りやめたことを見分けて、管理者にモデルを置く画面へのリンクを出す。
var ErrNotConfigured = errors.New("whisper.cpp is not configured")

// Transcriber は、16 kHz のモノラルの WAV から字幕を作る。
type Transcriber interface {
	// Model は、DB に残す、使うモデルの名前。
	Model() string
	// Transcribe は、wav の音声の字幕を、音声の始まりからの時間で、始まりの順に返す。
	// progress には、0 から 1 の進み具合を渡す。nil なら渡さない。
	Transcribe(ctx context.Context, wav string, progress func(ratio float64)) ([]Cue, error)
}

// Whisper は、whisper.cpp の whisper-cli を子プロセスとして動かす Transcriber。本体とはリンクしない (設計書 付録 B)。
// 録画の後処理なので、OS の最低の優先度で動かし、ライブ視聴の変換 (kyusu) に CPU を譲る。
type Whisper struct {
	command string
	model   string
	threads int
	lowerer *procprio.Lowerer
	// vad は、字幕の時刻を声のある区間に合わせる VAD。置いていなければ nil で、読む時間の見積もりだけで直す。
	vad *VAD
}

var _ Transcriber = new(Whisper)

// NewWhisper は、設定 cfg の whisper-cli とモデルを使う Whisper を返す。dataDir は、モデルのパスを省いたときに探すデータディレクトリ。
// threads は、whisper-cli に使わせるスレッドの数で、0 なら CPU の数から決める。管理者が画面で決める (#1869)。
// whisper-cli かモデルが見つからなければ、ErrNotConfigured を返す。lowerer は、whisper-cli を最低の優先度で動かす。nil なら優先度を変えない。
func NewWhisper(cfg *config.Captions, threads int, dataDir string, lowerer *procprio.Lowerer) (*Whisper, error) {
	command := cfg.WhisperCommand
	if command == "" {
		command = whisperCommand
	}
	resolved, err := exec.LookPath(command)
	if err != nil {
		return nil, fmt.Errorf("%w: whisper-cli is not found: %w", ErrNotConfigured, err)
	}
	model := cfg.WhisperModel
	if model == "" {
		if dataDir == "" {
			return nil, fmt.Errorf("%w: the model path is not set", ErrNotConfigured)
		}
		model = filepath.Join(dataDir, filepath.FromSlash(DefaultModelPath))
	}
	if info, err := os.Stat(model); err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: the model is not found at %s", ErrNotConfigured, model)
	}

	vad, err := NewVAD(cfg, dataDir, lowerer)
	if errors.Is(err, ErrVADNotPlaced) {
		vad = nil
	} else if err != nil {
		return nil, err
	}

	return &Whisper{command: resolved, model: model, threads: threadsOf(threads, runtime.NumCPU()), lowerer: lowerer, vad: vad}, nil
}

// threadsOf は、whisper-cli に使わせるスレッドの数を返す。configured が 0 なら、CPU の数の半分 (1 から maxThreads) にする。
func threadsOf(configured, cpus int) int {
	if configured > 0 {
		return configured
	}

	return min(max(cpus/2, 1), maxThreads)
}

// Model は、モデルのファイルの名前を返す。
func (w *Whisper) Model() string { return filepath.Base(w.model) }

// Transcribe は、whisper-cli に wav を読ませ、出力した WebVTT を読む。WebVTT は、wav と同じディレクトリに書かせる。
func (w *Whisper) Transcribe(ctx context.Context, wav string, progress func(ratio float64)) ([]Cue, error) {
	out := strings.TrimSuffix(wav, filepath.Ext(wav))
	// -sns は、音楽の記号などの、言葉でないトークンを出させない。-np は、字幕を標準出力に出させない (進み具合の行は出る)。
	// コマンドは、NewWhisper が管理者の設定から LookPath で引いたパスなので、PATH から引き直さないよう、exec.Cmd を直に組む。
	// そのため、ctx が終わったら、自分でプロセスを止める。
	cmd := &exec.Cmd{
		Path: w.command,
		Args: []string{
			w.command, "-m", w.model, "-f", wav, "-l", language, "-t", strconv.Itoa(w.threads),
			"-sns", "-pp", "-np", "-ovtt", "-of", out,
		},
		Stdout: io.Discard,
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to open the whisper-cli stderr: %w", err)
	}
	if err := w.lowerer.Start(cmd); err != nil {
		return nil, fmt.Errorf("failed to start whisper-cli: %w", err)
	}
	stop := context.AfterFunc(ctx, func() { _ = cmd.Process.Kill() })
	defer stop()
	tail := readStderr(stderr, progress)
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("whisper-cli was stopped: %w", ctx.Err())
		}

		return nil, fmt.Errorf("failed to transcribe with whisper-cli: %w: %s", err, strings.Join(tail, "; "))
	}
	file, err := os.Open(out + ".vtt")
	if err != nil {
		return nil, fmt.Errorf("failed to open the WebVTT written by whisper-cli: %w", err)
	}
	defer func() { _ = file.Close() }()
	cues, err := ParseVTT(file)
	if err != nil {
		return nil, err
	}
	// 字幕の時刻を、声のある区間 (VAD を置いていれば) と読む時間の見積もりで直す。
	var speech []Span
	if w.vad != nil {
		speech, err = w.vad.Speech(ctx, wav)
		if err != nil {
			return nil, err
		}
	}

	return FitCues(cues, speech), nil
}

// readStderr は、whisper-cli の標準エラー出力を最後まで読み、進み具合の行を progress に渡す。ほかの行は、末尾の数行を返す。
func readStderr(r io.Reader, progress func(ratio float64)) []string {
	tail := []string{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if value, ok := strings.CutPrefix(line, progressPrefix); ok {
			percent, err := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), "%")))
			if err == nil && progress != nil {
				progress(float64(min(max(percent, 0), 100)) / 100)
			}

			continue
		}
		tail = append(tail, line)
		if len(tail) > maxStderrLines {
			tail = tail[1:]
		}
	}
	// 読み残すと whisper-cli が書き込みで止まるので、読めなくなっても最後まで捨てる。
	_, _ = io.Copy(io.Discard, r)

	return tail
}

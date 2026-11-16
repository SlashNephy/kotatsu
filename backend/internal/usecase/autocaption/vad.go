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
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/lib/procprio"
)

const (
	// vadCommand は、設定で VAD のコマンドのパスを省いたときに、PATH から探すコマンドの名前。
	// whisper.cpp の examples/vad-speech-segments をビルドしたもの。
	vadCommand = "whisper-vad-speech-segments"
	// DefaultVADModelPath は、設定で VAD のモデルのパスを省いたときの、データディレクトリの中のモデルのパス。
	DefaultVADModelPath = "whisper/ggml-silero-vad.bin"
	// vadMinSilenceMs は、VAD が声の区間を分ける、最短の無音の長さ (ミリ秒)。台詞の間の短い間で分けるよう、既定の 100 より長くする。
	vadMinSilenceMs = 200
	// vadPadMs は、VAD が声の区間の前後にのばす長さ (ミリ秒)。台詞の頭と尻の弱い音を切らないようにする。
	vadPadMs = 100
)

// ErrVADNotPlaced は、VAD のコマンドかモデルが既定の場所にないことを表す。VAD は任意なので、字幕の生成は VAD なしで続ける。
var ErrVADNotPlaced = errors.New("the VAD is not placed")

// speechLine は、whisper-vad-speech-segments が標準出力に書く、声の区間の行。時刻は 10 ミリ秒の単位。
var speechLine = regexp.MustCompile(`^Speech segment \d+: start = ([0-9.]+), end = ([0-9.]+)$`)

// VAD は、whisper.cpp の whisper-vad-speech-segments を子プロセスとして動かし、音声の中の声の区間を求める。
// 書き起こしそのものは VAD なしで行い (VAD で切った音声を読ませると、時刻と文が崩れた。#1682)、字幕の時刻を声に合わせるのにだけ使う。
type VAD struct {
	command string
	model   string
	lowerer *procprio.Lowerer
}

// NewVAD は、設定 cfg の VAD のコマンドとモデルを使う VAD を返す。どちらも任意なので、既定の場所になければ ErrVADNotPlaced を返し、
// 呼ぶ側は、字幕の時刻を読む時間の見積もりだけで直す。設定で場所を書いたのに見つからなければ、ErrNotConfigured を返す。
func NewVAD(cfg *config.Captions, dataDir string, lowerer *procprio.Lowerer) (*VAD, error) {
	explicit := cfg.VADCommand != "" || cfg.VADModel != ""
	command := cfg.VADCommand
	if command == "" {
		command = vadCommand
	}
	resolved, err := exec.LookPath(command)
	if err != nil {
		if explicit {
			return nil, fmt.Errorf("%w: %s is not found: %w", ErrNotConfigured, command, err)
		}

		return nil, ErrVADNotPlaced
	}
	model := cfg.VADModel
	if model == "" {
		if dataDir == "" {
			if explicit {
				return nil, fmt.Errorf("%w: the VAD model path is not set", ErrNotConfigured)
			}

			return nil, ErrVADNotPlaced
		}
		model = filepath.Join(dataDir, filepath.FromSlash(DefaultVADModelPath))
	}
	if info, err := os.Stat(model); err != nil || !info.Mode().IsRegular() {
		if explicit {
			return nil, fmt.Errorf("%w: the VAD model is not found at %s", ErrNotConfigured, model)
		}

		return nil, ErrVADNotPlaced
	}

	return &VAD{command: resolved, model: model, lowerer: lowerer}, nil
}

// Speech は、wav の中の声の区間を、始まりの順に返す。
func (v *VAD) Speech(ctx context.Context, wav string) ([]Span, error) {
	// コマンドは、NewVAD が LookPath で引いたパスなので、PATH から引き直さないよう、exec.Cmd を直に組む。
	cmd := &exec.Cmd{
		Path: v.command,
		Args: []string{
			v.command, "-f", wav, "-vm", v.model,
			"-vsd", strconv.Itoa(vadMinSilenceMs), "-vp", strconv.Itoa(vadPadMs),
		},
		Stderr: io.Discard,
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to open the VAD stdout: %w", err)
	}
	if err := v.lowerer.Start(cmd); err != nil {
		return nil, fmt.Errorf("failed to start the VAD: %w", err)
	}
	stop := context.AfterFunc(ctx, func() { _ = cmd.Process.Kill() })
	defer stop()
	spans, parseErr := ParseSpeech(stdout)
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("the VAD was stopped: %w", ctx.Err())
		}

		return nil, fmt.Errorf("failed to detect speech: %w", err)
	}
	if parseErr != nil {
		return nil, parseErr
	}

	return spans, nil
}

// ParseSpeech は、whisper-vad-speech-segments の標準出力から、声の区間を読む。ほかの行は読み飛ばす。
func ParseSpeech(r io.Reader) ([]Span, error) {
	spans := []Span{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		match := speechLine.FindStringSubmatch(strings.TrimSpace(scanner.Text()))
		if match == nil {
			continue
		}
		start, err := strconv.ParseFloat(match[1], 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse the speech start %q: %w", match[1], err)
		}
		end, err := strconv.ParseFloat(match[2], 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse the speech end %q: %w", match[2], err)
		}
		spans = append(spans, Span{Start: centiseconds(start), End: centiseconds(end)})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read the VAD output: %w", err)
	}

	return spans, nil
}

// centiseconds は、10 ミリ秒の単位の値を time.Duration にする。
func centiseconds(v float64) time.Duration {
	return time.Duration(v * float64(10*time.Millisecond))
}

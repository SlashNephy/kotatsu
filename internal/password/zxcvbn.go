package password

import (
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/dop251/goja"
)

// zxcvbnBundle は、web/zxcvbn から作った zxcvbn-ts のバンドル。mise run generate で作り直す。
//
//go:embed zxcvbn.js.gz
var zxcvbnBundle []byte

const (
	// MaxStrengthInput は、zxcvbn-ts に渡すパスワードの長さ (文字数) の上限。
	// zxcvbn-ts の計算量は入力の長さに対して急に増えるので、これより長いパスワードは判定せずに十分強いとみなす。
	// 128 文字の総当たりは現実的でないからである。
	MaxStrengthInput = 128
	// maxUserInputs は、判定に使うユーザー名などの語の数の上限。
	maxUserInputs = 8
	// DefaultStrengthTimeout は、1 回の判定の時間の上限の既定値。ふつうは 0.5 秒ほどで終わる。
	// 入力の長さを MaxStrengthInput までにしているので、これを超えるのは、マシンがひどく混んでいるときである。
	DefaultStrengthTimeout = 5 * time.Second
)

// ErrStrengthTimeout は、強度の判定が時間内に終わらなかったことを表す。
var ErrStrengthTimeout = errors.New("password strength check timed out")

// Strength は、zxcvbn-ts の判定の結果。
type Strength struct {
	// Score は 0〜4。
	Score        int      `json:"score"`
	GuessesLog10 float64  `json:"guessesLog10"`
	Warning      *string  `json:"warning"`
	Suggestions  []string `json:"suggestions"`
}

// StrengthEstimator は、パスワードの強度を判定するもの。
type StrengthEstimator interface {
	Estimate(ctx context.Context, password string, userInputs []string) (*Strength, error)
}

var _ StrengthEstimator = new(Estimator)

// Estimator は、パスワードの強度を zxcvbn-ts で判定する (設計書 2.2)。
type Estimator struct {
	timeout time.Duration
	// slots は、同時に動かす判定の数を抑える。判定 1 回で VM が 37 MiB ほど使うので、同時に呼ばれてもメモリが急に増えないようにする (設計書 2.2)。
	slots chan struct{}

	// compile は、バンドルを解凍してコンパイルする。最初に判定するときに 1 回だけ行う。
	// 起動のたびにコンパイルすると、パスワードを扱わないロールまで起動が遅くなるからである。
	compile func() (*goja.Program, error)
}

// NewEstimator は Estimator を返す。埋め込んだバンドルは、最初に判定するときにコンパイルする。
// timeout は 1 回の判定の時間の上限で、0 なら DefaultStrengthTimeout にする。
func NewEstimator(timeout time.Duration) *Estimator {
	if timeout <= 0 {
		timeout = DefaultStrengthTimeout
	}

	return &Estimator{timeout: timeout, slots: make(chan struct{}, maxConcurrentEstimates), compile: sync.OnceValues(compileBundle)}
}

// compileBundle は、埋め込んだバンドルを解凍してコンパイルする。コンパイルしたものは、VM をまたいで使い回せる。
func compileBundle() (*goja.Program, error) {
	r, err := gzip.NewReader(bytes.NewReader(zxcvbnBundle))
	if err != nil {
		return nil, fmt.Errorf("failed to open the zxcvbn bundle: %w", err)
	}
	source, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read the zxcvbn bundle: %w", err)
	}
	program, err := goja.Compile("zxcvbn.js", string(source), true)
	if err != nil {
		return nil, fmt.Errorf("failed to compile the zxcvbn bundle: %w", err)
	}

	return program, nil
}

// maxConcurrentEstimates は、同時に動かす判定の数の上限。
const maxConcurrentEstimates = 2

// Estimate は、パスワードの強度を判定する。
//
// VM を判定のたびに作って捨てる。VM を保持すると、ヒープを 37 MiB ほど使い続けるからである。
// 判定はパスワードの設定と変更のときにしか行わないので、毎回作る時間 (0.3 秒ほど) は許容する。
// userInputs には、ユーザー名のように、パスワードに使われると推測しやすい語を渡す。
func (e *Estimator) Estimate(ctx context.Context, password string, userInputs []string) (*Strength, error) {
	if utf8.RuneCountInString(password) > MaxStrengthInput {
		return &Strength{Score: 4, GuessesLog10: 0, Warning: nil, Suggestions: []string{}}, nil
	}
	switch {
	case userInputs == nil:
		userInputs = []string{}
	case len(userInputs) > maxUserInputs:
		userInputs = userInputs[:maxUserInputs]
	}
	// 判定の枠が空くのを待つ。待つ時間も、1 回の判定の時間の上限に含める。
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	select {
	case e.slots <- struct{}{}:
		defer func() { <-e.slots }()
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: %w", ErrStrengthTimeout, context.Cause(ctx))
	}
	program, err := e.compile()
	if err != nil {
		return nil, err
	}
	input, err := json.Marshal(map[string]any{"password": password, "userInputs": userInputs})
	if err != nil {
		return nil, fmt.Errorf("failed to encode the strength input: %w", err)
	}

	vm := goja.New()
	stop := context.AfterFunc(ctx, func() {
		vm.Interrupt(ErrStrengthTimeout)
	})
	defer stop()

	if _, err := vm.RunProgram(program); err != nil {
		return nil, interrupted(err, "failed to load zxcvbn")
	}
	check, ok := goja.AssertFunction(vm.Get("kotatsuCheckStrength"))
	if !ok {
		return nil, errors.New("the zxcvbn bundle does not define kotatsuCheckStrength")
	}
	result, err := check(goja.Undefined(), vm.ToValue(string(input)))
	if err != nil {
		return nil, interrupted(err, "failed to check the password strength")
	}

	var strength Strength
	if err := json.Unmarshal([]byte(result.String()), &strength); err != nil {
		return nil, fmt.Errorf("failed to decode the strength: %w", err)
	}

	return &strength, nil
}

// interrupted は、VM を止めたことによるエラーなら ErrStrengthTimeout にする。
func interrupted(err error, message string) error {
	if _, ok := errors.AsType[*goja.InterruptedError](err); ok {
		return ErrStrengthTimeout
	}

	return fmt.Errorf("%s: %w", message, err)
}

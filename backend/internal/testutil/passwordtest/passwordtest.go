// Package passwordtest は、パスワードの強度の判定をテストで差し替えるものを提供する。
//
// zxcvbn-ts は、-race では 1 回の判定に数十秒かかる。強度の判定そのものを確かめないテストでは、これを使う。
// zxcvbn-ts での判定は、password パッケージのテストで確かめる。
package passwordtest

import (
	"context"
	"strings"

	"github.com/SlashNephy/kotatsu/internal/lib/password"
)

var _ password.StrengthEstimator = new(Estimator)

// Estimator は、Scores に載せたパスワードにはそのスコアを返す。
// ユーザー名などの推測しやすい語 (userInputs) を含むパスワードには、zxcvbn-ts と同じく 0 (弱すぎる) を返す。
// ほかのパスワードには 4 (十分強い) を返す。
type Estimator struct {
	Scores map[string]int
}

// NewEstimator は、weak に渡したパスワードだけを、スコア 0 (弱すぎる) とする Estimator を返す。
func NewEstimator(weak ...string) *Estimator {
	scores := make(map[string]int, len(weak))
	for _, phrase := range weak {
		scores[phrase] = 0
	}

	return &Estimator{Scores: scores}
}

// Estimate は、パスワードの強度を返す。
func (e *Estimator) Estimate(_ context.Context, phrase string, userInputs []string) (*password.Strength, error) {
	score, ok := e.Scores[phrase]
	if !ok {
		score = 4
	}
	for _, input := range userInputs {
		if input != "" && strings.Contains(strings.ToLower(phrase), strings.ToLower(input)) {
			score = 0
		}
	}

	return &password.Strength{Score: score, GuessesLog10: 0, Warning: nil, Suggestions: []string{}}, nil
}

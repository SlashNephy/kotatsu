package password_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/password"
)

// testStrengthTimeout は、テストでの判定の時間の上限。-race では goja が 10 倍以上遅くなるので、長くとる。
const testStrengthTimeout = 10 * time.Minute

// warningUserInputs は、ユーザーの入力に似ているときの警告文。
const warningUserInputs = "個人情報やページに関する情報は含めるべきではありません"

// TestEstimateMatchesNode は、goja で動かした zxcvbn-ts が、Node.js で動かした同じバンドルと同じ結果を返すことを確かめる。
// 期待値は、frontend/zxcvbn/dist/zxcvbn.js を Node.js の vm で読み込んで得た (設計書 2.2)。
func TestEstimateMatchesNode(t *testing.T) {
	t.Parallel()

	estimator := password.NewEstimator(testStrengthTimeout)

	tests := []struct {
		phrase       string
		userInputs   []string
		score        int
		guessesLog10 float64
		warning      string
	}{
		{phrase: "password", userInputs: []string{}, score: 0, guessesLog10: 0.4771212547196623, warning: "一般的に使用されるパスワードです"},
		{phrase: "kotatsu", userInputs: []string{}, score: 1, guessesLog10: 4.20682587603185},
		{phrase: "こたつでみかん", userInputs: []string{}, score: 2, guessesLog10: 7.000000043429445},
		{phrase: "猫はこたつで丸くなる", userInputs: []string{}, score: 3, guessesLog10: 10.000000000043428},
		{phrase: "correcthorsebatterystaple", userInputs: []string{}, score: 4, guessesLog10: 12.79742635911589},
		{phrase: "Tr0ub4dor&3", userInputs: []string{}, score: 4, guessesLog10: 10.519827993775719},
		{phrase: "kotatsu2026", userInputs: []string{}, score: 3, guessesLog10: 8.00395572454269},
		{phrase: "spica1234", userInputs: []string{"spica"}, score: 1, guessesLog10: 4.176091259055681, warning: warningUserInputs},
		{phrase: "mikan-kotatsu-neko-2026!", userInputs: []string{}, score: 4, guessesLog10: 20.167449670997208},
		// l33t の照合はパッチで速くしている (frontend/patches)。期待値は、パッチを当てる前の zxcvbn-ts を Node.js で動かして得た。
		{phrase: "p4ssw0rd", userInputs: []string{}, score: 0, guessesLog10: 0.9542425094393249, warning: "よく使われるパスワードに似ています"},
		{phrase: "P4$$w0rdP4$$w0rd", userInputs: []string{}, score: 0, guessesLog10: 1.8260748027008262, warning: `"abcabcabc"のような繰り返しの多い文字は推測されやすくなります`},
		{phrase: "|<0t4t$u", userInputs: []string{}, score: 1, guessesLog10: 4.53655844257153},
		{phrase: "m1k4n-k0t4t$u-n3k0", userInputs: []string{}, score: 4, guessesLog10: 15.057468306570247},
		{phrase: "q7#Vt9!mZ2@xLp4$Rk8&Wn3*Hj6^Bd5%Fs1(Gc0)Ya2-Ue7_Io9+Pw4=Ks8;Nm3:", userInputs: []string{}, score: 4, guessesLog10: 64},
		{phrase: strings.Repeat("k0t4t$u!", 16), userInputs: []string{}, score: 3, guessesLog10: 8.756666531372117},
		// 辞書の照合は、辞書の語に現れない 2 文字の組を含む部分を引かない (frontend/patches)。ユーザーの入力の語の組は、その場で加える。
		{phrase: "zqxjvw2026", userInputs: []string{"zqxjvw"}, score: 1, guessesLog10: 4.176091259055681, warning: warningUserInputs},
		{phrase: "2qxjvw!2026", userInputs: []string{"zqxjvw"}, score: 2, guessesLog10: 7.000434077479318, warning: warningUserInputs},
	}
	for _, tt := range tests {
		t.Run(tt.phrase, func(t *testing.T) {
			t.Parallel()

			got, err := estimator.Estimate(t.Context(), tt.phrase, tt.userInputs)
			require.NoError(t, err)
			assert.Equal(t, tt.score, got.Score)
			assert.InDelta(t, tt.guessesLog10, got.GuessesLog10, 1e-9)
			if tt.warning == "" {
				assert.Nil(t, got.Warning)
			} else {
				assert.Equal(t, &tt.warning, got.Warning)
			}
		})
	}
}

func TestEstimateLimits(t *testing.T) {
	t.Parallel()

	estimator := password.NewEstimator(testStrengthTimeout)

	// 上限より長い入力は、VM に渡さずに十分強いとみなす。
	got, err := estimator.Estimate(t.Context(), strings.Repeat("a", password.MaxStrengthInput+1), nil)
	require.NoError(t, err)
	assert.Equal(t, 4, got.Score)

	// 時間切れの context では、判定を打ち切る。
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = estimator.Estimate(ctx, "kotatsu", nil)
	require.ErrorIs(t, err, password.ErrStrengthTimeout)
}

package streamapi

import (
	"crypto/rand"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
)

const (
	// ProbePath は、プローブのデータのパス (設計書 7.3、#1513)。
	ProbePath = "/internal/streams/probe"
	// ProbeMinBytes と ProbeMaxBytes は、1 回に返すプローブのデータの量の下限と上限。範囲の外は 400 にする。
	// 下限は、往復の待ちに埋もれずに速さを測れる量。上限は、再生中の映像を細らせない量。どちらも初期値 (設計書の付録 A)。
	ProbeMinBytes = 64 << 10
	ProbeMaxBytes = 8 << 20
	// ProbeInterval は、同じユーザーにプローブのデータを返す最短の間隔 (プローブの間隔の制限)。画面が試す間隔 (30 秒) より短くし、
	// ふつうの使い方で 429 にならないようにする。初期値 (設計書の付録 A)。
	ProbeInterval = 5 * time.Second
	// probeBlockSize は、使い回す乱数の大きさ。返す量がこれより多ければ、繰り返して返す。
	probeBlockSize = 256 << 10
)

// ProbeHandler は、画質を上げる前に回線の余裕を確認する、プローブのデータを返す (GET /internal/streams/probe?bytes=N、設計書 7.3、#1513)。
// 内容は、起動したときに作った乱数の使い回し。圧縮されないように乱数にする。キャッシュさせない。
// 同じユーザーには、ProbeInterval に 1 回まで返し、超えたら 429 と Retry-After を返す。数えるのはこのインスタンスの中だけ。
// ログイン中のセッションでだけ受け付けるので (Authorizer.Session で包む)、ユーザーごとに数える。
type ProbeHandler struct {
	clock    clock.Clock
	interval time.Duration
	block    []byte

	mu sync.Mutex
	// last は、ユーザーの ID ごとの、最後に返した時刻。間隔より古いものは、次のリクエストで捨てる。
	last map[string]time.Time
}

var _ http.Handler = new(ProbeHandler)

// NewProbeHandler は ProbeHandler を返す。
func NewProbeHandler(clk clock.Clock) *ProbeHandler {
	block := make([]byte, probeBlockSize)
	// crypto/rand.Read は、失敗するとプログラムを止め、エラーを返さない。
	_, _ = rand.Read(block)

	return &ProbeHandler{clock: clk, interval: ProbeInterval, block: block, mu: sync.Mutex{}, last: map[string]time.Time{}}
}

func (h *ProbeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFrom(r.Context())
	if principal == nil {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)

		return
	}
	size, err := strconv.Atoi(r.URL.Query().Get("bytes"))
	if err != nil || size < ProbeMinBytes || size > ProbeMaxBytes {
		http.Error(w, "bytes must be an integer between "+strconv.Itoa(ProbeMinBytes)+" and "+strconv.Itoa(ProbeMaxBytes), http.StatusBadRequest)

		return
	}
	if wait := h.reserve(principal.UserID.String()); wait > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)

		return
	}

	header := w.Header()
	header.Set("Content-Type", "application/octet-stream")
	header.Set("Content-Length", strconv.Itoa(size))
	header.Set("Cache-Control", "no-store")
	header.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	for remaining := size; remaining > 0; {
		// 受け手が切れたら、書くのをやめる。
		if r.Context().Err() != nil {
			return
		}
		n := min(remaining, len(h.block))
		if _, err := w.Write(h.block[:n]); err != nil {
			return
		}
		remaining -= n
	}
}

// reserve は、key にプローブのデータを返してよければ、その時刻を記録して 0 を返す。まだ返せなければ、返せるまでの時間を返す。
func (h *ProbeHandler) reserve(key string) time.Duration {
	now := h.clock.Now()
	h.mu.Lock()
	defer h.mu.Unlock()

	for k, at := range h.last {
		if now.Sub(at) >= h.interval {
			delete(h.last, k)
		}
	}
	if at, ok := h.last[key]; ok {
		return h.interval - now.Sub(at)
	}
	h.last[key] = now

	return 0
}

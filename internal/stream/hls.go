package stream

import (
	"context"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/SlashNephy/kotatsu/internal/kyusu"
	"github.com/SlashNephy/kotatsu/internal/registry"
)

const (
	// kyusuRole は、サーバー側でのトランスコードを受け持つロールの名前。
	kyusuRole = "kyusu"
	// relayTimeout は、kyusu への中継にかける時間の上限。kyusu は、最初のセグメントができるまで、プレイリストを最大 20 秒待つ。
	relayTimeout = 30 * time.Second
)

// Instances は、ロールの動いているインスタンスを引く。*registry.Registry が満たす。
type Instances interface {
	ListAlive(ctx context.Context, role string) ([]*registry.Instance, error)
}

var _ Instances = new(registry.Registry)

// HLSHandler は、ライブ視聴の HLS を、担当の kyusu に中継する (GET /streams/services/{id}/hls/{preset}/{file}) (設計書 7.3)。
//
// 同じサービスを同じ画質で見る視聴者が 1 つのエンコーダーを共有できるよう、(サービス, 画質) から決まった kyusu に中継する。
// kyusu が増えたり減ったりして担当が変わると、見ている視聴者は新しい担当のエンコーダーに移る。
type HLSHandler struct {
	instances Instances
	token     *kyusu.Token
	client    *http.Client
	logger    *slog.Logger
}

var _ http.Handler = new(HLSHandler)

// NewHLSHandler は HLSHandler を返す。token は、kyusu へのリクエストに付ける。
func NewHLSHandler(instances Instances, token *kyusu.Token, logger *slog.Logger) *HLSHandler {
	return &HLSHandler{instances: instances, token: token, client: &http.Client{Timeout: relayTimeout}, logger: logger}
}

func (h *HLSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	serviceID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid service ID", http.StatusBadRequest)

		return
	}
	preset, file := r.PathValue("preset"), r.PathValue("file")
	instances, err := h.instances.ListAlive(r.Context(), kyusuRole)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "failed to list kyusu instances", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		return
	}
	if len(instances) == 0 {
		http.Error(w, "server-side transcoding is not available; no kyusu instance is running", http.StatusServiceUnavailable)

		return
	}
	instance := pick(instances, strconv.FormatInt(serviceID, 10)+"/"+preset)
	target := &url.URL{
		Scheme: "http", Host: instance.Address,
		// Path は、エスケープする前のパス。String でエスケープする。
		Path: kyusu.LivePathPrefix + strconv.FormatInt(serviceID, 10) + "/" + preset + "/" + file,
	}
	if err := h.relay(w, r, target); err != nil && r.Context().Err() == nil {
		h.logger.WarnContext(r.Context(), "failed to relay to kyusu", slog.String("instance", instance.Address), slog.Any("error", err))
	}
}

// relay は、target に GET を送り、状態と本文をそのまま返す。
func (h *HLSHandler) relay(w http.ResponseWriter, r *http.Request, target *url.URL) error {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target.String(), nil)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		return fmt.Errorf("failed to build the request to kyusu: %w", err)
	}
	h.token.Authorize(req)
	resp, err := h.client.Do(req)
	if err != nil {
		http.Error(w, "failed to reach kyusu", http.StatusBadGateway)

		return fmt.Errorf("failed to reach kyusu: %w", err)
	}
	defer resp.Body.Close()
	for _, name := range []string{"Content-Type", "Cache-Control"} {
		if value := resp.Header.Get(name); value != "" {
			w.Header().Set(name, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)

	return err
}

// pick は、key から決まるインスタンスを選ぶ (rendezvous hashing)。インスタンスごとに、key とインスタンスの ID のハッシュを求め、
// 最も大きいものを選ぶ。どの tenban からでも、同じ key には同じインスタンスを選び、インスタンスが増減しても、
// 担当が変わるのは、増えたインスタンスに移る key と、減ったインスタンスが受け持っていた key だけになる。
func pick(instances []*registry.Instance, key string) *registry.Instance {
	var best *registry.Instance
	var bestScore uint64
	for _, instance := range instances {
		hash := fnv.New64a()
		_, _ = hash.Write([]byte(key + "\n" + instance.ID.String()))
		if score := hash.Sum64(); best == nil || score > bestScore || (score == bestScore && instance.ID.String() < best.ID.String()) {
			best, bestScore = instance, score
		}
	}

	return best
}

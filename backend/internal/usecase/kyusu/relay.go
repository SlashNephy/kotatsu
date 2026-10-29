package kyusu

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

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/infra/registry"
)

const (
	// Role は、サーバー側でのトランスコードを受け持つロールの名前。インスタンスの登録簿で、kyusu を引くのに使う。
	Role = "kyusu"
	// relayTimeout は、kyusu への中継にかける時間の上限。kyusu は、ライブ視聴の最初のセグメントを最大 20 秒、
	// 録画のセグメントを最大 30 秒待つので、それより長くして、kyusu が待ちきれなかった理由を返せるようにする。
	relayTimeout = time.Minute
)

// Instances は、ロールの動いているインスタンスを引く。*registry.Registry が満たす。
type Instances interface {
	ListAlive(ctx context.Context, role string) ([]*registry.Instance, error)
}

var _ Instances = new(registry.Registry)

// Relay は、tenban で、HLS を担当の kyusu に中継する (設計書 7.3)。
// ライブ視聴は GET /streams/services/{id}/hls/{preset}/{audio}/{file}、録画は GET /streams/recordings/{id}/hls/{preset}/{audio}/{file}、
// タイムシフトのバッファは GET /streams/timeshift/{id}/hls/{preset}/{audio}/{file} で受け付ける。
//
// 同じものを同じ画質と音声で見る視聴者が 1 つのエンコーダーを共有できるよう、(サービスか録画, 画質, 音声) から決まった kyusu に中継する。
// kyusu が増えたり減ったりして担当が変わると、見ている視聴者は新しい担当のエンコーダーに移る。
type Relay struct {
	instances Instances
	token     *Token
	client    *http.Client
	logger    *slog.Logger
	// prefix は、kyusu のパスの前置き。canonical は、パスの {id} を確かめ、同じものが同じ文字列になるように直す。
	prefix    string
	canonical func(id string) (string, bool)
}

var _ http.Handler = new(Relay)

// NewRelay は、ライブ視聴の HLS を中継する Relay を返す。token は、kyusu へのリクエストに付ける。
func NewRelay(instances Instances, token *Token, logger *slog.Logger) *Relay {
	return &Relay{
		instances: instances, token: token, client: &http.Client{Timeout: relayTimeout}, logger: logger,
		prefix: LivePathPrefix,
		canonical: func(id string) (string, bool) {
			serviceID, err := strconv.ParseInt(id, 10, 64)

			return strconv.FormatInt(serviceID, 10), err == nil
		},
	}
}

// NewRecordingRelay は、録画の仮想 HLS を中継する Relay を返す。token は、kyusu へのリクエストに付ける。
func NewRecordingRelay(instances Instances, token *Token, logger *slog.Logger) *Relay {
	return &Relay{
		instances: instances, token: token, client: &http.Client{Timeout: relayTimeout}, logger: logger,
		prefix: RecordingPathPrefix,
		canonical: func(id string) (string, bool) {
			recordingID, err := uuid.Parse(id)

			return recordingID.String(), err == nil
		},
	}
}

// NewTimeshiftRelay は、タイムシフトのバッファの仮想 HLS を中継する Relay を返す。token は、kyusu へのリクエストに付ける。
func NewTimeshiftRelay(instances Instances, token *Token, logger *slog.Logger) *Relay {
	return &Relay{
		instances: instances, token: token, client: &http.Client{Timeout: relayTimeout}, logger: logger,
		prefix: TimeshiftPathPrefix,
		canonical: func(id string) (string, bool) {
			serviceID, from, ok := ParseTimeshiftID(id)

			return FormatTimeshiftID(serviceID, from), ok
		},
	}
}

func (h *Relay) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 書き方の違う同じ ID も、同じ kyusu に中継する。
	id, ok := h.canonical(r.PathValue("id"))
	if !ok {
		http.Error(w, "invalid ID", http.StatusBadRequest)

		return
	}
	preset, audio, file := r.PathValue("preset"), r.PathValue("audio"), r.PathValue("file")
	instances, err := h.instances.ListAlive(r.Context(), Role)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "failed to list kyusu instances", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		return
	}
	if len(instances) == 0 {
		http.Error(w, "server-side transcoding is not available; no kyusu instance is running", http.StatusServiceUnavailable)

		return
	}
	instance := pick(instances, h.prefix+id+"/"+preset+"/"+audio)
	target := &url.URL{
		Scheme: "http", Host: instance.Address,
		// Path は、エスケープする前のパス。String でエスケープする。
		Path: h.prefix + id + "/" + preset + "/" + audio + "/" + file,
	}
	if err := h.relay(w, r, target); err != nil && r.Context().Err() == nil {
		h.logger.WarnContext(r.Context(), "failed to relay to kyusu", slog.String("instance", instance.Address), slog.Any("error", err))
	}
}

// relay は、target に GET を送り、状態と本文をそのまま返す。
func (h *Relay) relay(w http.ResponseWriter, r *http.Request, target *url.URL) error {
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

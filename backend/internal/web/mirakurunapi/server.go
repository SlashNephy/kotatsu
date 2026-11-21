// Package mirakurunapi は、Kotatsu のチューナーサーバー (tunerserver.Client) を、Mirakurun 互換の API の一部として公開する (#1535)。
//
// nabe が、EDCB の NetworkTV の TS をほかのロールに配るのに使う。Kotatsu の Mirakurun のクライアント (infra/mirakurun) が
// 使う API (バージョン、チャンネル、サービス、番組、チューナー、イベント、ロゴ、サービスとチャンネルのストリーム) だけを持つ。
// mirakc の /events と /api/onair は持たないので、クライアントは Mirakurun として扱う。
package mirakurunapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/lib/innertoken"
	"github.com/SlashNephy/kotatsu/internal/lib/logattr"
	"github.com/SlashNephy/kotatsu/internal/usecase/nabe"
	"github.com/SlashNephy/kotatsu/internal/web/httpserver"
)

// streamBufferSize は、ストリームを 1 回に読んで書く大きさ。
const streamBufferSize = 188 * 348

// Server は、tunerserver.Client を Mirakurun 互換の API として公開する。認可はしないので、使う側がミドルウェアで守る。
type Server struct {
	source  tunerserver.Client
	version string
	logger  *slog.Logger
	router  http.Handler
}

var _ http.Handler = new(Server)

// NewServer は、source を公開する Server を返す。version は /api/version で名乗る版。
// ルートは echo で受ける (#2016)。http.ServeMux で受けていたときと同じく、崩れたパスはリダイレクトし、当たらないパスには素の 404 を返す。
func NewServer(source tunerserver.Client, version string, logger *slog.Logger) *Server {
	s := &Server{source: source, version: version, logger: logger, router: nil}
	router := httpserver.NewRouter(logger, http.NotFoundHandler())
	router.Pre(httpserver.CanonicalPath())
	for path, handler := range map[string]http.HandlerFunc{
		"/api/version":                        s.serveVersion,
		"/api/channels":                       s.serveChannels,
		"/api/services":                       s.serveServices,
		"/api/services/:id/logo":              s.serveLogo,
		"/api/services/:id/stream":            s.serveServiceStream,
		"/api/channels/:type/:channel/stream": s.serveChannelStream,
		"/api/programs":                       s.servePrograms,
		"/api/tuners":                         s.serveTuners,
		"/api/events/stream":                  s.serveEvents,
	} {
		httpserver.Handle(router, path, map[string]http.Handler{http.MethodGet: handler})
	}
	s.router = router

	return s
}

// NabeHandler は、source を Mirakurun 互換の API として、nabe.PathPrefix の下に公開するハンドラーを返す (#1535)。
// token を付けたリクエストだけ受け付ける。トークンは、ルートを探す前に確かめる。
func NabeHandler(source tunerserver.Client, token *innertoken.Token, version string, logger *slog.Logger) http.Handler {
	return token.Middleware(http.StripPrefix(strings.TrimSuffix(nabe.PathPrefix, "/"), NewServer(source, version, logger)))
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) serveVersion(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, func(v *mirakurun.Version) ([]byte, error) { return json.Marshal(v) }, &mirakurun.Version{Current: s.version, Latest: s.version, Server: ""})
}

func (s *Server) serveChannels(w http.ResponseWriter, r *http.Request) {
	channels, err := s.source.Channels(r.Context())
	if err != nil {
		s.writeError(w, r, err)

		return
	}
	writeJSON(w, mirakurun.MarshalChannels, channels)
}

func (s *Server) serveServices(w http.ResponseWriter, r *http.Request) {
	services, err := s.source.Services(r.Context())
	if err != nil {
		s.writeError(w, r, err)

		return
	}
	writeJSON(w, mirakurun.MarshalServices, services)
}

func (s *Server) servePrograms(w http.ResponseWriter, r *http.Request) {
	var query *tunerserver.ProgramsQuery
	values := r.URL.Query()
	if values.Has("networkId") || values.Has("serviceId") || values.Has("eventId") {
		query = &tunerserver.ProgramsQuery{NetworkID: 0, ServiceID: 0, EventID: 0}
		for key, field := range map[string]*int{"networkId": &query.NetworkID, "serviceId": &query.ServiceID, "eventId": &query.EventID} {
			if !values.Has(key) {
				continue
			}
			v, err := strconv.Atoi(values.Get(key))
			if err != nil {
				writeReason(w, http.StatusBadRequest, "invalid "+key)

				return
			}
			*field = v
		}
	}
	programs, err := s.source.Programs(r.Context(), query)
	if err != nil {
		s.writeError(w, r, err)

		return
	}
	writeJSON(w, mirakurun.MarshalPrograms, programs)
}

func (s *Server) serveTuners(w http.ResponseWriter, r *http.Request) {
	tuners, err := s.source.Tuners(r.Context())
	if err != nil {
		s.writeError(w, r, err)

		return
	}
	writeJSON(w, mirakurun.MarshalTuners, tuners)
}

func (s *Server) serveLogo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathServiceID(w, r)
	if !ok {
		return
	}
	logo, err := s.source.ServiceLogo(r.Context(), id)
	if err != nil {
		s.writeError(w, r, err)

		return
	}
	w.Header().Set("Content-Type", "image/png")
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(logo))
}

func (s *Server) serveServiceStream(w http.ResponseWriter, r *http.Request) {
	id, ok := pathServiceID(w, r)
	if !ok {
		return
	}
	stream, err := s.source.OpenServiceStream(r.Context(), id, streamOptions(r))
	s.serveStream(w, r, stream, err)
}

func (s *Server) serveChannelStream(w http.ResponseWriter, r *http.Request) {
	stream, err := s.source.OpenChannelStream(r.Context(), r.PathValue("type"), r.PathValue("channel"), streamOptions(r))
	s.serveStream(w, r, stream, err)
}

// serveStream は、開いたストリームを書く。チューナーを取られて終わったら、その理由をトレーラーで返す。
// ほかの理由で途中で切れたら、応答を打ち切り、受け手に、読み終えたのではなく切れたと分からせる。
func (s *Server) serveStream(w http.ResponseWriter, r *http.Request, stream io.ReadCloser, err error) {
	if err != nil {
		s.writeError(w, r, err)

		return
	}
	defer func() { _ = stream.Close() }()
	// 受け手が去ったら、TS が届くのを待っている Read を抜けさせる。分け合うストリームでは、閉じれば受け手から外れる。
	stop := context.AfterFunc(r.Context(), func() { _ = stream.Close() })
	defer stop()

	w.Header().Set("Content-Type", "video/MP2T")
	w.Header().Set("Trailer", mirakurun.StreamErrorTrailer)
	w.WriteHeader(http.StatusOK)
	controller := http.NewResponseController(w)
	_ = controller.Flush()
	buf := make([]byte, streamBufferSize)
	for {
		n, readErr := stream.Read(buf)
		if n > 0 {
			if _, err := w.Write(buf[:n]); err != nil {
				return
			}
			_ = controller.Flush()
		}
		switch {
		case readErr == nil:
			continue
		case errors.Is(readErr, io.EOF), r.Context().Err() != nil:
			return
		case errors.Is(readErr, tunerserver.ErrTunerPreempted):
			w.Header().Set(mirakurun.StreamErrorTrailer, mirakurun.StreamErrorPreempted)

			return
		default:
			s.logger.WarnContext(r.Context(), "the stream was cut off", logattr.String("path", r.URL.Path), slog.Any("error", readErr))
			panic(http.ErrAbortHandler)
		}
	}
}

// serveEvents は、イベントを、Mirakurun の /api/events/stream と同じ形 (閉じない JSON の配列) で書き続ける。
func (s *Server) serveEvents(w http.ResponseWriter, r *http.Request) {
	events, err := s.source.OpenEvents(r.Context())
	if err != nil {
		s.writeError(w, r, err)

		return
	}
	defer func() { _ = events.Close() }()
	// 受け手が去ったら、待っている Next を抜けさせる。
	stop := context.AfterFunc(r.Context(), func() { _ = events.Close() })
	defer stop()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	controller := http.NewResponseController(w)
	if _, err := io.WriteString(w, "[\n"); err != nil {
		return
	}
	_ = controller.Flush()
	separator := ""
	for {
		event, err := events.Next()
		if err != nil {
			return
		}
		data, err := mirakurun.MarshalEvent(event, time.Now())
		if err != nil {
			return
		}
		if _, err := io.WriteString(w, separator+string(data)+"\n"); err != nil {
			return
		}
		_ = controller.Flush()
		separator = ","
	}
}

// writeError は、エラーを Mirakurun と同じ形の応答にする。サービスかチャンネルがなければ 404 を、ほかは 503 を返す。
// 503 は、クライアントが、チューナーが足りない (ストリーム) か、ロゴをまだ受信していない (ロゴ) と読み替える。
// 一覧の API でも 503 にする。EDCB が EPG を読み込んでいる最中のように、少し後に取り直せばよいことが多いためである。
// 空の一覧で返すと、受け手が DB のサービスや番組を消してしまう。
func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	if r.Context().Err() != nil {
		return
	}
	status := http.StatusServiceUnavailable
	if errors.Is(err, tunerserver.ErrNotFound) {
		status = http.StatusNotFound
	}
	s.logger.DebugContext(r.Context(), "the tuner server request failed", logattr.String("path", r.URL.Path), slog.Any("error", err))
	writeReason(w, status, err.Error())
}

// writeReason は、Mirakurun のエラーと同じ形 (code と reason) の JSON を返す。
func writeReason(w http.ResponseWriter, status int, reason string) {
	body, err := json.Marshal(struct {
		Code   int    `json:"code"`
		Reason string `json:"reason"`
	}{Code: status, Reason: reason})
	if err != nil {
		http.Error(w, http.StatusText(status), status)

		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// writeJSON は、marshal で v を JSON にして返す。
func writeJSON[T any](w http.ResponseWriter, marshal func(T) ([]byte, error), v T) {
	body, err := marshal(v)
	if err != nil {
		writeReason(w, http.StatusInternalServerError, fmt.Sprintf("failed to encode the response: %v", err))

		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// pathServiceID は、パスのサービス ID を読む。読めなければ 400 を返して false を返す。
func pathServiceID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeReason(w, http.StatusBadRequest, "invalid service ID")

		return 0, false
	}

	return id, true
}

// streamOptions は、ストリームの要求の decode と優先度を読む。優先度が読めなければ 0 にする。
func streamOptions(r *http.Request) *tunerserver.StreamOptions {
	priority, _ := strconv.Atoi(r.Header.Get(mirakurun.PriorityHeader))

	return &tunerserver.StreamOptions{Decode: r.URL.Query().Get("decode") == "1", Priority: priority}
}

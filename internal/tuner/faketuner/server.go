package faketuner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/SlashNephy/kotatsu/internal/clock"
)

const (
	// priorityHeader は、Mirakurun がチューナーの優先度を受け取るヘッダー。
	priorityHeader = "X-Mirakurun-Priority"
	// streamChunkSize と streamInterval で、TS を流す速さを決める (おおよそ 6.5 MB/s)。
	streamChunkSize = 188 * 348
	streamInterval  = 10 * time.Millisecond
	versionString   = "3.9.0-faketuner"
)

// TunerConfig はチューナーの設定。
type TunerConfig struct {
	Name string
	// Types は、このチューナーが受信できるチャンネル種別。
	Types []string
}

// Options は Server の設定。
type Options struct {
	Channels []*Channel
	Programs []*Program
	Tuners   []*TunerConfig
	// TS は、サービスストリームで繰り返し流す TS。空なら null パケットを流す。
	TS    []byte
	Clock clock.Clock
}

// Server は偽のチューナーサーバー。http.Handler として使う。
type Server struct {
	mux   *http.ServeMux
	ts    []byte
	clock clock.Clock

	mu          sync.Mutex
	channels    []*Channel
	programs    map[int64]*Program
	tuners      []*tunerState
	subscribers map[chan *Event]struct{}
}

var _ http.Handler = new(Server)

type tunerState struct {
	config *TunerConfig
	// channel は、受信しているチャンネル (種別とチャンネルの組)。使っていなければ nil。
	channel *ChannelRef
	users   []*streamUser
}

type streamUser struct {
	user *TunerUser
	// kick は、チューナーを奪われた利用者のストリームを止める。
	kick context.CancelFunc
}

// New は Server を作る。
func New(opts *Options) *Server {
	clk := opts.Clock
	if clk == nil {
		clk = new(clock.Real)
	}

	s := &Server{
		mux:         http.NewServeMux(),
		ts:          opts.TS,
		clock:       clk,
		mu:          sync.Mutex{},
		channels:    opts.Channels,
		programs:    map[int64]*Program{},
		tuners:      make([]*tunerState, 0, len(opts.Tuners)),
		subscribers: map[chan *Event]struct{}{},
	}
	// 呼び出し側が持つ値を書き換えないよう、複製して持つ。
	for _, program := range opts.Programs {
		copied := *program
		s.programs[program.ID] = &copied
	}
	for _, config := range opts.Tuners {
		s.tuners = append(s.tuners, &tunerState{config: config, channel: nil, users: []*streamUser{}})
	}
	if len(s.ts) == 0 {
		s.ts = nullPackets(streamChunkSize / 188)
	}

	s.mux.HandleFunc("GET /api/version", s.handleVersion)
	s.mux.HandleFunc("GET /api/channels", s.handleChannels)
	s.mux.HandleFunc("GET /api/services", s.handleServices)
	s.mux.HandleFunc("GET /api/programs", s.handlePrograms)
	s.mux.HandleFunc("GET /api/tuners", s.handleTuners)
	s.mux.HandleFunc("GET /api/events/stream", s.handleEventsStream)
	s.mux.HandleFunc("GET /api/services/{id}/stream", s.handleServiceStream)

	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 時計のずれを再現できるよう、Date ヘッダーは偽チューナーサーバーの時計で返す。
	w.Header().Set("Date", s.clock.Now().UTC().Format(http.TimeFormat))
	s.mux.ServeHTTP(w, r)
}

// SetTuners は、チューナーの構成を置き換え、各チューナーの tuner の update イベントを送る。
// 使っている利用者がいるチューナーを置き換えると、その利用者は外れたものとして扱う。
func (s *Server) SetTuners(configs []*TunerConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, tuner := range s.tuners {
		for _, user := range tuner.users {
			user.kick()
		}
		// 外した利用者のストリームが閉じるときに、置き換えた後のチューナーを探さないよう、利用者を空にしておく。
		tuner.users = []*streamUser{}
	}
	s.tuners = make([]*tunerState, 0, len(configs))
	for _, config := range configs {
		s.tuners = append(s.tuners, &tunerState{config: config, channel: nil, users: []*streamUser{}})
	}
	for _, tuner := range s.tunersLocked() {
		s.publishLocked("tuner", "update", tuner)
	}
}

// AddProgram は番組を足し、program の create イベントを送る。
func (s *Server) AddProgram(program *Program) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored := *program
	s.programs[program.ID] = &stored
	// 送ったイベントの中身が、後の変更で書き換わらないよう、別の値を送る。
	event := stored
	s.publishLocked("program", "create", &event)
}

// RescheduleProgram は番組の開始時刻と長さを変え (時間の変更)、program の update イベントを送る。
func (s *Server) RescheduleProgram(id int64, startAt time.Time, duration time.Duration) error {
	return s.updateProgram(id, func(p *Program) {
		p.StartAt = startAt.UnixMilli()
		p.Duration = duration.Milliseconds()
	})
}

// ExtendProgram は番組の長さを extra だけ延ばし (延長)、program の update イベントを送る。
func (s *Server) ExtendProgram(id int64, extra time.Duration) error {
	return s.updateProgram(id, func(p *Program) {
		p.Duration += extra.Milliseconds()
	})
}

// ChangeProgramAudios は番組の音声の構成を変え、program の update イベントを送る。
func (s *Server) ChangeProgramAudios(id int64, audios []*Audio) error {
	return s.updateProgram(id, func(p *Program) {
		p.Audios = audios
	})
}

// RemoveProgram は番組を消し、program の remove イベントを送る。
func (s *Server) RemoveProgram(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.programs[id]; !ok {
		return fmt.Errorf("program %d not found", id)
	}
	delete(s.programs, id)
	// Mirakurun の remove イベントは、消した番組の ID だけを送る。
	s.publishLocked("program", "remove", map[string]int64{"id": id})

	return nil
}

// ForgetProgram は、イベントを送らずに番組を消す。
// Mirakurun は、放送予定から消えた番組を GC で消してもイベントを送らないことがある (設計書 3.3)。
func (s *Server) ForgetProgram(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.programs[id]; !ok {
		return fmt.Errorf("program %d not found", id)
	}
	delete(s.programs, id)

	return nil
}

// EventSubscribers は、イベントストリームを購読している接続の数を返す。テストで購読を待つのに使う。
func (s *Server) EventSubscribers() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.subscribers)
}

// Tuners は、今のチューナーの状態を返す。
func (s *Server) Tuners() []*Tuner {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.tunersLocked()
}

func (s *Server) updateProgram(id int64, update func(p *Program)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	program, ok := s.programs[id]
	if !ok {
		return fmt.Errorf("program %d not found", id)
	}
	update(program)
	copied := *program
	s.publishLocked("program", "update", &copied)

	return nil
}

func (s *Server) publishLocked(resource, eventType string, data any) {
	event := &Event{Resource: resource, Type: eventType, Data: data, Time: s.clock.Now().UnixMilli()}
	for subscriber := range s.subscribers {
		select {
		case subscriber <- event:
		default:
			// 読むのが遅い購読者のために他を止めない。Mirakurun のイベントも取りこぼしうるので、テストで困ることはない。
		}
	}
}

func (s *Server) tunersLocked() []*Tuner {
	tuners := make([]*Tuner, 0, len(s.tuners))
	for i, state := range s.tuners {
		users := make([]*TunerUser, 0, len(state.users))
		for _, u := range state.users {
			copied := *u.user
			users = append(users, &copied)
		}
		using := state.channel != nil
		tuners = append(tuners, &Tuner{
			Index:       i,
			Name:        state.config.Name,
			Types:       slices.Clone(state.config.Types),
			Command:     "",
			PID:         0,
			Users:       users,
			IsAvailable: true,
			IsRemote:    false,
			IsFree:      !using,
			IsUsing:     using,
			IsFault:     false,
		})
	}

	return tuners
}

func (s *Server) handleVersion(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, &Version{Current: versionString, Latest: versionString})
}

func (s *Server) handleChannels(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	writeJSON(w, http.StatusOK, s.channels)
}

func (s *Server) handleServices(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	services := []*Service{}
	for _, channel := range s.channels {
		for _, service := range channel.Services {
			copied := *service
			copied.Channel = &ChannelRef{Type: channel.Type, Channel: channel.Channel}
			services = append(services, &copied)
		}
	}
	writeJSON(w, http.StatusOK, services)
}

func (s *Server) handlePrograms(w http.ResponseWriter, r *http.Request) {
	// Mirakurun と同じく、networkId、serviceId、eventId で絞り込める。
	filters := map[string]func(p *Program) int{
		"networkId": func(p *Program) int { return p.NetworkID },
		"serviceId": func(p *Program) int { return p.ServiceID },
		"eventId":   func(p *Program) int { return p.EventID },
	}
	wants := map[string]int{}
	for key := range filters {
		raw := r.URL.Query().Get(key)
		if raw == "" {
			continue
		}
		v, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, key+" must be an integer")

			return
		}
		wants[key] = v
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	programs := []*Program{}
	for _, id := range slices.Sorted(maps.Keys(s.programs)) {
		program := s.programs[id]
		matched := true
		for key, want := range wants {
			if filters[key](program) != want {
				matched = false
			}
		}
		if matched {
			copied := *program
			programs = append(programs, &copied)
		}
	}
	writeJSON(w, http.StatusOK, programs)
}

func (s *Server) handleTuners(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.Tuners())
}

// handleEventsStream は Mirakurun のイベントストリームを再現する。
// SSE でも NDJSON でもなく、最初に "[" を送り、その後はイベントの JSON と "," を改行で区切って送り続け、配列を閉じない。
func (s *Server) handleEventsStream(w http.ResponseWriter, r *http.Request) {
	resource := r.URL.Query().Get("resource")
	eventType := r.URL.Query().Get("type")

	events := make(chan *Event, 64)
	s.mu.Lock()
	s.subscribers[events] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.subscribers, events)
		s.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	write := func(b []byte) bool {
		if _, err := w.Write(b); err != nil {
			return false
		}
		if flusher != nil {
			flusher.Flush()
		}

		return true
	}
	if !write([]byte("[\n")) {
		return
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-events:
			if (resource != "" && resource != event.Resource) || (eventType != "" && eventType != event.Type) {
				continue
			}
			b, err := json.Marshal(event)
			if err != nil {
				return
			}
			if !write(append(b, "\n,\n"...)) {
				return
			}
		}
	}
}

// handleServiceStream は、サービスのストリームを流す。チューナーを確保できなければ 503 を返す。
func (s *Server) handleServiceStream(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid service id")

		return
	}
	priority := 0
	if raw := r.Header.Get(priorityHeader); raw != "" {
		priority, err = strconv.Atoi(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, priorityHeader+" must be an integer")

			return
		}
	}

	ctx, kick := context.WithCancel(r.Context())
	defer kick()
	user := &streamUser{
		user: &TunerUser{ID: r.RemoteAddr, Priority: priority, Agent: r.UserAgent()},
		kick: kick,
	}
	release, err := s.acquire(id, user)
	if errors.Is(err, errServiceNotFound) {
		writeError(w, http.StatusNotFound, err.Error())

		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())

		return
	}
	defer release()

	w.Header().Set("Content-Type", "video/MP2T")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	ticker := time.NewTicker(streamInterval)
	defer ticker.Stop()
	for offset := 0; ; {
		select {
		case <-ctx.Done():
			// 接続が切れたか、チューナーを奪われた。
			return
		case <-ticker.C:
		}

		end := min(offset+streamChunkSize, len(s.ts))
		if _, err := w.Write(s.ts[offset:end]); err != nil {
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
		offset = end % len(s.ts)
	}
}

var (
	errServiceNotFound  = errors.New("service not found")
	errNoAvailableTuner = errors.New("no available tuners")
)

// acquire は、サービスを受信できるチューナーを Mirakurun と同じ順で選び、user を載せる。
//
//  1. 同じチャンネルを受信しているチューナーに相乗りする
//  2. 空いているチューナーを使う
//  3. 優先度が 0 以上なら、利用者の最高の優先度が自分より低いチューナーを、低い順に奪う
//
// 返す関数で、チューナーから user を外す。
func (s *Server) acquire(serviceID int64, user *streamUser) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	channel := s.channelOfLocked(serviceID)
	if channel == nil {
		return nil, fmt.Errorf("%w: %d", errServiceNotFound, serviceID)
	}

	candidates := []*tunerState{}
	for _, state := range s.tuners {
		if slices.Contains(state.config.Types, channel.Type) {
			candidates = append(candidates, state)
		}
	}

	tuner := pickTuner(candidates, channel, user.user.Priority)
	if tuner == nil {
		return nil, errNoAvailableTuner
	}
	if tuner.channel == nil || *tuner.channel != *channel {
		// 奪う場合は、今の利用者のストリームを止める。
		for _, u := range tuner.users {
			u.kick()
		}
		tuner.users = []*streamUser{}
		tuner.channel = channel
	}
	tuner.users = append(tuner.users, user)
	s.publishLocked("tuner", "update", s.tunersLocked()[slices.Index(s.tuners, tuner)])

	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		index := slices.Index(tuner.users, user)
		if index < 0 {
			// 奪われたときは、すでに外れている。
			return
		}
		tuner.users = slices.Delete(tuner.users, index, index+1)
		if len(tuner.users) == 0 {
			tuner.channel = nil
		}
		s.publishLocked("tuner", "update", s.tunersLocked()[slices.Index(s.tuners, tuner)])
	}, nil
}

func pickTuner(candidates []*tunerState, channel *ChannelRef, priority int) *tunerState {
	for _, state := range candidates {
		if state.channel != nil && *state.channel == *channel {
			return state
		}
	}
	for _, state := range candidates {
		if state.channel == nil {
			return state
		}
	}
	if priority < 0 {
		return nil
	}

	var lowest *tunerState
	for _, state := range candidates {
		if p := state.priority(); p < priority && (lowest == nil || p < lowest.priority()) {
			lowest = state
		}
	}

	return lowest
}

// priority は、利用者の中で最も高い優先度を返す。
// 利用者がいなければ -2 を返す。Mirakurun の TunerDevice#getPriority と同じ値にしている。
func (t *tunerState) priority() int {
	highest := -2
	for _, u := range t.users {
		highest = max(highest, u.user.Priority)
	}

	return highest
}

func (s *Server) channelOfLocked(serviceID int64) *ChannelRef {
	for _, channel := range s.channels {
		for _, service := range channel.Services {
			if service.ID == serviceID {
				return &ChannelRef{Type: channel.Type, Channel: channel.Channel}
			}
		}
	}

	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "failed to encode the response", http.StatusInternalServerError)

		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(append(b, '\n'))
}

func writeError(w http.ResponseWriter, status int, reason string) {
	writeJSON(w, status, map[string]any{"code": status, "reason": reason})
}

// nullPackets は、n 個の null パケット (PID 0x1FFF) からなる TS を作る。
func nullPackets(n int) []byte {
	ts := make([]byte, 0, n*188)
	for range n {
		packet := make([]byte, 188)
		packet[0] = 0x47
		packet[1] = 0x1F
		packet[2] = 0xFF
		packet[3] = 0x10
		for i := 4; i < len(packet); i++ {
			packet[i] = 0xFF
		}
		ts = append(ts, packet...)
	}

	return ts
}

package epgstationapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/epg"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
)

// broadcastTypes は、EPGStation の放送波の種別。config の broadcast と、schedules の絞り込みに使う。
func broadcastTypes() []string {
	return []string{"GR", "BS", "CS", "SKY"}
}

// secondsLimit は、時刻をミリ秒でなく秒で渡されたとみなす上限。ミリ秒なら 1973 年より前になる値を、秒とみなす。
const secondsLimit = 100_000_000_000

// eventIDDigits は、番組 ID で、イベント ID に割く桁。Mirakurun の番組 ID (サービス ID × 100000 + イベント ID) と同じにする。
const eventIDDigits = 100000

var errInvalidTime = errors.New("startAt and endAt must be integers, startAt must be before endAt, and the range must be at most 8 days")

type configResponse struct {
	// Broadcast は、放送波の種別ごとに、サービスがあるかどうか。
	Broadcast map[string]bool `json:"broadcast"`
}

type channel struct {
	ID                 int64  `json:"id"`
	ServiceID          int64  `json:"serviceId"`
	NetworkID          int64  `json:"networkId"`
	Name               string `json:"name"`
	HalfWidthName      string `json:"halfWidthName"`
	HasLogoData        bool   `json:"hasLogoData"`
	ChannelType        string `json:"channelType"`
	Channel            string `json:"channel"`
	Type               int64  `json:"type"`
	RemoteControlKeyID *int64 `json:"remoteControlKeyId,omitempty"`
}

type genre struct {
	Lv1 int `json:"lv1"`
	Lv2 int `json:"lv2"`
	Un1 int `json:"un1"`
	Un2 int `json:"un2"`
}

type program struct {
	ID          int64   `json:"id"`
	EventID     int64   `json:"eventId"`
	ServiceID   int64   `json:"serviceId"`
	NetworkID   int64   `json:"networkId"`
	ChannelID   int64   `json:"channelId"`
	StartAt     int64   `json:"startAt"`
	EndAt       int64   `json:"endAt"`
	Duration    int64   `json:"duration"`
	IsFree      bool    `json:"isFree"`
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	Extended    string  `json:"extended,omitempty"`
	Genres      []genre `json:"genres"`
	Genre1      *int    `json:"genre1,omitempty"`
	SubGenre1   *int    `json:"subGenre1,omitempty"`
}

type scheduleChannel struct {
	Channel  *channel   `json:"channel"`
	Programs []*program `json:"programs"`
}

// services は、番組表に出すサービスを、番組表の並び順で返す。非表示のサービスと、映像も音声も流さないサービスは外す。
func (h *Handler) services(r *http.Request) ([]sqlc.ListGuideServicesRow, error) {
	rows, err := sqlc.New(h.pool).ListGuideServices(r.Context(), sqlc.ListGuideServicesParams{
		IncludeHidden: false, MediaServiceTypes: epg.MediaServiceTypes(), Ids: []int64{},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list the services: %w", err)
	}

	return rows, nil
}

func channelOf(row *sqlc.ListGuideServicesRow) *channel {
	return &channel{
		ID: row.ID, ServiceID: row.Sid, NetworkID: row.NetworkID, Name: row.Name, HalfWidthName: halfWidth(row.Name),
		HasLogoData: row.LogoID != nil, ChannelType: row.ChannelType, Channel: row.Channel, Type: row.ServiceType,
		RemoteControlKeyID: row.RemoteControlKeyID,
	}
}

// Config は GET /api/config に答える。放送波の種別ごとに、サービスがあるかどうかを返す。
func (h *Handler) Config(w http.ResponseWriter, r *http.Request) {
	rows, err := h.services(r)
	if err != nil {
		h.fail(w, r, err)

		return
	}
	broadcast := make(map[string]bool, len(broadcastTypes()))
	for _, t := range broadcastTypes() {
		broadcast[t] = false
	}
	for _, row := range rows {
		if _, ok := broadcast[row.ChannelType]; ok {
			broadcast[row.ChannelType] = true
		}
	}
	h.writeJSON(w, r, &configResponse{Broadcast: broadcast})
}

// Channels は GET /api/channels に答える。
func (h *Handler) Channels(w http.ResponseWriter, r *http.Request) {
	rows, err := h.services(r)
	if err != nil {
		h.fail(w, r, err)

		return
	}
	channels := make([]*channel, 0, len(rows))
	for i := range rows {
		channels = append(channels, channelOf(&rows[i]))
	}
	h.writeJSON(w, r, channels)
}

// Schedules は GET /api/schedules に答える。startAt から endAt に重なる番組を、サービスごとに返す。
// GR、BS、CS、SKY に false を渡した種別のサービスは外す。isHalfWidth が true なら、番組名などを半角にする。
func (h *Handler) Schedules(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	from, errFrom := parseTime(query.Get("startAt"))
	to, errTo := parseTime(query.Get("endAt"))
	// 番組表の画面と同じく、一度に引ける時間帯の長さに上限を設ける。
	if errFrom != nil || errTo != nil || !from.Before(to) || to.Sub(from) > epg.MaxGuideRange {
		http.Error(w, errInvalidTime.Error(), http.StatusBadRequest)

		return
	}
	toHalfWidth := query.Get("isHalfWidth") == "true"
	rows, err := h.services(r)
	if err != nil {
		h.fail(w, r, err)

		return
	}
	schedules := make([]*scheduleChannel, 0, len(rows))
	byService := make(map[int64]*scheduleChannel, len(rows))
	ids := make([]int64, 0, len(rows))
	for i := range rows {
		// 種別を渡さなければ、その種別も返す。
		if query.Get(rows[i].ChannelType) == "false" {
			continue
		}
		schedule := &scheduleChannel{Channel: channelOf(&rows[i]), Programs: []*program{}}
		schedules = append(schedules, schedule)
		byService[rows[i].ID] = schedule
		ids = append(ids, rows[i].ID)
	}
	programs, err := sqlc.New(h.pool).ListProgramsInRange(r.Context(), sqlc.ListProgramsInRangeParams{ServiceIds: ids, FromAt: from, ToAt: to})
	if err != nil {
		h.fail(w, r, fmt.Errorf("failed to list the programs: %w", err))

		return
	}
	for i := range programs {
		schedule := byService[programs[i].ServiceID]
		p, err := programOf(&programs[i], schedule.Channel, toHalfWidth)
		if err != nil {
			h.fail(w, r, err)

			return
		}
		schedule.Programs = append(schedule.Programs, p)
	}
	h.writeJSON(w, r, schedules)
}

func programOf(row *sqlc.ListProgramsInRangeRow, ch *channel, toHalfWidth bool) (*program, error) {
	var genres []*mirakurun.Genre
	if err := json.Unmarshal(row.Genres, &genres); err != nil {
		return nil, fmt.Errorf("failed to decode the genres: %w", err)
	}
	var extended map[string]string
	if len(row.Extended) > 0 {
		if err := json.Unmarshal(row.Extended, &extended); err != nil {
			return nil, fmt.Errorf("failed to decode the extended description: %w", err)
		}
	}
	name, description := row.Name, row.Description
	if toHalfWidth {
		name, description = halfWidth(name), halfWidth(description)
	}
	p := &program{
		ID: row.ServiceID*eventIDDigits + row.EventID, EventID: row.EventID, ServiceID: ch.ServiceID, NetworkID: ch.NetworkID,
		ChannelID: row.ServiceID, StartAt: row.StartAt.UnixMilli(), EndAt: row.EndAt.UnixMilli(),
		Duration: row.EndAt.Sub(row.StartAt).Milliseconds(), IsFree: row.IsFree, Name: name, Description: description,
		Extended: extendedText(extended, toHalfWidth), Genres: make([]genre, 0, len(genres)), Genre1: nil, SubGenre1: nil,
	}
	for _, g := range genres {
		p.Genres = append(p.Genres, genre{Lv1: g.Lv1, Lv2: g.Lv2, Un1: g.Un1, Un2: g.Un2})
	}
	if len(genres) > 0 {
		p.Genre1, p.SubGenre1 = &genres[0].Lv1, &genres[0].Lv2
	}

	return p, nil
}

// extendedText は、番組の詳しい説明を、EPGStation と同じく、見出しと本文を 1 行ずつ交互に並べた文字列にする。
// 本文の改行は、行の交互が崩れないよう、空白に置き換える。DB の JSONB は見出しの順を残さないので、見出しの順に並べる。
func extendedText(extended map[string]string, toHalfWidth bool) string {
	lines := make([]string, 0, len(extended)*2)
	for _, heading := range slices.Sorted(maps.Keys(extended)) {
		lines = append(lines, oneLine(heading), oneLine(extended[heading]))
	}
	text := strings.Join(lines, "\n")
	if toHalfWidth {
		return halfWidth(text)
	}

	return text
}

// halfWidth は、全角の英数字と記号、全角の空白を半角にする。EPGStation の isHalfWidth と同じく、カタカナや
// ARIB の囲み文字 (🈑 など) はそのまま残す。NFKC は囲み文字を中の文字に分解し、番組名に「字」などが混ざるので使わない。
func halfWidth(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= '！' && r <= '～':
			return r - '！' + '!'
		case r == '　':
			return ' '
		default:
			return r
		}
	}, s)
}

// oneLine は、改行を空白に置き換える。ほかの空白 (全角の空白を含む) は、そのまま残す。
func oneLine(s string) string {
	return strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(s)
}

// parseTime は、UNIX 時刻を読む。EPGStation はミリ秒で受け取るが、秒で渡すクライアントもあるので、小さい値は秒とみなす。
func parseTime(s string) (time.Time, error) {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, errInvalidTime
	}
	if v < secondsLimit {
		return time.Unix(v, 0).UTC(), nil
	}

	return time.UnixMilli(v).UTC(), nil
}

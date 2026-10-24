package epgstationapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/usecase/thumbnail"
)

const (
	// defaultRecordedLimit と maxRecordedLimit は、録画の一覧で一度に返す件数の既定値と上限。
	defaultRecordedLimit = 24
	maxRecordedLimit     = 100
	stateRecording       = "recording"
)

type videoFile struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Filename string `json:"filename"`
	Type     string `json:"type"`
	Size     int64  `json:"size"`
}

type recordedItem struct {
	ID          int64       `json:"id"`
	ChannelID   int64       `json:"channelId"`
	StartAt     int64       `json:"startAt"`
	EndAt       int64       `json:"endAt"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Extended    string      `json:"extended,omitempty"`
	Genres      []genre     `json:"genres"`
	Genre1      *int        `json:"genre1,omitempty"`
	SubGenre1   *int        `json:"subGenre1,omitempty"`
	VideoFiles  []videoFile `json:"videoFiles"`
	Thumbnails  []int64     `json:"thumbnails"`
	IsRecording bool        `json:"isRecording"`
	IsProtected bool        `json:"isProtected"`
	IsEncoding  bool        `json:"isEncoding"`
}

type recordedResponse struct {
	Records []*recordedItem `json:"records"`
	Total   int64           `json:"total"`
}

// Recorded は GET /api/recorded に答える。録画を、録画を始めた日時の新しい順に、offset と limit で区切って返す。
func (h *Handler) Recorded(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	offset, errOffset := parseCount(query.Get("offset"), 0)
	limit, errLimit := parseCount(query.Get("limit"), defaultRecordedLimit)
	if errOffset != nil || errLimit != nil {
		http.Error(w, "offset and limit must be non-negative integers", http.StatusBadRequest)

		return
	}
	keyword := query.Get("keyword")
	q := sqlc.New(h.pool)
	rows, err := q.ListAPIRecordings(r.Context(), sqlc.ListAPIRecordingsParams{
		Keyword: keyword, Skip: offset, MaxCount: min(limit, maxRecordedLimit),
	})
	if err != nil {
		h.fail(w, r, fmt.Errorf("failed to list the recordings: %w", err))

		return
	}
	total, err := q.CountAPIRecordings(r.Context(), keyword)
	if err != nil {
		h.fail(w, r, fmt.Errorf("failed to count the recordings: %w", err))

		return
	}
	toHalfWidth := wantsHalfWidth(r)
	records := make([]*recordedItem, 0, len(rows))
	for i := range rows {
		item, err := recordedOf(&rows[i], toHalfWidth, h.hasThumbnail(rows[i].ImageSegmentID))
		if err != nil {
			h.fail(w, r, err)

			return
		}
		records = append(records, item)
	}
	h.writeJSON(w, r, &recordedResponse{Records: records, Total: total})
}

// RecordedItem は GET /api/recorded/{id} に答える。
func (h *Handler) RecordedItem(w http.ResponseWriter, r *http.Request) {
	apiID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid recorded ID", http.StatusBadRequest)

		return
	}
	row, err := sqlc.New(h.pool).GetAPIRecording(r.Context(), apiID)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "recorded not found", http.StatusNotFound)

		return
	}
	if err != nil {
		h.fail(w, r, fmt.Errorf("failed to get the recording: %w", err))

		return
	}
	listed := sqlc.ListAPIRecordingsRow(row)
	item, err := recordedOf(&listed, wantsHalfWidth(r), h.hasThumbnail(listed.ImageSegmentID))
	if err != nil {
		h.fail(w, r, err)

		return
	}
	h.writeJSON(w, r, item)
}

// WithRecording は、パスの {id} の、EPGStation 互換の録画の ID を、Kotatsu の録画の ID に読み替えて、next を呼ぶ。
// 録画の動画 (/api/videos/{id}) とサムネイル (/api/thumbnails/{id}) は、どちらも録画の ID で表すので、
// 既存の録画の配信と代表画像の配信に、そのままつなげる。file が空でなければ、パスの {file} にする。
func (h *Handler) WithRecording(next http.Handler, file string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid ID", http.StatusBadRequest)

			return
		}
		recordingID, err := sqlc.New(h.pool).GetRecordingIDByAPIID(r.Context(), apiID)
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)

			return
		}
		if err != nil {
			h.fail(w, r, fmt.Errorf("failed to look up the recording: %w", err))

			return
		}
		r.SetPathValue("id", recordingID.String())
		if file != "" {
			r.SetPathValue("file", file)
		}
		next.ServeHTTP(w, r)
	})
}

// hasThumbnail は、セグメントのファイル owner のディレクトリに、代表画像があるかを返す。
func (h *Handler) hasThumbnail(owner uuid.UUID) bool {
	if owner == uuid.Nil {
		return false
	}
	image := kago.DataLocation(owner, thumbnail.ThumbnailFileName)
	exists, err := h.store.Exists(image.PoolID, image.Path)

	return err == nil && exists
}

func recordedOf(row *sqlc.ListAPIRecordingsRow, toHalfWidth, hasThumbnail bool) (*recordedItem, error) {
	var genres []*mirakurun.Genre
	if err := json.Unmarshal(row.Genres, &genres); err != nil {
		return nil, fmt.Errorf("failed to decode the genres: %w", err)
	}
	var extended map[string]string
	if err := json.Unmarshal(row.Extended, &extended); err != nil {
		return nil, fmt.Errorf("failed to decode the extended description: %w", err)
	}
	name, description := row.Name, row.Description
	if toHalfWidth {
		name, description = halfWidth(name), halfWidth(description)
	}
	item := &recordedItem{
		ID: row.APIID, ChannelID: row.ServiceID, StartAt: row.StartAt.UnixMilli(), EndAt: row.EndAt.UnixMilli(),
		Name: name, Description: description, Extended: extendedText(extended, toHalfWidth),
		Genres: genresOf(genres), Genre1: nil, SubGenre1: nil,
		// 動画は、セグメントをつないだ TS の 1 つだけ。動画の ID には、録画の ID をそのまま使う。
		VideoFiles:  []videoFile{{ID: row.APIID, Name: "TS", Filename: strconv.FormatInt(row.APIID, 10) + ".m2ts", Type: "ts", Size: row.Size}},
		Thumbnails:  []int64{},
		IsRecording: row.State == stateRecording, IsProtected: row.Protected, IsEncoding: false,
	}
	if len(genres) > 0 {
		item.Genre1, item.SubGenre1 = &genres[0].Lv1, &genres[0].Lv2
	}
	// サムネイルも、録画ごとに 1 つだけなので、録画の ID をそのまま使う。
	if hasThumbnail {
		item.Thumbnails = []int64{row.APIID}
	}

	return item, nil
}

func genresOf(genres []*mirakurun.Genre) []genre {
	result := make([]genre, 0, len(genres))
	for _, g := range genres {
		result = append(result, genre{Lv1: g.Lv1, Lv2: g.Lv2, Un1: g.Un1, Un2: g.Un2})
	}

	return result
}

// parseCount は、0 以上の件数を読む。空なら fallback を返す。
func parseCount(s string, fallback int32) (int32, error) {
	if s == "" {
		return fallback, nil
	}
	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil || v < 0 {
		return 0, errors.New("invalid count")
	}

	return int32(v), nil
}

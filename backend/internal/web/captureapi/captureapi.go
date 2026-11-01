// Package captureapi は、キャプチャの画像を受け取り (POST /media/captures)、撮ったユーザーにだけ返す
// (GET /media/captures/{id}、設計書 7.5、#908)。
//
// キャプチャは撮ったユーザーだけのものなので、どちらもログイン中のセッションでだけ受け付け、署名付き URL では受け付けない。
// ほかのユーザーのキャプチャは、あることも知らせないよう、見つからないものとして答える。
package captureapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/query"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/capture"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
)

const (
	// MetadataPart と ImagePart は、受け取る multipart の部分の名前。MetadataPart は撮った場面の情報の JSON、ImagePart は PNG の画像。
	MetadataPart = "metadata"
	ImagePart    = "image"
	// maxMetadataBytes は、撮った場面の情報の JSON の大きさの上限。字幕の文の上限 (64 KiB) に余裕を足したもの。
	maxMetadataBytes = 128 << 10
	// multipartOverhead は、multipart の区切りと見出しに見込む大きさ。リクエスト全体の上限に足す。
	multipartOverhead = 64 << 10
	// imageCacheControl は、キャプチャの画像の Cache-Control。画像は書き換えないので、長く使い回させる。
	// 撮ったユーザーにだけ見せるので、共有のキャッシュには置かせない。
	imageCacheControl = "private, max-age=31536000, immutable"
)

var (
	errMissingMetadata    = errors.New("the metadata part is missing")
	errMissingImage       = errors.New("the image part is missing")
	errInvalidRecordingID = errors.New("invalid recording ID")
)

// Metadata は、画面が送る、撮った場面の情報。
type Metadata struct {
	ServiceName string  `json:"serviceName"`
	ProgramName *string `json:"programName"`
	Caption     *string `json:"caption"`
	// RecordingID は、録画で撮ったときの録画の ID (GraphQL の ID)。Position は、そのときの録画の始まりからの再生位置 (秒)。
	RecordingID *string  `json:"recordingId"`
	Position    *float64 `json:"position"`
	// BroadcastAt は、撮った場面の放送時刻。TakenAt は、撮った時刻 (端末の時計)。どちらも RFC 3339。
	BroadcastAt time.Time `json:"broadcastAt"`
	TakenAt     time.Time `json:"takenAt"`
}

// UploadResponse は、受け取ったキャプチャの応答。
type UploadResponse struct {
	// ID は、残したキャプチャの GraphQL の ID。
	ID string `json:"id"`
}

// UploadHandler は、キャプチャを受け取って残す (POST /media/captures)。
// 本文は multipart/form-data で、MetadataPart に撮った場面の情報の JSON を、ImagePart に PNG の画像を、この順に送る。
type UploadHandler struct {
	db       *pgxpool.Pool
	captures *capture.Captures
	logger   *slog.Logger
}

var _ http.Handler = new(UploadHandler)

// NewUploadHandler は UploadHandler を返す。
func NewUploadHandler(db *pgxpool.Pool, captures *capture.Captures, logger *slog.Logger) *UploadHandler {
	return &UploadHandler{db: db, captures: captures, logger: logger}
}

func (h *UploadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFrom(r.Context())
	if principal == nil {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)

		return
	}
	if !h.captures.Available() {
		http.Error(w, "captures are unavailable because the data directory is not configured", http.StatusServiceUnavailable)

		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, int64(h.captures.MaxImageBytes()+maxMetadataBytes+multipartOverhead))
	upload, err := h.readUpload(r)
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		http.Error(w, "the request is too large", http.StatusRequestEntityTooLarge)

		return
	case err != nil:
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}

	saved, err := h.captures.Save(r.Context(), h.db, principal.UserID, upload)
	switch {
	case errors.Is(err, capture.ErrUnavailable):
		http.Error(w, err.Error(), http.StatusServiceUnavailable)

		return
	case errors.Is(err, capture.ErrTooLarge):
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)

		return
	case errors.Is(err, capture.ErrInvalidImage), errors.Is(err, capture.ErrInvalidMetadata):
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	case err != nil:
		h.logger.ErrorContext(r.Context(), "failed to save the capture", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(UploadResponse{ID: graph.CaptureID(saved.ID)}); err != nil {
		h.logger.WarnContext(r.Context(), "failed to write the capture response", slog.Any("error", err))
	}
}

// readUpload は、multipart の本文から、撮った場面の情報と画像を読む。
func (h *UploadHandler) readUpload(r *http.Request) (*capture.Upload, error) {
	reader, err := r.MultipartReader()
	if err != nil {
		return nil, fmt.Errorf("the body must be multipart/form-data: %w", err)
	}
	var metadata *Metadata
	var image []byte
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read the body: %w", err)
		}
		switch part.FormName() {
		case MetadataPart:
			metadata, err = readMetadata(part)
		case ImagePart:
			// 上限を 1 バイト超えて読み、大きすぎることを Save に見分けさせる。
			image, err = io.ReadAll(io.LimitReader(part, int64(h.captures.MaxImageBytes())+1))
		}
		_ = part.Close()
		if err != nil {
			return nil, err
		}
	}
	if metadata == nil {
		return nil, errMissingMetadata
	}
	if image == nil {
		return nil, errMissingImage
	}

	return uploadOf(metadata, image)
}

func readMetadata(part *multipart.Part) (*Metadata, error) {
	data, err := io.ReadAll(io.LimitReader(part, maxMetadataBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read the metadata: %w", err)
	}
	if len(data) > maxMetadataBytes {
		return nil, fmt.Errorf("%w: the metadata is too large", capture.ErrInvalidMetadata)
	}
	var metadata Metadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("%w: %w", capture.ErrInvalidMetadata, err)
	}

	return &metadata, nil
}

// uploadOf は、画面が送った情報と画像を、残すキャプチャにする。
func uploadOf(metadata *Metadata, image []byte) (*capture.Upload, error) {
	upload := &capture.Upload{
		ServiceName: metadata.ServiceName,
		ProgramName: metadata.ProgramName,
		Caption:     metadata.Caption,
		RecordingID: nil,
		Position:    nil,
		BroadcastAt: metadata.BroadcastAt,
		TakenAt:     metadata.TakenAt,
		Image:       image,
	}
	if metadata.RecordingID != nil {
		recordingID, err := graph.ParseRecordingID(*metadata.RecordingID)
		if err != nil {
			return nil, errInvalidRecordingID
		}
		upload.RecordingID = &recordingID
	}
	if metadata.Position != nil {
		upload.Position = new(time.Duration(*metadata.Position * float64(time.Second)))
	}

	return upload, nil
}

// ImageHandler は、キャプチャの画像を、撮ったユーザーにだけ返す (GET /media/captures/{id})。
type ImageHandler struct {
	queries *query.Queries
	store   *kago.Store
	logger  *slog.Logger
}

var _ http.Handler = new(ImageHandler)

// NewImageHandler は ImageHandler を返す。store は、このインスタンスにマウントしたデータディレクトリを持つもの。
func NewImageHandler(db *pgxpool.Pool, store *kago.Store, logger *slog.Logger) *ImageHandler {
	return &ImageHandler{queries: query.New(db), store: store, logger: logger}
}

func (h *ImageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 見つからないなどの失敗の応答は覚えさせない。返せたときは、下で上書きする。
	w.Header().Set("Cache-Control", "no-store")
	principal := auth.PrincipalFrom(r.Context())
	if principal == nil {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)

		return
	}
	captureID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)

		return
	}
	found, err := h.queries.GetCapture(r.Context(), principal.UserID, captureID)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)

		return
	}
	if err != nil {
		h.internalError(w, r, err)

		return
	}
	location := kago.CaptureLocation(found.UserID, found.ID)
	f, err := h.store.Open(location.PoolID, location.Path)
	switch {
	case errors.Is(err, kago.ErrPoolNotMounted):
		http.Error(w, "the data directory is not mounted on this instance", http.StatusServiceUnavailable)

		return
	case errors.Is(err, fs.ErrNotExist):
		http.NotFound(w, r)

		return
	case err != nil:
		h.internalError(w, r, err)

		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", imageCacheControl)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "", found.CreatedAt, f)
}

func (h *ImageHandler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.ErrorContext(r.Context(), "failed to serve the capture image", slog.Any("error", err))
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

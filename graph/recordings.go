package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/graph/model"
	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/jikkyo"
	"github.com/SlashNephy/kotatsu/internal/library"
	"github.com/SlashNephy/kotatsu/internal/stream"
	"github.com/SlashNephy/kotatsu/internal/thumbnail"
)

const (
	// recordingType と tagType は、Recording と Tag の ID に付ける型の名前。
	recordingType = "Recording"
	tagType       = "Tag"
	// maxRecordings は、録画を 1 回に返す件数の上限。
	maxRecordings = 200
)

// recordingCursor は、録画のページを送るカーソル。最後に返した録画を始めた日時と ID。
type recordingCursor struct {
	RecordedFrom time.Time `json:"recordedFrom"`
	ID           uuid.UUID `json:"id"`
}

// librarian は、録画を消し、保護し、タグを付ける操作者を返す。視聴者には許さない (設計書 9.7)。
func librarian(ctx context.Context) (*audit.Actor, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	if !principal.Role.Can(auth.PermissionManageRecordings) {
		return nil, NewError(CodeForbidden, "viewers cannot change recordings")
	}

	return actorOf(ctx, principal), nil
}

// listRecordings は、録画を、録画を始めた日時の order の順に first 件まで返す。
func (r *Resolver) listRecordings(
	ctx context.Context, first int, after *string, filter *model.RecordingFilter, order model.RecordingOrder,
) (*model.RecordingConnection, error) {
	if _, err := viewer(ctx); err != nil {
		return nil, err
	}
	if first < 1 || first > maxRecordings {
		return nil, NewError(CodeBadUserInput, fmt.Sprintf("first must be between 1 and %d", maxRecordings))
	}
	params, err := searchParams(filter)
	if err != nil {
		return nil, err
	}
	// 次のページがあるかを知るため、1 件多く読む。
	params.MaxCount = int32(first + 1)
	if after != nil {
		var cursor recordingCursor
		if err := DecodeCursor(*after, &cursor); err != nil {
			return nil, NewError(CodeBadUserInput, "invalid cursor")
		}
		params.BeforeAt, params.BeforeID = &cursor.RecordedFrom, &cursor.ID
	}

	var rows []sqlc.Recording
	if order == model.RecordingOrderOldestFirst {
		rows, err = sqlc.New(r.db).SearchRecordingsOldest(ctx, sqlc.SearchRecordingsOldestParams{
			States: params.States, ServiceIds: params.ServiceIds, TagIds: params.TagIds, Keyword: params.Keyword,
			Since: params.Since, Until: params.Until, AfterAt: params.BeforeAt, AfterID: params.BeforeID, MaxCount: params.MaxCount,
		})
	} else {
		rows, err = sqlc.New(r.db).SearchRecordings(ctx, *params)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to search recordings: %w", err)
	}
	hasNext := len(rows) > first
	rows = rows[:min(len(rows), first)]
	recordings, err := r.recordingModels(ctx, rows)
	if err != nil {
		return nil, err
	}
	edges := make([]*model.RecordingEdge, 0, len(rows))
	for i := range rows {
		cursor, err := EncodeCursor(recordingCursor{RecordedFrom: rows[i].RecordedFrom, ID: rows[i].ID})
		if err != nil {
			return nil, err
		}
		edges = append(edges, &model.RecordingEdge{Cursor: cursor, Node: recordings[i]})
	}
	info := &model.PageInfo{HasNextPage: hasNext, HasPreviousPage: after != nil, StartCursor: nil, EndCursor: nil}
	if len(edges) > 0 {
		info.StartCursor, info.EndCursor = &edges[0].Cursor, &edges[len(edges)-1].Cursor
	}

	return &model.RecordingConnection{Edges: edges, PageInfo: info}, nil
}

// searchParams は、録画の絞り込みを、検索のクエリの引数にする。
func searchParams(filter *model.RecordingFilter) (*sqlc.SearchRecordingsParams, error) {
	params := &sqlc.SearchRecordingsParams{
		States: []string{}, ServiceIds: []int64{}, TagIds: []uuid.UUID{}, Keyword: nil, Since: nil, Until: nil, BeforeAt: nil, BeforeID: nil, MaxCount: 0,
	}
	if filter == nil {
		return params, nil
	}
	for _, state := range filter.States {
		params.States = append(params.States, strings.ToLower(string(state)))
	}
	for _, globalID := range filter.ServiceIds {
		typeName, raw, err := ParseGlobalID(globalID)
		if err != nil || typeName != serviceType {
			return nil, NewError(CodeBadUserInput, "invalid service ID")
		}
		serviceID, err := parseServiceID(raw)
		if err != nil {
			return nil, err
		}
		params.ServiceIds = append(params.ServiceIds, serviceID)
	}
	for _, globalID := range filter.TagIds {
		tagID, err := parseTypedID(globalID, tagType, "invalid tag ID")
		if err != nil {
			return nil, err
		}
		params.TagIds = append(params.TagIds, tagID)
	}
	if filter.Keyword != nil && strings.TrimSpace(*filter.Keyword) != "" {
		params.Keyword = new(escapeLike(strings.TrimSpace(*filter.Keyword)))
	}
	params.Since, params.Until = filter.Since, filter.Until

	return params, nil
}

// escapeLike は、LIKE の特別な文字 (%、_、\) を、文字そのものとして探すようにエスケープする。
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// recording は、ID の録画を返す。なければ NOT_FOUND を返す。
func (r *Resolver) recording(ctx context.Context, recordingID uuid.UUID) (*model.Recording, error) {
	rows, err := sqlc.New(r.db).ListRecordingsByIDs(ctx, []uuid.UUID{recordingID})
	if err != nil {
		return nil, fmt.Errorf("failed to get the recording: %w", err)
	}
	if len(rows) == 0 {
		return nil, NewError(CodeNotFound, "recording not found")
	}
	recordings, err := r.recordingModels(ctx, rows)
	if err != nil {
		return nil, err
	}

	return recordings[0], nil
}

// recordingModels は、録画を GraphQL の形にする。タグ、ファイル、ログイン中のユーザーの再生位置は、まとめて引く。
func (r *Resolver) recordingModels(ctx context.Context, rows []sqlc.Recording) ([]*model.Recording, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].ID)
	}
	q := sqlc.New(r.db)
	tagRows, err := q.ListRecordingTags(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to list the recording tags: %w", err)
	}
	tags := map[uuid.UUID][]*model.Tag{}
	for _, row := range tagRows {
		tags[row.RecordingID] = append(tags[row.RecordingID], &model.Tag{ID: GlobalID(tagType, row.ID.String()), Name: row.Name, Recordings: int(row.Recordings)})
	}
	fileRows, err := q.ListRecordingFilesByRecordings(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to list the recording files: %w", err)
	}
	files := map[uuid.UUID][]*model.RecordingFile{}
	for i := range fileRows {
		row := &fileRows[i]
		file := &model.RecordingFile{
			Kind: model.RecordingFileKind(strings.ToUpper(row.Kind)), PoolID: row.PoolID, Path: row.Path, Size: row.Size,
			StartedAt: row.StartedAt, EndedAt: row.EndedAt, URL: nil,
		}
		if file.Kind == model.RecordingFileKindOther {
			file.URL = new(stream.OtherFileURL(row.RecordingID, row.ID))
		}
		files[row.RecordingID] = append(files[row.RecordingID], file)
	}
	progressRows, err := q.ListWatchProgress(ctx, sqlc.ListWatchProgressParams{UserID: principal.UserID, RecordingIds: ids})
	if err != nil {
		return nil, fmt.Errorf("failed to list the watch progress: %w", err)
	}
	progress := map[uuid.UUID]int64{}
	for _, row := range progressRows {
		progress[row.RecordingID] = row.PositionMs
	}

	recordings := make([]*model.Recording, 0, len(rows))
	for i := range rows {
		recording, err := recordingModel(&rows[i])
		if err != nil {
			return nil, err
		}
		recording.Tags = orEmpty(tags[rows[i].ID])
		recording.Files = orEmpty(files[rows[i].ID])
		recording.ThumbnailURL, recording.PreviewURL = imageURLs(rows[i].ID, recording.Files)
		if position, ok := progress[rows[i].ID]; ok {
			recording.WatchPositionMs = &position
		}
		recordings = append(recordings, recording)
	}

	return recordings, nil
}

// watchEndMargin は、見終えたとみなす、録画の終わりからの時間。
// 再生画面が、続きから再生せずに最初から再生する判定 (web/src/yunomi/resume.ts の endMargin) と揃える。
const watchEndMargin = 10 * time.Second

// watchFrame は、ログイン中のユーザーが前回見た位置に合う、プレビュー画像のコマを返す。
// 再生位置とプレビュー画像のある録画だけ、preview.json を読む。読めなければ、コマなしとして扱い、代表画像を出させる。
func (r *Resolver) watchFrame(ctx context.Context, recording *model.Recording) *model.PreviewFrame {
	if recording.WatchPositionMs == nil || recording.PreviewURL == nil {
		return nil
	}
	position := time.Duration(*recording.WatchPositionMs) * time.Millisecond
	if recording.RecordedUntil != nil && position > recording.RecordedUntil.Sub(recording.RecordedFrom)-watchEndMargin {
		return nil
	}
	var preview *model.RecordingFile
	for _, file := range recording.Files {
		if file.Kind == model.RecordingFileKindPreview {
			preview = file
		}
	}
	if preview == nil {
		return nil
	}
	manifest, err := r.readManifest(preview)
	if err != nil {
		r.logger.WarnContext(ctx, "failed to read the preview manifest", slog.String("recording", recording.ID), slog.Any("error", err))

		return nil
	}
	index, ok := manifest.FrameBefore(position)
	if !ok {
		return nil
	}
	frame, ok := manifest.Frame(index)
	if !ok {
		return nil
	}

	return &model.PreviewFrame{
		URL: *recording.PreviewURL, X: frame.X, Y: frame.Y, Width: frame.Width, Height: frame.Height,
		SheetWidth: frame.SheetWidth, SheetHeight: frame.SheetHeight,
	}
}

// readManifest は、プレビュー画像と同じディレクトリの preview.json を、このインスタンスにマウントしたプールから読む。
func (r *Resolver) readManifest(preview *model.RecordingFile) (*thumbnail.Manifest, error) {
	// 付属ファイルをまとめて置くディレクトリ (kago.SidecarPoolID) にあるものも、同じように読む。
	f, err := r.store.Open(preview.PoolID, path.Join(path.Dir(preview.Path), thumbnail.ManifestFileName))
	if err != nil {
		return nil, fmt.Errorf("failed to open the preview manifest: %w", err)
	}
	defer f.Close()
	var manifest thumbnail.Manifest
	if err := json.NewDecoder(f).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("failed to decode the preview manifest: %w", err)
	}

	return &manifest, nil
}

// imageURLs は、録画の代表画像とプレビュー画像の URL のパスを返す。画像のファイルがなければ nil にする。
// 画像は、GET /media/recordings/{id}/{file} で配信する (thumbnail.ImageHandler)。
func imageURLs(recordingID uuid.UUID, files []*model.RecordingFile) (*string, *string) {
	var thumbnailURL, previewURL *string
	for _, file := range files {
		switch file.Kind {
		case model.RecordingFileKindThumbnail:
			thumbnailURL = new("/media/recordings/" + recordingID.String() + "/" + thumbnail.ThumbnailFileName)
		case model.RecordingFileKindPreview:
			previewURL = new("/media/recordings/" + recordingID.String() + "/" + thumbnail.PreviewFileName)
		case model.RecordingFileKindSegment, model.RecordingFileKindIndex, model.RecordingFileKindOther:
		}
	}

	return thumbnailURL, previewURL
}

func recordingModel(row *sqlc.Recording) (*model.Recording, error) {
	var gaps []*model.RecordingGap
	if err := json.Unmarshal(row.Gaps, &gaps); err != nil {
		return nil, fmt.Errorf("failed to decode the recording gaps: %w", err)
	}
	audios, err := toProgramAudios(row.Audios)
	if err != nil {
		return nil, fmt.Errorf("failed to decode the recording audios: %w", err)
	}
	recording := &model.Recording{
		ID: GlobalID(recordingType, row.ID.String()), EventID: nil, Name: row.Name, Description: row.Description,
		StartAt: row.StartAt, EndAt: row.EndAt, State: model.RecordingState(strings.ToUpper(row.State)), FailureReason: nil,
		Protected: row.Protected, Drops: row.Drops, Errors: row.Errors, Scrambled: row.Scrambled, Gaps: orEmpty(gaps),
		Audios:      audios,
		ProgramLost: row.ProgramLost, RecordedFrom: row.RecordedFrom, RecordedUntil: row.RecordedUntil,
		BroadcastStartedAt: row.BroadcastStartedAt, BroadcastEndedAt: row.BroadcastEndedAt, NotBroadcast: row.NotBroadcast,
		Tags: []*model.Tag{}, Files: []*model.RecordingFile{}, WatchPositionMs: nil, ThumbnailURL: nil, PreviewURL: nil,
		StreamURL:         "/streams/recordings/" + row.ID.String(),
		JikkyoCommentsURL: "/media/recordings/" + row.ID.String() + "/" + jikkyo.CommentsFileName,
		ServiceKey:        row.ServiceID,
	}
	if row.EventID != nil {
		recording.EventID = new(int(*row.EventID))
	}
	if row.FailureReason != nil {
		recording.FailureReason = new(model.RecordingFailureReason(strings.ToUpper(*row.FailureReason)))
	}

	return recording, nil
}

func orEmpty[T any](items []T) []T {
	if items == nil {
		return []T{}
	}

	return items
}

// listTags は、タグを、付いている録画の数と一緒に、名前の順に返す。
func (r *Resolver) listTags(ctx context.Context) ([]*model.Tag, error) {
	if _, err := viewer(ctx); err != nil {
		return nil, err
	}
	rows, err := sqlc.New(r.db).ListTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list tags: %w", err)
	}
	tags := make([]*model.Tag, 0, len(rows))
	for _, row := range rows {
		tags = append(tags, &model.Tag{ID: GlobalID(tagType, row.ID.String()), Name: row.Name, Recordings: int(row.Recordings)})
	}

	return tags, nil
}

func (r *Resolver) deleteRecording(ctx context.Context, globalID string) (string, error) {
	actor, err := librarian(ctx)
	if err != nil {
		return "", err
	}
	recordingID, err := parseRecordingID(globalID)
	if err != nil {
		return "", err
	}
	if err := r.library.Delete(ctx, r.db, actor, recordingID, "user"); err != nil {
		return "", libraryError(err)
	}

	return globalID, nil
}

func (r *Resolver) setRecordingProtected(ctx context.Context, globalID string, protected bool) (*model.Recording, error) {
	actor, err := librarian(ctx)
	if err != nil {
		return nil, err
	}
	recordingID, err := parseRecordingID(globalID)
	if err != nil {
		return nil, err
	}
	if err := r.library.SetProtected(ctx, r.db, actor, recordingID, protected); err != nil {
		return nil, libraryError(err)
	}

	return r.recording(ctx, recordingID)
}

func (r *Resolver) setRecordingTags(ctx context.Context, globalID string, tags []string) (*model.Recording, error) {
	actor, err := librarian(ctx)
	if err != nil {
		return nil, err
	}
	recordingID, err := parseRecordingID(globalID)
	if err != nil {
		return nil, err
	}
	if _, err := r.library.SetTags(ctx, r.db, actor, recordingID, tags); err != nil {
		return nil, libraryError(err)
	}

	return r.recording(ctx, recordingID)
}

func (r *Resolver) saveWatchPosition(ctx context.Context, globalID string, positionMs int64) (*model.Recording, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	recordingID, err := parseRecordingID(globalID)
	if err != nil {
		return nil, err
	}
	if err := r.library.SaveProgress(ctx, r.db, principal.UserID, recordingID, time.Duration(positionMs)*time.Millisecond); err != nil {
		return nil, libraryError(err)
	}

	return r.recording(ctx, recordingID)
}

func parseRecordingID(globalID string) (uuid.UUID, error) {
	return parseTypedID(globalID, recordingType, "invalid recording ID")
}

// libraryError は、録画の操作のエラーを、エラーコードを持つエラーにする。
func libraryError(err error) error {
	switch {
	case errors.Is(err, library.ErrNotFound):
		return NewError(CodeNotFound, "recording not found")
	case errors.Is(err, library.ErrProtected):
		return NewError(CodeConflict, "the recording is protected; unprotect it before deleting")
	case errors.Is(err, library.ErrRecording):
		return NewError(CodeConflict, "the recording is in progress")
	case errors.Is(err, library.ErrInvalidTags), errors.Is(err, library.ErrInvalidPosition):
		return NewError(CodeBadUserInput, err.Error())
	default:
		return err
	}
}

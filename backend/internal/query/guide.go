package query

import (
	"context"
	"fmt"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// ListGuideServicesParams は、ListGuideServices の条件。
type ListGuideServicesParams struct {
	IncludeHidden bool
	// IncludeGone なら、上流から消えたサービスも返す (#1973)。ID から引くときや、管理の画面で使う。
	IncludeGone       bool
	MediaServiceTypes []int64
	IDs               []int64
	// Now は、上流から消えたかどうかを決める時刻。
	Now time.Time
}

// GetGuideProgramParams は、GetGuideProgram の条件。
type GetGuideProgramParams struct {
	ServiceID int64
	EventID   int64
}

// ListProgramsInRangeParams は、ListProgramsInRange の条件。
type ListProgramsInRangeParams struct {
	ServiceIDs []int64
	FromAt     time.Time
	ToAt       time.Time
}

// GetArchivedProgramParams は、GetArchivedProgram の条件。
type GetArchivedProgramParams struct {
	ServiceID int64
	EventID   int64
	StartAt   time.Time
}

// ListGuideProgramsByKeysParams は、ListGuideProgramsByKeys の条件。
type ListGuideProgramsByKeysParams struct {
	ServiceIDs []int64
	EventIDs   []int64
}

// SearchProgramCandidatesParams は、SearchProgramCandidates の条件と、続きから読む位置。
type SearchProgramCandidatesParams struct {
	MediaServiceTypes    []int64
	IncludeArchive       bool
	ArchiveFrom          time.Time
	ArchiveTo            time.Time
	Now                  time.Time
	ChannelTypes         []string
	ServiceIDs           []int64
	ExcludedServiceIDs   []int64
	GenreLv1s            []int64
	ExcludedGenreLv1s    []int64
	FreeOnly             bool
	KeywordPatterns      []string
	KeywordInName        []bool
	KeywordCaseSensitive []bool
	KeywordInDescription []bool
	// RequiredGrams は、キーワードが必ず含む gram (#966、match.RequiredGrams)。空でなければ、gram の索引で候補を絞る。
	RequiredGrams  []string
	AfterStartAt   time.Time
	AfterServiceID int64
	AfterEventID   int64
	BatchSize      int32
}

// GetProgramForFileNameParams は、GetProgramForFileName の条件。
type GetProgramForFileNameParams struct {
	ServiceID int64
	EventID   int64
}

// GetGuideViewParams は、GetGuideView の条件。
type GetGuideViewParams struct {
	ID     uuid.UUID
	UserID uuid.UUID
}

// GetPresentVideoResolutionParams は、GetPresentVideoResolution の条件。
type GetPresentVideoResolutionParams struct {
	ServiceID int64
	Now       time.Time
}

// ListGuideServices は、番組表に並べるサービスを返す。
func (q *Queries) ListGuideServices(ctx context.Context, arg ListGuideServicesParams) ([]domain.Service, error) {
	result, err := sqlc.New(q.pool).ListGuideServices(ctx, sqlc.ListGuideServicesParams{
		IncludeHidden:     arg.IncludeHidden,
		IncludeGone:       arg.IncludeGone,
		GoneBefore:        domain.ServiceGoneBefore(arg.Now),
		MediaServiceTypes: arg.MediaServiceTypes,
		Ids:               arg.IDs,
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Service, len(result))
	for i := range result {
		out[i] = domain.Service(result[i])
	}

	return out, nil
}

// ListChannelTypes は、チャンネルの種類を、画面の並び順で返す。
func (q *Queries) ListChannelTypes(ctx context.Context) ([]domain.ChannelType, error) {
	result, err := sqlc.New(q.pool).ListChannelTypes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.ChannelType, len(result))
	for i := range result {
		out[i] = domain.ChannelType(result[i])
	}

	return out, nil
}

// ListChannelTypesByTypes は、types のチャンネルの種類を返す。
func (q *Queries) ListChannelTypesByTypes(ctx context.Context, types []string) ([]domain.ChannelType, error) {
	result, err := sqlc.New(q.pool).ListChannelTypesByTypes(ctx, types)
	if err != nil {
		return nil, err
	}
	out := make([]domain.ChannelType, len(result))
	for i := range result {
		out[i] = domain.ChannelType(result[i])
	}

	return out, nil
}

// ListChannelTypesWithServices は、types のうち、now の時点で上流から消えていないサービスを持つ種別を返す (#1973)。
func (q *Queries) ListChannelTypesWithServices(ctx context.Context, types []string, now time.Time) ([]string, error) {
	return sqlc.New(q.pool).ListChannelTypesWithServices(ctx, sqlc.ListChannelTypesWithServicesParams{
		Types:      types,
		GoneBefore: domain.ServiceGoneBefore(now),
	})
}

// GetGuideProgram は、現役の番組を 1 つ返す。なければ pgx.ErrNoRows を返す。
func (q *Queries) GetGuideProgram(ctx context.Context, arg GetGuideProgramParams) (domain.Program, error) {
	result, err := sqlc.New(q.pool).GetGuideProgram(ctx, sqlc.GetGuideProgramParams(arg))
	if err != nil {
		return domain.Program{}, err
	}

	return domain.Program(result), nil
}

// ListProgramsInRange は、サービスの、期間に重なる番組を返す。
func (q *Queries) ListProgramsInRange(ctx context.Context, arg ListProgramsInRangeParams) ([]domain.Program, error) {
	result, err := sqlc.New(q.pool).ListProgramsInRange(ctx, sqlc.ListProgramsInRangeParams{ServiceIds: arg.ServiceIDs, FromAt: arg.FromAt, ToAt: arg.ToAt})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Program, len(result))
	for i := range result {
		out[i] = domain.Program(result[i])
	}

	return out, nil
}

// ListArchivedProgramsInRange は、サービスの、期間に重なる、放送が終わった番組を返す (過去の番組表、#1121)。
func (q *Queries) ListArchivedProgramsInRange(ctx context.Context, arg ListProgramsInRangeParams) ([]domain.Program, error) {
	result, err := sqlc.New(q.pool).ListArchivedProgramsInRange(ctx, sqlc.ListArchivedProgramsInRangeParams{ServiceIds: arg.ServiceIDs, FromAt: arg.FromAt, ToAt: arg.ToAt})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Program, len(result))
	for i := range result {
		out[i] = domain.Program(result[i])
	}

	return out, nil
}

// GetOldestArchivedProgramStartAt は、最も古い、放送が終わった番組の開始時刻を返す。なければ pgx.ErrNoRows を返す。
func (q *Queries) GetOldestArchivedProgramStartAt(ctx context.Context) (time.Time, error) {
	return sqlc.New(q.pool).GetOldestArchivedProgramStartAt(ctx)
}

// ListProgramRecordings は、番組 (サービス, eventId) の、放送時間に重なる録画を返す (#1121)。
func (q *Queries) ListProgramRecordings(ctx context.Context, serviceID, eventID int64, startAt, endAt time.Time) ([]domain.Recording, error) {
	result, err := sqlc.New(q.pool).ListProgramRecordings(ctx, sqlc.ListProgramRecordingsParams{
		ServiceID: serviceID, EventID: &eventID, StartAt: startAt, EndAt: endAt,
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Recording, len(result))
	for i := range result {
		out[i] = domain.Recording(result[i])
	}

	return out, nil
}

// GetArchivedProgram は、アーカイブの番組を 1 つ返す。なければ pgx.ErrNoRows を返す。
func (q *Queries) GetArchivedProgram(ctx context.Context, arg GetArchivedProgramParams) (domain.Program, error) {
	result, err := sqlc.New(q.pool).GetArchivedProgram(ctx, sqlc.GetArchivedProgramParams(arg))
	if err != nil {
		return domain.Program{}, err
	}

	return domain.Program(result), nil
}

// ListGuideProgramsByKeys は、番組 (サービス, eventId) をまとめて返す。ない番組は返さない。
func (q *Queries) ListGuideProgramsByKeys(ctx context.Context, arg ListGuideProgramsByKeysParams) ([]domain.Program, error) {
	result, err := sqlc.New(q.pool).ListGuideProgramsByKeys(ctx, sqlc.ListGuideProgramsByKeysParams{ServiceIds: arg.ServiceIDs, EventIds: arg.EventIDs})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Program, len(result))
	for i := range result {
		out[i] = domain.Program(result[i])
	}

	return out, nil
}

// SearchProgramCandidates は、番組の検索の候補を、開始時刻、サービス、eventId の順に 1 回分返す。
// キーワードが必ず含む gram (RequiredGrams) があれば、gram の索引で絞る SQL を使う (#966)。
func (q *Queries) SearchProgramCandidates(ctx context.Context, arg SearchProgramCandidatesParams) ([]domain.ProgramCandidate, error) {
	if len(arg.RequiredGrams) > 0 {
		return q.searchProgramCandidatesByGrams(ctx, arg)
	}
	result, err := sqlc.New(q.pool).SearchProgramCandidates(ctx, sqlc.SearchProgramCandidatesParams{MediaServiceTypes: arg.MediaServiceTypes, IncludeArchive: arg.IncludeArchive, ArchiveFrom: arg.ArchiveFrom, ArchiveTo: arg.ArchiveTo, Now: arg.Now, ChannelTypes: arg.ChannelTypes, ServiceIds: arg.ServiceIDs, ExcludedServiceIds: arg.ExcludedServiceIDs, GenreLv1s: arg.GenreLv1s, ExcludedGenreLv1s: arg.ExcludedGenreLv1s, FreeOnly: arg.FreeOnly, KeywordPatterns: arg.KeywordPatterns, KeywordInName: arg.KeywordInName, KeywordCaseSensitive: arg.KeywordCaseSensitive, KeywordInDescription: arg.KeywordInDescription, AfterStartAt: arg.AfterStartAt, AfterServiceID: arg.AfterServiceID, AfterEventID: arg.AfterEventID, BatchSize: arg.BatchSize})
	if err != nil {
		return nil, err
	}
	out := make([]domain.ProgramCandidate, len(result))
	for i := range result {
		out[i] = domain.ProgramCandidate(result[i])
	}

	return out, nil
}

// searchProgramCandidatesByGrams は、gram の索引で絞って、番組の検索の候補を 1 回分返す (#966)。
// 引数の値ごとに計画を作らせる (plan_cache_mode = force_custom_plan)。プリペアドステートメントの汎用の計画では、
// アーカイブの期間の行の数を少なく見積もり、期間の索引で読んで gram を 1 件ずつ比べるので、30 日分で 1 秒近くかかる
// (開発用の環境の約 20 万件のアーカイブで、値ごとの計画の 9 ms に対し 850 ms)。SET LOCAL はトランザクションの中だけに効く。
func (q *Queries) searchProgramCandidatesByGrams(ctx context.Context, arg SearchProgramCandidatesParams) ([]domain.ProgramCandidate, error) {
	var out []domain.ProgramCandidate
	err := pgx.BeginFunc(ctx, q.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET LOCAL plan_cache_mode = force_custom_plan"); err != nil {
			return fmt.Errorf("failed to set the plan cache mode: %w", err)
		}
		result, err := sqlc.New(tx).SearchProgramCandidatesByGrams(ctx, sqlc.SearchProgramCandidatesByGramsParams{MediaServiceTypes: arg.MediaServiceTypes, IncludeArchive: arg.IncludeArchive, ArchiveFrom: arg.ArchiveFrom, ArchiveTo: arg.ArchiveTo, Now: arg.Now, ChannelTypes: arg.ChannelTypes, ServiceIds: arg.ServiceIDs, ExcludedServiceIds: arg.ExcludedServiceIDs, GenreLv1s: arg.GenreLv1s, ExcludedGenreLv1s: arg.ExcludedGenreLv1s, FreeOnly: arg.FreeOnly, KeywordPatterns: arg.KeywordPatterns, KeywordInName: arg.KeywordInName, KeywordCaseSensitive: arg.KeywordCaseSensitive, KeywordInDescription: arg.KeywordInDescription, RequiredGrams: arg.RequiredGrams, AfterStartAt: arg.AfterStartAt, AfterServiceID: arg.AfterServiceID, AfterEventID: arg.AfterEventID, BatchSize: arg.BatchSize})
		if err != nil {
			return err
		}
		out = make([]domain.ProgramCandidate, len(result))
		for i := range result {
			out[i] = domain.ProgramCandidate(result[i])
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return out, nil
}

// GetProgramForFileName は、録画ファイルの名前の見本に当てはめる番組を返す。なければ pgx.ErrNoRows を返す。
func (q *Queries) GetProgramForFileName(ctx context.Context, arg GetProgramForFileNameParams) (domain.FileNameProgram, error) {
	result, err := sqlc.New(q.pool).GetProgramForFileName(ctx, sqlc.GetProgramForFileNameParams(arg))
	if err != nil {
		return domain.FileNameProgram{}, err
	}

	return domain.FileNameProgram(result), nil
}

// GetPresentVideoResolution は、サービスで放送中の番組の映像の解像度を返す。
func (q *Queries) GetPresentVideoResolution(ctx context.Context, arg GetPresentVideoResolutionParams) (*string, error) {
	result, err := sqlc.New(q.pool).GetPresentVideoResolution(ctx, sqlc.GetPresentVideoResolutionParams(arg))
	if err != nil {
		return nil, err
	}

	return result, nil
}

// ServiceExists は、サービスがあるかを返す。
func (q *Queries) ServiceExists(ctx context.Context, id int64) (bool, error) {
	result, err := sqlc.New(q.pool).ServiceExists(ctx, id)
	if err != nil {
		return false, err
	}

	return result, nil
}

// GetGuideView は、ユーザーの番組表ビューを 1 つ返す。なければ pgx.ErrNoRows を返す。
func (q *Queries) GetGuideView(ctx context.Context, arg GetGuideViewParams) (domain.GuideView, error) {
	result, err := sqlc.New(q.pool).GetGuideView(ctx, sqlc.GetGuideViewParams(arg))
	if err != nil {
		return domain.GuideView{}, err
	}

	return domain.GuideView(result), nil
}

// ListGuideViews は、ユーザーの番組表ビューを返す。
func (q *Queries) ListGuideViews(ctx context.Context, userID uuid.UUID) ([]domain.GuideView, error) {
	result, err := sqlc.New(q.pool).ListGuideViews(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.GuideView, len(result))
	for i := range result {
		out[i] = domain.GuideView(result[i])
	}

	return out, nil
}

// ListWatchReservations は、ユーザーの視聴予約と、その番組を返す。
func (q *Queries) ListWatchReservations(ctx context.Context, userID uuid.UUID) ([]domain.WatchReservation, error) {
	result, err := sqlc.New(q.pool).ListWatchReservations(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.WatchReservation, len(result))
	for i := range result {
		out[i] = domain.WatchReservation(result[i])
	}

	return out, nil
}

// ListPinnedServiceIDs は、ユーザーがピン留めしたサービスの ID を、ピン留めした順に返す。
func (q *Queries) ListPinnedServiceIDs(ctx context.Context, userID uuid.UUID) ([]int64, error) {
	result, err := sqlc.New(q.pool).ListPinnedServiceIDs(ctx, userID)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// ProgramSpan は、番組 (サービス, eventId) とその放送時間。eventId は使い回されるので、時間と組にして番組を指す。
type ProgramSpan struct {
	ServiceID int64
	EventID   int64
	StartAt   time.Time
	EndAt     time.Time
}

// RecordedProgramKey は、RecordedPrograms が返す、録画のある番組を引くキー。時刻はミリ秒で持ち、タイムゾーンの違いで外さない。
type RecordedProgramKey struct {
	ServiceID   int64
	EventID     int64
	StartUnixMs int64
}

// Key は、番組 p を RecordedPrograms の結果から引くキーを返す。
func (p *ProgramSpan) Key() RecordedProgramKey {
	return RecordedProgramKey{ServiceID: p.ServiceID, EventID: p.EventID, StartUnixMs: p.StartAt.UnixMilli()}
}

// RecordedPrograms は、programs のうち、録画のある番組を返す (#1278)。失敗した録画は数えない。
func (q *Queries) RecordedPrograms(ctx context.Context, programs []*ProgramSpan) (map[RecordedProgramKey]bool, error) {
	recorded := make(map[RecordedProgramKey]bool, len(programs))
	if len(programs) == 0 {
		return recorded, nil
	}
	params := sqlc.ListRecordedProgramsParams{
		ServiceIds: make([]int64, 0, len(programs)),
		EventIds:   make([]int64, 0, len(programs)),
		StartAts:   make([]time.Time, 0, len(programs)),
		EndAts:     make([]time.Time, 0, len(programs)),
	}
	for _, program := range programs {
		params.ServiceIds = append(params.ServiceIds, program.ServiceID)
		params.EventIds = append(params.EventIds, program.EventID)
		params.StartAts = append(params.StartAts, program.StartAt)
		params.EndAts = append(params.EndAts, program.EndAt)
	}
	rows, err := sqlc.New(q.pool).ListRecordedPrograms(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list recorded programs: %w", err)
	}
	for _, row := range rows {
		recorded[RecordedProgramKey{ServiceID: row.ServiceID, EventID: row.EventID, StartUnixMs: row.StartAt.UnixMilli()}] = true
	}

	return recorded, nil
}

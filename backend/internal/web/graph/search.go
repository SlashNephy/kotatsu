package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/domain/match"
	"github.com/SlashNephy/kotatsu/internal/query"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

const (
	archiveType = "ProgramArchive"
	// defaultArchiveRange は、過去の番組の検索の期間の既定値 (設計書 3.4)。
	defaultArchiveRange = 30 * 24 * time.Hour
	// maxSearchResults は、一度に返す件数の上限。
	maxSearchResults = 100
	// searchBatchSize は、候補を DB から読む 1 回の件数。照合で落ちる候補が多いので、返す件数より多めに読む。
	searchBatchSize = 500
	// searchTimeout は、1 回の検索にかけてよい時間。一致する番組が少ないと、候補を最後まで照合するので、これで守る。
	// キーワードの多くは SQL で粗く絞るので、ふつうの検索はこれよりずっと早く終わる。
	searchTimeout = 15 * time.Second
	// maxConcurrentSearches は、tenban ごとに同時に照合する検索 (番組の検索とルールのプレビュー) の数の上限。
	maxConcurrentSearches = 4
)

// searchCursor は、番組の検索のカーソルの位置。並べ替えのキーと同じ。
type searchCursor struct {
	StartAt   time.Time `json:"startAt"`
	ServiceID int64     `json:"serviceId"`
	EventID   int64     `json:"eventId"`
}

func archiveGlobalID(serviceID, eventID int64, startAt time.Time) string {
	return GlobalID(archiveType, strconv.FormatInt(serviceID, 10)+"."+strconv.FormatInt(eventID, 10)+"."+strconv.FormatInt(startAt.Unix(), 10))
}

// searchPrograms は、条件に一致する番組を、開始時刻の順に first 件まで返す (設計書 3.5)。
// 期間、チャンネル種別、サービスでの絞り込みは SQL で行い、キーワードなどの照合はルールの照合と同じ関数で行う。
func (r *Resolver) searchPrograms(
	ctx context.Context,
	in *model.ProgramConditionsInput,
	archive *model.ArchiveRangeInput,
	first int,
	after *string,
) (*model.ProgramConnection, error) {
	if first < 1 || first > maxSearchResults {
		return nil, NewError(CodeBadUserInput, fmt.Sprintf("first must be between 1 and %d", maxSearchResults))
	}
	conditions, err := toConditions(in)
	if err != nil {
		return nil, err
	}
	matcher, err := match.Compile(conditions, r.location)
	if err != nil {
		return nil, NewError(CodeBadUserInput, err.Error())
	}

	params, err := r.searchParams(conditions, archive, after)
	if err != nil {
		return nil, err
	}
	edges, hasNext, err := r.collect(ctx, matcher, nil, params, first, after != nil)
	if err != nil {
		return nil, err
	}

	return &model.ProgramConnection{Edges: edges, PageInfo: pageInfo(edges, hasNext, after != nil)}, nil
}

// collect は、候補を順に照合し、一致した番組を first 件まで集める。accept が nil でなければ、accept も満たす番組だけを集める。
// first 件そろうか、候補を照合し終えるまで読む。続きがあるかは、first + 1 件目が一致するかで決めるので、
// 続きがあると返すのは、本当に続きがあるときだけになる。searchTimeout を過ぎたら、条件を絞ってもらうエラーにする。
// サブチャンネルが元のサービスと同じ番組を流している (イベント共有) ときは、元の番組も一致していれば、サブチャンネルの番組を省く。
// resumed は、params がカーソルの続きから読むかどうか。続きから読むときは、カーソルと同じ開始時刻の、前のページの番組も見て省く。
func (r *Resolver) collect(
	ctx context.Context,
	matcher *match.Matcher,
	accept func(startAt time.Time, endUndecided bool) bool,
	params *query.SearchProgramCandidatesParams,
	first int,
	resumed bool,
) ([]*model.ProgramEdge, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()
	// 同時に照合する検索の数を抑え、DB と CPU を長く占有させない (#541)。空くのを待つ時間も searchTimeout に含める。
	select {
	case r.searchSlots <- struct{}{}:
		defer func() { <-r.searchSlots }()
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, false, NewError(CodeBadUserInput, "too many searches are running; try again later")
		}

		return nil, false, ctx.Err()
	}
	shared := &sharedEvents{startAt: time.Time{}, services: map[int64]bool{}}
	if resumed {
		if err := r.primeSharedEvents(ctx, matcher, accept, *params, shared); err != nil {
			return nil, false, err
		}
	}
	edges := []*model.ProgramEdge{}
	for {
		rows, err := r.searchCandidates(ctx, params)
		if err != nil {
			return nil, false, err
		}
		for i := range rows {
			row := &rows[i]
			edge, matched, err := r.matchCandidate(matcher, accept, row)
			if err != nil {
				return nil, false, err
			}
			// 省くサブチャンネルの番組は、件数にも続きの有無にも数えない。
			matched = matched && !shared.duplicate(edge.Node)
			if matched && len(edges) == first {
				return edges, true, nil
			}
			params.AfterStartAt, params.AfterServiceID, params.AfterEventID = row.StartAt, row.ServiceID, row.EventID
			if matched {
				edges = append(edges, edge)
			}
		}
		if len(rows) < int(params.BatchSize) {
			return edges, false, nil
		}
	}
}

// searchCandidates は、params の位置から、候補を 1 回分読む。
func (r *Resolver) searchCandidates(ctx context.Context, params *query.SearchProgramCandidatesParams) ([]domain.ProgramCandidate, error) {
	rows, err := r.queries.SearchProgramCandidates(ctx, *params)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, NewError(CodeBadUserInput, "the search took too long; narrow down the conditions")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to search programs: %w", err)
	}

	return rows, nil
}

// primeSharedEvents は、カーソルと同じ開始時刻で、カーソルまで (前のページ) に一致した番組を、shared に覚えさせる。
// 前のページで元の番組を返していれば、このページの同じ番組のサブチャンネルを省ける。
func (r *Resolver) primeSharedEvents(
	ctx context.Context,
	matcher *match.Matcher,
	accept func(startAt time.Time, endUndecided bool) bool,
	params query.SearchProgramCandidatesParams,
	shared *sharedEvents,
) error {
	cursor := searchCursor{StartAt: params.AfterStartAt, ServiceID: params.AfterServiceID, EventID: params.AfterEventID}
	// 開始時刻の精度 (マイクロ秒) の分だけ前から読み、カーソルと同じ開始時刻の番組を先頭から読む。
	params.AfterStartAt, params.AfterServiceID, params.AfterEventID = cursor.StartAt.Add(-time.Microsecond), 0, 0
	for {
		rows, err := r.searchCandidates(ctx, &params)
		if err != nil {
			return err
		}
		for i := range rows {
			row := &rows[i]
			if row.StartAt.After(cursor.StartAt) || (row.StartAt.Equal(cursor.StartAt) &&
				(row.ServiceID > cursor.ServiceID || (row.ServiceID == cursor.ServiceID && row.EventID > cursor.EventID))) {
				return nil
			}
			edge, matched, err := r.matchCandidate(matcher, accept, row)
			if err != nil {
				return err
			}
			if matched {
				shared.duplicate(edge.Node)
			}
			params.AfterStartAt, params.AfterServiceID, params.AfterEventID = row.StartAt, row.ServiceID, row.EventID
		}
		if len(rows) < int(params.BatchSize) {
			return nil
		}
	}
}

// sharedEvents は、同じ開始時刻に一致した番組のサービスを覚え、イベント共有でサブチャンネルが元のサービスと同じ番組を流しているのを見分ける。
// 候補は開始時刻、サービスの順に並び、元のサービスは共有するサービスのうちサービス ID が最も小さいので、同じ開始時刻の中で先に来る。
// 残すのは、ルールの照合 (rule.pickSharedEvents) と同じく、サービス ID の最も小さい番組になる。
type sharedEvents struct {
	startAt  time.Time
	services map[int64]bool
}

// duplicate は、program が、すでに一致した元のサービスの番組と同じ番組かどうかを返す。そうでなければ、program を覚える。
// 元の番組が一致していない (条件で外れたなど) ときは、省くと見られなくなるので、false を返す。
func (s *sharedEvents) duplicate(program *model.Program) bool {
	if !program.StartAt.Equal(s.startAt) {
		s.startAt, s.services = program.StartAt, map[int64]bool{}
	}
	if program.SharedFromKey != 0 && s.services[program.SharedFromKey] {
		return true
	}
	s.services[program.ServiceKey] = true

	return false
}

// archivedProgram は、ProgramArchive の ID の中身 (サービス ID.eventId.開始時刻の UNIX 時間) から、アーカイブの番組を引く。
func (r *Resolver) archivedProgram(ctx context.Context, raw string) (*model.Program, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return nil, NewError(CodeBadUserInput, "invalid archived program ID")
	}
	serviceID, eventID, err := parseProgramID(parts[0] + "." + parts[1])
	if err != nil {
		return nil, err
	}
	unix, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return nil, NewError(CodeBadUserInput, "invalid archived program ID")
	}
	row, err := r.queries.GetArchivedProgram(ctx, query.GetArchivedProgramParams{ServiceID: serviceID, EventID: eventID, StartAt: time.Unix(unix, 0)})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, NewError(CodeNotFound, "program not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the archived program: %w", err)
	}
	program, err := toProgram(&row)
	if err != nil {
		return nil, err
	}
	program.ID = archiveGlobalID(serviceID, eventID, row.StartAt)
	program.Archived = true

	return program, nil
}

// searchParams は、候補を引くクエリの引数を作る。
func (r *Resolver) searchParams(conditions *match.Conditions, archive *model.ArchiveRangeInput, after *string) (*query.SearchProgramCandidatesParams, error) {
	params := &query.SearchProgramCandidatesParams{
		IncludeArchive:     archive != nil,
		Now:                r.clock.Now(),
		MediaServiceTypes:  domain.MediaServiceTypes(),
		ArchiveFrom:        time.Time{},
		ArchiveTo:          time.Time{},
		ChannelTypes:       conditions.ChannelTypes,
		ServiceIDs:         conditions.ServiceIDs,
		ExcludedServiceIDs: nonNil(conditions.ExcludeServiceIDs),
		GenreLv1s:          genreLv1s(conditions.Genres, false),
		ExcludedGenreLv1s:  genreLv1s(conditions.ExcludeGenres, true),
		FreeOnly:           conditions.FreeOnly,
		// キーワードの絞り込みは、setKeywordPatterns で入れる。
		KeywordPatterns:      []string{},
		KeywordInName:        []bool{},
		KeywordInDescription: []bool{},
		KeywordCaseSensitive: []bool{},
		// カーソルが無ければ、最も古い位置から読む。
		AfterStartAt:   time.Unix(0, 0).UTC(),
		AfterServiceID: 0,
		AfterEventID:   0,
		BatchSize:      searchBatchSize,
	}
	setKeywordPatterns(params, conditions)
	if archive != nil {
		params.ArchiveTo = params.Now
		if archive.To != nil {
			params.ArchiveTo = *archive.To
		}
		params.ArchiveFrom = params.ArchiveTo.Add(-defaultArchiveRange)
		if archive.From != nil {
			params.ArchiveFrom = *archive.From
		}
		if !params.ArchiveFrom.Before(params.ArchiveTo) {
			return nil, NewError(CodeBadUserInput, "archive.from must be before archive.to")
		}
	}
	if after != nil {
		var cursor searchCursor
		if err := DecodeCursor(*after, &cursor); err != nil {
			return nil, NewError(CodeBadUserInput, "invalid cursor")
		}
		params.AfterStartAt, params.AfterServiceID, params.AfterEventID = cursor.StartAt, cursor.ServiceID, cursor.EventID
	}

	return params, nil
}

// setKeywordPatterns は、SQL で粗く絞るキーワードを、LIKE のパターンにして params に入れる。
// SQL はすべてのパターンに一致する番組に絞るので、いずれかのキーワードに一致すればよいとき (KeywordMatchAny) は絞らない。
// 正規表現のキーワードと、詳細も見るキーワードは、正規化した列がないので、Go の照合に任せる。
// 照合関数と同じく NFKC で正規化した語を、正規化した列 (name_normalized、description_normalized) の部分一致で探す。
// 大文字と小文字を区別しないときの ILIKE は、Go の (?i) とほぼ同じ範囲を一致とみなす。
func setKeywordPatterns(params *query.SearchProgramCandidatesParams, conditions *match.Conditions) {
	if conditions.KeywordMatch == match.KeywordMatchAny {
		return
	}
	keywords := conditions.Keywords
	// LIKE のパターンで特別な意味を持つ文字は、文字どおりに一致させる。
	likeEscaper := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`)
	for _, keyword := range keywords {
		if keyword.Regexp || slices.Contains(keyword.Fields, match.FieldExtended) {
			continue
		}
		params.KeywordPatterns = append(params.KeywordPatterns, "%"+likeEscaper.Replace(domain.Normalize(keyword.Text))+"%")
		params.KeywordInName = append(params.KeywordInName, slices.Contains(keyword.Fields, match.FieldName))
		params.KeywordInDescription = append(params.KeywordInDescription, slices.Contains(keyword.Fields, match.FieldDescription))
		params.KeywordCaseSensitive = append(params.KeywordCaseSensitive, keyword.CaseSensitive)
	}
}

// matchCandidate は、候補が条件に一致するかを照合し、一致すれば一覧の要素にする。accept が nil でなければ、accept も満たす必要がある。
// 一覧の要素は、一致した候補だけで作る。ほとんどの候補は一致しないので、詳細などの JSON を読むのを省く (#852)。
func (r *Resolver) matchCandidate(
	matcher *match.Matcher,
	accept func(startAt time.Time, endUndecided bool) bool,
	row *domain.ProgramCandidate,
) (*model.ProgramEdge, bool, error) {
	if accept != nil && !accept(row.StartAt, row.EndUndecided) {
		return nil, false, nil
	}
	candidate, err := toMatchCandidate(row)
	if err != nil {
		return nil, false, err
	}
	if !matcher.Match(candidate) {
		return nil, false, nil
	}

	program, err := toProgram(&domain.Program{
		ServiceID: row.ServiceID, EventID: row.EventID, StartAt: row.StartAt, EndAt: row.EndAt,
		EndUndecided: row.EndUndecided, IsFree: row.IsFree, Name: row.Name, Description: row.Description,
		Extended: row.Extended, Genres: row.Genres, Audios: row.Audios, RelatedItems: row.RelatedItems,
	})
	if err != nil {
		return nil, false, err
	}
	if row.Archived {
		program.ID = archiveGlobalID(row.ServiceID, row.EventID, row.StartAt)
		program.Archived = true
	}
	cursor, err := EncodeCursor(searchCursor{StartAt: row.StartAt, ServiceID: row.ServiceID, EventID: row.EventID})
	if err != nil {
		return nil, false, err
	}

	return &model.ProgramEdge{Cursor: cursor, Node: program}, true, nil
}

// toMatchCandidate は、候補を照合する形にする。ルールの照合 (rule.toMatchProgram) と同じく、同期で正規化して持った列を使う。
// 候補ごとに NFKC で正規化し直すと、検索の時間の大半を占めるからである (#852)。
func toMatchCandidate(row *domain.ProgramCandidate) (*match.Program, error) {
	upstreamGenres := []*domain.Genre{}
	if err := json.Unmarshal(row.Genres, &upstreamGenres); err != nil {
		return nil, fmt.Errorf("failed to decode the genres of program %d/%d: %w", row.ServiceID, row.EventID, err)
	}
	genres := make([]*match.Genre, 0, len(upstreamGenres))
	for _, g := range upstreamGenres {
		genres = append(genres, &match.Genre{Lv1: g.Lv1, Lv2: new(g.Lv2)})
	}
	extended := row.ExtendedNormalized
	if extended == nil {
		// 列を足す前に書いた番組と、列を足す前にアーカイブへ移した番組は、上流の詳細から正規化する。
		upstream := map[string]string{}
		if err := json.Unmarshal(row.Extended, &upstream); err != nil {
			return nil, fmt.Errorf("failed to decode the extended of program %d/%d: %w", row.ServiceID, row.EventID, err)
		}
		extended = new(domain.NormalizeExtended(upstream))
	}

	sharedFrom, err := domain.SharedFrom(row.ServiceID, row.RelatedItems)
	if err != nil {
		return nil, fmt.Errorf("failed to decode the related items of program %d/%d: %w", row.ServiceID, row.EventID, err)
	}

	return &match.Program{
		ServiceID:   row.ServiceID,
		SharedFrom:  sharedFrom,
		ChannelType: row.ChannelType,
		Name:        row.NameNormalized,
		Description: row.DescriptionNormalized,
		Extended:    *extended,
		Genres:      genres,
		StartAt:     row.StartAt,
		Duration:    row.EndAt.Sub(row.StartAt),
		IsFree:      row.IsFree,
		// 囲み文字を括弧の書き方にする前に正規化した番組名では、囲み文字と番組名の文字を見分けられないので、
		// 囲み文字は正規化する前の番組名から取り出す。
		Marks: domain.Marks(row.Name),
	}, nil
}

// genreLv1s は、SQL で絞り込むジャンルの大分類を返す。wholeOnly なら、中分類を指定していない (大分類のすべての) ものだけを返す。
// 中分類まで指定した除外は、大分類で除くと除きすぎるので、Go の照合に任せる。
func genreLv1s(genres []*match.Genre, wholeOnly bool) []int64 {
	lv1s := []int64{}
	for _, g := range genres {
		if wholeOnly && g.Lv2 != nil {
			continue
		}
		lv1s = append(lv1s, int64(g.Lv1))
	}

	return lv1s
}

// pageInfo は、一覧のページの情報を作る。前のページは、カーソルを渡されたかどうかで判断する。
func pageInfo(edges []*model.ProgramEdge, hasNext, hasPrevious bool) *model.PageInfo {
	info := &model.PageInfo{HasNextPage: hasNext, HasPreviousPage: hasPrevious, StartCursor: nil, EndCursor: nil}
	if len(edges) > 0 {
		info.StartCursor = &edges[0].Cursor
		info.EndCursor = &edges[len(edges)-1].Cursor
	}

	return info
}

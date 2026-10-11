package epg

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/notify"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
)

const (
	// NotifyChannel は、EPG が変わったことを知らせる変更通知のチャンネル。
	NotifyChannel = "epg"
	// changeRetention は、EPG の変更の記録を残す期間。受け取る側は、これより長く止まっていたら全体を読み直す。
	changeRetention = 24 * time.Hour
	// serviceIDBase は、Mirakurun のサービス ID と番組 ID を組み立てる基数 (networkId × 100000 + serviceId)。
	serviceIDBase = 100_000
)

// Upstream は、同期に使うチューナーサーバーの API。*mirakurun.Client が満たす。
type Upstream interface {
	Services(ctx context.Context) ([]*mirakurun.Service, error)
	Programs(ctx context.Context, query *mirakurun.ProgramsQuery) ([]*mirakurun.Program, error)
	// OnAirPrograms は、放送中と次の番組 (EIT[p/f]) を返す。持たないサーバーでは空を返す。
	OnAirPrograms(ctx context.Context) ([]*mirakurun.OnAirProgram, error)
}

var _ Upstream = new(mirakurun.Client)

// Guard は、書き込みのトランザクションの中で、まだ neko のリーダーであることを確かめる。
// リーダーでなくなっていればエラーを返し、トランザクションを捨てさせる (設計書 1.4)。
type Guard func(ctx context.Context, tx pgx.Tx) error

// Result は、1 回の反映の結果。
type Result struct {
	ServicesUpserted int
	ProgramsUpserted int
	ProgramsDeleted  int
	// Version は、変更を通知したときの版数。変更が無く、通知しなかったときは 0。
	Version int64
}

// Reconciler は、チューナーサーバーの EPG を取得し、DB と差分で突き合わせて反映する (設計書 3.3)。
// 全件を削除して入れ直すことはせず、変わった行だけを書く。
type Reconciler struct {
	pool     *pgxpool.Pool
	upstream Upstream
	clock    clock.Clock
	guard    Guard
}

// NewReconciler は Reconciler を作る。
func NewReconciler(pool *pgxpool.Pool, upstream Upstream, clk clock.Clock, guard Guard) *Reconciler {
	return &Reconciler{pool: pool, upstream: upstream, clock: clk, guard: guard}
}

// SyncAll は、サービスと番組の全量を突き合わせる。
// 上流から消えた未来の番組は削除する。上流から消えたサービスは、予約や録画から参照されうるので残す。
func (r *Reconciler) SyncAll(ctx context.Context) (*Result, error) {
	upstreamServices, err := r.upstream.Services(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch services: %w", err)
	}
	upstreamPrograms, err := r.upstream.Programs(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch programs: %w", err)
	}

	q := sqlc.New(r.pool)
	currentServices, err := q.ListServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}
	currentPrograms, err := q.ListPrograms(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list programs: %w", err)
	}

	services := diffServices(upstreamServices, currentServices)
	known := map[int64]bool{}
	for _, service := range upstreamServices {
		known[service.ID] = true
	}
	for _, service := range currentServices {
		known[service.ID] = true
	}
	upstreamPrograms, err = r.withOnAir(ctx, upstreamPrograms, known)
	if err != nil {
		return nil, err
	}
	programs, err := diffPrograms(filterPrograms(upstreamPrograms, known), currentPrograms)
	if err != nil {
		return nil, err
	}

	return r.apply(ctx, services, programs)
}

// SyncServices は、指定したサービスの番組だけを突き合わせる。イベントで変化が分かったサービスに使う。
// DB に無いサービスは飛ばす。サービス自体は、全量の突き合わせで足す。
func (r *Reconciler) SyncServices(ctx context.Context, serviceIDs []int64) (*Result, error) {
	q := sqlc.New(r.pool)
	upstreamPrograms := []*mirakurun.Program{}
	currentPrograms := []sqlc.ListProgramsRow{}
	synced := map[int64]bool{}
	// イベントをまとめると同じサービスが何度も出てくるので、1 回ずつにする。
	for _, serviceID := range slices.Compact(slices.Sorted(slices.Values(serviceIDs))) {
		exists, err := q.ServiceExists(ctx, serviceID)
		if err != nil {
			return nil, fmt.Errorf("failed to look up service %d: %w", serviceID, err)
		}
		if !exists {
			continue
		}
		synced[serviceID] = true

		programs, err := r.upstream.Programs(ctx, &mirakurun.ProgramsQuery{
			NetworkID: int(serviceID / serviceIDBase),
			ServiceID: int(serviceID % serviceIDBase),
			EventID:   0,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to fetch programs of service %d: %w", serviceID, err)
		}
		upstreamPrograms = append(upstreamPrograms, programs...)

		rows, err := q.ListProgramsOfService(ctx, serviceID)
		if err != nil {
			return nil, fmt.Errorf("failed to list programs of service %d: %w", serviceID, err)
		}
		for _, row := range rows {
			currentPrograms = append(currentPrograms, sqlc.ListProgramsRow(row))
		}
	}

	upstreamPrograms, err := r.withOnAir(ctx, upstreamPrograms, synced)
	if err != nil {
		return nil, err
	}
	programs, err := diffPrograms(upstreamPrograms, currentPrograms)
	if err != nil {
		return nil, err
	}

	return r.apply(ctx, &serviceDiff{upserts: []*mirakurun.Service{}}, programs)
}

// withOnAir は、上流の番組に、放送中と次の番組 (EIT[p/f]) の内容を重ねる (設計書 5.1)。
// mirakc の /api/programs は EIT[schedule] から作るので、延長や繰り上げは、番組表を取り直すまで反映されない。
// 重ねるのは、services に含まれるサービスの番組だけにする。番組表にない番組 (差し込まれた番組) は足す。
func (r *Reconciler) withOnAir(ctx context.Context, programs []*mirakurun.Program, services map[int64]bool) ([]*mirakurun.Program, error) {
	if len(services) == 0 {
		return programs, nil
	}
	onair, err := r.upstream.OnAirPrograms(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch on-air programs: %w", err)
	}
	if len(onair) == 0 {
		return programs, nil
	}

	key := func(p *mirakurun.Program) programKey {
		return programKey{serviceID: int64(p.NetworkID)*serviceIDBase + int64(p.ServiceID), eventID: int64(p.EventID)}
	}
	merged := slices.Clone(programs)
	index := map[programKey]int{}
	for i, p := range merged {
		index[key(p)] = i
	}
	for _, service := range onair {
		if !services[service.ServiceID] {
			continue
		}
		for _, p := range []*mirakurun.Program{service.Current, service.Next} {
			if p == nil {
				continue
			}
			if i, ok := index[key(p)]; ok {
				merged[i] = p
			} else {
				index[key(p)] = len(merged)
				merged = append(merged, p)
			}
		}
	}

	return merged, nil
}

// apply は差分を 1 つのトランザクションで書き、変わったものがあれば通知する。
func (r *Reconciler) apply(ctx context.Context, services *serviceDiff, programs *programDiff) (*Result, error) {
	result := &Result{
		ServicesUpserted: len(services.upserts),
		ProgramsUpserted: len(programs.upserts),
		ProgramsDeleted:  0,
		Version:          0,
	}
	now := r.clock.Now()

	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if err := r.guard(ctx, tx); err != nil {
			return err
		}
		q := sqlc.New(tx)
		if err := writeServices(ctx, q, services.upserts, now); err != nil {
			return err
		}
		if err := writePrograms(ctx, q, programs.upserts, now); err != nil {
			return err
		}
		deleted, err := deletePrograms(ctx, q, programs.deletes, now)
		if err != nil {
			return err
		}
		result.ProgramsDeleted = len(deleted)

		ranges := changedRanges(programs, deleted)
		if result.ServicesUpserted == 0 && result.ProgramsUpserted == 0 && result.ProgramsDeleted == 0 {
			return nil
		}

		result.Version, err = notify.Publish(ctx, tx, r.clock, NotifyChannel)
		if err != nil {
			return err
		}
		for _, serviceID := range slices.Sorted(maps.Keys(ranges)) {
			span := ranges[serviceID]
			err := q.InsertEPGChange(ctx, sqlc.InsertEPGChangeParams{
				Version:   result.Version,
				ServiceID: serviceID,
				StartAt:   span.start,
				EndAt:     span.end,
				CreatedAt: now,
			})
			if err != nil {
				return fmt.Errorf("failed to record an EPG change: %w", err)
			}
		}
		if _, err := q.PruneEPGChanges(ctx, now.Add(-changeRetention)); err != nil {
			return fmt.Errorf("failed to prune EPG changes: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to apply EPG changes: %w", err)
	}

	return result, nil
}

// serviceDiff は、書き込むサービス。
type serviceDiff struct {
	upserts []*mirakurun.Service
}

func diffServices(upstream []*mirakurun.Service, current []sqlc.ListServicesRow) *serviceDiff {
	byID := map[int64]sqlc.ListServicesRow{}
	for _, row := range current {
		byID[row.ID] = row
	}

	diff := &serviceDiff{upserts: []*mirakurun.Service{}}
	for _, service := range upstream {
		if service.Channel == nil {
			continue
		}
		row, ok := byID[service.ID]
		// 行はポインタの列を持つので、== ではなく値で比べる。
		if ok && reflect.DeepEqual(row, serviceRow(service)) {
			continue
		}
		diff.upserts = append(diff.upserts, service)
	}

	return diff
}

// serviceRow は、上流のサービスを DB の行と比べられる形にする。
func serviceRow(service *mirakurun.Service) sqlc.ListServicesRow {
	return sqlc.ListServicesRow{
		ID:                 service.ID,
		NetworkID:          int64(service.NetworkID),
		Sid:                int64(service.ServiceID),
		Name:               service.Name,
		ServiceType:        int64(service.Type),
		ChannelType:        service.Channel.Type,
		Channel:            service.Channel.Channel,
		RemoteControlKeyID: int64Ptr(service.RemoteControlKeyID),
		LogoID:             int64Ptr(service.LogoID),
	}
}

func writeServices(ctx context.Context, q *sqlc.Queries, services []*mirakurun.Service, now time.Time) error {
	for _, service := range services {
		channelType := service.Channel.Type
		defaults := defaultChannelType(channelType)
		err := q.EnsureChannelType(ctx, sqlc.EnsureChannelTypeParams{
			Type:        channelType,
			DisplayName: defaults.displayName,
			SortOrder:   defaults.sortOrder,
		})
		if err != nil {
			return fmt.Errorf("failed to add channel type %s: %w", channelType, err)
		}

		row := serviceRow(service)
		err = q.UpsertService(ctx, sqlc.UpsertServiceParams{
			ID:                 row.ID,
			NetworkID:          row.NetworkID,
			Sid:                row.Sid,
			Name:               row.Name,
			NameNormalized:     Normalize(row.Name),
			ServiceType:        row.ServiceType,
			ChannelType:        row.ChannelType,
			Channel:            row.Channel,
			RemoteControlKeyID: row.RemoteControlKeyID,
			LogoID:             row.LogoID,
			Now:                now,
		})
		if err != nil {
			return fmt.Errorf("failed to upsert service %d: %w", service.ID, err)
		}
	}

	return nil
}

// program は、番組を DB の行と比べられる形にしたもの。JSON の列は、読み解いた値で比べる。
type program struct {
	ServiceID    int64
	EventID      int64
	MirakurunID  int64
	StartAt      time.Time
	EndAt        time.Time
	EndUndecided bool
	IsFree       bool
	Name         string
	Description  string
	Extended     map[string]string
	Genres       []*mirakurun.Genre
	RelatedItems []*mirakurun.RelatedItem
}

// programKey は番組の主キー (サービスと eventId の組)。
type programKey struct {
	serviceID int64
	eventID   int64
}

func (p *program) key() programKey {
	return programKey{serviceID: p.ServiceID, eventID: p.EventID}
}

// fromUpstream は、上流の番組を program にする。
// 終了時刻が未定のとき、Mirakurun は長さを 1 ミリ秒に、Mahiron は 0 にして返す (#23 の実測)。
func fromUpstream(p *mirakurun.Program) *program {
	start := p.Start()
	extended := p.Extended
	if extended == nil {
		extended = map[string]string{}
	}
	genres := p.Genres
	if genres == nil {
		genres = []*mirakurun.Genre{}
	}
	related := p.RelatedItems
	if related == nil {
		related = []*mirakurun.RelatedItem{}
	}

	return &program{
		ServiceID:    int64(p.NetworkID)*serviceIDBase + int64(p.ServiceID),
		EventID:      int64(p.EventID),
		MirakurunID:  p.ID,
		StartAt:      start,
		EndAt:        start.Add(time.Duration(p.Duration) * time.Millisecond),
		EndUndecided: p.Duration <= 1,
		IsFree:       p.IsFree,
		Name:         p.Name,
		Description:  p.Description,
		Extended:     extended,
		Genres:       genres,
		RelatedItems: related,
	}
}

// fromRow は、DB の行を program にする。
func fromRow(row sqlc.ListProgramsRow) (*program, error) {
	p := &program{
		ServiceID:    row.ServiceID,
		EventID:      row.EventID,
		MirakurunID:  row.MirakurunID,
		StartAt:      row.StartAt.UTC(),
		EndAt:        row.EndAt.UTC(),
		EndUndecided: row.EndUndecided,
		IsFree:       row.IsFree,
		Name:         row.Name,
		Description:  row.Description,
		Extended:     map[string]string{},
		Genres:       []*mirakurun.Genre{},
		RelatedItems: []*mirakurun.RelatedItem{},
	}
	for _, column := range []struct {
		raw []byte
		v   any
	}{{row.Extended, &p.Extended}, {row.Genres, &p.Genres}, {row.RelatedItems, &p.RelatedItems}} {
		if err := json.Unmarshal(column.raw, column.v); err != nil {
			return nil, fmt.Errorf("failed to decode program %d/%d: %w", row.ServiceID, row.EventID, err)
		}
	}

	return p, nil
}

// programDiff は、書き込む番組と消す番組。
type programDiff struct {
	upserts []*program
	deletes []*program
	// previous は、upserts の番組の更新前の内容。追加した番組は含まない。変わった時間帯を求めるのに使う。
	previous map[programKey]*program
}

// diffPrograms は、上流の番組と DB の番組を突き合わせる。current の範囲に無い番組は消さない。
func diffPrograms(upstream []*mirakurun.Program, current []sqlc.ListProgramsRow) (*programDiff, error) {
	byKey := map[programKey]*program{}
	for _, row := range current {
		p, err := fromRow(row)
		if err != nil {
			return nil, err
		}
		byKey[p.key()] = p
	}

	diff := &programDiff{upserts: []*program{}, deletes: []*program{}, previous: map[programKey]*program{}}
	seen := map[programKey]bool{}
	for _, up := range upstream {
		p := fromUpstream(up)
		seen[p.key()] = true
		old, ok := byKey[p.key()]
		if ok && reflect.DeepEqual(old, p) {
			continue
		}
		diff.upserts = append(diff.upserts, p)
		if ok {
			diff.previous[p.key()] = old
		}
	}
	for _, key := range slices.SortedFunc(maps.Keys(byKey), compareKeys) {
		if !seen[key] {
			diff.deletes = append(diff.deletes, byKey[key])
		}
	}

	return diff, nil
}

func compareKeys(a, b programKey) int {
	return cmp.Or(cmp.Compare(a.serviceID, b.serviceID), cmp.Compare(a.eventID, b.eventID))
}

// filterPrograms は、DB にも上流にも無いサービスの番組を除く。Mirakurun は、サービスの一覧に無いサービスの番組も返すことがある。
func filterPrograms(programs []*mirakurun.Program, known map[int64]bool) []*mirakurun.Program {
	return slices.DeleteFunc(slices.Clone(programs), func(p *mirakurun.Program) bool {
		return !known[int64(p.NetworkID)*serviceIDBase+int64(p.ServiceID)]
	})
}

func writePrograms(ctx context.Context, q *sqlc.Queries, programs []*program, now time.Time) error {
	if len(programs) == 0 {
		return nil
	}

	params := make([]sqlc.UpsertProgramsParams, 0, len(programs))
	for _, p := range programs {
		extended, genres, related, err := encodeJSONColumns(p)
		if err != nil {
			return err
		}
		params = append(params, sqlc.UpsertProgramsParams{
			ServiceID:             p.ServiceID,
			EventID:               p.EventID,
			MirakurunID:           p.MirakurunID,
			StartAt:               p.StartAt,
			EndAt:                 p.EndAt,
			EndUndecided:          p.EndUndecided,
			IsFree:                p.IsFree,
			Name:                  p.Name,
			Description:           p.Description,
			NameNormalized:        Normalize(p.Name),
			DescriptionNormalized: Normalize(p.Description),
			Extended:              extended,
			Genres:                genres,
			RelatedItems:          related,
			Now:                   now,
		})
	}

	errs := []error{}
	q.UpsertPrograms(ctx, params).Exec(func(i int, err error) {
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to upsert program %d/%d: %w", params[i].ServiceID, params[i].EventID, err))
		}
	})

	return errors.Join(errs...)
}

// deletePrograms は、放送が始まっていない番組を消し、消した番組を返す。
func deletePrograms(ctx context.Context, q *sqlc.Queries, programs []*program, now time.Time) ([]*program, error) {
	deleted := []*program{}
	if len(programs) == 0 {
		return deleted, nil
	}

	params := make([]sqlc.DeleteProgramsParams, 0, len(programs))
	for _, p := range programs {
		params = append(params, sqlc.DeleteProgramsParams{ServiceID: p.ServiceID, EventID: p.EventID, Now: now})
	}
	errs := []error{}
	q.DeletePrograms(ctx, params).Exec(func(i int, err error) {
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to delete program %d/%d: %w", params[i].ServiceID, params[i].EventID, err))
		}
	})
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	for _, p := range programs {
		if p.StartAt.After(now) {
			deleted = append(deleted, p)
		}
	}

	return deleted, nil
}

func encodeJSONColumns(p *program) ([]byte, []byte, []byte, error) {
	extended, err := json.Marshal(p.Extended)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to encode extended: %w", err)
	}
	genres, err := json.Marshal(p.Genres)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to encode genres: %w", err)
	}
	related, err := json.Marshal(p.RelatedItems)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to encode related items: %w", err)
	}

	return extended, genres, related, nil
}

// span は時間帯。
type span struct {
	start time.Time
	end   time.Time
}

func (s *span) include(start, end time.Time) {
	if start.Before(s.start) {
		s.start = start
	}
	if end.After(s.end) {
		s.end = end
	}
}

// changedRanges は、サービスごとに、変更のあった時間帯をまとめる。更新では、更新の前と後の両方の時間帯を含める。
func changedRanges(diff *programDiff, deleted []*program) map[int64]*span {
	ranges := map[int64]*span{}
	add := func(serviceID int64, start, end time.Time) {
		if s, ok := ranges[serviceID]; ok {
			s.include(start, end)

			return
		}
		ranges[serviceID] = &span{start: start, end: end}
	}
	for _, p := range diff.upserts {
		add(p.ServiceID, p.StartAt, p.EndAt)
		if old, ok := diff.previous[p.key()]; ok {
			add(old.ServiceID, old.StartAt, old.EndAt)
		}
	}
	for _, p := range deleted {
		add(p.ServiceID, p.StartAt, p.EndAt)
	}

	return ranges
}

func int64Ptr(v *int) *int64 {
	if v == nil {
		return nil
	}

	return new(int64(*v))
}

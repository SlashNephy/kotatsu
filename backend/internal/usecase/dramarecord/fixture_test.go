package dramarecord_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/lib/netactivity"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
	"github.com/SlashNephy/kotatsu/internal/testutil/simkltest"
	"github.com/SlashNephy/kotatsu/internal/usecase/animerecord"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/dramalibrary"
	"github.com/SlashNephy/kotatsu/internal/usecase/dramarecord"
	"github.com/SlashNephy/kotatsu/internal/usecase/epg"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

const (
	grNetworkID = 0x7FE0
	grServiceID = 0x0400
	// series は、番組に結びつける TMDB の作品の ID。
	series       int32 = 9001
	origin             = "http://kotatsu.test"
	clientID           = "simkl-client"
	clientSecret       = "simkl-secret"
)

func testNow() time.Time {
	return time.Date(2026, 10, 10, 12, 10, 0, 0, time.UTC)
}

func at(hour, minute int) time.Time {
	return time.Date(2026, 10, 10, hour, minute, 0, 0, time.UTC)
}

func serviceID() int64 {
	return faketuner.ServiceItemID(grNetworkID, grServiceID)
}

// simklItem は、偽の Simkl のウォッチリストの作品。
type simklItem = simkltest.Item

// fakeInserter は、入れたジョブを覚える。
type fakeInserter struct {
	mu         sync.Mutex
	records    []dramarecord.RecordArgs
	watchlists []dramarecord.WatchlistArgs
}

func (f *fakeInserter) InsertManyTx(_ context.Context, _ pgx.Tx, params []river.InsertManyParams) ([]*rivertype.JobInsertResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, param := range params {
		switch args := param.Args.(type) {
		case dramarecord.RecordArgs:
			f.records = append(f.records, args)
		case dramarecord.WatchlistArgs:
			f.watchlists = append(f.watchlists, args)
		default:
			return nil, errors.New("unexpected job")
		}
	}

	return []*rivertype.JobInsertResult{}, nil
}

func (f *fakeInserter) takeRecords() []dramarecord.RecordArgs {
	f.mu.Lock()
	defer f.mu.Unlock()
	taken := f.records
	f.records = nil

	return taken
}

type fixture struct {
	pool     *pgxpool.Pool
	clock    *clocktest.Fake
	simkl    *simkltest.Server
	service  *dramarecord.Service
	inserter *fakeInserter
	client   *auth.ClientInfo
	alice    *auth.Principal
	bob      *auth.Principal
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	clk := clocktest.NewFake(testNow())
	tuner := httptest.NewServer(faketuner.New(faketuner.DefaultOptions(testNow(), nil)))
	t.Cleanup(tuner.Close)
	client, err := mirakurun.New(&mirakurun.Options{URL: tuner.URL})
	require.NoError(t, err)
	_, err = epg.NewReconciler(pool, client, clk, func(context.Context, pgx.Tx) error { return nil }).SyncAll(t.Context())
	require.NoError(t, err)

	fake := simkltest.New(clientID, clientSecret, origin+"/internal/auth/simkl/callback")
	t.Cleanup(fake.Close)
	fake.SetNow(clk.Now)
	f := &fixture{
		pool: pool, clock: clk, simkl: fake, inserter: new(fakeInserter),
		client: &auth.ClientInfo{UserAgent: "kotatsu-test/1.0", SourceIP: netip.MustParseAddr("192.168.1.10"), Host: "kotatsu.test", Secure: false},
	}
	f.service, err = dramarecord.NewService(&dramarecord.Options{
		Clock: clk, Recorder: audit.NewRecorder(clk), AppSecret: strings.Repeat("k", 32),
		Destinations: []dramarecord.Destination{dramarecord.NewSimkl(dramarecord.NewHTTPClient(nil, netactivity.PurposeDramaRecord), fake.URL())},
		Logger:       slog.New(slog.DiscardHandler), Inserter: f.inserter, RequestInterval: 0,
	})
	require.NoError(t, err)
	f.alice = f.login(t, "alice", domain.RoleViewer)
	f.bob = f.login(t, "bob", domain.RoleMember)
	_, err = settings.NewStore(clk, audit.NewRecorder(clk)).Update(t.Context(), pool, audit.User(f.bob.UserID), &settings.Changes{DramaLinkingEnabled: new(true)})
	require.NoError(t, err)
	require.NoError(t, f.service.UpdateApp(t.Context(), pool, audit.User(f.bob.UserID), dramarecord.DestinationSimkl,
		&dramarecord.AppChanges{ClientID: clientID, ClientSecret: new(clientSecret)}))

	return f
}

func (f *fixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()

	_, err := f.pool.Exec(t.Context(), sql, args...)
	require.NoError(t, err)
}

func (f *fixture) login(t *testing.T, name string, role domain.Role) *auth.Principal {
	t.Helper()
	user, err := auth.NewUsers(f.clock).Create(t.Context(), f.pool, &auth.NewUser{Username: name, DisplayName: name, Role: role, PasswordHash: nil})
	require.NoError(t, err)
	_, session, err := auth.NewSessions(&auth.SessionsOptions{Clock: f.clock}).Create(t.Context(), f.pool, user.ID, f.client)
	require.NoError(t, err)

	return &auth.Principal{UserID: user.ID, Role: role, SessionID: session.ID, AuthenticatedAt: session.CreatedAt, OAuthClientID: ""}
}

// link は、hour 時 minute 分からの番組を、作品 seriesID のシーズン 1 の話 episode に結びつける。
func (f *fixture) link(t *testing.T, hour, minute int, seriesID, episode int32) {
	t.Helper()
	f.exec(t, `INSERT INTO program_drama_matches (service_id, start_at, event_id, title_key, tmdb_series_id, season_number, episode_number,
		series_source, episode_source, matched_at)
		SELECT service_id, start_at, event_id, 'ドラマ', $3, 1, $4, 'search', 'air_date', now() FROM programs WHERE service_id = $1 AND start_at = $2`,
		serviceID(), at(hour, minute), seriesID, episode)
}

// authorize は、principal の連携を始め、偽の Simkl の認可のエンドポイントが戻すリダイレクト先の問い合わせを返す。
func (f *fixture) authorize(t *testing.T, principal *auth.Principal) url.Values {
	t.Helper()
	authorizationURL, err := f.service.Begin(t.Context(), f.pool, principal, dramarecord.DestinationSimkl, origin)
	require.NoError(t, err)
	parsed, err := url.Parse(authorizationURL)
	require.NoError(t, err)
	assert.Equal(t, origin+"/internal/auth/simkl/callback", parsed.Query().Get("redirect_uri"))
	assert.Equal(t, "media:read media:write", parsed.Query().Get("scope"))
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, authorizationURL, nil)
	require.NoError(t, err)
	res, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())
	require.Equal(t, http.StatusFound, res.StatusCode)
	location, err := url.Parse(res.Header.Get("Location"))
	require.NoError(t, err)

	return location.Query()
}

func callbackOf(query url.Values) *dramarecord.Callback {
	return &dramarecord.Callback{State: query.Get("state"), Code: query.Get("code"), Error: query.Get("error"), Issuer: query.Get("iss")}
}

// connect は、principal と Simkl を連携する。
func (f *fixture) connect(t *testing.T, principal *auth.Principal) {
	t.Helper()
	query := f.authorize(t, principal)
	require.NoError(t, f.service.Complete(t.Context(), f.pool, principal, dramarecord.DestinationSimkl, callbackOf(query), f.client))
}

// watch は、principal が 12 時 minute 分から to まで、12 時台の番組をライブ視聴したことを報告する。
func (f *fixture) watch(t *testing.T, principal *auth.Principal, minute int, to time.Time) {
	t.Helper()
	target := &animerecord.Target{ServiceID: new(serviceID()), RecordingID: nil}
	ok, err := dramalibrary.Report(t.Context(), f.pool, f.inserter, principal.UserID, target,
		[]*animerecord.Interval{{From: at(12, minute), To: to}}, f.clock.Now())
	require.NoError(t, err)
	assert.True(t, ok)
}

// records は、ユーザー userID の記録を「シーズン:話:状態」の形で返す。
func (f *fixture) records(t *testing.T, userID uuid.UUID) []string {
	t.Helper()
	rows, err := f.pool.Query(t.Context(), `SELECT season_number || ':' || episode_number || ':' || state FROM drama_records
		WHERE user_id = $1 ORDER BY season_number, episode_number`, userID)
	require.NoError(t, err)
	records, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)

	return records
}

// notifications は、届いた通知の種類と、理由を返す。
func (f *fixture) notifications(t *testing.T) []string {
	t.Helper()
	rows, err := f.pool.Query(t.Context(), `SELECT kind || ':' || (payload->>'episodes') || ':' || coalesce(payload->>'reason', '')
		FROM notification_events ORDER BY occurred_at, kind`)
	require.NoError(t, err)
	kinds, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)

	return kinds
}

// entries は、ユーザー userID の見ている作品を「作品:状態」の形で返す。
func (f *fixture) entries(t *testing.T, userID uuid.UUID) []string {
	t.Helper()
	rows, err := f.pool.Query(t.Context(), `SELECT tmdb_series_id || ':' || status FROM drama_watchlist_entries
		WHERE user_id = $1 ORDER BY tmdb_series_id`, userID)
	require.NoError(t, err)
	entries, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)

	return entries
}

// runRecords は、入れた記録のジョブを、すべて 1 回ずつ実行する。
func (f *fixture) runRecords(t *testing.T, last bool) {
	t.Helper()
	for _, args := range f.inserter.takeRecords() {
		require.NoError(t, f.service.Record(t.Context(), f.pool, &args, last))
	}
}

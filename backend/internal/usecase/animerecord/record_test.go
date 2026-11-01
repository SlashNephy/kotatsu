package animerecord_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
	"github.com/SlashNephy/kotatsu/internal/usecase/animerecord"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/epg"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

const (
	grNetworkID = 0x7FE0
	grServiceID = 0x0400
	// testTID は、番組に結びつける作品のしょぼいカレンダーの TID。
	testTID = 6776
	// annictWorkID と aniListID は、testTID に対応する作品の ID。
	annictWorkID = 10
	aniListID    = 600
	origin       = "http://kotatsu.test"
	// testCode は、偽の記録先が受け付ける認可コード。
	testCode = "code"
	// annictThirdPending は、Annict に第 3 話を送る前の記録。
	annictThirdPending = "annict:3:pending"
	// annictThirdRecorded と annictFourthRecorded は、Annict に第 3 話と第 4 話を送れた記録。
	annictThirdRecorded  = "annict:3:recorded"
	annictFourthRecorded = "annict:4:recorded"
)

// testNow は、偽チューナーの EPG を取り込む時刻。番組は 11:00 から 30 分ごとで、12:00 から 12:30 の番組に話を結びつける。
func testNow() time.Time {
	return time.Date(2026, 10, 10, 12, 10, 0, 0, time.UTC)
}

func at(hour, minute int) time.Time {
	return time.Date(2026, 10, 10, hour, minute, 0, 0, time.UTC)
}

func serviceID() int64 {
	return faketuner.ServiceItemID(grNetworkID, grServiceID)
}

// fakeInserter は、入れたジョブを覚える。
type fakeInserter struct {
	mu   sync.Mutex
	jobs []animerecord.RecordArgs
	// wishlists は、入れた、見たいリストを取り込むジョブ (#928)。
	wishlists []animerecord.WishlistArgs
}

func (f *fakeInserter) InsertManyTx(_ context.Context, _ pgx.Tx, params []river.InsertManyParams) ([]*rivertype.JobInsertResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, param := range params {
		switch args := param.Args.(type) {
		case animerecord.RecordArgs:
			f.jobs = append(f.jobs, args)
		case animerecord.WishlistArgs:
			f.wishlists = append(f.wishlists, args)
		default:
			return nil, errors.New("unexpected job")
		}
	}

	return []*rivertype.JobInsertResult{}, nil
}

func (f *fakeInserter) taken() []animerecord.RecordArgs {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]animerecord.RecordArgs{}, f.jobs...)
}

type recordFixture struct {
	pool     *pgxpool.Pool
	clock    *clocktest.Fake
	service  *animerecord.Service
	annict   *fakeService
	anilist  *fakeService
	inserter *fakeInserter
	client   *auth.ClientInfo
	// alice は視聴者。視聴者も、自分が見たアニメを記録できる (設計書 9.7)。
	alice *auth.Principal
	bob   *auth.Principal
}

func newRecordFixture(t *testing.T) *recordFixture {
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

	f := &recordFixture{
		pool: pool, clock: clk, annict: newFakeService(t), anilist: newFakeService(t), inserter: new(fakeInserter),
		client: &auth.ClientInfo{UserAgent: "kotatsu-test/1.0", SourceIP: netip.MustParseAddr("192.168.1.10"), Host: "kotatsu.test", Secure: false},
	}
	httpClient := animerecord.NewHTTPClient()
	f.service, err = animerecord.NewService(&animerecord.Options{
		Clock: clk, Recorder: audit.NewRecorder(clk), AppSecret: strings.Repeat("k", 32),
		Destinations: []animerecord.Destination{
			animerecord.NewAnnict(httpClient, f.annict.server.URL), animerecord.NewAniList(httpClient, f.anilist.server.URL),
		},
		Logger: slog.New(slog.DiscardHandler), Inserter: f.inserter, WishlistInterval: 0,
	})
	require.NoError(t, err)
	f.alice = f.login(t, "alice", auth.RoleViewer)
	f.bob = f.login(t, "bob", auth.RoleMember)

	actor := audit.User(f.bob.UserID)
	_, err = settings.NewStore(clk, audit.NewRecorder(clk)).Update(t.Context(), pool, actor, &settings.Changes{
		Recording: nil, ArchiveRetentionDays: nil, NotificationRetentionDays: nil, Rule: nil, MCPEnabled: nil, DataBroadcastIPEnabled: nil,
		AnimeLinkingEnabled: new(true), DRCSReplacements: nil,
	})
	require.NoError(t, err)
	for _, name := range []string{animerecord.DestinationAnnict, animerecord.DestinationAniList} {
		require.NoError(t, f.service.UpdateApp(t.Context(), pool, actor, name, &animerecord.AppChanges{ClientID: "client", ClientSecret: new("secret")}))
	}

	// 12:00 からの番組に、第 3 話と第 4 話 (2 話連続の放送) を結びつける。
	for _, statement := range []string{
		`INSERT INTO program_anime_matches (service_id, start_at, event_id, matched_at) SELECT service_id, start_at, event_id, now() FROM programs WHERE service_id = $1 AND start_at = $2`,
		`INSERT INTO program_anime_episodes (service_id, start_at, position, source, syobocal_tid, count_start, count_end, subtitle, is_first, is_last)
			VALUES ($1, $2, 0, 'syobocal', 6776, 3, 4, NULL, false, false)`,
	} {
		_, err := pool.Exec(t.Context(), statement, serviceID(), at(12, 0))
		require.NoError(t, err)
	}
	_, err = pool.Exec(t.Context(), `INSERT INTO anime_works (source, syobocal_tid, position, annict_work_id, anilist_id, mal_id) VALUES ('arm', $1, 0, $2, $3, NULL)`,
		testTID, annictWorkID, aniListID)
	require.NoError(t, err)
	f.annict.works[annictWorkID] = &fakeWork{AnnictID: annictWorkID, Episodes: []fakeEpisode{
		{ID: "E3", AnnictID: 103, Number: new(3)}, {ID: "E4", AnnictID: 104, Number: new(4)},
	}}
	f.anilist.media[aniListID] = new(12)

	return f
}

func (f *recordFixture) login(t *testing.T, name string, role auth.Role) *auth.Principal {
	t.Helper()
	user, err := auth.NewUsers(f.clock).Create(t.Context(), f.pool, &auth.NewUser{Username: name, DisplayName: name, Role: role, PasswordHash: nil})
	require.NoError(t, err)
	_, session, err := auth.NewSessions(&auth.SessionsOptions{Clock: f.clock}).Create(t.Context(), f.pool, user.ID, f.client)
	require.NoError(t, err)

	return &auth.Principal{UserID: user.ID, Role: role, SessionID: session.ID, AuthenticatedAt: session.CreatedAt, OAuthClientID: ""}
}

// begin は、principal と記録先 name の連携を始め、記録先に送り出す URL の state を返す。
// 偽の記録先は、認可コード code を、トークン token と引き換える。
func (f *recordFixture) begin(t *testing.T, principal *auth.Principal, name, code, token string) string {
	t.Helper()
	fake := f.annict
	if name == animerecord.DestinationAniList {
		fake = f.anilist
	}
	fake.mu.Lock()
	fake.codes[code] = token
	fake.mu.Unlock()
	authorizationURL, err := f.service.Begin(t.Context(), f.pool, principal, name, origin)
	require.NoError(t, err)
	parsed, err := url.Parse(authorizationURL)
	require.NoError(t, err)
	query := parsed.Query()
	assert.Equal(t, origin+"/auth/anime-records/"+name+"/callback", query.Get("redirect_uri"))
	assert.Equal(t, "S256", query.Get("code_challenge_method"), "PKCE is used")
	assert.NotEmpty(t, query.Get("code_challenge"))

	return query.Get("state")
}

// connect は、principal と記録先 name を連携する。
func (f *recordFixture) connect(t *testing.T, principal *auth.Principal, name string) {
	t.Helper()
	token := "token-" + name + "-" + principal.UserID.String()
	state := f.begin(t, principal, name, "code-"+token, token)
	require.NoError(t, f.service.Complete(t.Context(), f.pool, principal, name, &animerecord.Callback{State: state, Code: "code-" + token, Error: ""}, f.client))
}

// TestConnect は、記録先から戻ってきた認可を、始めたセッションと記録先のまま戻ってきたときだけ受け付け、
// トークンを暗号化して保存することを確かめる (#927)。ほかのセッションやユーザーで始めた認可を使わせないため (CSRF とアカウントの取り違え)。
func TestConnect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// complete は、戻ってきたリクエストを受け付ける。state は、alice が Annict と始めた認可のもの。
		complete func(t *testing.T, f *recordFixture, state string) error
		wantErr  error
	}{
		{
			name: "the session that began it",
			complete: func(t *testing.T, f *recordFixture, state string) error {
				t.Helper()

				return f.service.Complete(t.Context(), f.pool, f.alice, animerecord.DestinationAnnict, &animerecord.Callback{State: state, Code: testCode, Error: ""}, f.client)
			},
			wantErr: nil,
		},
		{
			name: "another user's session",
			complete: func(t *testing.T, f *recordFixture, state string) error {
				t.Helper()

				return f.service.Complete(t.Context(), f.pool, f.bob, animerecord.DestinationAnnict, &animerecord.Callback{State: state, Code: testCode, Error: ""}, f.client)
			},
			wantErr: animerecord.ErrAuthorizationMismatch,
		},
		{
			name: "another session of the same user",
			complete: func(t *testing.T, f *recordFixture, state string) error {
				t.Helper()
				_, other, err := auth.NewSessions(&auth.SessionsOptions{Clock: f.clock}).Create(t.Context(), f.pool, f.alice.UserID, f.client)
				require.NoError(t, err)
				principal := *f.alice
				principal.SessionID = other.ID

				return f.service.Complete(t.Context(), f.pool, &principal, animerecord.DestinationAnnict, &animerecord.Callback{State: state, Code: testCode, Error: ""}, f.client)
			},
			wantErr: animerecord.ErrAuthorizationMismatch,
		},
		{
			name: "no session",
			complete: func(t *testing.T, f *recordFixture, state string) error {
				t.Helper()

				return f.service.Complete(t.Context(), f.pool, nil, animerecord.DestinationAnnict, &animerecord.Callback{State: state, Code: testCode, Error: ""}, f.client)
			},
			wantErr: animerecord.ErrAuthorizationMismatch,
		},
		{
			name: "another destination's callback",
			complete: func(t *testing.T, f *recordFixture, state string) error {
				t.Helper()

				return f.service.Complete(t.Context(), f.pool, f.alice, animerecord.DestinationAniList, &animerecord.Callback{State: state, Code: testCode, Error: ""}, f.client)
			},
			wantErr: animerecord.ErrAuthorizationMismatch,
		},
		{
			name: "an unknown state",
			complete: func(t *testing.T, f *recordFixture, _ string) error {
				t.Helper()

				return f.service.Complete(t.Context(), f.pool, f.alice, animerecord.DestinationAnnict, &animerecord.Callback{State: "forged", Code: testCode, Error: ""}, f.client)
			},
			wantErr: animerecord.ErrAuthorizationExpired,
		},
		{
			name: "the same state twice",
			complete: func(t *testing.T, f *recordFixture, state string) error {
				t.Helper()
				callback := &animerecord.Callback{State: state, Code: testCode, Error: ""}
				require.NoError(t, f.service.Complete(t.Context(), f.pool, f.alice, animerecord.DestinationAnnict, callback, f.client))
				_, err := f.pool.Exec(t.Context(), `DELETE FROM anime_record_connections`)
				require.NoError(t, err)

				return f.service.Complete(t.Context(), f.pool, f.alice, animerecord.DestinationAnnict, callback, f.client)
			},
			wantErr: animerecord.ErrAuthorizationExpired,
		},
		{
			name: "after the authorization expires",
			complete: func(t *testing.T, f *recordFixture, state string) error {
				t.Helper()
				f.clock.Advance(11 * time.Minute)

				return f.service.Complete(t.Context(), f.pool, f.alice, animerecord.DestinationAnnict, &animerecord.Callback{State: state, Code: testCode, Error: ""}, f.client)
			},
			wantErr: animerecord.ErrAuthorizationExpired,
		},
		{
			name: "denied by the user",
			complete: func(t *testing.T, f *recordFixture, state string) error {
				t.Helper()

				return f.service.Complete(t.Context(), f.pool, f.alice, animerecord.DestinationAnnict, &animerecord.Callback{State: state, Code: "", Error: "access_denied"}, f.client)
			},
			wantErr: animerecord.ErrAccessDenied,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newRecordFixture(t)
			state := f.begin(t, f.alice, animerecord.DestinationAnnict, testCode, "secret-token-value")

			err := tt.complete(t, f, state)
			connections, listErr := f.service.Connections(t.Context(), f.pool, f.alice.UserID)
			require.NoError(t, listErr)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Empty(t, connections)

				return
			}
			require.NoError(t, err)
			require.Len(t, connections, 1)
			assert.Equal(t, "annict-user", connections[0].AccountName)
			var stored []byte
			require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT access_token_encrypted FROM anime_record_connections WHERE user_id = $1`, f.alice.UserID).Scan(&stored))
			assert.NotContains(t, string(stored), "secret-token-value", "the token is encrypted")
			var summary string
			require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT summary::text FROM audit_events WHERE action = 'anime_record.connect'`).Scan(&summary))
			assert.NotContains(t, summary, "secret-token-value", "the audit log has no token")
			others, err := f.service.Connections(t.Context(), f.pool, f.bob.UserID)
			require.NoError(t, err)
			assert.Empty(t, others, "other users do not see the connection")
		})
	}
}

// TestBeginRequiresSessionAndApp は、ブラウザーのセッションでないと連携を始められず、連携が無効なら始められないことを確かめる。
func TestBeginRequiresSessionAndApp(t *testing.T) {
	t.Parallel()
	f := newRecordFixture(t)

	token := &auth.Principal{UserID: f.alice.UserID, Role: auth.RoleViewer, SessionID: uuid.Nil, AuthenticatedAt: time.Time{}, OAuthClientID: "mcp"}
	_, err := f.service.Begin(t.Context(), f.pool, token, animerecord.DestinationAnnict, origin)
	require.ErrorIs(t, err, animerecord.ErrSessionRequired)

	require.NoError(t, f.service.UpdateApp(t.Context(), f.pool, audit.User(f.bob.UserID), animerecord.DestinationAniList, &animerecord.AppChanges{ClientID: "", ClientSecret: nil}))
	_, err = f.service.Begin(t.Context(), f.pool, f.alice, animerecord.DestinationAniList, origin)
	require.ErrorIs(t, err, animerecord.ErrUnavailable, "the app is not registered")

	_, err = settings.NewStore(f.clock, audit.NewRecorder(f.clock)).Update(t.Context(), f.pool, audit.User(f.bob.UserID), &settings.Changes{
		Recording: nil, ArchiveRetentionDays: nil, NotificationRetentionDays: nil, Rule: nil, MCPEnabled: nil, DataBroadcastIPEnabled: nil,
		AnimeLinkingEnabled: new(false), DRCSReplacements: nil,
	})
	require.NoError(t, err)
	_, err = f.service.Begin(t.Context(), f.pool, f.alice, animerecord.DestinationAnnict, origin)
	require.ErrorIs(t, err, animerecord.ErrUnavailable, "the linking is disabled")
}

// interval は、h1:m1 から h2:m2 までの区間を返す。
func interval(h1, m1, h2, m2 int) *animerecord.Interval {
	return &animerecord.Interval{From: at(h1, m1), To: at(h2, m2)}
}

// pendingRecords は、ユーザーの記録を、記録先と話数の組にして返す。
func pendingRecords(t *testing.T, f *recordFixture, userID uuid.UUID) []string {
	t.Helper()
	rows, err := f.pool.Query(t.Context(), `SELECT destination || ':' || episode_number::text || ':' || state FROM anime_records WHERE user_id = $1 ORDER BY 1`, userID)
	require.NoError(t, err)
	records, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)

	return records
}

// TestReport は、プレイヤーが実際に再生した区間を足し、番組の長さの 20% (30 分の番組では 6 分) を見たら記録を作ることを確かめる (#927)。
// 同じ区間の送り直しや、複数のタブで重なった区間は多く数えず、見直しても記録もジョブも増えない。
func TestReport(t *testing.T) {
	t.Parallel()
	f := newRecordFixture(t)
	f.connect(t, f.alice, animerecord.DestinationAnnict)
	f.connect(t, f.alice, animerecord.DestinationAniList)
	live := &animerecord.Target{ServiceID: new(serviceID()), RecordingID: nil}

	// 5 分を、2 つのタブから重ねて、送り直しも含めて送る。重なった分は 1 回だけ数えるので、5 分のまま。
	for _, intervals := range [][]*animerecord.Interval{
		{interval(12, 0, 12, 3)},
		{interval(12, 2, 12, 5)},
		{interval(12, 0, 12, 3), interval(12, 2, 12, 5)},
	} {
		tracked, err := f.service.Report(t.Context(), f.pool, f.inserter, f.alice.UserID, live, intervals)
		require.NoError(t, err)
		assert.True(t, tracked)
	}
	assert.Empty(t, pendingRecords(t, f, f.alice.UserID), "5 minutes of a 30-minute program is below 20%")
	assert.Empty(t, f.inserter.taken())

	// 前の番組 (11:30 から) の区間は、この番組には数えない。
	_, err := f.service.Report(t.Context(), f.pool, f.inserter, f.alice.UserID, live, []*animerecord.Interval{interval(11, 50, 12, 0)})
	require.NoError(t, err)
	assert.Empty(t, pendingRecords(t, f, f.alice.UserID), "time outside the program does not count")

	// 1 分を足して 6 分になったら、2 話連続の放送の両方の話を、記録先ごとに記録する。
	_, err = f.service.Report(t.Context(), f.pool, f.inserter, f.alice.UserID, live, []*animerecord.Interval{interval(12, 5, 12, 6)})
	require.NoError(t, err)
	assert.Equal(t, []string{"anilist:3:pending", "anilist:4:pending", annictThirdPending, "annict:4:pending"}, pendingRecords(t, f, f.alice.UserID))
	jobs := f.inserter.taken()
	require.Len(t, jobs, 2)
	assert.ElementsMatch(t, []string{animerecord.DestinationAnnict, animerecord.DestinationAniList}, []string{jobs[0].Destination, jobs[1].Destination})

	// 見続けても、見直しても、同じ番組と話と記録先は 1 回だけ記録する。
	f.clock.Advance(30 * time.Minute)
	for _, intervals := range [][]*animerecord.Interval{{interval(12, 6, 12, 20)}, {interval(12, 0, 12, 30)}} {
		_, err := f.service.Report(t.Context(), f.pool, f.inserter, f.alice.UserID, live, intervals)
		require.NoError(t, err)
	}
	assert.Len(t, pendingRecords(t, f, f.alice.UserID), 4)
	assert.Len(t, f.inserter.taken(), 2, "no more jobs after the threshold")

	// bob は連携していないので、何も記録しない。
	tracked, err := f.service.Report(t.Context(), f.pool, f.inserter, f.bob.UserID, live, []*animerecord.Interval{interval(12, 0, 12, 30)})
	require.NoError(t, err)
	assert.False(t, tracked)
	assert.Empty(t, pendingRecords(t, f, f.bob.UserID))

	// 壊れた区間は受け付けない。
	_, err = f.service.Report(t.Context(), f.pool, f.inserter, f.alice.UserID, live, []*animerecord.Interval{interval(12, 6, 12, 5)})
	require.ErrorIs(t, err, animerecord.ErrInvalidReport)
	// 時計の進んだ端末から届いた、今より先の区間は、断らずに切り捨てる。断ると、端末が送り直し続ける。
	var before int64
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT played_ms FROM anime_watch_progress WHERE user_id = $1`, f.alice.UserID).Scan(&before))
	_, err = f.service.Report(t.Context(), f.pool, f.inserter, f.alice.UserID, live, []*animerecord.Interval{interval(13, 0, 14, 0)})
	require.NoError(t, err)
	var after int64
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT played_ms FROM anime_watch_progress WHERE user_id = $1`, f.alice.UserID).Scan(&after))
	assert.Equal(t, before, after, "time after now does not count")
}

// TestReportEnqueuesRiverJob は、割合に届いたら、本物の River のクライアントで記録のジョブを入れ、
// 報告を重ねても、終わるまでは同じ番組と記録先のジョブを 1 つにすることを確かめる。
func TestReportEnqueuesRiverJob(t *testing.T) {
	t.Parallel()
	f := newRecordFixture(t)
	f.connect(t, f.alice, animerecord.DestinationAnnict)
	require.NoError(t, jobs.Migrate(t.Context(), slog.New(slog.DiscardHandler), f.pool))
	client, err := jobs.NewClient(f.pool, slog.New(slog.DiscardHandler), &jobs.Options{PollOnly: false, Workers: nil, PeriodicJobs: nil})
	require.NoError(t, err)
	live := &animerecord.Target{ServiceID: new(serviceID()), RecordingID: nil}

	_, err = f.service.Report(t.Context(), f.pool, client, f.alice.UserID, live, []*animerecord.Interval{interval(12, 0, 12, 10)})
	require.NoError(t, err)
	// 記録をやり直させても (行を消して作り直させても)、待っているジョブは重ねない。
	_, err = f.pool.Exec(t.Context(), `DELETE FROM anime_records`)
	require.NoError(t, err)
	_, err = f.service.Report(t.Context(), f.pool, client, f.alice.UserID, live, []*animerecord.Interval{interval(12, 0, 12, 10)})
	require.NoError(t, err)
	var count int
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT count(*) FROM river_job WHERE kind = 'anime_record'`).Scan(&count))
	assert.Equal(t, 1, count)
}

// TestReportWithoutInserter は、ジョブを入れられなければ、記録の行も残さず、次の報告で作り直せることを確かめる。
func TestReportWithoutInserter(t *testing.T) {
	t.Parallel()
	f := newRecordFixture(t)
	f.connect(t, f.alice, animerecord.DestinationAnnict)
	live := &animerecord.Target{ServiceID: new(serviceID()), RecordingID: nil}

	_, err := f.service.Report(t.Context(), f.pool, nil, f.alice.UserID, live, []*animerecord.Interval{interval(12, 0, 12, 10)})
	require.Error(t, err)
	assert.Empty(t, pendingRecords(t, f, f.alice.UserID), "no orphan records")
	_, err = f.service.Report(t.Context(), f.pool, f.inserter, f.alice.UserID, live, []*animerecord.Interval{interval(12, 0, 12, 10)})
	require.NoError(t, err)
	assert.Len(t, f.inserter.taken(), 1)
}

// TestReportRecording は、録画の再生でも、録画した番組の結びつきで記録することを確かめる。ライブ視聴で見た区間と合わせて数える。
func TestReportRecording(t *testing.T) {
	t.Parallel()
	f := newRecordFixture(t)
	f.connect(t, f.alice, animerecord.DestinationAnnict)
	recordingID := uuid.MustParse("0192f000-0000-7000-8000-000000000001")
	for _, statement := range []string{
		`INSERT INTO recordings (id, service_id, name, description, start_at, end_at, state, recorded_from, created_at, updated_at)
			VALUES ($1, $2, 'テスト', '', $3, $4, 'completed', $3, now(), now())`,
		`INSERT INTO recording_anime_matches (recording_id, matched_at) VALUES ($1, now())`,
		`INSERT INTO recording_anime_episodes (recording_id, position, source, syobocal_tid, count_start, count_end, is_first, is_last)
			VALUES ($1, 0, 'syobocal', 6776, 3, 3, false, false)`,
	} {
		args := []any{recordingID}
		if strings.Contains(statement, "INSERT INTO recordings ") {
			args = []any{recordingID, serviceID(), at(12, 0), at(12, 30)}
		}
		_, err := f.pool.Exec(t.Context(), statement, args...)
		require.NoError(t, err)
	}

	_, err := f.service.Report(t.Context(), f.pool, f.inserter, f.alice.UserID, &animerecord.Target{ServiceID: new(serviceID()), RecordingID: nil},
		[]*animerecord.Interval{interval(12, 0, 12, 3)})
	require.NoError(t, err)
	_, err = f.service.Report(t.Context(), f.pool, f.inserter, f.alice.UserID, &animerecord.Target{ServiceID: nil, RecordingID: new(recordingID)},
		[]*animerecord.Interval{interval(12, 3, 12, 6)})
	require.NoError(t, err)
	assert.Equal(t, []string{annictThirdPending}, pendingRecords(t, f, f.alice.UserID), "the recording's episodes are recorded")
}

// TestRecord は、記録のジョブが、話を 1 話ずつ送り、送れた話は送り直さず、送れなかった話だけを再試行で送ることを確かめる (#927)。
func TestRecord(t *testing.T) {
	t.Parallel()
	f := newRecordFixture(t)
	f.connect(t, f.alice, animerecord.DestinationAnnict)
	live := &animerecord.Target{ServiceID: new(serviceID()), RecordingID: nil}
	_, err := f.service.Report(t.Context(), f.pool, f.inserter, f.alice.UserID, live, []*animerecord.Interval{interval(12, 0, 12, 10)})
	require.NoError(t, err)
	jobs := f.inserter.taken()
	require.Len(t, jobs, 1)

	// 第 3 話は送れ、第 4 話の記録は Annict が 500 を返したので、再試行に回す。
	f.annict.mu.Lock()
	f.annict.failures["createRecord"] = 1
	f.annict.works[annictWorkID].Episodes = []fakeEpisode{{ID: "E4", AnnictID: 104, Number: new(4)}, {ID: "E3", AnnictID: 103, Number: new(3)}}
	f.annict.mu.Unlock()
	err = f.service.Record(t.Context(), f.pool, &jobs[0], false)
	require.Error(t, err, "the job is retried")
	records, _, _ := f.annict.snapshot()
	assert.Equal(t, []string{"E4"}, records, "only one episode reached Annict")
	assert.ElementsMatch(t, []string{annictThirdPending, annictFourthRecorded}, pendingRecords(t, f, f.alice.UserID))

	// 再試行では、送れなかった話だけを送る。
	require.NoError(t, f.service.Record(t.Context(), f.pool, &jobs[0], false))
	records, _, _ = f.annict.snapshot()
	assert.Equal(t, []string{"E4", "E3"}, records)
	// もう一度動いても、送り直さない。
	require.NoError(t, f.service.Record(t.Context(), f.pool, &jobs[0], false))
	records, _, _ = f.annict.snapshot()
	assert.Equal(t, []string{"E4", "E3"}, records, "recorded episodes are never sent again")

	rows, err := f.pool.Query(t.Context(), `SELECT e.kind, e.payload->>'episodes', coalesce(e.payload->>'externalUrl', '') FROM notification_events e ORDER BY e.occurred_at, e.id`)
	require.NoError(t, err)
	type event struct{ Kind, Episodes, URL string }
	events, err := pgx.CollectRows(rows, pgx.RowToStructByPos[event])
	require.NoError(t, err)
	assert.Equal(t, []event{
		{Kind: "anime_recorded", Episodes: "#4", URL: f.annict.server.URL + "/works/10/episodes/104"},
		{Kind: "anime_recorded", Episodes: "#3", URL: f.annict.server.URL + "/works/10/episodes/103"},
	}, events)
	var audience []uuid.UUID
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT audience FROM notification_events LIMIT 1`).Scan(&audience))
	assert.Equal(t, []uuid.UUID{f.alice.UserID}, audience, "only the user who watched is notified")
}

// TestRecordGivesUp は、送り直しても記録できない話と、最後の試行でも送れなかった話を、記録できなかったと通知することを確かめる。
func TestRecordGivesUp(t *testing.T) {
	t.Parallel()
	f := newRecordFixture(t)
	f.connect(t, f.alice, animerecord.DestinationAnnict)
	// Annict には第 3 話しかなく、第 4 話は見つからない。
	f.annict.works[annictWorkID].Episodes = []fakeEpisode{{ID: "E3", AnnictID: 103, Number: new(3)}}
	_, err := f.service.Report(t.Context(), f.pool, f.inserter, f.alice.UserID, &animerecord.Target{ServiceID: new(serviceID()), RecordingID: nil},
		[]*animerecord.Interval{interval(12, 0, 12, 10)})
	require.NoError(t, err)
	jobs := f.inserter.taken()
	require.Len(t, jobs, 1)

	require.NoError(t, f.service.Record(t.Context(), f.pool, &jobs[0], false), "a missing episode is not retried")
	assert.Equal(t, []string{annictThirdRecorded, "annict:4:failed"}, pendingRecords(t, f, f.alice.UserID))
	var message string
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT payload->>'reason' FROM notification_events WHERE kind = 'anime_record_failed'`).Scan(&message))
	assert.Contains(t, message, "not on Annict")

	// 連携を外した後に残った記録は、送らずに諦める。
	_, err = f.pool.Exec(t.Context(), `UPDATE anime_records SET state = 'pending' WHERE episode_number = 4`)
	require.NoError(t, err)
	require.NoError(t, f.service.Disconnect(t.Context(), f.pool, audit.User(f.alice.UserID), f.alice.UserID, animerecord.DestinationAnnict))
	assert.Equal(t, []string{"token-annict-" + f.alice.UserID.String()}, f.annict.revoked, "the Annict token is revoked")
	require.NoError(t, f.service.Record(t.Context(), f.pool, &jobs[0], false))
	assert.Equal(t, []string{annictThirdRecorded, "annict:4:failed"}, pendingRecords(t, f, f.alice.UserID))
}

// reportAndTakeJob は、alice が 12:00 からの番組を 10 分見たことを報告し、入ったジョブを 1 つ返す。
func reportAndTakeJob(t *testing.T, f *recordFixture) *animerecord.RecordArgs {
	t.Helper()
	_, err := f.service.Report(t.Context(), f.pool, f.inserter, f.alice.UserID, &animerecord.Target{ServiceID: new(serviceID()), RecordingID: nil},
		[]*animerecord.Interval{interval(12, 0, 12, 10)})
	require.NoError(t, err)
	jobs := f.inserter.taken()
	require.Len(t, jobs, 1)

	return &jobs[0]
}

// watchingOnEpisode は、alice の設定を、最終話以外を見たら「見てる」にするようにする。
func watchingOnEpisode(t *testing.T, f *recordFixture) {
	t.Helper()
	_, err := f.service.SaveSettings(t.Context(), f.pool, f.alice.UserID, &animerecord.UserSettings{
		ThresholdPercent: animerecord.DefaultThresholdPercent,
		Rules:            &animerecord.StatusRules{WatchingOnFirst: false, WatchingOnEpisode: true, WatchedOnLast: false},
	})
	require.NoError(t, err)
}

// TestRecordStatusFailureDoesNotDuplicate は、記録を付けた後に作品の状態を変えられなくても、送り直しで記録を二重に付けず、
// 記録できなかったとも知らせないことを確かめる。状態の変更は、記録とは別の、できたらよい操作にする。
func TestRecordStatusFailureDoesNotDuplicate(t *testing.T) {
	t.Parallel()
	f := newRecordFixture(t)
	f.connect(t, f.alice, animerecord.DestinationAnnict)
	watchingOnEpisode(t, f)
	job := reportAndTakeJob(t, f)
	f.annict.mu.Lock()
	f.annict.failures[opUpdateStatus] = 1
	f.annict.mu.Unlock()

	_ = f.service.Record(t.Context(), f.pool, job, false)
	require.NoError(t, f.service.Record(t.Context(), f.pool, job, false))
	records, _, _ := f.annict.snapshot()
	assert.Equal(t, []string{"E3", "E4"}, records, "each episode is recorded once")
	assert.Equal(t, []string{annictThirdRecorded, annictFourthRecorded}, pendingRecords(t, f, f.alice.UserID))
	var failed int
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT count(*) FROM notification_events WHERE kind = 'anime_record_failed'`).Scan(&failed))
	assert.Zero(t, failed)
}

// TestRecordSurvivesCancelAfterCreate は、記録を付けた直後にジョブの期限が来ても、付けたことを残し、送り直しで二重に付けないことを確かめる。
func TestRecordSurvivesCancelAfterCreate(t *testing.T) {
	t.Parallel()
	f := newRecordFixture(t)
	f.connect(t, f.alice, animerecord.DestinationAnnict)
	watchingOnEpisode(t, f)
	job := reportAndTakeJob(t, f)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	f.annict.mu.Lock()
	f.annict.hooks[opUpdateStatus] = cancel
	f.annict.mu.Unlock()

	require.Error(t, f.service.Record(ctx, f.pool, job, false), "the job ran out of time")
	f.annict.mu.Lock()
	delete(f.annict.hooks, opUpdateStatus)
	f.annict.mu.Unlock()
	require.NoError(t, f.service.Record(t.Context(), f.pool, job, false))
	records, _, _ := f.annict.snapshot()
	assert.Equal(t, []string{"E3", "E4"}, records, "the episode recorded before the deadline is not recorded again")
}

// TestRecordPicksUpRowsAddedWhileRunning は、ジョブが動いている間に増えた話 (照合し直しで話が結びついたときなど) も、同じジョブで送ることを確かめる。
// 動いている間は、同じ番組と記録先のジョブを重ねて入れないので、送らないと送られないまま残る。
func TestRecordPicksUpRowsAddedWhileRunning(t *testing.T) {
	t.Parallel()
	f := newRecordFixture(t)
	f.connect(t, f.alice, animerecord.DestinationAnnict)
	job := reportAndTakeJob(t, f)
	f.annict.mu.Lock()
	f.annict.works[annictWorkID].Episodes = append(f.annict.works[annictWorkID].Episodes, fakeEpisode{ID: "E5", AnnictID: 105, Number: new(5)})
	added := false
	f.annict.hooks[opCreateRecord] = func() {
		if added {
			return
		}
		added = true
		_, err := f.pool.Exec(context.Background(), `INSERT INTO anime_records (id, user_id, service_id, start_at, destination, syobocal_tid, episode_number,
			is_first, is_last, program_name, state, created_at, updated_at)
			VALUES (gen_random_uuid(), $1, $2, $3, 'annict', $4, 5, $5, $5, 'テスト', 'pending', now(), now())`,
			f.alice.UserID, serviceID(), at(12, 0), testTID, false)
		assert.NoError(t, err)
	}
	f.annict.mu.Unlock()

	require.NoError(t, f.service.Record(t.Context(), f.pool, job, false))
	assert.Equal(t, []string{annictThirdRecorded, annictFourthRecorded, "annict:5:recorded"}, pendingRecords(t, f, f.alice.UserID))
}

package graph_test

import (
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
)

const (
	// timeshiftServiceKey は、faketuner の GR のサービス。
	timeshiftServiceKey = 3273601024
	varServiceID        = "serviceId"
	varRetention        = "retentionSeconds"
	timeshiftChannels   = `{ timeshiftChannels { service { id } retentionSeconds bufferedFrom bufferedUntil recording } }`
	setTimeshiftChannel = `mutation($serviceId: ID!, $retentionSeconds: Int) {
		setTimeshiftChannel(serviceId: $serviceId, retentionSeconds: $retentionSeconds) { service { id } retentionSeconds bufferedFrom bufferedUntil recording } }`
	removeTimeshiftChannel = `mutation($serviceId: ID!) { removeTimeshiftChannel(serviceId: $serviceId) }`
)

type timeshiftChannel struct {
	Service struct {
		ID string `json:"id"`
	} `json:"service"`
	RetentionSeconds int        `json:"retentionSeconds"`
	BufferedFrom     *time.Time `json:"bufferedFrom"`
	BufferedUntil    *time.Time `json:"bufferedUntil"`
	Recording        bool       `json:"recording"`
}

func timeshiftServiceID() string {
	return graph.GlobalID("Service", strconv.FormatInt(timeshiftServiceKey, 10))
}

// listTimeshiftChannels は、タイムシフトで録るチャンネルの一覧を読む。
func (f *fixture) listTimeshiftChannels(t *testing.T) []timeshiftChannel {
	t.Helper()

	var got struct {
		TimeshiftChannels []timeshiftChannel `json:"timeshiftChannels"`
	}
	require.Empty(t, f.query(t, timeshiftChannels, nil, &got))

	return got.TimeshiftChannels
}

// lastTimeshiftAuditAction は、タイムシフトのチャンネルの、最も新しい監査ログの操作の種類を返す。
func (f *fixture) lastTimeshiftAuditAction(t *testing.T) string {
	t.Helper()

	var action string
	require.NoError(t, f.pool.QueryRow(t.Context(),
		"SELECT action FROM audit_events WHERE target_type = 'timeshift_channel' AND target_id = '3273601024' ORDER BY occurred_at DESC, id DESC LIMIT 1").Scan(&action))

	return action
}

// TestTimeshiftChannels は、管理者だけが、タイムシフトで録るチャンネルを読み、足し、保持時間を変え、外せることと、
// バッファの範囲と録っているかを返すことを確かめる。
func TestTimeshiftChannels(t *testing.T) {
	t.Parallel()

	f := newFixture(t, &graph.Options{TimeshiftDir: t.TempDir()})
	f.syncEPG(t)
	set := map[string]any{varServiceID: timeshiftServiceID()}

	// 管理者でなければ、読むことも変えることもできない。
	assert.Equal(t, []string{codeForbidden}, f.query(t, `{ timeshiftAvailable }`, nil, nil))
	assert.Equal(t, []string{codeForbidden}, f.query(t, timeshiftChannels, nil, nil))
	assert.Equal(t, []string{codeForbidden}, f.query(t, setTimeshiftChannel, set, nil))
	assert.Equal(t, []string{codeForbidden}, f.query(t, removeTimeshiftChannel, set, nil))

	admin := f.as(t, adminName, auth.RoleAdmin)
	var available struct {
		TimeshiftAvailable bool `json:"timeshiftAvailable"`
	}
	require.Empty(t, admin.query(t, `{ timeshiftAvailable }`, nil, &available))
	assert.True(t, available.TimeshiftAvailable)
	assert.Empty(t, admin.listTimeshiftChannels(t))

	// 保持時間を省くと、既定の 2 時間にする。まだ録っていないので、バッファは空。
	var added struct {
		SetTimeshiftChannel timeshiftChannel `json:"setTimeshiftChannel"`
	}
	require.Empty(t, admin.query(t, setTimeshiftChannel, set, &added))
	want := timeshiftChannel{RetentionSeconds: 7200, BufferedFrom: nil, BufferedUntil: nil, Recording: false}
	want.Service.ID = timeshiftServiceID()
	assert.Equal(t, want, added.SetTimeshiftChannel)
	assert.Equal(t, []timeshiftChannel{want}, admin.listTimeshiftChannels(t))
	assert.Equal(t, "timeshift_channel.updated", admin.lastTimeshiftAuditAction(t))

	// 書き終えたチャンクと、書いている途中のチャンクがある。書いている途中のものは、今まで録れている。
	now := f.clock.Now()
	exec := func(sql string, args ...any) {
		t.Helper()
		_, err := f.pool.Exec(t.Context(), sql, args...)
		require.NoError(t, err)
	}
	exec(`INSERT INTO timeshift_chunks (id, service_id, path, size, started_at, ended_at, created_at)
		VALUES ($1, $2, 'a.m2ts', 1, $3, $4, $3), ($5, $2, 'b.m2ts', 1, $4, NULL, $4)`,
		id.New(), timeshiftServiceKey, now.Add(-time.Hour), now.Add(-55*time.Minute), id.New())
	got := admin.listTimeshiftChannels(t)
	require.Len(t, got, 1)
	require.NotNil(t, got[0].BufferedFrom)
	require.NotNil(t, got[0].BufferedUntil)
	assert.True(t, got[0].BufferedFrom.Equal(now.Add(-time.Hour)), got[0].BufferedFrom)
	assert.True(t, got[0].BufferedUntil.Equal(now), got[0].BufferedUntil)

	// mikan が割り振りを持ち、リースが切れていなければ、録っている。リースが切れていれば、録っていない。
	holder := id.New()
	exec(`INSERT INTO timeshift_assignments (id, service_id, priority, state, generation, holder, lease_expires_at, created_at, updated_at)
		VALUES ($1, $2, 0, 'recording', 1, $3, $4, $5, $5)`, id.New(), timeshiftServiceKey, holder, now.Add(time.Minute), now)
	assert.True(t, admin.listTimeshiftChannels(t)[0].Recording)
	exec(`UPDATE timeshift_assignments SET lease_expires_at = $1`, now.Add(-time.Second))
	assert.False(t, admin.listTimeshiftChannels(t)[0].Recording)

	// 保持時間を変える。
	require.Empty(t, admin.query(t, setTimeshiftChannel, map[string]any{varServiceID: timeshiftServiceID(), varRetention: 3600}, &added))
	assert.Equal(t, 3600, added.SetTimeshiftChannel.RetentionSeconds)
	assert.Equal(t, 3600, admin.listTimeshiftChannels(t)[0].RetentionSeconds)

	// 外すと、一覧から消え、外したサービスの ID を返す。もう一度外そうとすると、NOT_FOUND を返す。
	var removed struct {
		RemoveTimeshiftChannel string `json:"removeTimeshiftChannel"`
	}
	require.Empty(t, admin.query(t, removeTimeshiftChannel, set, &removed))
	assert.Equal(t, timeshiftServiceID(), removed.RemoveTimeshiftChannel)
	assert.Empty(t, admin.listTimeshiftChannels(t))
	assert.Equal(t, "timeshift_channel.deleted", admin.lastTimeshiftAuditAction(t))
	assert.Equal(t, []string{codeNotFound}, admin.query(t, removeTimeshiftChannel, set, nil))

	// 第三者に影響しない設定の変更なので、直前の認証から時間が経っても、再認証を求めない (設計書 9.5)。
	f.clock.Advance(auth.RecentAuthenticationWindow + time.Second)
	assert.Empty(t, admin.query(t, setTimeshiftChannel, set, nil))
	assert.Empty(t, admin.query(t, removeTimeshiftChannel, set, nil))
}

// TestSetTimeshiftChannelRejectsInvalidInput は、範囲外の保持時間と、ないサービスを拒み、保存しないことを確かめる。
func TestSetTimeshiftChannelRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	f := newFixture(t, &graph.Options{TimeshiftDir: t.TempDir()})
	f.syncEPG(t)
	admin := f.as(t, adminName, auth.RoleAdmin)

	tests := []struct {
		name      string
		serviceID string
		retention int
		want      string
	}{
		{name: "下限より短い保持時間", serviceID: timeshiftServiceID(), retention: 599, want: codeBadUserInput},
		{name: "上限より長い保持時間", serviceID: timeshiftServiceID(), retention: 86401, want: codeBadUserInput},
		{name: "存在しないサービス", serviceID: graph.GlobalID("Service", "999999"), retention: 3600, want: codeNotFound},
		{name: "サービスでない ID", serviceID: graph.GlobalID("Rule", "1"), retention: 3600, want: codeBadUserInput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			variables := map[string]any{varServiceID: tt.serviceID, varRetention: tt.retention}
			assert.Equal(t, []string{tt.want}, admin.query(t, setTimeshiftChannel, variables, nil))
		})
	}
	// 下限と上限そのものは受け付ける。拒んだものは保存していない。
	for _, retention := range []int{600, 86400} {
		require.Empty(t, admin.query(t, setTimeshiftChannel, map[string]any{varServiceID: timeshiftServiceID(), varRetention: retention}, nil), retention)
	}
	got := admin.listTimeshiftChannels(t)
	require.Len(t, got, 1)
	assert.Equal(t, 86400, got[0].RetentionSeconds)
}

// TestTimeshiftUnavailable は、タイムシフトのディレクトリを設定していない tenban では、使えないことを返し、
// チャンネルを足させないことを確かめる。neko は、足されたチャンネルを割り振るので、ここで止める。
func TestTimeshiftUnavailable(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	admin := f.as(t, adminName, auth.RoleAdmin)
	var available struct {
		TimeshiftAvailable bool `json:"timeshiftAvailable"`
	}
	require.Empty(t, admin.query(t, `{ timeshiftAvailable }`, nil, &available))
	assert.False(t, available.TimeshiftAvailable)
	set := map[string]any{varServiceID: timeshiftServiceID(), varRetention: 3600}
	assert.Equal(t, []string{codeConflict}, admin.query(t, setTimeshiftChannel, set, nil))
	assert.Empty(t, admin.listTimeshiftChannels(t))

	// 前に設定したチャンネルは、使えなくなっても外せる。
	_, err := f.pool.Exec(t.Context(), `INSERT INTO timeshift_channels (service_id, retention_seconds, created_at, updated_at, updated_by)
		VALUES ($1, 3600, $2, $2, NULL)`, timeshiftServiceKey, f.clock.Now())
	require.NoError(t, err)
	require.Empty(t, admin.query(t, removeTimeshiftChannel, map[string]any{varServiceID: timeshiftServiceID()}, nil))
	assert.Empty(t, admin.listTimeshiftChannels(t))
}

const timeshiftPrograms = `query($id: ID!) { node(id: $id) { ... on Service {
	timeshiftPrograms { service { id } eventId name startAt endAt playableFrom fromStart gaps { from to } streamUrl hlsUrl } } } }`

type timeshiftProgram struct {
	Service struct {
		ID string `json:"id"`
	} `json:"service"`
	EventID      int        `json:"eventId"`
	Name         string     `json:"name"`
	StartAt      time.Time  `json:"startAt"`
	EndAt        *time.Time `json:"endAt"`
	PlayableFrom time.Time  `json:"playableFrom"`
	FromStart    bool       `json:"fromStart"`
	Gaps         []struct {
		From time.Time `json:"from"`
		To   time.Time `json:"to"`
	} `json:"gaps"`
	StreamURL string `json:"streamUrl"`
	HlsURL    string `json:"hlsUrl"`
}

// TestTimeshiftPrograms は、視聴者が、バッファから再生できる番組と、それを流す URL を読めることを確かめる (設計書 7.2)。
// タイムシフトのディレクトリがない tenban では、バッファを流せないので、番組を返さない。
func TestTimeshiftPrograms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		configured bool
		want       int
	}{
		{name: "タイムシフトのディレクトリがある", configured: true, want: 1},
		{name: "タイムシフトのディレクトリがない", configured: false, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := &graph.Options{}
			if tt.configured {
				opts.TimeshiftDir = t.TempDir()
			}
			f := newFixture(t, opts)
			f.syncEPG(t)
			now := f.clock.Now()
			exec := func(sql string, args ...any) {
				t.Helper()
				_, err := f.pool.Exec(t.Context(), sql, args...)
				require.NoError(t, err)
			}
			exec(`INSERT INTO timeshift_channels (service_id, retention_seconds, created_at, updated_at) VALUES ($1, 7200, $2, $2)`, timeshiftServiceKey, now)
			// 番組の頭のチャンクは破棄した。2 つ目のチャンクの前に穴がある。
			exec(`INSERT INTO timeshift_chunks (id, service_id, path, size, started_at, ended_at, created_at)
				VALUES ($1, $2, 'a.m2ts', 1, $3, $4, $3), ($5, $2, 'b.m2ts', 1, $6, NULL, $6)`,
				id.New(), timeshiftServiceKey, now.Add(-30*time.Minute), now.Add(-25*time.Minute), id.New(), now.Add(-20*time.Minute))
			exec(`INSERT INTO timeshift_programs (service_id, started_at, event_id, name, ended_at) VALUES ($1, $2, 100, '番組', NULL)`,
				timeshiftServiceKey, now.Add(-40*time.Minute))

			viewer := f.as(t, "viewer", auth.RoleViewer)
			var got struct {
				Node struct {
					TimeshiftPrograms []timeshiftProgram `json:"timeshiftPrograms"`
				} `json:"node"`
			}
			require.Empty(t, viewer.query(t, timeshiftPrograms, map[string]any{varID: timeshiftServiceID()}, &got))
			require.Len(t, got.Node.TimeshiftPrograms, tt.want)
			if tt.want == 0 {
				var buffer struct {
					Node struct {
						TimeshiftBuffer *struct {
							From time.Time `json:"from"`
						} `json:"timeshiftBuffer"`
					} `json:"node"`
				}
				require.Empty(t, viewer.query(t, `query($id: ID!) { node(id: $id) { ... on Service { timeshiftBuffer { from } } } }`,
					map[string]any{varID: timeshiftServiceID()}, &buffer))
				assert.Nil(t, buffer.Node.TimeshiftBuffer)

				return
			}
			program := got.Node.TimeshiftPrograms[0]
			assert.Equal(t, timeshiftServiceID(), program.Service.ID)
			assert.Equal(t, 100, program.EventID)
			assert.Nil(t, program.EndAt)
			assert.True(t, now.Add(-40*time.Minute).Equal(program.StartAt))
			assert.True(t, now.Add(-30*time.Minute).Equal(program.PlayableFrom))
			assert.False(t, program.FromStart)
			require.Len(t, program.Gaps, 1)
			assert.True(t, now.Add(-25*time.Minute).Equal(program.Gaps[0].From))
			assert.True(t, now.Add(-20*time.Minute).Equal(program.Gaps[0].To))
			assert.Equal(t, "/streams/timeshift/3273601024?from="+url.QueryEscape(program.PlayableFrom.UTC().Format(time.RFC3339Nano)), program.StreamURL)
			assert.Equal(t, "/streams/timeshift/3273601024-"+strconv.FormatInt(program.PlayableFrom.UnixMicro(), 10), program.HlsURL)

			// サービスのバッファの範囲も返す。今チャンクを書いているので、終わりの途切れはない。
			var buffer struct {
				Node struct {
					TimeshiftBuffer *struct {
						From time.Time `json:"from"`
						Gaps []struct {
							From time.Time `json:"from"`
							To   time.Time `json:"to"`
						} `json:"gaps"`
						StreamURL string `json:"streamUrl"`
					} `json:"timeshiftBuffer"`
				} `json:"node"`
			}
			require.Empty(t, viewer.query(t, `query($id: ID!) { node(id: $id) { ... on Service { timeshiftBuffer { from gaps { from to } streamUrl } } } }`,
				map[string]any{varID: timeshiftServiceID()}, &buffer))
			require.NotNil(t, buffer.Node.TimeshiftBuffer)
			assert.True(t, now.Add(-30*time.Minute).Equal(buffer.Node.TimeshiftBuffer.From))
			require.Len(t, buffer.Node.TimeshiftBuffer.Gaps, 1)
			assert.True(t, now.Add(-25*time.Minute).Equal(buffer.Node.TimeshiftBuffer.Gaps[0].From))
			assert.Equal(t, "/streams/timeshift/3273601024?from="+url.QueryEscape(buffer.Node.TimeshiftBuffer.From.UTC().Format(time.RFC3339Nano)),
				buffer.Node.TimeshiftBuffer.StreamURL)

			// 始まりの時刻で 1 つの番組を引く。ない番組は NOT_FOUND。
			const one = `query($serviceId: ID!, $startAt: Time!) { timeshiftProgram(serviceId: $serviceId, startAt: $startAt) { eventId } }`
			var single struct {
				TimeshiftProgram struct {
					EventID int `json:"eventId"`
				} `json:"timeshiftProgram"`
			}
			require.Empty(t, viewer.query(t, one, map[string]any{varServiceID: timeshiftServiceID(), varStartAt: program.StartAt}, &single))
			assert.Equal(t, 100, single.TimeshiftProgram.EventID)
			assert.Equal(t, []string{codeNotFound}, viewer.query(t, one, map[string]any{varServiceID: timeshiftServiceID(), varStartAt: now}, nil))
		})
	}
}

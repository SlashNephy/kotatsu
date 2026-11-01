package capture_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/query"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/capture"
)

const (
	serviceID = 3273601024
	// maxImageBytes は、テストで受け付ける画像の大きさの上限。大きすぎる画像を、20 MiB を作らずに確かめる。
	maxImageBytes = 64
)

// pngImage は、PNG の先頭の 8 バイトを持つ、テストの画像。
const pngImage = "\x89PNG\r\n\x1a\nimage"

type fixture struct {
	pool     *pgxpool.Pool
	clock    *clocktest.Fake
	dataRoot string
	captures *capture.Captures
	// users は、名前ごとのユーザーの ID。
	users map[string]uuid.UUID
	// recordingID は、テストの録画の ID。
	recordingID uuid.UUID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	clk := clocktest.NewFake(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	users := map[string]uuid.UUID{}
	for _, name := range []string{"alice", "bob"} {
		user, err := auth.NewUsers(clk).Create(t.Context(), pool, &auth.NewUser{
			Username: name, DisplayName: name, Role: auth.RoleViewer, PasswordHash: nil,
		})
		require.NoError(t, err)
		users[name] = user.ID
	}
	_, err = pool.Exec(t.Context(), "INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10)")
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
		VALUES ($1, 32736, 1024, 'こたつテスト1', 'こたつテスト1', 1, 'GR', '27', $2, $2)`, serviceID, clk.Now())
	require.NoError(t, err)
	recordingID := id.New()
	_, err = pool.Exec(t.Context(), `INSERT INTO recordings (id, service_id, name, description, start_at, end_at, state, recorded_from, created_at, updated_at)
		VALUES ($1, $2, '録画', '', $3, $3, 'completed', $3, $3, $3)`, recordingID, serviceID, clk.Now())
	require.NoError(t, err)
	dataRoot := t.TempDir()

	return &fixture{
		pool: pool, clock: clk, dataRoot: dataRoot, users: users, recordingID: recordingID,
		captures: capture.New(kago.New(nil, nil).WithDataDir(dataRoot), clk, slog.New(slog.DiscardHandler), maxImageBytes),
	}
}

// upload は、こたつテスト1 で撮った、番組名と字幕のあるキャプチャを返す。
func (f *fixture) upload() *capture.Upload {
	return &capture.Upload{
		ServiceName: "ＮＨＫ総合", ProgramName: new("ニュース"), Caption: new("こんにちは"), RecordingID: nil, Position: nil,
		BroadcastAt: f.clock.Now().Add(-time.Minute), TakenAt: f.clock.Now(), Image: []byte(pngImage),
	}
}

// files は、データディレクトリにあるファイルの、相対パスを返す。
func (f *fixture) files(t *testing.T) []string {
	t.Helper()

	files := []string{}
	require.NoError(t, filepath.WalkDir(f.dataRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(f.dataRoot, path)
		files = append(files, filepath.ToSlash(rel))

		return err
	}))

	return files
}

// TestSave は、画像をデータディレクトリの「captures/ユーザーの ID/キャプチャの ID.png」に書き、撮った場面の情報を残すことを確かめる。
func TestSave(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		recordingID func(f *fixture) *uuid.UUID
		// wantRecording は、残したキャプチャが、録画と再生位置を持つかどうか。
		wantRecording bool
	}{
		{name: "ライブで撮ったもの", recordingID: func(*fixture) *uuid.UUID { return nil }, wantRecording: false},
		{name: "録画で撮ったもの", recordingID: func(f *fixture) *uuid.UUID { return &f.recordingID }, wantRecording: true},
		{name: "撮った後に消された録画", recordingID: func(*fixture) *uuid.UUID { return new(id.New()) }, wantRecording: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			upload := f.upload()
			upload.RecordingID = tt.recordingID(f)
			if upload.RecordingID != nil {
				upload.Position = new(90 * time.Second)
			}
			saved, err := f.captures.Save(t.Context(), f.pool, f.users["alice"], upload)
			require.NoError(t, err)

			location := kago.CaptureLocation(f.users["alice"], saved.ID)
			assert.Equal(t, []string{location.Path}, f.files(t))
			data, err := os.ReadFile(filepath.Join(f.dataRoot, location.Path))
			require.NoError(t, err)
			assert.Equal(t, []byte(pngImage), data)

			got, err := query.New(f.pool).GetCapture(t.Context(), f.users["alice"], saved.ID)
			require.NoError(t, err)
			assert.Equal(t, "ＮＨＫ総合", got.ServiceName)
			assert.Equal(t, new("ニュース"), got.ProgramName)
			assert.Equal(t, new("こんにちは"), got.Caption)
			assert.Equal(t, int64(len(pngImage)), got.Size)
			assert.True(t, upload.BroadcastAt.Equal(got.BroadcastAt))
			if tt.wantRecording {
				assert.Equal(t, &f.recordingID, got.RecordingID)
				assert.Equal(t, new(90*time.Second), got.Position)
			} else {
				assert.Nil(t, got.RecordingID)
				assert.Nil(t, got.Position)
			}
		})
	}
}

// TestSaveRejects は、PNG でない画像、大きすぎる画像、足りない情報を受け付けず、画像も書かないことを確かめる。
func TestSaveRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		modify func(f *fixture, upload *capture.Upload)
		want   error
	}{
		{name: "PNG でない画像", modify: func(_ *fixture, u *capture.Upload) { u.Image = []byte("GIF89a....") }, want: capture.ErrInvalidImage},
		{name: "空の画像", modify: func(_ *fixture, u *capture.Upload) { u.Image = []byte{} }, want: capture.ErrInvalidImage},
		{
			name: "大きすぎる画像",
			modify: func(_ *fixture, u *capture.Upload) {
				u.Image = append([]byte(pngImage), make([]byte, maxImageBytes)...)
			},
			want: capture.ErrTooLarge,
		},
		{name: "サービス名がない", modify: func(_ *fixture, u *capture.Upload) { u.ServiceName = " " }, want: capture.ErrInvalidMetadata},
		{name: "放送時刻がない", modify: func(_ *fixture, u *capture.Upload) { u.BroadcastAt = time.Time{} }, want: capture.ErrInvalidMetadata},
		{name: "UTF-8 でない番組名", modify: func(_ *fixture, u *capture.Upload) { u.ProgramName = new("\xff") }, want: capture.ErrInvalidMetadata},
		{name: "録画のない再生位置", modify: func(_ *fixture, u *capture.Upload) { u.Position = new(time.Second) }, want: capture.ErrInvalidMetadata},
		{
			name: "負の再生位置",
			modify: func(f *fixture, u *capture.Upload) {
				u.RecordingID, u.Position = &f.recordingID, new(-time.Second)
			},
			want: capture.ErrInvalidMetadata,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			upload := f.upload()
			tt.modify(f, upload)
			_, err := f.captures.Save(t.Context(), f.pool, f.users["alice"], upload)
			require.ErrorIs(t, err, tt.want)
			assert.Empty(t, f.files(t))
		})
	}
}

// TestUnavailableWithoutDataDir は、データディレクトリを設定していなければ、残すのも消すのも断ることを確かめる。
func TestUnavailableWithoutDataDir(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	captures := capture.New(kago.New(nil, nil), f.clock, slog.New(slog.DiscardHandler), 0)
	assert.False(t, captures.Available())
	_, err := captures.Save(t.Context(), f.pool, f.users["alice"], f.upload())
	require.ErrorIs(t, err, capture.ErrUnavailable)
	require.ErrorIs(t, captures.Delete(t.Context(), f.pool, f.users["alice"], id.New()), capture.ErrUnavailable)
}

// TestDelete は、撮ったユーザーだけが、行と画像を消せることを確かめる。
func TestDelete(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	saved, err := f.captures.Save(t.Context(), f.pool, f.users["alice"], f.upload())
	require.NoError(t, err)
	location := kago.CaptureLocation(f.users["alice"], saved.ID)

	// ほかのユーザーには、ないものとして答え、画像も残す。
	require.ErrorIs(t, f.captures.Delete(t.Context(), f.pool, f.users["bob"], saved.ID), capture.ErrNotFound)
	assert.Equal(t, []string{location.Path}, f.files(t))

	require.NoError(t, f.captures.Delete(t.Context(), f.pool, f.users["alice"], saved.ID))
	assert.Empty(t, f.files(t))
	_, err = query.New(f.pool).GetCapture(t.Context(), f.users["alice"], saved.ID)
	require.ErrorIs(t, err, pgx.ErrNoRows)
	require.ErrorIs(t, f.captures.Delete(t.Context(), f.pool, f.users["alice"], saved.ID), capture.ErrNotFound)
}

// TestRecordingDeletionKeepsCapture は、録画を消しても、キャプチャと画像は残し、録画の参照だけを外すことを確かめる。
func TestRecordingDeletionKeepsCapture(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	upload := f.upload()
	upload.RecordingID, upload.Position = &f.recordingID, new(30*time.Second)
	saved, err := f.captures.Save(t.Context(), f.pool, f.users["alice"], upload)
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), "DELETE FROM recordings WHERE id = $1", f.recordingID)
	require.NoError(t, err)

	got, err := query.New(f.pool).GetCapture(t.Context(), f.users["alice"], saved.ID)
	require.NoError(t, err)
	assert.Nil(t, got.RecordingID)
	assert.Equal(t, []string{kago.CaptureLocation(f.users["alice"], saved.ID).Path}, f.files(t))
}

// TestListCaptures は、撮ったユーザーのキャプチャだけを、残した新しい順に返し、探す言葉で番組名とサービス名から絞ることを確かめる。
func TestListCaptures(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	save := func(user, serviceName string, programName *string) uuid.UUID {
		upload := f.upload()
		upload.ServiceName, upload.ProgramName = serviceName, programName
		saved, err := f.captures.Save(t.Context(), f.pool, f.users[user], upload)
		require.NoError(t, err)
		f.clock.Advance(time.Second)

		return saved.ID
	}
	news := save("alice", "ＮＨＫ総合", new("ニュース７"))
	anime := save("alice", "こたつテスト1", new("アニメ 100%"))
	unknown := save("alice", "こたつテスト1", nil)
	_ = save("bob", "ＮＨＫ総合", new("ニュース７"))

	tests := []struct {
		name    string
		keyword string
		want    []uuid.UUID
	}{
		{name: "絞らなければ、自分のものを新しい順に返す", keyword: "", want: []uuid.UUID{unknown, anime, news}},
		{name: "半角と小文字でも、全角のサービス名に当たる", keyword: "nhk", want: []uuid.UUID{news}},
		{name: "空白で区切った語は、すべてを含むものだけ", keyword: "こたつ　アニメ", want: []uuid.UUID{anime}},
		{name: "番組名とサービス名にまたがる語", keyword: "ニュース 総合", want: []uuid.UUID{news}},
		{name: "LIKE の特別な文字は、文字そのものとして探す", keyword: "0%", want: []uuid.UUID{anime}},
		{name: "合うものがない", keyword: "ドラマ", want: []uuid.UUID{}},
	}
	for _, tt := range tests {
		got, err := query.New(f.pool).ListCaptures(t.Context(), f.users["alice"], &query.CaptureListQuery{Keyword: tt.keyword, Before: nil, Limit: 10})
		require.NoError(t, err, tt.name)
		ids := make([]uuid.UUID, 0, len(got))
		for _, c := range got {
			ids = append(ids, c.ID)
		}
		assert.Equal(t, tt.want, ids, tt.name)
	}
}

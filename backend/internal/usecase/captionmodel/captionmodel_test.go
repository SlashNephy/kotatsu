package captionmodel_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/captionmodel"
)

const (
	// modelBody は、手元のサーバーが返すモデルの中身。
	modelBody = "ggml model body"
	// testModelID は、テストの一覧の書き起こしのモデルの ID。
	testModelID = "test-whisper"
	// modelPath は、データディレクトリの中の、書き起こしのモデルの置き場所。
	modelPath = "whisper/ggml-model.bin"
	// vadPath は、データディレクトリの中の、VAD のモデルの置き場所。
	vadPath = "whisper/ggml-silero-vad.bin"
	// oldBody は、前に置いてあったモデルの中身。
	oldBody = "old model"
	// sileroID は、一覧の Silero VAD の ID。
	sileroID = "silero-v5.1.2"
	// stateCancelled は、取りやめたジョブの状態。
	stateCancelled = "cancelled"
	schemeHTTPS    = "https"
)

type fixture struct {
	pool     *pgxpool.Pool
	clock    *clocktest.Fake
	dataDir  string
	inserter *jobs.Client
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	require.NoError(t, jobs.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	inserter, err := jobs.NewClient(pool, slog.New(slog.DiscardHandler), &jobs.Options{PollOnly: false, Workers: nil, PeriodicJobs: nil})
	require.NoError(t, err)

	return &fixture{pool: pool, clock: clocktest.NewFake(time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)), dataDir: t.TempDir(), inserter: inserter}
}

func checksum(body string) string {
	sum := sha256.Sum256([]byte(body))

	return hex.EncodeToString(sum[:])
}

// testModel は、手元のサーバー url から落とす、書き起こしのモデル。
func testModel(url string) captionmodel.Model {
	return captionmodel.Model{
		ID: testModelID, Kind: captionmodel.KindWhisper, Name: "テスト", URL: url + "/model.bin",
		Size: int64(len(modelBody)), SHA256: checksum(modelBody), License: "MIT", Recommended: true,
	}
}

// TestWorker は、モデルを落として照合し、既定の場所に置いて記録することと、照合できなければ前のモデルを残すことを確かめる。
func TestWorker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// body と status は、手元のサーバーが返すもの。
		body   string
		status int
		// modelID と kind は、ジョブの引数。
		modelID string
		kind    captionmodel.Kind
		// wantCancel は、やり直さずに取りやめるかどうか。wantErr は、失敗するかどうか。
		wantErr    bool
		wantCancel bool
		// wantBody は、置き場所のファイルの中身。
		wantBody string
	}{
		{name: "照合できたら置き換える", body: modelBody, status: http.StatusOK, modelID: testModelID, kind: captionmodel.KindWhisper, wantErr: false, wantCancel: false, wantBody: modelBody},
		{name: "SHA-256 が違えば取りやめて前のモデルを残す", body: "ggml model BODY", status: http.StatusOK, modelID: testModelID, kind: captionmodel.KindWhisper, wantErr: true, wantCancel: true, wantBody: oldBody},
		{name: "途中で切れたらやり直す", body: "ggml", status: http.StatusOK, modelID: testModelID, kind: captionmodel.KindWhisper, wantErr: true, wantCancel: false, wantBody: oldBody},
		{name: "大きすぎるものは読み切らずにやり直す", body: modelBody + "!", status: http.StatusOK, modelID: testModelID, kind: captionmodel.KindWhisper, wantErr: true, wantCancel: false, wantBody: oldBody},
		{name: "入手先がエラーならやり直す", body: "", status: http.StatusServiceUnavailable, modelID: testModelID, kind: captionmodel.KindWhisper, wantErr: true, wantCancel: false, wantBody: oldBody},
		{name: "一覧にないモデルは取りやめる", body: modelBody, status: http.StatusOK, modelID: "unknown", kind: captionmodel.KindWhisper, wantErr: true, wantCancel: true, wantBody: oldBody},
		{name: "種類の合わないモデルは取りやめる", body: modelBody, status: http.StatusOK, modelID: testModelID, kind: captionmodel.KindVAD, wantErr: true, wantCancel: true, wantBody: oldBody},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			t.Cleanup(server.Close)
			dest := filepath.Join(f.dataDir, filepath.FromSlash(modelPath))
			require.NoError(t, os.MkdirAll(filepath.Dir(dest), 0o755))
			require.NoError(t, os.WriteFile(dest, []byte(oldBody), 0o600))
			worker := captionmodel.NewWorker(f.pool, f.clock, slog.New(slog.DiscardHandler), nil, f.dataDir)
			captionmodel.SetCatalog(worker, []captionmodel.Model{testModel(server.URL)})

			err := worker.Work(t.Context(), &river.Job[captionmodel.DownloadArgs]{
				JobRow: new(rivertype.JobRow), Args: captionmodel.DownloadArgs{Slot: tt.kind, ModelID: tt.modelID},
			})

			if tt.wantErr {
				require.Error(t, err)
				var cancel *rivertype.JobCancelError
				assert.Equal(t, tt.wantCancel, errors.As(err, &cancel))
			} else {
				require.NoError(t, err)
			}
			got, err := os.ReadFile(dest)
			require.NoError(t, err)
			assert.Equal(t, tt.wantBody, string(got))
			assert.NoFileExists(t, dest+".part")
			records, err := sqlc.New(f.pool).ListCaptionModels(t.Context())
			require.NoError(t, err)
			if tt.wantErr {
				assert.Empty(t, records)
			} else {
				require.Len(t, records, 1)
				assert.Equal(t, sqlc.CaptionModel{
					Kind: "WHISPER", ModelID: testModelID, Size: int64(len(modelBody)), SHA256: checksum(modelBody), InstalledAt: f.clock.Now(),
				}, records[0])
			}
		})
	}
}

// TestEnqueueTx は、同じ種類のダウンロードを重ねて入れず、ほかの種類は入れることを確かめる。
func TestEnqueueTx(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	whisper, ok := captionmodel.Lookup("kotoba-whisper-v2.0-q5_0")
	require.True(t, ok)
	small, ok := captionmodel.Lookup("whisper-small-q5_1")
	require.True(t, ok)
	vad, ok := captionmodel.Lookup(sileroID)
	require.True(t, ok)
	tests := []struct {
		name  string
		model *captionmodel.Model
		want  bool
	}{
		{name: "最初のダウンロードは入れる", model: whisper, want: true},
		{name: "同じ種類のほかのモデルは重ねて入れない", model: small, want: false},
		{name: "ほかの種類は入れる", model: vad, want: true},
	}
	for _, tt := range tests {
		var inserted bool
		err := pgx.BeginFunc(t.Context(), f.pool, func(tx pgx.Tx) error {
			var err error
			inserted, err = captionmodel.EnqueueTx(t.Context(), tx, f.inserter, tt.model)

			return err
		})
		require.NoError(t, err, tt.name)
		assert.Equal(t, tt.want, inserted, tt.name)
	}
}

// insertJob は、種類 kind のモデル modelID のダウンロードのジョブを入れ、状態を state にし、errorText を最後のエラーにする。
func (f *fixture) insertJob(t *testing.T, kind captionmodel.Kind, modelID, state, errorText string) int64 {
	t.Helper()

	result, err := f.inserter.Insert(t.Context(), captionmodel.DownloadArgs{Slot: kind, ModelID: modelID}, nil)
	require.NoError(t, err)
	errs := "{}"
	if errorText != "" {
		errs = `{"{\"at\":\"2026-11-16T12:00:00Z\",\"attempt\":1,\"error\":\"` + errorText + `\",\"trace\":\"\"}"}`
	}
	_, err = f.pool.Exec(t.Context(), `UPDATE river_job SET state = $2::river_job_state, errors = $3::jsonb[],
		finalized_at = CASE WHEN $2 IN ('completed', 'cancelled', 'discarded') THEN now() END, attempt = 1, attempted_at = now() WHERE id = $1`,
		result.Job.ID, state, errs)
	require.NoError(t, err)

	return result.Job.ID
}

// TestStatuses は、置いたモデルとダウンロードのジョブとファイルから、種類ごとの状態を決めることを確かめる。
func TestStatuses(t *testing.T) {
	t.Parallel()

	kotoba, ok := captionmodel.Lookup("kotoba-whisper-v2.0-q5_0")
	require.True(t, ok)
	tests := []struct {
		name string
		// record は、DB に記録した書き起こしのモデルの大きさ。0 なら記録しない。
		record int64
		// file は、置き場所に置くファイルの中身。空なら置かない。
		file string
		// job は、書き起こしのモデルのダウンロードのジョブの状態と最後のエラー。空なら入れない。
		jobState, jobError string
		progress           bool
		// mounted は、データディレクトリをマウントしているかどうか。
		mounted bool
		cfg     config.Captions
		want    captionmodel.State
		// wantModel は、置いたモデルが一覧のものと分かるかどうか。wantSize は、置いたモデルの大きさ (置いていなければ 0)。
		wantModel      bool
		wantSize       int64
		wantDownload   bool
		wantProgress   bool
		wantOverridden bool
	}{
		{name: "置いていない", mounted: true, want: captionmodel.StateNotPlaced},
		{name: "画面から置いた", record: 9, file: "123456789", mounted: true, want: captionmodel.StatePlaced, wantModel: true, wantSize: 9},
		{name: "手で置いた", file: "1234", mounted: true, want: captionmodel.StatePlaced, wantSize: 4},
		{name: "記録の後に手で置き換えた", record: 9, file: "1234", mounted: true, want: captionmodel.StatePlaced, wantSize: 4},
		{name: "記録があってもファイルがなければ置いていない", record: 9, mounted: true, want: captionmodel.StateNotPlaced},
		{name: "マウントしていなければ記録で決める", record: 9, mounted: false, want: captionmodel.StatePlaced, wantModel: true, wantSize: 9},
		{name: "待っている", mounted: true, jobState: "available", want: captionmodel.StateDownloading, wantDownload: true},
		{name: "ダウンロードしている", mounted: true, jobState: "running", progress: true, want: captionmodel.StateDownloading, wantDownload: true, wantProgress: true},
		{name: "試す回数を使い切った", record: 9, file: "123456789", mounted: true, jobState: "discarded", jobError: "failed", want: captionmodel.StateFailed, wantModel: true, wantSize: 9, wantDownload: true},
		{name: "照合に失敗した", mounted: true, jobState: stateCancelled, jobError: "the SHA-256 of the downloaded model does not match", want: captionmodel.StateFailed, wantDownload: true},
		{name: "動いている間に取り消した", mounted: true, jobState: stateCancelled, jobError: "job cancelled remotely", want: captionmodel.StateNotPlaced},
		{name: "待っている間に取り消した", mounted: true, jobState: stateCancelled, want: captionmodel.StateNotPlaced},
		{name: "終えたジョブは出さない", record: 9, file: "123456789", mounted: true, jobState: "completed", want: captionmodel.StatePlaced, wantModel: true, wantSize: 9},
		{name: "設定で場所を書いた", mounted: true, cfg: config.Captions{WhisperModel: "/models/ggml.bin"}, want: captionmodel.StateNotPlaced, wantOverridden: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			if tt.record > 0 {
				require.NoError(t, sqlc.New(f.pool).UpsertCaptionModel(t.Context(), sqlc.UpsertCaptionModelParams{
					Kind: "WHISPER", ModelID: kotoba.ID, Size: tt.record, SHA256: kotoba.SHA256, Now: f.clock.Now(),
				}))
			}
			if tt.file != "" {
				dest := filepath.Join(f.dataDir, filepath.FromSlash(modelPath))
				require.NoError(t, os.MkdirAll(filepath.Dir(dest), 0o755))
				require.NoError(t, os.WriteFile(dest, []byte(tt.file), 0o600))
			}
			if tt.jobState != "" {
				jobID := f.insertJob(t, captionmodel.KindWhisper, kotoba.ID, tt.jobState, tt.jobError)
				if tt.progress {
					require.NoError(t, sqlc.New(f.pool).UpsertJobProgress(t.Context(), sqlc.UpsertJobProgressParams{
						JobID: jobID, Attempt: 1, Ratio: 0.25, RemainingMs: new(int64(60000)), Now: f.clock.Now(),
					}))
				}
			}
			dataDir := ""
			if tt.mounted {
				dataDir = f.dataDir
			}

			statuses, err := captionmodel.Statuses(t.Context(), f.pool, dataDir, &tt.cfg)
			require.NoError(t, err)

			require.Len(t, statuses, 2)
			assert.Equal(t, captionmodel.KindVAD, statuses[1].Kind)
			assert.Equal(t, captionmodel.StateNotPlaced, statuses[1].State)
			got := statuses[0]
			assert.Equal(t, captionmodel.KindWhisper, got.Kind)
			assert.Equal(t, tt.want, got.State)
			assert.Equal(t, tt.wantOverridden, got.Overridden)
			if tt.wantSize == 0 {
				assert.Nil(t, got.Installed)
			} else {
				require.NotNil(t, got.Installed)
				assert.Equal(t, tt.wantSize, got.Installed.Size)
				assert.Equal(t, tt.wantModel, got.Installed.Model != nil)
			}
			if !tt.wantDownload {
				assert.Nil(t, got.Download)

				return
			}
			require.NotNil(t, got.Download)
			require.NotNil(t, got.Download.Model)
			assert.Equal(t, kotoba.ID, got.Download.Model.ID)
			if tt.wantProgress {
				require.NotNil(t, got.Download.Progress)
				assert.InDelta(t, 0.25, got.Download.Progress.Ratio, 1e-9)
				assert.Equal(t, new(time.Minute), got.Download.Progress.Remaining)
			} else {
				assert.Nil(t, got.Download.Progress)
			}
		})
	}
}

// TestRemove は、置いたモデルのファイルと記録を消すことと、ダウンロードしている間や、置いていなければ消さないことを確かめる。
func TestRemove(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// record と file は、VAD のモデルを記録したか、置いたか。active は、VAD のダウンロードが動いているか。
		record, file, active bool
		mounted              bool
		wantErr              error
		wantModel            string
	}{
		{name: "画面から置いたモデルを消す", record: true, file: true, mounted: true, wantErr: nil, wantModel: sileroID},
		{name: "手で置いたモデルを消す", file: true, mounted: true, wantErr: nil, wantModel: ""},
		{name: "ファイルのない記録を消す", record: true, mounted: true, wantErr: nil, wantModel: sileroID},
		{name: "置いていない", mounted: true, wantErr: captionmodel.ErrNotFound},
		{name: "ダウンロードしている間は消さない", record: true, file: true, active: true, mounted: true, wantErr: captionmodel.ErrDownloading},
		{name: "マウントしていなければ消せない", record: true, file: true, mounted: false, wantErr: captionmodel.ErrNoDataDir},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			dest := filepath.Join(f.dataDir, filepath.FromSlash(vadPath))
			if tt.record {
				require.NoError(t, sqlc.New(f.pool).UpsertCaptionModel(t.Context(), sqlc.UpsertCaptionModelParams{
					Kind: "VAD", ModelID: sileroID, Size: 5, SHA256: checksum("model"), Now: f.clock.Now(),
				}))
			}
			if tt.file {
				require.NoError(t, os.MkdirAll(filepath.Dir(dest), 0o755))
				require.NoError(t, os.WriteFile(dest, []byte("model"), 0o600))
			}
			if tt.active {
				f.insertJob(t, captionmodel.KindVAD, sileroID, "running", "")
			}
			dataDir := ""
			if tt.mounted {
				dataDir = f.dataDir
			}

			var removed *sqlc.CaptionModel
			err := pgx.BeginFunc(t.Context(), f.pool, func(tx pgx.Tx) error {
				var err error
				removed, err = captionmodel.Remove(t.Context(), tx, dataDir, captionmodel.KindVAD)

				return err
			})

			records, listErr := sqlc.New(f.pool).ListCaptionModels(t.Context())
			require.NoError(t, listErr)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Len(t, records, map[bool]int{true: 1, false: 0}[tt.record])
				if tt.file {
					assert.FileExists(t, dest)
				}

				return
			}
			require.NoError(t, err)
			assert.Empty(t, records)
			assert.NoFileExists(t, dest)
			if tt.wantModel == "" {
				assert.Nil(t, removed)
			} else {
				require.NotNil(t, removed)
				assert.Equal(t, tt.wantModel, removed.ModelID)
			}
		})
	}
}

// TestAllowedHost は、ダウンロードの転送を、HTTPS の Hugging Face とその配信網にだけ許すことを確かめる。
func TestAllowedHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		scheme string
		host   string
		want   bool
	}{
		{name: "Hugging Face", scheme: schemeHTTPS, host: "huggingface.co", want: true},
		{name: "配信網", scheme: schemeHTTPS, host: "us.aws.cdn.hf.co", want: true},
		{name: "以前の配信網", scheme: schemeHTTPS, host: "cdn-lfs.huggingface.co", want: true},
		{name: "HTTP には従わない", scheme: "http", host: "huggingface.co", want: false},
		{name: "名前の似たほかのホスト", scheme: schemeHTTPS, host: "evilhf.co", want: false},
		{name: "ほかのホスト", scheme: schemeHTTPS, host: "example.com", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, captionmodel.AllowedHost(tt.scheme, tt.host))
		})
	}
}

// TestCatalog は、一覧のモデルが、種類ごとに 1 つ勧めるものを持ち、置き場所の違う種類が入り混じらないことを確かめる。
func TestCatalog(t *testing.T) {
	t.Parallel()

	recommended := map[captionmodel.Kind]int{}
	ids := map[string]bool{}
	for _, m := range captionmodel.Catalog() {
		assert.False(t, ids[m.ID], "duplicate ID %s", m.ID)
		ids[m.ID] = true
		assert.True(t, captionmodel.AllowedHost(schemeHTTPS, hostOf(t, m.URL)), m.ID)
		assert.Len(t, m.SHA256, sha256.Size*2, m.ID)
		if m.Recommended {
			recommended[m.Kind]++
		}
	}
	assert.Equal(t, map[captionmodel.Kind]int{captionmodel.KindWhisper: 1, captionmodel.KindVAD: 1}, recommended)
}

func hostOf(t *testing.T, raw string) string {
	t.Helper()

	parsed, err := url.Parse(raw)
	require.NoError(t, err)

	return parsed.Hostname()
}

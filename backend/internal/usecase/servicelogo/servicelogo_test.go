package servicelogo_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/servicelogo"
)

const (
	networkID = 4
	logoID    = 9
	// serviceID は、ロゴの番号 logoID を持つサービス。
	serviceID = 400141
	// noLogoServiceID は、ロゴの番号を持たないサービス。
	noLogoServiceID = 400151
)

type fixture struct {
	pool      *pgxpool.Pool
	clock     *clocktest.Fake
	collector *servicelogo.Collector
	store     *servicelogo.Store
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	clk := clocktest.NewFake(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	_, err = pool.Exec(t.Context(), "INSERT INTO channel_types (type, display_name, sort_order) VALUES ('BS', 'BS', 20)")
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, logo_id, created_at, updated_at)
		VALUES ($1, $2, 141, 'こたつ BS', 'こたつ BS', 1, 'BS', 'BS13_0', $3, $4, $4), ($5, $2, 151, 'こたつ BS2', 'こたつ BS2', 1, 'BS', 'BS13_0', NULL, $4, $4)`,
		serviceID, networkID, logoID, clk.Now(), noLogoServiceID)
	require.NoError(t, err)

	return &fixture{
		pool: pool, clock: clk, collector: servicelogo.NewCollector(clk, slog.New(slog.DiscardHandler)), store: servicelogo.NewStore(pool),
	}
}

// broadcastPNG は、幅 width のパレットの PNG から、PLTE と tRNS を除いたものを返す。放送のロゴは、パレットを持たずに送られる。
func broadcastPNG(t *testing.T, width int) []byte {
	t.Helper()

	img := image.NewPaletted(image.Rect(0, 0, width, 1), color.Palette{color.Black, color.White})
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	data := buf.Bytes()
	out := append([]byte{}, data[:8]...)
	for rest := data[8:]; len(rest) > 0; {
		length := int(binary.BigEndian.Uint32(rest))
		if name := string(rest[4:8]); name != "PLTE" && name != "tRNS" {
			out = append(out, rest[:12+length]...)
		}
		rest = rest[12+length:]
	}

	return out
}

func (f *fixture) flush(ctx context.Context) {
	f.collector.Flush(ctx, sqlc.New(f.pool))
}

// TestCollector は、集めたロゴを、パレットを補って書き、サービスのロゴとして返すことを確かめる (#1308)。
// 種類が複数あれば、HD の大きいロゴを返す。
func TestCollector(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	small, large := broadcastPNG(t, 48), broadcastPNG(t, 64)
	_, err := f.store.ServiceLogo(t.Context(), serviceID)
	require.ErrorIs(t, err, servicelogo.ErrNoLogo)

	f.collector.Observe(&ts.Logo{NetworkID: networkID, LogoID: logoID, Type: 2, Version: 1, PNG: small})
	f.collector.Observe(&ts.Logo{NetworkID: networkID, LogoID: logoID, Type: 5, Version: 1, PNG: large})
	f.flush(t.Context())

	got, err := f.store.ServiceLogo(t.Context(), serviceID)
	require.NoError(t, err)
	config, err := png.DecodeConfig(bytes.NewReader(got))
	require.NoError(t, err)
	assert.Equal(t, 64, config.Width)
	want, err := ts.LogoPNG(large)
	require.NoError(t, err)
	assert.Equal(t, want, got)

	// ロゴの番号を持たないサービスと、ないサービスには返さない。
	for _, id := range []int64{noLogoServiceID, 1} {
		_, err := f.store.ServiceLogo(t.Context(), id)
		require.ErrorIs(t, err, servicelogo.ErrNoLogo)
	}
}

// TestCollectorVersion は、一度書いた版のロゴは書き直さず、版が替われば書き直すことを確かめる (#1308)。
func TestCollectorVersion(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	data := broadcastPNG(t, 64)
	f.collector.Observe(&ts.Logo{NetworkID: networkID, LogoID: logoID, Type: 5, Version: 1, PNG: data})
	f.flush(t.Context())
	_, err := f.pool.Exec(t.Context(), "UPDATE service_logos SET data = 'written by the test'")
	require.NoError(t, err)
	read := func() string {
		t.Helper()

		var data []byte
		require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT data FROM service_logos").Scan(&data))

		return string(data)
	}

	// 同じ版は、繰り返し届いても書き直さない。
	f.collector.Observe(&ts.Logo{NetworkID: networkID, LogoID: logoID, Type: 5, Version: 1, PNG: data})
	f.flush(t.Context())
	assert.Equal(t, "written by the test", read())

	f.collector.Observe(&ts.Logo{NetworkID: networkID, LogoID: logoID, Type: 5, Version: 2, PNG: data})
	f.flush(t.Context())
	want, err := ts.LogoPNG(data)
	require.NoError(t, err)
	assert.Equal(t, string(want), read())
}

// TestCollectorRetry は、DB に書けなかったロゴを、次の Flush で書き直すことを確かめる (#1308)。読めないロゴは書かない。
func TestCollectorRetry(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.collector.Observe(&ts.Logo{NetworkID: networkID, LogoID: logoID, Type: 5, Version: 1, PNG: broadcastPNG(t, 64)})
	f.collector.Observe(&ts.Logo{NetworkID: networkID, LogoID: logoID, Type: 2, Version: 1, PNG: []byte("\x89PNG\r\n\x1a\nbroken")})
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	f.flush(canceled)
	count := func() int {
		t.Helper()

		var n int
		require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT count(*) FROM service_logos").Scan(&n))

		return n
	}
	assert.Zero(t, count())

	f.flush(t.Context())
	assert.Equal(t, 1, count())
	_, err := f.store.ServiceLogo(t.Context(), serviceID)
	require.NoError(t, err)
}

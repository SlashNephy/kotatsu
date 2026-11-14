package settings_test

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

// TestURLGate は、外部プレイヤーと IPTV のクライアントの URL を受け付けるかを、要求のたびに DB から読み、
// DB に届かない間は、届いていた間に読めた設定を使うことを確かめる (#1755、#1084)。読めた設定がなければ、DB に届かないことを返す。
func TestURLGate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// warm は、DB が止まる前に、一度設定を読んでおくかどうか。
		warm         bool
		wantExternal bool
		wantIPTV     bool
		wantErr      error
	}{
		{name: "DB が止まる前に読めた設定を使う", warm: true, wantExternal: true, wantIPTV: false, wantErr: nil},
		{name: "読めた設定がなければ、DB に届かないことを返す", warm: false, wantExternal: false, wantIPTV: false, wantErr: settings.ErrDatabaseUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pool := newDB(t)
			clk := clocktest.NewFake(testNow())
			_, err := settings.NewStore(clk, audit.NewRecorder(clk)).Update(t.Context(), pool, audit.System("test"),
				&settings.Changes{ExternalPlayerURLsEnabled: new(true)})
			require.NoError(t, err)
			conn := databasetest.NewSwitch(t, pool)
			gate := settings.NewURLGate(conn.Pool, clk, slog.New(slog.DiscardHandler))
			if tt.warm {
				enabled, err := gate.ExternalPlayerURLsEnabled(t.Context())
				require.NoError(t, err)
				require.True(t, enabled)
			}
			conn.Down()

			external, err := gate.ExternalPlayerURLsEnabled(t.Context())
			require.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.wantExternal, external)
			iptv, err := gate.IPTVFeedsEnabled(t.Context())
			require.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.wantIPTV, iptv)
		})
	}
}

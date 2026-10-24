package webui_test

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/web/spa"
	"github.com/SlashNephy/kotatsu/internal/web/webui"
)

// TestAppServes は、埋め込んだ SPA から、ビルドの有無にかかわらず SPA の配信を作れることを確かめる。
// CI の Go のジョブは SPA をビルドしないので、dist には .gitkeep だけがある。
func TestAppServes(t *testing.T) {
	t.Parallel()

	_, err := spa.NewHandler(webui.App(), slog.New(slog.DiscardHandler))
	require.NoError(t, err)
}

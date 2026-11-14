package cli

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/infra/nicolog"
	"github.com/SlashNephy/kotatsu/internal/infra/syobocal"
	"github.com/SlashNephy/kotatsu/internal/infra/workids"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/netactivity"
	"github.com/SlashNephy/kotatsu/internal/usecase/anime"
	"github.com/SlashNephy/kotatsu/internal/usecase/animerecord"
)

// animeJobParts は、nekonote で、アニメの番組と作品を結びつけ、見たアニメを記録するジョブの部品 (設計書 3.8、#927)。
type animeJobParts struct {
	importer *anime.Importer
	matcher  *anime.Matcher
	nicolog  *anime.NicologMatcher
	records  *animerecord.Service
}

// animeLogger は、アニメの番組と作品を結びつける部品のロガー。wire は型で依存を見分けるので、ほかのロガーと分ける。
type animeLogger struct {
	*slog.Logger
}

func animeLoggerOf(logger *slog.Logger) animeLogger {
	return animeLogger{Logger: logger.With(slog.String("component", "anime"))}
}

// newSyobocalClient は、しょぼいカレンダーへ問い合わせる部品を作る。
// 問い合わせの間隔は、このプロセスの取り込みと照合のジョブすべてで共有するので、1 つだけ作る。
func newSyobocalClient(deps *Deps) *syobocal.Client {
	return syobocal.NewClient(&syobocal.Options{
		HTTPClient: netactivity.NewClient(deps.Network, netactivity.PurposeAnimeProgramSync, netactivity.ClientOptions{Transport: nil, Timeout: 0, CheckRedirect: nil}),
		URL:        "", Interval: 0,
	})
}

// newRecordingAnimeRecords は、見たアニメを、ユーザーが連携した Annict と AniList に記録する部品を作る (#927)。
func newRecordingAnimeRecords(deps *Deps, logger *slog.Logger, inserter *jobs.Client) (*animerecord.Service, error) {
	return newAnimeRecords(deps, logger, inserter, netactivity.PurposeAnimeRecord)
}

// newAnimeImporter は、連携先の情報を取り込む部品を作る (設計書 3.8)。管理者が Annict のトークンを設定していれば、
// Annict と AniList から作品 ID の対応も作る (#1223)。トークンは、見たアニメの記録の連携 (records) のものを使う。
func newAnimeImporter(
	deps *Deps, pool *pgxpool.Pool, clk clock.Clock, syobocalClient *syobocal.Client, inserter *jobs.Client, records *animerecord.Service, logger animeLogger,
) *anime.Importer {
	return anime.NewImporter(pool, clk, animeDataClient(deps.Config, deps.Network), syobocalClient, inserter, logger.Logger).WithWorkIDs(&anime.WorkIDOptions{
		Fetcher: workids.NewClient(&workids.Options{
			HTTPClient: workids.NewHTTPClient(deps.Network, netactivity.PurposeAnimeWorkIDs), AnnictBaseURL: deps.Config.AnimeRecords.AnnictURL,
			AniListBaseURL: deps.Config.AnimeRecords.AniListURL, JikanBaseURL: deps.Config.AnimeRecords.JikanURL,
		}),
		Tokens: records, AnnictInterval: anime.DefaultAnnictRequestInterval, AniListInterval: anime.DefaultAniListRequestInterval,
	})
}

// newAnimeMatcher は、アニメと特撮の番組を、しょぼいカレンダーで作品と話に結びつける部品を作る。
func newAnimeMatcher(pool *pgxpool.Pool, clk clock.Clock, syobocalClient *syobocal.Client, logger animeLogger) *anime.Matcher {
	return anime.NewMatcher(pool, clk, syobocalClient, logger.Logger)
}

// newNicologMatcher は、録画を、上映会のコメント (NICOLOG) と結びつける部品を作る (#1363)。
func newNicologMatcher(deps *Deps, pool *pgxpool.Pool, clk clock.Clock, logger animeLogger) *anime.NicologMatcher {
	return anime.NewNicologMatcher(pool, clk, nicolog.NewClient(&nicolog.Options{HTTPClient: nicolog.NewHTTPClient(deps.Network), URL: ""}),
		anime.DefaultNicologRequestInterval, logger.Logger)
}

package animerecord

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/workids"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

const (
	// ArtworkRefreshInterval は、高画質の画像を引き直すまでの長さ (#1315)。画像は、めったに変わらない。
	ArtworkRefreshInterval = 30 * 24 * time.Hour
	// maxArtworkTargets は、1 回に高画質の画像を引く作品の数の上限。AniList の 1 回の問い合わせに収まる数にする。
	maxArtworkTargets = workids.AniListBatchSize
	// maxJikanRequests は、1 回に Jikan へ問い合わせる作品の数の上限。Jikan の公開の API は、1 分に 60 回までなので、控えめにする。
	maxJikanRequests = 10
)

// ArtworkSource は、MyAnimeList の作品 ID から、作品の高画質の画像を引く (#1315)。*workids.Client が満たす。
type ArtworkSource interface {
	AniListCoversByMAL(ctx context.Context, malIDs []int32) (map[int32]string, error)
	JikanImage(ctx context.Context, malID int32) (string, error)
}

var _ ArtworkSource = new(workids.Client)

// RefreshArtworks は、見たいリストの Annict の作品のうち、高画質の画像をまだ引いていない作品の画像を引き、見たいリストの画像にする (#1315)。
// Annict の画像は、作品の公式サイトの小さな画像のことが多いので、AniList の coverImage.extraLarge を先に引き、
// AniList にない作品は、Jikan (MyAnimeList) の大きい画像を引く。MyAnimeList の作品 ID は、作品 ID の対応 (anime_works) から引く。
// 管理者が連携を無効にしているときと、画像を引く先がないときは、何もしない。引けなかった作品は、次の機会に引き直す。
func (s *Service) RefreshArtworks(ctx context.Context, db Database) error {
	if s.artworks == nil {
		return nil
	}
	system, err := settings.Load(ctx, db)
	if err != nil {
		return err
	}
	if !system.AnimeLinkingEnabled {
		return nil
	}
	q := sqlc.New(db)
	targets, err := q.ListAnimeArtworkTargets(ctx, sqlc.ListAnimeArtworkTargetsParams{
		StaleBefore: s.clock.Now().Add(-ArtworkRefreshInterval), MaxCount: maxArtworkTargets,
	})
	if err != nil {
		return fmt.Errorf("failed to list the works to fetch the artworks: %w", err)
	}
	if len(targets) == 0 {
		return nil
	}
	covers, err := s.artworks.AniListCoversByMAL(ctx, targets)
	if err != nil {
		return fmt.Errorf("failed to fetch the artworks from AniList: %w", err)
	}
	jikan := &jikanTurns{left: maxJikanRequests, used: 0}
	for _, malID := range targets {
		url, found := covers[malID]
		if !found {
			var looked bool
			if url, looked, err = s.lookUpJikan(ctx, jikan, malID); err != nil {
				return err
			}
			if !looked {
				continue
			}
		}
		var imageURL *string
		if url != "" {
			imageURL = &url
		}
		if err := q.UpsertAnimeArtwork(ctx, sqlc.UpsertAnimeArtworkParams{MalID: malID, ImageURL: imageURL, Now: s.clock.Now()}); err != nil {
			return fmt.Errorf("failed to save the artwork: %w", err)
		}
	}
	if err := q.ApplyAnimeArtworks(ctx); err != nil {
		return fmt.Errorf("failed to apply the artworks to the wishlists: %w", err)
	}

	return nil
}

// jikanTurns は、1 回の RefreshArtworks で Jikan に問い合わせられる残りの数と、問い合わせた数。
type jikanTurns struct {
	left int
	used int
}

// lookUpJikan は、Jikan から MyAnimeList の作品 malID の画像を引く。画像がないか、作品がなければ空を返す。
// 問い合わせられる数を使い切っていれば、引かずに false を返す。Jikan に届かないか断られたら、残りは次の機会に回し、false を返す。
// 問い合わせの間は、見たいリストの問い合わせと同じだけ空ける。ctx が終わったときだけ、エラーを返す。
func (s *Service) lookUpJikan(ctx context.Context, turns *jikanTurns, malID int32) (string, bool, error) {
	if turns.left <= 0 {
		return "", false, nil
	}
	if turns.used > 0 {
		if err := sleep(ctx, s.wishlistInterval); err != nil {
			return "", false, err
		}
	}
	turns.left--
	turns.used++
	url, err := s.artworks.JikanImage(ctx, malID)
	switch {
	case err == nil:
		return url, true, nil
	case errors.Is(err, workids.ErrNotFound):
		return "", true, nil
	case ctx.Err() != nil:
		return "", false, fmt.Errorf("canceled while fetching the artwork: %w", ctx.Err())
	default:
		s.logger.WarnContext(ctx, "failed to fetch the artwork from Jikan", slog.Int("mal_id", int(malID)), slog.Any("error", err))
		turns.left = 0

		return "", false, nil
	}
}

// sleep は、ctx が終わるか、d が過ぎるまで待つ。間を空けるのは実際の時間なので、壁の時計で測る。
func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("canceled while waiting to fetch the artwork: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

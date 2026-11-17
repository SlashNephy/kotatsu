package graph

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/filelib"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

const (
	// targetLibraryArtifacts は、ライブラリのファイルの生成物を片付けたときの、監査ログの対象の種類。対象の ID は持たない。
	targetLibraryArtifacts = "library_artifacts"
	// actionLibraryArtifactsClear は、ライブラリのファイルの生成物を片付けたときの、監査ログの種類 (#1896)。
	actionLibraryArtifactsClear = "library_artifacts.clear"
)

// libraryArtifacts は、データディレクトリの、ライブラリのファイルの生成物の量を返す。
func (r *Resolver) libraryArtifacts(ctx context.Context) (*model.LibraryArtifacts, error) {
	if err := administer(ctx); err != nil {
		return nil, err
	}
	usage, err := r.librarySource.Indexer().Artifacts()
	if errors.Is(err, filelib.ErrNoDataDir) {
		return nil, NewError(CodeConflict, "the data directory is not configured")
	}
	if err != nil {
		return nil, err
	}

	return &model.LibraryArtifacts{Size: usage.Size, Files: usage.Files}, nil
}

// clearLibraryArtifacts は、データディレクトリの、ライブラリのファイルの生成物を丸ごと消す (設計書 5.8、#1896)。
// 同じデータディレクトリをマウントした tenban が消す。nekonote のジョブにしないのは、この tenban が作っている途中の索引を、
// 取りやめてから消すためである。まだ動いていないライブラリのジョブの行も消す。
// 家族の中で完結する管理の操作なので、再認証は求めない (設計書 9.5)。
func (r *Resolver) clearLibraryArtifacts(ctx context.Context) (*model.LibraryArtifacts, error) {
	_, actor, err := r.administratorActor(ctx, "only administrators can clear the library artifacts")
	if err != nil {
		return nil, err
	}
	indexer := r.librarySource.Indexer()
	var usage *filelib.Usage
	// 監査ログを書けなければ消さない。
	err = pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		var err error
		usage, err = indexer.ClearArtifacts(ctx, func(u *filelib.Usage) error {
			jobs, err := filelib.DeleteWaitingJobs(ctx, tx)
			if err != nil {
				return err
			}

			return r.recorder.Record(ctx, tx, &audit.Event{
				Actor: actor, Action: actionLibraryArtifactsClear, TargetType: targetLibraryArtifacts, TargetID: "",
				Summary:  map[string]any{"size": u.Size, "files": u.Files, "jobs": jobs},
				SourceIP: netip.Addr{}, UserAgent: "",
			})
		})

		return err
	})
	switch {
	case errors.Is(err, filelib.ErrNoDataDir):
		return nil, NewError(CodeConflict, "the data directory is not configured")
	case errors.Is(err, filelib.ErrClearing):
		return nil, NewError(CodeConflict, "the library artifacts are being cleared")
	case err != nil:
		return nil, err
	}
	// 置き場は移したので、使われない。消しきれなければ、次に片付けるときに消す。
	if err := indexer.PurgeDetached(); err != nil {
		r.logger.WarnContext(ctx, "failed to remove the detached library artifacts", slog.Any("error", err))
	}

	return &model.LibraryArtifacts{Size: usage.Size, Files: usage.Files}, nil
}

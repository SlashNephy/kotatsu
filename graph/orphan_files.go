package graph

import (
	"context"
	"fmt"

	"github.com/SlashNephy/kotatsu/graph/model"
	"github.com/SlashNephy/kotatsu/internal/library"
)

// orphanFiles は、DB にないファイルを返す。
func (r *Resolver) orphanFiles(ctx context.Context) ([]*model.OrphanFile, error) {
	if err := administer(ctx); err != nil {
		return nil, err
	}
	orphans, err := r.library.FindOrphans(ctx, r.db)
	if err != nil {
		return nil, err
	}
	result := make([]*model.OrphanFile, 0, len(orphans))
	for _, orphan := range orphans {
		result = append(result, &model.OrphanFile{
			PoolID: orphan.PoolID, Path: orphan.Entry.Path, Sidecar: orphan.Entry.Sidecar, Size: orphan.Entry.Size, ModifiedAt: orphan.Entry.ModifiedAt,
		})
	}

	return result, nil
}

// deleteOrphanFiles は、DB にないファイルを確かめ直して消す。
func (r *Resolver) deleteOrphanFiles(ctx context.Context, files []*model.OrphanFileInput) (*model.DeleteOrphanFilesPayload, error) {
	actor, err := administrator(ctx)
	if err != nil {
		return nil, err
	}
	if len(files) > maxCleanUpRecordings {
		return nil, NewError(CodeBadUserInput, fmt.Sprintf("files must be at most %d", maxCleanUpRecordings))
	}
	targets := make([]*library.OrphanTarget, 0, len(files))
	for _, file := range files {
		targets = append(targets, &library.OrphanTarget{PoolID: file.PoolID, Path: file.Path, Sidecar: file.Sidecar})
	}
	result, err := r.library.DeleteOrphans(ctx, r.db, actor, targets)
	if err != nil {
		return nil, err
	}

	return &model.DeleteOrphanFilesPayload{Deleted: result.Deleted, Skipped: result.Skipped}, nil
}

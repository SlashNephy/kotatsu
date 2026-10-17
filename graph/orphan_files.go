package graph

import (
	"context"
	"errors"
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
	payload := &model.DeleteOrphanFilesPayload{Deleted: 0, Skipped: 0}
	for _, file := range files {
		err := r.library.DeleteOrphan(ctx, r.db, actor, file.PoolID, file.Path, file.Sidecar)
		switch {
		case errors.Is(err, library.ErrNotOrphan), errors.Is(err, library.ErrNotFound):
			payload.Skipped++
		case err != nil:
			return nil, err
		default:
			payload.Deleted++
		}
	}

	return payload, nil
}

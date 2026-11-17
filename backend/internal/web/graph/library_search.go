package graph

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/SlashNephy/kotatsu/internal/infra/medialib"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

const (
	// librarySearchLimit は、ライブラリの名前の検索で返す件数の上限。
	librarySearchLimit = 100
	// maxLibrarySearchRunes は、ライブラリの名前の検索の語の長さの上限。
	maxLibrarySearchRunes = 100
)

// librarySearch は、ライブラリ name の、ディレクトリ dir の下から、名前に query を含む再生できるファイルを返す (#1902)。
func (r *Resolver) librarySearch(ctx context.Context, name, dir, query string) (*model.LibrarySearchResult, error) {
	if err := requireWatch(ctx); err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	if query == "" || utf8.RuneCountInString(query) > maxLibrarySearchRunes {
		return nil, NewError(CodeBadUserInput, "the query must be 1 to 100 characters")
	}
	l, ok := r.libraries.Get(name)
	if !ok {
		return nil, NewError(CodeNotFound, "library not found")
	}
	searcher, ok := l.(medialib.Searcher)
	if !ok {
		return nil, NewError(CodeBadUserInput, "the library cannot be searched")
	}
	entries, truncated, err := searcher.Search(ctx, dir, query, librarySearchLimit)
	if err != nil {
		return nil, libraryReadError(err)
	}
	result := &model.LibrarySearchResult{Entries: make([]*model.LibraryEntry, 0, len(entries)), Truncated: truncated}
	for _, entry := range entries {
		result.Entries = append(result.Entries, &model.LibraryEntry{
			Name: entry.Name, Path: entry.Path, Directory: false, Size: entry.Size, ModifiedAt: entry.ModifiedAt, Playable: true,
		})
	}

	return result, nil
}

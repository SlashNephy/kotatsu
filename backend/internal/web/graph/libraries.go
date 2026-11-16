package graph

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/medialib"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/usecase/fileimport"
	"github.com/SlashNephy/kotatsu/internal/usecase/filelib"
	"github.com/SlashNephy/kotatsu/internal/usecase/library"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

const (
	// libraryFileType は、ライブラリのファイルのグローバル ID の型名。
	libraryFileType = "LibraryFile"
	// libraryFileIDSeparator は、ライブラリのファイルのグローバル ID で、ファイルの ID (filelib.FileID) と、ライブラリの名前とパスを
	// 符号化したもの (filelib.Ref.Encode) を区切る文字。どちらにも現れない。
	libraryFileIDSeparator = "."
	// libraryStreamPrefix は、ライブラリのファイルの配信のパスの前置き。後ろに filelib.Ref.Encode を付ける。
	libraryStreamPrefix = "/internal/streams/library/"
)

// requireWatch は、ログイン中のユーザーが、録画を見られる (家族全員) ことを確かめる。ライブラリは録画と同じ権限で使う (設計書 5.8)。
func requireWatch(ctx context.Context) error {
	principal, err := viewer(ctx)
	if err != nil {
		return err
	}
	if !principal.Role.Can(domain.PermissionWatch) {
		return NewError(CodeForbidden, "watching is not allowed")
	}

	return nil
}

func toLibrary(l medialib.Library) *model.Library {
	kind := model.LibraryKindFolder
	if l.Kind() == medialib.KindPool {
		kind = model.LibraryKindPool
	}

	return &model.Library{Name: l.Name(), Kind: kind}
}

func (r *Resolver) librariesList(ctx context.Context) ([]*model.Library, error) {
	if err := requireWatch(ctx); err != nil {
		return nil, err
	}
	libraries := []*model.Library{}
	for _, l := range r.libraries.All() {
		libraries = append(libraries, toLibrary(l))
	}

	return libraries, nil
}

// libraryReadError は、ライブラリを読むときのエラーを、GraphQL のエラーにする。
func libraryReadError(err error) error {
	switch {
	case errors.Is(err, medialib.ErrInvalidPath):
		return NewError(CodeBadUserInput, "the path is outside the library")
	case errors.Is(err, medialib.ErrNotFound), errors.Is(err, filelib.ErrNotAFile):
		return NewError(CodeNotFound, "file not found")
	case errors.Is(err, fileimport.ErrBDAV):
		return NewError(CodeBadUserInput, "Blu-ray .m2ts files with 192-byte packets are not supported")
	case errors.Is(err, fileimport.ErrNotTS):
		return NewError(CodeBadUserInput, "the file is not an MPEG-2 TS")
	default:
		return fmt.Errorf("failed to read the library: %w", err)
	}
}

func (r *Resolver) libraryDirectory(ctx context.Context, name, dir string) (*model.LibraryDirectory, error) {
	if err := requireWatch(ctx); err != nil {
		return nil, err
	}
	l, ok := r.libraries.Get(name)
	if !ok {
		return nil, NewError(CodeNotFound, "library not found")
	}
	entries, err := l.List(ctx, dir)
	if err != nil {
		return nil, libraryReadError(err)
	}
	dirs, files := []*model.LibraryEntry{}, []*model.LibraryEntry{}
	for _, entry := range entries {
		item := &model.LibraryEntry{
			Name: entry.Name, Path: entry.Path, Directory: entry.Dir, Size: entry.Size, ModifiedAt: entry.ModifiedAt,
			Playable: !entry.Dir && medialib.Playable(entry.Name),
		}
		switch {
		case entry.Dir:
			dirs = append(dirs, item)
		case item.Playable:
			files = append(files, item)
		}
	}
	cleaned, _ := medialib.CleanPath(dir)

	return &model.LibraryDirectory{Library: toLibrary(l), Path: cleaned, Entries: slices.Concat(dirs, files)}, nil
}

func (r *Resolver) libraryFile(ctx context.Context, name, path string) (*model.LibraryFile, error) {
	if err := requireWatch(ctx); err != nil {
		return nil, err
	}
	if _, err := medialib.CleanPath(path); err != nil {
		return nil, libraryReadError(err)
	}

	return r.openLibraryFile(ctx, filelib.Ref{Library: name, Path: path})
}

// openLibraryFile は、ファイル ref を開き、GraphQL の形にする。開くと、索引を作り終えていなければ作り始める。
func (r *Resolver) openLibraryFile(ctx context.Context, ref filelib.Ref) (*model.LibraryFile, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	file, err := r.librarySource.Open(ctx, ref)
	if err != nil {
		return nil, libraryReadError(err)
	}
	video := file.Index.Video
	if video == nil {
		video, err = r.probeLibraryVideo(ctx, file)
		if err != nil && !errors.Is(err, filelib.ErrNoVideo) {
			return nil, libraryReadError(err)
		}
	}
	index := &model.LibraryFileIndex{Complete: file.Index.Complete, Writing: file.Writing, SeekableSeconds: 0, DurationSeconds: nil}
	if seekable, duration, ok := file.Timeline(); ok {
		index.SeekableSeconds = seekable.Seconds()
		index.DurationSeconds = new(duration.Seconds())
	}
	var watched *int64
	if position, ok, err := library.FileProgress(ctx, r.db, principal.UserID, file.ID()); err != nil {
		return nil, err
	} else if ok {
		watched = new(position.Milliseconds())
	}
	encoded := ref.Encode()

	return &model.LibraryFile{
		ID: GlobalID(libraryFileType, file.ID()+libraryFileIDSeparator+encoded), Library: toLibrary(file.Library), Path: ref.Path, Name: file.Entry.Name,
		Size: file.Entry.Size, ModifiedAt: file.Entry.ModifiedAt, VideoFormat: libraryVideoFormat(video), Index: index,
		WatchPositionMs: watched, StreamURL: libraryStreamPrefix + encoded, HlsURL: libraryStreamPrefix + encoded,
	}, nil
}

// probeLibraryVideo は、索引の記録にまだ映像の形式がなければ、ファイルの頭から読む。読めなければ filelib.ErrNoVideo を返す。
func (r *Resolver) probeLibraryVideo(ctx context.Context, file *filelib.File) (*ts.VideoFormat, error) {
	f, err := file.Library.Open(ctx, file.Ref.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return filelib.ProbeVideo(f, file.Entry.Size)
}

// libraryVideoFormat は、ファイルから読んだ映像の形式から、映像の種類を決める。ブラウザ内での変換は MPEG-2 だけを変換するので、
// MPEG-2 でない映像は、BS4K と同じく UHD にして、kyusu に回す。4320p は SHV にする。読めなければ HD とみなす。
func libraryVideoFormat(video *ts.VideoFormat) model.VideoFormat {
	switch {
	case video == nil || video.Codec == "mpeg2":
		return model.VideoFormatHd
	case video.Height >= 4320:
		return model.VideoFormatShv
	default:
		return model.VideoFormatUhd
	}
}

func (r *Resolver) saveLibraryWatchPosition(ctx context.Context, globalID string, positionMs int64) (*model.LibraryFile, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	if !principal.Role.Can(domain.PermissionWatch) {
		return nil, NewError(CodeForbidden, "watching is not allowed")
	}
	if positionMs < 0 {
		return nil, NewError(CodeBadUserInput, "the position must not be negative")
	}
	typeName, raw, err := ParseGlobalID(globalID)
	if err != nil || typeName != libraryFileType {
		return nil, NewError(CodeBadUserInput, "invalid library file ID")
	}
	fileID, encoded, _ := strings.Cut(raw, libraryFileIDSeparator)
	ref, ok := filelib.DecodeRef(encoded)
	if !ok {
		return nil, NewError(CodeBadUserInput, "invalid library file ID")
	}
	file, err := r.librarySource.Open(ctx, ref)
	if err != nil {
		return nil, libraryReadError(err)
	}
	// 見ている間にファイルを書き換えたら、前のファイルの再生位置を、書き換えた後のファイルのものとして残さない。
	if file.ID() == fileID {
		err := r.library.SaveFileProgress(ctx, r.db, principal.UserID, file.ID(), time.Duration(positionMs)*time.Millisecond)
		if err != nil {
			return nil, libraryError(err)
		}
	}

	return r.openLibraryFile(ctx, ref)
}

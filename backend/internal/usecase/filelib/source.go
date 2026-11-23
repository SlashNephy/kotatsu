package filelib

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/SlashNephy/kotatsu/internal/infra/medialib"
	"github.com/SlashNephy/kotatsu/internal/usecase/stream"
)

// ErrNotAFile は、パスが再生できるファイル (拡張子が .ts、.m2ts、.mts のもの) でないことを表す。
var ErrNotAFile = errors.New("the path is not a playable TS file")

// Source は、ライブラリのファイルを、録画の再生と同じく、インデックスを使って指定した位置から流す (設計書 7.2)。
// tenban の配信と、kyusu の仮想 HLS が使う。
type Source struct {
	libraries *medialib.Set
	indexer   *Indexer
}

// NewSource は Source を返す。indexer がインデックスを作るなら (tenban)、開くたびに、インデックスができていなければ作り始める。
func NewSource(libraries *medialib.Set, indexer *Indexer) *Source {
	return &Source{libraries: libraries, indexer: indexer}
}

// Indexer は、インデックスと画像を読み書きする Indexer を返す。
func (s *Source) Indexer() *Indexer {
	return s.indexer
}

// File は、開いたライブラリのファイル。
type File struct {
	Ref   Ref
	Entry *medialib.Entry
	// Library は、ファイルのあるライブラリ。
	Library medialib.Library
	// Index は、開いた時点のインデックス。
	Index *Index
	// Writing は、書いている途中とみなして、インデックスを作らないかどうか。
	Writing bool
	// Recording は、インデックスを使って流すもの。
	Recording *stream.Recording
}

// Open は、ファイル ref を開く。ライブラリかファイルがなければ medialib.ErrNotFound を、再生できるファイルでなければ ErrNotAFile を返す。
// インデックスがまだできていなければ、作り始める (tenban のとき)。
func (s *Source) Open(ctx context.Context, ref Ref) (*File, error) {
	library, ok := s.libraries.Get(ref.Library)
	if !ok {
		return nil, fmt.Errorf("%w: library %s", medialib.ErrNotFound, ref.Library)
	}
	// Google Drive では、パスはファイルの ID なので、拡張子は情報の名前で見る。
	if _, ok := library.(medialib.Identifier); !ok && !medialib.Playable(ref.Path) {
		return nil, ErrNotAFile
	}
	entry, err := library.Stat(ctx, ref.Path)
	if err != nil {
		return nil, err
	}
	if entry.Dir || !medialib.Playable(entry.Name) {
		return nil, ErrNotAFile
	}
	s.indexer.Ensure(ctx, ref, entry)
	index, err := s.indexer.Read(ref, entry)
	if err != nil {
		return nil, err
	}
	open := func(ctx context.Context) (stream.File, error) {
		return library.Open(ctx, ref.Path)
	}

	return &File{
		Ref: ref, Entry: entry, Library: library, Index: index, Writing: !index.Complete && s.indexer.Writing(entry),
		Recording: stream.OpenFile(entry.Name, open, index.Entries, !index.Complete),
	}, nil
}

// OpenEncoded は、Ref.Encode で作った文字列のファイルを開く。kyusu の仮想 HLS が、パスの ID から開くのに使う。
// 読めない文字列なら medialib.ErrNotFound を返す。
func (s *Source) OpenEncoded(ctx context.Context, id string) (*stream.Recording, error) {
	ref, ok := DecodeRef(id)
	if !ok {
		return nil, fmt.Errorf("%w: invalid file ID", medialib.ErrNotFound)
	}
	file, err := s.Open(ctx, ref)
	if err != nil {
		return nil, err
	}

	return file.Recording, nil
}

// Timeline は、再生できる長さを返す。seekable は、インデックスのできた所 (最後の I フレームの位置) で、ここまでシークできる。
// duration は、ファイル全体の再生の時間。インデックスを作り終えていれば、録画と同じく求め、作っている途中なら、インデックスのできた所までの
// バイト数と時間の比から見込む。インデックスがまだなければ、どちらも分からないので ok を false にする。
func (f *File) Timeline() (time.Duration, time.Duration, bool) {
	keyframes := f.Recording.Keyframes()
	if len(f.Index.Entries) == 0 || len(keyframes) == 0 {
		return 0, 0, false
	}
	seekable := keyframes[len(keyframes)-1]
	if f.Index.Complete {
		if duration, ok := f.Recording.Duration(); ok {
			return duration, duration, true
		}

		return seekable, seekable, true
	}
	last := f.Index.Entries[len(f.Index.Entries)-1].Offset
	if last <= 0 {
		return seekable, seekable, true
	}

	return seekable, max(seekable, time.Duration(float64(seekable)*float64(f.Entry.Size)/float64(last))), true
}

// ID は、再生位置を保存するときのファイルの ID。Google Drive では、ライブラリの名前とパスでなく、ファイルの ID から作る
// (medialib.IdentityOf)。ライブラリの名前やパスの形を変えても、再生位置を引き継ぐ。
func (f *File) ID() string {
	namespace, identity := medialib.IdentityOf(f.Library, f.Ref.Path)

	return FileID(Ref{Library: namespace, Path: identity}, f.Entry.Size, f.Entry.ModifiedAt)
}

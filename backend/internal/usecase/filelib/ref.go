// Package filelib は、録画として登録していない TS を、ライブラリ (medialib) から選んで、録画と同じ経路で再生する (設計書 5.8、#1734)。
//
// ファイルは、ライブラリの名前と、ライブラリの中のパスで指す (Ref)。シークのインデックスは、初めて開いたときに tenban が裏で作り (Indexer)、
// データディレクトリに置いて使い回す。ファイルの大きさか更新日時が変われば作り直す。
package filelib

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/SlashNephy/kotatsu/internal/infra/medialib"
)

// Ref は、ライブラリの中のファイル。
type Ref struct {
	// Library は、ライブラリの名前。
	Library string
	// Path は、ライブラリの中の / 区切りの相対パス。
	Path string
}

// refSeparator は、Encode で、ライブラリの名前とパスを区切る文字。どちらにも現れない。
const refSeparator = "\x00"

// Encode は、Ref を、URL のパスに置ける 1 つの文字列にする。配信と kyusu の仮想 HLS の URL、GraphQL の ID に使う。
// HLS のセグメントは、プレイリストからの相対の URL で取得するので、パスにスラッシュを含めない。
func (r Ref) Encode() string {
	return base64.RawURLEncoding.EncodeToString([]byte(r.Library + refSeparator + r.Path))
}

// DecodeRef は、Encode で作った文字列を Ref に戻す。読めないか、パスがライブラリの中の正規のファイルのパスでなければ false を返す。
func DecodeRef(s string) (Ref, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Ref{Library: "", Path: ""}, false
	}
	library, p, ok := strings.Cut(string(raw), refSeparator)
	if !ok || library == "" {
		return Ref{Library: "", Path: ""}, false
	}
	cleaned, err := medialib.CleanPath(p)
	if err != nil || cleaned == "" || cleaned != p {
		return Ref{Library: "", Path: ""}, false
	}

	return Ref{Library: library, Path: p}, true
}

// FileID は、ファイルを見分ける ID。ライブラリの名前、パス、大きさ、更新日時から作るので、ファイルを書き換えれば別のファイルになる。
// ユーザーごとの再生位置を、この ID ごとに保存する。
func FileID(ref Ref, size int64, modifiedAt time.Time) string {
	sum := sha256.Sum256([]byte(ref.Library + refSeparator + ref.Path + refSeparator +
		strconv.FormatInt(size, 10) + refSeparator + strconv.FormatInt(modifiedAt.UnixNano(), 10)))

	return hex.EncodeToString(sum[:16])
}

// indexKey は、データディレクトリに置くインデックスのディレクトリの名前。ライブラリの名前とパスから作り、大きさと更新日時は含めない。
// ファイルを書き換えたら、同じディレクトリのインデックスを作り直して置き換えるので、古いインデックスが溜まらない。
func indexKey(ref Ref) string {
	sum := sha256.Sum256([]byte(ref.Library + refSeparator + ref.Path))

	return hex.EncodeToString(sum[:16])
}

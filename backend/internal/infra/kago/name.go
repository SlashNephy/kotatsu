package kago

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"unicode"
	"unicode/utf8"
)

const (
	// maxNameBytes は、多くのファイルシステムでの、名前 1 つあたりの長さの上限。
	maxNameBytes = 255
	// maxFileNameBytes は、録画ファイルの名前の長さの上限。
	maxFileNameBytes = maxNameBytes
	// maxSequence は、同じ名前のファイルがあるときに付ける連番の上限。
	maxSequence = 10_000
	// fallbackName は、整えた結果が空になったときの名前。
	fallbackName = "untitled"
	// maxExtBytes は、拡張子の長さの上限。
	maxExtBytes = 16
	// trimmedChars は、名前の前後から取り除く文字 (空白、ドット、全角空白)。
	trimmedChars = " .\u3000"
	// windowsUnsafeChars は、Windows でファイル名に使えないので、取り除く記号。
	windowsUnsafeChars = `:*?"<>|`
	// maxDirDepth は、ファイル名のテンプレートで掘れるディレクトリの深さの上限。これより深い分は捨てる。
	maxDirDepth = 8
)

// RenderName は、ファイル名のテンプレートに data を当てはめ、ファイル名として使えるように整えて ext を付け、
// プール内の相対パスを返す。ext はドットを含めて渡す (例: ".m2ts")。
//
// 結果の / はディレクトリの区切りとして扱い (例: GR/2026-10/番組名.m2ts)、ディレクトリの名前の 1 つずつも、ファイル名と同じく整える。
// 整えて空になった要素 (空の要素、.、..) は捨てるので、プールの外は指さない。maxDirDepth より深い分も捨てる。
func RenderName(tmpl *template.Template, data any, ext string) (string, error) {
	if len(ext) > maxExtBytes || !strings.HasPrefix(ext, ".") || ext[1:] != SanitizeName(ext[1:]) {
		return "", fmt.Errorf("%w: invalid extension %q", ErrInvalidPath, ext)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&cappedWriter{buf: &buf}, data); err != nil {
		return "", fmt.Errorf("failed to render the file name: %w", err)
	}
	parts := strings.Split(buf.String(), "/")
	dirs := make([]string, 0, len(parts)-1)
	for _, part := range parts[:len(parts)-1] {
		if dir := sanitizeDirName(part); dir != "" && len(dirs) < maxDirDepth {
			dirs = append(dirs, dir)
		}
	}

	return path.Join(append(dirs, fitName(SanitizeName(parts[len(parts)-1]), ext, ""))...), nil
}

// sanitizeDirName は、ディレクトリの名前を、ファイル名と同じく整え、長さの上限に収める。整えて空になれば空を返す。
func sanitizeDirName(name string) string {
	return cutName(stripName(name), maxNameBytes)
}

// cutName は、name を limit バイト以内に、文字の境界で切り詰める。切った位置の前の空白とドットも取り除き、
// 予約された名前が残れば _ を付ける。
func cutName(name string, limit int) string {
	if len(name) <= limit {
		return name
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(name[cut]) {
		cut--
	}

	return escapeReservedName(strings.TrimRight(name[:cut], trimmedChars))
}

// SanitizeName は、ファイル名として使えない文字を取り除く。
//
//   - パス区切り (/ と \) と、制御文字を取り除く
//   - Windows で使えない記号 (: * ? " < > |) を取り除く。プールのファイルは SMB などでほかのツールからも見るため
//   - UTF-8 として正しくないバイトを取り除く
//   - 前後の空白とドットを取り除く (先頭のドットは隠しファイルになり、末尾のドットは SMB で扱えないため)
//   - Windows が予約している名前 (CON、NUL、COM1 など) なら、最初のドットの前に _ を付ける
//   - 何も残らなければ "untitled" にする
func SanitizeName(name string) string {
	if name = stripName(name); name == "" {
		return fallbackName
	}

	return name
}

// stripName は、SanitizeName と同じく整え、何も残らなければ空を返す。
func stripName(name string) string {
	name = strings.ToValidUTF8(name, "")
	name = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || unicode.IsControl(r) || strings.ContainsRune(windowsUnsafeChars, r) {
			return -1
		}

		return r
	}, name)

	return escapeReservedName(strings.Trim(name, trimmedChars))
}

// escapeReservedName は、Windows が装置の名前として予約している名前 (CON、NUL、COM1 など) なら、最初のドットの前に _ を付ける。
// Windows では、予約された名前は、拡張子を付けてもファイルの名前にもディレクトリの名前にも使えないため。
func escapeReservedName(name string) string {
	base, rest, hasExt := strings.Cut(name, ".")
	if !isReservedName(strings.TrimRight(base, " ")) {
		return name
	}
	if !hasExt {
		return base + "_"
	}

	return base + "_." + rest
}

// isReservedName は、name (拡張子と末尾の空白を除いたもの) が、Windows の予約された名前かを返す。大文字と小文字は区別しない。
func isReservedName(name string) bool {
	upper := strings.ToUpper(name)
	switch upper {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return true
	}
	for _, prefix := range []string{"COM", "LPT"} {
		if n, ok := strings.CutPrefix(upper, prefix); ok {
			// 数字のほか、上付きの ¹、²、³ も番号とみなされる。
			return len(n) == 1 && '0' <= n[0] && n[0] <= '9' || n == "¹" || n == "²" || n == "³"
		}
	}

	return false
}

// fitName は、base + suffix + ext が maxFileNameBytes 以内に収まるよう、base を文字の境界で切り詰める。
func fitName(base, ext, suffix string) string {
	limit := max(maxFileNameBytes-len(ext)-len(suffix), 0)
	if len(base) > limit {
		cut := limit
		for cut > 0 && !utf8.RuneStart(base[cut]) {
			cut--
		}
		// 切った位置の前に空白やドットが残ると、SanitizeName を通した名前と食い違うので取り除く。
		// 取り除いて予約された名前が残ったときも、SanitizeName と同じく _ を付ける。
		base = escapeReservedName(strings.TrimRight(base[:cut], trimmedChars))
		if base == "" {
			base = fallbackName
		}
	}

	return base + suffix + ext
}

// Create は、プールの dir (プール内の相対パス) に、name のファイルを新しく作って開く。dir がなければ作る。
// 同じ名前のファイルがあれば、拡張子の前に " (2)"、" (3)" のような連番を付ける。
// ファイルは O_EXCL で作るので、複数のプロセスが同時に同じ名前で作っても、別々のファイルになる。
// 開いたファイルと、そのプール内の相対パス (/ 区切り) を返す。
func (s *Store) Create(poolID, dir, name string) (*os.File, string, error) {
	if name != SanitizeName(name) || len(name) > maxFileNameBytes {
		return nil, "", fmt.Errorf("%w: file name %q must be sanitized and fitted", ErrInvalidPath, name)
	}

	if dir != "" {
		if err := s.mkdirAll(poolID, dir); err != nil {
			return nil, "", err
		}
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for n := 1; n <= maxSequence; n++ {
		candidate := name
		if n > 1 {
			candidate = fitName(base, ext, " ("+strconv.Itoa(n)+")")
		}
		rel := path.Join(dir, candidate)
		abs, err := s.Path(poolID, rel)
		if err != nil {
			return nil, "", err
		}

		f, err := os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return nil, "", fmt.Errorf("failed to create a file in pool %s: %w", poolID, err)
		}

		return f, rel, nil
	}

	return nil, "", fmt.Errorf("too many files named %q in pool %s", name, poolID)
}

// maxRenderedBytes は、テンプレートを当てはめた結果の長さの上限。ファイル名は maxNameBytes までなので、これで足りる。
const maxRenderedBytes = 4096

// errRenderedTooLong は、テンプレートを当てはめた結果が長すぎることを表す。
var errRenderedTooLong = errors.New("the rendered file name is too long")

// cappedWriter は、maxRenderedBytes を超えて書こうとしたら、書くのをやめてエラーにする。
type cappedWriter struct {
	buf *bytes.Buffer
}

var _ io.Writer = new(cappedWriter)

func (c *cappedWriter) Write(p []byte) (int, error) {
	if c.buf.Len()+len(p) > maxRenderedBytes {
		return 0, errRenderedTooLong
	}

	return c.buf.Write(p)
}

// mkdirAll は、プールの中に dir (プール内の相対パス) を作る。プールの中に限って作るので、シンボリックリンクでもプールの外には作らない。
func (s *Store) mkdirAll(poolID, dir string) error {
	mount, ok := s.mounts[poolID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrPoolNotMounted, poolID)
	}
	if !filepath.IsLocal(dir) {
		return fmt.Errorf("%w: %q", ErrInvalidPath, dir)
	}
	root, err := os.OpenRoot(mount)
	if err != nil {
		return fmt.Errorf("failed to open pool %s: %w", poolID, err)
	}
	defer root.Close()
	if err := root.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create a directory in pool %s: %w", poolID, err)
	}

	return nil
}

// SequencedName は、ファイル名 name の拡張子の前に " (n)" を付け、名前の上限に収める。n が 1 以下なら、そのまま返す。
func SequencedName(name string, n int) string {
	if n <= 1 {
		return name
	}
	ext := filepath.Ext(name)

	return fitName(strings.TrimSuffix(name, ext), ext, " ("+strconv.Itoa(n)+")")
}

// Rename は、プールの中で、録画ファイル from (プール内の相対パス) を dir/name に移す。生成物は、ファイルの ID で置くので移さない。
// name には連番を付ける前の名前を渡し、start 番目の連番 (1 なら連番なし) から試す。移す先に同じ名前のものがあれば、
// 次の連番を試す。from のあったディレクトリが空になれば消す。
// プールの中に限って移すので、シンボリックリンクでプールの外には移さない。移した先の相対パス (/ 区切り) を返す。
func (s *Store) Rename(poolID, from, dir, name string, start int) (string, error) {
	if name != SanitizeName(name) || len(name) > maxFileNameBytes || !filepath.IsLocal(from) || (dir != "" && !filepath.IsLocal(dir)) {
		return "", fmt.Errorf("%w: %q -> %q/%q", ErrInvalidPath, from, dir, name)
	}
	mount, ok := s.mounts[poolID]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrPoolNotMounted, poolID)
	}
	root, err := os.OpenRoot(mount)
	if err != nil {
		return "", fmt.Errorf("failed to open pool %s: %w", poolID, err)
	}
	defer root.Close()
	if dir != "" {
		if err := root.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("failed to create a directory in pool %s: %w", poolID, err)
		}
	}
	for n := max(start, 1); n <= maxSequence; n++ {
		rel := path.Join(dir, SequencedName(name, n))
		if rel == filepath.ToSlash(filepath.Clean(from)) {
			return rel, nil
		}
		// 大文字と小文字や正規化だけを変えるときは、移す先が自分自身として見つかる。ふさがっているとはみなさない。
		if exists(root, rel) && !sameFile(root, from, rel) {
			continue
		}
		if err := root.Rename(from, rel); err != nil {
			return "", fmt.Errorf("failed to rename a file in pool %s: %w", poolID, err)
		}
		for parent := filepath.Dir(from); parent != "." && root.Remove(parent) == nil; parent = filepath.Dir(parent) {
			continue
		}

		return rel, nil
	}

	return "", fmt.Errorf("too many files named %q in pool %s", name, poolID)
}

// Restore は、Rename で移した録画ファイル from (プール内の相対パス) を、元のパス to に戻す。
// to は、移す前に実際にあったパスなので、今の整え方で使えない文字 (設計書 4.7) を含んでいても、整え直さずに戻す。
// to のディレクトリが空になって消えていれば作り直す。to に別のものがあれば、上書きせずに失敗する。
// from のあったディレクトリが空になれば消す。プールの中に限って移す。
func (s *Store) Restore(poolID, from, to string) error {
	if !filepath.IsLocal(from) || !filepath.IsLocal(to) {
		return fmt.Errorf("%w: %q -> %q", ErrInvalidPath, from, to)
	}
	if filepath.Clean(from) == filepath.Clean(to) {
		return nil
	}
	mount, ok := s.mounts[poolID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrPoolNotMounted, poolID)
	}
	root, err := os.OpenRoot(mount)
	if err != nil {
		return fmt.Errorf("failed to open pool %s: %w", poolID, err)
	}
	defer root.Close()
	if dir := filepath.Dir(to); dir != "." {
		if err := root.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create a directory in pool %s: %w", poolID, err)
		}
	}
	// 大文字と小文字や正規化だけを変えた付け直しを戻すときは、元のパスが自分自身として見つかる。ふさがっているとはみなさない。
	if exists(root, to) && !sameFile(root, from, to) {
		return fmt.Errorf("failed to restore a file in pool %s: %q already exists", poolID, to)
	}
	if err := root.Rename(from, to); err != nil {
		return fmt.Errorf("failed to restore a file in pool %s: %w", poolID, err)
	}
	for parent := filepath.Dir(from); parent != "." && root.Remove(parent) == nil; parent = filepath.Dir(parent) {
		continue
	}

	return nil
}

// sameFile は、root の中の a と b が、同じファイルを指すかを返す。シンボリックリンクはたどらない。
func sameFile(root *os.Root, a, b string) bool {
	infoA, err := root.Lstat(a)
	if err != nil {
		return false
	}
	infoB, err := root.Lstat(b)

	return err == nil && os.SameFile(infoA, infoB)
}

// exists は、root の中に rel があるかを返す。シンボリックリンクはたどらない。
func exists(root *os.Root, rel string) bool {
	_, err := root.Lstat(rel)

	return err == nil
}

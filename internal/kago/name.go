package kago

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
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
	// maxFileNameBytes は、録画ファイルの名前の長さの上限。付属ファイル用のディレクトリの名前 (名前 + .kotatsu) も
	// 上限に収まるよう、その分を空けておく。
	maxFileNameBytes = maxNameBytes - len(sidecarSuffix)
	// maxSequence は、同じ名前のファイルがあるときに付ける連番の上限。
	maxSequence = 10_000
	// fallbackName は、整えた結果が空になったときの名前。
	fallbackName = "untitled"
	// maxExtBytes は、拡張子の長さの上限。
	maxExtBytes = 16
	// trimmedChars は、名前の前後から取り除く文字 (空白、ドット、全角空白)。
	trimmedChars = " .\u3000"
)

// RenderName は、ファイル名のテンプレートに data を当てはめ、ファイル名として使えるように整えて ext を付ける。
// ext はドットを含めて渡す (例: ".m2ts")。
func RenderName(tmpl *template.Template, data any, ext string) (string, error) {
	if len(ext) > maxExtBytes || !strings.HasPrefix(ext, ".") || ext[1:] != SanitizeName(ext[1:]) {
		return "", fmt.Errorf("%w: invalid extension %q", ErrInvalidPath, ext)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&cappedWriter{buf: &buf}, data); err != nil {
		return "", fmt.Errorf("failed to render the file name: %w", err)
	}

	return fitName(SanitizeName(buf.String()), ext, ""), nil
}

// SanitizeName は、ファイル名として使えない文字を取り除く。
//
//   - パス区切り (/ と \) と、制御文字を取り除く
//   - UTF-8 として正しくないバイトを取り除く
//   - 前後の空白とドットを取り除く (先頭のドットは隠しファイルになり、末尾のドットは SMB で扱えないため)
//   - 何も残らなければ "untitled" にする
func SanitizeName(name string) string {
	name = strings.ToValidUTF8(name, "")
	name = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || unicode.IsControl(r) {
			return -1
		}

		return r
	}, name)
	name = strings.Trim(name, trimmedChars)
	if name == "" {
		return fallbackName
	}

	return name
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
		base = strings.TrimRight(base[:cut], trimmedChars)
		if base == "" {
			base = fallbackName
		}
	}

	return base + suffix + ext
}

// Create は、プールの dir (プール内の相対パス) に、name のファイルを新しく作って開く。
// 同じ名前のファイルがあれば、拡張子の前に " (2)"、" (3)" のような連番を付ける。
// ファイルは O_EXCL で作るので、複数のプロセスが同時に同じ名前で作っても、別々のファイルになる。
// 開いたファイルと、そのプール内の相対パスを返す。
func (s *Store) Create(poolID, dir, name string) (*os.File, string, error) {
	if name != SanitizeName(name) || len(name) > maxFileNameBytes {
		return nil, "", fmt.Errorf("%w: file name %q must be sanitized and fitted", ErrInvalidPath, name)
	}

	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for n := 1; n <= maxSequence; n++ {
		candidate := name
		if n > 1 {
			candidate = fitName(base, ext, " ("+strconv.Itoa(n)+")")
		}
		rel := filepath.Join(dir, candidate)
		path, err := s.Path(poolID, rel)
		if err != nil {
			return nil, "", err
		}

		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
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

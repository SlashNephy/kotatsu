// printlicenses は、本体 (cmd/kotatsu) に組み込む Go のモジュールと、そのライセンスの文書の一覧を JSON で書き出す (#1604)。
// 本体をビルドする前に、backend/ で次のように動かし、licenses パッケージに埋め込ませる。
//
//	go run ./internal/web/licenses/printlicenses -o internal/web/licenses/dist/go.json
//
// 組み込むモジュールは OS で変わる (macOS だけの purego など) ので、配布する OS ごとに go list で集めて合わせる。
// 動いているバイナリでは、licenses.Load が、そのバイナリに組み込まれたものに絞る。
// ライセンスの文書が見つからないモジュールがあれば、一覧を書かずに失敗する。
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/SlashNephy/kotatsu/internal/web/licenses"
)

// mainPackage は、一覧を作る本体のパッケージ。
const mainPackage = "./cmd/kotatsu"

// licenseFile は、モジュールの直下にある、ライセンスの文書と NOTICE のファイルの名前。
var licenseFile = regexp.MustCompile(`(?i)^(licen[cs]e|copying|notice)([.\-_].*)?$`)

// errNoLicense は、ライセンスの文書が見つからないことを表す。
var errNoLicense = errors.New("no license file found")

type module struct {
	path    string
	version string
	dir     string
}

func main() {
	output := flag.String("o", "", "the file to write the license list to (standard output if empty)")
	flag.Parse()
	if err := run(context.Background(), *output); err != nil {
		fmt.Fprintln(os.Stderr, "printlicenses:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, output string) error {
	modules, err := linkedModules(ctx)
	if err != nil {
		return err
	}
	list := &licenses.List{Modules: make([]*licenses.Module, 0, len(modules)+1)}
	std, err := standardLibrary(ctx)
	if err != nil {
		return err
	}
	list.Modules = append(list.Modules, std)
	var missing []string
	for _, m := range modules {
		files, err := readLicenseFiles(m.dir)
		if errors.Is(err, errNoLicense) {
			missing = append(missing, m.path+"@"+m.version)

			continue
		}
		if err != nil {
			return fmt.Errorf("%s: %w", m.path, err)
		}
		list.Modules = append(list.Modules, &licenses.Module{Path: m.path, Version: m.version, Files: files})
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w for %s", errNoLicense, strings.Join(missing, ", "))
	}

	encoded, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode the license list: %w", err)
	}
	encoded = append(encoded, '\n')
	if output == "" {
		_, err = os.Stdout.Write(encoded)

		return err
	}
	// 埋め込むのは同じユーザーの go build なので、ほかのユーザーには読ませない。
	if err := os.WriteFile(output, encoded, 0o600); err != nil {
		return fmt.Errorf("failed to write the license list: %w", err)
	}

	return nil
}

// linkedModules は、配布するどれかの OS で本体に組み込むモジュールを、パスの順に返す。本体のモジュールは除く。
func linkedModules(ctx context.Context) ([]*module, error) {
	found := map[string]*module{}
	// 本体を配布する OS (設計書 11.1、11.7)。
	for _, goos := range []string{"linux", "darwin", "windows"} {
		cmd := exec.CommandContext(ctx, "go", "list", "-deps",
			"-f", "{{with .Module}}{{if not .Main}}{{.Path}}\t{{.Version}}\t{{.Dir}}{{end}}{{end}}", mainPackage)
		cmd.Env = append(os.Environ(), "GOOS="+goos, "CGO_ENABLED=0")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("go list for %s failed: %w: %s", goos, err, stderr.String())
		}
		scanner := bufio.NewScanner(bytes.NewReader(out))
		for scanner.Scan() {
			fields := strings.Split(scanner.Text(), "\t")
			if len(fields) != 3 {
				continue
			}
			if fields[2] == "" {
				return nil, fmt.Errorf("the module %s@%s is not downloaded; run go mod download", fields[0], fields[1])
			}
			found[fields[0]] = &module{path: fields[0], version: fields[1], dir: fields[2]}
		}
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("failed to read the go list output: %w", err)
		}
	}
	modules := make([]*module, 0, len(found))
	for _, m := range found {
		modules = append(modules, m)
	}
	slices.SortFunc(modules, func(a, b *module) int { return strings.Compare(a.path, b.path) })

	return modules, nil
}

// standardLibrary は、Go の標準ライブラリとランタイムのライセンスを返す。どのバイナリにも組み込まれる。
func standardLibrary(ctx context.Context) (*licenses.Module, error) {
	out, err := exec.CommandContext(ctx, "go", "env", "GOROOT", "GOVERSION").Output()
	if err != nil {
		return nil, fmt.Errorf("go env failed: %w", err)
	}
	// Windows の go env は CRLF で区切るので、空白で分ける。GOROOT に空白を含む環境は扱わない。
	lines := strings.Fields(string(out))
	if len(lines) != 2 {
		return nil, fmt.Errorf("unexpected go env output: %q", out)
	}
	files, err := readLicenseFiles(lines[0])
	if err != nil {
		return nil, fmt.Errorf("the Go standard library: %w", err)
	}

	return &licenses.Module{Path: licenses.StandardLibraryPath, Version: lines[1], Files: files}, nil
}

// readLicenseFiles は、dir の直下にあるライセンスの文書と NOTICE を、名前の順に読む。
func readLicenseFiles(dir string) ([]*licenses.File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", dir, err)
	}
	var files []*licenses.File
	for _, entry := range entries {
		if entry.IsDir() || !licenseFile.MatchString(entry.Name()) {
			continue
		}
		text, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", entry.Name(), err)
		}
		files = append(files, &licenses.File{Name: entry.Name(), Text: string(text)})
	}
	if len(files) == 0 {
		return nil, errNoLicense
	}

	return files, nil
}

// Package sharedfile は、読んでいる間も、ほかの処理がファイルを消したり名前を変えたりできるように、ファイルを開く (#1096)。
//
// Windows の os.Open は、FILE_SHARE_DELETE を付けずに開くので、開いている間は、ほかから消すことも名前を変えることもできない。
// os.Root の操作は FILE_SHARE_DELETE を付けて開くので、ファイルのあるディレクトリを os.Root として開いて扱う。
// Linux と macOS では、os の同じ名前の関数と同じに動く。
//
// os.Root の中に限って開くので、ディレクトリの外を指すシンボリックリンクはたどらない。
package sharedfile

import (
	"os"
	"path/filepath"
)

// Open は、path のファイルを、読むために開く。
func Open(path string) (*os.File, error) {
	return os.OpenInRoot(filepath.Dir(path), filepath.Base(path))
}

// ReadFile は、path のファイルを読む。
func ReadFile(path string) ([]byte, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer root.Close()

	return root.ReadFile(filepath.Base(path))
}

// Rename は、oldpath を newpath に移す。newpath があれば置き換える。
// 同じディレクトリの中で移すときは、os.Root.Rename を使う。Windows では POSIX の意味の rename になり、newpath を
// FILE_SHARE_DELETE 付きで開いている読み手がいても置き換えられる (対応しないファイルシステムでは、os.Rename と同じ)。
func Rename(oldpath, newpath string) error {
	dir := filepath.Dir(newpath)
	if filepath.Dir(oldpath) != dir {
		return os.Rename(oldpath, newpath)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()

	return root.Rename(filepath.Base(oldpath), filepath.Base(newpath))
}

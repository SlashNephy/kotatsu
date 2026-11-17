package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"filippo.io/age"
)

// fileMode は、バックアップのファイルの許可。バックアップには秘密情報が入るので、持ち主だけが読めるようにする。
const backupFileMode = 0o600

// WriteFile は、Write の出力を path に書く。path と同じディレクトリの一時ファイルに書き、書き終えたら path に名前を変える。
// 途中で失敗したら、一時ファイルを消す。path が既にあれば、上書きせずに誤りにする。
// 一時ファイルの名前は . で始めるので、古いファイルを消すとき (Prune) に、書きかけのファイルを選ばない。
func WriteFile(ctx context.Context, opts *WriteOptions, path string, recipients []age.Recipient) (*WriteResult, error) {
	if _, err := os.Lstat(path); err == nil {
		return nil, fmt.Errorf("the backup file %s already exists", path)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return nil, fmt.Errorf("failed to create the backup file: %w", err)
	}
	ok := false
	defer func() {
		if !ok {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	if err := tmp.Chmod(backupFileMode); err != nil {
		return nil, fmt.Errorf("failed to restrict the permission of the backup file: %w", err)
	}
	result, err := Write(ctx, opts, tmp, recipients)
	if err != nil {
		return nil, err
	}
	if err := tmp.Sync(); err != nil {
		return nil, fmt.Errorf("failed to flush the backup file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("failed to close the backup file: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return nil, fmt.Errorf("failed to rename the backup file: %w", err)
	}
	ok = true

	return result, nil
}

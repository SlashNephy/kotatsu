// Package backup は、DB とキャプチャの画像を、age の公開鍵で暗号化した 1 つのファイルに書き出し、空の DB に戻す (設計書 11.4、#1926)。
//
// ファイルは、age で暗号化した gzip の tar である。tar には、次の順にエントリーを置く。
//
//   - manifest.json: 形式の版、書き出したときのスキーマの版 (goose の版)、表と列の並び
//   - secret_key: アプリの秘密キー。DB の中の暗号化した値を読むのに要る
//   - tables/<表>/<連番>: 表ごとの COPY TO の出力 (テキスト形式) を、chunkSize ごとに区切ったもの。外部キーの順に並べる
//   - captures/<ユーザーの ID>/<キャプチャの ID>.png: キャプチャの画像
//   - end.json: 書き終えたことと、表ごとの大きさ。これがなければ、途中で切れたファイルとみなして戻さない
//
// 書き出しは 1 つのトランザクション (REPEATABLE READ) の中で行うので、表どうしは同じ時点のものになる。
// 戻すときは、書き出したときのスキーマの版まで kotatsu migrate で上げてから流し込み、最後に最新まで上げる。
package backup

import (
	"errors"
	"time"
)

const (
	// formatVersion は、ファイルの形式の版。形を変えて、前の版のファイルを読めなくなるときに上げる。
	formatVersion = 1
	// chunkSize は、表の COPY の出力を区切る大きさ。tar のエントリーは大きさを先に書くので、この大きさまでメモリにためてから書く。
	chunkSize = 4 << 20
	// maxManifestSize は、manifest.json と end.json の大きさの上限。表の数と列の数で決まり、普通は数十 KB に収まる。
	maxManifestSize = 4 << 20
	// maxSecretKeySize は、アプリの秘密キーの大きさの上限。
	maxSecretKeySize = 64 << 10
	// maxCaptureSize は、キャプチャの画像の大きさの上限。受け付ける画像の上限 (capture.DefaultMaxImageBytes) に余裕を持たせる。
	maxCaptureSize = 64 << 20

	manifestName   = "manifest.json"
	secretKeyName  = "secret_key"
	tablesPrefix   = "tables/"
	capturesPrefix = "captures/"
	endName        = "end.json"
	// captureExt は、キャプチャの画像の拡張子。kago.CaptureLocation と同じ。
	captureExt = ".png"
	// fileMode は、tar のエントリーの許可。戻すときは使わない。
	fileMode = 0o600
)

var (
	// ErrNoRecipients は、暗号化する公開鍵がないことを表す。
	ErrNoRecipients = errors.New("no age recipients to encrypt the backup to")
	// ErrNoSecretKey は、アプリの秘密キーを設定していないことを表す。
	ErrNoSecretKey = errors.New("the app secret key (auth.secret_key) is required to make a backup")
	// ErrNoDataDir は、キャプチャがあるのに、データディレクトリを設定していないことを表す。
	ErrNoDataDir = errors.New("the data directory (storage.data_dir) is required because the backup has captures")
	// ErrSchemaNewer は、DB のスキーマがこのバイナリより新しいので、書き出せないことを表す。
	ErrSchemaNewer = errors.New("the database schema is newer than this binary; make the backup with the newer kotatsu")
	// ErrUnclassifiedTable は、バックアップに入れるか決めていない表があることを表す。
	ErrUnclassifiedTable = errors.New("a table is not classified for backups")
	// ErrDatabaseNotEmpty は、戻す先の DB が空でないことを表す。
	ErrDatabaseNotEmpty = errors.New("the database to restore into must be empty")
	// ErrInvalidBackup は、バックアップのファイルが壊れているか、途中で切れていることを表す。
	ErrInvalidBackup = errors.New("invalid backup file")
	// ErrUnsupportedFormat は、このバイナリが読めない形式のファイルであることを表す。
	ErrUnsupportedFormat = errors.New("unsupported backup format")
	// ErrBackupTooOld は、バックアップのスキーマが古すぎて、このバイナリでは戻せないことを表す (#2040)。
	ErrBackupTooOld = errors.New("the backup was made with a Kotatsu older than v2026.11.5; restore it with v2026.11.5 and make the backup again")
	// ErrBackupNewer は、バックアップのスキーマがこのバイナリより新しいので、戻せないことを表す。
	ErrBackupNewer = errors.New("the backup was made with a newer schema than this binary; restore it with the newer kotatsu")
)

// Manifest は、バックアップの先頭に置く、中身の説明。
type Manifest struct {
	// Format は、ファイルの形式の版 (formatVersion)。
	Format int `json:"format"`
	// KotatsuVersion は、書き出した Kotatsu の版。
	KotatsuVersion string `json:"kotatsuVersion"`
	// SchemaVersion は、書き出したときのスキーマの版 (db/schema.sum。v2026.11.5 までは goose の版)。
	// 戻すときは、今のスキーマに流し込んでから、この版より新しい 1 回だけのスクリプトを流す (#2040)。
	SchemaVersion int64 `json:"schemaVersion"`
	// CreatedAt は、書き出した日時。
	CreatedAt time.Time `json:"createdAt"`
	// Tables は、書き出した表と列。戻すときは、この順に流し込む (外部キーで指される表が先)。
	Tables []*Table `json:"tables"`
	// Captures は、キャプチャの行の数。データディレクトリに画像がなかったものも数える。
	Captures int `json:"captures"`
}

// Table は、書き出した表と、COPY の列の並び。
type Table struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
}

// end は、書き終えたことを表す、最後のエントリー。
type end struct {
	// TableBytes は、表ごとの COPY の出力の大きさ。戻すときに読んだ大きさと比べる。
	TableBytes map[string]int64 `json:"tableBytes"`
	// Captures は、書き出したキャプチャの画像の数。
	Captures int `json:"captures"`
}

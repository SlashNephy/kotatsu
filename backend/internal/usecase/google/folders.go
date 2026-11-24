package google

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/googledrive"
	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
)

// MyDriveName は、フォルダを選ぶときの、マイドライブの根元の名前。画面は、この名前を訳して出す。
const MyDriveName = "My Drive"

var (
	// ErrNotAFolder は、選ぼうとしたものがフォルダでないか、見えないことを表す。
	ErrNotAFolder = errors.New("not a folder in the Google Drive")
	// ErrInvalidLibraryName は、ライブラリの名前が規則に合わないか、ほかのライブラリと重なることを表す。
	ErrInvalidLibraryName = errors.New("invalid library name")
	// ErrFolderNotSelected は、外そうとしたフォルダを選んでいないことを表す。
	ErrFolderNotSelected = errors.New("the folder is not selected")
)

// DriveFolder は、管理者がライブラリとして見せると選んだフォルダ。
type DriveFolder struct {
	ID string
	// DriveID は、フォルダのある共有ドライブの ID。マイドライブなら空。
	DriveID string
	// Name は、選んだときのフォルダの名前。
	Name string
	// Library は、ライブラリの名前。
	Library string
	AddedAt time.Time
}

// DriveFolders は、選んだフォルダを、選んだ順に返す。
func (s *Service) DriveFolders(ctx context.Context, db sqlc.DBTX) ([]*DriveFolder, error) {
	rows, err := sqlc.New(db).ListGoogleDriveFolders(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list the Google Drive folders: %w", err)
	}
	folders := make([]*DriveFolder, 0, len(rows))
	for _, row := range rows {
		folders = append(folders, &DriveFolder{ID: row.FolderID, DriveID: row.DriveID, Name: row.Name, Library: row.LibraryName, AddedAt: row.AddedAt})
	}

	return folders, nil
}

// FolderChoice は、見せるフォルダを選ぶときに出す、Drive のフォルダ。
type FolderChoice struct {
	ID   string
	Name string
}

// BrowseDriveFolders は、見せるフォルダを選ぶために、フォルダ parentID の中のフォルダを、名前の順に返す。parentID が空なら、
// マイドライブ (root) と、見える共有ドライブを返す。管理者だけが使う。
func (s *Service) BrowseDriveFolders(ctx context.Context, db sqlc.DBTX, parentID string) ([]*FolderChoice, error) {
	client, err := s.adminClient(ctx, db)
	if err != nil {
		return nil, err
	}
	if parentID == "" {
		drives, err := client.Drives(ctx)
		if err != nil {
			return nil, err
		}
		choices := make([]*FolderChoice, 0, 1+len(drives))
		choices = append(choices, &FolderChoice{ID: googledrive.MyDriveID, Name: MyDriveName})
		for _, drive := range drives {
			choices = append(choices, &FolderChoice{ID: drive.ID, Name: drive.Name})
		}

		return choices, nil
	}
	parent, err := client.Get(ctx, parentID)
	if errors.Is(err, googledrive.ErrNotFound) || (err == nil && !parent.Folder()) {
		return nil, ErrNotAFolder
	}
	if err != nil {
		return nil, err
	}
	children, err := client.Children(ctx, parent.ID, parent.DriveID)
	if err != nil {
		return nil, err
	}
	choices := []*FolderChoice{}
	for _, child := range children {
		if child.Folder() && !strings.HasPrefix(child.Name, ".") {
			choices = append(choices, &FolderChoice{ID: child.ID, Name: child.Name})
		}
	}
	slices.SortStableFunc(choices, func(a, b *FolderChoice) int { return strings.Compare(a.Name, b.Name) })

	return choices, nil
}

// AddDriveFolder は、フォルダ folderID を、名前 libraryName のライブラリとして見せるフォルダに足す。マイドライブの根元 (root) や
// 共有ドライブの根元も選べる。Drive のファイルを家族に見せ始める操作なので、呼ぶ側で、直近の再認証を確認する (設計書 9.5)。
// 名前が規則に合わないか、ほかのライブラリと重なれば、ErrInvalidLibraryName を返す。
func (s *Service) AddDriveFolder(ctx context.Context, db Database, actor *audit.Actor, folderID, libraryName string) error {
	libraryName = strings.TrimSpace(libraryName)
	client, err := s.adminClient(ctx, db)
	if err != nil {
		return err
	}
	folder, err := client.Get(ctx, strings.TrimSpace(folderID))
	if errors.Is(err, googledrive.ErrNotFound) || (err == nil && !folder.Folder()) {
		return ErrNotAFolder
	}
	if err != nil {
		return err
	}
	name := folder.Name
	// 共有ドライブの根元は、files.get では決まった名前 (「ドライブ」など) が返るので、共有ドライブの名前を取得する。
	if folder.DriveID != "" && folder.ID == folder.DriveID {
		drive, err := client.GetDrive(ctx, folder.DriveID)
		if err != nil {
			return err
		}
		name = drive.Name
	}
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		if err := s.checkLibraryName(ctx, q, folder.ID, libraryName); err != nil {
			return err
		}
		added, err := q.InsertGoogleDriveFolder(ctx, sqlc.InsertGoogleDriveFolderParams{
			FolderID: folder.ID, LibraryName: libraryName, DriveID: folder.DriveID, Name: name, Now: s.clock.Now(),
		})
		if err != nil {
			return fmt.Errorf("failed to add the Google Drive folder: %w", err)
		}
		if added == 0 {
			return fmt.Errorf("%w: the folder is already chosen", ErrInvalidLibraryName)
		}

		return s.recorder.Record(ctx, tx, &audit.Event{
			Actor: actor, Action: "google_drive.folder.add", TargetType: auditTargetDrive, TargetID: folder.ID,
			Summary: map[string]any{"name": name, "library": libraryName}, SourceIP: actor.SourceIP, UserAgent: actor.UserAgent,
		})
	})
	if err != nil {
		return err
	}
	s.forgetConnections()

	return nil
}

// RenameDriveFolder は、フォルダ folderID のライブラリの名前を libraryName に変える。見せるものは変わらないので、再認証は求めない。
// 再生位置とインデックスは、Drive のファイルの ID で検索するので、名前を変えても引き継ぐ。
func (s *Service) RenameDriveFolder(ctx context.Context, db Database, actor *audit.Actor, folderID, libraryName string) error {
	libraryName = strings.TrimSpace(libraryName)
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		if err := s.checkLibraryName(ctx, q, folderID, libraryName); err != nil {
			return err
		}
		previous, err := q.RenameGoogleDriveFolder(ctx, sqlc.RenameGoogleDriveFolderParams{LibraryName: libraryName, FolderID: folderID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrFolderNotSelected
		}
		if err != nil {
			return fmt.Errorf("failed to rename the Google Drive library: %w", err)
		}
		if previous == libraryName {
			return nil
		}

		return s.recorder.Record(ctx, tx, &audit.Event{
			Actor: actor, Action: "google_drive.folder.rename", TargetType: auditTargetDrive, TargetID: folderID,
			Summary:  map[string]any{"library": map[string]any{"from": previous, "to": libraryName}},
			SourceIP: actor.SourceIP, UserAgent: actor.UserAgent,
		})
	})
	if err != nil {
		return err
	}
	s.forgetConnections()

	return nil
}

// checkLibraryName は、名前 name を、フォルダ folderID のライブラリの名前にできるかを確認する。保存先プールの ID と同じ規則
// (英小文字、数字、-、_) に合い、設定のライブラリ (プールとフォルダ) と、ほかのフォルダのライブラリと重ならないこと。
func (s *Service) checkLibraryName(ctx context.Context, q *sqlc.Queries, folderID, name string) error {
	if !config.IsValidPoolID(name) {
		return fmt.Errorf("%w: use 1 to 63 lowercase letters, digits, - and _, starting with a letter or a digit", ErrInvalidLibraryName)
	}
	if slices.Contains(s.reservedNames, name) {
		return fmt.Errorf("%w: %q is used by a storage pool or a library folder", ErrInvalidLibraryName, name)
	}
	other, err := q.GetGoogleDriveFolderByLibrary(ctx, name)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil
	case err != nil:
		return fmt.Errorf("failed to get the Google Drive library: %w", err)
	case other.FolderID != folderID:
		return fmt.Errorf("%w: %q is used by another Google Drive folder", ErrInvalidLibraryName, name)
	default:
		return nil
	}
}

// RemoveDriveFolder は、フォルダ folderID を、見せるフォルダから外す。見せるものを減らす操作なので、再認証は求めない。
func (s *Service) RemoveDriveFolder(ctx context.Context, db Database, actor *audit.Actor, folderID string) error {
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		row, err := sqlc.New(tx).DeleteGoogleDriveFolder(ctx, folderID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrFolderNotSelected
		}
		if err != nil {
			return fmt.Errorf("failed to remove the Google Drive folder: %w", err)
		}

		return s.recorder.Record(ctx, tx, &audit.Event{
			Actor: actor, Action: "google_drive.folder.remove", TargetType: auditTargetDrive, TargetID: row.FolderID,
			Summary: map[string]any{"name": row.Name}, SourceIP: actor.SourceIP, UserAgent: actor.UserAgent,
		})
	})
	if err != nil {
		return err
	}
	s.forgetConnections()

	return nil
}

// adminClient は、管理者の操作 (フォルダを選ぶ) で Drive を読むクライアントを返す。接続していなければ ErrNotConnected を返す。
func (s *Service) adminClient(ctx context.Context, db sqlc.DBTX) (*googledrive.Client, error) {
	s.tokensMu.Lock()
	if s.admin == nil {
		s.admin = &DriveTokens{
			service: s, db: db, mu: sync.Mutex{}, row: nil, folders: nil, checkedAt: time.Time{}, token: nil, tokenFor: nil, expiry: time.Time{},
		}
		s.tokens = append(s.tokens, s.admin)
	}
	tokens := s.admin
	s.tokensMu.Unlock()
	connected, err := tokens.Connected(ctx)
	if err != nil {
		return nil, err
	}
	if !connected {
		return nil, ErrNotConnected
	}

	return s.DriveClient(tokens), nil
}

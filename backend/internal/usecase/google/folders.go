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
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
)

// MyDriveName は、フォルダを選ぶときの、マイドライブの根元の名前。画面は、この名前を訳して出す。
const MyDriveName = "My Drive"

var (
	// ErrNotAFolder は、選ぼうとしたものがフォルダでないか、見えないことを表す。
	ErrNotAFolder = errors.New("not a folder in the Google Drive")
	// ErrFolderNotSelected は、外そうとしたフォルダを選んでいないことを表す。
	ErrFolderNotSelected = errors.New("the folder is not selected")
)

// DriveFolder は、管理者がライブラリとして見せると選んだフォルダ。
type DriveFolder struct {
	ID string
	// DriveID は、フォルダのある共有ドライブの ID。マイドライブなら空。
	DriveID string
	// Name は、選んだときのフォルダの名前。
	Name    string
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
		folders = append(folders, &DriveFolder{ID: row.FolderID, DriveID: row.DriveID, Name: row.Name, AddedAt: row.AddedAt})
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

// AddDriveFolder は、フォルダ folderID を、ライブラリとして見せるフォルダに足す。マイドライブの根元 (root) や共有ドライブの根元も選べる。
// Drive のファイルを家族に見せ始める操作なので、呼ぶ側で、直近の再認証を確かめる (設計書 9.5)。
func (s *Service) AddDriveFolder(ctx context.Context, db Database, actor *audit.Actor, folderID string) error {
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
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		added, err := sqlc.New(tx).InsertGoogleDriveFolder(ctx, sqlc.InsertGoogleDriveFolderParams{
			FolderID: folder.ID, DriveID: folder.DriveID, Name: folder.Name, Now: s.clock.Now(),
		})
		if err != nil {
			return fmt.Errorf("failed to add the Google Drive folder: %w", err)
		}
		if added == 0 {
			return nil
		}

		return s.recorder.Record(ctx, tx, &audit.Event{
			Actor: actor, Action: "google_drive.folder.add", TargetType: auditTargetDrive, TargetID: folder.ID,
			Summary: map[string]any{"name": folder.Name}, SourceIP: actor.SourceIP, UserAgent: actor.UserAgent,
		})
	})
	if err != nil {
		return err
	}
	s.forgetConnections()

	return nil
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

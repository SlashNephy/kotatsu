package google_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/medialib"
	"github.com/SlashNephy/kotatsu/internal/testutil/googletest"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/google"
)

const (
	// videosID と archiveID は、テストの Drive のフォルダの ID。sharedDriveID は、共有ドライブの ID。
	videosID      = "videos"
	archiveID     = "archive"
	sharedDriveID = "shared"
)

// TestDriveFolders は、管理者が見せるフォルダを選び、外せることと、接続を外すか別のアカウントで接続し直すと、
// 選んだフォルダも外すことを確かめる (#1902)。
func TestDriveFolders(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.fake.AddFile(&googletest.File{ID: videosID, Name: videosID, MIMEType: googletest.FolderMIMEType, Parent: googletest.MyDriveID})
	f.fake.AddDrive(&googletest.Drive{ID: sharedDriveID, Name: "Backup"})
	f.fake.AddFile(&googletest.File{ID: archiveID, Name: archiveID, MIMEType: googletest.FolderMIMEType, Parent: sharedDriveID})
	actor := audit.User(f.admin.UserID)

	_, err := f.service.BrowseDriveFolders(t.Context(), f.pool, "")
	require.ErrorIs(t, err, google.ErrNotConnected, "cannot browse before connecting")
	f.registerApp(t)
	_, err = f.complete(t, f.admin, f.authorize(t))
	require.NoError(t, err)

	// フォルダをたどって選ぶ。根元には、マイドライブと共有ドライブを並べ、ファイルは出さない。
	roots, err := f.service.BrowseDriveFolders(t.Context(), f.pool, "")
	require.NoError(t, err)
	assert.Equal(t, []*google.FolderChoice{{ID: googletest.MyDriveID, Name: google.MyDriveName}, {ID: sharedDriveID, Name: "Backup"}}, roots)
	children, err := f.service.BrowseDriveFolders(t.Context(), f.pool, googletest.MyDriveID)
	require.NoError(t, err)
	assert.Equal(t, []*google.FolderChoice{{ID: videosID, Name: videosID}}, children)
	_, err = f.service.BrowseDriveFolders(t.Context(), f.pool, "a")
	require.ErrorIs(t, err, google.ErrNotAFolder)

	tokens := f.service.DriveTokens(f.pool)
	folders, err := tokens.Folders(t.Context())
	require.NoError(t, err)
	assert.Empty(t, folders)

	require.ErrorIs(t, f.service.AddDriveFolder(t.Context(), f.pool, actor, "a"), google.ErrNotAFolder, "a file cannot be chosen")
	require.ErrorIs(t, f.service.AddDriveFolder(t.Context(), f.pool, actor, "missing"), google.ErrNotAFolder)
	require.NoError(t, f.service.AddDriveFolder(t.Context(), f.pool, actor, videosID))
	require.NoError(t, f.service.AddDriveFolder(t.Context(), f.pool, actor, archiveID))
	require.NoError(t, f.service.AddDriveFolder(t.Context(), f.pool, actor, videosID), "adding twice is fine")
	selected, err := f.service.DriveFolders(t.Context(), f.pool)
	require.NoError(t, err)
	require.Len(t, selected, 2)
	// 共有ドライブの ID は、名前で探す範囲を絞るのに使う。
	assert.ElementsMatch(t, []string{"", sharedDriveID}, []string{selected[0].DriveID, selected[1].DriveID})
	// 同じ部品の DriveTokens は、選んだフォルダをすぐに読み直す。
	folders, err = tokens.Folders(t.Context())
	require.NoError(t, err)
	assert.ElementsMatch(t, []*medialib.DriveFolder{{ID: videosID, DriveID: ""}, {ID: archiveID, DriveID: sharedDriveID}}, folders)

	require.NoError(t, f.service.RemoveDriveFolder(t.Context(), f.pool, actor, archiveID))
	require.ErrorIs(t, f.service.RemoveDriveFolder(t.Context(), f.pool, actor, archiveID), google.ErrFolderNotSelected)
	var adds, removes int
	require.NoError(t, f.pool.QueryRow(t.Context(),
		`SELECT count(*) FILTER (WHERE action = 'google_drive.folder.add'), count(*) FILTER (WHERE action = 'google_drive.folder.remove') FROM audit_events`,
	).Scan(&adds, &removes))
	assert.Equal(t, 2, adds, "adding twice is recorded once")
	assert.Equal(t, 1, removes)

	// 同じアカウントで接続し直しても、選んだフォルダは残す。
	_, err = f.complete(t, f.admin, f.authorize(t))
	require.NoError(t, err)
	selected, err = f.service.DriveFolders(t.Context(), f.pool)
	require.NoError(t, err)
	assert.Len(t, selected, 1)
	// 別のアカウントで接続し直したら、前のアカウントで選んだフォルダは外す。
	f.fake.Account = "other@example.com"
	_, err = f.complete(t, f.admin, f.authorize(t))
	require.NoError(t, err)
	selected, err = f.service.DriveFolders(t.Context(), f.pool)
	require.NoError(t, err)
	assert.Empty(t, selected)

	// 接続を外すと、選んだフォルダも外す。
	require.NoError(t, f.service.AddDriveFolder(t.Context(), f.pool, actor, videosID))
	require.NoError(t, f.service.DisconnectDrive(t.Context(), f.pool, actor))
	selected, err = f.service.DriveFolders(t.Context(), f.pool)
	require.NoError(t, err)
	assert.Empty(t, selected)
}

package google_test

import (
	"slices"
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
	// backupName は、共有ドライブの名前。
	backupName = "Backup"
	// videosLibrary は、videos のライブラリの名前。
	videosLibrary = "videos"
)

// connectedFixture は、モックの Drive にフォルダを置き、クライアントを登録して接続した fixture を返す。
//
//	マイドライブ: videos/、共有ドライブ Backup: archive/
func connectedFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	f.fake.AddFile(&googletest.File{ID: videosID, Name: videosID, MIMEType: googletest.FolderMIMEType, Parent: googletest.MyDriveID})
	f.fake.AddDrive(&googletest.Drive{ID: sharedDriveID, Name: backupName})
	f.fake.AddFile(&googletest.File{ID: archiveID, Name: "archive", MIMEType: googletest.FolderMIMEType, Parent: sharedDriveID})
	f.registerApp(t)
	_, err := f.complete(t, f.admin, f.authorize(t))
	require.NoError(t, err)

	return f
}

// TestDriveFolders は、管理者が見せるフォルダを選び、外せることと、接続を外すか別のアカウントで接続し直すと、
// 選んだフォルダも外すことを確認する (#1902)。
func TestDriveFolders(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.fake.AddFile(&googletest.File{ID: videosID, Name: videosID, MIMEType: googletest.FolderMIMEType, Parent: googletest.MyDriveID})
	f.fake.AddDrive(&googletest.Drive{ID: sharedDriveID, Name: backupName})
	f.fake.AddFile(&googletest.File{ID: archiveID, Name: "archive", MIMEType: googletest.FolderMIMEType, Parent: sharedDriveID})
	actor := audit.User(f.admin.UserID)

	_, err := f.service.BrowseDriveFolders(t.Context(), f.pool, "")
	require.ErrorIs(t, err, google.ErrNotConnected, "cannot browse before connecting")
	f.registerApp(t)
	_, err = f.complete(t, f.admin, f.authorize(t))
	require.NoError(t, err)

	// フォルダをたどって選ぶ。根元には、マイドライブと共有ドライブを並べ、ファイルは出さない。
	roots, err := f.service.BrowseDriveFolders(t.Context(), f.pool, "")
	require.NoError(t, err)
	assert.Equal(t, []*google.FolderChoice{{ID: googletest.MyDriveID, Name: google.MyDriveName}, {ID: sharedDriveID, Name: backupName}}, roots)
	children, err := f.service.BrowseDriveFolders(t.Context(), f.pool, googletest.MyDriveID)
	require.NoError(t, err)
	assert.Equal(t, []*google.FolderChoice{{ID: videosID, Name: videosID}}, children)
	_, err = f.service.BrowseDriveFolders(t.Context(), f.pool, "a")
	require.ErrorIs(t, err, google.ErrNotAFolder)

	tokens := f.service.DriveTokens(f.pool)
	folders, err := tokens.Folders(t.Context())
	require.NoError(t, err)
	assert.Empty(t, folders)

	require.ErrorIs(t, f.service.AddDriveFolder(t.Context(), f.pool, actor, "a", "a"), google.ErrNotAFolder, "a file cannot be chosen")
	require.ErrorIs(t, f.service.AddDriveFolder(t.Context(), f.pool, actor, "missing", "missing"), google.ErrNotAFolder)
	require.NoError(t, f.service.AddDriveFolder(t.Context(), f.pool, actor, videosID, videosLibrary))
	// 共有ドライブの根元は、files.get が共有ドライブの名前を返さないので、共有ドライブの名前を残す。
	require.NoError(t, f.service.AddDriveFolder(t.Context(), f.pool, actor, sharedDriveID, " atmos "))
	require.ErrorIs(t, f.service.AddDriveFolder(t.Context(), f.pool, actor, videosID, "again"), google.ErrInvalidLibraryName, "the folder is already chosen")
	selected, err := f.service.DriveFolders(t.Context(), f.pool)
	require.NoError(t, err)
	require.Len(t, selected, 2)
	shared := selected[slices.IndexFunc(selected, func(f *google.DriveFolder) bool { return f.ID == sharedDriveID })]
	assert.Equal(t, backupName, shared.Name)
	assert.Equal(t, "atmos", shared.Library)
	// 同じ部品の DriveTokens は、選んだフォルダをすぐに読み直す。
	folders, err = tokens.Folders(t.Context())
	require.NoError(t, err)
	assert.ElementsMatch(t, []*medialib.DriveFolder{
		{ID: videosID, DriveID: "", Library: videosLibrary}, {ID: sharedDriveID, DriveID: sharedDriveID, Library: "atmos"},
	}, folders)

	require.NoError(t, f.service.RemoveDriveFolder(t.Context(), f.pool, actor, sharedDriveID))
	require.ErrorIs(t, f.service.RemoveDriveFolder(t.Context(), f.pool, actor, sharedDriveID), google.ErrFolderNotSelected)
	var adds, removes int
	require.NoError(t, f.pool.QueryRow(t.Context(),
		`SELECT count(*) FILTER (WHERE action = 'google_drive.folder.add'), count(*) FILTER (WHERE action = 'google_drive.folder.remove') FROM audit_events`,
	).Scan(&adds, &removes))
	assert.Equal(t, 2, adds)
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
	require.NoError(t, f.service.AddDriveFolder(t.Context(), f.pool, actor, videosID, videosLibrary))
	require.NoError(t, f.service.DisconnectDrive(t.Context(), f.pool, actor))
	selected, err = f.service.DriveFolders(t.Context(), f.pool)
	require.NoError(t, err)
	assert.Empty(t, selected)
}

// TestDriveLibraryNames は、ライブラリの名前を、保存先プールの ID と同じ規則で確認し、設定のライブラリやほかのフォルダの
// ライブラリと重ねないことと、後で変えられることを確認する (#1902)。
func TestDriveLibraryNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		library string
		wantErr error
	}{
		{name: "valid", library: "archive_2026-a"},
		{name: "uppercase", library: "Archive", wantErr: google.ErrInvalidLibraryName},
		{name: "japanese", library: "録画", wantErr: google.ErrInvalidLibraryName},
		{name: "starting with a symbol", library: "-archive", wantErr: google.ErrInvalidLibraryName},
		{name: "empty", library: " ", wantErr: google.ErrInvalidLibraryName},
		// 設定のライブラリ (保存先プールとフォルダ) と重ねない。
		{name: "storage pool", library: reservedPool, wantErr: google.ErrInvalidLibraryName},
		// ほかのフォルダのライブラリと重ねない。
		{name: "another folder", library: videosLibrary, wantErr: google.ErrInvalidLibraryName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := connectedFixture(t)
			actor := audit.User(f.admin.UserID)
			require.NoError(t, f.service.AddDriveFolder(t.Context(), f.pool, actor, videosID, videosLibrary))
			err := f.service.AddDriveFolder(t.Context(), f.pool, actor, archiveID, tt.library)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
		})
	}

	f := connectedFixture(t)
	actor := audit.User(f.admin.UserID)
	require.NoError(t, f.service.AddDriveFolder(t.Context(), f.pool, actor, videosID, videosLibrary))
	require.NoError(t, f.service.AddDriveFolder(t.Context(), f.pool, actor, archiveID, "archive"))
	require.ErrorIs(t, f.service.RenameDriveFolder(t.Context(), f.pool, actor, archiveID, videosLibrary), google.ErrInvalidLibraryName)
	require.ErrorIs(t, f.service.RenameDriveFolder(t.Context(), f.pool, actor, archiveID, "Bad Name"), google.ErrInvalidLibraryName)
	require.ErrorIs(t, f.service.RenameDriveFolder(t.Context(), f.pool, actor, "missing", "anything"), google.ErrFolderNotSelected)
	require.NoError(t, f.service.RenameDriveFolder(t.Context(), f.pool, actor, archiveID, "backup"))
	require.NoError(t, f.service.RenameDriveFolder(t.Context(), f.pool, actor, archiveID, "backup"), "keeping the name is fine")
	selected, err := f.service.DriveFolders(t.Context(), f.pool)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{videosLibrary, "backup"}, []string{selected[0].Library, selected[1].Library})
	var summary string
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT summary::text FROM audit_events WHERE action = 'google_drive.folder.rename'`).Scan(&summary))
	assert.Contains(t, summary, `"to": "backup"`)
}

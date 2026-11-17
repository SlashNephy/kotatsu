package googletest

import (
	"context"
	"slices"

	"github.com/SlashNephy/kotatsu/internal/infra/medialib"
)

// Scope は、決まった接続と、決まった見せるフォルダを返す medialib.DriveScope。Google Drive のライブラリのテストで使う。
type Scope struct {
	// Disconnected は、接続していないものとするかどうか。
	Disconnected bool
	// Selected は、見せるフォルダ。
	Selected []*medialib.DriveFolder
}

var _ medialib.DriveScope = new(Scope)

// NewScope は、接続していて、folders を見せる Scope を返す。
func NewScope(folders ...*medialib.DriveFolder) *Scope {
	return &Scope{Disconnected: false, Selected: folders}
}

func (s *Scope) Connected(context.Context) (bool, error) { return !s.Disconnected, nil }

func (s *Scope) Folders(context.Context) ([]*medialib.DriveFolder, error) {
	return slices.Clone(s.Selected), nil
}

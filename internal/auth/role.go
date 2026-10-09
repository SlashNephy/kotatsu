// Package auth は、ユーザー、ロール、セッション、認可を扱う (設計書 9.5、9.7)。
package auth

import (
	"errors"
	"fmt"
	"slices"
)

// Role はユーザーのロール。
type Role string

const (
	// RoleAdmin は管理者。すべての操作ができる。
	RoleAdmin Role = "admin"
	// RoleMember はメンバー。ユーザーとシステムの管理のほかは、すべての操作ができる。
	// ルール、予約、録画は家族で共有するので、誰が作ったものでも変更や削除ができる。
	RoleMember Role = "member"
	// RoleViewer は視聴者。子どもや来客のように、見るだけの人を想定する。
	RoleViewer Role = "viewer"
)

// ErrInvalidRole は、知らないロールであることを表す。
var ErrInvalidRole = errors.New("invalid role")

// ParseRole は、DB に保存した文字列をロールにする。
func ParseRole(s string) (Role, error) {
	switch r := Role(s); r {
	case RoleAdmin, RoleMember, RoleViewer:
		return r, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidRole, s)
	}
}

// Permission は、ロールに許す操作のまとまり (設計書 9.7 の表の各行)。
type Permission string

const (
	// PermissionWatch は、ライブ視聴、録画の再生、自分の設定と再生位置の変更。
	PermissionWatch Permission = "watch"
	// PermissionManageSchedule は、ルールと予約の作成、変更、削除。
	PermissionManageSchedule Permission = "manage_schedule"
	// PermissionManageRecordings は、録画の削除、保護、タグ付け。
	PermissionManageRecordings Permission = "manage_recordings"
	// PermissionAdminister は、ユーザー、システム設定、Webhook、保存先プール、監査ログの管理。
	PermissionAdminister Permission = "administer"
)

// permissionsOf は、ロールごとに許す操作 (設計書 9.7)。
func permissionsOf(role Role) []Permission {
	switch role {
	case RoleAdmin:
		return []Permission{PermissionWatch, PermissionManageSchedule, PermissionManageRecordings, PermissionAdminister}
	case RoleMember:
		return []Permission{PermissionWatch, PermissionManageSchedule, PermissionManageRecordings}
	case RoleViewer:
		return []Permission{PermissionWatch}
	default:
		return []Permission{}
	}
}

// Can は、ロールが操作を許されているかどうかを返す。
func (r Role) Can(p Permission) bool {
	return slices.Contains(permissionsOf(r), p)
}

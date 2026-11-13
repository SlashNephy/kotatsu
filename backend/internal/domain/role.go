package domain

import "slices"

//go:generate go tool go-enum --values --nocomments --sql

// Role はユーザーのロール (設計書 9.7)。
//
// ENUM(
// admin // 管理者。すべての操作ができる。
// member // メンバー。ユーザーとシステムの管理のほかは、すべての操作ができる。ルール、予約、録画は家族で共有するので、誰が作ったものでも変更や削除ができる。
// viewer // 視聴者。子どもや来客のように、見るだけの人を想定する。
// )
type Role string

// Permission は、ロールに許す操作のまとまり (設計書 9.7 の表の各行)。
//
// ENUM(
// watch // ライブ視聴、録画の再生、自分の設定と再生位置の変更。
// manage_schedule // ルールと予約の作成、変更、削除。
// manage_recordings // 録画の削除、保護、タグ付け。
// administer // ユーザー、システム設定、Webhook、保存先プール、監査ログの管理。
// )
type Permission string

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

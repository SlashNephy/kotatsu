package oauth

import (
	"slices"
	"strings"

	"github.com/SlashNephy/kotatsu/internal/auth"
)

const (
	// ScopeRead は、番組、予約、ルール、録画を読むツールを使えるスコープ。
	ScopeRead = "mcp:read"
	// ScopeWrite は、予約とルールを変えるツールも使えるスコープ。読むツールも使える。
	ScopeWrite = "mcp:write"
)

// supportedScopes は、メタデータに載せるスコープ。
func supportedScopes() []string {
	return []string{ScopeRead, ScopeWrite}
}

// parseScopes は、scope の値 (空白で区切った一覧) から、Kotatsu のスコープだけを、重ねずに並べ直して返す。
// openid や offline_access のような、知らないスコープは無視する (RFC 6749 3.3)。リフレッシュトークンはいつも発行する。
// 何も指定がなければ、すべてのスコープを求めたものとし、同意の画面で選ばせる。
func parseScopes(raw string) []string {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return supportedScopes()
	}
	scopes := make([]string, 0, len(supportedScopes()))
	for _, scope := range supportedScopes() {
		if slices.Contains(fields, scope) {
			scopes = append(scopes, scope)
		}
	}

	return scopes
}

// AllowedScopes は、ロールに許すスコープを返す。視聴者は予約とルールを変えられないので、書くスコープを持てない (設計書 9.7)。
func AllowedScopes(role auth.Role) []string {
	if role.Can(auth.PermissionManageSchedule) {
		return supportedScopes()
	}

	return []string{ScopeRead}
}

// narrowScopes は、連携のスコープを、ユーザーの今のロールに許すものに絞る。
// 同意した後にロールを下げられたユーザーの連携が、下げる前のスコープのまま使われないようにする。
// 書くスコープだけの連携でも、読むスコープを含むので、視聴者に下げたら読むスコープを残す。
func narrowScopes(scopes []string, role auth.Role) []string {
	allowed := AllowedScopes(role)
	narrowed := make([]string, 0, len(allowed))
	for _, scope := range allowed {
		if HasScope(scopes, scope) {
			narrowed = append(narrowed, scope)
		}
	}

	return narrowed
}

// HasScope は、scopes が、want のスコープのツールを使えるかどうかを返す。書くスコープは、読むスコープを含む。
func HasScope(scopes []string, want string) bool {
	if slices.Contains(scopes, want) {
		return true
	}

	return want == ScopeRead && slices.Contains(scopes, ScopeWrite)
}

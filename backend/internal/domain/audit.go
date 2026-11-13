package domain

//go:generate go tool go-enum --values --nocomments --sql

// AuditActorType は、監査ログの操作者の種類。
//
// ENUM(
// user // ユーザーの操作。
// system // システムのロールの操作 (neko による自動予約など)。
// )
type AuditActorType string

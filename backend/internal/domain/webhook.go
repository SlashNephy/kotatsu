package domain

//go:generate go tool go-enum --values --nocomments

// WebhookFormat は、Webhook で送る形式 (設計書 6.2)。
//
// ENUM(
// JSON = json // 汎用の JSON。イベントの種類と、画面に出す文の材料をそのまま送る。
// discord // Discord の Webhook の形式。
// slack // Slack の Incoming Webhook の形式。
// )
type WebhookFormat string

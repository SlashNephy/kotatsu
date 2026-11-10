// Package netactivity は、サーバーが行った外部との通信を、抽象化した集計として記録する (#1369、設計書 11.8)。
//
// 記録するのは、時間帯、目的、接続先のホスト名、きっかけ、結果、回数、送受信量だけ。URL のパスとクエリ、ヘッダー、本文、
// ユーザー、番組は扱わない。外部と通信する HTTP クライアントは、NewClient で作る。
package netactivity

import (
	"slices"
)

// Purpose は、通信の目的。画面の題名と一文はこの種類ごとに決まり、通信の内容から文を作らない。
type Purpose string

// 通信の目的の一覧。増やすときは、画面の文 (i18n の networkActivity.purposes) と docs/privacy.md も直す。
const (
	PurposeJikkyoChannels    Purpose = "JIKKYO_CHANNELS"
	PurposeJikkyoComments    Purpose = "JIKKYO_COMMENTS"
	PurposeJikkyoKakolog     Purpose = "JIKKYO_KAKOLOG"
	PurposeNicologComments   Purpose = "NICOLOG_COMMENTS"
	PurposeAnimeProgramSync  Purpose = "ANIME_PROGRAM_SYNC"
	PurposeAnimeMappings     Purpose = "ANIME_MAPPINGS"
	PurposeAnimeWorkIDs      Purpose = "ANIME_WORK_IDS"
	PurposeAnimeArtworks     Purpose = "ANIME_ARTWORKS"
	PurposeAnimeRecord       Purpose = "ANIME_RECORD"
	PurposeAnimeLink         Purpose = "ANIME_LINK"
	PurposeAnimeImage        Purpose = "ANIME_IMAGE"
	PurposeUserAvatar        Purpose = "USER_AVATAR"
	PurposeOIDC              Purpose = "OIDC"
	PurposeHIBP              Purpose = "HIBP"
	PurposeMCPClientMetadata Purpose = "MCP_CLIENT_METADATA"
	PurposeUpdateCheck       Purpose = "UPDATE_CHECK"
	PurposeTelemetryTraces   Purpose = "TELEMETRY_TRACES"
	PurposeTelemetryMetrics  Purpose = "TELEMETRY_METRICS"
	PurposeWebPush           Purpose = "WEB_PUSH"
	PurposeWebhook           Purpose = "WEBHOOK"
	PurposeDataBroadcast     Purpose = "DATA_BROADCAST"
)

// Initiator は、通信のきっかけ。
type Initiator string

const (
	// InitiatorScheduled は、定期処理やジョブによる通信。
	InitiatorScheduled Initiator = "SCHEDULED"
	// InitiatorUser は、誰かが画面を操作したことによる通信。
	InitiatorUser Initiator = "USER"
)

// initiatorOf は、目的 p のきっかけを返す。
func initiatorOf(p Purpose) Initiator {
	switch p {
	case PurposeAnimeProgramSync, PurposeAnimeMappings, PurposeAnimeWorkIDs, PurposeAnimeArtworks, PurposeAnimeRecord,
		PurposeUpdateCheck, PurposeTelemetryTraces, PurposeTelemetryMetrics, PurposeWebPush, PurposeWebhook:
		return InitiatorScheduled
	case PurposeJikkyoChannels, PurposeJikkyoComments, PurposeJikkyoKakolog, PurposeNicologComments, PurposeAnimeLink, PurposeAnimeImage,
		PurposeUserAvatar, PurposeOIDC, PurposeHIBP, PurposeMCPClientMetadata, PurposeDataBroadcast:
		return InitiatorUser
	default:
		return InitiatorUser
	}
}

// visibleHost は、目的 p の通信の接続先 host を、記録に残してよければそのまま、残せなければ空で返す。
//
// 接続先から視聴の内容や設定の中身が分かるものは、ホスト名を残さない。決まった接続先の一覧にあるものだけを残すので、
// 設定で別の接続先に向け直したときも、そのホスト名は残らない。
func visibleHost(p Purpose, host string) string {
	hosts, anyHost := knownHosts(p)
	if anyHost || slices.Contains(hosts, host) {
		return host
	}

	return ""
}

// knownHosts は、目的 p で名前を残してよい接続先を返す。2 つ目が true なら、どのホストでも残す。
func knownHosts(p Purpose) ([]string, bool) {
	const (
		nxJikkyo   = "nx-jikkyo.tsukumijima.net"
		annict     = "annict.com"
		annictAPI  = "api.annict.com"
		aniList    = "anilist.co"
		aniListAPI = "graphql.anilist.co"
	)

	switch p {
	case PurposeJikkyoChannels, PurposeJikkyoComments:
		return []string{nxJikkyo}, false
	case PurposeJikkyoKakolog:
		return []string{"jikkyo.tsukumijima.net"}, false
	case PurposeNicologComments:
		return []string{"nicolog.oyasumi.today"}, false
	case PurposeAnimeProgramSync:
		return []string{"cal.syoboi.jp"}, false
	case PurposeAnimeMappings:
		return []string{"raw.githubusercontent.com"}, false
	case PurposeAnimeWorkIDs:
		return []string{annictAPI, aniListAPI}, false
	case PurposeAnimeArtworks:
		return []string{aniListAPI, "api.jikan.moe"}, false
	case PurposeAnimeRecord, PurposeAnimeLink:
		return []string{annict, annictAPI, aniList, aniListAPI}, false
	case PurposeHIBP:
		return []string{"api.pwnedpasswords.com"}, false
	case PurposeUpdateCheck:
		return []string{"api.github.com"}, false
	case PurposeWebPush:
		// 送り先は、購読したブラウザのベンダーのプッシュサービスで、視聴の内容は分からない。
		return nil, true
	case PurposeAnimeImage, PurposeUserAvatar, PurposeOIDC, PurposeMCPClientMetadata, PurposeTelemetryTraces,
		PurposeTelemetryMetrics, PurposeWebhook, PurposeDataBroadcast:
		return nil, false
	default:
		return nil, false
	}
}

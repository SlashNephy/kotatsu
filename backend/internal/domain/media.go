package domain

import (
	"slices"
	"time"
)

// MediaServiceTypes は、映像か音声を流すサービスの種別 (ARIB STD-B10 第2部 6.2.13)。デジタル TV、デジタル音声、
// 臨時映像、臨時音声、プロモーション映像、プロモーション音声、超高精細度 4K 専用 TV のサービス。EPGStation の isMediaService と揃える。
// ワンセグやデータ放送 (0xC0)、エンジニアリングサービス (0xA4) などは、視聴も録画もしないので、取り込まず、検索やルールの対象にもしない。
func MediaServiceTypes() []int64 {
	return []int64{0x01, 0x02, 0xA1, 0xA2, 0xA5, 0xA6, 0xAD}
}

// MaxGuideRange は、番組表で一度に取得できる時間帯の長さの上限。EPG が届く 8 日分を、1 つのチャンネルの番組表で一度に引けるようにする (#482)。
// 夏時間のある地域では、切り替えの日の放送日が 25 時間になるので、1 時間の余裕を持たせる。
const MaxGuideRange = 8*24*time.Hour + time.Hour

// IsMediaService は、サービスの種別が、映像か音声を流すサービスかどうかを返す。
func IsMediaService(serviceType int64) bool {
	return slices.Contains(MediaServiceTypes(), serviceType)
}

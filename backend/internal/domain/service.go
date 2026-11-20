package domain

import "time"

// ServiceGoneAfter は、上流のサービスの一覧に無いまま、この時間がたったサービスを、消えたものとみなすまでの猶予 (#1973)。
// チューナーサーバーの設定を一時的に間違えたときや、スキャンし直している途中で、番組表の列がすぐに消えないようにする。
const ServiceGoneAfter = 24 * time.Hour

// ServiceGoneBefore は、now の時点で、上流に無いと気づいた時刻 (services.gone_at) がこれより前なら消えたとみなす、境の時刻を返す。
func ServiceGoneBefore(now time.Time) time.Time {
	return now.Add(-ServiceGoneAfter)
}

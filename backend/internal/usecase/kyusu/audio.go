package kyusu

//go:generate go tool go-enum --values --nocomments

// Audio は、HLS に入れる音声 (設計書 7.5)。tsreadex が整えた後の、第 1 音声か第 2 音声かを選ぶ。
//
// tsreadex は、デュアルモノを 2 つのモノラル音声に分け、右 (副音声) を第 2 音声にする。
// 音声の ES が 2 つある番組 (二か国語放送など) では、2 つ目の ES が第 2 音声になる。
// 第 2 音声がない番組では、第 1 音声を写したものが第 2 音声になる (tsreadexArgs の -b)。
//
// ENUM(
// First = "1" // 第 1 音声 (主音声)。
// Second = "2" // 第 2 音声 (副音声や、2 つ目の音声)。
// )
//
// URL には、値 ("1" か "2") をそのまま使う。
type Audio string

// streamIndex は、FFmpeg の -map 0:a:N に渡す、入力の音声の番号を返す。
func (a Audio) streamIndex() string {
	if a == AudioSecond {
		return "1"
	}

	return "0"
}

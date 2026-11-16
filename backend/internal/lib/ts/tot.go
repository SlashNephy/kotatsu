package ts

import "time"

const (
	// tableIDTDT と tableIDTOT は、TDT と TOT の table_id。
	tableIDTDT = 0x70
	tableIDTOT = 0x73
	// jstOffset は、日本標準時の UTC からのずれ (秒)。TOT の時刻は日本標準時で書かれる。
	jstOffset = 9 * 60 * 60
)

// parseTOT は、TDT か TOT のセクションから、放送の現在の時刻 (JST_time) を読む (#1309、ARIB STD-B10 5.2.8、5.2.9)。
// JST_time は、MJD の 16 ビットと、時、分、秒の BCD の 24 ビット。読めなければ false を返す。
func parseTOT(section []byte) (time.Time, bool) {
	if len(section) < 8 || (section[0] != tableIDTDT && section[0] != tableIDTOT) {
		return time.Time{}, false
	}

	return parseJSTTime(section[3:8])
}

// parseJSTTime は、MJD の 16 ビットと、時、分、秒の BCD の 24 ビットからなる 5 バイトの時刻を読む。
// TOT の JST_time と、EIT の start_time が、この形をとる。未定 (すべてのビットが 1) か読めなければ false を返す。
func parseJSTTime(b []byte) (time.Time, bool) {
	mjd := int(b[0])<<8 | int(b[1])
	hour, okH := bcd(b[2])
	minute, okM := bcd(b[3])
	second, okS := bcd(b[4])
	if !okH || !okM || !okS || hour > 23 || minute > 59 || second > 60 || mjd == 0xFFFF {
		return time.Time{}, false
	}
	// MJD 0 は 1858-11-17。
	date := time.Date(1858, 11, 17, 0, 0, 0, 0, time.FixedZone("JST", jstOffset)).AddDate(0, 0, mjd)

	return date.Add(time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute + time.Duration(second)*time.Second), true
}

// bcd は、2 桁の BCD を読む。BCD でなければ false を返す。
func bcd(b byte) (int, bool) {
	hi, lo := int(b>>4), int(b&0x0F)
	if hi > 9 || lo > 9 {
		return 0, false
	}

	return hi*10 + lo, true
}

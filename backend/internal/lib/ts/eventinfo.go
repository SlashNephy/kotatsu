package ts

const (
	// descriptorTagShortEvent は、短形式イベント記述子 (ARIB STD-B10) のタグ。番組名と概要を持つ。
	descriptorTagShortEvent = 0x4D
	// descriptorTagExtendedEvent は、拡張形式イベント記述子のタグ。詳細 (見出しと本文) を持つ。
	descriptorTagExtendedEvent = 0x4E
	// descriptorTagSeries は、シリーズ記述子のタグ。話数を持つ。
	descriptorTagSeries = 0xD5
	// seriesMinLength は、シリーズ記述子の、series_name_char より前の長さ。
	seriesMinLength = 8
	// shortEventMinLength は、短形式イベント記述子の、event_name_char より前の長さ。
	shortEventMinLength = 4
	// extendedEventMinLength は、拡張形式イベント記述子の、項目の並びより前の長さ。
	extendedEventMinLength = 5
)

// EventInfo は、EIT の記述子から読んだ番組の情報 (#1500)。文字列は DecodeARIBString で読んだもの。
// 記述子がなければ、その項目は空 (Series は nil) にする。
type EventInfo struct {
	Name        string
	Description string
	// Extended は、詳細の見出しと本文。送られた順に並べる。
	Extended []*ExtendedItem
	// Series は、シリーズ記述子の内容。記述子がなければ nil。
	Series *EventSeries
}

// ExtendedItem は、拡張形式イベント記述子の項目 (見出しと本文)。
type ExtendedItem struct {
	Heading string
	Text    string
}

// EventSeries は、シリーズ記述子の内容。
type EventSeries struct {
	// Episode は episode_number (12 ビット)。送らない局では 0 にする。
	Episode uint16
	// Repeat は repeat_label (4 ビット)。0 なら初回。
	Repeat uint8
}

// rawExtendedItem は、読む前の詳細の項目。
type rawExtendedItem struct {
	heading []byte
	text    []byte
}

// parseEventInfo は、EIT のイベントの記述子の並びから、番組名、概要、詳細、シリーズを読む。
// 詳細の項目は、記述子をまたいで続くことがある (見出しのない項目は前の項目の続き) ので、バイトをつないでから読む。
func parseEventInfo(descriptors []byte) *EventInfo {
	info := &EventInfo{Name: "", Description: "", Extended: []*ExtendedItem{}, Series: nil}
	items := []*rawExtendedItem{}
	for len(descriptors) >= 2 {
		tag, length := descriptors[0], int(descriptors[1])
		if 2+length > len(descriptors) {
			break
		}
		body := descriptors[2 : 2+length]
		descriptors = descriptors[2+length:]
		switch tag {
		case descriptorTagShortEvent:
			parseShortEvent(body, info)
		case descriptorTagExtendedEvent:
			items = parseExtendedEvent(body, items)
		case descriptorTagSeries:
			// series_id (2)、repeat_label (4 ビット)、program_pattern、expire_date_valid_flag、expire_date (2)、
			// episode_number (12 ビット)、last_episode_number (12 ビット)、series_name_char。
			if len(body) < seriesMinLength {
				continue
			}
			info.Series = &EventSeries{Episode: uint16(body[5])<<4 | uint16(body[6]>>4), Repeat: body[2] >> 4}
		}
	}
	for _, item := range items {
		info.Extended = append(info.Extended, &ExtendedItem{Heading: DecodeARIBString(item.heading), Text: DecodeARIBString(item.text)})
	}

	return info
}

// parseShortEvent は、短形式イベント記述子の本体から、番組名と概要を読む。
// ISO_639_language_code (3)、event_name_length、event_name_char、text_length、text_char。
func parseShortEvent(body []byte, info *EventInfo) {
	if len(body) < shortEventMinLength {
		return
	}
	nameLength := int(body[3])
	if shortEventMinLength+nameLength >= len(body) {
		return
	}
	info.Name = DecodeARIBString(body[shortEventMinLength : shortEventMinLength+nameLength])
	rest := body[shortEventMinLength+nameLength:]
	if textLength := int(rest[0]); 1+textLength <= len(rest) {
		info.Description = DecodeARIBString(rest[1 : 1+textLength])
	}
}

// parseExtendedEvent は、拡張形式イベント記述子の本体の項目を、items に足して返す。
// descriptor_number と last_descriptor_number、ISO_639_language_code (3)、length_of_items、項目の並び、text_length、text_char。
func parseExtendedEvent(body []byte, items []*rawExtendedItem) []*rawExtendedItem {
	if len(body) < extendedEventMinLength {
		return items
	}
	loop := body[extendedEventMinLength:min(extendedEventMinLength+int(body[4]), len(body))]
	for len(loop) >= 2 {
		headingLength := int(loop[0])
		if 1+headingLength >= len(loop) {
			break
		}
		heading := loop[1 : 1+headingLength]
		textLength := int(loop[1+headingLength])
		start := 2 + headingLength
		if start+textLength > len(loop) {
			break
		}
		text := loop[start : start+textLength]
		loop = loop[start+textLength:]
		if headingLength == 0 && len(items) > 0 {
			last := items[len(items)-1]
			last.text = append(last.text, text...)

			continue
		}
		items = append(items, &rawExtendedItem{heading: append([]byte{}, heading...), text: append([]byte{}, text...)})
	}

	return items
}

package ts

// PID の種類 (#473)。PMT に書かれたストリームは、映像、音声、そのほか。PSI と SI は、PAT、PMT、PMT の CA 記述子が指す ECM と、決まった PID のテーブル。
const (
	PIDKindVideo   = "video"
	PIDKindAudio   = "audio"
	PIDKindOther   = "other"
	PIDKindPSI     = "psi"
	PIDKindUnknown = "unknown"
)

// PIDQuality は、PID ごとの品質の集計と、その PID の種類。録画のセグメントごとに JSON で記録する (#473)。
type PIDQuality struct {
	PID       uint16 `json:"pid"`
	Kind      string `json:"kind"`
	Packets   int64  `json:"packets"`
	Drops     int64  `json:"drops"`
	Errors    int64  `json:"errors"`
	Scrambled int64  `json:"scrambled"`
	// Payloads と ScrambledPayloads は、PIDStats の同じ名前の数 (#1517)。記録する前に録ったセグメントでは nil。
	Payloads          *int64 `json:"payloads,omitempty"`
	ScrambledPayloads *int64 `json:"scrambledPayloads,omitempty"`
	// StreamType は、PMT の stream_type (#827)。PMT に載っていない PID と、記録する前に録ったセグメントでは nil。
	StreamType *uint8 `json:"streamType,omitempty"`
	// ComponentTag は、ストリーム識別記述子の component_tag (#827)。記述子がなければ nil。
	ComponentTag *uint8 `json:"componentTag,omitempty"`
}

// PIDQualities は、PID ごとの統計 pids に、stats の PMT と決まった PID から分かる種類を添える。
// pids は、stats.PIDs か、引き継ぎで重なった区間を除いた stats.PIDsBeforeCutoff を渡す。
func PIDQualities(stats *Stats, pids []*PIDStats) []PIDQuality {
	kinds := map[uint16]string{}
	streams := map[uint16]*Stream{}
	for _, program := range stats.Programs {
		kinds[program.PMTPID] = PIDKindPSI
		// CA 記述子が指す ECM は、ES ではなくセクションで送るテーブルなので、番組情報とみなす。
		for _, pid := range program.ECMPIDs {
			kinds[pid] = PIDKindPSI
		}
		for _, stream := range program.Streams {
			streams[stream.PID] = stream
			switch stream.Kind {
			case StreamKindVideo:
				kinds[stream.PID] = PIDKindVideo
			case StreamKindAudio:
				kinds[stream.PID] = PIDKindAudio
			case StreamKindOther:
				kinds[stream.PID] = PIDKindOther
			}
		}
	}
	result := make([]PIDQuality, 0, len(pids))
	for _, pid := range pids {
		kind, ok := kinds[pid.PID]
		switch {
		case ok:
		case isSIPID(pid.PID):
			kind = PIDKindPSI
		default:
			kind = PIDKindUnknown
		}
		var streamType, componentTag *uint8
		if stream, ok := streams[pid.PID]; ok {
			streamType = new(stream.StreamType)
			componentTag = stream.ComponentTag
		}
		quality := PIDQuality{
			PID: pid.PID, Kind: kind, Packets: pid.Packets, Drops: pid.Drops, Errors: pid.Errors, Scrambled: pid.Scrambled,
			Payloads: new(pid.Payloads), ScrambledPayloads: new(pid.ScrambledPayloads),
			StreamType: streamType, ComponentTag: componentTag,
		}
		result = append(result, quality)
	}

	return result
}

// JudgedDrops は、PID ごとの統計 pids のドロップのうち、録画の状態と重複の回避の判定に数えるものの合計を返す (設計書 5.3)。
// stats の PMT で映像か音声とされた PID のドロップだけを数える。字幕・データ、番組情報、PMT に載っていない PID のドロップは、
// 見たときの乱れにならないので数えない。BS4K の録画では、番組の境目で字幕の MMT パケットの通し番号が飛び、TS に変換したときに
// 字幕の PID の continuity counter も飛ぶ。受信で本当に欠けると、映像と音声の PID にもドロップが出る。
// PMT を読めていなければ、どれが映像と音声か分からないので、すべての PID のドロップを数える。
func JudgedDrops(stats *Stats, pids []*PIDStats) int64 {
	media := map[uint16]bool{}
	for _, program := range stats.Programs {
		for _, stream := range program.Streams {
			if stream.Kind == StreamKindVideo || stream.Kind == StreamKindAudio {
				media[stream.PID] = true
			}
		}
	}
	var total int64
	for _, pid := range pids {
		if len(media) == 0 || media[pid.PID] {
			total += pid.Drops
		}
	}

	return total
}

// isSIPID は、pid が、ARIB で決まった PID で送る PSI と SI のテーブル (PAT、CAT、NIT、SDT/BAT、EIT、RST、TDT/TOT、SDTT、BIT、CDT など) かどうかを返す。
func isSIPID(pid uint16) bool {
	switch pid {
	case 0x0000, 0x0001, 0x0010, 0x0011, 0x0012, 0x0013, 0x0014, 0x0023, 0x0024, 0x0026, 0x0027, 0x0028, 0x0029:
		return true
	default:
		return false
	}
}

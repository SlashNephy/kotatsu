package ts

// PID の種類 (#473)。PMT に書かれたストリームは、映像、音声、そのほか。PSI と SI は、PAT、PMT と、決まった PID の表。
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
		for _, stream := range program.Streams {
			streams[stream.PID] = stream
			switch stream.Kind {
			case StreamVideo:
				kinds[stream.PID] = PIDKindVideo
			case StreamAudio:
				kinds[stream.PID] = PIDKindAudio
			case StreamOther:
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
			StreamType: streamType, ComponentTag: componentTag,
		}
		result = append(result, quality)
	}

	return result
}

// isSIPID は、pid が、ARIB で決まった PID で送る PSI と SI の表 (PAT、CAT、NIT、SDT/BAT、EIT、RST、TDT/TOT、SDTT、BIT、CDT など) かどうかを返す。
func isSIPID(pid uint16) bool {
	switch pid {
	case 0x0000, 0x0001, 0x0010, 0x0011, 0x0012, 0x0013, 0x0014, 0x0023, 0x0024, 0x0026, 0x0027, 0x0028, 0x0029:
		return true
	default:
		return false
	}
}

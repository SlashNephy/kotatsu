package tunerserver

import "io"

// EventType は、イベントが知らせる変化の種類。
type EventType string

const (
	// EventProgramCreated、EventProgramUpdated、EventProgramRemoved は、番組が作られた、変わった、消えたこと。
	// Event の ServiceID に、番組が変わったサービスを持つ。
	EventProgramCreated EventType = "program-created"
	EventProgramUpdated EventType = "program-updated"
	EventProgramRemoved EventType = "program-removed"
	// EventServiceChanged は、サービスが作られたか、変わったか、消えたこと。
	EventServiceChanged EventType = "service-changed"
	// EventTunerChanged は、チューナーの状態か構成が変わったこと。
	EventTunerChanged EventType = "tuner-changed"
)

// Event は、チューナーサーバーから届いた、何が変わったかの知らせ。
//
// イベントは取りこぼしや遅れがあるので、「変化があった」という合図として扱う。データの正しさは全量の取得で保証する (設計書 3.3)。
type Event struct {
	Type EventType
	// ServiceID は、番組のイベントで、番組が変わったサービスの ID (networkId × 100000 + serviceId)。ほかのイベントでは 0。
	ServiceID int64
}

// EventStream は、チューナーサーバーのイベントを順に読む。読み終えたら必ず Close を呼ぶ。
type EventStream interface {
	// Next は次のイベントを返す。実装は、Kotatsu が使わないイベントを読み飛ばす。
	// ストリームが区切りで終わったら io.EOF を、途中で切れたら io.ErrUnexpectedEOF を返す。
	Next() (*Event, error)
	io.Closer
}

package tunerserver

import "errors"

// ErrRefused は、チューナーサーバーが応答したが、求めたものを返さなかったこと。
// ErrTunerUnavailable、ErrNotFound、ErrLogoUnavailable も、errors.Is でこれに当たる。
var ErrRefused = errors.New("the tuner server refused the request")

var (
	// ErrTunerUnavailable は、ストリームを開くのに、チューナーを確保できなかったこと。呼び出し側は再試行できる。
	ErrTunerUnavailable error = &refusal{msg: "no tuner is available"}
	// ErrNotFound は、求めたサービスかチャンネルが、チューナーサーバーにないこと。
	ErrNotFound error = &refusal{msg: "the tuner server has no such service or channel"}
	// ErrLogoUnavailable は、サービスのロゴを、チューナーサーバーがまだ受信していないこと。
	ErrLogoUnavailable error = &refusal{msg: "the tuner server has not received the logo"}
)

// ErrClockUnavailable は、チューナーサーバーの時計を読めないこと。EDCB の CtrlCmd には、HTTP の Date ヘッダーに当たるものがない (#1532)。
var ErrClockUnavailable = errors.New("the tuner server does not tell its clock")

var (
	// ErrStreamStalled は、ストリームが、接続を保ったまま、データを送らなくなったこと。
	ErrStreamStalled = errors.New("the tuner server stopped sending the stream")
	// ErrStreamOpenTimeout は、ストリームを開くときに、チューナーサーバーが上限の時間までに応答しなかったこと。
	ErrStreamOpenTimeout = errors.New("the tuner server did not respond to the stream request in time")
)

// refusal は、ErrRefused の一種であるエラー。
type refusal struct {
	msg string
}

var _ error = new(refusal)

func (e *refusal) Error() string {
	return e.msg
}

// Is は、ErrRefused に当たることを errors.Is に知らせる。
func (e *refusal) Is(target error) bool {
	return target == ErrRefused
}

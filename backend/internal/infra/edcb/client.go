// Package edcb は、EDCB の EpgTimerSrv を、CtrlCmd の TCP で呼ぶクライアント (#1529、設計書 3.1)。
//
// CtrlCmd は、HTTP ではない独自のバイナリのプロトコルで、1 つの接続で 1 つのコマンドを送り、応答を受け取る。
// 要求と応答は、コマンド (または戻り値) と本文の大きさの DWORD に、本文が続く。数はリトルエンディアン、文字列は UTF-16LE で書く。
// 定義は、xtne6f/EDCB のソース (Common/CtrlCmdDef.h、Common/CtrlCmdUtil.cpp、Common/StructDef.h) を読んで、
// Go で書き起こした (付録 B)。CtrlCmd の TCP には、パスワードも TLS もない。
package edcb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

// コマンド (EDCB の Common/CtrlCmdDef.h)。
const (
	cmdRelayViewStream  = 301
	cmdEnumTunerReserve = 1016
	cmdEnumService      = 1021
	cmdEnumPGInfoEx     = 1029
	cmdFileCopy         = 1060
	cmdEnumTunerProcess = 1066
	cmdNWTVIDSetCh      = 1073
	cmdNWTVIDClose      = 1074
	cmdGetStatusNotify2 = 2200
	cmdFileCopy2        = 2060
)

// cmdVersion は、「2」の付くコマンドで、要求の本文の先頭に送る版 (CMD_VER)。応答も、この版で書かれる。
const cmdVersion = 5

// 戻り値 (EDCB の Common/ErrDef.h)。
const (
	statusSuccess         = 1
	statusError           = 0
	statusNotSupported    = 203
	statusInvalidArgument = 204
	statusBusy            = 208
)

// DefaultPort は、EpgTimerSrv の TCP の待ち受けの既定のポート。
const DefaultPort = "4510"

const (
	defaultDialTimeout    = 10 * time.Second
	defaultRequestTimeout = 60 * time.Second
	// maxResponseSize は、受け取る応答の本文の上限。FILE_COPY2 は、まとめて 256 MiB まで返す。
	maxResponseSize = 512 << 20
	// networkTVSendTCP は、NetworkTV のチューナーに TS を TCP で送らせる送信モード (SET_CH_INFO の ch)。
	networkTVSendTCP = 2
)

var (
	// ErrBusy は、EpgTimerSrv が、EPG のデータを読み込んでいる最中などで、今は応えられないことを表す (CMD_ERR_BUSY)。
	ErrBusy = errors.New("EpgTimerSrv is busy")
	// ErrNotSupported は、EpgTimerSrv がコマンドに対応していないことを表す (CMD_NON_SUPPORT)。
	ErrNotSupported = errors.New("EpgTimerSrv does not support the command")
	// ErrFailed は、EpgTimerSrv がコマンドに失敗したことを表す (CMD_ERR や、引数の誤り)。
	ErrFailed = errors.New("EpgTimerSrv failed the command")
)

// StatusError は、EpgTimerSrv が成功以外の戻り値を返したことを表す。errors.Is で ErrBusy などと比べられる。
type StatusError struct {
	Command uint32
	Status  uint32
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("EpgTimerSrv returned %d for command %d", e.Status, e.Command)
}

func (e *StatusError) Is(target error) bool {
	switch e.Status {
	case statusBusy:
		return target == ErrBusy
	case statusNotSupported:
		return target == ErrNotSupported
	default:
		return target == ErrFailed
	}
}

// Options は Client の設定。
type Options struct {
	// Address は、EpgTimerSrv の TCP の待ち受け (host:port)。ポートを省けば DefaultPort を使う。
	// IPv6 のアドレスは [::1]:4510 のように書く。
	Address string
	// DialTimeout は、接続するまでの上限。0 なら 10 秒。
	DialTimeout time.Duration
	// RequestTimeout は、コマンドを送ってから応答を読み終えるまでの上限。0 なら 60 秒。
	// 通知を待つ WaitNotification と、TS を受け取る RelayStream には掛けない。
	RequestTimeout time.Duration
	// Dial は、接続を張る。nil なら net.Dialer を使う。テストで差し替える。
	Dial func(ctx context.Context, network, address string) (net.Conn, error)
}

// Client は、EpgTimerSrv の CtrlCmd を呼ぶ。複数の goroutine から使ってよい。
type Client struct {
	address        string
	requestTimeout time.Duration
	dial           func(ctx context.Context, network, address string) (net.Conn, error)
}

// New は Client を作る。
func New(opts *Options) (*Client, error) {
	address, err := normalizeAddress(opts.Address)
	if err != nil {
		return nil, err
	}
	dialTimeout := opts.DialTimeout
	if dialTimeout == 0 {
		dialTimeout = defaultDialTimeout
	}
	requestTimeout := opts.RequestTimeout
	if requestTimeout == 0 {
		requestTimeout = defaultRequestTimeout
	}
	dial := opts.Dial
	if dial == nil {
		dialer := &net.Dialer{Timeout: dialTimeout}
		dial = dialer.DialContext
	}

	return &Client{address: address, requestTimeout: requestTimeout, dial: dial}, nil
}

// normalizeAddress は、host:port の形にする。ポートがなければ DefaultPort を足す。
func normalizeAddress(address string) (string, error) {
	if address == "" {
		return "", errors.New("the EDCB address is empty")
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		// ポートを省いた形 (例: 192.168.1.10、::1) として読み直す。
		host, port = address, DefaultPort
	}
	if host == "" {
		return "", fmt.Errorf("the EDCB address %q has no host", address)
	}
	if n, err := strconv.Atoi(port); err != nil || n <= 0 || n > 65535 {
		return "", fmt.Errorf("the EDCB address %q has an invalid port", address)
	}

	return net.JoinHostPort(host, port), nil
}

// Address は、接続先 (host:port) を返す。
func (c *Client) Address() string {
	return c.address
}

// Services は、EPG のデータベースにあるサービスを返す (ENUM_SERVICE)。
// EpgTimerSrv が EPG のデータを読み込み終えていなければ ErrBusy を返す。
func (c *Client) Services(ctx context.Context) ([]*Service, error) {
	payload, err := c.send(ctx, cmdEnumService, nil, c.requestTimeout)
	if err != nil {
		return nil, err
	}

	return decode(cmdEnumService, payload, decodeServices)
}

// EventsQuery は、Events で引く番組の条件。
type EventsQuery struct {
	// Services は、引くサービス。空なら、すべてのサービスを引く。
	Services []ServiceKey
	// From と To は、番組の開始時刻の範囲 [From, To)。どちらもゼロ値なら、開始時刻が未定の番組も含めてすべてを引く。
	// 片方だけを指定したときは、もう片方を限りのないものとし、開始時刻が未定の番組は含めない。
	From, To time.Time
}

// Events は、番組をサービスごとに返す (ENUM_PG_INFO_EX)。番組のないサービスは含まない。
// EpgTimerSrv が EPG のデータを読み込み終えていなければ ErrBusy を返す。
func (c *Client) Events(ctx context.Context, query *EventsQuery) ([]*ServiceEvents, error) {
	var e encoder
	// キーは、(マスク, 値) の組を並べる。サービスのキーにマスクを OR したものが値と等しければ当たる。
	// 最後の 2 つは、開始時刻の範囲 (FILETIME と同じ 1601 年からの 100 ナノ秒、日本標準時)。
	keys := make([]int64, 0, 2*len(query.Services)+2)
	if len(query.Services) == 0 {
		keys = append(keys, allServicesMask, allServicesMask)
	}
	for _, s := range query.Services {
		keys = append(keys, 0, s.key())
	}
	from, to := int64(0), int64(maxInt64)
	if !query.From.IsZero() || !query.To.IsZero() {
		if !query.From.IsZero() {
			from = fileTime(query.From)
		} else {
			// 0 から始めると、EpgTimerSrv は開始時刻が未定の番組も含めるので、1 にする。
			from = 1
		}
		if !query.To.IsZero() {
			to = fileTime(query.To)
		}
	}
	keys = append(keys, from, to)
	e.vector(len(keys), func(e *encoder, i int) { e.i64(keys[i]) })
	payload, err := c.send(ctx, cmdEnumPGInfoEx, e.buf, c.requestTimeout)
	if err != nil {
		return nil, err
	}

	return decode(cmdEnumPGInfoEx, payload, decodeServiceEvents)
}

const (
	// allServicesMask は、すべてのサービスに当たるマスク (ONID、TSID、SID の 48 ビット)。
	allServicesMask = 0xFFFF_FFFF_FFFF
	maxInt64        = 1<<63 - 1
	// fileTimeEpoch は、FILETIME の起点 (1601-01-01) から Unix の起点までの 100 ナノ秒の数。
	fileTimeEpoch = 116444736000000000
)

// fileTime は、時刻を、EDCB が番組の時刻を比べるのに使う、日本標準時の FILETIME の値にする。
// EDCB は、SYSTEMTIME をタイムゾーンなしで FILETIME に直すので、日本標準時の壁時計の時刻として数える。
func fileTime(t time.Time) int64 {
	wall := t.In(jst())
	local := time.Date(wall.Year(), wall.Month(), wall.Day(), wall.Hour(), wall.Minute(), wall.Second(), wall.Nanosecond(), time.UTC)

	return local.UnixNano()/100 + fileTimeEpoch
}

// TunerReserves は、チューナーごとの予約の ID を返す (ENUM_TUNER_RESERVE)。
func (c *Client) TunerReserves(ctx context.Context) ([]*TunerReserve, error) {
	payload, err := c.send(ctx, cmdEnumTunerReserve, nil, c.requestTimeout)
	if err != nil {
		return nil, err
	}

	return decode(cmdEnumTunerReserve, payload, decodeTunerReserves)
}

// TunerProcesses は、EpgTimerSrv が起動しているチューナーのプロセスの状態を返す (ENUM_TUNER_PROCESS)。
func (c *Client) TunerProcesses(ctx context.Context) ([]*TunerProcess, error) {
	payload, err := c.send(ctx, cmdEnumTunerProcess, nil, c.requestTimeout)
	if err != nil {
		return nil, err
	}

	return decode(cmdEnumTunerProcess, payload, decodeTunerProcesses)
}

// OpenNetworkTV は、NetworkTV の ID id のチューナーで、サービスを開き、TS を TCP で送らせる (NWTV_ID_SET_CH)。
// 返すのは、チューナーのプロセスの ID で、RelayStream に渡す。ID は EpgTimerSrv 全体で 1 つの名前空間なので、
// ほかのクライアント (KonomiTV や EpgTimer の視聴) と重ならないものを使う。
// 開いた直後は、TS を送る準備ができていないことがあるので、RelayStream が失敗したら、少し待ってやり直す。
func (c *Client) OpenNetworkTV(ctx context.Context, id int32, service ServiceKey) (int32, error) {
	payload, err := c.send(ctx, cmdNWTVIDSetCh, encodeSetCh(true, service, id), c.requestTimeout)
	if err != nil {
		return 0, err
	}

	return decode(cmdNWTVIDSetCh, payload, func(d *decoder) int32 { return d.i32() })
}

// NetworkTVProcess は、NetworkTV の ID id のチューナーが開いていれば、そのプロセスの ID と true を返す
// (NWTV_ID_SET_CH を useSID=0 で送る)。EDCB の予約にチューナーを取られて閉じていれば、false を返す。
func (c *Client) NetworkTVProcess(ctx context.Context, id int32) (int32, bool, error) {
	payload, err := c.send(ctx, cmdNWTVIDSetCh, encodeSetCh(false, ServiceKey{ONID: 0, TSID: 0, SID: 0}, id), c.requestTimeout)
	var status *StatusError
	if errors.As(err, &status) && status.Status == statusError {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	pid, err := decode(cmdNWTVIDSetCh, payload, func(d *decoder) int32 { return d.i32() })
	if err != nil {
		return 0, false, err
	}

	return pid, true, nil
}

// encodeSetCh は、SET_CH_INFO を書く。NetworkTV のコマンドでは、space が NetworkTV の ID、ch が送信モードになる。
func encodeSetCh(useSID bool, service ServiceKey, id int32) []byte {
	var e encoder
	e.sized(func(e *encoder) {
		e.i32(boolInt32(useSID))
		e.u16(service.ONID)
		e.u16(service.TSID)
		e.u16(service.SID)
		e.i32(1) // useBonCh: ID と送信モードを指定する。
		e.i32(id)
		e.i32(networkTVSendTCP)
	})

	return e.buf
}

func boolInt32(v bool) int32 {
	if v {
		return 1
	}

	return 0
}

// CloseNetworkTV は、NetworkTV の ID id のチューナーを閉じる (NWTV_ID_CLOSE)。
// TS の中継の接続を切っても、EpgTimerSrv はチューナーを閉じないので、使い終えたら必ず呼ぶ。
func (c *Client) CloseNetworkTV(ctx context.Context, id int32) error {
	var e encoder
	e.i32(id)
	_, err := c.send(ctx, cmdNWTVIDClose, e.buf, c.requestTimeout)

	return err
}

// RelayStream は、チューナーのプロセス processID の TS を受け取る (RELAY_VIEW_STREAM)。
// 返す io.ReadCloser は、読み終えたら必ず閉じる。ctx が終わっても閉じる。1 つのプロセスの TS を同時に受け取れるのは 2 つまでで、
// 3 つ目は失敗する。EDCB の予約にチューナーを取られると、接続は切れないまま、予約の局の TS に変わる。
func (c *Client) RelayStream(ctx context.Context, processID int32) (io.ReadCloser, error) {
	conn, err := c.dial(ctx, "tcp", c.address)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to EpgTimerSrv: %w", err)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	fail := func(err error) (io.ReadCloser, error) {
		stop()
		_ = conn.Close()

		return nil, contextError(ctx, err)
	}
	var e encoder
	e.i32(processID)
	if err := writeRequest(conn, cmdRelayViewStream, e.buf); err != nil {
		return fail(fmt.Errorf("failed to send command %d: %w", cmdRelayViewStream, err))
	}
	status, _, err := readHeader(conn)
	if err != nil {
		return fail(fmt.Errorf("failed to read the response to command %d: %w", cmdRelayViewStream, err))
	}
	if status != statusSuccess {
		return fail(&StatusError{Command: cmdRelayViewStream, Status: status})
	}

	return &relayStream{Conn: conn, stop: stop}, nil
}

// relayStream は、TS を受け取っている接続。閉じたら、ctx を見張るのもやめる。
type relayStream struct {
	net.Conn

	stop func() bool
}

func (s *relayStream) Close() error {
	s.stop()

	return s.Conn.Close()
}

// WaitNotification は、巡回カウンタが count より後の、最初の通知を返す (GET_STATUS_NOTIFY2)。
// そのような通知がなければ、EpgTimerSrv は応答を保留するので、通知が来るか ctx が終わるまで待つ (ロングポーリング)。
// count が 0 なら、待たずに、今の動作の状態 (NotifyUpdateSrvStatus) を、最後の通知の巡回カウンタと一緒に返す。
// 初めて呼ぶときは count に 0 を渡し、以降は返った Notification の Count を渡す。
func (c *Client) WaitNotification(ctx context.Context, count uint32) (*Notification, error) {
	var e encoder
	e.u16(cmdVersion)
	e.u32(count)
	payload, err := c.send(ctx, cmdGetStatusNotify2, e.buf, 0)
	if err != nil {
		return nil, err
	}

	return decode(cmdGetStatusNotify2, payload, versioned(decodeNotification))
}

// ChannelSettings は、EpgTimerSrv が読み込んだチャンネルの設定 (ChSet5.txt) を返す (FILE_COPY)。
// 文字コードは、EDCB の設定のまま (Linux では UTF-8、Windows では UTF-8 か Shift_JIS) で返す。
func (c *Client) ChannelSettings(ctx context.Context) ([]byte, error) {
	var e encoder
	e.str("ChSet5.txt")

	return c.send(ctx, cmdFileCopy, e.buf, c.requestTimeout)
}

// Files は、EpgTimerSrv の設定のフォルダーのファイルをまとめて返す (FILE_COPY2)。
// 読めるのは、ChSet5.txt、LogoData.ini と、ロゴの画像 (LogoData\*.png など) に限られる。
// 返せなかったファイルは、Data を空にして返す。
func (c *Client) Files(ctx context.Context, names []string) ([]*File, error) {
	var e encoder
	e.u16(cmdVersion)
	e.vector(len(names), func(e *encoder, i int) { e.str(names[i]) })
	payload, err := c.send(ctx, cmdFileCopy2, e.buf, c.requestTimeout)
	if err != nil {
		return nil, err
	}

	return decode(cmdFileCopy2, payload, versioned(decodeFiles))
}

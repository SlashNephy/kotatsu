package database

import (
	"errors"
	"io"
	"net"

	"github.com/jackc/pgx/v5/pgconn"
)

// IsUnavailable は、err が、DB に届かないか、DB が止まりかけていることによるエラーかどうかを返す。
// そうなら、問い合わせ自体は正しいので、DB が戻ってからやり直せば通る (#1073)。
func IsUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := errors.AsType[*pgconn.ConnectError](err); ok {
		return true
	}
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		// 57P01 (admin_shutdown)、57P02 (crash_shutdown)、57P03 (cannot_connect_now) は、DB を止めているか起動しているところ。
		switch pgErr.Code {
		case "57P01", "57P02", "57P03":
			return true
		}

		return false
	}
	if _, ok := errors.AsType[net.Error](err); ok {
		return true
	}

	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || pgconn.Timeout(err) || pgconn.SafeToRetry(err)
}

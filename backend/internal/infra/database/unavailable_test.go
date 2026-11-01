package database_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"syscall"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
)

func TestIsUnavailable(t *testing.T) {
	t.Parallel()

	refused := &net.OpError{Op: "dial", Net: "tcp", Source: nil, Addr: nil, Err: syscall.ECONNREFUSED}
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "connection refused", err: fmt.Errorf("failed to renew: %w", refused), want: true},
		{name: "connection lost in the middle", err: fmt.Errorf("failed to read: %w", io.ErrUnexpectedEOF), want: true},
		{name: "server shutting down", err: &pgconn.PgError{Code: "57P01"}, want: true},
		{name: "server starting up", err: &pgconn.PgError{Code: "57P03"}, want: true},
		{name: "query timed out", err: fmt.Errorf("failed to renew: %w", context.DeadlineExceeded), want: true},
		{name: "unique violation", err: &pgconn.PgError{Code: "23505"}, want: false},
		{name: "statement timeout", err: &pgconn.PgError{Code: "57014"}, want: false},
		{name: "unrelated error", err: errors.New("the pool is not mounted"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, database.IsUnavailable(tt.err))
		})
	}
}

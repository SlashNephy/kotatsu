package jikkyo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/usecase/jikkyo"
)

// addedService と jkOverride は、画面で足した対応のテストで使うサービスとチャンネル。
const (
	addedService = "6:999"
	jkOverride   = "jk103"
	jkAdded      = "jk200"
)

// staticOverrides は、画面で足した対応として、いつも overrides を返す。
func staticOverrides(overrides map[string]string) jikkyo.OverridesFunc {
	return func(context.Context) (map[string]string, error) { return overrides, nil }
}

func TestTableLookup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		overrides map[string]string
		networkID int64
		serviceID int64
		wantID    string
		wantOK    bool
	}{
		{name: "BS は、ネットワーク ID とサービス ID の組で引く", networkID: 4, serviceID: 101, wantID: jkBS, wantOK: true},
		{name: "地上波は、地域のネットワーク ID によらず、サービス ID で引く", networkID: 0x7FE0, serviceID: 0x0400, wantID: jkNHK, wantOK: true},
		{name: "ほかの地域の地上波", networkID: 0x7FD1, serviceID: 0x2800, wantID: jkNHK, wantOK: true},
		{name: "地上波のサブチャンネルは、1 つ前のメインチャンネルに寄せる", networkID: 0x7FE0, serviceID: 0x0401, wantID: jkNHK, wantOK: true},
		{name: "地上波のサブチャンネルは、2 つ前のメインチャンネルにも寄せる", networkID: 0x7FE0, serviceID: 0x0402, wantID: jkNHK, wantOK: true},
		{name: "実況のチャンネルがないと書かれた地上波", networkID: 0x7F80, serviceID: 0x6438, wantID: "", wantOK: false},
		{name: "地上波でないネットワークでは、サービス ID だけでは引かない", networkID: 4, serviceID: 0x0400, wantID: "", wantOK: false},
		{name: "対応表にない BS", networkID: 4, serviceID: 999, wantID: "", wantOK: false},
		{
			name: "画面で足したサービス", overrides: map[string]string{addedService: jkAdded},
			networkID: 6, serviceID: 999, wantID: jkAdded, wantOK: true,
		},
		{
			name: "画面で足した対応は、同梱した対応表より優先する", overrides: map[string]string{"4:101": jkOverride},
			networkID: 4, serviceID: 101, wantID: jkOverride, wantOK: true,
		},
		{
			name: "画面で足した地上波は、ネットワーク ID まで一致したときだけ使う", overrides: map[string]string{"32736:1024": "jk8"},
			networkID: 0x7FE1, serviceID: 0x0400, wantID: jkNHK, wantOK: true,
		},
		{
			// 保存するときに確かめているので、崩れた対応は、引くときに読み飛ばす。
			name: "崩れた対応は読み飛ばす", overrides: map[string]string{"4-101": jkOverride, "bs:101": jkOverride, "4:101": "101"},
			networkID: 4, serviceID: 101, wantID: jkBS, wantOK: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			table, err := jikkyo.NewTable(staticOverrides(tt.overrides), clocktest.NewFake(time.Now()), nil)
			require.NoError(t, err)
			id, ok := table.Lookup(t.Context(), tt.networkID, tt.serviceID)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantID, id)
		})
	}
}

// TestTableReadsOverridesAgain は、画面で足した対応を、少しの間は覚えて読み直さず、時間が経つか Invalidate で読み直すことを確かめる (#1869)。
// 読めないときは、前に読んだものを使い続ける。
func TestTableReadsOverridesAgain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// next は、最初に読んだ後に起こすこと。
		next     func(clk *clocktest.Fake, table *jikkyo.Table)
		nextErr  error
		wantID   string
		wantRead int
	}{
		{name: "覚えている間は読み直さない", next: func(clk *clocktest.Fake, _ *jikkyo.Table) { clk.Advance(5 * time.Second) }, wantID: jkAdded, wantRead: 1},
		{name: "時間が経ったら読み直す", next: func(clk *clocktest.Fake, _ *jikkyo.Table) { clk.Advance(11 * time.Second) }, wantID: "jk201", wantRead: 2},
		{name: "Invalidate したらすぐ読み直す", next: func(_ *clocktest.Fake, table *jikkyo.Table) { table.Invalidate() }, wantID: "jk201", wantRead: 2},
		{
			name: "読めなければ、前に読んだものを使う", next: func(_ *clocktest.Fake, table *jikkyo.Table) { table.Invalidate() },
			nextErr: errors.New("the database is down"), wantID: jkAdded, wantRead: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			clk := clocktest.NewFake(time.Now())
			reads := 0
			table, err := jikkyo.NewTable(func(context.Context) (map[string]string, error) {
				reads++
				if reads > 1 && tt.nextErr != nil {
					return nil, tt.nextErr
				}

				return map[string]string{addedService: "jk" + []string{"200", "201"}[min(reads, 2)-1]}, nil
			}, clk, nil)
			require.NoError(t, err)
			first, ok := table.Lookup(t.Context(), 6, 999)
			require.True(t, ok)
			require.Equal(t, jkAdded, first)

			tt.next(clk, table)
			id, ok := table.Lookup(t.Context(), 6, 999)
			assert.True(t, ok)
			assert.Equal(t, tt.wantID, id)
			assert.Equal(t, tt.wantRead, reads)
		})
	}
}

func TestTableChannel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		id     string
		wantOK bool
	}{
		{name: "同梱した対応表のチャンネル", id: jkBS, wantOK: true},
		{name: "画面で足したチャンネル", id: jkAdded, wantOK: true},
		{name: "どちらにもないチャンネル", id: "jk99999", wantOK: false},
		{name: "空", id: "", wantOK: false},
	}
	table, err := jikkyo.NewTable(staticOverrides(map[string]string{addedService: jkAdded}), clocktest.NewFake(time.Now()), nil)
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			id, ok := table.Channel(t.Context(), tt.id)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.id, id)
			}
		})
	}
}

func TestIsChannelID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		id   string
		want bool
	}{
		{id: "jk1", want: true},
		{id: "jk211", want: true},
		{id: "101", want: false},
		{id: "jk", want: false},
		{id: "jk-1", want: false},
		{id: "jk1a", want: false},
		{id: "JK1", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, jikkyo.IsChannelID(tt.id))
		})
	}
}

// TestTableKeepsReadingAfterCanceledRequest は、取り消された要求で読めなかったときは、覚えた時刻を進めず、
// 次の要求ですぐ読み直すことを確かめる (#1869)。
func TestTableKeepsReadingAfterCanceledRequest(t *testing.T) {
	t.Parallel()

	reads := 0
	table, err := jikkyo.NewTable(func(ctx context.Context) (map[string]string, error) {
		reads++
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		return map[string]string{addedService: jkAdded}, nil
	}, clocktest.NewFake(time.Now()), nil)
	require.NoError(t, err)

	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	_, ok := table.Lookup(canceled, 6, 999)
	assert.False(t, ok)

	id, ok := table.Lookup(t.Context(), 6, 999)
	assert.True(t, ok)
	assert.Equal(t, jkAdded, id)
	assert.Equal(t, 2, reads)
}

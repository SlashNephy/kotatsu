package assign_test

import (
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/usecase/assign"
)

// onService は、相乗りの単位がサービスのチューナーサーバー (EDCB) で、予約のサービスを service にする。
func onService(r *assign.Reservation, service int64) *assign.Reservation {
	r.Service = service

	return r
}

// blocked は、Kotatsu の外の予約 (EDCB の予約) が、base から startMinute 分後から endMinute 分後までふさぐ GR のチューナー 0。
func blocked(startMinute, endMinute int) *assign.Tuner {
	return &assign.Tuner{Index: 0, Types: []string{typeGR}, Blocks: []assign.Block{{
		Start: base().Add(time.Duration(startMinute) * time.Minute), End: base().Add(time.Duration(endMinute) * time.Minute),
	}}}
}

// TestAssignEDCB は、EDCB の性質 (相乗りの単位がサービスであること、EDCB の予約がチューナーをふさぐこと) を渡した割り当てを確かめる (#1536)。
// Mirakurun 互換のサーバーでは、サービスを 0 にし、ふさぐ時間帯を空にするので、今までどおりになる。
func TestAssignEDCB(t *testing.T) {
	t.Parallel()

	at := func(minute int) time.Time { return base().Add(time.Duration(minute) * time.Minute) }
	conflict := func(n, startMinute, minutes int) *assign.Assignment {
		return &assign.Assignment{
			ReservationID: reservationID(n), State: domain.AssignmentStateConflict, Tuner: assign.NoTuner,
			PlannedStart: at(startMinute).Add(-margin), PlannedEnd: at(startMinute + minutes).Add(margin),
		}
	}
	tests := []struct {
		name         string
		reservations []*assign.Reservation
		tuners       []*assign.Tuner
		want         []*assign.Assignment
	}{
		{
			// Mirakurun は、同じチャンネルの別のサービスを 1 本のチューナーで受ける。
			name:         "mirakurun shares a tuner between services on the same channel",
			reservations: []*assign.Reservation{program(1, "27", 0, 30), program(2, "27", 0, 30)},
			tuners:       grTuners(1),
			want: []*assign.Assignment{
				{ReservationID: reservationID(1), State: domain.AssignmentStateOK, Tuner: 0, PlannedStart: at(0).Add(-margin), PlannedEnd: at(30).Add(margin)},
				{ReservationID: reservationID(2), State: domain.AssignmentStateOK, Tuner: 0, PlannedStart: at(0).Add(-margin), PlannedEnd: at(30).Add(margin)},
			},
		},
		{
			// EDCB の TS はサービスで絞られるので、同じチャンネルでも別のサービスは別のチューナーになる。
			name:         "edcb needs a tuner per service on the same channel",
			reservations: []*assign.Reservation{onService(program(1, "27", 0, 30), 1), onService(program(2, "27", 0, 30), 2)},
			tuners:       grTuners(1),
			want: []*assign.Assignment{
				{ReservationID: reservationID(1), State: domain.AssignmentStateOK, Tuner: 0, PlannedStart: at(0).Add(-margin), PlannedEnd: at(30).Add(margin)},
				conflict(2, 0, 30),
			},
		},
		{
			// 同じサービスの予約は、EDCB でも 1 本の NetworkTV を分け合うので (#1535)、重なってもマージンも削らない。
			name:         "edcb shares a tuner within one service",
			reservations: []*assign.Reservation{onService(program(1, "27", 0, 60), 1), onService(program(2, "27", 30, 60), 1)},
			tuners:       grTuners(1),
			want: []*assign.Assignment{
				{ReservationID: reservationID(1), State: domain.AssignmentStateOK, Tuner: 0, PlannedStart: at(0).Add(-margin), PlannedEnd: at(60).Add(margin)},
				{ReservationID: reservationID(2), State: domain.AssignmentStateOK, Tuner: 0, PlannedStart: at(30).Add(-margin), PlannedEnd: at(90).Add(margin)},
			},
		},
		{
			// EDCB の予約がふさぐチューナーは使わない。もう 1 本に置けない予約は競合になる。
			name:         "a tuner blocked by an edcb reservation is not used",
			reservations: []*assign.Reservation{program(1, "27", 0, 30), program(2, "26", 0, 30)},
			tuners:       []*assign.Tuner{blocked(-5, 45), grTuners(2)[1]},
			want: []*assign.Assignment{
				{ReservationID: reservationID(1), State: domain.AssignmentStateOK, Tuner: 1, PlannedStart: at(0).Add(-margin), PlannedEnd: at(30).Add(margin)},
				conflict(2, 0, 30),
			},
		},
		{
			// EDCB の予約が途中からふさぐなら、その前の本編だけを録る (一部のみ)。
			name:         "an edcb reservation starting midway cuts the end",
			reservations: []*assign.Reservation{program(1, "27", 0, 30)},
			tuners:       []*assign.Tuner{blocked(15, 60)},
			want: []*assign.Assignment{
				{ReservationID: reservationID(1), State: domain.AssignmentStatePartial, Tuner: 0, PlannedStart: at(0).Add(-margin), PlannedEnd: at(15)},
			},
		},
		{
			// EDCB の予約の直後に始まる予約は、開始マージンだけを削り、本編は録る。
			name:         "the start margin yields to an edcb reservation just before",
			reservations: []*assign.Reservation{program(1, "27", 0, 30)},
			tuners:       []*assign.Tuner{blocked(-30, 0)},
			want: []*assign.Assignment{
				{ReservationID: reservationID(1), State: domain.AssignmentStateOK, Tuner: 0, PlannedStart: at(0), PlannedEnd: at(30).Add(margin)},
			},
		},
		{
			// 録画が始まった予約も、EDCB の予約には勝てない。EDCB がチューナーを引き継ぐ前までを録る見込みにする。
			name:         "a recording reservation is cut where an edcb reservation takes its tuner",
			reservations: []*assign.Reservation{recordingOn(program(1, "27", 0, 60), 0)},
			tuners:       []*assign.Tuner{blocked(30, 90)},
			want: []*assign.Assignment{
				{ReservationID: reservationID(1), State: domain.AssignmentStatePartial, Tuner: 0, PlannedStart: at(0).Add(-margin), PlannedEnd: at(30)},
			},
		},
		{
			// ふさぐ時間帯が予約と重ならなければ、今までどおりに置く。
			name:         "a block outside the reservation does not matter",
			reservations: []*assign.Reservation{program(1, "27", 0, 30)},
			tuners:       []*assign.Tuner{blocked(60, 90)},
			want: []*assign.Assignment{
				{ReservationID: reservationID(1), State: domain.AssignmentStateOK, Tuner: 0, PlannedStart: at(0).Add(-margin), PlannedEnd: at(30).Add(margin)},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, assign.Assign(tt.reservations, tt.tuners))
		})
	}
}

// TestPeersEDCB は、相乗りの単位がサービスのとき、同じチャンネルの別のサービスの予約を、チューナーを取り合う相手に含めることを確かめる (#1536)。
func TestPeersEDCB(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		reservations []*assign.Reservation
		want         [][]uuid.UUID
	}{
		// Mirakurun では、同じチャンネルの予約は 1 本を共有するので、競合しない。
		{name: "mirakurun", reservations: []*assign.Reservation{program(1, "27", 0, 60), program(2, "27", 0, 60)}, want: [][]uuid.UUID{{}, {}}},
		{
			name:         "edcb",
			reservations: []*assign.Reservation{onService(program(1, "27", 0, 60), 1), onService(program(2, "27", 0, 60), 2)},
			want:         [][]uuid.UUID{{}, {reservationID(1)}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tuners := grTuners(1)
			assert.Equal(t, tt.want, assign.Peers(tt.reservations, tuners, assign.Assign(tt.reservations, tuners)))
		})
	}
}

package assign_test

import (
	"slices"
	"testing"
	"uuid"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/usecase/assign"
)

func TestPeers(t *testing.T) {
	t.Parallel()

	// combined は、地上波と衛星を兼ねるチューナー 0 と、地上波だけのチューナー 1。
	combined := []*assign.Tuner{{Index: 0, Types: []string{typeGR, typeBS}}, {Index: 1, Types: []string{typeGR}}}
	tests := []struct {
		name         string
		reservations []*assign.Reservation
		tuners       []*assign.Tuner
		// want は、予約の番号ごとの相手の番号。相手のない予約は書かない。
		want map[int][]int
	}{
		{
			name:         "競合しなければ、相手はない",
			reservations: []*assign.Reservation{program(1, "27", 0, 60), program(2, "26", 0, 60)},
			tuners:       grTuners(2),
			want:         map[int][]int{},
		},
		{
			name:         "兼用のチューナーを地上波が先に使うと、衛星の相手はその地上波だけになる",
			reservations: []*assign.Reservation{program(1, "27", 0, 60), program(2, "26", 0, 60), onBS(program(3, "BS1", 0, 60))},
			tuners:       combined,
			want:         map[int][]int{3: {1}},
		},
		{
			name: "同じチャンネルでチューナーを共有する予約は、どちらも相手になる",
			reservations: []*assign.Reservation{
				withPriority(program(1, "27", 0, 30), 1), withPriority(program(2, "27", 20, 40), 1), program(3, "26", 10, 30),
			},
			tuners: grTuners(1),
			want:   map[int][]int{3: {1, 2}},
		},
		{
			name: "同じチャンネルでチューナーを共有している予約は、相手にしない",
			reservations: []*assign.Reservation{
				withPriority(program(1, "27", 0, 60), 1), program(2, "26", 30, 60), program(3, "26", 60, 30),
			},
			tuners: grTuners(1),
			want:   map[int][]int{2: {1}},
		},
		{
			name: "録画中の予約がチューナーを塞いでいれば、優先度の高い予約の相手になる",
			reservations: []*assign.Reservation{
				recordingOn(program(1, "27", 0, 60), 0), withPriority(program(2, "26", 10, 30), 2),
			},
			tuners: grTuners(1),
			want:   map[int][]int{2: {1}},
		},
		{
			name:         "一部のみの予約は、本編を削った相手を示す",
			reservations: []*assign.Reservation{program(1, "27", 0, 60), program(2, "26", 30, 60)},
			tuners:       grTuners(1),
			want:         map[int][]int{2: {1}},
		},
		{
			name: "時間の重ならない予約は、相手にしない",
			reservations: []*assign.Reservation{
				program(1, "27", 0, 60), program(2, "25", 120, 30), program(3, "26", 10, 30),
			},
			tuners: grTuners(1),
			want:   map[int][]int{3: {1}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := peersByNumber(tt.reservations, tt.tuners)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestPeersIgnoresInputOrder は、相手の並びが、予約を渡す順に左右されないことを確かめる。
func TestPeersIgnoresInputOrder(t *testing.T) {
	t.Parallel()

	reservations := []*assign.Reservation{
		withPriority(program(1, "27", 20, 40), 1), withPriority(program(2, "25", 0, 30), 1), program(3, "26", 10, 30),
	}
	want := peersByNumber(reservations, grTuners(2))
	assert.Equal(t, map[int][]int{3: {2, 1}}, want)

	reversed := slices.Clone(reservations)
	slices.Reverse(reversed)
	assert.Equal(t, want, peersByNumber(reversed, grTuners(2)))
}

// peersByNumber は、割り当ててから相手を求め、予約の番号 (reservationID の n) で返す。
func peersByNumber(reservations []*assign.Reservation, tuners []*assign.Tuner) map[int][]int {
	number := func(id uuid.UUID) int {
		for n := 1; n <= len(reservations)*10; n++ {
			if reservationID(n) == id {
				return n
			}
		}

		return 0
	}
	got := map[int][]int{}
	for i, peers := range assign.Peers(reservations, tuners, assign.Assign(reservations, tuners)) {
		if len(peers) == 0 {
			continue
		}
		numbers := make([]int, 0, len(peers))
		for _, id := range peers {
			numbers = append(numbers, number(id))
		}
		got[number(reservations[i].ID)] = numbers
	}

	return got
}

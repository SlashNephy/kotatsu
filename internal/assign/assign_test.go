package assign_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"

	"github.com/SlashNephy/kotatsu/internal/assign"
)

const (
	typeGR = "GR"
	typeBS = "BS"
	margin = 15 * time.Second
)

func base() time.Time {
	return time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
}

func reservationID(n int) uuid.UUID {
	return uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-%012d", n))
}

// program は、base から startMinute 分後に始まる minutes 分の、GR の channel の予約。
func program(n int, channel string, startMinute, minutes int) *assign.Reservation {
	start := base().Add(time.Duration(startMinute) * time.Minute)

	return &assign.Reservation{
		ID: reservationID(n), ChannelType: typeGR, Channel: channel,
		StartAt: start, EndAt: start.Add(time.Duration(minutes) * time.Minute),
		StartMargin: margin, EndMargin: margin, Priority: 0, Recording: false, Tuner: 0,
	}
}

func withPriority(r *assign.Reservation, priority int) *assign.Reservation {
	r.Priority = priority

	return r
}

func recordingOn(r *assign.Reservation, tuner int) *assign.Reservation {
	r.Recording, r.Tuner = true, tuner

	return r
}

func startingLater(r *assign.Reservation, d time.Duration) *assign.Reservation {
	r.StartAt = r.StartAt.Add(d)

	return r
}

func onBS(r *assign.Reservation) *assign.Reservation {
	r.ChannelType = typeBS

	return r
}

func grTuners(n int) []*assign.Tuner {
	tuners := make([]*assign.Tuner, 0, n)
	for i := range n {
		tuners = append(tuners, &assign.Tuner{Index: i, Types: []string{typeGR}})
	}

	return tuners
}

func TestAssign(t *testing.T) {
	t.Parallel()

	at := func(minute int) time.Time { return base().Add(time.Duration(minute) * time.Minute) }
	tests := []struct {
		name         string
		reservations []*assign.Reservation
		tuners       []*assign.Tuner
		want         []*assign.Assignment
	}{
		{
			name:         "back-to-back programs on one tuner trim the end margin of the earlier one first",
			reservations: []*assign.Reservation{program(1, "27", 0, 30), program(2, "26", 30, 30)},
			tuners:       grTuners(1),
			want: []*assign.Assignment{
				// 前の録画の終了マージンを先に削り、それでも重なる分は後の録画の開始マージンを削る。本編はどちらも削らない。
				{ReservationID: reservationID(1), State: assign.StateOK, Tuner: 0, PlannedStart: at(0).Add(-margin), PlannedEnd: at(30)},
				{ReservationID: reservationID(2), State: assign.StateOK, Tuner: 0, PlannedStart: at(30), PlannedEnd: at(60).Add(margin)},
			},
		},
		{
			name: "only the end margin of the earlier one is trimmed when it is enough",
			reservations: []*assign.Reservation{
				program(1, "27", 0, 30),
				// 本編どうしの間が 20 秒あり、マージンは 10 秒重なる。前の録画の終了マージンを 10 秒削れば、後の開始マージンは残る。
				startingLater(program(2, "26", 30, 30), 20*time.Second),
			},
			tuners: grTuners(1),
			want: []*assign.Assignment{
				{ReservationID: reservationID(1), State: assign.StateOK, Tuner: 0, PlannedStart: at(0).Add(-margin), PlannedEnd: at(30).Add(5 * time.Second)},
				{ReservationID: reservationID(2), State: assign.StateOK, Tuner: 0, PlannedStart: at(30).Add(5 * time.Second), PlannedEnd: at(60).Add(margin)},
			},
		},
		{
			name:         "a second tuner is chosen when it avoids trimming",
			reservations: []*assign.Reservation{program(1, "27", 0, 30), program(2, "26", 30, 30)},
			tuners:       grTuners(2),
			want: []*assign.Assignment{
				{ReservationID: reservationID(1), State: assign.StateOK, Tuner: 0, PlannedStart: at(0).Add(-margin), PlannedEnd: at(30).Add(margin)},
				{ReservationID: reservationID(2), State: assign.StateOK, Tuner: 1, PlannedStart: at(30).Add(-margin), PlannedEnd: at(60).Add(margin)},
			},
		},
		{
			name:         "same channel shares one tuner",
			reservations: []*assign.Reservation{program(1, "27", 0, 60), program(2, "27", 30, 60)},
			tuners:       grTuners(1),
			want: []*assign.Assignment{
				{ReservationID: reservationID(1), State: assign.StateOK, Tuner: 0, PlannedStart: at(0).Add(-margin), PlannedEnd: at(60).Add(margin)},
				{ReservationID: reservationID(2), State: assign.StateOK, Tuner: 0, PlannedStart: at(30).Add(-margin), PlannedEnd: at(90).Add(margin)},
			},
		},
		{
			name: "higher priority wins and the lower one is cut partially",
			reservations: []*assign.Reservation{
				program(1, "27", 0, 60),
				withPriority(program(2, "26", 30, 60), 5),
			},
			tuners: grTuners(1),
			want: []*assign.Assignment{
				// 優先度の低い方の本編を削る。本編はマージンより優先するので、高い方の開始マージンは、低い方の残った本編に譲る。
				{ReservationID: reservationID(1), State: assign.StatePartial, Tuner: 0, PlannedStart: at(0).Add(-margin), PlannedEnd: at(30)},
				{ReservationID: reservationID(2), State: assign.StateOK, Tuner: 0, PlannedStart: at(30), PlannedEnd: at(90).Add(margin)},
			},
		},
		{
			name:         "conflict when the core is covered entirely",
			reservations: []*assign.Reservation{program(1, "27", 0, 60), program(2, "26", 10, 30)},
			tuners:       grTuners(1),
			want: []*assign.Assignment{
				{ReservationID: reservationID(1), State: assign.StateOK, Tuner: 0, PlannedStart: at(0).Add(-margin), PlannedEnd: at(60).Add(margin)},
				{ReservationID: reservationID(2), State: assign.StateConflict, Tuner: assign.NoTuner, PlannedStart: at(10).Add(-margin), PlannedEnd: at(40).Add(margin)},
			},
		},
		{
			name: "concurrent programs on the same channel are counted once",
			reservations: []*assign.Reservation{
				withPriority(program(1, "27", 0, 60), 10),
				withPriority(program(2, "27", 0, 60), 10),
				program(3, "26", 0, 90),
			},
			tuners: grTuners(1),
			want: []*assign.Assignment{
				{ReservationID: reservationID(1), State: assign.StateOK, Tuner: 0, PlannedStart: at(0).Add(-margin), PlannedEnd: at(60)},
				{ReservationID: reservationID(2), State: assign.StateOK, Tuner: 0, PlannedStart: at(0).Add(-margin), PlannedEnd: at(60)},
				{ReservationID: reservationID(3), State: assign.StatePartial, Tuner: 0, PlannedStart: at(60), PlannedEnd: at(90).Add(margin)},
			},
		},
		{
			name: "the margin of the winner does not swallow the remaining core of the loser",
			reservations: []*assign.Reservation{
				func() *assign.Reservation {
					r := withPriority(program(1, "27", 0, 30), 10)
					r.EndMargin = 15 * time.Minute

					return r
				}(),
				func() *assign.Reservation {
					r := program(2, "26", 20, 20)
					r.StartMargin, r.EndMargin = 0, 0

					return r
				}(),
			},
			tuners: grTuners(1),
			want: []*assign.Assignment{
				{ReservationID: reservationID(1), State: assign.StateOK, Tuner: 0, PlannedStart: at(0).Add(-margin), PlannedEnd: at(30)},
				{ReservationID: reservationID(2), State: assign.StatePartial, Tuner: 0, PlannedStart: at(30), PlannedEnd: at(40)},
			},
		},
		{
			name: "conflict when no tuner receives the channel type",
			reservations: []*assign.Reservation{
				onBS(program(1, "BS15_0", 0, 30)),
			},
			tuners: grTuners(2),
			want: []*assign.Assignment{
				{ReservationID: reservationID(1), State: assign.StateConflict, Tuner: assign.NoTuner, PlannedStart: at(0).Add(-margin), PlannedEnd: at(30).Add(margin)},
			},
		},
		{
			name: "recording reservation keeps its tuner even against a higher priority",
			reservations: []*assign.Reservation{
				recordingOn(program(1, "27", -10, 60), 1),
				withPriority(program(2, "26", 0, 30), 10),
			},
			tuners: grTuners(2),
			want: []*assign.Assignment{
				{ReservationID: reservationID(1), State: assign.StateOK, Tuner: 1, PlannedStart: at(-10).Add(-margin), PlannedEnd: at(50).Add(margin)},
				{ReservationID: reservationID(2), State: assign.StateOK, Tuner: 0, PlannedStart: at(0).Add(-margin), PlannedEnd: at(30).Add(margin)},
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

// genInput は、何本かの GR と BS のチューナーと、3 つのチャンネルにまたがる予約を作る。録画が始まった予約も混ぜる。
func genInput(t *rapid.T) ([]*assign.Reservation, []*assign.Tuner) {
	tunerCount := rapid.IntRange(0, 3).Draw(t, "tuners")
	tuners := make([]*assign.Tuner, 0, tunerCount)
	for i := range tunerCount {
		types := rapid.SampledFrom([][]string{{typeGR}, {typeBS}, {typeGR, typeBS}}).Draw(t, fmt.Sprintf("types%d", i))
		tuners = append(tuners, &assign.Tuner{Index: i, Types: types})
	}

	reservationCount := rapid.IntRange(0, 12).Draw(t, "reservations")
	reservations := make([]*assign.Reservation, 0, reservationCount)
	// 録画中のチューナーは、チャンネルごとに 1 本にする。同じチャンネルの録画は 1 本のチューナーを共有するからである。
	usedTuners := map[int]bool{}
	recordingTuner := map[string]int{}
	for i := range reservationCount {
		channel := rapid.SampledFrom([]string{"27", "26", "BS"}).Draw(t, fmt.Sprintf("channel%d", i))
		channelType := typeGR
		if channel == "BS" {
			channelType = typeBS
		}
		start := base().Add(time.Duration(rapid.IntRange(0, 300).Draw(t, fmt.Sprintf("start%d", i))) * time.Minute)
		r := &assign.Reservation{
			ID:          reservationID(i + 1),
			ChannelType: channelType,
			Channel:     channel,
			StartAt:     start,
			EndAt:       start.Add(time.Duration(rapid.IntRange(1, 120).Draw(t, fmt.Sprintf("minutes%d", i))) * time.Minute),
			StartMargin: time.Duration(rapid.IntRange(0, 120).Draw(t, fmt.Sprintf("startMargin%d", i))) * time.Second,
			EndMargin:   time.Duration(rapid.IntRange(0, 120).Draw(t, fmt.Sprintf("endMargin%d", i))) * time.Second,
			Priority:    rapid.IntRange(-2, 2).Draw(t, fmt.Sprintf("priority%d", i)),
			Recording:   false,
			Tuner:       0,
		}
		// 録画が始まった予約は、受信できる、別のチャンネルの録画に使っていないチューナーで録っていることにする。
		if rapid.Bool().Draw(t, fmt.Sprintf("recording%d", i)) {
			if tuner, ok := recordingTuner[channel]; ok {
				r.Recording, r.Tuner = true, tuner
			} else {
				for _, tuner := range tuners {
					if !usedTuners[tuner.Index] && slices.Contains(tuner.Types, channelType) {
						r.Recording, r.Tuner = true, tuner.Index
						usedTuners[tuner.Index] = true
						recordingTuner[channel] = tuner.Index

						break
					}
				}
			}
		}
		reservations = append(reservations, r)
	}

	return reservations, tuners
}

func byID(assignments []*assign.Assignment) map[uuid.UUID]*assign.Assignment {
	result := map[uuid.UUID]*assign.Assignment{}
	for _, a := range assignments {
		result[a.ReservationID] = a
	}

	return result
}

// TestAssignProperties は、どんな入力でも成り立つべき性質を確かめる。
func TestAssignProperties(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		reservations, tuners := genInput(t)
		assignments := assign.Assign(reservations, tuners)
		got := byID(assignments)
		if len(got) != len(reservations) {
			t.Fatalf("got %d assignments for %d reservations", len(got), len(reservations))
		}

		// 同じ入力からは、並びが違っても同じ結果が出る。
		shuffled := rapid.Permutation(reservations).Draw(t, "shuffled")
		if again := byID(assign.Assign(shuffled, tuners)); !assert.ObjectsAreEqual(got, again) {
			t.Fatalf("the result depends on the order of the input")
		}

		for _, r := range reservations {
			checkAssignment(t, r, got, reservations, tuners)
		}
	})
}

// checkAssignment は、予約 r の割り当てが、どんな入力でも成り立つべき性質を満たすかを確かめる。
func checkAssignment(t *rapid.T, r *assign.Reservation, got map[uuid.UUID]*assign.Assignment, reservations []*assign.Reservation, tuners []*assign.Tuner) {
	a := got[r.ID]
	// 録画が始まった予約は、そのチューナーから動かさず、削りもしない。
	if r.Recording && (a.Tuner != r.Tuner || a.State != assign.StateOK) {
		t.Fatalf("recording reservation %s was moved or cut: %+v", r.ID, a)
	}
	// 予定の時刻は、マージンを含めた区間からはみ出さない。
	if a.PlannedStart.Before(r.StartAt.Add(-r.StartMargin)) || a.PlannedEnd.After(r.EndAt.Add(r.EndMargin)) {
		t.Fatalf("planned time of %s exceeds its margins: %+v", r.ID, a)
	}
	// 正常なら、本編をすべて録る。
	if a.State == assign.StateOK && (a.PlannedStart.After(r.StartAt) || a.PlannedEnd.Before(r.EndAt)) {
		t.Fatalf("reservation %s is ok but its core is cut: %+v", r.ID, a)
	}
	if a.State == assign.StateConflict {
		return
	}
	// 割り当てたチューナーは、そのチャンネルの種別を受信できる。
	tuner := tuners[slices.IndexFunc(tuners, func(tuner *assign.Tuner) bool { return tuner.Index == a.Tuner })]
	if !slices.Contains(tuner.Types, r.ChannelType) {
		t.Fatalf("reservation %s is assigned to tuner %d that cannot receive %s", r.ID, a.Tuner, r.ChannelType)
	}
	// 同じチューナーで別のチャンネルを録る時間は重ならない。
	for _, other := range reservations {
		b := got[other.ID]
		if other.ID == r.ID || b.Tuner != a.Tuner || other.Channel == r.Channel {
			continue
		}
		if a.PlannedStart.Before(b.PlannedEnd) && b.PlannedStart.Before(a.PlannedEnd) {
			t.Fatalf("reservations %s and %s overlap on tuner %d: %+v %+v", r.ID, other.ID, a.Tuner, a, b)
		}
	}
}

// TestAssignDoesNotCutCoresForMarginsOnly は、本編どうしが重ならない予約が 1 本のチューナーに収まるとき、
// マージンの重なりだけで本編を削らないことを確かめる。
func TestAssignDoesNotCutCoresForMarginsOnly(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		count := rapid.IntRange(1, 10).Draw(t, "programs")
		reservations := make([]*assign.Reservation, 0, count)
		next := base()
		for i := range count {
			start := next.Add(time.Duration(rapid.IntRange(0, 5).Draw(t, fmt.Sprintf("gap%d", i))) * time.Minute)
			end := start.Add(time.Duration(rapid.IntRange(1, 60).Draw(t, fmt.Sprintf("minutes%d", i))) * time.Minute)
			reservations = append(reservations, &assign.Reservation{
				ID: reservationID(i + 1), ChannelType: typeGR, Channel: rapid.SampledFrom([]string{"27", "26", "25"}).Draw(t, fmt.Sprintf("channel%d", i)),
				StartAt: start, EndAt: end,
				StartMargin: time.Duration(rapid.IntRange(0, 300).Draw(t, fmt.Sprintf("startMargin%d", i))) * time.Second,
				EndMargin:   time.Duration(rapid.IntRange(0, 300).Draw(t, fmt.Sprintf("endMargin%d", i))) * time.Second,
				Priority:    rapid.IntRange(-2, 2).Draw(t, fmt.Sprintf("priority%d", i)),
				Recording:   false, Tuner: 0,
			})
			next = end
		}

		for _, a := range assign.Assign(reservations, grTuners(1)) {
			if a.State != assign.StateOK {
				t.Fatalf("reservation %s is %s although only margins overlap", a.ReservationID, a.State)
			}
		}
	})
}

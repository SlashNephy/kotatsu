package sleep_test

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/usecase/sleep"
)

// baseTime は、テストの今の時刻。
func baseTime() time.Time {
	return time.Date(2026, 11, 12, 3, 0, 0, 0, time.UTC)
}

// TestDecide は、スリープさせないかと、起こす予約の時刻の判定を確かめる (#1522、設計書 11.7)。
func TestDecide(t *testing.T) {
	t.Parallel()

	base := baseTime()
	opts := &sleep.Options{Interval: 0, WakeLead: 5 * time.Minute, InhibitLead: 10 * time.Minute}
	tests := []struct {
		name        string
		activity    sleep.Activity
		now         time.Time
		wantInhibit bool
		wantReasons []string
		wantWakeAt  time.Time
	}{
		{
			name:        "nothing to do lets the system sleep",
			activity:    sleep.Activity{Recordings: 0, Encodings: 0, Viewers: 0, Airing: 0, NextStart: time.Time{}},
			now:         base,
			wantInhibit: false, wantReasons: []string{}, wantWakeAt: time.Time{},
		},
		{
			name:        "a recording in progress",
			activity:    sleep.Activity{Recordings: 1, Encodings: 0, Viewers: 0, Airing: 0, NextStart: time.Time{}},
			now:         base,
			wantInhibit: true, wantReasons: []string{sleep.ReasonRecording}, wantWakeAt: time.Time{},
		},
		{
			name:        "a post-processing job running",
			activity:    sleep.Activity{Recordings: 0, Encodings: 2, Viewers: 0, Airing: 0, NextStart: time.Time{}},
			now:         base,
			wantInhibit: true, wantReasons: []string{sleep.ReasonEncoding}, wantWakeAt: time.Time{},
		},
		{
			name:        "someone watching live",
			activity:    sleep.Activity{Recordings: 0, Encodings: 0, Viewers: 1, Airing: 0, NextStart: time.Time{}},
			now:         base,
			wantInhibit: true, wantReasons: []string{sleep.ReasonWatching}, wantWakeAt: time.Time{},
		},
		{
			name:        "a reservation in its planned time that mikan has not taken yet",
			activity:    sleep.Activity{Recordings: 0, Encodings: 0, Viewers: 0, Airing: 1, NextStart: time.Time{}},
			now:         base,
			wantInhibit: true, wantReasons: []string{sleep.ReasonDue}, wantWakeAt: time.Time{},
		},
		{
			name:        "the next recording just outside the inhibit lead is not a reason, and a wake is scheduled",
			activity:    sleep.Activity{Recordings: 0, Encodings: 0, Viewers: 0, Airing: 0, NextStart: base.Add(10 * time.Minute)},
			now:         base,
			wantInhibit: false, wantReasons: []string{}, wantWakeAt: base.Add(5 * time.Minute),
		},
		{
			name:        "the next recording just inside the inhibit lead",
			activity:    sleep.Activity{Recordings: 0, Encodings: 0, Viewers: 0, Airing: 0, NextStart: base.Add(10*time.Minute - time.Nanosecond)},
			now:         base,
			wantInhibit: true, wantReasons: []string{sleep.ReasonNextIsClose}, wantWakeAt: base.Add(5*time.Minute - time.Second),
		},
		{
			name:        "the wake time is truncated to the second",
			activity:    sleep.Activity{Recordings: 0, Encodings: 0, Viewers: 0, Airing: 0, NextStart: base.Add(time.Hour + 1500*time.Millisecond)},
			now:         base,
			wantInhibit: false, wantReasons: []string{}, wantWakeAt: base.Add(55*time.Minute + time.Second),
		},
		{
			name:        "no wake once the wake time has come",
			activity:    sleep.Activity{Recordings: 0, Encodings: 0, Viewers: 0, Airing: 0, NextStart: base.Add(5 * time.Minute)},
			now:         base,
			wantInhibit: true, wantReasons: []string{sleep.ReasonNextIsClose}, wantWakeAt: time.Time{},
		},
		{
			name:        "every reason at once",
			activity:    sleep.Activity{Recordings: 1, Encodings: 1, Viewers: 1, Airing: 1, NextStart: base.Add(time.Minute)},
			now:         base,
			wantInhibit: true,
			wantReasons: []string{sleep.ReasonRecording, sleep.ReasonEncoding, sleep.ReasonWatching, sleep.ReasonDue, sleep.ReasonNextIsClose},
			wantWakeAt:  time.Time{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := sleep.Decide(&tt.activity, tt.now, opts)
			assert.Equal(t, tt.wantInhibit, d.Inhibit)
			assert.Equal(t, tt.wantReasons, d.Reasons)
			assert.True(t, tt.wantWakeAt.Equal(d.WakeAt), "wake at %s, want %s", d.WakeAt, tt.wantWakeAt)
		})
	}
}

// osCall は、1 回の判定や止めたときに、OS に伝えるはずのこと。伝えたら err を返す期待を m に置き、その期待を返す。
type osCall func(m *MockSystem, err error) any

// inhibit は、スリープの抑止を取ること。
func inhibit(m *MockSystem, err error) any {
	return m.EXPECT().SetInhibit(true).Return(err)
}

// release は、スリープの抑止をやめること。
func release(m *MockSystem, err error) any {
	return m.EXPECT().SetInhibit(false).Return(err)
}

// cancelWake は、起こす予約を消すこと。
func cancelWake(m *MockSystem, err error) any {
	return m.EXPECT().CancelWake().Return(err)
}

// scheduleWake は、at に起こす予約を置くこと。
func scheduleWake(at time.Time) osCall {
	return func(m *MockSystem, err error) any {
		return m.EXPECT().ScheduleWake(gomock.Cond(func(got time.Time) bool { return got.Equal(at) })).Return(err)
	}
}

// expectInOrder は、calls を、この順に 1 回ずつ伝えるという期待を m に置く。
func expectInOrder(m *MockSystem, calls []osCall, err error) {
	expected := make([]any, 0, len(calls))
	for _, call := range calls {
		expected = append(expected, call(m, err))
	}
	gomock.InOrder(expected...)
}

// TestController は、状態が変わったときだけ OS に伝えることと、伝えられなければ次の判定でもう一度試すことを確かめる。
func TestController(t *testing.T) {
	t.Parallel()

	base := baseTime()
	idle := func() *sleep.Activity {
		return &sleep.Activity{Recordings: 0, Encodings: 0, Viewers: 0, Airing: 0, NextStart: time.Time{}}
	}
	type step struct {
		// advance だけ時計を進めてから判定する。
		advance  time.Duration
		activity *sleep.Activity
		readErr  error
		osErr    error
		want     []osCall
	}
	tests := []struct {
		name  string
		steps []step
		// stop は、最後に止めたときに OS に伝えること。
		stop []osCall
	}{
		{
			name: "takes the inhibition once while recording and releases it after",
			steps: []step{
				{advance: 0, activity: &sleep.Activity{Recordings: 1, Encodings: 0, Viewers: 0, Airing: 0, NextStart: time.Time{}}, want: []osCall{inhibit}},
				{advance: 30 * time.Second, activity: &sleep.Activity{Recordings: 1, Encodings: 1, Viewers: 0, Airing: 0, NextStart: time.Time{}}, want: []osCall{}},
				{advance: 30 * time.Second, activity: idle(), want: []osCall{release}},
				{advance: 30 * time.Second, activity: idle(), want: []osCall{}},
			},
			stop: []osCall{cancelWake},
		},
		{
			name: "schedules the wake once and rewrites it when the next recording changes",
			steps: []step{
				{advance: 0, activity: &sleep.Activity{Recordings: 0, Encodings: 0, Viewers: 0, Airing: 0, NextStart: base.Add(time.Hour)}, want: []osCall{scheduleWake(base.Add(55 * time.Minute))}},
				{advance: 30 * time.Second, activity: &sleep.Activity{Recordings: 0, Encodings: 0, Viewers: 0, Airing: 0, NextStart: base.Add(time.Hour)}, want: []osCall{}},
				{advance: 30 * time.Second, activity: &sleep.Activity{Recordings: 0, Encodings: 0, Viewers: 0, Airing: 0, NextStart: base.Add(2 * time.Hour)}, want: []osCall{scheduleWake(base.Add(115 * time.Minute))}},
				{advance: 30 * time.Second, activity: idle(), want: []osCall{cancelWake}},
			},
			stop: []osCall{cancelWake},
		},
		{
			name: "forgets the wake that has come without cancelling it, and releases the inhibition on stop",
			steps: []step{
				{advance: 0, activity: &sleep.Activity{Recordings: 0, Encodings: 0, Viewers: 0, Airing: 0, NextStart: base.Add(20 * time.Minute)}, want: []osCall{scheduleWake(base.Add(15 * time.Minute))}},
				// 起こす時刻を過ぎて起き、抑止の前倒しに入った。予約は OS が使い終えているので、消さない。
				{advance: 16 * time.Minute, activity: &sleep.Activity{Recordings: 0, Encodings: 0, Viewers: 0, Airing: 0, NextStart: base.Add(20 * time.Minute)}, want: []osCall{inhibit}},
			},
			stop: []osCall{release, cancelWake},
		},
		{
			name: "retries what it could not tell the OS at the next check",
			steps: []step{
				{
					advance: 0, activity: &sleep.Activity{Recordings: 1, Encodings: 0, Viewers: 0, Airing: 0, NextStart: base.Add(time.Hour)}, osErr: errors.New("denied"),
					want: []osCall{inhibit, scheduleWake(base.Add(55 * time.Minute))},
				},
				{
					advance: 30 * time.Second, activity: &sleep.Activity{Recordings: 1, Encodings: 0, Viewers: 0, Airing: 0, NextStart: base.Add(time.Hour)},
					want: []osCall{inhibit, scheduleWake(base.Add(55 * time.Minute))},
				},
				{advance: 30 * time.Second, activity: &sleep.Activity{Recordings: 1, Encodings: 0, Viewers: 0, Airing: 0, NextStart: base.Add(time.Hour)}, want: []osCall{}},
			},
			stop: []osCall{release, cancelWake},
		},
		{
			name: "keeps the current state when the activity cannot be read",
			steps: []step{
				{advance: 0, activity: &sleep.Activity{Recordings: 1, Encodings: 0, Viewers: 0, Airing: 0, NextStart: time.Time{}}, want: []osCall{inhibit}},
				{advance: 30 * time.Second, activity: nil, readErr: errors.New("database is down"), want: []osCall{}},
				{advance: 30 * time.Second, activity: idle(), want: []osCall{release}},
			},
			stop: []osCall{cancelWake},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			clk := clocktest.NewFake(base)
			ctrl := gomock.NewController(t)
			source := NewMockSource(ctrl)
			system := NewMockSystem(ctrl)
			c := sleep.NewController(source, system, clk, slog.New(slog.DiscardHandler),
				&sleep.Options{Interval: 0, WakeLead: 5 * time.Minute, InhibitLead: 10 * time.Minute})
			for i, s := range tt.steps {
				clk.Advance(s.advance)
				source.EXPECT().Activity(gomock.Any(), gomock.Any()).Return(s.activity, s.readErr)
				expectInOrder(system, s.want, s.osErr)
				c.Step(t.Context())
				// 伝えなかったことがあれば、この判定で落とす。伝えるはずでないことは、伝えた時点で落ちる。
				assert.True(t, ctrl.Satisfied(), "step %d", i)
			}
			expectInOrder(system, tt.stop, nil)
			c.Stop(t.Context())
			assert.True(t, ctrl.Satisfied(), "stop")
		})
	}
}

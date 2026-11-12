package sleep_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/usecase/sleep"
)

// fakeSystem が記録する、OS に伝えたこと。
const (
	callInhibit = "inhibit"
	callRelease = "release"
	callCancel  = "cancel"
	// callScheduleAt0355 は、03:55:00 に起こす予約を置いたこと。
	callScheduleAt0355 = "schedule 03:55:00"
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

// fakeSource は、決めた Activity か、エラーを返す。
type fakeSource struct {
	activity *sleep.Activity
	err      error
}

func (s *fakeSource) Activity(context.Context, time.Time) (*sleep.Activity, error) {
	return s.activity, s.err
}

// fakeSystem は、OS に伝えたことを記録する。err を入れると、どの呼び出しも失敗する。
type fakeSystem struct {
	calls []string
	err   error
}

func (s *fakeSystem) SetInhibit(on bool) error {
	if on {
		s.calls = append(s.calls, callInhibit)
	} else {
		s.calls = append(s.calls, callRelease)
	}

	return s.err
}

func (s *fakeSystem) ScheduleWake(at time.Time) error {
	s.calls = append(s.calls, "schedule "+at.Format(time.TimeOnly))

	return s.err
}

func (s *fakeSystem) CancelWake() error {
	s.calls = append(s.calls, callCancel)

	return s.err
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
		want     []string
	}
	tests := []struct {
		name  string
		steps []step
		// stop は、最後に止めたときに OS に伝えること。
		stop []string
	}{
		{
			name: "takes the inhibition once while recording and releases it after",
			steps: []step{
				{advance: 0, activity: &sleep.Activity{Recordings: 1, Encodings: 0, Viewers: 0, Airing: 0, NextStart: time.Time{}}, want: []string{callInhibit}},
				{advance: 30 * time.Second, activity: &sleep.Activity{Recordings: 1, Encodings: 1, Viewers: 0, Airing: 0, NextStart: time.Time{}}, want: []string{}},
				{advance: 30 * time.Second, activity: idle(), want: []string{callRelease}},
				{advance: 30 * time.Second, activity: idle(), want: []string{}},
			},
			stop: []string{callCancel},
		},
		{
			name: "schedules the wake once and rewrites it when the next recording changes",
			steps: []step{
				{advance: 0, activity: &sleep.Activity{Recordings: 0, Encodings: 0, Viewers: 0, Airing: 0, NextStart: base.Add(time.Hour)}, want: []string{callScheduleAt0355}},
				{advance: 30 * time.Second, activity: &sleep.Activity{Recordings: 0, Encodings: 0, Viewers: 0, Airing: 0, NextStart: base.Add(time.Hour)}, want: []string{}},
				{advance: 30 * time.Second, activity: &sleep.Activity{Recordings: 0, Encodings: 0, Viewers: 0, Airing: 0, NextStart: base.Add(2 * time.Hour)}, want: []string{"schedule 04:55:00"}},
				{advance: 30 * time.Second, activity: idle(), want: []string{callCancel}},
			},
			stop: []string{callCancel},
		},
		{
			name: "forgets the wake that has come without cancelling it, and releases the inhibition on stop",
			steps: []step{
				{advance: 0, activity: &sleep.Activity{Recordings: 0, Encodings: 0, Viewers: 0, Airing: 0, NextStart: base.Add(20 * time.Minute)}, want: []string{"schedule 03:15:00"}},
				// 起こす時刻を過ぎて起き、抑止の前倒しに入った。予約は OS が使い終えているので、消さない。
				{advance: 16 * time.Minute, activity: &sleep.Activity{Recordings: 0, Encodings: 0, Viewers: 0, Airing: 0, NextStart: base.Add(20 * time.Minute)}, want: []string{callInhibit}},
			},
			stop: []string{callRelease, callCancel},
		},
		{
			name: "retries what it could not tell the OS at the next check",
			steps: []step{
				{
					advance: 0, activity: &sleep.Activity{Recordings: 1, Encodings: 0, Viewers: 0, Airing: 0, NextStart: base.Add(time.Hour)}, osErr: errors.New("denied"),
					want: []string{callInhibit, callScheduleAt0355},
				},
				{
					advance: 30 * time.Second, activity: &sleep.Activity{Recordings: 1, Encodings: 0, Viewers: 0, Airing: 0, NextStart: base.Add(time.Hour)},
					want: []string{callInhibit, callScheduleAt0355},
				},
				{advance: 30 * time.Second, activity: &sleep.Activity{Recordings: 1, Encodings: 0, Viewers: 0, Airing: 0, NextStart: base.Add(time.Hour)}, want: []string{}},
			},
			stop: []string{callRelease, callCancel},
		},
		{
			name: "keeps the current state when the activity cannot be read",
			steps: []step{
				{advance: 0, activity: &sleep.Activity{Recordings: 1, Encodings: 0, Viewers: 0, Airing: 0, NextStart: time.Time{}}, want: []string{callInhibit}},
				{advance: 30 * time.Second, activity: nil, readErr: errors.New("database is down"), want: []string{}},
				{advance: 30 * time.Second, activity: idle(), want: []string{callRelease}},
			},
			stop: []string{callCancel},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			clk := clocktest.NewFake(base)
			source := &fakeSource{activity: nil, err: nil}
			system := &fakeSystem{calls: []string{}, err: nil}
			c := sleep.NewController(source, system, clk, slog.New(slog.DiscardHandler),
				&sleep.Options{Interval: 0, WakeLead: 5 * time.Minute, InhibitLead: 10 * time.Minute})
			for i, s := range tt.steps {
				clk.Advance(s.advance)
				source.activity, source.err = s.activity, s.readErr
				system.calls, system.err = []string{}, s.osErr
				c.Step(t.Context())
				assert.Equal(t, s.want, system.calls, "step %d", i)
			}
			system.calls, system.err = []string{}, nil
			c.Stop(t.Context())
			assert.Equal(t, tt.stop, system.calls, "stop")
		})
	}
}

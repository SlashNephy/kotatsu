package tunerserver_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
)

// TestTunerOf は、利用者の ID から、その利用者が使っているチューナーを見つけることを確かめる (#1467)。
func TestTunerOf(t *testing.T) {
	t.Parallel()

	tuners := []*tunerserver.Tuner{
		{Index: 0, Name: "GR0", Users: []*tunerserver.TunerUser{{ID: "a", Priority: 0, Agent: ""}}},
		{Index: -1, Name: "remote / T0", Users: []*tunerserver.TunerUser{{ID: "b", Priority: 0, Agent: ""}, {ID: "c", Priority: 0, Agent: ""}}},
		nil,
		{Index: 2, Name: "GR2", Users: []*tunerserver.TunerUser{}},
	}
	tests := []struct {
		name   string
		userID string
		want   *tunerserver.Tuner
	}{
		{name: "ローカルのチューナー", userID: "a", want: tuners[0]},
		{name: "相乗りしているリモートのチューナー", userID: "c", want: tuners[1]},
		{name: "使っていない", userID: "d", want: nil},
		{name: "一覧に null が混じっていても飛ばす", userID: "e", want: nil},
		{name: "ID を返さないチューナーサーバー", userID: "", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Same(t, tt.want, tunerserver.TunerOf(tuners, tt.userID))
		})
	}
}

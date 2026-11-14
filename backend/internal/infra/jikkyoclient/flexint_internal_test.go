package jikkyoclient

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFlexIntUnmarshalJSON は、NX-Jikkyo が数でも文字列でも送る時刻を読み、読めない値でもコメントを捨てないことを確かめる (#1515)。
func TestFlexIntUnmarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
		want flexInt
	}{
		{name: "数", data: `1794647289`, want: 1794647289},
		{name: "数を書いた文字列", data: `"1794647289"`, want: 1794647289},
		{name: "数でない文字列", data: `"abc"`, want: 0},
		{name: "null", data: `null`, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got struct {
				Date flexInt `json:"date"`
			}
			require.NoError(t, json.Unmarshal([]byte(`{"date":`+tt.data+`}`), &got))
			assert.Equal(t, tt.want, got.Date)
		})
	}
}

package graph_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/graph"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/player"
)

const (
	registerPlayer   = `mutation($state: PlayerStateInput!) { registerPlayer(state: $state) { id current state { title paused } } }`
	updatePlayer     = `mutation($id: ID!, $state: PlayerStateInput!, $announce: Boolean!) { updatePlayer(id: $id, state: $state, announce: $announce) { id state { paused } } }`
	unregisterPlayer = `mutation($id: ID!) { unregisterPlayer(id: $id) }`
	sendCommand      = `mutation($id: ID!, $command: PlayerCommandInput!) { sendPlayerCommand(id: $id, command: $command) }`
	listPlayers      = `{ players { id current state { kind title paused volume audios { id label } } } }`
	varState         = "state"
	varCommand       = "command"
	varAnnounce      = "announce"
	livePlayerTitle  = "こたつのニュース"
	keyCommandType   = "type"
	keyVolume        = "volume"
	commandSeek      = "SEEK"
	commandSetVolume = "SET_VOLUME"
)

type playerItem struct {
	ID      string `json:"id"`
	Current bool   `json:"current"`
	State   struct {
		Kind   string  `json:"kind"`
		Title  string  `json:"title"`
		Paused bool    `json:"paused"`
		Volume float64 `json:"volume"`
		Audios []struct {
			ID    string `json:"id"`
			Label string `json:"label"`
		} `json:"audios"`
	} `json:"state"`
}

func liveState(paused bool) map[string]any {
	return map[string]any{
		"kind": "LIVE", "target": graph.GlobalID("Service", "3273601024"), "title": livePlayerTitle, "paused": paused,
		keyVolume: 0.8, "muted": false, "audios": []map[string]any{{"id": "main", "label": "主音声"}}, "audio": "main",
		"dataBroadcast": false, "keyGroups": []string{},
	}
}

func (f *fixture) registerPlayer(t *testing.T) string {
	t.Helper()

	var got struct {
		RegisterPlayer playerItem `json:"registerPlayer"`
	}
	require.Empty(t, f.query(t, registerPlayer, map[string]any{varState: liveState(false)}, &got))
	assert.True(t, got.RegisterPlayer.Current)

	return got.RegisterPlayer.ID
}

func (f *fixture) players(t *testing.T) []playerItem {
	t.Helper()

	var got struct {
		Players []playerItem `json:"players"`
	}
	require.Empty(t, f.query(t, listPlayers, nil, &got))

	return got.Players
}

// TestPlayers は、プレイヤーを登録した端末と、同じユーザーのほかの端末だけが、プレイヤーを見られることを確かめる (#459)。
func TestPlayers(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	playerID := f.registerPlayer(t)

	// 同じユーザーのほかの端末からは見えるが、自分の端末のものとはしない。
	device := f.withNewSession(t)
	listed := device.players(t)
	require.Len(t, listed, 1)
	assert.Equal(t, playerID, listed[0].ID)
	assert.False(t, listed[0].Current)
	assert.Equal(t, livePlayerTitle, listed[0].State.Title)
	assert.Equal(t, "主音声", listed[0].State.Audios[0].Label)
	assert.True(t, f.players(t)[0].Current)
	// ほかのユーザーには見えない。
	other := f.as(t, otherUser, auth.RoleMember)
	assert.Empty(t, other.players(t))

	// 状態を送れるのは、登録した端末だけ。
	vars := map[string]any{varID: playerID, varState: liveState(true), varAnnounce: true}
	assert.Equal(t, []string{codeNotFound}, device.query(t, updatePlayer, vars, nil))
	require.Empty(t, f.query(t, updatePlayer, vars, nil))
	assert.True(t, device.players(t)[0].State.Paused)

	// 送られてこなくなったら、閉じたとみなす。スリープから戻ったときのように、また送れば、一覧に戻る。
	f.clock.Advance(player.ExpireAfter + time.Second)
	assert.Empty(t, device.players(t))
	require.Empty(t, f.query(t, updatePlayer, vars, nil))
	assert.Len(t, device.players(t), 1)
	// 閉じたとみなしたプレイヤーは、次にどれかのプレイヤーを登録したときに消すので、もう送れない。
	f.clock.Advance(player.ExpireAfter + time.Second)
	again := f.registerPlayer(t)
	assert.Equal(t, []string{codeNotFound}, f.query(t, updatePlayer, vars, nil))

	// 取り消せるのも、登録した端末だけ。
	assert.Equal(t, []string{codeNotFound}, device.query(t, unregisterPlayer, map[string]any{varID: again}, nil))
	require.Empty(t, f.query(t, unregisterPlayer, map[string]any{varID: again}, nil))
	assert.Empty(t, device.players(t))
}

// TestPlayerRejectsInvalidInput は、大きすぎる状態と、要る値のない操作を断ることを確かめる。
func TestPlayerRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	playerID := f.registerPlayer(t)

	tooLoud := liveState(false)
	tooLoud[keyVolume] = 3
	negative := liveState(false)
	negative[keyVolume] = -0.1
	tooMany := liveState(false)
	keyGroups := make([]string, 9)
	for i := range keyGroups {
		keyGroups[i] = "basic"
	}
	tooMany["keyGroups"] = keyGroups
	for _, state := range []map[string]any{tooLoud, negative, tooMany} {
		assert.Equal(t, []string{codeBadUserInput}, f.query(t, registerPlayer, map[string]any{varState: state}, nil))
	}

	tests := []struct {
		name    string
		command map[string]any
	}{
		{name: "位置のないシーク", command: map[string]any{keyCommandType: commandSeek}},
		{name: "負の位置", command: map[string]any{keyCommandType: commandSeek, "position": -1}},
		{name: "大きすぎる音量", command: map[string]any{keyCommandType: commandSetVolume, keyVolume: 2.5}},
		{name: "負の音量", command: map[string]any{keyCommandType: commandSetVolume, keyVolume: -0.1}},
		{name: "音声のない切り替え", command: map[string]any{keyCommandType: "SELECT_AUDIO"}},
		{name: "値のないミュート", command: map[string]any{keyCommandType: "SET_MUTED"}},
		{name: "キーのない押下", command: map[string]any{keyCommandType: "PRESS_KEY"}},
	}
	for _, tt := range tests {
		assert.Equal(t, []string{codeBadUserInput}, f.query(t, sendCommand, map[string]any{varID: playerID, varCommand: tt.command}, nil), tt.name)
	}
}

// TestPlayerCommands は、同じユーザーのほかの端末から送った操作が、届いた順に、操作される端末にだけ届くことを確かめる。
func TestPlayerCommands(t *testing.T) {
	t.Parallel()

	f := newFixtureWithChanges(t)
	playerID := f.registerPlayer(t)
	// Web の画面と同じく after に 0 を渡す (設計書 7.7)。after がなければ、購読が受け付けられる前に送った操作は届かない。
	// SSE の購読は、応答のヘッダーが返った後にリゾルバーが動くので、ヘッダーを待っても、受け付けられたかは分からない (#849)。
	commands := f.subscribe(t, `subscription { playerCommands(id: "`+playerID+`", after: "0") { type position volume } }`)
	changes := f.subscribe(t, `subscription { playersChanged }`)
	assert.JSONEq(t, `{"playersChanged":1}`, string(next(t, changes)))

	remote := f.withNewSession(t)
	// ほかの端末のプレイヤーへの操作は、購読できない。
	denied := remote.subscribe(t, `subscription { playerCommands(id: "`+playerID+`") { type } }`)
	assert.Contains(t, string(next(t, denied)), codeNotFound)

	for _, command := range []map[string]any{
		{keyCommandType: "PAUSE"},
		{keyCommandType: commandSeek, "position": 120.5},
		{keyCommandType: commandSetVolume, keyVolume: 0.5},
	} {
		require.Empty(t, remote.query(t, sendCommand, map[string]any{varID: playerID, varCommand: command}, nil))
	}
	assert.JSONEq(t, `{"playerCommands":{"type":"PAUSE","position":null,"volume":null}}`, string(next(t, commands)))
	assert.JSONEq(t, `{"playerCommands":{"type":"SEEK","position":120.5,"volume":null}}`, string(next(t, commands)))
	assert.JSONEq(t, `{"playerCommands":{"type":"SET_VOLUME","position":null,"volume":0.5}}`, string(next(t, commands)))

	// 再生位置が進んだだけなら、一覧は読み直させない。状態が変わったときだけ知らせる。
	require.Empty(t, f.query(t, updatePlayer, map[string]any{varID: playerID, varState: liveState(false), varAnnounce: false}, nil))
	require.Empty(t, f.query(t, updatePlayer, map[string]any{varID: playerID, varState: liveState(true), varAnnounce: true}, nil))
	assert.JSONEq(t, `{"playersChanged":2}`, string(next(t, changes)))

	// 取り消したら、ほかの端末に一覧を読み直させる。
	require.Empty(t, f.query(t, unregisterPlayer, map[string]any{varID: playerID}, nil))
	assert.JSONEq(t, `{"playersChanged":3}`, string(next(t, changes)))

	// ほかのユーザーは、操作を送れない。
	other := f.as(t, otherUser, auth.RoleMember)
	assert.Equal(t, []string{codeNotFound}, other.query(t, sendCommand, map[string]any{varID: playerID, varCommand: map[string]any{keyCommandType: "PLAY"}}, nil))
}

// TestPlayerCommandsReplay は、つなぎ直すときに、最後に受け取った操作より後に届いた操作を、すべての種類について取りこぼさないことを確かめる。
func TestPlayerCommandsReplay(t *testing.T) {
	t.Parallel()

	f := newFixtureWithChanges(t)
	playerID := f.registerPlayer(t)
	remote := f.withNewSession(t)
	send := func(command map[string]any) {
		t.Helper()

		require.Empty(t, remote.query(t, sendCommand, map[string]any{varID: playerID, varCommand: command}, nil))
	}
	send(map[string]any{keyCommandType: "PLAY"})

	// 1 つめの操作を受け取った後に切れ、その間に操作が届いた。
	send(map[string]any{keyCommandType: "SELECT_AUDIO", "audio": "sub"})
	send(map[string]any{keyCommandType: "PRESS_KEY", "key": "d"})
	send(map[string]any{keyCommandType: "STOP"})
	commands := f.subscribe(t, `subscription { playerCommands(id: "`+playerID+`", after: "1") { id type audio key } }`)
	assert.JSONEq(t, `{"playerCommands":{"id":"2","type":"SELECT_AUDIO","audio":"sub","key":null}}`, string(next(t, commands)))
	assert.JSONEq(t, `{"playerCommands":{"id":"3","type":"PRESS_KEY","audio":null,"key":"d"}}`, string(next(t, commands)))
	assert.JSONEq(t, `{"playerCommands":{"id":"4","type":"STOP","audio":null,"key":null}}`, string(next(t, commands)))

	// つないだ後に届いた操作も、続けて受け取る。
	send(map[string]any{keyCommandType: "SET_MUTED", "muted": true})
	assert.JSONEq(t, `{"playerCommands":{"id":"5","type":"SET_MUTED","audio":null,"key":null}}`, string(next(t, commands)))
}

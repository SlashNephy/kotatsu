package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/player"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

const (
	playerType = "Player"
	// maxPlayerAudios、maxPlayerKeyGroups、maxPlayerText は、プレイヤーが送る状態の大きさの上限。DB に書くので、際限なく受け取らない。
	maxPlayerAudios    = 16
	maxPlayerKeyGroups = 8
	maxPlayerText      = 256
	// maxPlayerVolume は、音量の上限 (プレイヤーは 200% まで上げられる)。
	maxPlayerVolume = 2
)

var errPlayersUnavailable = errors.New("players are not configured")

func parsePlayerID(globalID string) (uuid.UUID, error) {
	typeName, raw, err := ParseGlobalID(globalID)
	if err != nil || typeName != playerType {
		return uuid.Nil, NewError(CodeBadUserInput, "invalid player ID")
	}
	playerID, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, NewError(CodeBadUserInput, "invalid player ID")
	}

	return playerID, nil
}

// playerState は、プレイヤーが送った状態を確かめ、DB に書く JSON にする。
func playerState(state *model.PlayerStateInput) ([]byte, error) {
	tooLong := func(s *string) bool { return s != nil && utf8.RuneCountInString(*s) > maxPlayerText }
	if len(state.Audios) > maxPlayerAudios || len(state.KeyGroups) > maxPlayerKeyGroups ||
		tooLong(state.Title) || tooLong(state.Audio) || utf8.RuneCountInString(state.Target) > maxPlayerText ||
		state.Volume < 0 || state.Volume > maxPlayerVolume {
		return nil, NewError(CodeBadUserInput, "the player state is invalid")
	}
	for _, audio := range state.Audios {
		if tooLong(&audio.ID) || tooLong(&audio.Label) {
			return nil, NewError(CodeBadUserInput, "the player state is invalid")
		}
	}
	for _, group := range state.KeyGroups {
		if tooLong(&group) {
			return nil, NewError(CodeBadUserInput, "the player state is invalid")
		}
	}
	data, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the player state: %w", err)
	}

	return data, nil
}

// playerCommand は、操作を確かめ、DB に書く JSON にする。操作の種類ごとに、要る値があるかを確かめる。
func playerCommand(command *model.PlayerCommandInput) ([]byte, error) {
	valid := true
	switch command.Type {
	case model.PlayerCommandTypeSeek:
		valid = command.Position != nil && *command.Position >= 0
	case model.PlayerCommandTypeSetVolume:
		valid = command.Volume != nil && *command.Volume >= 0 && *command.Volume <= maxPlayerVolume
	case model.PlayerCommandTypeSetMuted:
		valid = command.Muted != nil
	case model.PlayerCommandTypeSelectAudio:
		valid = command.Audio != nil && utf8.RuneCountInString(*command.Audio) <= maxPlayerText
	case model.PlayerCommandTypePressKey:
		valid = command.Key != nil && utf8.RuneCountInString(*command.Key) <= maxPlayerText
	case model.PlayerCommandTypeOpen:
		valid = openTarget(command.ServiceID, command.RecordingID)
	case model.PlayerCommandTypePlay, model.PlayerCommandTypePause, model.PlayerCommandTypeStop:
	default:
		valid = false
	}
	if !valid {
		return nil, NewError(CodeBadUserInput, "the player command is invalid")
	}
	data, err := json.Marshal(command)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the player command: %w", err)
	}

	return data, nil
}

// openTarget は、開かせるもの (サービスか録画のどちらか 1 つ) の ID が正しいかを確かめる (#802)。
// サービスや録画があるかは確かめない。開いた画面が、見つからないことを示す。
func openTarget(serviceID, recordingID *string) bool {
	switch {
	case serviceID != nil && recordingID == nil:
		_, err := parseServiceGlobalID(*serviceID)

		return err == nil
	case serviceID == nil && recordingID != nil:
		_, err := parseRecordingID(*recordingID)

		return err == nil
	default:
		return false
	}
}

func playerOf(p *player.Player, sessionID uuid.UUID) (*model.Player, error) {
	var state model.PlayerState
	if err := json.Unmarshal(p.State, &state); err != nil {
		return nil, fmt.Errorf("failed to decode the player state: %w", err)
	}

	return &model.Player{
		ID: GlobalID(playerType, p.ID.String()), Current: p.SessionID == sessionID, UserAgent: p.UserAgent,
		State: &state, StartedAt: p.StartedAt, LastSeenAt: p.LastSeenAt,
	}, nil
}

func playerCommandOf(c *player.Command) (*model.PlayerCommand, error) {
	var command model.PlayerCommand
	if err := json.Unmarshal(c.Command, &command); err != nil {
		return nil, fmt.Errorf("failed to decode the player command: %w", err)
	}
	command.ID = strconv.FormatInt(c.ID, 10)

	return &command, nil
}

func playerError(err error) error {
	if errors.Is(err, player.ErrNotFound) {
		return NewError(CodeNotFound, "player not found")
	}

	return err
}

// currentPlayer は、このセッションの、ID のプレイヤーを、一覧から引く。
func (r *Resolver) currentPlayer(ctx context.Context, principal *auth.Principal, playerID uuid.UUID) (*model.Player, error) {
	players, err := r.players.List(ctx, principal.UserID)
	if err != nil {
		return nil, err
	}
	for _, p := range players {
		if p.ID == playerID {
			return playerOf(p, principal.SessionID)
		}
	}

	return nil, NewError(CodeNotFound, "player not found")
}

// subscribePlayersChanged は、ログイン中のユーザーのプレイヤーの一覧の版を、今の版から送り、上がるたびに送る。
func (r *Resolver) subscribePlayersChanged(ctx context.Context) (<-chan int, error) {
	if r.players == nil || r.changes == nil {
		return nil, errPlayersUnavailable
	}
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	// 版を読む前に購読し、読んでから購読するまでの間の変更を取りこぼさないようにする。
	wake, unsubscribe := r.changes.Subscribe()
	channelVersion := r.changes.Version(player.NotifyChannel)
	version, err := r.players.Version(ctx, principal.UserID)
	if err != nil {
		unsubscribe()

		return nil, err
	}
	out := make(chan int, 1)
	out <- int(version)
	go func() {
		defer close(out)
		defer unsubscribe()
		for {
			select {
			case <-ctx.Done():
				return
			case <-wake:
			}
			latest := r.changes.Version(player.NotifyChannel)
			if latest <= channelVersion {
				continue
			}
			channelVersion = latest
			current, err := r.players.Version(ctx, principal.UserID)
			if err != nil {
				// 閉じれば、クライアントがつなぎ直す。
				return
			}
			// ほかのユーザーのプレイヤーの変化や、操作では、版は上がらない。
			if current <= version {
				continue
			}
			version = current
			select {
			case <-ctx.Done():
				return
			case out <- int(version):
			}
		}
	}()

	return out, nil
}

// subscribePlayerCommands は、このセッションのプレイヤーへの操作を、届いた順に送る。after があれば、その操作より後に
// 届いたものから送り、なければ、購読を始めた後に届いたものから送る。
func (r *Resolver) subscribePlayerCommands(ctx context.Context, id string, after *string) (<-chan *model.PlayerCommand, error) {
	if r.players == nil || r.changes == nil {
		return nil, errPlayersUnavailable
	}
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	playerID, err := parsePlayerID(id)
	if err != nil {
		return nil, err
	}
	if err := r.players.Owned(ctx, principal.SessionID, playerID); err != nil {
		return nil, playerError(err)
	}
	wake, unsubscribe := r.changes.Subscribe()
	channelVersion := r.changes.Version(player.NotifyChannel)
	last, err := r.players.LastCommandID(ctx, playerID)
	if err != nil {
		unsubscribe()

		return nil, err
	}
	from, replay, err := commandCursor(after, last)
	if err != nil {
		unsubscribe()

		return nil, err
	}
	out := make(chan *model.PlayerCommand, 1)
	go func() {
		defer close(out)
		defer unsubscribe()
		last := from
		for {
			if !replay {
				select {
				case <-ctx.Done():
					return
				case <-wake:
				}
				// ほかのチャンネルの変更では、操作を読まない。
				latest := r.changes.Version(player.NotifyChannel)
				if latest <= channelVersion {
					continue
				}
				channelVersion = latest
			}
			replay = false
			var ok bool
			if last, ok = r.deliverPlayerCommands(ctx, playerID, last, out); !ok {
				return
			}
		}
	}()

	return out, nil
}

// commandCursor は、どの操作の後から送るかを決める。after があれば、その操作の後から送り、まだ送っていない操作があれば
// すぐに送る (つなぎ直したとき)。なければ、今の最も新しい操作 last の後から送る。
func commandCursor(after *string, last int64) (int64, bool, error) {
	if after == nil {
		return last, false, nil
	}
	from, err := strconv.ParseInt(*after, 10, 64)
	if err != nil || from < 0 {
		return 0, false, NewError(CodeBadUserInput, "invalid command ID")
	}

	return min(from, last), from < last, nil
}

// deliverPlayerCommands は、after より後の操作を、届いた順に out へ送り、最後に送った操作の ID を返す。
// 読めなかったか、購読が終わったら false を返す。
func (r *Resolver) deliverPlayerCommands(ctx context.Context, playerID uuid.UUID, after int64, out chan<- *model.PlayerCommand) (int64, bool) {
	commands, err := r.players.Commands(ctx, playerID, after)
	if err != nil {
		return after, false
	}
	last := after
	for _, c := range commands {
		command, err := playerCommandOf(c)
		if err != nil {
			return last, false
		}
		select {
		case <-ctx.Done():
			return last, false
		case out <- command:
			last = c.ID
		}
	}

	return last, true
}

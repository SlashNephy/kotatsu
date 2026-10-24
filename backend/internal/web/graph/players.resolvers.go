package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// RegisterPlayer is the resolver for the registerPlayer field.
func (r *mutationResolver) RegisterPlayer(ctx context.Context, state model.PlayerStateInput) (*model.Player, error) {
	if r.players == nil {
		return nil, errPlayersUnavailable
	}
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	data, err := playerState(&state)
	if err != nil {
		return nil, err
	}
	playerID, err := r.players.Register(ctx, principal.UserID, principal.SessionID, data)
	if err != nil {
		return nil, err
	}

	return r.currentPlayer(ctx, principal, playerID)
}

// UpdatePlayer is the resolver for the updatePlayer field.
func (r *mutationResolver) UpdatePlayer(ctx context.Context, id string, state model.PlayerStateInput, announce bool) (*model.Player, error) {
	if r.players == nil {
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
	data, err := playerState(&state)
	if err != nil {
		return nil, err
	}
	if err := r.players.Update(ctx, principal.UserID, principal.SessionID, playerID, data, announce); err != nil {
		return nil, playerError(err)
	}

	return r.currentPlayer(ctx, principal, playerID)
}

// UnregisterPlayer is the resolver for the unregisterPlayer field.
func (r *mutationResolver) UnregisterPlayer(ctx context.Context, id string) (string, error) {
	if r.players == nil {
		return "", errPlayersUnavailable
	}
	principal, err := viewer(ctx)
	if err != nil {
		return "", err
	}
	playerID, err := parsePlayerID(id)
	if err != nil {
		return "", err
	}
	if err := r.players.Unregister(ctx, principal.UserID, principal.SessionID, playerID); err != nil {
		return "", playerError(err)
	}

	return id, nil
}

// SendPlayerCommand is the resolver for the sendPlayerCommand field.
func (r *mutationResolver) SendPlayerCommand(ctx context.Context, id string, command model.PlayerCommandInput) (bool, error) {
	if r.players == nil {
		return false, errPlayersUnavailable
	}
	principal, err := viewer(ctx)
	if err != nil {
		return false, err
	}
	playerID, err := parsePlayerID(id)
	if err != nil {
		return false, err
	}
	data, err := playerCommand(&command)
	if err != nil {
		return false, err
	}
	if err := r.players.Send(ctx, principal.UserID, playerID, data); err != nil {
		return false, playerError(err)
	}

	return true, nil
}

// Players is the resolver for the players field.
func (r *queryResolver) Players(ctx context.Context) ([]*model.Player, error) {
	if r.players == nil {
		return nil, errPlayersUnavailable
	}
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	players, err := r.players.List(ctx, principal.UserID)
	if err != nil {
		return nil, err
	}
	result := make([]*model.Player, 0, len(players))
	for _, p := range players {
		item, err := playerOf(p, principal.SessionID)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}

	return result, nil
}

// PlayersChanged is the resolver for the playersChanged field.
func (r *subscriptionResolver) PlayersChanged(ctx context.Context) (<-chan int, error) {
	return r.subscribePlayersChanged(ctx)
}

// PlayerCommands is the resolver for the playerCommands field.
func (r *subscriptionResolver) PlayerCommands(ctx context.Context, id string, after *string) (<-chan *model.PlayerCommand, error) {
	return r.subscribePlayerCommands(ctx, id, after)
}

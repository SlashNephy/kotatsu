package graph

import (
	"context"
	"errors"

	"github.com/SlashNephy/kotatsu/graph/model"
	"github.com/SlashNephy/kotatsu/internal/assign"
	"github.com/SlashNephy/kotatsu/internal/epg"
	"github.com/SlashNephy/kotatsu/internal/reserve"
	"github.com/SlashNephy/kotatsu/internal/rule"
	"github.com/SlashNephy/kotatsu/internal/tunerstate"
)

// Changes は、ロール間の変更の通知を、subscription の購読者に知らせる。*notify.Broadcaster が満たす。
type Changes interface {
	// Subscribe は、変更があると値を受け取れるチャンネルと、購読をやめる関数を返す。
	Subscribe() (<-chan struct{}, func())
	// Version は、チャンネルの今の版数を返す。
	Version(channel string) int64
}

// errChangesUnavailable は、変更の通知が設定されていないことを表す。
var errChangesUnavailable = errors.New("change notifications are not configured")

// dataChannels は、dataChanged で知らせるデータの種類と、変更の通知のチャンネルの対応。
func dataChannels() []struct {
	channel model.DataChannel
	notify  string
} {
	return []struct {
		channel model.DataChannel
		notify  string
	}{
		{channel: model.DataChannelEpg, notify: epg.NotifyChannel},
		{channel: model.DataChannelRules, notify: rule.NotifyChannel},
		{channel: model.DataChannelReservations, notify: reserve.NotifyChannel},
		{channel: model.DataChannelAssignments, notify: assign.NotifyChannel},
		{channel: model.DataChannelReservationConflicts, notify: assign.ConflictChannel},
		{channel: model.DataChannelTuners, notify: tunerstate.NotifyChannel},
	}
}

// DataChannels は、dataChanged で知らせる変更の通知のチャンネルを返す。tenban は、これらを見張る。
func DataChannels() []string {
	channels := make([]string, 0, len(dataChannels()))
	for _, c := range dataChannels() {
		channels = append(channels, c.notify)
	}

	return channels
}

// subscribeDataChanges は、購読を始めたときに今の版数を送り、その後は版数が上がったデータの種類を送る。
func (r *Resolver) subscribeDataChanges(ctx context.Context) (<-chan *model.DataChange, error) {
	if _, err := viewer(ctx); err != nil {
		return nil, err
	}
	if r.changes == nil {
		return nil, errChangesUnavailable
	}
	// 版数を読む前に購読し、読んだ後の変更を取りこぼさないようにする。
	wake, unsubscribe := r.changes.Subscribe()
	last := map[model.DataChannel]int64{}
	for _, c := range dataChannels() {
		last[c.channel] = r.changes.Version(c.notify)
	}

	out := make(chan *model.DataChange, len(dataChannels()))
	go func() {
		defer close(out)
		defer unsubscribe()
		send := func(change *model.DataChange) bool {
			select {
			case <-ctx.Done():
				return false
			case out <- change:
				return true
			}
		}
		for _, c := range dataChannels() {
			if !send(&model.DataChange{Channel: c.channel, Version: int(last[c.channel])}) {
				return
			}
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-wake:
			}
			for _, c := range dataChannels() {
				version := r.changes.Version(c.notify)
				if version <= last[c.channel] {
					continue
				}
				last[c.channel] = version
				if !send(&model.DataChange{Channel: c.channel, Version: int(version)}) {
					return
				}
			}
		}
	}()

	return out, nil
}

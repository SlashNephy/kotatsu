package graph

import (
	"context"
	"errors"
	"slices"

	"github.com/SlashNephy/kotatsu/internal/assign"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/epg"
	"github.com/SlashNephy/kotatsu/internal/notification"
	"github.com/SlashNephy/kotatsu/internal/notify"
	"github.com/SlashNephy/kotatsu/internal/player"
	"github.com/SlashNephy/kotatsu/internal/reserve"
	"github.com/SlashNephy/kotatsu/internal/rule"
	"github.com/SlashNephy/kotatsu/internal/tunerstate"
	"github.com/SlashNephy/kotatsu/internal/usersettings"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
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

// dataChannel は、dataChanged で知らせるデータの種類と、変更の通知のチャンネルの対応。adminOnly のものは、管理者にだけ知らせる。
type dataChannel struct {
	channel   model.DataChannel
	notify    string
	adminOnly bool
}

// dataChannels は、dataChanged で知らせるデータの種類の一覧。
func dataChannels() []dataChannel {
	return []dataChannel{
		{channel: model.DataChannelEpg, notify: epg.NotifyChannel, adminOnly: false},
		{channel: model.DataChannelRules, notify: rule.NotifyChannel, adminOnly: false},
		{channel: model.DataChannelReservations, notify: reserve.NotifyChannel, adminOnly: false},
		{channel: model.DataChannelAssignments, notify: assign.NotifyChannel, adminOnly: false},
		{channel: model.DataChannelReservationConflicts, notify: assign.ConflictChannel, adminOnly: false},
		{channel: model.DataChannelTuners, notify: tunerstate.NotifyChannel, adminOnly: false},
		{channel: model.DataChannelRecordings, notify: notify.RecordingsChannel, adminOnly: false},
		{channel: model.DataChannelAdmin, notify: notify.AuditChannel, adminOnly: true},
	}
}

// WatchedChannels は、subscription で知らせるのに使う、変更の通知のチャンネルを返す。tenban は、これらを見張る。
func WatchedChannels() []string {
	channels := make([]string, 0, len(dataChannels())+3)
	for _, c := range dataChannels() {
		channels = append(channels, c.notify)
	}

	return append(channels, notification.NotifyChannel, usersettings.NotifyChannel, player.NotifyChannel)
}

// subscribeDataChanges は、購読を始めたときに今の版数を送り、その後は版数が上がったデータの種類を送る。
func (r *Resolver) subscribeDataChanges(ctx context.Context) (<-chan *model.DataChange, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	if r.changes == nil {
		return nil, errChangesUnavailable
	}
	channels := dataChannels()
	if principal.Role != auth.RoleAdmin {
		channels = slices.DeleteFunc(channels, func(c dataChannel) bool { return c.adminOnly })
	}
	// 版数を読む前に購読し、読んだ後の変更を取りこぼさないようにする。
	wake, unsubscribe := r.changes.Subscribe()
	last := map[model.DataChannel]int64{}
	for _, c := range channels {
		last[c.channel] = r.changes.Version(c.notify)
	}

	out := make(chan *model.DataChange, len(channels))
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
		for _, c := range channels {
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
			for _, c := range channels {
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

// subscribeUnreadNotificationCount は、購読を始めたときに未読の通知の数を送り、その後は数が変わったときに送る。
// 通知が増えるか既読にするたびに、購読者ごとに数を読み直す。
func (r *Resolver) subscribeUnreadNotificationCount(ctx context.Context) (<-chan int, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	if r.changes == nil {
		return nil, errChangesUnavailable
	}
	// 数を読む前に購読し、読んだ後の変更を取りこぼさないようにする。
	wake, unsubscribe := r.changes.Subscribe()
	version := r.changes.Version(notification.NotifyChannel)
	count, err := r.inbox.UnreadCount(ctx, r.db, principal.UserID)
	if err != nil {
		unsubscribe()

		return nil, err
	}

	out := make(chan int, 1)
	out <- count
	go func() {
		defer close(out)
		defer unsubscribe()
		for {
			select {
			case <-ctx.Done():
				return
			case <-wake:
			}
			latest := r.changes.Version(notification.NotifyChannel)
			if latest <= version {
				continue
			}
			version = latest
			current, err := r.inbox.UnreadCount(ctx, r.db, principal.UserID)
			if err != nil {
				// 読めなければ、購読を終える。クライアントは、購読し直して今の数を受け取る。
				return
			}
			if current == count {
				continue
			}
			count = current
			select {
			case <-ctx.Done():
				return
			case out <- count:
			}
		}
	}()

	return out, nil
}

// subscribeViewerSettings は、購読を始めたときにログイン中のユーザーの設定の版数を送り、その後は版数が上がったときに送る。
// ほかのユーザーが設定を変えたときは、版数を読み直すだけで、何も送らない。
func (r *Resolver) subscribeViewerSettings(ctx context.Context) (<-chan int, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	if r.changes == nil {
		return nil, errChangesUnavailable
	}
	// 版数を読む前に購読し、読んだ後の変更を取りこぼさないようにする。
	wake, unsubscribe := r.changes.Subscribe()
	channelVersion := r.changes.Version(usersettings.NotifyChannel)
	version, err := usersettings.Version(ctx, r.db, principal.UserID)
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
			latest := r.changes.Version(usersettings.NotifyChannel)
			if latest <= channelVersion {
				continue
			}
			channelVersion = latest
			current, err := usersettings.Version(ctx, r.db, principal.UserID)
			if err != nil {
				// 読めなければ、購読を終える。クライアントは、購読し直して今の版数を受け取る。
				return
			}
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

// subscribeViewerEvents は、データの変更、設定の版数、未読の通知の数を、1 つの購読にまとめて送る。
// どれかの購読が終わったら、まとめた購読も終え、クライアントにつなぎ直させる。
func (r *Resolver) subscribeViewerEvents(ctx context.Context) (<-chan *model.ViewerEvent, error) {
	ctx, cancel := context.WithCancel(ctx)
	data, err := r.subscribeDataChanges(ctx)
	if err != nil {
		cancel()

		return nil, err
	}
	settings, err := r.subscribeViewerSettings(ctx)
	if err != nil {
		cancel()

		return nil, err
	}
	unread, err := r.subscribeUnreadNotificationCount(ctx)
	if err != nil {
		cancel()

		return nil, err
	}
	out := make(chan *model.ViewerEvent, cap(data)+2)
	go func() {
		defer close(out)
		defer cancel()
		for {
			var event *model.ViewerEvent
			select {
			case <-ctx.Done():
				return
			case change, ok := <-data:
				if !ok {
					return
				}
				event = &model.ViewerEvent{DataChange: change, ViewerSettingsVersion: nil, UnreadNotificationCount: nil}
			case version, ok := <-settings:
				if !ok {
					return
				}
				event = &model.ViewerEvent{DataChange: nil, ViewerSettingsVersion: &version, UnreadNotificationCount: nil}
			case count, ok := <-unread:
				if !ok {
					return
				}
				event = &model.ViewerEvent{DataChange: nil, ViewerSettingsVersion: nil, UnreadNotificationCount: &count}
			}
			select {
			case <-ctx.Done():
				return
			case out <- event:
			}
		}
	}()

	return out, nil
}

package lineup

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

const (
	// DefaultSameRoundInterval は、続けて選局できないチャンネルの判定で、前に数えた失敗からこの間に始めた失敗を、同じ 1 回と数える間隔
	// (#1519、設計書 付録 A)。録画は、断られても予定の終わりまで開き直すので、1 つの失敗した録画が、1 回の周回に当たる。
	// 続けて並んだ番組 (30 分ごとなど) の録画を、別々の回に数えないよう、それより長くする。
	DefaultSameRoundInterval = time.Hour
	// DefaultUnreachableThreshold は、続けて選局できないと決める、続けて受信できなかった回数 (#1519、設計書 付録 A)。
	DefaultUnreachableThreshold = 3
	// tuningLookback は、続けて選局できないチャンネルの判定に使う録画の期間。週に 1 回の番組でも、回数に届くよう長くする。
	tuningLookback = 30 * 24 * time.Hour
)

// TuningOutcome は、チャンネルを選局した 1 回の結果。録画の 1 つに当たる。
type TuningOutcome struct {
	// ChannelType と Channel は、選局したチャンネル (チューナーサーバーのチャンネル種別とチャンネル)。
	ChannelType string
	Channel     string
	// At は、選局を始めた時刻 (録画を始めた時刻)。
	At time.Time
	// Received は、受信できたか。false なら、チューナーサーバーに選局を断られた (tuning_refused)。
	Received bool
}

// UnreachableRule は、続けて選局できないチャンネルを決める規則 (#1519)。
type UnreachableRule struct {
	// SameRoundInterval は、前に数えた失敗からこの間に始めた失敗を、同じ 1 回と数える間隔。
	SameRoundInterval time.Duration
	// Threshold は、続けて選局できないと決める、続けて受信できなかった回数。
	Threshold int
}

// streak は、チャンネルごとの、続けて受信できなかった数。
type streak struct {
	count int
	// countedAt は、最後に数えた失敗の時刻。この時刻から SameRoundInterval の間の失敗は、同じ回とみなす。
	countedAt time.Time
	// startedAt は、続き始めた (最初に数えた) 失敗の時刻。
	startedAt time.Time
}

// Unreachable は、選局の結果 outcomes (始めた順) から、続けて選局できないチャンネルを、チャンネル種別ごとに、チャンネルの昇順で返す (#1519)。
//   - 選局を断られた回を数え、受信できたら 0 に戻す。前に数えた失敗から SameRoundInterval の間の失敗は、同じ 1 回と数える。
//     受信できた後の最初の失敗は、間が短くても 1 回と数える。
//   - Threshold 回続けて受信できず、かつ、続き始めてから、同じ種別のほかのチャンネルが 1 つでも受信できているときだけ、続けて選局できないと決める。
//     その種別が全部だめなら、原因はチャンネルではなく、アンテナかチューナーにあるからである。
func Unreachable(outcomes []TuningOutcome, rule UnreachableRule) map[string][]string {
	type key struct{ channelType, channel string }
	streaks := map[key]*streak{}
	for _, outcome := range outcomes {
		k := key{channelType: outcome.ChannelType, channel: outcome.Channel}
		if outcome.Received {
			delete(streaks, k)

			continue
		}
		s := streaks[k]
		if s == nil {
			s = &streak{count: 0, countedAt: time.Time{}, startedAt: time.Time{}}
			streaks[k] = s
		}
		if s.count > 0 && outcome.At.Sub(s.countedAt) <= rule.SameRoundInterval {
			continue
		}
		if s.count == 0 {
			s.startedAt = outcome.At
		}
		s.count++
		s.countedAt = outcome.At
	}

	unreachable := map[string][]string{}
	for k, s := range streaks {
		if s.count < rule.Threshold || !receivedOther(outcomes, k.channelType, k.channel, s.startedAt) {
			continue
		}
		unreachable[k.channelType] = append(unreachable[k.channelType], k.channel)
	}
	for channelType := range unreachable {
		slices.Sort(unreachable[channelType])
	}

	return unreachable
}

// receivedOther は、since 以降に、チャンネル種別 channelType のうち、channel でないチャンネルを受信できたかを返す。
func receivedOther(outcomes []TuningOutcome, channelType, channel string, since time.Time) bool {
	for _, outcome := range outcomes {
		if outcome.Received && outcome.ChannelType == channelType && outcome.Channel != channel && !outcome.At.Before(since) {
			return true
		}
	}

	return false
}

// checkTuning は、録画の結果から、続けて選局できないチャンネルを決め、チャンネル種別ごとに管理者に通知する (#1519)。
// 続いている間は、同じチャンネルの組を繰り返し知らせず、なくなったら忘れる。
// チューナーサーバーの一覧にないチャンネル (スキャンし直して消したものなど) は、通知しない。
func (c *Checker) checkTuning(ctx context.Context, q *sqlc.Queries, services []*tunerserver.Service) error {
	rows, err := q.ListTuningOutcomes(ctx, c.clock.Now().Add(-tuningLookback))
	if err != nil {
		return fmt.Errorf("failed to list the tuning outcomes: %w", err)
	}
	outcomes := make([]TuningOutcome, 0, len(rows))
	for _, row := range rows {
		outcomes = append(outcomes, TuningOutcome{
			ChannelType: row.ChannelType, Channel: row.Channel, At: row.RecordedFrom, Received: row.Received,
		})
	}
	verdicts := Unreachable(outcomes, c.tuningRule)
	registered := registeredChannels(services)
	types := map[string]bool{}
	for channelType := range verdicts {
		types[channelType] = true
	}
	for channelType := range c.unreachable {
		types[channelType] = true
	}
	for _, channelType := range slices.Sorted(maps.Keys(types)) {
		channels := slices.DeleteFunc(slices.Clone(verdicts[channelType]), func(channel string) bool {
			return !registered[channelType][channel]
		})
		if len(channels) == 0 {
			delete(c.unreachable, channelType)

			continue
		}
		joined := strings.Join(channels, ",")
		if c.unreachable[channelType] == joined {
			continue
		}
		c.logger.InfoContext(ctx, "channels have failed to tune repeatedly",
			slog.String("kind", channelType), slog.Any("channels", channels))
		if err := c.notify(ctx, q, map[string]any{payloadKind: channelType, "unreachable": channels}); err != nil {
			return err
		}
		c.unreachable[channelType] = joined
	}

	return nil
}

// registeredChannels は、チューナーサーバーのサービスが属するチャンネルを、チャンネル種別ごとの集合にする。
func registeredChannels(services []*tunerserver.Service) map[string]map[string]bool {
	registered := map[string]map[string]bool{}
	for _, service := range services {
		if service == nil || service.Channel == nil {
			continue
		}
		if registered[service.Channel.Type] == nil {
			registered[service.Channel.Type] = map[string]bool{}
		}
		registered[service.Channel.Type][service.Channel.Channel] = true
	}

	return registered
}

package graph

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/usecase/timeshift"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// timeshiftChannels は、タイムシフトで録るチャンネルを、バッファの範囲と録っているかと一緒に返す。
func (r *Resolver) timeshiftChannels(ctx context.Context) ([]*model.TimeshiftChannel, error) {
	rows, err := r.queries.ListTimeshiftChannels(ctx, r.clock.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to list timeshift channels: %w", err)
	}
	channels := make([]*model.TimeshiftChannel, 0, len(rows))
	for i := range rows {
		channels = append(channels, toTimeshiftChannel(&rows[i]))
	}

	return channels, nil
}

// setTimeshiftChannel は、サービスをタイムシフトで録るチャンネルにし、変えた後のチャンネルを返す。
// タイムシフトのディレクトリがない tenban では足させない。neko は、足されたチャンネルをそのまま割り振るので、
// 設定していない構成で、録れない割り振りを作らないようにする。
func (r *Resolver) setTimeshiftChannel(ctx context.Context, serviceGlobalID string, retentionSeconds *int) (*model.TimeshiftChannel, error) {
	_, actor, err := r.administratorActor(ctx, "only administrators can change timeshift channels")
	if err != nil {
		return nil, err
	}
	serviceID, err := parseServiceGlobalID(serviceGlobalID)
	if err != nil {
		return nil, err
	}
	retention := int(timeshift.DefaultRetention.Seconds())
	if retentionSeconds != nil {
		retention = *retentionSeconds
	}
	if _, err := timeshift.ValidateRetention(retention); err != nil {
		return nil, timeshiftError(err)
	}
	if !r.store.HasTimeshiftDir() {
		return nil, NewError(CodeConflict, "timeshift is not configured on this instance; set storage.timeshift_dir")
	}
	if err := r.timeshift.Set(ctx, r.db, actor, serviceID, retention); err != nil {
		return nil, timeshiftError(err)
	}
	channels, err := r.timeshiftChannels(ctx)
	if err != nil {
		return nil, err
	}
	for _, channel := range channels {
		if channel.ServiceKey == serviceID {
			return channel, nil
		}
	}

	// 書いた直後に、ほかの管理者が外した。
	return nil, NewError(CodeNotFound, "timeshift channel not found")
}

// removeTimeshiftChannel は、サービスをタイムシフトで録るチャンネルから外し、外したサービスの ID を返す。
// タイムシフトのディレクトリがない tenban でも、前に足したチャンネルを外せるようにする。
func (r *Resolver) removeTimeshiftChannel(ctx context.Context, serviceGlobalID string) (string, error) {
	_, actor, err := r.administratorActor(ctx, "only administrators can change timeshift channels")
	if err != nil {
		return "", err
	}
	serviceID, err := parseServiceGlobalID(serviceGlobalID)
	if err != nil {
		return "", err
	}
	if err := r.timeshift.Remove(ctx, r.db, actor, serviceID); err != nil {
		return "", timeshiftError(err)
	}

	return serviceGlobalID, nil
}

// timeshiftPrograms は、サービスのバッファから再生できる番組を返す。どのロールも視聴できるので、ログインだけを確かめる (設計書 9.7)。
// この tenban にタイムシフトのディレクトリがなければ、バッファを流せないので、空を返す。
func (r *Resolver) timeshiftPrograms(ctx context.Context, serviceID int64) ([]*model.TimeshiftProgram, error) {
	if _, err := viewer(ctx); err != nil {
		return nil, err
	}
	if !r.store.HasTimeshiftDir() {
		return []*model.TimeshiftProgram{}, nil
	}
	programs, err := timeshift.PlayablePrograms(ctx, r.db, serviceID, r.clock.Now())
	if err != nil {
		return nil, err
	}
	result := make([]*model.TimeshiftProgram, 0, len(programs))
	for _, program := range programs {
		result = append(result, toTimeshiftProgram(serviceID, program))
	}

	return result, nil
}

// timeshiftProgram は、サービスのバッファから再生できる、startAt に始まった番組を返す。なければ NOT_FOUND を返す。
func (r *Resolver) timeshiftProgram(ctx context.Context, serviceGlobalID string, startAt time.Time) (*model.TimeshiftProgram, error) {
	serviceID, err := parseServiceGlobalID(serviceGlobalID)
	if err != nil {
		return nil, err
	}
	programs, err := r.timeshiftPrograms(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	for _, program := range programs {
		if program.StartAt.Equal(startAt) {
			return program, nil
		}
	}

	return nil, NewError(CodeNotFound, "timeshift program not found")
}

// toTimeshiftProgram は、バッファから再生できる番組を GraphQL の形にする。
func toTimeshiftProgram(serviceID int64, program *timeshift.PlayableProgram) *model.TimeshiftProgram {
	gaps := make([]*model.RecordingGap, 0, len(program.Gaps))
	for _, gap := range program.Gaps {
		gaps = append(gaps, &model.RecordingGap{From: gap.From, To: gap.To})
	}

	return &model.TimeshiftProgram{
		EventID: int(program.EventID), Name: program.Name, StartAt: program.StartedAt, EndAt: program.EndedAt,
		PlayableFrom: program.PlayableFrom, Gaps: gaps, ServiceKey: serviceID,
	}
}

// toTimeshiftChannel は、タイムシフトで録るチャンネルを GraphQL の形にする。
func toTimeshiftChannel(channel *domain.TimeshiftChannel) *model.TimeshiftChannel {
	return &model.TimeshiftChannel{
		RetentionSeconds: int(channel.RetentionSeconds),
		BufferedFrom:     channel.BufferedFrom,
		BufferedUntil:    channel.BufferedUntil,
		Recording:        channel.Recording,
		ServiceKey:       channel.ServiceID,
	}
}

// timeshiftError は、タイムシフトのチャンネルの設定のエラーを、エラーコードを持つエラーにする。
func timeshiftError(err error) error {
	switch {
	case errors.Is(err, timeshift.ErrInvalidRetention):
		return NewError(CodeBadUserInput, err.Error())
	case errors.Is(err, timeshift.ErrServiceNotFound):
		return NewError(CodeNotFound, "service not found")
	case errors.Is(err, timeshift.ErrChannelNotFound):
		return NewError(CodeNotFound, "timeshift channel not found")
	default:
		return err
	}
}

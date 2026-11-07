package calendarapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/query"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
)

const (
	// pastWindow は、終わった予約を載せておく長さ。終わった直後の予定がカレンダーからすぐ消えないようにする。
	pastWindow = 7 * 24 * time.Hour
	// futureWindow は、これから始まる予約を載せる長さ。
	futureWindow = 366 * 24 * time.Hour
	// maxEvents は、載せる予約の数の上限。
	maxEvents = 1000
	// calendarName は、カレンダーのアプリに出すカレンダーの名前。
	calendarName = "Kotatsu の録画予約"
)

// Authenticator は、購読用の URL の秘密の値から、発行したユーザーを引く。*auth.CalendarFeeds が満たす。
type Authenticator interface {
	Authenticate(ctx context.Context, token string) (*auth.Principal, error)
}

var _ Authenticator = new(auth.CalendarFeeds)

// Queries は、予約とサービスを読む。*query.Queries が満たす。
type Queries interface {
	ListReservations(ctx context.Context, query *query.ReservationListQuery) ([]*domain.Reservation, error)
	ListGuideServices(ctx context.Context, arg query.ListGuideServicesParams) ([]domain.Service, error)
}

var _ Queries = new(query.Queries)

// Handler は、予約を iCalendar で返す (GET /calendar/{token}/reservations.ics)。
// カレンダーのアプリは Cookie もヘッダーも送れないので、パスに入れた秘密の値で認証する。
type Handler struct {
	auth    Authenticator
	queries Queries
	clock   clock.Clock
	logger  *slog.Logger
}

// NewHandler は Handler を返す。
func NewHandler(authenticator Authenticator, queries Queries, clk clock.Clock, logger *slog.Logger) *Handler {
	return &Handler{auth: authenticator, queries: queries, clock: clk, logger: logger}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	principal, err := h.auth.Authenticate(ctx, r.PathValue("token"))
	if errors.Is(err, auth.ErrUnauthenticated) {
		// 秘密の値が違うことを、見つからないことと区別させない。
		http.NotFound(w, r)

		return
	}
	if err != nil {
		h.fail(ctx, w, "failed to authenticate the calendar feed", err)

		return
	}
	if !principal.Role.Can(auth.PermissionWatch) {
		http.NotFound(w, r)

		return
	}
	now := h.clock.Now()
	enabled := true
	reservations, err := h.queries.ListReservations(ctx, &query.ReservationListQuery{
		From: now.Add(-pastWindow), To: now.Add(futureWindow), States: []domain.AssignmentState{}, Enabled: &enabled,
		RuleID: nil, AfterStartAt: nil, AfterID: nil, Limit: maxEvents,
	})
	if err != nil {
		h.fail(ctx, w, "failed to list reservations for the calendar feed", err)

		return
	}
	names, err := h.serviceNames(ctx, reservations)
	if err != nil {
		h.fail(ctx, w, "failed to list services for the calendar feed", err)

		return
	}
	events := make([]*Event, 0, len(reservations))
	for _, reservation := range reservations {
		events = append(events, &Event{
			UID: reservation.ID.String() + "@kotatsu", Summary: reservation.Name, Location: names[reservation.ServiceID],
			Description: reservation.Description, StartAt: reservation.StartAt, EndAt: reservation.EndAt, UpdatedAt: reservation.UpdatedAt,
		})
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	// URL に秘密の値を含むので、途中のキャッシュに残させない。
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = w.Write([]byte(Calendar(calendarName, events, now)))
}

// serviceNames は、予約のサービスの名前を、サービスの ID で引けるようにして返す。
func (h *Handler) serviceNames(ctx context.Context, reservations []*domain.Reservation) (map[int64]string, error) {
	ids := make([]int64, 0, len(reservations))
	seen := map[int64]bool{}
	for _, reservation := range reservations {
		if !seen[reservation.ServiceID] {
			seen[reservation.ServiceID] = true
			ids = append(ids, reservation.ServiceID)
		}
	}
	names := make(map[int64]string, len(ids))
	if len(ids) == 0 {
		return names, nil
	}
	services, err := h.queries.ListGuideServices(ctx, query.ListGuideServicesParams{IncludeHidden: true, MediaServiceTypes: []int64{}, IDs: ids})
	if err != nil {
		return nil, err
	}
	for i := range services {
		names[services[i].ID] = services[i].Name
	}

	return names, nil
}

func (h *Handler) fail(ctx context.Context, w http.ResponseWriter, message string, err error) {
	h.logger.ErrorContext(ctx, message, slog.Any("error", err))
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

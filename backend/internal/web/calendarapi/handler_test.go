package calendarapi_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/query"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/web/calendarapi"
)

// validToken は、fakeAuthenticator が受け付ける値。値そのものに意味はないので、印に続けて作る。
const validToken = calendarMark + "valid"

// calendarMark は、購読用の URL の秘密の値の印。
const calendarMark = "kotatsu_cal_"

// fakeAuthenticator は、validToken だけを受け付ける。
type fakeAuthenticator struct {
	role auth.Role
}

func (f *fakeAuthenticator) Authenticate(_ context.Context, token string) (*auth.Principal, error) {
	if token != validToken {
		return nil, auth.ErrUnauthenticated
	}

	return &auth.Principal{UserID: uuid.New(), Role: f.role, SessionID: uuid.Nil(), AuthenticatedAt: time.Time{}, OAuthClientID: ""}, nil
}

// fakeQueries は、決まった予約とサービスを返し、引いた条件を覚える。
type fakeQueries struct {
	reservations []*domain.Reservation
	listed       *query.ReservationListQuery
}

func (f *fakeQueries) ListReservations(_ context.Context, q *query.ReservationListQuery) ([]*domain.Reservation, error) {
	f.listed = q

	return f.reservations, nil
}

func (f *fakeQueries) ListGuideServices(_ context.Context, arg query.ListGuideServicesParams) ([]domain.Service, error) {
	services := make([]domain.Service, 0, len(arg.IDs))
	for _, id := range arg.IDs {
		services = append(services, domain.Service{ID: id, Name: "こたつテレビ"})
	}

	return services, nil
}

// TestHandler は、秘密の値が正しければ、有効な予約を iCalendar で返し、違えば 404 を返すことを確かめる (#1255)。
func TestHandler(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	reservationID := uuid.MustParse("0192f000-0000-7000-8000-000000000001")
	tests := []struct {
		name       string
		token      string
		role       auth.Role
		wantStatus int
	}{
		{name: "valid", token: validToken, role: auth.RoleMember, wantStatus: http.StatusOK},
		{name: "viewer", token: validToken, role: auth.RoleViewer, wantStatus: http.StatusOK},
		{name: "wrong token", token: calendarMark + "wrong", role: auth.RoleMember, wantStatus: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			queries := &fakeQueries{reservations: []*domain.Reservation{{
				ID: reservationID, ServiceID: 3273701032, Name: "ニュース", Description: "説明",
				StartAt: now.Add(time.Hour), EndAt: now.Add(2 * time.Hour), Enabled: true, UpdatedAt: now,
			}}}
			mux := http.NewServeMux()
			mux.Handle("GET /api/calendar/{token}/reservations.ics",
				calendarapi.NewHandler(&fakeAuthenticator{role: tt.role}, queries, clocktest.NewFake(now), slog.New(slog.DiscardHandler)))
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/calendar/"+tt.token+"/reservations.ics", nil))

			require.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantStatus != http.StatusOK {
				assert.Nil(t, queries.listed, "reservations are not read")

				return
			}
			assert.Equal(t, "text/calendar; charset=utf-8", rec.Header().Get("Content-Type"))
			assert.Equal(t, "private, no-store", rec.Header().Get("Cache-Control"))
			body := rec.Body.String()
			assert.Contains(t, body, "UID:"+reservationID.String()+"@kotatsu\r\n")
			assert.Contains(t, body, "SUMMARY:ニュース\r\n")
			assert.Contains(t, body, "LOCATION:こたつテレビ\r\n")
			assert.Equal(t, 1, strings.Count(body, "BEGIN:VEVENT"))
			// 有効な予約だけを、終わってから 7 日の間と、これから 1 年の分だけ引く。
			require.NotNil(t, queries.listed.Enabled)
			assert.True(t, *queries.listed.Enabled)
			assert.Equal(t, now.Add(-7*24*time.Hour), queries.listed.From)
			assert.Equal(t, now.Add(366*24*time.Hour), queries.listed.To)
		})
	}
}

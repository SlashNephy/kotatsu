package graph_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const avatarQuery = `{ viewer { avatarUrl } }`

// TestViewerAvatarURL は、IdP が picture を返したユーザーにだけ、アイコンを中継する URL を返すことを確かめる (#778)。
func TestViewerAvatarURL(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	var got struct {
		Viewer struct {
			AvatarURL *string `json:"avatarUrl"`
		} `json:"viewer"`
	}
	require.Empty(t, f.query(t, avatarQuery, nil, &got))
	assert.Nil(t, got.Viewer.AvatarURL)

	_, err := f.pool.Exec(t.Context(), `INSERT INTO user_oidc_identities (id, user_id, issuer, subject, picture, created_at, last_used_at)
		VALUES ($1, $2, 'https://idp.example', 'sub', 'https://idp.example/avatar.png', now(), now())`, uuid.New(), f.userID)
	require.NoError(t, err)
	require.Empty(t, f.query(t, avatarQuery, nil, &got))
	require.NotNil(t, got.Viewer.AvatarURL)
	assert.Equal(t, "/media/users/"+f.userID.String()+"/avatar", *got.Viewer.AvatarURL)
}

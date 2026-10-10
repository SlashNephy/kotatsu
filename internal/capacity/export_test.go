package capacity

import (
	"context"

	"github.com/google/uuid"
)

// Delete は delete をテストから呼ぶ。消す候補を選んだ後に保護された録画を、消さないことを確かめるのに使う。
func (m *Manager) Delete(ctx context.Context, recordingID uuid.UUID, poolID string) (bool, error) {
	return m.delete(ctx, recordingID, poolID)
}

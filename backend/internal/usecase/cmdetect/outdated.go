package cmdetect

import (
	"context"
	"fmt"
	"time"
	"uuid"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// selectableDetectors は、管理者が選べる見分け方を返す (#1814)。名前と版を読むだけなので、作らずに型の値で返す。
// 見分け方を足したら、ここにも足す。
func selectableDetectors() []Detector {
	return []Detector{(*FFmpegDetector)(nil), (*LogoDetector)(nil)}
}

// currentVersion は、名前が name の見分け方の、今の版を返す。知らない名前なら false を返す。
func currentVersion(name string) (int32, bool) {
	for _, detector := range selectableDetectors() {
		if detector.Name() == name {
			return detector.Version(), true
		}
	}

	return 0, false
}

// OutdatedRecording は、最後に見分けた結果が、今の見分け方の今の版でない録画 (#1814)。
type OutdatedRecording struct {
	RecordingID uuid.UUID
	Name        string
	StartAt     time.Time
	// Detector と DetectorVersion は、最後に見分けた見分け方と、その版。
	Detector        string
	DetectorVersion int32
	// HasMaterial は、今の見分け方の今の形式の材料が残っていて、録画を読み直さずに判定し直せそうかどうか。
	// 材料があっても、セグメントが変わった (つないだ) ときなどは、ジョブが読み直す。
	HasMaterial bool
}

// Outdated は、管理者が選んだ見分け方の名前と今の版と、最後に見分けた結果がその見分け方の今の版より古い録画を、
// 放送の古い順に返す (#1814)。includeOtherDetectors なら、ほかの見分け方で最後に見分けた録画も返す。
// 見分け方を替えたときに、替える前の見分け方の結果を直すのに使う。
func Outdated(ctx context.Context, db sqlc.DBTX, includeOtherDetectors bool) (string, int32, []OutdatedRecording, error) {
	detector, err := selectedDetector(ctx, db)
	if err != nil {
		return "", 0, nil, err
	}
	version, ok := currentVersion(detector)
	if !ok {
		return "", 0, nil, fmt.Errorf("unknown CM detector %q", detector)
	}
	rows, err := sqlc.New(db).ListOutdatedRecordingCMDetections(ctx, sqlc.ListOutdatedRecordingCMDetectionsParams{
		Detector:              detector,
		MaterialFormatVersion: materialFormatVersion,
		DetectorVersion:       version,
		IncludeOtherDetectors: includeOtherDetectors,
	})
	if err != nil {
		return "", 0, nil, fmt.Errorf("failed to list the recordings with outdated CM sections: %w", err)
	}
	recordings := make([]OutdatedRecording, 0, len(rows))
	for _, row := range rows {
		recordings = append(recordings, OutdatedRecording{
			RecordingID: row.RecordingID, Name: row.Name, StartAt: row.StartAt,
			Detector: row.Detector, DetectorVersion: row.DetectorVersion, HasMaterial: row.HasMaterial,
		})
	}

	return detector, version, recordings, nil
}

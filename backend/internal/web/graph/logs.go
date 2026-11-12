package graph

import (
	"context"
	"errors"

	"github.com/SlashNephy/kotatsu/internal/lib/logring"
	"github.com/SlashNephy/kotatsu/internal/usecase/logreport"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// errLogsUnavailable は、ログを読む部品を組んでいないこと。
var errLogsUnavailable = errors.New("logs are not available")

// logReport は、直近のログと、不具合の報告に要るものを返す (#1521)。管理者だけが使える。
func (r *Resolver) logReport(ctx context.Context) (*model.LogReport, error) {
	if err := administer(ctx); err != nil {
		return nil, err
	}
	if r.logs == nil {
		return nil, errLogsUnavailable
	}
	report, err := r.logs.Report(ctx, requestOrigin(ctx))
	if err != nil {
		return nil, err
	}

	sources := make([]*model.LogSource, 0, len(report.Sources))
	for _, source := range report.Sources {
		lines := make([]*model.LogLine, 0, len(source.Lines))
		for _, line := range source.Lines {
			lines = append(lines, &model.LogLine{Time: line.Time, Level: logLevelOf(line.Level), Inferred: line.Inferred, Text: line.Text})
		}
		var failure *model.LogSourceFailure
		if source.Failure != logreport.FailureNone {
			failure = new(model.LogSourceFailure(source.Failure))
		}
		sources = append(sources, &model.LogSource{
			Kind: model.LogSourceKind(source.Kind), Lines: lines, Total: source.Total, Failure: failure,
			Version: source.Version, Os: source.OS, Arch: source.Arch, Runtime: source.Runtime,
		})
	}

	return &model.LogReport{Sources: sources, RedactHosts: report.Hosts, InstanceID: report.InstanceID, Roles: report.Roles}, nil
}

func logLevelOf(level logring.Level) model.LogLevel {
	switch level {
	case logring.LevelInfo:
		return model.LogLevelInfo
	case logring.LevelWarn:
		return model.LogLevelWarn
	case logring.LevelError:
		return model.LogLevelError
	default:
		return model.LogLevelInfo
	}
}

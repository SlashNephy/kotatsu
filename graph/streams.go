package graph

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/graph/model"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/kyusu"
	"github.com/SlashNephy/kotatsu/internal/registry"
	"github.com/SlashNephy/kotatsu/internal/stream"
)

// createStreamURL は、サービスか録画の、署名付きの映像の URL を発行する (設計書 7.6、9.6)。
// 映像を見ることは誰にでも許されているので、ログインしていれば発行する。
func (r *Resolver) createStreamURL(ctx context.Context, target string) (*model.StreamURL, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	if r.streamSigner == nil {
		return nil, errors.New("stream URL signer is not configured")
	}
	resource, err := r.streamResource(ctx, target)
	if err != nil {
		return nil, err
	}
	signed := r.streamSigner.Sign(resource, principal.UserID, r.clock.Now().Add(stream.DefaultURLLifetime))

	return &model.StreamURL{URL: signed.URL, ExpiresAt: signed.ExpiresAt}, nil
}

// streamResource は、Service か Recording の ID から、映像のパスを返す。ないものの URL は発行しない。
func (r *Resolver) streamResource(ctx context.Context, target string) (string, error) {
	typeName, _, err := ParseGlobalID(target)
	if err != nil {
		return "", NewError(CodeBadUserInput, "invalid target ID")
	}
	q := sqlc.New(r.db)
	switch typeName {
	case serviceType:
		serviceID, err := parseServiceGlobalID(target)
		if err != nil {
			return "", err
		}
		exists, err := q.ServiceExists(ctx, serviceID)
		if err != nil {
			return "", fmt.Errorf("failed to look up the service: %w", err)
		}
		if !exists {
			return "", NewError(CodeNotFound, "service not found")
		}

		return stream.ServicePath(serviceID), nil
	case recordingType:
		recordingID, err := parseRecordingID(target)
		if err != nil {
			return "", err
		}
		rows, err := q.ListRecordingsByIDs(ctx, []uuid.UUID{recordingID})
		if err != nil {
			return "", fmt.Errorf("failed to look up the recording: %w", err)
		}
		if len(rows) == 0 {
			return "", NewError(CodeNotFound, "recording not found")
		}

		return stream.RecordingPath(recordingID), nil
	default:
		return "", NewError(CodeBadUserInput, "target must be a Service or Recording ID")
	}
}

// transcoding は、サーバー側でのトランスコードを使えるかと、選べる画質を返す (設計書 7.1)。
// kyusu は、トランスコードできるときだけ登録簿に載るので、生きている kyusu がいれば使えるとみなす。
func (r *Resolver) transcoding(ctx context.Context) (*model.Transcoding, error) {
	if _, err := viewer(ctx); err != nil {
		return nil, err
	}
	// 登録簿を読むだけなので、このインスタンスの ID は使わない。
	instances, err := registry.New(r.db, r.clock, uuid.Nil).ListAlive(ctx, kyusu.Role)
	if err != nil {
		return nil, err
	}
	presets := make([]*model.StreamPreset, 0, len(kyusu.Presets()))
	for _, preset := range kyusu.Presets() {
		var codec model.VideoCodec
		switch preset.Codec {
		case kyusu.CodecH264:
			codec = model.VideoCodecH264
		case kyusu.CodecHEVC:
			codec = model.VideoCodecHevc
		default:
			return nil, fmt.Errorf("unknown codec of preset %q: %s", preset.Name, preset.Codec)
		}
		presets = append(presets, &model.StreamPreset{
			Name: preset.Name, Height: preset.Height, VideoBitrate: preset.VideoBitrate, Codec: codec, Passthrough: preset.Passthrough,
		})
	}

	return &model.Transcoding{Available: len(instances) > 0, Presets: presets}, nil
}

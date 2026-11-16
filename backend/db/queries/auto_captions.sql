-- name: UpsertRecordingAutoCaptions :exec
-- 録画の音声から自動生成した字幕を書く (設計書 6.1、#1682)。生成し直したときは、前の字幕と置き換える。
INSERT INTO recording_auto_captions (recording_id, model, generator_version, generated_at, cues)
VALUES (@recording_id, @model, @generator_version, @now, @cues)
ON CONFLICT (recording_id) DO UPDATE SET model = EXCLUDED.model, generator_version = EXCLUDED.generator_version,
  generated_at = EXCLUDED.generated_at, cues = EXCLUDED.cues;

-- name: GetRecordingAutoCaptions :one
-- 録画の自動生成した字幕を返す。生成していなければ行を返さない。
SELECT model, generator_version, generated_at, cues FROM recording_auto_captions WHERE recording_id = @recording_id;

-- name: GetRecordingAutoCaptionsSummary :one
-- 録画の自動生成した字幕の、生成した日時とモデルと字幕の数を返す。録画の詳細の状態に使い、字幕そのものは読まない。
SELECT model, generated_at, jsonb_array_length(cues)::integer AS cue_count FROM recording_auto_captions WHERE recording_id = @recording_id;

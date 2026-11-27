-- name: UpsertRecordingARIBCaptions :exec
-- 録画の TS から取り出した ARIB 字幕の文を書く (設計書 6.1、#2217)。取り出し直したときは、前の字幕と置き換える。
INSERT INTO recording_arib_captions (recording_id, extractor_version, extracted_at, cues, search_grams)
VALUES (@recording_id, @extractor_version, @now, @cues, @search_grams::text[])
ON CONFLICT (recording_id) DO UPDATE SET extractor_version = EXCLUDED.extractor_version, extracted_at = EXCLUDED.extracted_at,
  cues = EXCLUDED.cues, search_grams = EXCLUDED.search_grams;

-- name: GetRecordingARIBCaptions :one
-- 録画の ARIB 字幕の文を返す。取り出していなければ行を返さない。
SELECT extractor_version, extracted_at, cues FROM recording_arib_captions WHERE recording_id = @recording_id;

-- name: ListRecordingARIBCaptionCues :many
-- 録画 recording_ids の ARIB 字幕の文を返す。字幕の文で探すとき (#2217) に、候補の録画の字幕をまとめて読む。
SELECT recording_id, cues FROM recording_arib_captions WHERE recording_id = ANY(@recording_ids::uuid[]);

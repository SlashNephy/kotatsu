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

-- name: ListRecordingsForARIBCaptionExtraction :many
-- ARIB 字幕の文を取り出す録画を、放送の開始の順に返す (kotatsu admin extract-arib-captions、#2218)。
-- 録画中でなく、TS のセグメントを書いた録画のうち、取り出していないか、取り出し方の古いバージョン (extractor_version) で取り出した録画を選ぶ。
-- include_all なら、今のバージョンで取り出した録画も選ぶ。extractor_version は、取り出していなければ NULL。
SELECT r.id AS recording_id, r.name, r.start_at, a.extractor_version
FROM recordings r
LEFT JOIN recording_arib_captions a ON a.recording_id = r.id
WHERE r.state <> 'recording'
  AND EXISTS (SELECT 1 FROM recording_files f WHERE f.recording_id = r.id AND f.kind = 'segment' AND f.size > 0)
  AND (@include_all::boolean OR a.recording_id IS NULL OR a.extractor_version < @extractor_version::integer)
ORDER BY r.start_at, r.id;

-- name: UpsertRecordingAutoCaptions :exec
-- 録画の音声から自動生成した字幕を書く (設計書 6.1、#1682)。生成し直したときは、前の字幕と置き換える。
-- 字幕の文で探すための gram (#2214) も、同じ字幕から作って書く。
INSERT INTO recording_auto_captions (recording_id, model, generator_version, generated_at, cues, search_grams, search_grams_generated_at)
VALUES (@recording_id, @model, @generator_version, @now, @cues, @search_grams::text[], @now)
ON CONFLICT (recording_id) DO UPDATE SET model = EXCLUDED.model, generator_version = EXCLUDED.generator_version,
  generated_at = EXCLUDED.generated_at, cues = EXCLUDED.cues,
  search_grams = EXCLUDED.search_grams, search_grams_generated_at = EXCLUDED.search_grams_generated_at;

-- name: GetRecordingAutoCaptions :one
-- 録画の自動生成した字幕を返す。生成していなければ行を返さない。
SELECT model, generator_version, generated_at, cues FROM recording_auto_captions WHERE recording_id = @recording_id;

-- name: GetRecordingAutoCaptionsSummary :one
-- 録画の自動生成した字幕の、生成した日時とモデルと字幕の数を返す。録画の詳細の状態に使い、字幕そのものは読まない。
SELECT model, generated_at, jsonb_array_length(cues)::integer AS cue_count FROM recording_auto_captions WHERE recording_id = @recording_id;

-- name: ListRecordingAutoCaptionCues :many
-- 録画の自動生成した字幕を、録画ごとに返す。字幕の文で探すとき (#2214) に、候補の録画の字幕をまとめて読む。
SELECT recording_id, cues FROM recording_auto_captions WHERE recording_id = ANY(@recording_ids::uuid[]);

-- name: ListRecordingAutoCaptionsWithoutSearchGrams :many
-- 字幕の文で探すための gram (#2214) が、今の字幕から作ったものでない字幕を返す。
-- カラムを足す前に書いた字幕と、カラムを知らない古いバイナリが書き直した字幕である。
SELECT recording_id, generated_at, cues FROM recording_auto_captions
WHERE search_grams_generated_at IS DISTINCT FROM generated_at
ORDER BY recording_id
LIMIT @max_count;

-- name: SetRecordingAutoCaptionsSearchGrams :batchexec
-- 字幕の gram を書く。読んだ後に字幕を書き直していたら、書かない (次の kotatsu migrate で作り直す)。
UPDATE recording_auto_captions SET search_grams = @search_grams::text[], search_grams_generated_at = generated_at
WHERE recording_id = @recording_id AND generated_at = @generated_at;

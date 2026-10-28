package epgstation

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	selectRules = `SELECT id, enable, isTimeSpecification,
		keyword, keyCS, keyRegExp, name, description, extended,
		ignoreKeyword, ignoreKeyCS, ignoreKeyRegExp, ignoreName, ignoreDescription, ignoreExtended,
		GR, BS, CS, SKY, channelIds, genres, times, isFree, durationMin, durationMax, searchPeriods,
		avoidDuplicate, periodToAvoidDuplicate, allowEndLack, tags, parentDirectoryName, directory, recordedFormat
		FROM rule ORDER BY id`
	selectManualReserves = `SELECT id, isSkip, allowEndLack, isTimeSpecified, isEventRelay,
		programId, channelId, channelType, startAt, endAt, name, tags, parentDirectoryName, directory, recordedFormat
		FROM reserve WHERE ruleId IS NULL ORDER BY id`
	selectRecordings = `SELECT r.id, r.ruleId, r.programId, r.channelId, r.isProtected, r.isRecording, r.startAt, r.endAt,
		r.name, r.description, r.extended, r.genre1, r.subGenre1, r.genre2, r.subGenre2, r.genre3, r.subGenre3,
		d.errorCnt, d.dropCnt, d.scramblingCnt
		FROM recorded r LEFT JOIN drop_log_file d ON d.id = r.dropLogFileId ORDER BY r.id`
	selectVideoFiles = `SELECT id, recordedId, parentDirectoryName, filePath, type, name, size
		FROM video_file ORDER BY recordedId, id`
	selectRecordingTags = `SELECT recordedId, recordedTagId FROM recorded_tags_recorded_tag ORDER BY recordedId, recordedTagId`
	selectHistories     = `SELECT id, name, channelId, endAt FROM recorded_history ORDER BY id`
	selectTags          = `SELECT id, name, color FROM recorded_tag ORDER BY id`
)

// Read は、取り込みの対象のデータを、一つの読み取り専用のトランザクションで読む。
func (s *Source) Read(ctx context.Context) (*Snapshot, error) {
	snapshot := &Snapshot{
		Rules:          []*Rule{},
		ManualReserves: []*Reserve{},
		Recordings:     []*Recording{},
		Histories:      []*RecordedHistory{},
		Tags:           []*Tag{},
	}
	err := s.readOnly(ctx, func(tx *sql.Tx) error {
		var err error
		if snapshot.Rules, err = readRules(ctx, tx); err != nil {
			return err
		}
		if snapshot.ManualReserves, err = readManualReserves(ctx, tx); err != nil {
			return err
		}
		if snapshot.Recordings, err = readRecordings(ctx, tx); err != nil {
			return err
		}
		if snapshot.Histories, err = readHistories(ctx, tx); err != nil {
			return err
		}
		snapshot.Tags, err = readTags(ctx, tx)

		return err
	})
	if err != nil {
		return nil, err
	}

	return snapshot, nil
}

// scanAll は、query の各行を scan で読み、結果を順に返す。
func scanAll[T any](ctx context.Context, tx *sql.Tx, table, query string, scan func(rows *sql.Rows) (T, error)) ([]T, error) {
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", table, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	items := []T{}
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", table, err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", table, err)
	}

	return items, nil
}

func readRules(ctx context.Context, tx *sql.Tx) ([]*Rule, error) {
	return scanAll(ctx, tx, "rule", selectRules, func(rows *sql.Rows) (*Rule, error) {
		rule := new(Rule)
		var (
			keyword, ignoreKeyword                     sql.NullString
			gr, bs, cs, sky                            bool
			channelIDs, genres, times, periods, tags   sql.NullString
			durationMin, durationMax, avoidPeriod      sql.NullInt64
			parentDirectory, directory, recordedFormat sql.NullString
		)
		err := rows.Scan(
			&rule.ID, &rule.Enabled, &rule.IsTimeSpecification,
			&keyword, &rule.Keyword.CaseSensitive, &rule.Keyword.Regexp,
			&rule.Keyword.Name, &rule.Keyword.Description, &rule.Keyword.Extended,
			&ignoreKeyword, &rule.IgnoreKeyword.CaseSensitive, &rule.IgnoreKeyword.Regexp,
			&rule.IgnoreKeyword.Name, &rule.IgnoreKeyword.Description, &rule.IgnoreKeyword.Extended,
			&gr, &bs, &cs, &sky, &channelIDs, &genres, &times, &rule.FreeOnly, &durationMin, &durationMax, &periods,
			&rule.AvoidDuplicate, &avoidPeriod, &rule.AllowEndLack, &tags, &parentDirectory, &directory, &recordedFormat,
		)
		if err != nil {
			return nil, err
		}

		rule.Keyword.Keyword = keyword.String
		rule.IgnoreKeyword.Keyword = ignoreKeyword.String
		rule.ChannelTypes = channelTypes(gr, bs, cs, sky)
		rule.DurationMin = intOrNil(durationMin)
		rule.DurationMax = intOrNil(durationMax)
		rule.PeriodToAvoidDuplicate = intOrNil(avoidPeriod)
		rule.ParentDirectoryName = parentDirectory.String
		rule.Directory = directory.String
		rule.RecordedFormat = recordedFormat.String

		if rule.ChannelIDs, err = decodeJSON[int64](channelIDs, "channelIds"); err != nil {
			return nil, ruleError(rule.ID, err)
		}
		if rule.Genres, err = decodeJSON[*Genre](genres, "genres"); err != nil {
			return nil, ruleError(rule.ID, err)
		}
		if rule.Times, err = decodeJSON[*SearchTime](times, "times"); err != nil {
			return nil, ruleError(rule.ID, err)
		}
		if rule.TagIDs, err = decodeJSON[int64](tags, "tags"); err != nil {
			return nil, ruleError(rule.ID, err)
		}
		if rule.SearchPeriods, err = decodePeriods(periods); err != nil {
			return nil, ruleError(rule.ID, err)
		}

		return rule, nil
	})
}

func readManualReserves(ctx context.Context, tx *sql.Tx) ([]*Reserve, error) {
	return scanAll(ctx, tx, "reserve", selectManualReserves, func(rows *sql.Rows) (*Reserve, error) {
		reserve := new(Reserve)
		var (
			programID                                  sql.NullInt64
			startAt, endAt                             int64
			name, tags                                 sql.NullString
			parentDirectory, directory, recordedFormat sql.NullString
		)
		err := rows.Scan(
			&reserve.ID, &reserve.IsSkip, &reserve.AllowEndLack, &reserve.IsTimeSpecified, &reserve.IsEventRelay,
			&programID, &reserve.ChannelID, &reserve.ChannelType, &startAt, &endAt, &name, &tags,
			&parentDirectory, &directory, &recordedFormat,
		)
		if err != nil {
			return nil, err
		}

		reserve.ProgramID = int64OrNil(programID)
		reserve.StartAt = fromMillis(startAt)
		reserve.EndAt = fromMillis(endAt)
		reserve.Name = name.String
		reserve.ParentDirectoryName = parentDirectory.String
		reserve.Directory = directory.String
		reserve.RecordedFormat = recordedFormat.String
		if reserve.TagIDs, err = decodeJSON[int64](tags, "tags"); err != nil {
			return nil, fmt.Errorf("reserve %d: %w", reserve.ID, err)
		}

		return reserve, nil
	})
}

func readRecordings(ctx context.Context, tx *sql.Tx) ([]*Recording, error) {
	recordings, err := scanAll(ctx, tx, "recorded", selectRecordings, func(rows *sql.Rows) (*Recording, error) {
		recording := new(Recording)
		var (
			ruleID, programID                                       sql.NullInt64
			startAt, endAt                                          int64
			description, extended                                   sql.NullString
			genre1, subGenre1, genre2, subGenre2, genre3, subGenre3 sql.NullInt64
			errorCount, dropCount, scramblingCount                  sql.NullInt64
		)
		err := rows.Scan(
			&recording.ID, &ruleID, &programID, &recording.ChannelID, &recording.IsProtected, &recording.IsRecording,
			&startAt, &endAt, &recording.Name, &description, &extended,
			&genre1, &subGenre1, &genre2, &subGenre2, &genre3, &subGenre3,
			&errorCount, &dropCount, &scramblingCount,
		)
		if err != nil {
			return nil, err
		}

		recording.Files = []*VideoFile{}
		recording.TagIDs = []int64{}
		recording.RuleID = int64OrNil(ruleID)
		recording.ProgramID = int64OrNil(programID)
		recording.StartAt = fromMillis(startAt)
		recording.EndAt = fromMillis(endAt)
		recording.Description = description.String
		recording.Extended = extended.String
		recording.Genres = genresOf([][2]sql.NullInt64{{genre1, subGenre1}, {genre2, subGenre2}, {genre3, subGenre3}})
		// drop_log_file の 3 つの列は NOT NULL なので、どれかが NULL なら LEFT JOIN で行が無かったことになる。
		if errorCount.Valid {
			recording.Drop = &DropLog{
				ErrorCount:      errorCount.Int64,
				DropCount:       dropCount.Int64,
				ScramblingCount: scramblingCount.Int64,
			}
		}

		return recording, nil
	})
	if err != nil {
		return nil, err
	}

	byID := make(map[int64]*Recording, len(recordings))
	for _, recording := range recordings {
		byID[recording.ID] = recording
	}
	if err := readVideoFiles(ctx, tx, byID); err != nil {
		return nil, err
	}
	if err := readRecordingTags(ctx, tx, byID); err != nil {
		return nil, err
	}

	return recordings, nil
}

// readVideoFiles は、録画のファイルを読んで、byID の録画に持たせる。録画が消えて残ったファイルの行は捨てる。
func readVideoFiles(ctx context.Context, tx *sql.Tx, byID map[int64]*Recording) error {
	type row struct {
		recordedID int64
		file       *VideoFile
	}
	rows, err := scanAll(ctx, tx, "video_file", selectVideoFiles, func(rows *sql.Rows) (*row, error) {
		r := &row{recordedID: 0, file: new(VideoFile)}
		var fileType string
		err := rows.Scan(&r.file.ID, &r.recordedID, &r.file.ParentDirectoryName, &r.file.FilePath, &fileType, &r.file.Name, &r.file.Size)
		r.file.Type = VideoFileType(fileType)
		// Windows の EPGStation は、サブディレクトリを \ で区切って記録する (Node の path.join)。Kotatsu のプール内の相対パスは / で区切る (#1098)。
		// EPGStation は、ファイル名の \ を ￥ に置き換えるので、\ は区切りだけに使われる。
		r.file.FilePath = strings.ReplaceAll(r.file.FilePath, `\`, "/")

		return r, err
	})
	if err != nil {
		return err
	}
	for _, r := range rows {
		if recording, ok := byID[r.recordedID]; ok {
			recording.Files = append(recording.Files, r.file)
		}
	}

	return nil
}

// readRecordingTags は、録画に付いたタグの ID を読んで、byID の録画に持たせる。
func readRecordingTags(ctx context.Context, tx *sql.Tx, byID map[int64]*Recording) error {
	pairs, err := scanAll(ctx, tx, "recorded_tags_recorded_tag", selectRecordingTags, func(rows *sql.Rows) ([2]int64, error) {
		var pair [2]int64
		err := rows.Scan(&pair[0], &pair[1])

		return pair, err
	})
	if err != nil {
		return err
	}
	for _, pair := range pairs {
		if recording, ok := byID[pair[0]]; ok {
			recording.TagIDs = append(recording.TagIDs, pair[1])
		}
	}

	return nil
}

func readHistories(ctx context.Context, tx *sql.Tx) ([]*RecordedHistory, error) {
	return scanAll(ctx, tx, "recorded_history", selectHistories, func(rows *sql.Rows) (*RecordedHistory, error) {
		history := new(RecordedHistory)
		var endAt int64
		if err := rows.Scan(&history.ID, &history.Name, &history.ChannelID, &endAt); err != nil {
			return nil, err
		}
		history.EndAt = fromMillis(endAt)

		return history, nil
	})
}

func readTags(ctx context.Context, tx *sql.Tx) ([]*Tag, error) {
	return scanAll(ctx, tx, "recorded_tag", selectTags, func(rows *sql.Rows) (*Tag, error) {
		tag := new(Tag)
		err := rows.Scan(&tag.ID, &tag.Name, &tag.Color)

		return tag, err
	})
}

func ruleError(id int64, err error) error {
	return fmt.Errorf("rule %d: %w", id, err)
}

// channelTypes は、チャンネル種別ごとの真偽値の列を、種別の集合にする。
func channelTypes(gr, bs, cs, sky bool) []string {
	types := []string{}
	for _, t := range []struct {
		name    string
		enabled bool
	}{{"GR", gr}, {"BS", bs}, {"CS", cs}, {"SKY", sky}} {
		if t.enabled {
			types = append(types, t.name)
		}
	}

	return types
}

// genresOf は、genre1〜3 と subGenre1〜3 の列を、ジャンルの一覧にする。大分類が NULL の組は捨てる。
func genresOf(pairs [][2]sql.NullInt64) []*Genre {
	genres := []*Genre{}
	for _, pair := range pairs {
		if !pair[0].Valid {
			continue
		}
		genres = append(genres, &Genre{Genre: int(pair[0].Int64), SubGenre: intOrNil(pair[1])})
	}

	return genres
}

// decodeJSON は、JSON の配列を入れた列を読む。NULL なら空のスライスを返す。
func decodeJSON[T any](column sql.NullString, name string) ([]T, error) {
	items := []T{}
	if !column.Valid || column.String == "" {
		return items, nil
	}
	if err := json.Unmarshal([]byte(column.String), &items); err != nil {
		return nil, fmt.Errorf("failed to decode %s: %w", name, err)
	}

	return items, nil
}

// decodePeriods は、searchPeriods の列 ({startAt, endAt} のミリ秒の配列) を読む。
func decodePeriods(column sql.NullString) ([]*Period, error) {
	raw, err := decodeJSON[struct {
		StartAt int64 `json:"startAt"`
		EndAt   int64 `json:"endAt"`
	}](column, "searchPeriods")
	if err != nil {
		return nil, err
	}

	periods := make([]*Period, 0, len(raw))
	for _, p := range raw {
		periods = append(periods, &Period{StartAt: fromMillis(p.StartAt), EndAt: fromMillis(p.EndAt)})
	}

	return periods, nil
}

func intOrNil(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}

	return new(int(v.Int64))
}

func int64OrNil(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}

	return new(v.Int64)
}

// fromMillis は、EPGStation が保存する Unix 時間のミリ秒を、UTC の時刻にする (設計書 8.1)。
func fromMillis(ms int64) time.Time {
	return time.UnixMilli(ms).UTC()
}

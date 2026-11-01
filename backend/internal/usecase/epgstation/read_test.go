package epgstation_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/testutil/epgstationtest"
	"github.com/SlashNephy/kotatsu/internal/usecase/epgstation"
)

// 番組とチャンネルの ID は、EPGStation では 32 ビットに収まらない値になる。
const (
	channelID = 3273601024
	programID = 327360102412345
	// recordedDirectory は、EPGStation の既定の録画の保存先ディレクトリの名前。
	recordedDirectory = "recorded"
	// animeDirectory と encodedDirectory は、録画の保存先ディレクトリやサブディレクトリの名前。
	animeDirectory   = "anime"
	encodedDirectory = "encoded"
	// recordedFormat は、ルールと予約のファイル名の形式。
	recordedFormat = "%TITLE%"
	// encodeName は、エンコードしたファイルの、エンコードの設定の名前。
	encodeName = "H.264"
	// animeTag はタグの名前。
	animeTag = "アニメ"
	// startAt は 2026-10-10 21:00 (JST) のミリ秒。
	startAt = 1791633600000
	endAt   = startAt + 30*60*1000
)

// seed は、方言によらず実行できる INSERT 文で、テストのデータを入れる。
func seed() []string {
	return []string{
		`INSERT INTO recorded_tag (id, name, halfWidthName, color) VALUES (1, 'アニメ', 'アニメ', '#ff0000'), (2, 'お気に入り', 'お気に入り', '#00ff00')`,
		`INSERT INTO rule (id, updateCnt, enable, keyword, halfWidthKeyword, keyCS, name, description,
		ignoreKeyword, halfWidthIgnoreKeyword, ignoreKeyRegExp, ignoreExtended,
		GR, BS, channelIds, genres, times, isFree, durationMin, durationMax, searchPeriods,
		avoidDuplicate, periodToAvoidDuplicate, allowEndLack, tags, parentDirectoryName, directory, recordedFormat, mode1)
		VALUES (1, 3, 1, 'ゆるキャン', 'ゆるキャン', 1, 1, 1,
		'再放送|総集編', '再放送|総集編', 1, 1,
		1, 1, '[3273601024]', '[{"genre":7,"subGenre":0},{"genre":6}]', '[{"start":21,"range":3,"week":127}]', 1, 600, 3600,
		'[{"startAt":1791633600000,"endAt":1791720000000}]',
		1, 30, 0, '[1,2]', 'recorded', 'anime', '%TITLE%', 'H.264')`,
		// 項目をほとんど省いた時刻指定のルール。
		`INSERT INTO rule (id, isTimeSpecification) VALUES (2, 1)`,
		`INSERT INTO reserve (id, updateTime, ruleId, programId, channelId, channel, channelType, startAt, endAt, name, tags, parentDirectoryName, directory, recordedFormat)
		VALUES (1, 1, NULL, 327360102412345, 3273601024, '27', 'GR', 1791633600000, 1791635400000, 'ニュース', '[2]', 'recorded', 'news', '%TITLE%')`,
		`INSERT INTO reserve (id, updateTime, ruleId, isTimeSpecified, isSkip, isEventRelay, channelId, channel, channelType, startAt, endAt)
		VALUES (2, 1, NULL, 1, 1, 1, 3273601024, '27', 'GR', 1791633600000, 1791635400000)`,
		// ルール予約は読まない。
		`INSERT INTO reserve (id, updateTime, ruleId, ruleUpdateCnt, programId, channelId, channel, channelType, startAt, endAt, name)
		VALUES (3, 1, 1, 3, 327360102412346, 3273601024, '27', 'GR', 1791633600000, 1791635400000, 'ゆるキャン△')`,
		`INSERT INTO drop_log_file (id, errorCnt, dropCnt, scramblingCnt, filePath) VALUES (1, 4, 5, 6, '1.m2ts.log')`,
		`INSERT INTO recorded (id, reserveId, ruleId, programId, channelId, isProtected, startAt, endAt, duration, name, halfWidthName,
		description, halfWidthDescription, extended, halfWidthExtended, genre1, subGenre1, genre2, subGenre2, isRecording, dropLogFileId)
		VALUES (10, 3, 1, 327360102412346, 3273601024, 1, 1791633600000, 1791635400000, 1800000, 'ゆるキャン△', 'ゆるキャン△',
		'概要', '概要', '詳細', '詳細', 7, 0, 6, NULL, 0, 1)`,
		// 録画中で、番組もルールもドロップの集計もない録画。
		`INSERT INTO recorded (id, channelId, startAt, endAt, duration, name, halfWidthName, isRecording)
		VALUES (11, 3273601024, 1791633600000, 1791635400000, 1800000, '時刻指定', '時刻指定', 1)`,
		`INSERT INTO video_file (id, parentDirectoryName, filePath, type, name, size, recordedId)
		VALUES (100, 'recorded', 'anime/ゆるキャン△.m2ts', 'ts', 'TS', 1234567890123, 10),
		(101, 'encoded', 'ゆるキャン△.mp4', 'encoded', 'H.264', 456, 10)`,
		`INSERT INTO recorded_tags_recorded_tag (recordedId, recordedTagId) VALUES (10, 1), (10, 2)`,
		`INSERT INTO recorded_history (id, name, channelId, endAt) VALUES (1, 'ゆるキャン△', 3273601024, 1791635400000)`,
	}
}

func expectedSnapshot() *epgstation.Snapshot {
	return &epgstation.Snapshot{
		Rules: []*epgstation.Rule{
			{
				ID:                  1,
				Enabled:             true,
				IsTimeSpecification: false,
				Keyword: epgstation.KeywordOption{
					Keyword: "ゆるキャン", CaseSensitive: true, Regexp: false, Name: true, Description: true, Extended: false,
				},
				IgnoreKeyword: epgstation.KeywordOption{
					Keyword: "再放送|総集編", CaseSensitive: false, Regexp: true, Name: false, Description: false, Extended: true,
				},
				ChannelTypes:           []string{"GR", "BS"},
				ChannelIDs:             []int64{channelID},
				Genres:                 []*epgstation.Genre{{Genre: 7, SubGenre: new(0)}, {Genre: 6, SubGenre: nil}},
				Times:                  []*epgstation.SearchTime{{Start: new(21), Range: new(3), Week: 127}},
				FreeOnly:               true,
				DurationMin:            new(600),
				DurationMax:            new(3600),
				SearchPeriods:          []*epgstation.Period{{StartAt: time.UnixMilli(startAt).UTC(), EndAt: time.UnixMilli(1791720000000).UTC()}},
				AvoidDuplicate:         true,
				PeriodToAvoidDuplicate: new(30),
				AllowEndLack:           false,
				TagIDs:                 []int64{1, 2},
				ParentDirectoryName:    recordedDirectory,
				Directory:              animeDirectory,
				RecordedFormat:         recordedFormat,
			},
			{
				ID:                     2,
				Enabled:                false,
				IsTimeSpecification:    true,
				Keyword:                epgstation.KeywordOption{Keyword: "", CaseSensitive: false, Regexp: false, Name: false, Description: false, Extended: false},
				IgnoreKeyword:          epgstation.KeywordOption{Keyword: "", CaseSensitive: false, Regexp: false, Name: false, Description: false, Extended: false},
				ChannelTypes:           []string{},
				ChannelIDs:             []int64{},
				Genres:                 []*epgstation.Genre{},
				Times:                  []*epgstation.SearchTime{},
				FreeOnly:               false,
				DurationMin:            nil,
				DurationMax:            nil,
				SearchPeriods:          []*epgstation.Period{},
				AvoidDuplicate:         false,
				PeriodToAvoidDuplicate: nil,
				// EPGStation の既定値は true。
				AllowEndLack:        true,
				TagIDs:              []int64{},
				ParentDirectoryName: "",
				Directory:           "",
				RecordedFormat:      "",
			},
		},
		ManualReserves: []*epgstation.Reserve{
			{
				ID:                  1,
				IsSkip:              false,
				AllowEndLack:        false,
				IsTimeSpecified:     false,
				IsEventRelay:        false,
				ProgramID:           new(int64(programID)),
				ChannelID:           channelID,
				ChannelType:         "GR",
				StartAt:             time.UnixMilli(startAt).UTC(),
				EndAt:               time.UnixMilli(endAt).UTC(),
				Name:                "ニュース",
				TagIDs:              []int64{2},
				ParentDirectoryName: recordedDirectory,
				Directory:           "news",
				RecordedFormat:      recordedFormat,
			},
			{
				ID:                  2,
				IsSkip:              true,
				AllowEndLack:        false,
				IsTimeSpecified:     true,
				IsEventRelay:        true,
				ProgramID:           nil,
				ChannelID:           channelID,
				ChannelType:         "GR",
				StartAt:             time.UnixMilli(startAt).UTC(),
				EndAt:               time.UnixMilli(endAt).UTC(),
				Name:                "",
				TagIDs:              []int64{},
				ParentDirectoryName: "",
				Directory:           "",
				RecordedFormat:      "",
			},
		},
		Recordings: []*epgstation.Recording{
			{
				ID:          10,
				RuleID:      new(int64(1)),
				ProgramID:   new(int64(programID + 1)),
				ChannelID:   channelID,
				IsProtected: true,
				IsRecording: false,
				StartAt:     time.UnixMilli(startAt).UTC(),
				EndAt:       time.UnixMilli(endAt).UTC(),
				Name:        "ゆるキャン△",
				Description: "概要",
				Extended:    "詳細",
				Genres:      []*epgstation.Genre{{Genre: 7, SubGenre: new(0)}, {Genre: 6, SubGenre: nil}},
				Files: []*epgstation.VideoFile{
					{ID: 100, ParentDirectoryName: recordedDirectory, FilePath: "anime/ゆるキャン△.m2ts", Type: epgstation.VideoFileTypeTS, Name: "TS", Size: 1234567890123},
					{ID: 101, ParentDirectoryName: encodedDirectory, FilePath: "ゆるキャン△.mp4", Type: epgstation.VideoFileTypeEncoded, Name: encodeName, Size: 456},
				},
				Drop:   &epgstation.DropLog{ErrorCount: 4, DropCount: 5, ScramblingCount: 6},
				TagIDs: []int64{1, 2},
			},
			{
				ID:          11,
				RuleID:      nil,
				ProgramID:   nil,
				ChannelID:   channelID,
				IsProtected: false,
				IsRecording: true,
				StartAt:     time.UnixMilli(startAt).UTC(),
				EndAt:       time.UnixMilli(endAt).UTC(),
				Name:        "時刻指定",
				Description: "",
				Extended:    "",
				Genres:      []*epgstation.Genre{},
				Files:       []*epgstation.VideoFile{},
				Drop:        nil,
				TagIDs:      []int64{},
			},
		},
		Histories: []*epgstation.RecordedHistory{
			{ID: 1, Name: "ゆるキャン△", ChannelID: channelID, EndAt: time.UnixMilli(endAt).UTC()},
		},
		Tags: []*epgstation.Tag{
			{ID: 1, Name: animeTag, Color: "#ff0000"},
			{ID: 2, Name: "お気に入り", Color: "#00ff00"},
		},
	}
}

// TestReadUsesSlashInFilePath は、Windows の EPGStation が \ で区切って記録した録画のファイルのパスを、
// Kotatsu のプール内の相対パスと同じ / 区切りにして読むことを確かめる (#1098)。
// EPGStation は、ファイル名の \ を ￥ に置き換えるので、\ は区切りだけに使われる。
func TestReadUsesSlashInFilePath(t *testing.T) {
	t.Parallel()

	for _, dialect := range epgstationtest.Dialects() {
		t.Run(string(dialect), func(t *testing.T) {
			t.Parallel()

			db := epgstationtest.New(t, dialect)
			epgstationtest.Exec(t, db.Writer, seed()...)
			// MySQL は文字列のリテラルの \ をエスケープとみなすので、値はパラメーターで渡す。
			_, err := db.Writer.ExecContext(t.Context(), `INSERT INTO video_file (id, parentDirectoryName, filePath, type, name, size, recordedId)
				VALUES (?, ?, ?, ?, ?, ?, ?)`, 102, encodedDirectory, `anime\sub\ゆるキャン△.mp4`, "encoded", "H.265", 789, 10)
			require.NoError(t, err)
			source, err := epgstation.Open(t.Context(), db.URL)
			require.NoError(t, err)
			t.Cleanup(func() {
				_ = source.Close()
			})

			got, err := source.Read(t.Context())
			require.NoError(t, err)
			paths := map[int64]string{}
			for _, recording := range got.Recordings {
				for _, file := range recording.Files {
					paths[file.ID] = file.FilePath
				}
			}
			assert.Equal(t, "anime/sub/ゆるキャン△.mp4", paths[102])
			assert.Equal(t, "anime/ゆるキャン△.m2ts", paths[100])
		})
	}
}

func TestRead(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		seed []string
		want *epgstation.Snapshot
	}{
		{
			name: "reads every kind of data",
			seed: seed(),
			want: expectedSnapshot(),
		},
		{
			name: "reads an empty database",
			seed: []string{},
			want: &epgstation.Snapshot{
				Rules:          []*epgstation.Rule{},
				ManualReserves: []*epgstation.Reserve{},
				Recordings:     []*epgstation.Recording{},
				Histories:      []*epgstation.RecordedHistory{},
				Tags:           []*epgstation.Tag{},
			},
		},
	}
	for _, dialect := range epgstationtest.Dialects() {
		for _, tt := range tests {
			t.Run(string(dialect)+"/"+tt.name, func(t *testing.T) {
				t.Parallel()

				db := epgstationtest.New(t, dialect)
				epgstationtest.Exec(t, db.Writer, tt.seed...)

				source, err := epgstation.Open(t.Context(), db.URL)
				require.NoError(t, err)
				t.Cleanup(func() {
					_ = source.Close()
				})

				got, err := source.Read(t.Context())
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			})
		}
	}
}

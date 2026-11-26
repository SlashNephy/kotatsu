package backup

import "strings"

// includedTables は、バックアップに入れるテーブルを返す (設計書 11.4、#1926)。
// テーブルを足したら、ここか excludedTables のどちらかに足す。どちらにもないテーブルがあると、作成を止める。
// 足し忘れは、マイグレーションを当てた DB から作成するテストで見つける。
func includedTables() []string {
	return []string{
		"schema_compatibility",
		"leases",
		"instances",
		"audit_events",
		"users",
		"api_tokens",
		"stream_urls",
		"calendar_feeds",
		"oauth_authorization_codes",
		"oauth_grants",
		"oauth_tokens",
		"setup_tokens",
		"login_challenges",
		"user_totps",
		"totp_enrollments",
		"user_recovery_codes",
		"user_oidc_identities",
		"user_passkeys",
		"webauthn_ceremonies",
		"login_throttles",
		"system_settings",
		"notification_events",
		"notifications",
		"notification_preferences",
		"user_setting_versions",
		"player_versions",
		"user_settings",
		"web_push_subscriptions",
		"vapid_keys",
		"webhooks",
		"channel_types",
		"services",
		"broadcast_lineups",
		"terrestrial_lineups",
		"broadcast_service_activities",
		"service_logos",
		"program_archives",
		"reservations",
		"recording_assignments",
		"tuners",
		"tuner_occupancies",
		"tuner_server_status",
		"rule_groups",
		"rules",
		"reservation_rules",
		"storage_pools",
		"pinned_services",
		"recordings",
		"recording_failure_counts",
		"scramble_alerts",
		"live_scramble_verdicts",
		"recording_files",
		"recording_live_stats",
		"recording_cm_detections",
		"recording_cm_sections",
		"recording_cm_materials",
		"recording_auto_captions",
		"caption_models",
		"tags",
		"recording_tags",
		"watch_progress",
		"library_watch_progress",
		"recording_history",
		"guide_views",
		"watch_rules",
		"watch_reservations",
		"watch_rule_dismissals",
		"captures",
		"import_sources",
		"timeshift_channels",
		"timeshift_watches",
		"timeshift_assignments",
		"timeshift_chunks",
		"timeshift_programs",
		"timeshift_promotions",
		"syobocal_titles",
		"anime_title_aliases",
		"anime_channel_definitions",
		"anime_channel_overrides",
		"anime_works",
		"anime_imports",
		"syobocal_programs",
		"anime_episode_broadcasts",
		"recording_nicolog_files",
		"anime_title_broadcast_lookups",
		"syobocal_lookups",
		"program_anime_matches",
		"program_anime_episodes",
		"recording_anime_matches",
		"recording_anime_episodes",
		"anime_record_apps",
		"anime_import_tokens",
		"annict_works",
		"annict_work_sync",
		"anime_record_connections",
		"anime_record_settings",
		"anime_watch_progress",
		"anime_records",
		"anime_artworks",
		"anime_wishlist_entries",
		"anime_following_fetches",
		"anime_following_viewings",
		"anime_following_watching",
		"network_activity",
		"edcb_network_tvs",
		"edcb_network_tv_holders",
		"tmdb_searches",
		"tmdb_series",
		"tmdb_seasons",
		"program_drama_matches",
		"recording_drama_matches",
		"tmdb_tokens",
		"drama_title_overrides",
		"drama_watchlist_entries",
		"drama_watched_episodes",
		"drama_record_apps",
		"drama_record_connections",
		"drama_record_settings",
		"drama_records",
		"drama_watchlist_imports",
		"google_apps",
		"google_drive_connections",
		"google_drive_folders",
		"backup_settings",
		"backup_runs",
	}
}

// refersToSessions は、sessions を指すので外すテーブルの理由。
const refersToSessions = "refers to sessions"

// excludedTables は、バックアップに入れないテーブルと、その理由を返す (設計書 11.4、#1926)。
// 戻した後は、マイグレーションが作った空のテーブル (と、マイグレーションが入れた行) のまま始める。
func excludedTables() map[string]string {
	return map[string]string{
		"programs":        "the EPG is fetched again from the tuner server",
		"epg_changes":     "the change log of programs; its versions start over with change_versions",
		"change_versions": "the versions of change notifications start over after a restore",
		// 照合のカーソルは epg_changes のバージョンを持つ。バージョンが 1 から数え直すので、残すと、照合が変更を読み飛ばす。
		// 行がなければ、照合は今のバージョンから読み始め、それまでの番組は定期的な照合し直しが受け持つ (設計書 3.8、3.9)。
		"anime_match_cursor": "holds a version of epg_changes, which starts over after a restore",
		"drama_match_cursor": "holds a version of epg_changes, which starts over after a restore",
		"sessions":           "restoring sessions would revive old sign-ins; everyone signs in again",
		// 次の 5 つは、sessions を外部キーで指す途中の状態 (ログインの手続き、連携の認可、再生中の端末) なので、sessions と組で外す。
		"players":                     refersToSessions,
		"player_commands":             "refers to players",
		"oidc_logins":                 refersToSessions,
		"anime_record_authorizations": refersToSessions,
		"drama_record_authorizations": refersToSessions,
		"google_authorizations":       refersToSessions,
		// River のジョブの ID を持つので、River のテーブルと組で外す。残すと、戻した後のジョブが同じ ID の古い進み具合を読む。
		"job_progress":     "holds IDs of River jobs, which are not backed up",
		"goose_db_version": "the schema version is applied by kotatsu migrate",
		"goose_lock":       "the lock of kotatsu migrate",
	}
}

// nullColumns は、作成するときに NULL にするカラムを返す。外したテーブルを指す、NULL にできるカラムが当たる。
func nullColumns() map[string][]string {
	return map[string][]string{
		// Web Push の購読は残し、ログインしたセッションとの結びつきだけを外す。セッションを失効させたときに購読を止めるのに使うカラムである。
		"web_push_subscriptions": {"session_id"},
	}
}

// riverTablePrefix は、River のテーブルの名前の接頭辞。River のテーブルは River のマイグレーションが作り、ジョブは戻さない。
// River のバージョンによってテーブルが増えるので、名前を並べずに接頭辞で外す。
const riverTablePrefix = "river_"

// isExcluded は、テーブル name をバックアップに入れないかを返す。
func isExcluded(name string) bool {
	_, ok := excludedTables()[name]

	return ok || strings.HasPrefix(name, riverTablePrefix)
}

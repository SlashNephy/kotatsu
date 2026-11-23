-- v2026.11.5 の kotatsu migrate (goose のマイグレーション) で上げた DB のスキーマと、goose の版 (#2040)。
-- goose のマイグレーションのファイルを消した後も、新しい仕組みへ移る処理を確かめるために置く。
-- goose のマイグレーションを 20261120170214 まで当てた DB を pg_dump で書き出し、psql の命令とセッションの設定を外した。
-- 手では直さない。

CREATE TABLE public.anime_artworks (
    mal_id integer NOT NULL,
    image_url text,
    fetched_at timestamp with time zone NOT NULL
);

CREATE TABLE public.anime_channel_definitions (
    channel_type text NOT NULL,
    network_id bigint NOT NULL,
    sid bigint NOT NULL,
    name text NOT NULL,
    syobocal_channel_id integer,
    annict_channel_id integer,
    jikkyo_channel_id integer
);

CREATE TABLE public.anime_channel_overrides (
    service_id bigint NOT NULL,
    syobocal_channel_id integer,
    annict_channel_id integer,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT anime_channel_overrides_annict_channel_id_check CHECK ((annict_channel_id > 0)),
    CONSTRAINT anime_channel_overrides_check CHECK (((syobocal_channel_id IS NOT NULL) OR (annict_channel_id IS NOT NULL))),
    CONSTRAINT anime_channel_overrides_syobocal_channel_id_check CHECK ((syobocal_channel_id > 0))
);

CREATE TABLE public.anime_episode_broadcasts (
    pid integer NOT NULL,
    tid integer NOT NULL,
    channel_id integer NOT NULL,
    start_at timestamp with time zone NOT NULL,
    end_at timestamp with time zone NOT NULL,
    offset_seconds integer NOT NULL,
    episode_count double precision NOT NULL
);

CREATE TABLE public.anime_following_fetches (
    user_id uuid NOT NULL,
    destination text NOT NULL,
    syobocal_tid integer NOT NULL,
    checked_at timestamp with time zone NOT NULL,
    fetched_at timestamp with time zone,
    error text,
    retry_at timestamp with time zone
);

CREATE TABLE public.anime_following_viewings (
    user_id uuid NOT NULL,
    destination text NOT NULL,
    syobocal_tid integer NOT NULL,
    username text NOT NULL,
    name text NOT NULL,
    avatar_url text,
    status text NOT NULL,
    progress integer,
    episodes_count integer,
    score double precision,
    notes text,
    CONSTRAINT anime_following_viewings_status_check CHECK ((status = ANY (ARRAY['watched'::text, 'watching'::text, 'repeating'::text, 'on_hold'::text, 'stop_watching'::text, 'wanna_watch'::text])))
);

CREATE TABLE public.anime_import_tokens (
    id smallint NOT NULL,
    annict_token_encrypted bytea NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT anime_import_tokens_id_check CHECK ((id = 1))
);

CREATE TABLE public.anime_imports (
    source text NOT NULL,
    etag text,
    checked_at timestamp with time zone NOT NULL,
    imported_at timestamp with time zone,
    item_count integer NOT NULL,
    error text,
    CONSTRAINT anime_imports_source_check CHECK ((source = ANY (ARRAY['saya_definitions'::text, 'arm'::text, 'syobocal_titles'::text, 'annict_works'::text])))
);

CREATE TABLE public.anime_match_cursor (
    id smallint NOT NULL,
    epg_version bigint NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT anime_match_cursor_id_check CHECK ((id = 1))
);

CREATE TABLE public.anime_record_apps (
    destination text NOT NULL,
    client_id text NOT NULL,
    client_secret_encrypted bytea NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT anime_record_apps_destination_check CHECK ((destination = ANY (ARRAY['annict'::text, 'anilist'::text])))
);

CREATE TABLE public.anime_record_authorizations (
    id uuid NOT NULL,
    state_hash bytea NOT NULL,
    session_id uuid NOT NULL,
    user_id uuid NOT NULL,
    destination text NOT NULL,
    code_verifier text NOT NULL,
    redirect_uri text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    CONSTRAINT anime_record_authorizations_destination_check CHECK ((destination = ANY (ARRAY['annict'::text, 'anilist'::text])))
);

CREATE TABLE public.anime_record_connections (
    user_id uuid NOT NULL,
    destination text NOT NULL,
    access_token_encrypted bytea NOT NULL,
    account_name text NOT NULL,
    connected_at timestamp with time zone NOT NULL,
    wishlist_checked_at timestamp with time zone,
    wishlist_imported_at timestamp with time zone,
    wishlist_error text,
    wishlist_retry_at timestamp with time zone,
    CONSTRAINT anime_record_connections_destination_check CHECK ((destination = ANY (ARRAY['annict'::text, 'anilist'::text])))
);

CREATE TABLE public.anime_record_settings (
    user_id uuid NOT NULL,
    threshold_percent smallint DEFAULT 20 NOT NULL,
    watching_on_first boolean DEFAULT false NOT NULL,
    watching_on_episode boolean DEFAULT false NOT NULL,
    watched_on_last boolean DEFAULT false NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    notify_wishlist boolean DEFAULT false NOT NULL,
    CONSTRAINT anime_record_settings_threshold_percent_check CHECK (((threshold_percent >= 1) AND (threshold_percent <= 100)))
);

CREATE TABLE public.anime_records (
    id uuid NOT NULL,
    user_id uuid NOT NULL,
    service_id bigint NOT NULL,
    start_at timestamp with time zone NOT NULL,
    destination text NOT NULL,
    syobocal_tid integer NOT NULL,
    episode_number double precision,
    subtitle text,
    is_first boolean NOT NULL,
    is_last boolean NOT NULL,
    program_name text NOT NULL,
    state text NOT NULL,
    url text,
    error text,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT anime_records_destination_check CHECK ((destination = ANY (ARRAY['annict'::text, 'anilist'::text]))),
    CONSTRAINT anime_records_state_check CHECK ((state = ANY (ARRAY['pending'::text, 'recorded'::text, 'failed'::text])))
);

CREATE TABLE public.anime_title_broadcast_lookups (
    tid integer NOT NULL,
    fetched_at timestamp with time zone NOT NULL
);

CREATE TABLE public.anime_watch_progress (
    user_id uuid NOT NULL,
    service_id bigint NOT NULL,
    start_at timestamp with time zone NOT NULL,
    played jsonb NOT NULL,
    played_ms bigint NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE TABLE public.anime_wishlist_entries (
    user_id uuid NOT NULL,
    destination text NOT NULL,
    work_id integer NOT NULL,
    status text NOT NULL,
    title text NOT NULL,
    fetched_at timestamp with time zone NOT NULL,
    next_episode_id text,
    next_episode_number integer,
    next_episode_number_text text,
    next_episode_title text,
    progress integer,
    episodes_count integer,
    season_year integer,
    season_name text,
    image_url text,
    CONSTRAINT anime_wishlist_entries_progress_check CHECK ((progress >= 0)),
    CONSTRAINT anime_wishlist_entries_season_name_check CHECK ((season_name = ANY (ARRAY['winter'::text, 'spring'::text, 'summer'::text, 'autumn'::text]))),
    CONSTRAINT anime_wishlist_entries_status_check CHECK ((status = ANY (ARRAY['wanna_watch'::text, 'watching'::text])))
);

CREATE TABLE public.anime_works (
    source text NOT NULL,
    syobocal_tid integer NOT NULL,
    "position" smallint NOT NULL,
    annict_work_id integer,
    anilist_id integer,
    mal_id integer,
    CONSTRAINT anime_works_source_check CHECK ((source = ANY (ARRAY['annict'::text, 'arm'::text])))
);

CREATE TABLE public.annict_work_sync (
    id smallint NOT NULL,
    seasonal_at timestamp with time zone,
    full_cursor text,
    full_completed_at timestamp with time zone,
    CONSTRAINT annict_work_sync_id_check CHECK ((id = 1))
);

CREATE TABLE public.annict_works (
    annict_id integer NOT NULL,
    syobocal_tid integer,
    mal_anime_id integer,
    anilist_id integer,
    anilist_checked_at timestamp with time zone,
    fetched_at timestamp with time zone NOT NULL
);

CREATE TABLE public.api_tokens (
    id uuid NOT NULL,
    user_id uuid NOT NULL,
    name text NOT NULL,
    token_hash bytea NOT NULL,
    created_at timestamp with time zone NOT NULL,
    last_used_at timestamp with time zone
);

CREATE TABLE public.audit_events (
    id uuid NOT NULL,
    occurred_at timestamp with time zone NOT NULL,
    actor_type text NOT NULL,
    actor_user_id uuid,
    actor_role text,
    action text NOT NULL,
    target_type text NOT NULL,
    target_id text,
    summary jsonb NOT NULL,
    source_ip inet,
    user_agent text,
    actor_client text,
    CONSTRAINT audit_events_actor_check CHECK ((((actor_type = 'user'::text) AND (actor_user_id IS NOT NULL) AND (actor_role IS NULL)) OR ((actor_type = 'system'::text) AND (actor_user_id IS NULL) AND (actor_role IS NOT NULL))))
);

CREATE TABLE public.backup_runs (
    id uuid NOT NULL,
    trigger text NOT NULL,
    started_at timestamp with time zone NOT NULL,
    finished_at timestamp with time zone NOT NULL,
    file_name text,
    size_bytes bigint,
    error text,
    CONSTRAINT backup_runs_check CHECK (((error IS NULL) = (file_name IS NOT NULL))),
    CONSTRAINT backup_runs_size_bytes_check CHECK ((size_bytes >= 0)),
    CONSTRAINT backup_runs_trigger_check CHECK ((trigger = ANY (ARRAY['schedule'::text, 'manual'::text])))
);

CREATE TABLE public.backup_settings (
    id smallint NOT NULL,
    recipients text[] NOT NULL,
    interval_hours integer NOT NULL,
    generations integer NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT backup_settings_generations_check CHECK (((generations >= 1) AND (generations <= 365))),
    CONSTRAINT backup_settings_id_check CHECK ((id = 1)),
    CONSTRAINT backup_settings_interval_hours_check CHECK (((interval_hours >= 1) AND (interval_hours <= 720)))
);

CREATE TABLE public.broadcast_lineups (
    network_id integer NOT NULL,
    version smallint NOT NULL,
    streams jsonb NOT NULL,
    observed_at timestamp with time zone NOT NULL
);

CREATE TABLE public.calendar_feeds (
    user_id uuid NOT NULL,
    token_hash bytea NOT NULL,
    created_at timestamp with time zone NOT NULL,
    last_used_at timestamp with time zone,
    kind text DEFAULT 'calendar'::text NOT NULL,
    CONSTRAINT calendar_feeds_kind_check CHECK ((kind = ANY (ARRAY['calendar'::text, 'iptv'::text])))
);

CREATE TABLE public.caption_models (
    kind text NOT NULL,
    model_id text NOT NULL,
    size bigint NOT NULL,
    sha256 text NOT NULL,
    installed_at timestamp with time zone NOT NULL
);

CREATE TABLE public.captures (
    id uuid NOT NULL,
    user_id uuid NOT NULL,
    recording_id uuid,
    position_ms bigint,
    service_name text NOT NULL,
    program_name text,
    service_name_normalized text NOT NULL,
    program_name_normalized text NOT NULL,
    caption text,
    broadcast_at timestamp with time zone NOT NULL,
    taken_at timestamp with time zone NOT NULL,
    size bigint NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT captures_position_ms_check CHECK ((position_ms >= 0))
);

CREATE TABLE public.change_versions (
    channel text NOT NULL,
    version bigint NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE TABLE public.channel_types (
    type text NOT NULL,
    display_name text NOT NULL,
    sort_order integer NOT NULL
);

CREATE TABLE public.drama_match_cursor (
    id smallint NOT NULL,
    epg_version bigint NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT drama_match_cursor_id_check CHECK ((id = 1))
);

CREATE TABLE public.drama_record_apps (
    destination text NOT NULL,
    client_id text NOT NULL,
    client_secret_encrypted bytea NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT drama_record_apps_destination_check CHECK ((destination = 'simkl'::text))
);

CREATE TABLE public.drama_record_authorizations (
    id uuid NOT NULL,
    state_hash bytea NOT NULL,
    session_id uuid NOT NULL,
    user_id uuid NOT NULL,
    destination text NOT NULL,
    code_verifier text NOT NULL,
    redirect_uri text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    CONSTRAINT drama_record_authorizations_destination_check CHECK ((destination = 'simkl'::text))
);

CREATE TABLE public.drama_record_connections (
    user_id uuid NOT NULL,
    destination text NOT NULL,
    access_token_encrypted bytea NOT NULL,
    refresh_token_encrypted bytea NOT NULL,
    access_token_expires_at timestamp with time zone NOT NULL,
    account_name text NOT NULL,
    connected_at timestamp with time zone NOT NULL,
    reauthorization_required boolean DEFAULT false NOT NULL,
    watchlist_activity text,
    watchlist_removed_activity text,
    watchlist_checked_at timestamp with time zone,
    watchlist_imported_at timestamp with time zone,
    watchlist_error text,
    watchlist_retry_at timestamp with time zone,
    CONSTRAINT drama_record_connections_destination_check CHECK ((destination = 'simkl'::text))
);

CREATE TABLE public.drama_record_settings (
    user_id uuid NOT NULL,
    threshold_percent smallint DEFAULT 20 NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT drama_record_settings_threshold_percent_check CHECK (((threshold_percent >= 1) AND (threshold_percent <= 100)))
);

CREATE TABLE public.drama_records (
    id uuid NOT NULL,
    user_id uuid NOT NULL,
    service_id bigint NOT NULL,
    start_at timestamp with time zone NOT NULL,
    destination text NOT NULL,
    tmdb_series_id integer NOT NULL,
    season_number integer NOT NULL,
    episode_number integer NOT NULL,
    program_name text NOT NULL,
    watched_at timestamp with time zone NOT NULL,
    state text NOT NULL,
    url text,
    error text,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT drama_records_destination_check CHECK ((destination = 'simkl'::text)),
    CONSTRAINT drama_records_episode_number_check CHECK ((episode_number > 0)),
    CONSTRAINT drama_records_season_number_check CHECK ((season_number >= 0)),
    CONSTRAINT drama_records_state_check CHECK ((state = ANY (ARRAY['pending'::text, 'recorded'::text, 'failed'::text]))),
    CONSTRAINT drama_records_tmdb_series_id_check CHECK ((tmdb_series_id > 0))
);

CREATE TABLE public.drama_title_overrides (
    title_key text NOT NULL,
    tmdb_series_id integer,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT drama_title_overrides_title_key_check CHECK ((title_key <> ''::text)),
    CONSTRAINT drama_title_overrides_tmdb_series_id_check CHECK ((tmdb_series_id > 0))
);

CREATE TABLE public.drama_watched_episodes (
    user_id uuid NOT NULL,
    tmdb_series_id integer NOT NULL,
    season_number integer NOT NULL,
    episode_number integer NOT NULL,
    source text NOT NULL,
    watched_at timestamp with time zone NOT NULL,
    CONSTRAINT drama_watched_episodes_episode_number_check CHECK ((episode_number > 0)),
    CONSTRAINT drama_watched_episodes_season_number_check CHECK ((season_number >= 0)),
    CONSTRAINT drama_watched_episodes_source_check CHECK ((source = ANY (ARRAY['playback'::text, 'manual'::text]))),
    CONSTRAINT drama_watched_episodes_tmdb_series_id_check CHECK ((tmdb_series_id > 0))
);

CREATE TABLE public.drama_watchlist_entries (
    user_id uuid NOT NULL,
    tmdb_series_id integer NOT NULL,
    status text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT drama_watchlist_entries_status_check CHECK ((status = ANY (ARRAY['watching'::text, 'wanna_watch'::text]))),
    CONSTRAINT drama_watchlist_entries_tmdb_series_id_check CHECK ((tmdb_series_id > 0))
);

CREATE TABLE public.drama_watchlist_imports (
    user_id uuid NOT NULL,
    destination text NOT NULL,
    tmdb_series_id integer NOT NULL,
    simkl_id bigint,
    status text NOT NULL,
    applied boolean NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT drama_watchlist_imports_destination_check CHECK ((destination = 'simkl'::text)),
    CONSTRAINT drama_watchlist_imports_status_check CHECK ((status = ANY (ARRAY['watching'::text, 'plantowatch'::text, 'hold'::text, 'completed'::text, 'dropped'::text]))),
    CONSTRAINT drama_watchlist_imports_tmdb_series_id_check CHECK ((tmdb_series_id > 0))
);

CREATE TABLE public.edcb_network_tv_holders (
    network_tv_id integer NOT NULL,
    instance_id uuid NOT NULL,
    acquired_at timestamp with time zone NOT NULL
);

CREATE TABLE public.edcb_network_tvs (
    id integer NOT NULL,
    network_id integer NOT NULL,
    service_id integer NOT NULL,
    acquired_at timestamp with time zone NOT NULL,
    released_at timestamp with time zone,
    closing_at timestamp with time zone,
    CONSTRAINT edcb_network_tvs_id_check CHECK ((id > 0))
);

CREATE TABLE public.epg_changes (
    version bigint NOT NULL,
    service_id bigint NOT NULL,
    start_at timestamp with time zone NOT NULL,
    end_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone NOT NULL
);

CREATE TABLE public.google_apps (
    id smallint NOT NULL,
    client_id text NOT NULL,
    client_secret_encrypted bytea NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT google_apps_id_check CHECK ((id = 1))
);

CREATE TABLE public.google_authorizations (
    id uuid NOT NULL,
    state_hash bytea NOT NULL,
    session_id uuid NOT NULL,
    user_id uuid NOT NULL,
    purpose text NOT NULL,
    code_verifier text NOT NULL,
    redirect_uri text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    CONSTRAINT google_authorizations_purpose_check CHECK ((purpose = 'drive'::text))
);

CREATE TABLE public.google_drive_connections (
    id smallint NOT NULL,
    refresh_token_encrypted bytea NOT NULL,
    account_name text NOT NULL,
    connected_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    CONSTRAINT google_drive_connections_id_check CHECK ((id = 1))
);

CREATE TABLE public.google_drive_folders (
    folder_id text NOT NULL,
    drive_id text NOT NULL,
    name text NOT NULL,
    added_at timestamp with time zone NOT NULL,
    library_name text NOT NULL,
    CONSTRAINT google_drive_folders_folder_id_check CHECK ((folder_id <> ''::text)),
    CONSTRAINT google_drive_folders_library_name_check CHECK ((library_name ~ '^[a-z0-9][a-z0-9_-]{0,62}$'::text))
);

CREATE TABLE public.goose_db_version (
    id integer NOT NULL,
    version_id bigint NOT NULL,
    is_applied boolean NOT NULL,
    tstamp timestamp without time zone DEFAULT now() NOT NULL
);

ALTER TABLE public.goose_db_version ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.goose_db_version_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

CREATE TABLE public.guide_views (
    id uuid NOT NULL,
    user_id uuid NOT NULL,
    name text NOT NULL,
    settings jsonb NOT NULL,
    hide_empty_services boolean NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE TABLE public.import_sources (
    source text NOT NULL,
    kind text NOT NULL,
    source_id bigint NOT NULL,
    target_id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL
);

CREATE TABLE public.instances (
    id uuid NOT NULL,
    role text NOT NULL,
    address text NOT NULL,
    started_at timestamp with time zone NOT NULL,
    heartbeat_at timestamp with time zone NOT NULL
);

CREATE TABLE public.job_progress (
    job_id bigint NOT NULL,
    attempt integer NOT NULL,
    ratio double precision NOT NULL,
    remaining_ms bigint,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT job_progress_ratio_check CHECK (((ratio >= (0)::double precision) AND (ratio <= (1)::double precision))),
    CONSTRAINT job_progress_remaining_ms_check CHECK ((remaining_ms >= 0))
);

CREATE TABLE public.leases (
    name text NOT NULL,
    holder uuid NOT NULL,
    generation bigint NOT NULL,
    acquired_at timestamp with time zone NOT NULL,
    renewed_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL
);

CREATE TABLE public.library_watch_progress (
    user_id uuid NOT NULL,
    file_id text NOT NULL,
    position_ms bigint NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT library_watch_progress_file_id_check CHECK ((file_id ~ '^[0-9a-f]{32}$'::text)),
    CONSTRAINT library_watch_progress_position_ms_check CHECK ((position_ms >= 0))
);

CREATE TABLE public.live_scramble_verdicts (
    service_id bigint NOT NULL,
    decided_at timestamp with time zone NOT NULL
);

CREATE TABLE public.login_challenges (
    id uuid NOT NULL,
    token_hash bytea NOT NULL,
    user_id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    user_agent text NOT NULL,
    source_ip inet,
    failed_attempts integer DEFAULT 0 NOT NULL,
    webauthn_session jsonb
);

CREATE TABLE public.login_throttles (
    key text NOT NULL,
    failures integer NOT NULL,
    locked_until timestamp with time zone NOT NULL,
    last_failed_at timestamp with time zone NOT NULL
);

CREATE TABLE public.network_activity (
    hour timestamp with time zone NOT NULL,
    purpose text NOT NULL,
    host text NOT NULL,
    initiator text NOT NULL,
    success boolean NOT NULL,
    count bigint NOT NULL,
    sent_bytes bigint NOT NULL,
    received_bytes bigint NOT NULL
);

CREATE TABLE public.notification_events (
    id uuid NOT NULL,
    kind text NOT NULL,
    occurred_at timestamp with time zone NOT NULL,
    payload jsonb NOT NULL,
    dispatched_at timestamp with time zone,
    audience uuid[],
    by_rule boolean DEFAULT false NOT NULL,
    CONSTRAINT notification_events_kind_check CHECK ((kind = ANY (ARRAY['recording_started'::text, 'recording_completed'::text, 'recording_failed'::text, 'reservation_conflict'::text, 'storage_low'::text, 'tuner_server_unreachable'::text, 'watch_reminder'::text, 'anime_recorded'::text, 'anime_record_failed'::text, 'drama_recorded'::text, 'drama_record_failed'::text, 'scrambled_recordings'::text, 'lineup_changed'::text])))
);

CREATE TABLE public.notification_preferences (
    user_id uuid NOT NULL,
    kind text NOT NULL,
    in_app boolean NOT NULL,
    web_push boolean NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    rule_reservations boolean DEFAULT true NOT NULL,
    CONSTRAINT notification_preferences_kind_check CHECK ((kind = ANY (ARRAY['recording_started'::text, 'recording_completed'::text, 'recording_failed'::text, 'reservation_conflict'::text, 'storage_low'::text, 'tuner_server_unreachable'::text, 'watch_reminder'::text, 'anime_recorded'::text, 'anime_record_failed'::text, 'drama_recorded'::text, 'drama_record_failed'::text, 'scrambled_recordings'::text, 'lineup_changed'::text])))
);

CREATE TABLE public.notifications (
    id uuid NOT NULL,
    user_id uuid NOT NULL,
    event_id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL,
    read_at timestamp with time zone
);

CREATE TABLE public.oauth_authorization_codes (
    code_hash bytea NOT NULL,
    user_id uuid NOT NULL,
    client_id text NOT NULL,
    client_name text NOT NULL,
    redirect_uri text NOT NULL,
    code_challenge text NOT NULL,
    scopes text[] NOT NULL,
    resource text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL
);

CREATE TABLE public.oauth_grants (
    id uuid NOT NULL,
    user_id uuid NOT NULL,
    client_id text NOT NULL,
    client_name text NOT NULL,
    scopes text[] NOT NULL,
    resource text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    last_used_at timestamp with time zone
);

CREATE TABLE public.oauth_tokens (
    token_hash bytea NOT NULL,
    grant_id uuid NOT NULL,
    kind text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone,
    CONSTRAINT oauth_tokens_check CHECK (((kind = 'refresh'::text) OR (used_at IS NULL))),
    CONSTRAINT oauth_tokens_kind_check CHECK ((kind = ANY (ARRAY['access'::text, 'refresh'::text])))
);

CREATE TABLE public.oidc_logins (
    id uuid NOT NULL,
    token_hash bytea NOT NULL,
    state text NOT NULL,
    nonce text NOT NULL,
    code_verifier text NOT NULL,
    redirect_uri text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    session_id uuid,
    return_to text,
    link boolean DEFAULT false NOT NULL
);

CREATE TABLE public.pinned_services (
    user_id uuid NOT NULL,
    service_id bigint NOT NULL,
    created_at timestamp with time zone NOT NULL
);

CREATE TABLE public.player_commands (
    id bigint NOT NULL,
    player_id uuid NOT NULL,
    command jsonb NOT NULL,
    created_at timestamp with time zone NOT NULL
);

ALTER TABLE public.player_commands ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.player_commands_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

CREATE TABLE public.player_versions (
    user_id uuid NOT NULL,
    version bigint NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE TABLE public.players (
    id uuid NOT NULL,
    user_id uuid NOT NULL,
    session_id uuid NOT NULL,
    state jsonb NOT NULL,
    started_at timestamp with time zone NOT NULL,
    last_seen_at timestamp with time zone NOT NULL
);

CREATE TABLE public.program_anime_episodes (
    service_id bigint NOT NULL,
    start_at timestamp with time zone NOT NULL,
    "position" smallint NOT NULL,
    source text NOT NULL,
    syobocal_tid integer,
    syobocal_pid integer,
    count_start double precision,
    count_end double precision,
    subtitle text,
    is_first boolean NOT NULL,
    is_last boolean NOT NULL,
    is_rerun boolean DEFAULT false NOT NULL,
    CONSTRAINT program_anime_episodes_source_check CHECK ((source = ANY (ARRAY['syobocal'::text, 'title'::text])))
);

CREATE TABLE public.program_anime_matches (
    service_id bigint NOT NULL,
    start_at timestamp with time zone NOT NULL,
    event_id bigint NOT NULL,
    matched_at timestamp with time zone NOT NULL,
    finalized_at timestamp with time zone
);

CREATE TABLE public.program_archives (
    service_id bigint NOT NULL,
    event_id bigint NOT NULL,
    mirakurun_id bigint NOT NULL,
    start_at timestamp with time zone NOT NULL,
    end_at timestamp with time zone NOT NULL,
    end_undecided boolean NOT NULL,
    is_free boolean NOT NULL,
    name text NOT NULL,
    description text NOT NULL,
    name_normalized text NOT NULL,
    description_normalized text NOT NULL,
    extended jsonb NOT NULL,
    genres jsonb NOT NULL,
    related_items jsonb NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    archived_at timestamp with time zone NOT NULL,
    audios jsonb DEFAULT '[]'::jsonb NOT NULL,
    extended_normalized text,
    video_resolution text,
    series_episode integer,
    series_repeat boolean
);

CREATE TABLE public.program_drama_matches (
    service_id bigint NOT NULL,
    start_at timestamp with time zone NOT NULL,
    event_id bigint NOT NULL,
    title_key text NOT NULL,
    tmdb_series_id integer,
    season_number integer,
    episode_number integer,
    series_source text,
    episode_source text,
    matched_at timestamp with time zone NOT NULL,
    finalized_at timestamp with time zone,
    CONSTRAINT program_drama_matches_check CHECK (((tmdb_series_id IS NULL) = (series_source IS NULL))),
    CONSTRAINT program_drama_matches_check1 CHECK (((episode_number IS NULL) = (episode_source IS NULL))),
    CONSTRAINT program_drama_matches_check2 CHECK (((episode_number IS NULL) OR ((tmdb_series_id IS NOT NULL) AND (season_number IS NOT NULL)))),
    CONSTRAINT program_drama_matches_episode_number_check CHECK ((episode_number > 0)),
    CONSTRAINT program_drama_matches_episode_source_check CHECK ((episode_source = ANY (ARRAY['air_date'::text, 'title'::text]))),
    CONSTRAINT program_drama_matches_season_number_check CHECK ((season_number >= 0)),
    CONSTRAINT program_drama_matches_series_source_check CHECK ((series_source = ANY (ARRAY['search'::text, 'override'::text]))),
    CONSTRAINT program_drama_matches_tmdb_series_id_check CHECK ((tmdb_series_id > 0))
);

CREATE TABLE public.program_rewrites (
    service_id bigint NOT NULL,
    event_id bigint NOT NULL,
    start_at timestamp with time zone NOT NULL,
    field text NOT NULL,
    before text NOT NULL,
    after text NOT NULL,
    observed_at timestamp with time zone NOT NULL,
    CONSTRAINT program_rewrites_field_check CHECK ((field = ANY (ARRAY['name'::text, 'description'::text])))
);

CREATE TABLE public.programs (
    service_id bigint NOT NULL,
    event_id bigint NOT NULL,
    mirakurun_id bigint NOT NULL,
    start_at timestamp with time zone NOT NULL,
    end_at timestamp with time zone NOT NULL,
    end_undecided boolean NOT NULL,
    is_free boolean NOT NULL,
    name text NOT NULL,
    description text NOT NULL,
    name_normalized text NOT NULL,
    description_normalized text NOT NULL,
    extended jsonb NOT NULL,
    genres jsonb NOT NULL,
    related_items jsonb NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    audios jsonb DEFAULT '[]'::jsonb NOT NULL,
    extended_normalized text,
    video_resolution text,
    series_episode integer,
    series_repeat boolean,
    series text,
    episode_number integer
);

CREATE TABLE public.recording_anime_episodes (
    recording_id uuid NOT NULL,
    "position" smallint NOT NULL,
    source text NOT NULL,
    syobocal_tid integer,
    syobocal_pid integer,
    count_start double precision,
    count_end double precision,
    subtitle text,
    is_first boolean NOT NULL,
    is_last boolean NOT NULL,
    is_rerun boolean DEFAULT false NOT NULL,
    CONSTRAINT recording_anime_episodes_source_check CHECK ((source = ANY (ARRAY['syobocal'::text, 'title'::text])))
);

CREATE TABLE public.recording_anime_matches (
    recording_id uuid NOT NULL,
    matched_at timestamp with time zone NOT NULL
);

CREATE TABLE public.recording_assignments (
    id uuid NOT NULL,
    reservation_id uuid NOT NULL,
    priority integer NOT NULL,
    state text NOT NULL,
    generation bigint NOT NULL,
    holder uuid,
    lease_expires_at timestamp with time zone,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    stream_generation bigint DEFAULT 0 NOT NULL,
    CONSTRAINT recording_assignments_state_check CHECK ((state = ANY (ARRAY['waiting'::text, 'recording'::text, 'handover'::text, 'finished'::text])))
);

CREATE TABLE public.recording_auto_captions (
    recording_id uuid NOT NULL,
    model text NOT NULL,
    generator_version integer NOT NULL,
    generated_at timestamp with time zone NOT NULL,
    cues jsonb NOT NULL
);

CREATE TABLE public.recording_cm_detections (
    recording_id uuid NOT NULL,
    detector text NOT NULL,
    detector_version integer NOT NULL,
    detected_at timestamp with time zone NOT NULL,
    note text DEFAULT ''::text NOT NULL,
    video_missing_ms bigint DEFAULT 0 NOT NULL
);

CREATE TABLE public.recording_cm_materials (
    recording_id uuid NOT NULL,
    detector text NOT NULL,
    format_version integer NOT NULL,
    data bytea NOT NULL,
    created_at timestamp with time zone NOT NULL
);

CREATE TABLE public.recording_cm_sections (
    recording_id uuid NOT NULL,
    detector text NOT NULL,
    start_offset_ms bigint NOT NULL,
    end_offset_ms bigint NOT NULL,
    CONSTRAINT recording_cm_sections_check CHECK ((end_offset_ms > start_offset_ms)),
    CONSTRAINT recording_cm_sections_start_offset_ms_check CHECK ((start_offset_ms >= 0))
);

CREATE TABLE public.recording_drama_matches (
    recording_id uuid NOT NULL,
    tmdb_series_id integer,
    season_number integer,
    episode_number integer,
    series_source text,
    episode_source text,
    matched_at timestamp with time zone NOT NULL,
    CONSTRAINT recording_drama_matches_check CHECK (((tmdb_series_id IS NULL) = (series_source IS NULL))),
    CONSTRAINT recording_drama_matches_check1 CHECK (((episode_number IS NULL) = (episode_source IS NULL))),
    CONSTRAINT recording_drama_matches_check2 CHECK (((episode_number IS NULL) OR ((tmdb_series_id IS NOT NULL) AND (season_number IS NOT NULL)))),
    CONSTRAINT recording_drama_matches_episode_number_check CHECK ((episode_number > 0)),
    CONSTRAINT recording_drama_matches_episode_source_check CHECK ((episode_source = ANY (ARRAY['air_date'::text, 'title'::text]))),
    CONSTRAINT recording_drama_matches_season_number_check CHECK ((season_number >= 0)),
    CONSTRAINT recording_drama_matches_series_source_check CHECK ((series_source = ANY (ARRAY['search'::text, 'override'::text]))),
    CONSTRAINT recording_drama_matches_tmdb_series_id_check CHECK ((tmdb_series_id > 0))
);

CREATE TABLE public.recording_failure_counts (
    reason text NOT NULL,
    count bigint DEFAULT 0 NOT NULL,
    last_failed_at timestamp with time zone,
    reset_at timestamp with time zone,
    CONSTRAINT recording_failure_counts_reason_check CHECK ((reason = ANY (ARRAY['tuner_unavailable'::text, 'tuner_server_unreachable'::text, 'stream_interrupted'::text, 'storage_unavailable'::text, 'storage_full'::text, 'program_not_found'::text, 'cancelled'::text, 'recorder_unavailable'::text, 'tuner_preempted'::text, 'tuning_refused'::text])))
);

CREATE TABLE public.recording_files (
    id uuid NOT NULL,
    recording_id uuid NOT NULL,
    kind text NOT NULL,
    pool_id text NOT NULL,
    path text NOT NULL,
    size bigint NOT NULL,
    started_at timestamp with time zone NOT NULL,
    ended_at timestamp with time zone,
    created_at timestamp with time zone NOT NULL,
    pid_stats jsonb,
    video jsonb,
    tuner_index bigint,
    tuner_name text,
    transcode_frame_rate integer,
    external boolean DEFAULT false NOT NULL,
    CONSTRAINT recording_files_kind_check CHECK ((kind = ANY (ARRAY['segment'::text, 'other'::text])))
);

CREATE TABLE public.recording_history (
    id uuid NOT NULL,
    service_id bigint NOT NULL,
    title_key text NOT NULL,
    name text NOT NULL,
    start_at timestamp with time zone NOT NULL,
    end_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone NOT NULL,
    recording_id uuid,
    description text DEFAULT ''::text NOT NULL,
    series_episode integer,
    syobocal_tid integer,
    anime_count double precision,
    imported boolean DEFAULT false NOT NULL,
    series text,
    tmdb_series_id integer,
    tmdb_season_number integer,
    tmdb_episode_number integer
);

CREATE TABLE public.recording_live_stats (
    recording_id uuid NOT NULL,
    bytes bigint NOT NULL,
    drops bigint NOT NULL,
    errors bigint NOT NULL,
    scrambled bigint NOT NULL,
    bitrate bigint NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE TABLE public.recording_nicolog_files (
    recording_id uuid NOT NULL,
    path text,
    checked_at timestamp with time zone NOT NULL
);

CREATE TABLE public.recording_tags (
    recording_id uuid NOT NULL,
    tag_id uuid NOT NULL
);

CREATE TABLE public.recordings (
    id uuid NOT NULL,
    reservation_id uuid,
    service_id bigint NOT NULL,
    event_id bigint,
    name text NOT NULL,
    description text NOT NULL,
    start_at timestamp with time zone NOT NULL,
    end_at timestamp with time zone NOT NULL,
    state text NOT NULL,
    failure_reason text,
    protected boolean DEFAULT false NOT NULL,
    drops bigint DEFAULT 0 NOT NULL,
    errors bigint DEFAULT 0 NOT NULL,
    scrambled bigint DEFAULT 0 NOT NULL,
    recorded_from timestamp with time zone NOT NULL,
    recorded_until timestamp with time zone,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    gaps jsonb DEFAULT '[]'::jsonb NOT NULL,
    program_lost boolean DEFAULT false NOT NULL,
    audios jsonb DEFAULT '[]'::jsonb NOT NULL,
    broadcast_started_at timestamp with time zone,
    broadcast_ended_at timestamp with time zone,
    not_broadcast boolean DEFAULT false NOT NULL,
    aired_event_id bigint,
    genres jsonb DEFAULT '[]'::jsonb NOT NULL,
    rule_id uuid,
    rule_name text,
    api_id bigint NOT NULL,
    video_resolution text,
    waited_for_broadcast boolean DEFAULT false NOT NULL,
    name_normalized text,
    description_normalized text,
    failure_detail text,
    scrambled_ratio double precision,
    scramble_warning boolean DEFAULT false NOT NULL,
    extended jsonb DEFAULT '{}'::jsonb NOT NULL,
    CONSTRAINT recordings_failure_reason_check CHECK ((failure_reason = ANY (ARRAY['tuner_unavailable'::text, 'tuner_server_unreachable'::text, 'stream_interrupted'::text, 'storage_unavailable'::text, 'storage_full'::text, 'program_not_found'::text, 'cancelled'::text, 'recorder_unavailable'::text, 'tuner_preempted'::text, 'tuning_refused'::text]))),
    CONSTRAINT recordings_state_check CHECK ((state = ANY (ARRAY['recording'::text, 'completed'::text, 'completed_with_errors'::text, 'partial'::text, 'failed'::text])))
);

ALTER TABLE public.recordings ALTER COLUMN api_id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.recordings_api_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

CREATE TABLE public.reservation_rules (
    reservation_id uuid NOT NULL,
    rule_id uuid NOT NULL
);

CREATE TABLE public.reservations (
    id uuid NOT NULL,
    kind text NOT NULL,
    service_id bigint NOT NULL,
    event_id bigint,
    start_at timestamp with time zone NOT NULL,
    end_at timestamp with time zone NOT NULL,
    name text NOT NULL,
    description text NOT NULL,
    extended jsonb NOT NULL,
    genres jsonb NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    follow_event_relay boolean DEFAULT true NOT NULL,
    settings jsonb NOT NULL,
    created_at timestamp with time zone NOT NULL,
    created_by uuid,
    updated_at timestamp with time zone NOT NULL,
    updated_by uuid,
    assignment_state text DEFAULT 'pending'::text NOT NULL,
    assigned_tuner bigint,
    planned_start_at timestamp with time zone,
    planned_end_at timestamp with time zone,
    relayed_from uuid,
    assignment_peers uuid[] DEFAULT '{}'::uuid[] NOT NULL,
    disabled_reason text,
    disabled_service_id bigint,
    episode_recheck_waived boolean DEFAULT false NOT NULL,
    CONSTRAINT reservations_assignment_state_check CHECK ((assignment_state = ANY (ARRAY['pending'::text, 'ok'::text, 'partial'::text, 'conflict'::text]))),
    CONSTRAINT reservations_disabled_reason_check CHECK ((disabled_reason = 'recorded_episode'::text)),
    CONSTRAINT reservations_event_check CHECK (((kind = 'time'::text) = (event_id IS NULL))),
    CONSTRAINT reservations_kind_check CHECK ((kind = ANY (ARRAY['program'::text, 'time'::text, 'rule'::text]))),
    CONSTRAINT reservations_time_check CHECK ((start_at < end_at))
);

CREATE TABLE public.rule_groups (
    id uuid NOT NULL,
    name text NOT NULL,
    "position" integer NOT NULL,
    created_at timestamp with time zone NOT NULL,
    created_by uuid,
    updated_at timestamp with time zone NOT NULL,
    updated_by uuid
);

CREATE TABLE public.rules (
    id uuid NOT NULL,
    name text NOT NULL,
    memo text NOT NULL,
    enabled boolean NOT NULL,
    conditions jsonb NOT NULL,
    settings jsonb NOT NULL,
    valid_from timestamp with time zone,
    valid_until timestamp with time zone,
    avoid_duplicates boolean NOT NULL,
    duplicate_window_days integer,
    follow_event_relay boolean NOT NULL,
    created_at timestamp with time zone NOT NULL,
    created_by uuid,
    updated_at timestamp with time zone NOT NULL,
    updated_by uuid,
    bundle_same_episodes boolean DEFAULT false NOT NULL,
    skip_earlier_episodes boolean DEFAULT false NOT NULL,
    group_id uuid,
    CONSTRAINT rules_duplicate_window_check CHECK (((duplicate_window_days IS NULL) OR (duplicate_window_days > 0))),
    CONSTRAINT rules_valid_period_check CHECK (((valid_from IS NULL) OR (valid_until IS NULL) OR (valid_from < valid_until)))
);

CREATE TABLE public.schema_compatibility (
    id boolean DEFAULT true NOT NULL,
    min_binary_version bigint NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT schema_compatibility_id_check CHECK (id)
);

CREATE TABLE public.scramble_alerts (
    id boolean DEFAULT true NOT NULL,
    consecutive bigint NOT NULL,
    notified_at timestamp with time zone,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT scramble_alerts_id_check CHECK (id)
);

CREATE TABLE public.service_logos (
    network_id bigint NOT NULL,
    logo_id bigint NOT NULL,
    logo_type smallint NOT NULL,
    version bigint NOT NULL,
    data bytea NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE TABLE public.services (
    id bigint NOT NULL,
    network_id bigint NOT NULL,
    sid bigint NOT NULL,
    name text NOT NULL,
    name_normalized text NOT NULL,
    service_type bigint NOT NULL,
    channel_type text NOT NULL,
    channel text NOT NULL,
    remote_control_key_id bigint,
    logo_id bigint,
    hidden boolean DEFAULT false NOT NULL,
    sort_order integer,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    gone_at timestamp with time zone
);

CREATE TABLE public.sessions (
    id uuid NOT NULL,
    token_hash bytea NOT NULL,
    user_id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL,
    authenticated_at timestamp with time zone NOT NULL,
    last_seen_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    user_agent text NOT NULL,
    source_ip inet
);

CREATE TABLE public.setup_tokens (
    token_hash bytea NOT NULL,
    created_at timestamp with time zone NOT NULL
);

CREATE TABLE public.storage_pools (
    id text NOT NULL,
    priority integer NOT NULL,
    min_free_bytes bigint NOT NULL,
    low_since timestamp with time zone,
    updated_at timestamp with time zone NOT NULL,
    total_bytes bigint,
    free_bytes bigint,
    measured_at timestamp with time zone,
    CONSTRAINT storage_pools_min_free_bytes_check CHECK ((min_free_bytes >= 0))
);

CREATE TABLE public.stream_urls (
    id uuid NOT NULL,
    user_id uuid NOT NULL,
    resource text NOT NULL,
    target_name text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    last_used_at timestamp with time zone
);

CREATE TABLE public.syobocal_lookups (
    channel_id integer NOT NULL,
    day_start timestamp with time zone NOT NULL,
    fetched_at timestamp with time zone NOT NULL
);

CREATE TABLE public.syobocal_programs (
    pid integer NOT NULL,
    tid integer NOT NULL,
    channel_id integer NOT NULL,
    start_at timestamp with time zone NOT NULL,
    end_at timestamp with time zone NOT NULL,
    offset_seconds integer NOT NULL,
    count text NOT NULL,
    sub_title text NOT NULL,
    st_sub_title text NOT NULL,
    flag integer NOT NULL,
    deleted boolean NOT NULL,
    fetched_at timestamp with time zone NOT NULL
);

CREATE TABLE public.syobocal_titles (
    tid integer NOT NULL,
    title text NOT NULL,
    title_key text NOT NULL,
    last_update timestamp with time zone
);

CREATE TABLE public.system_settings (
    id smallint NOT NULL,
    recording_defaults jsonb NOT NULL,
    archive_retention_days integer NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    notification_retention_days integer DEFAULT 90 NOT NULL,
    rule_defaults jsonb DEFAULT '{}'::jsonb NOT NULL,
    mcp_enabled boolean DEFAULT false NOT NULL,
    drcs_replacements jsonb DEFAULT '{}'::jsonb NOT NULL,
    data_broadcast_ip_enabled boolean DEFAULT false NOT NULL,
    anime_linking_enabled boolean DEFAULT false NOT NULL,
    watch_progress_retention_days integer DEFAULT 365 NOT NULL,
    network_activity_enabled boolean DEFAULT false NOT NULL,
    network_activity_enabled_at timestamp with time zone,
    network_activity_retention_days integer DEFAULT 30 NOT NULL,
    comment_sources text[] DEFAULT '{}'::text[] NOT NULL,
    timeshift_while_watching boolean DEFAULT false NOT NULL,
    timeshift_while_watching_retention_seconds integer DEFAULT 3600 CONSTRAINT system_settings_timeshift_while_watching_retention_sec_not_null NOT NULL,
    rule_unmatched_grace_minutes integer DEFAULT 180 NOT NULL,
    external_player_urls_enabled boolean DEFAULT false NOT NULL,
    iptv_feeds_enabled boolean DEFAULT false NOT NULL,
    recording_downloads_enabled boolean DEFAULT false NOT NULL,
    epgstation_api_enabled boolean DEFAULT false NOT NULL,
    tvtplay_chapters_enabled boolean DEFAULT false NOT NULL,
    cm_detector text DEFAULT 'ffmpeg'::text NOT NULL,
    drama_linking_enabled boolean DEFAULT false NOT NULL,
    audit_retention_days integer DEFAULT 365 NOT NULL,
    audit_client_info_retention_days integer DEFAULT 90 NOT NULL,
    hibp_enabled boolean DEFAULT false NOT NULL,
    web_push_subject text DEFAULT ''::text NOT NULL,
    thumbnail_previews_enabled boolean DEFAULT true NOT NULL,
    sleep_control_enabled boolean DEFAULT true NOT NULL,
    caption_threads integer DEFAULT 0 NOT NULL,
    jikkyo_channels jsonb DEFAULT '{}'::jsonb NOT NULL,
    CONSTRAINT system_settings_archive_retention_days_check CHECK ((archive_retention_days >= 0)),
    CONSTRAINT system_settings_audit_client_info_retention_days_check CHECK (((audit_client_info_retention_days >= 1) AND (audit_client_info_retention_days <= 3650))),
    CONSTRAINT system_settings_audit_retention_days_check CHECK (((audit_retention_days >= 1) AND (audit_retention_days <= 3650))),
    CONSTRAINT system_settings_caption_threads_check CHECK (((caption_threads >= 0) AND (caption_threads <= 256))),
    CONSTRAINT system_settings_id_check CHECK ((id = 1)),
    CONSTRAINT system_settings_network_activity_retention_days_check CHECK ((network_activity_retention_days >= 1)),
    CONSTRAINT system_settings_notification_retention_days_check CHECK ((notification_retention_days >= 0)),
    CONSTRAINT system_settings_rule_unmatched_grace_minutes_check CHECK (((rule_unmatched_grace_minutes >= 0) AND (rule_unmatched_grace_minutes <= 1440))),
    CONSTRAINT system_settings_timeshift_while_watching_retention_second_check CHECK (((timeshift_while_watching_retention_seconds >= 600) AND (timeshift_while_watching_retention_seconds <= 86400))),
    CONSTRAINT system_settings_watch_progress_retention_days_check CHECK ((watch_progress_retention_days >= 0)),
    CONSTRAINT system_settings_web_push_subject_check CHECK (((web_push_subject = ''::text) OR (web_push_subject ~~ 'mailto:%'::text) OR (web_push_subject ~~ 'https://%'::text)))
);

CREATE TABLE public.tags (
    id uuid NOT NULL,
    name text NOT NULL,
    created_at timestamp with time zone NOT NULL
);

CREATE TABLE public.terrestrial_lineups (
    network_id integer NOT NULL,
    transport_stream_id integer NOT NULL,
    version smallint NOT NULL,
    services jsonb NOT NULL,
    observed_at timestamp with time zone NOT NULL
);

CREATE TABLE public.timeshift_assignments (
    id uuid NOT NULL,
    service_id bigint NOT NULL,
    priority integer NOT NULL,
    state text NOT NULL,
    generation bigint NOT NULL,
    holder uuid,
    lease_expires_at timestamp with time zone,
    stream_generation bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT timeshift_assignments_state_check CHECK ((state = ANY (ARRAY['waiting'::text, 'recording'::text, 'handover'::text, 'finished'::text])))
);

CREATE TABLE public.timeshift_channels (
    service_id bigint NOT NULL,
    retention_seconds integer NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    updated_by uuid,
    while_watching boolean DEFAULT false NOT NULL,
    CONSTRAINT timeshift_channels_retention_seconds_check CHECK (((retention_seconds >= 600) AND (retention_seconds <= 86400)))
);

CREATE TABLE public.timeshift_chunks (
    id uuid NOT NULL,
    service_id bigint NOT NULL,
    path text NOT NULL,
    size bigint NOT NULL,
    started_at timestamp with time zone NOT NULL,
    ended_at timestamp with time zone,
    created_at timestamp with time zone NOT NULL
);

CREATE TABLE public.timeshift_programs (
    service_id bigint NOT NULL,
    started_at timestamp with time zone NOT NULL,
    event_id bigint NOT NULL,
    name text NOT NULL,
    ended_at timestamp with time zone
);

CREATE TABLE public.timeshift_promotions (
    recording_id uuid NOT NULL,
    service_id bigint NOT NULL,
    from_at timestamp with time zone NOT NULL,
    to_at timestamp with time zone NOT NULL,
    created_by uuid,
    created_at timestamp with time zone NOT NULL
);

CREATE TABLE public.timeshift_watches (
    service_id bigint NOT NULL,
    watching_until timestamp with time zone NOT NULL
);

CREATE TABLE public.tmdb_searches (
    query text NOT NULL,
    results jsonb NOT NULL,
    fetched_at timestamp with time zone NOT NULL
);

CREATE TABLE public.tmdb_seasons (
    series_id integer NOT NULL,
    season_number integer NOT NULL,
    episodes jsonb NOT NULL,
    fetched_at timestamp with time zone NOT NULL,
    CONSTRAINT tmdb_seasons_season_number_check CHECK ((season_number >= 0))
);

CREATE TABLE public.tmdb_series (
    id integer NOT NULL,
    name text NOT NULL,
    original_name text NOT NULL,
    first_air_date date,
    seasons jsonb NOT NULL,
    fetched_at timestamp with time zone NOT NULL,
    CONSTRAINT tmdb_series_id_check CHECK ((id > 0))
);

CREATE TABLE public.tmdb_tokens (
    id smallint NOT NULL,
    token_encrypted bytea NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT tmdb_tokens_id_check CHECK ((id = 1))
);

CREATE TABLE public.totp_enrollments (
    user_id uuid NOT NULL,
    secret_encrypted bytea NOT NULL,
    created_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL
);

CREATE TABLE public.tuner_occupancies (
    tuner_index bigint NOT NULL,
    start_at timestamp with time zone NOT NULL,
    end_at timestamp with time zone NOT NULL
);

CREATE TABLE public.tuner_server_status (
    id boolean DEFAULT true NOT NULL,
    degraded_since timestamp with time zone,
    last_error text NOT NULL,
    clock_skew_ms bigint,
    checked_at timestamp with time zone NOT NULL,
    broadcast_clock_skew_ms bigint,
    broadcast_clock_checked_at timestamp with time zone,
    server_kind text,
    server_version text,
    CONSTRAINT tuner_server_status_id_check CHECK (id)
);

CREATE TABLE public.tuners (
    tuner_index bigint NOT NULL,
    name text NOT NULL,
    types text[] NOT NULL,
    is_available boolean NOT NULL,
    is_remote boolean NOT NULL,
    is_fault boolean NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE TABLE public.user_oidc_identities (
    id uuid NOT NULL,
    user_id uuid NOT NULL,
    issuer text NOT NULL,
    subject text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    last_used_at timestamp with time zone,
    picture text
);

CREATE TABLE public.user_passkeys (
    id uuid NOT NULL,
    user_id uuid NOT NULL,
    credential_id bytea NOT NULL,
    credential jsonb NOT NULL,
    rp_id text NOT NULL,
    name text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    last_used_at timestamp with time zone
);

CREATE TABLE public.user_recovery_codes (
    id uuid NOT NULL,
    user_id uuid NOT NULL,
    code_hash bytea NOT NULL,
    created_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone
);

CREATE TABLE public.user_setting_versions (
    user_id uuid NOT NULL,
    version bigint NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE TABLE public.user_settings (
    user_id uuid NOT NULL,
    captions_enabled boolean DEFAULT true NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    watch_reminder_minutes integer DEFAULT 5 NOT NULL,
    comment_settings jsonb,
    caption_settings jsonb,
    comment_sources text[] DEFAULT '{}'::text[] NOT NULL,
    CONSTRAINT user_settings_watch_reminder_minutes_check CHECK (((watch_reminder_minutes >= 0) AND (watch_reminder_minutes <= 60)))
);

CREATE TABLE public.user_totps (
    user_id uuid NOT NULL,
    secret_encrypted bytea NOT NULL,
    last_used_step bigint NOT NULL,
    created_at timestamp with time zone NOT NULL
);

CREATE TABLE public.users (
    id uuid NOT NULL,
    username text NOT NULL,
    display_name text NOT NULL,
    role text NOT NULL,
    password_hash text,
    disabled_at timestamp with time zone,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT users_role_check CHECK ((role = ANY (ARRAY['admin'::text, 'member'::text, 'viewer'::text])))
);

CREATE TABLE public.vapid_keys (
    id boolean DEFAULT true NOT NULL,
    public_key bytea NOT NULL,
    private_key_ciphertext bytea NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT vapid_keys_id_check CHECK (id)
);

CREATE TABLE public.watch_progress (
    user_id uuid NOT NULL,
    recording_id uuid NOT NULL,
    position_ms bigint NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT watch_progress_position_ms_check CHECK ((position_ms >= 0))
);

CREATE TABLE public.watch_reservations (
    user_id uuid NOT NULL,
    service_id bigint NOT NULL,
    event_id bigint NOT NULL,
    created_at timestamp with time zone NOT NULL,
    watch_rule_id uuid,
    from_wishlist boolean DEFAULT false NOT NULL
);

CREATE TABLE public.watch_rule_dismissals (
    user_id uuid NOT NULL,
    service_id bigint NOT NULL,
    event_id bigint NOT NULL,
    created_at timestamp with time zone NOT NULL
);

CREATE TABLE public.watch_rules (
    id uuid NOT NULL,
    user_id uuid NOT NULL,
    name text NOT NULL,
    enabled boolean NOT NULL,
    conditions jsonb NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE TABLE public.web_push_subscriptions (
    id uuid NOT NULL,
    user_id uuid NOT NULL,
    endpoint text NOT NULL,
    p256dh bytea NOT NULL,
    auth bytea NOT NULL,
    user_agent text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    session_id uuid
);

CREATE TABLE public.webauthn_ceremonies (
    id uuid NOT NULL,
    token_hash bytea NOT NULL,
    kind text NOT NULL,
    user_id uuid,
    session jsonb NOT NULL,
    created_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    CONSTRAINT webauthn_ceremonies_kind_check CHECK ((kind = ANY (ARRAY['registration'::text, 'login'::text, 'reauthentication'::text])))
);

CREATE TABLE public.webhooks (
    id uuid NOT NULL,
    name text NOT NULL,
    url text NOT NULL,
    format text NOT NULL,
    kinds text[] NOT NULL,
    secret_ciphertext bytea NOT NULL,
    enabled boolean NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT webhooks_format_check CHECK ((format = ANY (ARRAY['json'::text, 'discord'::text, 'slack'::text])))
);

ALTER TABLE ONLY public.anime_artworks
    ADD CONSTRAINT anime_artworks_pkey PRIMARY KEY (mal_id);

ALTER TABLE ONLY public.anime_channel_definitions
    ADD CONSTRAINT anime_channel_definitions_pkey PRIMARY KEY (channel_type, network_id, sid);

ALTER TABLE ONLY public.anime_channel_overrides
    ADD CONSTRAINT anime_channel_overrides_pkey PRIMARY KEY (service_id);

ALTER TABLE ONLY public.anime_episode_broadcasts
    ADD CONSTRAINT anime_episode_broadcasts_pkey PRIMARY KEY (pid);

ALTER TABLE ONLY public.anime_following_fetches
    ADD CONSTRAINT anime_following_fetches_pkey PRIMARY KEY (user_id, destination, syobocal_tid);

ALTER TABLE ONLY public.anime_following_viewings
    ADD CONSTRAINT anime_following_viewings_pkey PRIMARY KEY (user_id, destination, syobocal_tid, username);

ALTER TABLE ONLY public.anime_import_tokens
    ADD CONSTRAINT anime_import_tokens_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.anime_imports
    ADD CONSTRAINT anime_imports_pkey PRIMARY KEY (source);

ALTER TABLE ONLY public.anime_match_cursor
    ADD CONSTRAINT anime_match_cursor_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.anime_record_apps
    ADD CONSTRAINT anime_record_apps_pkey PRIMARY KEY (destination);

ALTER TABLE ONLY public.anime_record_authorizations
    ADD CONSTRAINT anime_record_authorizations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.anime_record_authorizations
    ADD CONSTRAINT anime_record_authorizations_state_hash_key UNIQUE (state_hash);

ALTER TABLE ONLY public.anime_record_connections
    ADD CONSTRAINT anime_record_connections_pkey PRIMARY KEY (user_id, destination);

ALTER TABLE ONLY public.anime_record_settings
    ADD CONSTRAINT anime_record_settings_pkey PRIMARY KEY (user_id);

ALTER TABLE ONLY public.anime_records
    ADD CONSTRAINT anime_records_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.anime_records
    ADD CONSTRAINT anime_records_user_id_service_id_start_at_destination_syobo_key UNIQUE NULLS NOT DISTINCT (user_id, service_id, start_at, destination, syobocal_tid, episode_number);

ALTER TABLE ONLY public.anime_title_broadcast_lookups
    ADD CONSTRAINT anime_title_broadcast_lookups_pkey PRIMARY KEY (tid);

ALTER TABLE ONLY public.anime_watch_progress
    ADD CONSTRAINT anime_watch_progress_pkey PRIMARY KEY (user_id, service_id, start_at);

ALTER TABLE ONLY public.anime_wishlist_entries
    ADD CONSTRAINT anime_wishlist_entries_pkey PRIMARY KEY (user_id, destination, work_id);

ALTER TABLE ONLY public.anime_works
    ADD CONSTRAINT anime_works_pkey PRIMARY KEY (source, syobocal_tid, "position");

ALTER TABLE ONLY public.annict_work_sync
    ADD CONSTRAINT annict_work_sync_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.annict_works
    ADD CONSTRAINT annict_works_pkey PRIMARY KEY (annict_id);

ALTER TABLE ONLY public.api_tokens
    ADD CONSTRAINT api_tokens_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.api_tokens
    ADD CONSTRAINT api_tokens_token_hash_key UNIQUE (token_hash);

ALTER TABLE ONLY public.audit_events
    ADD CONSTRAINT audit_events_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.backup_runs
    ADD CONSTRAINT backup_runs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.backup_settings
    ADD CONSTRAINT backup_settings_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.broadcast_lineups
    ADD CONSTRAINT broadcast_lineups_pkey PRIMARY KEY (network_id);

ALTER TABLE ONLY public.calendar_feeds
    ADD CONSTRAINT calendar_feeds_pkey PRIMARY KEY (user_id, kind);

ALTER TABLE ONLY public.calendar_feeds
    ADD CONSTRAINT calendar_feeds_token_hash_key UNIQUE (token_hash);

ALTER TABLE ONLY public.caption_models
    ADD CONSTRAINT caption_models_pkey PRIMARY KEY (kind);

ALTER TABLE ONLY public.captures
    ADD CONSTRAINT captures_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.change_versions
    ADD CONSTRAINT change_versions_pkey PRIMARY KEY (channel);

ALTER TABLE ONLY public.channel_types
    ADD CONSTRAINT channel_types_pkey PRIMARY KEY (type);

ALTER TABLE ONLY public.drama_match_cursor
    ADD CONSTRAINT drama_match_cursor_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.drama_record_apps
    ADD CONSTRAINT drama_record_apps_pkey PRIMARY KEY (destination);

ALTER TABLE ONLY public.drama_record_authorizations
    ADD CONSTRAINT drama_record_authorizations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.drama_record_authorizations
    ADD CONSTRAINT drama_record_authorizations_state_hash_key UNIQUE (state_hash);

ALTER TABLE ONLY public.drama_record_connections
    ADD CONSTRAINT drama_record_connections_pkey PRIMARY KEY (user_id, destination);

ALTER TABLE ONLY public.drama_record_settings
    ADD CONSTRAINT drama_record_settings_pkey PRIMARY KEY (user_id);

ALTER TABLE ONLY public.drama_records
    ADD CONSTRAINT drama_records_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.drama_records
    ADD CONSTRAINT drama_records_user_id_service_id_start_at_destination_tmdb__key UNIQUE (user_id, service_id, start_at, destination, tmdb_series_id, season_number, episode_number);

ALTER TABLE ONLY public.drama_title_overrides
    ADD CONSTRAINT drama_title_overrides_pkey PRIMARY KEY (title_key);

ALTER TABLE ONLY public.drama_watched_episodes
    ADD CONSTRAINT drama_watched_episodes_pkey PRIMARY KEY (user_id, tmdb_series_id, season_number, episode_number);

ALTER TABLE ONLY public.drama_watchlist_entries
    ADD CONSTRAINT drama_watchlist_entries_pkey PRIMARY KEY (user_id, tmdb_series_id);

ALTER TABLE ONLY public.drama_watchlist_imports
    ADD CONSTRAINT drama_watchlist_imports_pkey PRIMARY KEY (user_id, destination, tmdb_series_id);

ALTER TABLE ONLY public.edcb_network_tv_holders
    ADD CONSTRAINT edcb_network_tv_holders_pkey PRIMARY KEY (network_tv_id, instance_id);

ALTER TABLE ONLY public.edcb_network_tvs
    ADD CONSTRAINT edcb_network_tvs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.google_apps
    ADD CONSTRAINT google_apps_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.google_authorizations
    ADD CONSTRAINT google_authorizations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.google_authorizations
    ADD CONSTRAINT google_authorizations_state_hash_key UNIQUE (state_hash);

ALTER TABLE ONLY public.google_drive_connections
    ADD CONSTRAINT google_drive_connections_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.google_drive_folders
    ADD CONSTRAINT google_drive_folders_library_name_key UNIQUE (library_name);

ALTER TABLE ONLY public.google_drive_folders
    ADD CONSTRAINT google_drive_folders_pkey PRIMARY KEY (folder_id);

ALTER TABLE ONLY public.goose_db_version
    ADD CONSTRAINT goose_db_version_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.guide_views
    ADD CONSTRAINT guide_views_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.import_sources
    ADD CONSTRAINT import_sources_pkey PRIMARY KEY (source, kind, source_id);

ALTER TABLE ONLY public.instances
    ADD CONSTRAINT instances_pkey PRIMARY KEY (id, role);

ALTER TABLE ONLY public.job_progress
    ADD CONSTRAINT job_progress_pkey PRIMARY KEY (job_id);

ALTER TABLE ONLY public.leases
    ADD CONSTRAINT leases_pkey PRIMARY KEY (name);

ALTER TABLE ONLY public.library_watch_progress
    ADD CONSTRAINT library_watch_progress_pkey PRIMARY KEY (user_id, file_id);

ALTER TABLE ONLY public.live_scramble_verdicts
    ADD CONSTRAINT live_scramble_verdicts_pkey PRIMARY KEY (service_id);

ALTER TABLE ONLY public.login_challenges
    ADD CONSTRAINT login_challenges_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.login_challenges
    ADD CONSTRAINT login_challenges_token_hash_key UNIQUE (token_hash);

ALTER TABLE ONLY public.login_throttles
    ADD CONSTRAINT login_throttles_pkey PRIMARY KEY (key);

ALTER TABLE ONLY public.network_activity
    ADD CONSTRAINT network_activity_pkey PRIMARY KEY (hour, purpose, host, initiator, success);

ALTER TABLE ONLY public.notification_events
    ADD CONSTRAINT notification_events_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.notification_preferences
    ADD CONSTRAINT notification_preferences_pkey PRIMARY KEY (user_id, kind);

ALTER TABLE ONLY public.notifications
    ADD CONSTRAINT notifications_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.notifications
    ADD CONSTRAINT notifications_user_id_event_id_key UNIQUE (user_id, event_id);

ALTER TABLE ONLY public.oauth_authorization_codes
    ADD CONSTRAINT oauth_authorization_codes_pkey PRIMARY KEY (code_hash);

ALTER TABLE ONLY public.oauth_grants
    ADD CONSTRAINT oauth_grants_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.oauth_tokens
    ADD CONSTRAINT oauth_tokens_pkey PRIMARY KEY (token_hash);

ALTER TABLE ONLY public.oidc_logins
    ADD CONSTRAINT oidc_logins_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.oidc_logins
    ADD CONSTRAINT oidc_logins_token_hash_key UNIQUE (token_hash);

ALTER TABLE ONLY public.pinned_services
    ADD CONSTRAINT pinned_services_pkey PRIMARY KEY (user_id, service_id);

ALTER TABLE ONLY public.player_commands
    ADD CONSTRAINT player_commands_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.player_versions
    ADD CONSTRAINT player_versions_pkey PRIMARY KEY (user_id);

ALTER TABLE ONLY public.players
    ADD CONSTRAINT players_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.program_anime_episodes
    ADD CONSTRAINT program_anime_episodes_pkey PRIMARY KEY (service_id, start_at, "position");

ALTER TABLE ONLY public.program_anime_matches
    ADD CONSTRAINT program_anime_matches_pkey PRIMARY KEY (service_id, start_at);

ALTER TABLE ONLY public.program_archives
    ADD CONSTRAINT program_archives_pkey PRIMARY KEY (service_id, event_id, start_at);

ALTER TABLE ONLY public.program_drama_matches
    ADD CONSTRAINT program_drama_matches_pkey PRIMARY KEY (service_id, start_at);

ALTER TABLE ONLY public.programs
    ADD CONSTRAINT programs_pkey PRIMARY KEY (service_id, event_id);

ALTER TABLE ONLY public.recording_anime_episodes
    ADD CONSTRAINT recording_anime_episodes_pkey PRIMARY KEY (recording_id, "position");

ALTER TABLE ONLY public.recording_anime_matches
    ADD CONSTRAINT recording_anime_matches_pkey PRIMARY KEY (recording_id);

ALTER TABLE ONLY public.recording_assignments
    ADD CONSTRAINT recording_assignments_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.recording_assignments
    ADD CONSTRAINT recording_assignments_reservation_id_key UNIQUE (reservation_id);

ALTER TABLE ONLY public.recording_auto_captions
    ADD CONSTRAINT recording_auto_captions_pkey PRIMARY KEY (recording_id);

ALTER TABLE ONLY public.recording_cm_detections
    ADD CONSTRAINT recording_cm_detections_pkey PRIMARY KEY (recording_id, detector);

ALTER TABLE ONLY public.recording_cm_materials
    ADD CONSTRAINT recording_cm_materials_pkey PRIMARY KEY (recording_id, detector);

ALTER TABLE ONLY public.recording_cm_sections
    ADD CONSTRAINT recording_cm_sections_pkey PRIMARY KEY (recording_id, detector, start_offset_ms);

ALTER TABLE ONLY public.recording_drama_matches
    ADD CONSTRAINT recording_drama_matches_pkey PRIMARY KEY (recording_id);

ALTER TABLE ONLY public.recording_failure_counts
    ADD CONSTRAINT recording_failure_counts_pkey PRIMARY KEY (reason);

ALTER TABLE ONLY public.recording_files
    ADD CONSTRAINT recording_files_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.recording_files
    ADD CONSTRAINT recording_files_pool_id_path_key UNIQUE (pool_id, path);

ALTER TABLE ONLY public.recording_history
    ADD CONSTRAINT recording_history_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.recording_live_stats
    ADD CONSTRAINT recording_live_stats_pkey PRIMARY KEY (recording_id);

ALTER TABLE ONLY public.recording_nicolog_files
    ADD CONSTRAINT recording_nicolog_files_pkey PRIMARY KEY (recording_id);

ALTER TABLE ONLY public.recording_tags
    ADD CONSTRAINT recording_tags_pkey PRIMARY KEY (recording_id, tag_id);

ALTER TABLE ONLY public.recordings
    ADD CONSTRAINT recordings_api_id_key UNIQUE (api_id);

ALTER TABLE ONLY public.recordings
    ADD CONSTRAINT recordings_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.recordings
    ADD CONSTRAINT recordings_reservation_id_key UNIQUE (reservation_id);

ALTER TABLE ONLY public.reservation_rules
    ADD CONSTRAINT reservation_rules_pkey PRIMARY KEY (reservation_id, rule_id);

ALTER TABLE ONLY public.reservations
    ADD CONSTRAINT reservations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.rule_groups
    ADD CONSTRAINT rule_groups_name_key UNIQUE (name);

ALTER TABLE ONLY public.rule_groups
    ADD CONSTRAINT rule_groups_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.rules
    ADD CONSTRAINT rules_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.schema_compatibility
    ADD CONSTRAINT schema_compatibility_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.scramble_alerts
    ADD CONSTRAINT scramble_alerts_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.service_logos
    ADD CONSTRAINT service_logos_pkey PRIMARY KEY (network_id, logo_id, logo_type);

ALTER TABLE ONLY public.services
    ADD CONSTRAINT services_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT sessions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT sessions_token_hash_key UNIQUE (token_hash);

ALTER TABLE ONLY public.setup_tokens
    ADD CONSTRAINT setup_tokens_pkey PRIMARY KEY (token_hash);

ALTER TABLE ONLY public.storage_pools
    ADD CONSTRAINT storage_pools_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.stream_urls
    ADD CONSTRAINT stream_urls_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.syobocal_lookups
    ADD CONSTRAINT syobocal_lookups_pkey PRIMARY KEY (channel_id, day_start);

ALTER TABLE ONLY public.syobocal_programs
    ADD CONSTRAINT syobocal_programs_pkey PRIMARY KEY (pid);

ALTER TABLE ONLY public.syobocal_titles
    ADD CONSTRAINT syobocal_titles_pkey PRIMARY KEY (tid);

ALTER TABLE ONLY public.system_settings
    ADD CONSTRAINT system_settings_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.tags
    ADD CONSTRAINT tags_name_key UNIQUE (name);

ALTER TABLE ONLY public.tags
    ADD CONSTRAINT tags_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.terrestrial_lineups
    ADD CONSTRAINT terrestrial_lineups_pkey PRIMARY KEY (network_id);

ALTER TABLE ONLY public.timeshift_assignments
    ADD CONSTRAINT timeshift_assignments_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.timeshift_assignments
    ADD CONSTRAINT timeshift_assignments_service_id_key UNIQUE (service_id);

ALTER TABLE ONLY public.timeshift_channels
    ADD CONSTRAINT timeshift_channels_pkey PRIMARY KEY (service_id);

ALTER TABLE ONLY public.timeshift_chunks
    ADD CONSTRAINT timeshift_chunks_path_key UNIQUE (path);

ALTER TABLE ONLY public.timeshift_chunks
    ADD CONSTRAINT timeshift_chunks_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.timeshift_programs
    ADD CONSTRAINT timeshift_programs_pkey PRIMARY KEY (service_id, started_at);

ALTER TABLE ONLY public.timeshift_promotions
    ADD CONSTRAINT timeshift_promotions_pkey PRIMARY KEY (recording_id);

ALTER TABLE ONLY public.timeshift_promotions
    ADD CONSTRAINT timeshift_promotions_service_id_from_at_key UNIQUE (service_id, from_at);

ALTER TABLE ONLY public.timeshift_watches
    ADD CONSTRAINT timeshift_watches_pkey PRIMARY KEY (service_id);

ALTER TABLE ONLY public.tmdb_searches
    ADD CONSTRAINT tmdb_searches_pkey PRIMARY KEY (query);

ALTER TABLE ONLY public.tmdb_seasons
    ADD CONSTRAINT tmdb_seasons_pkey PRIMARY KEY (series_id, season_number);

ALTER TABLE ONLY public.tmdb_series
    ADD CONSTRAINT tmdb_series_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.tmdb_tokens
    ADD CONSTRAINT tmdb_tokens_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.totp_enrollments
    ADD CONSTRAINT totp_enrollments_pkey PRIMARY KEY (user_id);

ALTER TABLE ONLY public.tuner_occupancies
    ADD CONSTRAINT tuner_occupancies_pkey PRIMARY KEY (tuner_index, start_at, end_at);

ALTER TABLE ONLY public.tuner_server_status
    ADD CONSTRAINT tuner_server_status_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.tuners
    ADD CONSTRAINT tuners_pkey PRIMARY KEY (tuner_index);

ALTER TABLE ONLY public.user_oidc_identities
    ADD CONSTRAINT user_oidc_identities_issuer_subject_key UNIQUE (issuer, subject);

ALTER TABLE ONLY public.user_oidc_identities
    ADD CONSTRAINT user_oidc_identities_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.user_passkeys
    ADD CONSTRAINT user_passkeys_credential_id_key UNIQUE (credential_id);

ALTER TABLE ONLY public.user_passkeys
    ADD CONSTRAINT user_passkeys_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.user_recovery_codes
    ADD CONSTRAINT user_recovery_codes_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.user_recovery_codes
    ADD CONSTRAINT user_recovery_codes_user_id_code_hash_key UNIQUE (user_id, code_hash);

ALTER TABLE ONLY public.user_setting_versions
    ADD CONSTRAINT user_setting_versions_pkey PRIMARY KEY (user_id);

ALTER TABLE ONLY public.user_settings
    ADD CONSTRAINT user_settings_pkey PRIMARY KEY (user_id);

ALTER TABLE ONLY public.user_totps
    ADD CONSTRAINT user_totps_pkey PRIMARY KEY (user_id);

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.vapid_keys
    ADD CONSTRAINT vapid_keys_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.watch_progress
    ADD CONSTRAINT watch_progress_pkey PRIMARY KEY (user_id, recording_id);

ALTER TABLE ONLY public.watch_reservations
    ADD CONSTRAINT watch_reservations_pkey PRIMARY KEY (user_id, service_id, event_id);

ALTER TABLE ONLY public.watch_rule_dismissals
    ADD CONSTRAINT watch_rule_dismissals_pkey PRIMARY KEY (user_id, service_id, event_id);

ALTER TABLE ONLY public.watch_rules
    ADD CONSTRAINT watch_rules_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.web_push_subscriptions
    ADD CONSTRAINT web_push_subscriptions_endpoint_key UNIQUE (endpoint);

ALTER TABLE ONLY public.web_push_subscriptions
    ADD CONSTRAINT web_push_subscriptions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.webauthn_ceremonies
    ADD CONSTRAINT webauthn_ceremonies_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.webauthn_ceremonies
    ADD CONSTRAINT webauthn_ceremonies_token_hash_key UNIQUE (token_hash);

ALTER TABLE ONLY public.webhooks
    ADD CONSTRAINT webhooks_pkey PRIMARY KEY (id);

CREATE INDEX anime_episode_broadcasts_tid_episode_count_idx ON public.anime_episode_broadcasts USING btree (tid, episode_count);

CREATE INDEX anime_record_authorizations_expires_at_idx ON public.anime_record_authorizations USING btree (expires_at);

CREATE INDEX anime_watch_progress_updated_at_idx ON public.anime_watch_progress USING btree (updated_at);

CREATE INDEX anime_works_anilist_id_idx ON public.anime_works USING btree (anilist_id) WHERE (anilist_id IS NOT NULL);

CREATE INDEX anime_works_annict_work_id_idx ON public.anime_works USING btree (annict_work_id) WHERE (annict_work_id IS NOT NULL);

CREATE INDEX anime_works_syobocal_tid_idx ON public.anime_works USING btree (syobocal_tid);

CREATE INDEX annict_works_anilist_checked_at_idx ON public.annict_works USING btree (anilist_checked_at NULLS FIRST) WHERE (mal_anime_id IS NOT NULL);

CREATE INDEX api_tokens_user_id_idx ON public.api_tokens USING btree (user_id);

CREATE INDEX audit_events_occurred_at_idx ON public.audit_events USING btree (occurred_at);

CREATE INDEX backup_runs_started_at_idx ON public.backup_runs USING btree (started_at DESC);

CREATE INDEX captures_recording_id_idx ON public.captures USING btree (recording_id);

CREATE INDEX captures_user_id_created_at_idx ON public.captures USING btree (user_id, created_at DESC, id DESC);

CREATE INDEX drama_record_authorizations_expires_at_idx ON public.drama_record_authorizations USING btree (expires_at);

CREATE INDEX drama_watched_episodes_tmdb_series_id_idx ON public.drama_watched_episodes USING btree (tmdb_series_id, season_number);

CREATE INDEX drama_watchlist_entries_tmdb_series_id_idx ON public.drama_watchlist_entries USING btree (tmdb_series_id);

CREATE INDEX drama_watchlist_imports_pending_idx ON public.drama_watchlist_imports USING btree (user_id, destination) WHERE (NOT applied);

CREATE INDEX epg_changes_created_at_idx ON public.epg_changes USING btree (created_at);

CREATE INDEX epg_changes_version_idx ON public.epg_changes USING btree (version);

CREATE INDEX google_authorizations_expires_at_idx ON public.google_authorizations USING btree (expires_at);

CREATE UNIQUE INDEX guide_views_user_id_name_idx ON public.guide_views USING btree (user_id, name);

CREATE INDEX instances_role_heartbeat_at_idx ON public.instances USING btree (role, heartbeat_at);

CREATE INDEX library_watch_progress_updated_at_idx ON public.library_watch_progress USING btree (updated_at);

CREATE INDEX login_challenges_user_id_idx ON public.login_challenges USING btree (user_id);

CREATE INDEX notification_events_dispatched_idx ON public.notification_events USING btree (occurred_at) WHERE (dispatched_at IS NOT NULL);

CREATE INDEX notification_events_pending_idx ON public.notification_events USING btree (occurred_at) WHERE (dispatched_at IS NULL);

CREATE INDEX notifications_event_id_idx ON public.notifications USING btree (event_id);

CREATE INDEX notifications_user_id_created_at_idx ON public.notifications USING btree (user_id, created_at DESC, id DESC);

CREATE INDEX oauth_grants_user_id_idx ON public.oauth_grants USING btree (user_id);

CREATE INDEX oauth_tokens_expires_at_idx ON public.oauth_tokens USING btree (expires_at);

CREATE INDEX oauth_tokens_grant_id_idx ON public.oauth_tokens USING btree (grant_id);

CREATE INDEX oidc_logins_expires_at_idx ON public.oidc_logins USING btree (expires_at);

CREATE INDEX pinned_services_service_id_idx ON public.pinned_services USING btree (service_id);

CREATE INDEX player_commands_player_id_idx ON public.player_commands USING btree (player_id, id);

CREATE INDEX players_user_id_idx ON public.players USING btree (user_id, last_seen_at);

CREATE INDEX program_anime_episodes_syobocal_tid_idx ON public.program_anime_episodes USING btree (syobocal_tid) WHERE (syobocal_tid IS NOT NULL);

CREATE INDEX program_anime_matches_service_id_event_id_idx ON public.program_anime_matches USING btree (service_id, event_id);

CREATE INDEX program_archives_end_at_idx ON public.program_archives USING btree (end_at);

CREATE INDEX program_archives_service_id_start_at_idx ON public.program_archives USING btree (service_id, start_at);

CREATE INDEX program_archives_start_at_idx ON public.program_archives USING btree (start_at);

CREATE INDEX program_drama_matches_service_id_event_id_idx ON public.program_drama_matches USING btree (service_id, event_id);

CREATE INDEX program_drama_matches_title_key_idx ON public.program_drama_matches USING btree (title_key);

CREATE INDEX program_drama_matches_tmdb_series_id_idx ON public.program_drama_matches USING btree (tmdb_series_id) WHERE (tmdb_series_id IS NOT NULL);

CREATE INDEX program_rewrites_observed_at_idx ON public.program_rewrites USING btree (observed_at);

CREATE INDEX programs_end_at_idx ON public.programs USING btree (end_at);

CREATE INDEX programs_series_start_at_idx ON public.programs USING btree (series, start_at) WHERE (episode_number IS NOT NULL);

CREATE INDEX programs_service_id_start_at_idx ON public.programs USING btree (service_id, start_at);

CREATE INDEX programs_start_at_idx ON public.programs USING btree (start_at);

CREATE INDEX recording_anime_episodes_syobocal_tid_idx ON public.recording_anime_episodes USING btree (syobocal_tid) WHERE (syobocal_tid IS NOT NULL);

CREATE INDEX recording_drama_matches_tmdb_series_id_idx ON public.recording_drama_matches USING btree (tmdb_series_id) WHERE (tmdb_series_id IS NOT NULL);

CREATE INDEX recording_files_recording_id_idx ON public.recording_files USING btree (recording_id, started_at);

CREATE INDEX recording_history_recording_id_idx ON public.recording_history USING btree (recording_id);

CREATE INDEX recording_history_series_idx ON public.recording_history USING btree (series);

CREATE INDEX recording_history_title_key_service_id_end_at_idx ON public.recording_history USING btree (title_key, service_id, end_at);

CREATE INDEX recording_history_tmdb_series_id_idx ON public.recording_history USING btree (tmdb_series_id) WHERE (tmdb_series_id IS NOT NULL);

CREATE INDEX recording_tags_tag_id_idx ON public.recording_tags USING btree (tag_id, recording_id);

CREATE INDEX recordings_recorded_from_id_idx ON public.recordings USING btree (recorded_from DESC, id DESC);

CREATE INDEX recordings_rule_id_idx ON public.recordings USING btree (rule_id) WHERE (rule_id IS NOT NULL);

CREATE INDEX recordings_service_id_event_id_idx ON public.recordings USING btree (service_id, event_id);

CREATE INDEX reservation_rules_rule_id_idx ON public.reservation_rules USING btree (rule_id);

CREATE UNIQUE INDEX reservations_program_idx ON public.reservations USING btree (service_id, event_id) WHERE (event_id IS NOT NULL);

CREATE INDEX reservations_relayed_from_idx ON public.reservations USING btree (relayed_from) WHERE (relayed_from IS NOT NULL);

CREATE INDEX reservations_start_at_idx ON public.reservations USING btree (start_at);

CREATE INDEX rules_group_id_idx ON public.rules USING btree (group_id);

CREATE INDEX sessions_user_id_idx ON public.sessions USING btree (user_id);

CREATE INDEX stream_urls_expires_at_idx ON public.stream_urls USING btree (expires_at);

CREATE INDEX syobocal_programs_channel_id_start_at_idx ON public.syobocal_programs USING btree (channel_id, start_at);

CREATE INDEX syobocal_programs_end_at_idx ON public.syobocal_programs USING btree (end_at);

CREATE INDEX syobocal_titles_title_key_idx ON public.syobocal_titles USING btree (title_key);

CREATE INDEX timeshift_chunks_service_started_at_idx ON public.timeshift_chunks USING btree (service_id, started_at);

CREATE INDEX user_oidc_identities_user_id_idx ON public.user_oidc_identities USING btree (user_id);

CREATE INDEX user_passkeys_user_id_idx ON public.user_passkeys USING btree (user_id);

CREATE UNIQUE INDEX users_lower_username_idx ON public.users USING btree (lower(username));

CREATE INDEX watch_progress_recording_id_idx ON public.watch_progress USING btree (recording_id);

CREATE INDEX watch_progress_updated_at_idx ON public.watch_progress USING btree (updated_at);

CREATE INDEX watch_reservations_service_id_event_id_idx ON public.watch_reservations USING btree (service_id, event_id);

CREATE INDEX watch_reservations_watch_rule_id_idx ON public.watch_reservations USING btree (watch_rule_id);

CREATE INDEX watch_rules_user_id_created_at_idx ON public.watch_rules USING btree (user_id, created_at, id);

CREATE INDEX web_push_subscriptions_session_id_idx ON public.web_push_subscriptions USING btree (session_id);

CREATE INDEX web_push_subscriptions_user_id_idx ON public.web_push_subscriptions USING btree (user_id);

CREATE INDEX webauthn_ceremonies_expires_at_idx ON public.webauthn_ceremonies USING btree (expires_at);

ALTER TABLE ONLY public.anime_channel_overrides
    ADD CONSTRAINT anime_channel_overrides_service_id_fkey FOREIGN KEY (service_id) REFERENCES public.services(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.anime_following_fetches
    ADD CONSTRAINT anime_following_fetches_user_id_destination_fkey FOREIGN KEY (user_id, destination) REFERENCES public.anime_record_connections(user_id, destination) ON DELETE CASCADE;

ALTER TABLE ONLY public.anime_following_viewings
    ADD CONSTRAINT anime_following_viewings_user_id_destination_syobocal_tid_fkey FOREIGN KEY (user_id, destination, syobocal_tid) REFERENCES public.anime_following_fetches(user_id, destination, syobocal_tid) ON DELETE CASCADE;

ALTER TABLE ONLY public.anime_record_authorizations
    ADD CONSTRAINT anime_record_authorizations_session_id_fkey FOREIGN KEY (session_id) REFERENCES public.sessions(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.anime_record_authorizations
    ADD CONSTRAINT anime_record_authorizations_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.anime_record_connections
    ADD CONSTRAINT anime_record_connections_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.anime_record_settings
    ADD CONSTRAINT anime_record_settings_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.anime_records
    ADD CONSTRAINT anime_records_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.anime_watch_progress
    ADD CONSTRAINT anime_watch_progress_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.anime_wishlist_entries
    ADD CONSTRAINT anime_wishlist_entries_user_id_destination_fkey FOREIGN KEY (user_id, destination) REFERENCES public.anime_record_connections(user_id, destination) ON DELETE CASCADE;

ALTER TABLE ONLY public.api_tokens
    ADD CONSTRAINT api_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.calendar_feeds
    ADD CONSTRAINT calendar_feeds_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.captures
    ADD CONSTRAINT captures_recording_id_fkey FOREIGN KEY (recording_id) REFERENCES public.recordings(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.captures
    ADD CONSTRAINT captures_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.drama_record_authorizations
    ADD CONSTRAINT drama_record_authorizations_session_id_fkey FOREIGN KEY (session_id) REFERENCES public.sessions(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.drama_record_authorizations
    ADD CONSTRAINT drama_record_authorizations_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.drama_record_connections
    ADD CONSTRAINT drama_record_connections_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.drama_record_settings
    ADD CONSTRAINT drama_record_settings_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.drama_records
    ADD CONSTRAINT drama_records_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.drama_watched_episodes
    ADD CONSTRAINT drama_watched_episodes_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.drama_watchlist_entries
    ADD CONSTRAINT drama_watchlist_entries_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.drama_watchlist_imports
    ADD CONSTRAINT drama_watchlist_imports_user_id_destination_fkey FOREIGN KEY (user_id, destination) REFERENCES public.drama_record_connections(user_id, destination) ON DELETE CASCADE;

ALTER TABLE ONLY public.edcb_network_tv_holders
    ADD CONSTRAINT edcb_network_tv_holders_network_tv_id_fkey FOREIGN KEY (network_tv_id) REFERENCES public.edcb_network_tvs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.google_authorizations
    ADD CONSTRAINT google_authorizations_session_id_fkey FOREIGN KEY (session_id) REFERENCES public.sessions(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.google_authorizations
    ADD CONSTRAINT google_authorizations_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.guide_views
    ADD CONSTRAINT guide_views_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.library_watch_progress
    ADD CONSTRAINT library_watch_progress_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.login_challenges
    ADD CONSTRAINT login_challenges_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.notification_preferences
    ADD CONSTRAINT notification_preferences_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.notifications
    ADD CONSTRAINT notifications_event_id_fkey FOREIGN KEY (event_id) REFERENCES public.notification_events(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.notifications
    ADD CONSTRAINT notifications_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.oauth_authorization_codes
    ADD CONSTRAINT oauth_authorization_codes_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.oauth_grants
    ADD CONSTRAINT oauth_grants_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.oauth_tokens
    ADD CONSTRAINT oauth_tokens_grant_id_fkey FOREIGN KEY (grant_id) REFERENCES public.oauth_grants(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.oidc_logins
    ADD CONSTRAINT oidc_logins_session_id_fkey FOREIGN KEY (session_id) REFERENCES public.sessions(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.pinned_services
    ADD CONSTRAINT pinned_services_service_id_fkey FOREIGN KEY (service_id) REFERENCES public.services(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.pinned_services
    ADD CONSTRAINT pinned_services_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.player_commands
    ADD CONSTRAINT player_commands_player_id_fkey FOREIGN KEY (player_id) REFERENCES public.players(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.player_versions
    ADD CONSTRAINT player_versions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.players
    ADD CONSTRAINT players_session_id_fkey FOREIGN KEY (session_id) REFERENCES public.sessions(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.players
    ADD CONSTRAINT players_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.program_anime_episodes
    ADD CONSTRAINT program_anime_episodes_service_id_start_at_fkey FOREIGN KEY (service_id, start_at) REFERENCES public.program_anime_matches(service_id, start_at) ON DELETE CASCADE;

ALTER TABLE ONLY public.program_anime_matches
    ADD CONSTRAINT program_anime_matches_service_id_fkey FOREIGN KEY (service_id) REFERENCES public.services(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.program_archives
    ADD CONSTRAINT program_archives_service_id_fkey FOREIGN KEY (service_id) REFERENCES public.services(id);

ALTER TABLE ONLY public.program_drama_matches
    ADD CONSTRAINT program_drama_matches_service_id_fkey FOREIGN KEY (service_id) REFERENCES public.services(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.programs
    ADD CONSTRAINT programs_service_id_fkey FOREIGN KEY (service_id) REFERENCES public.services(id);

ALTER TABLE ONLY public.recording_anime_episodes
    ADD CONSTRAINT recording_anime_episodes_recording_id_fkey FOREIGN KEY (recording_id) REFERENCES public.recording_anime_matches(recording_id) ON DELETE CASCADE;

ALTER TABLE ONLY public.recording_anime_matches
    ADD CONSTRAINT recording_anime_matches_recording_id_fkey FOREIGN KEY (recording_id) REFERENCES public.recordings(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.recording_assignments
    ADD CONSTRAINT recording_assignments_reservation_id_fkey FOREIGN KEY (reservation_id) REFERENCES public.reservations(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.recording_auto_captions
    ADD CONSTRAINT recording_auto_captions_recording_id_fkey FOREIGN KEY (recording_id) REFERENCES public.recordings(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.recording_cm_detections
    ADD CONSTRAINT recording_cm_detections_recording_id_fkey FOREIGN KEY (recording_id) REFERENCES public.recordings(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.recording_cm_materials
    ADD CONSTRAINT recording_cm_materials_recording_id_fkey FOREIGN KEY (recording_id) REFERENCES public.recordings(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.recording_cm_sections
    ADD CONSTRAINT recording_cm_sections_recording_id_detector_fkey FOREIGN KEY (recording_id, detector) REFERENCES public.recording_cm_detections(recording_id, detector) ON DELETE CASCADE;

ALTER TABLE ONLY public.recording_drama_matches
    ADD CONSTRAINT recording_drama_matches_recording_id_fkey FOREIGN KEY (recording_id) REFERENCES public.recordings(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.recording_files
    ADD CONSTRAINT recording_files_recording_id_fkey FOREIGN KEY (recording_id) REFERENCES public.recordings(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.recording_history
    ADD CONSTRAINT recording_history_recording_id_fkey FOREIGN KEY (recording_id) REFERENCES public.recordings(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.recording_history
    ADD CONSTRAINT recording_history_service_id_fkey FOREIGN KEY (service_id) REFERENCES public.services(id);

ALTER TABLE ONLY public.recording_live_stats
    ADD CONSTRAINT recording_live_stats_recording_id_fkey FOREIGN KEY (recording_id) REFERENCES public.recordings(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.recording_nicolog_files
    ADD CONSTRAINT recording_nicolog_files_recording_id_fkey FOREIGN KEY (recording_id) REFERENCES public.recordings(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.recording_tags
    ADD CONSTRAINT recording_tags_recording_id_fkey FOREIGN KEY (recording_id) REFERENCES public.recordings(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.recording_tags
    ADD CONSTRAINT recording_tags_tag_id_fkey FOREIGN KEY (tag_id) REFERENCES public.tags(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.recordings
    ADD CONSTRAINT recordings_reservation_id_fkey FOREIGN KEY (reservation_id) REFERENCES public.reservations(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.recordings
    ADD CONSTRAINT recordings_rule_id_fkey FOREIGN KEY (rule_id) REFERENCES public.rules(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.recordings
    ADD CONSTRAINT recordings_service_id_fkey FOREIGN KEY (service_id) REFERENCES public.services(id);

ALTER TABLE ONLY public.reservation_rules
    ADD CONSTRAINT reservation_rules_reservation_id_fkey FOREIGN KEY (reservation_id) REFERENCES public.reservations(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.reservation_rules
    ADD CONSTRAINT reservation_rules_rule_id_fkey FOREIGN KEY (rule_id) REFERENCES public.rules(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.reservations
    ADD CONSTRAINT reservations_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.reservations
    ADD CONSTRAINT reservations_relayed_from_fkey FOREIGN KEY (relayed_from) REFERENCES public.reservations(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.reservations
    ADD CONSTRAINT reservations_service_id_fkey FOREIGN KEY (service_id) REFERENCES public.services(id);

ALTER TABLE ONLY public.reservations
    ADD CONSTRAINT reservations_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.rule_groups
    ADD CONSTRAINT rule_groups_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.rule_groups
    ADD CONSTRAINT rule_groups_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.rules
    ADD CONSTRAINT rules_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.rules
    ADD CONSTRAINT rules_group_id_fkey FOREIGN KEY (group_id) REFERENCES public.rule_groups(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.rules
    ADD CONSTRAINT rules_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.services
    ADD CONSTRAINT services_channel_type_fkey FOREIGN KEY (channel_type) REFERENCES public.channel_types(type);

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.stream_urls
    ADD CONSTRAINT stream_urls_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.timeshift_assignments
    ADD CONSTRAINT timeshift_assignments_service_id_fkey FOREIGN KEY (service_id) REFERENCES public.services(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.timeshift_channels
    ADD CONSTRAINT timeshift_channels_service_id_fkey FOREIGN KEY (service_id) REFERENCES public.services(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.timeshift_channels
    ADD CONSTRAINT timeshift_channels_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.timeshift_chunks
    ADD CONSTRAINT timeshift_chunks_service_id_fkey FOREIGN KEY (service_id) REFERENCES public.services(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.timeshift_programs
    ADD CONSTRAINT timeshift_programs_service_id_fkey FOREIGN KEY (service_id) REFERENCES public.services(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.timeshift_promotions
    ADD CONSTRAINT timeshift_promotions_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.timeshift_promotions
    ADD CONSTRAINT timeshift_promotions_recording_id_fkey FOREIGN KEY (recording_id) REFERENCES public.recordings(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.timeshift_promotions
    ADD CONSTRAINT timeshift_promotions_service_id_fkey FOREIGN KEY (service_id) REFERENCES public.services(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.timeshift_watches
    ADD CONSTRAINT timeshift_watches_service_id_fkey FOREIGN KEY (service_id) REFERENCES public.services(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.totp_enrollments
    ADD CONSTRAINT totp_enrollments_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.user_oidc_identities
    ADD CONSTRAINT user_oidc_identities_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.user_passkeys
    ADD CONSTRAINT user_passkeys_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.user_recovery_codes
    ADD CONSTRAINT user_recovery_codes_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.user_setting_versions
    ADD CONSTRAINT user_setting_versions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.user_settings
    ADD CONSTRAINT user_settings_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.user_totps
    ADD CONSTRAINT user_totps_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.watch_progress
    ADD CONSTRAINT watch_progress_recording_id_fkey FOREIGN KEY (recording_id) REFERENCES public.recordings(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.watch_progress
    ADD CONSTRAINT watch_progress_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.watch_reservations
    ADD CONSTRAINT watch_reservations_service_id_fkey FOREIGN KEY (service_id) REFERENCES public.services(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.watch_reservations
    ADD CONSTRAINT watch_reservations_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.watch_reservations
    ADD CONSTRAINT watch_reservations_watch_rule_id_fkey FOREIGN KEY (watch_rule_id) REFERENCES public.watch_rules(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.watch_rule_dismissals
    ADD CONSTRAINT watch_rule_dismissals_service_id_fkey FOREIGN KEY (service_id) REFERENCES public.services(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.watch_rule_dismissals
    ADD CONSTRAINT watch_rule_dismissals_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.watch_rules
    ADD CONSTRAINT watch_rules_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.web_push_subscriptions
    ADD CONSTRAINT web_push_subscriptions_session_id_fkey FOREIGN KEY (session_id) REFERENCES public.sessions(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.web_push_subscriptions
    ADD CONSTRAINT web_push_subscriptions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.webauthn_ceremonies
    ADD CONSTRAINT webauthn_ceremonies_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

INSERT INTO public.goose_db_version VALUES (1, 0, true, '2026-11-23 02:22:07.153096');
INSERT INTO public.goose_db_version VALUES (2, 20261009102215, true, '2026-11-23 02:22:07.16911');
INSERT INTO public.goose_db_version VALUES (3, 20261009105537, true, '2026-11-23 02:22:07.171514');
INSERT INTO public.goose_db_version VALUES (4, 20261009114515, true, '2026-11-23 02:22:07.177118');
INSERT INTO public.goose_db_version VALUES (5, 20261009144311, true, '2026-11-23 02:22:07.179436');
INSERT INTO public.goose_db_version VALUES (6, 20261009144517, true, '2026-11-23 02:22:07.182076');
INSERT INTO public.goose_db_version VALUES (7, 20261009165859, true, '2026-11-23 02:22:07.186433');
INSERT INTO public.goose_db_version VALUES (8, 20261009173452, true, '2026-11-23 02:22:07.196733');
INSERT INTO public.goose_db_version VALUES (9, 20261009180001, true, '2026-11-23 02:22:07.199945');
INSERT INTO public.goose_db_version VALUES (10, 20261009180647, true, '2026-11-23 02:22:07.202353');
INSERT INTO public.goose_db_version VALUES (11, 20261009183023, true, '2026-11-23 02:22:07.203827');
INSERT INTO public.goose_db_version VALUES (12, 20261009191951, true, '2026-11-23 02:22:07.206603');
INSERT INTO public.goose_db_version VALUES (13, 20261009194520, true, '2026-11-23 02:22:07.20707');
INSERT INTO public.goose_db_version VALUES (14, 20261009210555, true, '2026-11-23 02:22:07.207924');
INSERT INTO public.goose_db_version VALUES (15, 20261010000901, true, '2026-11-23 02:22:07.208879');
INSERT INTO public.goose_db_version VALUES (16, 20261010005217, true, '2026-11-23 02:22:07.210424');
INSERT INTO public.goose_db_version VALUES (17, 20261010005441, true, '2026-11-23 02:22:07.212543');
INSERT INTO public.goose_db_version VALUES (18, 20261010012323, true, '2026-11-23 02:22:07.212791');
INSERT INTO public.goose_db_version VALUES (19, 20261010021816, true, '2026-11-23 02:22:07.21333');
INSERT INTO public.goose_db_version VALUES (20, 20261010032917, true, '2026-11-23 02:22:07.215411');
INSERT INTO public.goose_db_version VALUES (21, 20261010035738, true, '2026-11-23 02:22:07.215718');
INSERT INTO public.goose_db_version VALUES (22, 20261010054641, true, '2026-11-23 02:22:07.217583');
INSERT INTO public.goose_db_version VALUES (23, 20261010070138, true, '2026-11-23 02:22:07.218261');
INSERT INTO public.goose_db_version VALUES (24, 20261010071813, true, '2026-11-23 02:22:07.220972');
INSERT INTO public.goose_db_version VALUES (25, 20261010074717, true, '2026-11-23 02:22:07.221852');
INSERT INTO public.goose_db_version VALUES (26, 20261010082530, true, '2026-11-23 02:22:07.222563');
INSERT INTO public.goose_db_version VALUES (27, 20261010083211, true, '2026-11-23 02:22:07.22367');
INSERT INTO public.goose_db_version VALUES (28, 20261010085305, true, '2026-11-23 02:22:07.226324');
INSERT INTO public.goose_db_version VALUES (29, 20261010092629, true, '2026-11-23 02:22:07.226798');
INSERT INTO public.goose_db_version VALUES (30, 20261010094944, true, '2026-11-23 02:22:07.227481');
INSERT INTO public.goose_db_version VALUES (31, 20261010103919, true, '2026-11-23 02:22:07.227786');
INSERT INTO public.goose_db_version VALUES (32, 20261010114512, true, '2026-11-23 02:22:07.230307');
INSERT INTO public.goose_db_version VALUES (33, 20261010122220, true, '2026-11-23 02:22:07.232051');
INSERT INTO public.goose_db_version VALUES (34, 20261010124710, true, '2026-11-23 02:22:07.232712');
INSERT INTO public.goose_db_version VALUES (35, 20261010131209, true, '2026-11-23 02:22:07.233018');
INSERT INTO public.goose_db_version VALUES (36, 20261010232840, true, '2026-11-23 02:22:07.233908');
INSERT INTO public.goose_db_version VALUES (37, 20261011141802, true, '2026-11-23 02:22:07.234603');
INSERT INTO public.goose_db_version VALUES (38, 20261014153543, true, '2026-11-23 02:22:07.235326');
INSERT INTO public.goose_db_version VALUES (39, 20261014172207, true, '2026-11-23 02:22:07.235764');
INSERT INTO public.goose_db_version VALUES (40, 20261015014428, true, '2026-11-23 02:22:07.236118');
INSERT INTO public.goose_db_version VALUES (41, 20261015102002, true, '2026-11-23 02:22:07.236349');
INSERT INTO public.goose_db_version VALUES (42, 20261015125552, true, '2026-11-23 02:22:07.236627');
INSERT INTO public.goose_db_version VALUES (43, 20261017015122, true, '2026-11-23 02:22:07.236842');
INSERT INTO public.goose_db_version VALUES (44, 20261017050956, true, '2026-11-23 02:22:07.238262');
INSERT INTO public.goose_db_version VALUES (45, 20261017084415, true, '2026-11-23 02:22:07.238924');
INSERT INTO public.goose_db_version VALUES (46, 20261017085909, true, '2026-11-23 02:22:07.239154');
INSERT INTO public.goose_db_version VALUES (47, 20261017101214, true, '2026-11-23 02:22:07.240591');
INSERT INTO public.goose_db_version VALUES (48, 20261017130848, true, '2026-11-23 02:22:07.241508');
INSERT INTO public.goose_db_version VALUES (49, 20261017134329, true, '2026-11-23 02:22:07.241669');
INSERT INTO public.goose_db_version VALUES (50, 20261018034710, true, '2026-11-23 02:22:07.242076');
INSERT INTO public.goose_db_version VALUES (51, 20261018052916, true, '2026-11-23 02:22:07.242236');
INSERT INTO public.goose_db_version VALUES (52, 20261018055523, true, '2026-11-23 02:22:07.243212');
INSERT INTO public.goose_db_version VALUES (53, 20261018072046, true, '2026-11-23 02:22:07.244515');
INSERT INTO public.goose_db_version VALUES (54, 20261018094040, true, '2026-11-23 02:22:07.244684');
INSERT INTO public.goose_db_version VALUES (55, 20261019122336, true, '2026-11-23 02:22:07.247089');
INSERT INTO public.goose_db_version VALUES (56, 20261019144322, true, '2026-11-23 02:22:07.247283');
INSERT INTO public.goose_db_version VALUES (57, 20261020022223, true, '2026-11-23 02:22:07.247612');
INSERT INTO public.goose_db_version VALUES (58, 20261022134249, true, '2026-11-23 02:22:07.247963');
INSERT INTO public.goose_db_version VALUES (59, 20261022151654, true, '2026-11-23 02:22:07.250829');
INSERT INTO public.goose_db_version VALUES (60, 20261026042334, true, '2026-11-23 02:22:07.251012');
INSERT INTO public.goose_db_version VALUES (61, 20261026044501, true, '2026-11-23 02:22:07.251841');
INSERT INTO public.goose_db_version VALUES (62, 20261026112752, true, '2026-11-23 02:22:07.252038');
INSERT INTO public.goose_db_version VALUES (63, 20261026113754, true, '2026-11-23 02:22:07.252346');
INSERT INTO public.goose_db_version VALUES (64, 20261031152620, true, '2026-11-23 02:22:07.255692');
INSERT INTO public.goose_db_version VALUES (65, 20261031161654, true, '2026-11-23 02:22:07.256398');
INSERT INTO public.goose_db_version VALUES (66, 20261031212745, true, '2026-11-23 02:22:07.256705');
INSERT INTO public.goose_db_version VALUES (67, 20261031222140, true, '2026-11-23 02:22:07.257775');
INSERT INTO public.goose_db_version VALUES (68, 20261031230434, true, '2026-11-23 02:22:07.257998');
INSERT INTO public.goose_db_version VALUES (69, 20261031235744, true, '2026-11-23 02:22:07.258147');
INSERT INTO public.goose_db_version VALUES (70, 20261101002400, true, '2026-11-23 02:22:07.259734');
INSERT INTO public.goose_db_version VALUES (71, 20261101024016, true, '2026-11-23 02:22:07.262002');
INSERT INTO public.goose_db_version VALUES (72, 20261101032617, true, '2026-11-23 02:22:07.262266');
INSERT INTO public.goose_db_version VALUES (73, 20261101034515, true, '2026-11-23 02:22:07.264967');
INSERT INTO public.goose_db_version VALUES (74, 20261101040216, true, '2026-11-23 02:22:07.268041');
INSERT INTO public.goose_db_version VALUES (75, 20261101060655, true, '2026-11-23 02:22:07.270413');
INSERT INTO public.goose_db_version VALUES (76, 20261101071442, true, '2026-11-23 02:22:07.272072');
INSERT INTO public.goose_db_version VALUES (77, 20261101084935, true, '2026-11-23 02:22:07.277808');
INSERT INTO public.goose_db_version VALUES (78, 20261101123249, true, '2026-11-23 02:22:07.279327');
INSERT INTO public.goose_db_version VALUES (79, 20261101234057, true, '2026-11-23 02:22:07.279961');
INSERT INTO public.goose_db_version VALUES (80, 20261102001547, true, '2026-11-23 02:22:07.280409');
INSERT INTO public.goose_db_version VALUES (81, 20261102014335, true, '2026-11-23 02:22:07.280865');
INSERT INTO public.goose_db_version VALUES (82, 20261102023243, true, '2026-11-23 02:22:07.28126');
INSERT INTO public.goose_db_version VALUES (83, 20261102032335, true, '2026-11-23 02:22:07.281682');
INSERT INTO public.goose_db_version VALUES (84, 20261107044440, true, '2026-11-23 02:22:07.28379');
INSERT INTO public.goose_db_version VALUES (85, 20261107053731, true, '2026-11-23 02:22:07.284143');
INSERT INTO public.goose_db_version VALUES (86, 20261107065410, true, '2026-11-23 02:22:07.285182');
INSERT INTO public.goose_db_version VALUES (87, 20261107072014, true, '2026-11-23 02:22:07.285399');
INSERT INTO public.goose_db_version VALUES (88, 20261107074534, true, '2026-11-23 02:22:07.285631');
INSERT INTO public.goose_db_version VALUES (89, 20261107083102, true, '2026-11-23 02:22:07.286401');
INSERT INTO public.goose_db_version VALUES (90, 20261107092133, true, '2026-11-23 02:22:07.287204');
INSERT INTO public.goose_db_version VALUES (91, 20261107093927, true, '2026-11-23 02:22:07.287807');
INSERT INTO public.goose_db_version VALUES (92, 20261109080340, true, '2026-11-23 02:22:07.288123');
INSERT INTO public.goose_db_version VALUES (93, 20261109120000, true, '2026-11-23 02:22:07.289809');
INSERT INTO public.goose_db_version VALUES (94, 20261109130000, true, '2026-11-23 02:22:07.29027');
INSERT INTO public.goose_db_version VALUES (95, 20261109140000, true, '2026-11-23 02:22:07.290774');
INSERT INTO public.goose_db_version VALUES (96, 20261109150000, true, '2026-11-23 02:22:07.291497');
INSERT INTO public.goose_db_version VALUES (97, 20261110034240, true, '2026-11-23 02:22:07.292605');
INSERT INTO public.goose_db_version VALUES (98, 20261110044912, true, '2026-11-23 02:22:07.293642');
INSERT INTO public.goose_db_version VALUES (99, 20261110053854, true, '2026-11-23 02:22:07.294562');
INSERT INTO public.goose_db_version VALUES (100, 20261110093607, true, '2026-11-23 02:22:07.295335');
INSERT INTO public.goose_db_version VALUES (101, 20261111050930, true, '2026-11-23 02:22:07.296473');
INSERT INTO public.goose_db_version VALUES (102, 20261111052510, true, '2026-11-23 02:22:07.296675');
INSERT INTO public.goose_db_version VALUES (103, 20261112035924, true, '2026-11-23 02:22:07.296859');
INSERT INTO public.goose_db_version VALUES (104, 20261112063157, true, '2026-11-23 02:22:07.297207');
INSERT INTO public.goose_db_version VALUES (105, 20261112103738, true, '2026-11-23 02:22:07.29748');
INSERT INTO public.goose_db_version VALUES (106, 20261112104044, true, '2026-11-23 02:22:07.298052');
INSERT INTO public.goose_db_version VALUES (107, 20261112105744, true, '2026-11-23 02:22:07.299231');
INSERT INTO public.goose_db_version VALUES (108, 20261112113947, true, '2026-11-23 02:22:07.299645');
INSERT INTO public.goose_db_version VALUES (109, 20261112115223, true, '2026-11-23 02:22:07.300496');
INSERT INTO public.goose_db_version VALUES (110, 20261112132338, true, '2026-11-23 02:22:07.30067');
INSERT INTO public.goose_db_version VALUES (111, 20261112134845, true, '2026-11-23 02:22:07.301509');
INSERT INTO public.goose_db_version VALUES (112, 20261112150304, true, '2026-11-23 02:22:07.302075');
INSERT INTO public.goose_db_version VALUES (113, 20261113115901, true, '2026-11-23 02:22:07.30247');
INSERT INTO public.goose_db_version VALUES (114, 20261113152156, true, '2026-11-23 02:22:07.302652');
INSERT INTO public.goose_db_version VALUES (115, 20261113162119, true, '2026-11-23 02:22:07.303183');
INSERT INTO public.goose_db_version VALUES (116, 20261113173102, true, '2026-11-23 02:22:07.303945');
INSERT INTO public.goose_db_version VALUES (117, 20261114063115, true, '2026-11-23 02:22:07.304513');
INSERT INTO public.goose_db_version VALUES (118, 20261114072125, true, '2026-11-23 02:22:07.305215');
INSERT INTO public.goose_db_version VALUES (119, 20261114083357, true, '2026-11-23 02:22:07.305756');
INSERT INTO public.goose_db_version VALUES (120, 20261114132533, true, '2026-11-23 02:22:07.306597');
INSERT INTO public.goose_db_version VALUES (121, 20261114140854, true, '2026-11-23 02:22:07.30719');
INSERT INTO public.goose_db_version VALUES (122, 20261114153515, true, '2026-11-23 02:22:07.308231');
INSERT INTO public.goose_db_version VALUES (123, 20261114155118, true, '2026-11-23 02:22:07.308532');
INSERT INTO public.goose_db_version VALUES (124, 20261114182309, true, '2026-11-23 02:22:07.308967');
INSERT INTO public.goose_db_version VALUES (125, 20261114190556, true, '2026-11-23 02:22:07.30927');
INSERT INTO public.goose_db_version VALUES (126, 20261114205013, true, '2026-11-23 02:22:07.309793');
INSERT INTO public.goose_db_version VALUES (127, 20261114214813, true, '2026-11-23 02:22:07.310505');
INSERT INTO public.goose_db_version VALUES (128, 20261115154511, true, '2026-11-23 02:22:07.312439');
INSERT INTO public.goose_db_version VALUES (129, 20261115214338, true, '2026-11-23 02:22:07.314252');
INSERT INTO public.goose_db_version VALUES (130, 20261115223239, true, '2026-11-23 02:22:07.314879');
INSERT INTO public.goose_db_version VALUES (131, 20261115232010, true, '2026-11-23 02:22:07.315149');
INSERT INTO public.goose_db_version VALUES (132, 20261116033534, true, '2026-11-23 02:22:07.315725');
INSERT INTO public.goose_db_version VALUES (133, 20261116034238, true, '2026-11-23 02:22:07.31767');
INSERT INTO public.goose_db_version VALUES (134, 20261116084054, true, '2026-11-23 02:22:07.318255');
INSERT INTO public.goose_db_version VALUES (135, 20261116085349, true, '2026-11-23 02:22:07.323867');
INSERT INTO public.goose_db_version VALUES (136, 20261116090425, true, '2026-11-23 02:22:07.324579');
INSERT INTO public.goose_db_version VALUES (137, 20261116090426, true, '2026-11-23 02:22:07.325405');
INSERT INTO public.goose_db_version VALUES (138, 20261116110105, true, '2026-11-23 02:22:07.326174');
INSERT INTO public.goose_db_version VALUES (139, 20261116113046, true, '2026-11-23 02:22:07.326626');
INSERT INTO public.goose_db_version VALUES (140, 20261116150800, true, '2026-11-23 02:22:07.327575');
INSERT INTO public.goose_db_version VALUES (141, 20261117020000, true, '2026-11-23 02:22:07.3283');
INSERT INTO public.goose_db_version VALUES (142, 20261117020100, true, '2026-11-23 02:22:07.329736');
INSERT INTO public.goose_db_version VALUES (143, 20261117064557, true, '2026-11-23 02:22:07.330056');
INSERT INTO public.goose_db_version VALUES (144, 20261117070056, true, '2026-11-23 02:22:07.332068');
INSERT INTO public.goose_db_version VALUES (145, 20261117084042, true, '2026-11-23 02:22:07.334415');
INSERT INTO public.goose_db_version VALUES (146, 20261117135114, true, '2026-11-23 02:22:07.335049');
INSERT INTO public.goose_db_version VALUES (147, 20261117155106, true, '2026-11-23 02:22:07.335741');
INSERT INTO public.goose_db_version VALUES (148, 20261118123247, true, '2026-11-23 02:22:07.341801');
INSERT INTO public.goose_db_version VALUES (149, 20261120170214, true, '2026-11-23 02:22:07.343297');

INSERT INTO public.recording_failure_counts VALUES ('tuner_unavailable', 0, NULL, NULL);
INSERT INTO public.recording_failure_counts VALUES ('tuner_server_unreachable', 0, NULL, NULL);
INSERT INTO public.recording_failure_counts VALUES ('stream_interrupted', 0, NULL, NULL);
INSERT INTO public.recording_failure_counts VALUES ('storage_unavailable', 0, NULL, NULL);
INSERT INTO public.recording_failure_counts VALUES ('storage_full', 0, NULL, NULL);
INSERT INTO public.recording_failure_counts VALUES ('program_not_found', 0, NULL, NULL);
INSERT INTO public.recording_failure_counts VALUES ('cancelled', 0, NULL, NULL);
INSERT INTO public.recording_failure_counts VALUES ('recorder_unavailable', 0, NULL, NULL);
INSERT INTO public.recording_failure_counts VALUES ('tuner_preempted', 0, NULL, NULL);
INSERT INTO public.recording_failure_counts VALUES ('tuning_refused', 0, NULL, NULL);

SELECT pg_catalog.setval('public.goose_db_version_id_seq', 149, true);

package domain

//go:generate go tool go-enum --values --nocomments --sql

// RecordingState は、録画の状態 (設計書 5.5)。
//
// ENUM(recording, completed, completed_with_errors, partial, failed)
type RecordingState string

// RecordingFailureReason は、録画に失敗した理由 (設計書 5.5)。
//
// ENUM(
// tuner_unavailable
// tuner_server_unreachable
// stream_interrupted
// storage_unavailable
// storage_full
// program_not_found
// cancelled
// recorder_unavailable // 録画を始める時刻に、割り振りを取る mikan がいなかったこと (#517)。
// )
type RecordingFailureReason string

// RecordingAssignmentState は、mikan への割り振りの状態 (recording_assignments.state と timeshift_assignments.state)。
//
// ENUM(
// waiting // まだ mikan が取っていない割り振り。
// recording // mikan が取って録画している割り振り。
// handover // 止まる mikan が、引き継ぎを待っている割り振り (設計書 5.4)。
// finished // 録画を終えた割り振り。
// )
type RecordingAssignmentState string

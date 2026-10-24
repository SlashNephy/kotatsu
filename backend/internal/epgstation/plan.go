package epgstation

import (
	"cmp"
	"slices"
	"time"
)

// PlanOptions は、取り込みの計画の条件。
type PlanOptions struct {
	// Now は今の時刻。開始済みの手動予約は取り込まない。
	Now time.Time
	// Pools は、EPGStation の録画の保存先ディレクトリの名前から、Kotatsu のプール ID への対応。
	Pools map[string]string
}

// Plan は取り込みの計画。dry run で表示する。
type Plan struct {
	Rules           int
	ProgramReserves int
	TimeReserves    int
	// Recordings は取り込む録画の数。録画中のものは含まない。
	Recordings int
	// TSFiles と OtherFiles は、取り込む録画のファイルのうち、プールが対応付けられているものの数。
	// エンコード済みの MP4 のような TS 以外のファイルは、「その他のファイル」として登録する。
	TSFiles    int
	OtherFiles int
	DropLogs   int
	Histories  int
	Tags       int

	// StartedReserves は、開始済みなので取り込まない手動予約の数。
	StartedReserves int
	// RecordingsInProgress は、録画中なので取り込まない録画の数。
	RecordingsInProgress int

	// UnmappedDirectories は、プールが対応付けられていない録画の保存先ディレクトリ。名前の順に並べる。
	UnmappedDirectories []*UnmappedDirectory
}

// UnmappedDirectory は、プールが対応付けられていない録画の保存先ディレクトリと、それを参照しているものの数。
type UnmappedDirectory struct {
	Name     string
	Files    int
	Rules    int
	Reserves int
}

// NewPlan は、読み取ったデータから取り込みの計画を作る。
func NewPlan(snapshot *Snapshot, opts *PlanOptions) *Plan {
	p := &planner{
		opts: opts,
		plan: &Plan{
			Rules:                len(snapshot.Rules),
			ProgramReserves:      0,
			TimeReserves:         0,
			Recordings:           0,
			TSFiles:              0,
			OtherFiles:           0,
			DropLogs:             0,
			Histories:            len(snapshot.Histories),
			Tags:                 len(snapshot.Tags),
			StartedReserves:      0,
			RecordingsInProgress: 0,
			UnmappedDirectories:  []*UnmappedDirectory{},
		},
		unmapped: map[string]*UnmappedDirectory{},
	}
	for _, rule := range snapshot.Rules {
		p.addRule(rule)
	}
	for _, reserve := range snapshot.ManualReserves {
		p.addReserve(reserve)
	}
	for _, recording := range snapshot.Recordings {
		p.addRecording(recording)
	}

	for _, dir := range p.unmapped {
		p.plan.UnmappedDirectories = append(p.plan.UnmappedDirectories, dir)
	}
	slices.SortFunc(p.plan.UnmappedDirectories, func(a, b *UnmappedDirectory) int {
		return cmp.Compare(a.Name, b.Name)
	})

	return p.plan
}

// planner は、取り込みの計画を数えながら組み立てる。
type planner struct {
	opts     *PlanOptions
	plan     *Plan
	unmapped map[string]*UnmappedDirectory
}

// lookup は、プールが対応付けられていないディレクトリの集計を返す。対応付けられていれば nil を返す。
func (p *planner) lookup(name string) *UnmappedDirectory {
	if _, ok := p.opts.Pools[name]; ok {
		return nil
	}
	dir, ok := p.unmapped[name]
	if !ok {
		dir = &UnmappedDirectory{Name: name, Files: 0, Rules: 0, Reserves: 0}
		p.unmapped[name] = dir
	}

	return dir
}

// addRule はルールを数える。ルールと予約でディレクトリの名前が空なら、EPGStation の既定の保存先を使うので、
// 対応付けを求めない。
func (p *planner) addRule(rule *Rule) {
	if rule.ParentDirectoryName == "" {
		return
	}
	if dir := p.lookup(rule.ParentDirectoryName); dir != nil {
		dir.Rules++
	}
}

// addReserve は手動予約を数える。開始済みのものは取り込まない。
func (p *planner) addReserve(reserve *Reserve) {
	if !reserve.StartAt.After(p.opts.Now) {
		p.plan.StartedReserves++

		return
	}
	if reserve.IsTimeSpecified {
		p.plan.TimeReserves++
	} else {
		p.plan.ProgramReserves++
	}
	if reserve.ParentDirectoryName == "" {
		return
	}
	if dir := p.lookup(reserve.ParentDirectoryName); dir != nil {
		dir.Reserves++
	}
}

// addRecording は録画とそのファイルを数える。録画中のものは取り込まない。
func (p *planner) addRecording(recording *Recording) {
	if recording.IsRecording {
		p.plan.RecordingsInProgress++

		return
	}
	p.plan.Recordings++
	if recording.Drop != nil {
		p.plan.DropLogs++
	}
	for _, file := range recording.Files {
		if dir := p.lookup(file.ParentDirectoryName); dir != nil {
			dir.Files++

			continue
		}
		if file.Type == VideoFileTypeTS {
			p.plan.TSFiles++
		} else {
			p.plan.OtherFiles++
		}
	}
}

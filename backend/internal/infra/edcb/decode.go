package edcb

import (
	"time"
)

// minItemSize は、配列の要素の最小のバイト数。構造体は大きさ (DWORD) だけでも 4 バイトで、DWORD の要素も 4 バイト。
const minItemSize = 4

// 読み方は、EDCB の Common/CtrlCmdUtil.cpp の ReadVALUE と、Common/StructDef.h の型に合わせる。

func decodeService(d *decoder) *Service {
	s := new(Service)
	d.structure(func(d *decoder) {
		s.ONID, s.TSID, s.SID = d.u16(), d.u16(), d.u16()
		s.Type = d.u8()
		s.PartialReception = d.u8() != 0
		s.ProviderName = d.str()
		s.Name = d.str()
		s.NetworkName = d.str()
		s.TSName = d.str()
		s.RemoteControlKeyID = d.u8()
	})

	return s
}

func decodeServices(d *decoder) []*Service {
	services := []*Service{}
	d.vector(func(d *decoder) {
		services = append(services, decodeService(d))
	})

	return services
}

func decodeEventRef(d *decoder) EventRef {
	var ref EventRef
	d.structure(func(d *decoder) {
		ref.ONID, ref.TSID, ref.SID, ref.EventID = d.u16(), d.u16(), d.u16(), d.u16()
	})

	return ref
}

func decodeEventGroup(d *decoder) *EventGroup {
	group := &EventGroup{Type: 0, Events: []EventRef{}}
	if !d.optionalStructure(func(d *decoder) {
		group.Type = d.u8()
		d.vector(func(d *decoder) {
			group.Events = append(group.Events, decodeEventRef(d))
		})
	}) {
		return nil
	}

	return group
}

func decodeEvent(d *decoder) *Event {
	e := new(Event)
	e.Genres = []Genre{}
	e.Audios = []AudioComponent{}
	d.structure(func(d *decoder) {
		e.ONID, e.TSID, e.SID, e.EventID = d.u16(), d.u16(), d.u16(), d.u16()
		e.StartKnown = d.u8() != 0
		start := d.systemTime()
		if e.StartKnown {
			e.Start = start
		}
		e.DurationKnown = d.u8() != 0
		duration := time.Duration(d.u32()) * time.Second
		if e.DurationKnown {
			e.Duration = duration
		}
		d.optionalStructure(func(d *decoder) {
			e.Short = &ShortEvent{Name: d.str(), Text: d.str()}
		})
		d.optionalStructure(func(d *decoder) {
			text := d.str()
			e.Extended = &text
		})
		d.optionalStructure(func(d *decoder) {
			d.vector(func(d *decoder) {
				d.structure(func(d *decoder) {
					e.Genres = append(e.Genres, Genre{Level1: d.u8(), Level2: d.u8(), User1: d.u8(), User2: d.u8()})
				})
			})
		})
		d.optionalStructure(func(d *decoder) {
			e.Video = &Component{StreamContent: d.u8(), ComponentType: d.u8(), ComponentTag: d.u8(), Text: d.str()}
		})
		d.optionalStructure(func(d *decoder) {
			d.vector(func(d *decoder) {
				d.structure(func(d *decoder) {
					e.Audios = append(e.Audios, AudioComponent{
						StreamContent: d.u8(), ComponentType: d.u8(), ComponentTag: d.u8(), StreamType: d.u8(),
						SimulcastGroupTag: d.u8(), ESMultiLingual: d.u8() != 0, MainComponent: d.u8() != 0,
						QualityIndicator: d.u8(), SamplingRate: d.u8(), Text: d.str(),
					})
				})
			})
		})
		e.Group = decodeEventGroup(d)
		e.Relay = decodeEventGroup(d)
		e.Scrambled = d.u8() != 0
	})

	return e
}

func decodeServiceEvents(d *decoder) []*ServiceEvents {
	list := []*ServiceEvents{}
	d.vector(func(d *decoder) {
		item := &ServiceEvents{Service: nil, Events: []*Event{}}
		d.structure(func(d *decoder) {
			item.Service = decodeService(d)
			d.vector(func(d *decoder) {
				item.Events = append(item.Events, decodeEvent(d))
			})
		})
		list = append(list, item)
	})

	return list
}

func decodeTunerReserves(d *decoder) []*TunerReserve {
	list := []*TunerReserve{}
	d.vector(func(d *decoder) {
		r := &TunerReserve{TunerID: 0, TunerName: "", ReserveIDs: []uint32{}}
		d.structure(func(d *decoder) {
			r.TunerID = d.u32()
			r.TunerName = d.str()
			d.vector(func(d *decoder) {
				r.ReserveIDs = append(r.ReserveIDs, d.u32())
			})
		})
		list = append(list, r)
	})

	return list
}

func decodeTunerProcesses(d *decoder) []*TunerProcess {
	list := []*TunerProcess{}
	d.vector(func(d *decoder) {
		p := new(TunerProcess)
		d.structure(func(d *decoder) {
			p.TunerID = d.u32()
			p.ProcessID = d.i32()
			p.Drop = d.u64()
			p.Scramble = d.u64()
			p.SignalLevel = d.f32()
			p.Space, p.Channel, p.ONID, p.TSID = d.i32(), d.i32(), d.i32(), d.i32()
			p.Recording = d.u8() != 0
			p.CapturingEPG = d.u8() != 0
			p.ExtraFlags = d.u16()
		})
		list = append(list, p)
	})

	return list
}

func decodeNotification(d *decoder) *Notification {
	n := new(Notification)
	d.structure(func(d *decoder) {
		n.ID = d.u32()
		n.Time = d.systemTime()
		n.Param1, n.Param2, n.Count = d.u32(), d.u32(), d.u32()
		n.Param4, n.Param5, n.Param6 = d.str(), d.str(), d.str()
	})

	return n
}

func decodeFiles(d *decoder) []*File {
	files := []*File{}
	d.vector(func(d *decoder) {
		f := &File{Name: "", Data: []byte{}}
		d.structure(func(d *decoder) {
			f.Name = d.str()
			size := int(d.u32())
			_ = d.u32() // 予約された DWORD。
			if data := d.take(size); data != nil {
				f.Data = append([]byte{}, data...)
			}
		})
		files = append(files, f)
	})

	return files
}

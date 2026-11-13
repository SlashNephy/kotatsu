package iptvapi

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/query"
)

// xmltvTimeLayout は、XMLTV の時刻の書き方。
const xmltvTimeLayout = "20060102150405 -0700"

// xmltvDocument は、XMLTV の文書 (xmltv.dtd)。
type xmltvDocument struct {
	XMLName       xml.Name          `xml:"tv"`
	GeneratorName string            `xml:"generator-info-name,attr"`
	Channels      []*xmltvChannel   `xml:"channel"`
	Programmes    []*xmltvProgramme `xml:"programme"`
}

type xmltvChannel struct {
	ID          string     `xml:"id,attr"`
	DisplayName xmltvText  `xml:"display-name"`
	Icon        *xmltvIcon `xml:"icon"`
}

type xmltvIcon struct {
	Src string `xml:"src,attr"`
}

type xmltvText struct {
	Lang  string `xml:"lang,attr"`
	Value string `xml:",chardata"`
}

type xmltvProgramme struct {
	Start      string      `xml:"start,attr"`
	Stop       string      `xml:"stop,attr,omitempty"`
	Channel    string      `xml:"channel,attr"`
	Title      xmltvText   `xml:"title"`
	Desc       *xmltvText  `xml:"desc"`
	Categories []xmltvText `xml:"category"`
}

// Guide は、番組表を XMLTV で返す echo のハンドラー (GET /api/iptv/:token/guide.xml)。
// M3U に載せるサービスの、放送中とこれから 7 日の番組を載せる。
func (h *Handler) Guide(c *echo.Context) error {
	w, ctx, logger := c.Response(), c.Request().Context(), c.Logger()
	services, err := h.services(ctx)
	if err != nil {
		fail(ctx, logger, w, "failed to list the services for the IPTV guide", err)

		return nil
	}
	ids := make([]int64, 0, len(services))
	for i := range services {
		ids = append(ids, services[i].ID)
	}
	now := h.clock.Now()
	programs, err := h.queries.ListProgramsInRange(ctx, query.ListProgramsInRangeParams{ServiceIDs: ids, FromAt: now.Add(-guidePast), ToAt: now.Add(guideFuture)})
	if err != nil {
		fail(ctx, logger, w, "failed to list the programs for the IPTV guide", err)

		return nil
	}
	body, err := Guide(h.base(c), services, programs)
	if err != nil {
		fail(ctx, logger, w, "failed to write the IPTV guide", err)

		return nil
	}

	return c.Blob(http.StatusOK, "application/xml; charset=utf-8", body)
}

// Guide は、サービス services と番組 programs の XMLTV を、配信の URL の頭 base から作る。
// チャンネルの ID は、M3U の tvg-id と同じく、Kotatsu のサービス ID にする。時刻は UTC で書く。
// 終了時刻が未定の番組は、終わりを書かない (XMLTV では省ける)。
func Guide(base string, services []domain.Service, programs []domain.Program) ([]byte, error) {
	doc := &xmltvDocument{
		XMLName: xml.Name{Space: "", Local: "tv"}, GeneratorName: "Kotatsu",
		Channels: make([]*xmltvChannel, 0, len(services)), Programmes: make([]*xmltvProgramme, 0, len(programs)),
	}
	for i := range services {
		id := strconv.FormatInt(services[i].ID, 10)
		channel := &xmltvChannel{ID: id, DisplayName: xmltvText{Lang: "ja", Value: services[i].Name}, Icon: nil}
		if services[i].LogoID != nil {
			channel.Icon = &xmltvIcon{Src: base + "/services/" + id + "/logo"}
		}
		doc.Channels = append(doc.Channels, channel)
	}
	for i := range programs {
		programme, err := programmeOf(&programs[i])
		if err != nil {
			return nil, err
		}
		doc.Programmes = append(doc.Programmes, programme)
	}
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	buf.WriteString(`<!DOCTYPE tv SYSTEM "xmltv.dtd">` + "\n")
	encoder := xml.NewEncoder(&buf)
	encoder.Indent("", "  ")
	if err := encoder.Encode(doc); err != nil {
		return nil, fmt.Errorf("failed to encode the XMLTV: %w", err)
	}
	buf.WriteString("\n")

	return buf.Bytes(), nil
}

// programmeOf は、番組を XMLTV の番組にする。ジャンルは、大分類だけを日本語で書く。
func programmeOf(program *domain.Program) (*xmltvProgramme, error) {
	programme := &xmltvProgramme{
		Start: program.StartAt.UTC().Format(xmltvTimeLayout), Stop: "", Channel: strconv.FormatInt(program.ServiceID, 10),
		Title: xmltvText{Lang: "ja", Value: program.Name}, Desc: nil, Categories: []xmltvText{},
	}
	if !program.EndUndecided {
		programme.Stop = program.EndAt.UTC().Format(xmltvTimeLayout)
	}
	if program.Description != "" {
		programme.Desc = &xmltvText{Lang: "ja", Value: program.Description}
	}
	var genres []*domain.Genre
	if len(program.Genres) > 0 {
		if err := json.Unmarshal(program.Genres, &genres); err != nil {
			return nil, fmt.Errorf("failed to decode the genres: %w", err)
		}
	}
	seen := map[string]bool{}
	for _, genre := range genres {
		if genre == nil {
			continue
		}
		if name := genreName(genre.Lv1); name != "" && !seen[name] {
			seen[name] = true
			programme.Categories = append(programme.Categories, xmltvText{Lang: "ja", Value: name})
		}
	}

	return programme, nil
}

// genreName は、ジャンルの大分類 (ARIB STD-B10 付録 H) の名前を返す。分からなければ空を返す。
func genreName(lv1 int) string {
	names := []string{
		"ニュース／報道", "スポーツ", "情報／ワイドショー", "ドラマ", "音楽", "バラエティ", "映画", "アニメ／特撮",
		"ドキュメンタリー／教養", "劇場／公演", "趣味／教育", "福祉",
	}
	if lv1 < 0 || lv1 >= len(names) {
		return ""
	}

	return names[lv1]
}

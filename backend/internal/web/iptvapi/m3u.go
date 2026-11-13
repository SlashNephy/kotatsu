package iptvapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/SlashNephy/kotatsu/internal/domain"
)

// channelTypeGR は、地上波のチャンネルの種類。
const channelTypeGR = "GR"

// Playlist は、ライブ視聴のストリームの一覧を M3U で返す (GET /api/iptv/{token}/channels.m3u)。
// サービスごとに、無変換の TS のストリームと、ロゴと、XMLTV の番組表のチャンネルの ID (tvg-id) を書く。
func (h *Handler) Playlist(w http.ResponseWriter, r *http.Request) {
	services, err := h.services(r.Context())
	if err != nil {
		h.fail(r.Context(), w, "failed to list the services for the IPTV playlist", err)

		return
	}
	w.Header().Set("Content-Type", "audio/x-mpegurl; charset=utf-8")
	_, _ = w.Write([]byte(Playlist(h.base(r), services)))
}

// Playlist は、サービス services の M3U を、配信の URL の頭 base から作る。
func Playlist(base string, services []domain.Service) string {
	var b strings.Builder
	b.WriteString(`#EXTM3U url-tvg="` + base + `/guide.xml" x-tvg-url="` + base + `/guide.xml"` + "\n")
	for i := range services {
		service := &services[i]
		id := strconv.FormatInt(service.ID, 10)
		name := attribute(service.Name)
		b.WriteString(`#EXTINF:-1 tvg-id="` + id + `" tvg-name="` + name + `"`)
		if service.LogoID != nil {
			b.WriteString(` tvg-logo="` + base + "/services/" + id + `/logo"`)
		}
		if number := channelNumber(service); number != "" {
			b.WriteString(` tvg-chno="` + number + `"`)
		}
		b.WriteString(` group-title="` + attribute(service.ChannelType) + `",` + line(service.Name) + "\n")
		b.WriteString(base + "/services/" + id + "/stream.m2ts\n")
	}

	return b.String()
}

// channelNumber は、サービスのチャンネル番号を返す。地上波はリモコンキー ID、ほかはサービス ID (BS 141 など)。
// 地上波でリモコンキー ID がなければ空を返す。
func channelNumber(service *domain.Service) string {
	if service.ChannelType == channelTypeGR {
		if service.RemoteControlKeyID == nil {
			return ""
		}

		return strconv.FormatInt(*service.RemoteControlKeyID, 10)
	}

	return strconv.FormatInt(service.Sid, 10)
}

// line は、M3U の 1 行に収まるよう、改行を空白にする。
func line(value string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(value)
}

// attribute は、M3U の属性の値に入れられるよう、改行を空白に、二重引用符を一重引用符にする。
func attribute(value string) string {
	return strings.ReplaceAll(line(value), `"`, "'")
}

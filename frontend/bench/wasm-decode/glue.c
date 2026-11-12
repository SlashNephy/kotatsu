// #1516 の測定用の薄い口。parser でフレームに区切り、デコードしてフレームの面を JS に見せる。
#include <emscripten/emscripten.h>
#include <libavcodec/avcodec.h>
#include <libavutil/frame.h>
#include <libavutil/mem.h>

typedef struct Dec {
  AVCodecContext *c;
  AVCodecParserContext *p; // 映像だけ
  AVPacket *pkt;
  AVFrame *f;
} Dec;

// kind: 0 = mpeg2video, 1 = aac
EMSCRIPTEN_KEEPALIVE Dec *dec_open(int kind) {
  const AVCodec *codec = avcodec_find_decoder(kind == 0 ? AV_CODEC_ID_MPEG2VIDEO : AV_CODEC_ID_AAC);
  if (!codec) return NULL;
  Dec *d = av_mallocz(sizeof(Dec));
  d->c = avcodec_alloc_context3(codec);
  d->c->thread_count = 1;
  if (avcodec_open2(d->c, codec, NULL) < 0) return NULL;
  if (kind == 0) d->p = av_parser_init(AV_CODEC_ID_MPEG2VIDEO);
  d->pkt = av_packet_alloc();
  d->f = av_frame_alloc();
  return d;
}

EMSCRIPTEN_KEEPALIVE void dec_skip_b(Dec *d, int on) { d->c->skip_frame = on ? AVDISCARD_NONREF : AVDISCARD_DEFAULT; }

// 映像: parser を通して 1 フレーム分を切り出す。消費したバイト数を返す。*out に切り出せたか。
EMSCRIPTEN_KEEPALIVE int dec_parse(Dec *d, const uint8_t *buf, int size, double pts, int *out) {
  uint8_t *data = NULL;
  int len = 0;
  int64_t ipts = pts < 0 ? AV_NOPTS_VALUE : (int64_t)pts;
  int used = av_parser_parse2(d->p, d->c, &data, &len, buf, size, ipts, ipts, 0);
  *out = 0;
  if (len > 0) {
    av_packet_unref(d->pkt);
    if (av_new_packet(d->pkt, len) < 0) return -1;
    memcpy(d->pkt->data, data, len);
    d->pkt->pts = d->p->pts;
    *out = 1;
  }
  return used;
}

// 切り出した (映像) か、渡した (音声) パケットを送る。
EMSCRIPTEN_KEEPALIVE int dec_send_parsed(Dec *d) { return avcodec_send_packet(d->c, d->pkt); }

EMSCRIPTEN_KEEPALIVE int dec_send(Dec *d, const uint8_t *buf, int size, double pts) {
  av_packet_unref(d->pkt);
  if (buf) {
    if (av_new_packet(d->pkt, size) < 0) return -1;
    memcpy(d->pkt->data, buf, size);
    d->pkt->pts = pts < 0 ? AV_NOPTS_VALUE : (int64_t)pts;
    return avcodec_send_packet(d->c, d->pkt);
  }
  return avcodec_send_packet(d->c, NULL);
}

EMSCRIPTEN_KEEPALIVE int dec_receive(Dec *d) { return avcodec_receive_frame(d->c, d->f); }
EMSCRIPTEN_KEEPALIVE uint8_t *frame_data(Dec *d, int i) { return d->f->data[i]; }
EMSCRIPTEN_KEEPALIVE int frame_linesize(Dec *d, int i) { return d->f->linesize[i]; }
EMSCRIPTEN_KEEPALIVE int frame_width(Dec *d) { return d->f->width; }
EMSCRIPTEN_KEEPALIVE int frame_height(Dec *d) { return d->f->height; }
EMSCRIPTEN_KEEPALIVE int frame_nb_samples(Dec *d) { return d->f->nb_samples; }
EMSCRIPTEN_KEEPALIVE int frame_flags(Dec *d) { return d->f->flags; }
EMSCRIPTEN_KEEPALIVE int frame_pict_type(Dec *d) { return d->f->pict_type; }
EMSCRIPTEN_KEEPALIVE double frame_pts(Dec *d) { return (double)d->f->pts; }
EMSCRIPTEN_KEEPALIVE uint8_t *buf_alloc(int n) { return av_malloc(n); }
EMSCRIPTEN_KEEPALIVE void buf_free(uint8_t *p) { av_free(p); }
EMSCRIPTEN_KEEPALIVE void dec_flush(Dec *d) { avcodec_flush_buffers(d->c); }

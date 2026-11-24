// ブラウザ内でデコードする経路 (#1516、設計書 7.2) で、FFmpeg のデコーダを JS から呼ぶための薄い口。
// 映像は mpegvideo の parser でフレームに区切ってから mpeg2video でデコードし、音声は ADTS のフレームを 1 つずつ AAC でデコードする。
// デコードしたフレームの面は、WASM のメモリを指すまま JS に見せる (JS 側で複製せずにテクスチャへ上げる)。
// 指したメモリは、次に dec_receive を呼ぶまで使える。
//
// 時刻は、JS が伸ばした PTS (90kHz) を double で受け渡す。PTS のないフレームは、frame_has_pts が 0 を返す。
#include <emscripten/emscripten.h>
#include <libavcodec/avcodec.h>
#include <libavutil/frame.h>
#include <libavutil/mem.h>
#include <string.h>

typedef struct Dec {
  int kind;
  AVCodecContext *c;
  AVCodecParserContext *p; // 映像だけ
  AVPacket *pkt;
  AVFrame *f;
} Dec;

enum { KIND_MPEG2VIDEO = 0, KIND_AAC = 1 };

static int64_t to_ts(double pts) { return pts != pts ? AV_NOPTS_VALUE : (int64_t)pts; }

EMSCRIPTEN_KEEPALIVE void dec_close(Dec *d) {
  if (!d) return;
  if (d->p) av_parser_close(d->p);
  av_packet_free(&d->pkt);
  av_frame_free(&d->f);
  avcodec_free_context(&d->c);
  av_free(d);
}

// dec_open は、kind のデコーダを開く。開けなければ NULL を返す。
EMSCRIPTEN_KEEPALIVE Dec *dec_open(int kind) {
  const AVCodec *codec =
      avcodec_find_decoder(kind == KIND_MPEG2VIDEO ? AV_CODEC_ID_MPEG2VIDEO : AV_CODEC_ID_AAC);
  if (!codec) return NULL;
  Dec *d = av_mallocz(sizeof(Dec));
  if (!d) return NULL;
  d->kind = kind;
  d->c = avcodec_alloc_context3(codec);
  d->pkt = av_packet_alloc();
  d->f = av_frame_alloc();
  if (!d->c || !d->pkt || !d->f) {
    dec_close(d);
    return NULL;
  }
  // スレッドは使わない (crossOriginIsolated が要らないようにする)。
  d->c->thread_count = 1;
  if (avcodec_open2(d->c, codec, NULL) < 0) {
    dec_close(d);
    return NULL;
  }
  if (kind == KIND_MPEG2VIDEO) {
    d->p = av_parser_init(AV_CODEC_ID_MPEG2VIDEO);
    if (!d->p) {
      dec_close(d);
      return NULL;
    }
  }
  return d;
}

// dec_skip_b は、B フレームのデコードを省くかを切り替える (間に合わないとき)。
EMSCRIPTEN_KEEPALIVE void dec_skip_b(Dec *d, int on) {
  d->c->skip_frame = on ? AVDISCARD_NONREF : AVDISCARD_DEFAULT;
}

// dec_parse は、映像の parser にデータを渡し、消費したバイト数を返す。1 フレーム分を切り出せたら *out を 1 にする。
// pts は、このデータの先頭から始まるフレームの PTS (なければ NaN)。
EMSCRIPTEN_KEEPALIVE int dec_parse(Dec *d, const uint8_t *buf, int size, double pts, int *out) {
  uint8_t *data = NULL;
  int len = 0;
  int64_t ts = to_ts(pts);
  int used = av_parser_parse2(d->p, d->c, &data, &len, buf, size, ts, ts, 0);
  *out = 0;
  if (len > 0) {
    av_packet_unref(d->pkt);
    if (av_new_packet(d->pkt, len) < 0) return -1;
    memcpy(d->pkt->data, data, len);
    d->pkt->pts = d->p->pts;
    d->pkt->dts = d->p->dts;
    *out = 1;
  }
  return used;
}

// dec_send_parsed は、dec_parse で切り出したパケットをデコーダへ送る。
EMSCRIPTEN_KEEPALIVE int dec_send_parsed(Dec *d) { return avcodec_send_packet(d->c, d->pkt); }

// dec_send は、パケット (音声の ADTS のフレーム) をデコーダへ送る。buf が NULL なら、残りを出させる (drain)。
EMSCRIPTEN_KEEPALIVE int dec_send(Dec *d, const uint8_t *buf, int size, double pts) {
  av_packet_unref(d->pkt);
  if (!buf) return avcodec_send_packet(d->c, NULL);
  if (av_new_packet(d->pkt, size) < 0) return -1;
  memcpy(d->pkt->data, buf, size);
  d->pkt->pts = to_ts(pts);
  return avcodec_send_packet(d->c, d->pkt);
}

// dec_receive は、デコードしたフレームを 1 つ受け取る。受け取れたら 0、まだなければ負の値 (AVERROR(EAGAIN) など)。
EMSCRIPTEN_KEEPALIVE int dec_receive(Dec *d) { return avcodec_receive_frame(d->c, d->f); }

// dec_flush は、デコーダと parser の中に残っているデータを破棄する。選局したときと、飛んだときに呼ぶ。
EMSCRIPTEN_KEEPALIVE int dec_flush(Dec *d) {
  avcodec_flush_buffers(d->c);
  if (d->p) {
    av_parser_close(d->p);
    d->p = av_parser_init(AV_CODEC_ID_MPEG2VIDEO);
    if (!d->p) return -1;
  }
  return 0;
}

EMSCRIPTEN_KEEPALIVE uint8_t *frame_data(Dec *d, int i) { return d->f->data[i]; }
EMSCRIPTEN_KEEPALIVE int frame_linesize(Dec *d, int i) { return d->f->linesize[i]; }
EMSCRIPTEN_KEEPALIVE int frame_width(Dec *d) { return d->f->width; }
EMSCRIPTEN_KEEPALIVE int frame_height(Dec *d) { return d->f->height; }
EMSCRIPTEN_KEEPALIVE int frame_format(Dec *d) { return d->f->format; }
EMSCRIPTEN_KEEPALIVE int frame_flags(Dec *d) { return d->f->flags; }
EMSCRIPTEN_KEEPALIVE int frame_pict_type(Dec *d) { return d->f->pict_type; }
EMSCRIPTEN_KEEPALIVE int frame_has_pts(Dec *d) { return d->f->pts != AV_NOPTS_VALUE; }
EMSCRIPTEN_KEEPALIVE double frame_pts(Dec *d) { return (double)d->f->pts; }
EMSCRIPTEN_KEEPALIVE int frame_sar_num(Dec *d) { return d->f->sample_aspect_ratio.num; }
EMSCRIPTEN_KEEPALIVE int frame_sar_den(Dec *d) { return d->f->sample_aspect_ratio.den; }
EMSCRIPTEN_KEEPALIVE int frame_colorspace(Dec *d) { return d->f->colorspace; }
EMSCRIPTEN_KEEPALIVE int frame_color_range(Dec *d) { return d->f->color_range; }
EMSCRIPTEN_KEEPALIVE int frame_nb_samples(Dec *d) { return d->f->nb_samples; }
EMSCRIPTEN_KEEPALIVE int frame_sample_rate(Dec *d) { return d->f->sample_rate; }
EMSCRIPTEN_KEEPALIVE int frame_channels(Dec *d) { return d->f->ch_layout.nb_channels; }

// buf_alloc と buf_free は、JS がデータを WASM へ渡すための領域を取る。
EMSCRIPTEN_KEEPALIVE uint8_t *buf_alloc(int n) { return av_malloc(n); }
EMSCRIPTEN_KEEPALIVE void buf_free(uint8_t *p) { av_free(p); }

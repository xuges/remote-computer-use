#include "rcu.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <vncbaseframebuffer.h>
#include <vncconnection.h>

typedef struct {
    rcu *s;
    VncConnection *conn;
    uint8_t *buffer;
    int tls;
} vnc_state;
static void quiet(const gchar *d, GLogLevelFlags l, const gchar *m, gpointer p) {
    (void)d;
    (void)l;
    (void)m;
    (void)p;
}
static GLogWriterOutput quiet_writer(GLogLevelFlags l, const GLogField *f, gsize n, gpointer p) {
    (void)l;
    (void)f;
    (void)n;
    (void)p;
    return G_LOG_WRITER_HANDLED;
}
static void auth_failure(VncConnection *c, const char *reason, gpointer p) {
    (void)c;
    (void)reason;
    rcu_fail(((vnc_state *)p)->s, RCU_AUTH);
}
static void unsupported(VncConnection *c, guint type, gpointer p) {
    (void)c;
    (void)type;
    rcu_fail(((vnc_state *)p)->s, RCU_UNSUPPORTED);
}
static void disconnected(VncConnection *c, gpointer p) {
    (void)c;
    rcu_fail(((vnc_state *)p)->s, RCU_LOST);
}
static void error_cb(VncConnection *c, const char *m, gpointer p) {
    (void)c;
    (void)m;
    vnc_state *v = p;
    rcu_fail(v->s, v->tls ? RCU_TLS : RCU_CONNECT);
}
static int offered(GValueArray *a, int type) {
    for (guint i = 0; i < a->n_values; i++)
        if (g_value_get_enum(g_value_array_get_nth(a, i)) == type)
            return 1;
    return 0;
}
static void choose_auth(VncConnection *c, GValueArray *a, gpointer p) {
    vnc_state *v = p;
    int prefs[] = {VNC_CONNECTION_AUTH_VENCRYPT, VNC_CONNECTION_AUTH_TLS, VNC_CONNECTION_AUTH_ARD,
                   VNC_CONNECTION_AUTH_VNC, VNC_CONNECTION_AUTH_NONE};
    for (size_t i = 0; i < sizeof(prefs) / sizeof(prefs[0]); i++) {
        if (!offered(a, prefs[i]))
            continue;
        // Anonymous TLS lacks authenticated server identity; do not select it.
        if (prefs[i] == VNC_CONNECTION_AUTH_TLS)
            continue;
        if (i >= 2 && !v->s->allow_plain)
            continue;
        v->tls = prefs[i] == VNC_CONNECTION_AUTH_VENCRYPT;
        vnc_connection_set_auth_type(c, prefs[i]);
        return;
    }
    rcu_fail(v->s, RCU_UNSUPPORTED);
    vnc_connection_shutdown(c);
}
static void choose_subtype(VncConnection *c, guint type, GValueArray *a, gpointer p) {
    vnc_state *v = p;
    (void)type;
    int prefs[] = {VNC_CONNECTION_AUTH_VENCRYPT_X509VNC, VNC_CONNECTION_AUTH_VENCRYPT_X509PLAIN,
                   VNC_CONNECTION_AUTH_VENCRYPT_X509NONE};
    for (size_t i = 0; i < sizeof(prefs) / sizeof(prefs[0]); i++)
        if (offered(a, prefs[i])) {
            vnc_connection_set_auth_subtype(c, prefs[i]);
            return;
        }
    rcu_fail(v->s, RCU_UNSUPPORTED);
    vnc_connection_shutdown(c);
}
static void credential(VncConnection *c, GValueArray *a, gpointer p) {
    vnc_state *v = p;
    for (guint i = 0; i < a->n_values; i++) {
        int kind = g_value_get_enum(g_value_array_get_nth(a, i));
        const char *value = NULL;
        if (kind == VNC_CONNECTION_CREDENTIAL_PASSWORD)
            value = v->s->password;
        else if (kind == VNC_CONNECTION_CREDENTIAL_USERNAME)
            value = v->s->user;
        else if (kind == VNC_CONNECTION_CREDENTIAL_CLIENTNAME)
            value = "remote-computer-use";
        if (!value || !vnc_connection_set_credential(c, kind, value)) {
            rcu_fail(v->s, RCU_AUTH);
            vnc_connection_shutdown(c);
            return;
        }
    }
}
static int configure_frame(vnc_state *v, int w, int h) {
    if (!rcu_resize(v->s, w, h))
        return 0;
    uint8_t *b = calloc((size_t)w * h, 4);
    if (!b) {
        rcu_fail(v->s, RCU_CONNECT);
        return 0;
    }
    VncPixelFormat fmt = {.bits_per_pixel = 32,
                          .depth = 24,
                          .byte_order = G_LITTLE_ENDIAN,
                          .true_color_flag = 1,
                          .red_max = 255,
                          .green_max = 255,
                          .blue_max = 255,
                          .red_shift = 0,
                          .green_shift = 8,
                          .blue_shift = 16};
    VncBaseFramebuffer *fb =
        vnc_base_framebuffer_new(b, w, h, w * 4, &fmt, vnc_connection_get_pixel_format(v->conn));
    if (!fb || !vnc_connection_set_framebuffer(v->conn, VNC_FRAMEBUFFER(fb))) {
        if (fb)
            g_object_unref(fb);
        free(b);
        rcu_fail(v->s, RCU_CONNECT);
        return 0;
    }
    g_object_unref(fb);
    free(v->buffer);
    v->buffer = b;
    return 1;
}
static void initialized(VncConnection *c, gpointer p) {
    vnc_state *v = p;
    int w = vnc_connection_get_width(c), h = vnc_connection_get_height(c);
    if (!configure_frame(v, w, h)) {
        vnc_connection_shutdown(c);
        return;
    }
    gint32 enc[] = {VNC_CONNECTION_ENCODING_DESKTOP_RESIZE, VNC_CONNECTION_ENCODING_RICH_CURSOR,
                    VNC_CONNECTION_ENCODING_COPY_RECT,      VNC_CONNECTION_ENCODING_ZRLE,
                    VNC_CONNECTION_ENCODING_HEXTILE,        VNC_CONNECTION_ENCODING_RAW};
    vnc_connection_set_encodings(c, sizeof(enc) / sizeof(enc[0]), enc);
    vnc_connection_framebuffer_update_request(c, FALSE, 0, 0, w, h);
}
static void resized(VncConnection *c, gint w, gint h, gpointer p) {
    vnc_state *v = p;
    if (!v->buffer)
        return;
    if (configure_frame(v, w, h))
        vnc_connection_framebuffer_update_request(c, FALSE, 0, 0, w, h);
    else
        vnc_connection_shutdown(c);
}
static void updated(VncConnection *c, gint x, gint y, gint w, gint h, gpointer p) {
    vnc_state *v = p;
    int width = vnc_connection_get_width(c), height = vnc_connection_get_height(c);
    if (v->buffer)
        rcu_publish(v->s, v->buffer, width * 4, x, y, w, h);
    vnc_connection_framebuffer_update_request(c, TRUE, 0, 0, width, height);
}
static int send_event(vnc_state *v, rcu_event e) {
    VncConnection *c = v->conn;
    gboolean ok = TRUE;
    if (e.kind == 1)
        ok = vnc_connection_pointer_event(c, e.c, e.a, e.b);
    else if (e.kind == 2)
        ok = vnc_connection_key_event(c, e.c, e.a, 0);
    else if (e.kind == 3) {
        uint32_t k = e.a;
        if (k > 255)
            k |= 0x01000000;
        ok = vnc_connection_key_event(c, TRUE, k, 0) && vnc_connection_key_event(c, FALSE, k, 0);
    } else if (e.kind == 4) {
        for (int axis = 0; axis < 2; axis++) {
            int delta = axis ? e.b : e.a;
            int button = axis ? (delta > 0 ? 5 : 4) : (delta > 0 ? 7 : 6);
            for (int i = 0; i < abs(delta); i++)
                ok = ok && vnc_connection_pointer_event(c, 1u << (button - 1), e.c, e.d) &&
                     vnc_connection_pointer_event(c, 0, e.c, e.d);
        }
    } else
        return RCU_UNSUPPORTED;
    return ok ? 1 : RCU_LOST;
}
void *backend_run(void *ptr) {
    rcu *s = ptr;
    vnc_state v = {.s = s};
    g_log_set_default_handler(quiet, NULL);
    g_log_set_writer_func(quiet_writer, NULL, NULL);
    if (*s->fingerprint) {
        rcu_fail(s, RCU_UNSUPPORTED);
        return NULL;
    }
    v.conn = vnc_connection_new();
    if (!v.conn) {
        rcu_fail(s, RCU_CONNECT);
        return NULL;
    }
    g_signal_connect(v.conn, "vnc-auth-choose-type", G_CALLBACK(choose_auth), &v);
    g_signal_connect(v.conn, "vnc-auth-choose-subtype", G_CALLBACK(choose_subtype), &v);
    g_signal_connect(v.conn, "vnc-auth-credential", G_CALLBACK(credential), &v);
    g_signal_connect(v.conn, "vnc-auth-failure", G_CALLBACK(auth_failure), &v);
    g_signal_connect(v.conn, "vnc-auth-unsupported", G_CALLBACK(unsupported), &v);
    g_signal_connect(v.conn, "vnc-error", G_CALLBACK(error_cb), &v);
    g_signal_connect(v.conn, "vnc-disconnected", G_CALLBACK(disconnected), &v);
    g_signal_connect(v.conn, "vnc-initialized", G_CALLBACK(initialized), &v);
    g_signal_connect(v.conn, "vnc-desktop-resize", G_CALLBACK(resized), &v);
    g_signal_connect(v.conn, "vnc-framebuffer-update", G_CALLBACK(updated), &v);
    vnc_connection_set_shared(v.conn, TRUE);
    char port[16];
    snprintf(port, sizeof(port), "%u", s->port);
    if (!vnc_connection_open_host(v.conn, s->host, port))
        rcu_fail(s, RCU_CONNECT);
    while (!rcu_stopping(s)) {
        while (g_main_context_pending(NULL))
            g_main_context_iteration(NULL, FALSE);
        uint64_t info[4];
        if (rcu_status(s, info) < 0)
            break;
        rcu_event e;
        if (rcu_take(s, &e))
            rcu_complete(s, e.id, send_event(&v, e));
        usleep(2000);
    }
    vnc_connection_shutdown(v.conn);
    while (vnc_connection_is_open(v.conn)) {
        g_main_context_iteration(NULL, FALSE);
        usleep(1000);
    }
    g_object_unref(v.conn);
    free(v.buffer);
    return NULL;
}
void backend_interrupt(rcu *s) {
    (void)s;
}

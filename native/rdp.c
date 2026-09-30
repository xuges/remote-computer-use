#include "rcu.h"
#include <arpa/inet.h>
#include <freerdp/error.h>
#include <freerdp/freerdp.h>
#include <freerdp/gdi/gdi.h>
#include <freerdp/input.h>
#include <freerdp/settings.h>
#include <openssl/pem.h>
#include <openssl/x509v3.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <winpr/synch.h>
#include <winpr/wlog.h>

typedef struct {
    rdpContext context;
    rcu *s;
    unsigned buttons;
} rcu_rdp;
static rcu *owner(freerdp *i) {
    return ((rcu_rdp *)i->context)->s;
}

// Certificate policy is explicit per connection. A configured pin must also
// reject a different CA-valid certificate. No TOFU store or native prompting.
static int verify(freerdp *i, const BYTE *data, size_t length, const char *host, UINT16 port,
                  DWORD flags) {
    (void)port;
    (void)flags;
    rcu *s = owner(i);
    if (s->skip_tls_verify)
        return 1;
    int valid = 0;
    BIO *bio = BIO_new_mem_buf(data, (int)length);
    X509 *cert = bio ? PEM_read_bio_X509(bio, NULL, NULL, NULL) : NULL;
    if (cert && *s->fingerprint) {
        unsigned char md[EVP_MAX_MD_SIZE];
        unsigned int n = 0;
        char hex[65] = {0};
        if (X509_digest(cert, EVP_sha256(), md, &n) && n == 32) {
            for (unsigned int k = 0; k < n; k++)
                snprintf(hex + k * 2, 3, "%02x", md[k]);
            valid = strcmp(hex, s->fingerprint) == 0;
        }
    } else if (cert) {
        X509_STORE *store = X509_STORE_new();
        X509_STORE_CTX *ctx = X509_STORE_CTX_new();
        STACK_OF(X509) *chain = sk_X509_new_null();
        X509 *next;
        if (chain)
            while ((next = PEM_read_bio_X509(bio, NULL, NULL, NULL)) != NULL)
                sk_X509_push(chain, next);
        if (store && ctx && chain && X509_STORE_set_default_paths(store) &&
            X509_STORE_CTX_init(ctx, store, cert, chain)) {
            X509_VERIFY_PARAM *p = X509_STORE_CTX_get0_param(ctx);
            unsigned char ip[16];
            int numeric = inet_pton(AF_INET, host, ip) == 1 || inet_pton(AF_INET6, host, ip) == 1;
            int named = numeric ? X509_VERIFY_PARAM_set1_ip_asc(p, host)
                                : X509_VERIFY_PARAM_set1_host(p, host, 0);
            X509_VERIFY_PARAM_set_purpose(p, X509_PURPOSE_SSL_SERVER);
            if (named)
                valid = X509_verify_cert(ctx) == 1;
        }
        sk_X509_pop_free(chain, X509_free);
        X509_STORE_CTX_free(ctx);
        X509_STORE_free(store);
    }
    X509_free(cert);
    BIO_free(bio);
    if (!valid)
        rcu_fail(s, RCU_TLS);
    return valid ? 1 : -1;
}
static BOOL authenticate(freerdp *i, char **user, char **password, char **domain,
                         rdp_auth_reason reason) {
    (void)reason;
    (void)domain;
    if (user && *user && password && *password)
        return TRUE;
    rcu_fail(owner(i), RCU_AUTH);
    return FALSE;
}
static BOOL resize(rdpContext *c) {
    UINT32 w = freerdp_settings_get_uint32(c->settings, FreeRDP_DesktopWidth),
           h = freerdp_settings_get_uint32(c->settings, FreeRDP_DesktopHeight);
    if (!rcu_resize(((rcu_rdp *)c)->s, w, h))
        return FALSE;
    return gdi_resize(c->gdi, w, h);
}
static BOOL begin_paint(rdpContext *c) {
    if (c->gdi && c->gdi->primary && c->gdi->primary->hdc->hwnd)
        c->gdi->primary->hdc->hwnd->invalid->null = TRUE;
    return TRUE;
}
static BOOL end_paint(rdpContext *c) {
    rdpGdi *g = c->gdi;
    if (!g || !g->primary_buffer)
        return FALSE;
    if (!g->primary || !g->primary->hdc->hwnd || g->primary->hdc->hwnd->invalid->null)
        return TRUE;
    rcu_publish(((rcu_rdp *)c)->s, g->primary_buffer, g->stride, 0, 0, g->width, g->height);
    return TRUE;
}
static BOOL post_connect(freerdp *i) {
    if (!gdi_init(i, PIXEL_FORMAT_RGBA32))
        return FALSE;
    rdpContext *c = i->context;
    if (!rcu_resize(owner(i), c->gdi->width, c->gdi->height))
        return FALSE;
    c->update->BeginPaint = begin_paint;
    c->update->EndPaint = end_paint;
    c->update->DesktopResize = resize;
    return TRUE;
}
static void post_disconnect(freerdp *i) {
    gdi_free(i);
}
static int send_event(rcu_rdp *c, rcu_event e) {
    rdpInput *in = c->context.input;
    BOOL ok = TRUE;
    if (e.kind == 1) {
        ok = freerdp_input_send_mouse_event(in, PTR_FLAGS_MOVE, e.a, e.b);
        UINT16 flags[] = {PTR_FLAGS_BUTTON1, PTR_FLAGS_BUTTON3, PTR_FLAGS_BUTTON2};
        for (int k = 0; k < 3; k++) {
            unsigned bit = 1u << k;
            if ((c->buttons ^ e.c) & bit)
                ok = freerdp_input_send_mouse_event(
                         in, flags[k] | ((e.c & bit) ? PTR_FLAGS_DOWN : 0), e.a, e.b) &&
                     ok;
        }
        c->buttons = e.c;
    } else if (e.kind == 2) {
        ok = freerdp_input_send_keyboard_event_ex(in, e.c, FALSE, e.b);
    } else if (e.kind == 3) {
        uint32_t cp = e.a;
        UINT16 units[2];
        int count = 1;
        if (cp <= 0xffff)
            units[0] = cp;
        else {
            cp -= 0x10000;
            units[0] = 0xd800 + (cp >> 10);
            units[1] = 0xdc00 + (cp & 0x3ff);
            count = 2;
        }
        for (int k = 0; k < count; k++) {
            ok = freerdp_input_send_unicode_keyboard_event(in, 0, units[k]) && ok;
            ok = freerdp_input_send_unicode_keyboard_event(in, KBD_FLAGS_RELEASE, units[k]) && ok;
        }
    } else if (e.kind == 4) {
        for (int axis = 0; axis < 2; axis++) {
            int delta = axis ? e.b : e.a;
            int rotation = (delta > 0 ? 120 : -120) * (axis ? -1 : 1);
            UINT16 flags =
                (axis ? PTR_FLAGS_WHEEL : PTR_FLAGS_HWHEEL) | (rotation & WheelRotationMask);
            for (int k = 0; k < abs(delta); k++)
                ok = freerdp_input_send_mouse_event(in, flags, e.c, e.d) && ok;
        }
    } else
        return RCU_UNSUPPORTED;
    return ok ? 1 : RCU_LOST;
}
void *backend_run(void *ptr) {
    rcu *s = ptr;
    freerdp *i = freerdp_new();
    if (!i) {
        rcu_fail(s, RCU_CONNECT);
        return NULL;
    }
    WLog_SetLogLevel(WLog_GetRoot(), WLOG_OFF);
    i->ContextSize = sizeof(rcu_rdp);
    if (!freerdp_context_new(i)) {
        freerdp_free(i);
        rcu_fail(s, RCU_CONNECT);
        return NULL;
    }
    ((rcu_rdp *)i->context)->s = s;
    pthread_mutex_lock(&s->mutex);
    s->impl = i;
    pthread_mutex_unlock(&s->mutex);
    i->PostConnect = post_connect;
    i->PostDisconnect = post_disconnect;
    i->VerifyX509Certificate = verify;
    i->AuthenticateEx = authenticate;
    rdpSettings *cfg = i->context->settings;
    BOOL ok = freerdp_settings_set_string(cfg, FreeRDP_ServerHostname, s->host) &&
              freerdp_settings_set_uint32(cfg, FreeRDP_ServerPort, s->port) &&
              freerdp_settings_set_string(cfg, FreeRDP_Username, s->user) &&
              freerdp_settings_set_string(cfg, FreeRDP_Password, s->password) &&
              freerdp_settings_set_string(cfg, FreeRDP_Domain, s->domain) &&
              freerdp_settings_set_uint32(cfg, FreeRDP_DesktopWidth, s->requested_width) &&
              freerdp_settings_set_uint32(cfg, FreeRDP_DesktopHeight, s->requested_height) &&
              freerdp_settings_set_uint32(cfg, FreeRDP_ColorDepth, 32) &&
              freerdp_settings_set_uint32(cfg, FreeRDP_TcpConnectTimeout, 10000) &&
              freerdp_settings_set_bool(cfg, FreeRDP_ExternalCertificateManagement, TRUE) &&
              freerdp_settings_set_bool(cfg, FreeRDP_RdpSecurity, FALSE) &&
              freerdp_settings_set_bool(cfg, FreeRDP_TlsSecurity, TRUE) &&
              freerdp_settings_set_bool(cfg, FreeRDP_NlaSecurity, TRUE) &&
              freerdp_settings_set_bool(cfg, FreeRDP_SoftwareGdi, TRUE) &&
              freerdp_settings_set_bool(cfg, FreeRDP_SupportGraphicsPipeline, FALSE) &&
              freerdp_settings_set_bool(cfg, FreeRDP_RemoteFxCodec, FALSE) &&
              freerdp_settings_set_bool(cfg, FreeRDP_RedirectClipboard, FALSE) &&
              freerdp_settings_set_bool(cfg, FreeRDP_AudioPlayback, FALSE) &&
              freerdp_settings_set_bool(cfg, FreeRDP_AutoReconnectionEnabled, FALSE);
    if (!ok || rcu_stopping(s) || !freerdp_connect(i)) {
        UINT32 code = freerdp_get_last_error(i->context);
        rcu_fail(s, (code == FREERDP_ERROR_AUTHENTICATION_FAILED ||
                     code == FREERDP_ERROR_CONNECT_LOGON_FAILURE ||
                     code == FREERDP_ERROR_CONNECT_WRONG_PASSWORD)
                        ? RCU_AUTH
                        : RCU_CONNECT);
    } else {
        while (!rcu_stopping(s) && !freerdp_shall_disconnect_context(i->context)) {
            HANDLE handles[64];
            DWORD count = freerdp_get_event_handles(i->context, handles, 64);
            if (!count) {
                rcu_fail(s, RCU_LOST);
                break;
            }
            DWORD wait = WaitForMultipleObjects(count, handles, FALSE, 5);
            if (wait == WAIT_FAILED ||
                (wait != WAIT_TIMEOUT && !freerdp_check_event_handles(i->context))) {
                rcu_fail(s, RCU_LOST);
                break;
            }
            rcu_event e;
            if (rcu_take(s, &e))
                rcu_complete(s, e.id, send_event((rcu_rdp *)i->context, e));
        }
        if (!rcu_stopping(s))
            rcu_fail(s, RCU_LOST);
    }
    // Remove the shared instance before freeing so interruption cannot race it.
    pthread_mutex_lock(&s->mutex);
    s->impl = NULL;
    pthread_mutex_unlock(&s->mutex);
    freerdp_disconnect(i);
    freerdp_context_free(i);
    freerdp_free(i);
    return NULL;
}
void backend_interrupt(rcu *s) {
    pthread_mutex_lock(&s->mutex);
    freerdp *i = s->impl;
    if (i)
        freerdp_abort_connect_context(i->context);
    pthread_mutex_unlock(&s->mutex);
}

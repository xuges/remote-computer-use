#include "rcu.h"
#include <stdlib.h>
#include <string.h>
#include <stdio.h>

uint32_t rcu_abi_version(void) {
    return 2;
}
rcu *rcu_open(const char *host, uint32_t port, const char *user, const char *password,
              const char *domain, const char *fingerprint, uint32_t width, uint32_t height,
              uint32_t plain, uint32_t skip_tls_verify) {
#ifdef _WIN32
    // The worker already moved its startup reply to a private Win32 handle.
    // Silence the C runtime streams as well as the Win32 standard handles.
    (void)freopen("NUL", "w", stdout);
    (void)freopen("NUL", "w", stderr);
#endif
    rcu *s = calloc(1, sizeof(*s));
    if (!s)
        return NULL;
    pthread_mutex_init(&s->mutex, NULL);
    s->host = strdup(host);
    s->user = strdup(user);
    s->password = strdup(password);
    s->domain = strdup(domain);
    s->fingerprint = strdup(fingerprint);
    s->port = port;
    s->requested_width = width;
    s->requested_height = height;
    s->allow_plain = plain;
    s->skip_tls_verify = skip_tls_verify;
    if (!s->host || !s->user || !s->password || !s->domain || !s->fingerprint) {
        rcu_close(s);
        return NULL;
    }
    if (pthread_create(&s->thread, NULL, backend_run, s) != 0) {
        rcu_close(s);
        return NULL;
    }
    s->started = 1;
    return s;
}
int rcu_stopping(rcu *s) {
    pthread_mutex_lock(&s->mutex);
    int v = s->stop;
    pthread_mutex_unlock(&s->mutex);
    return v;
}
void rcu_fail(rcu *s, int code) {
    pthread_mutex_lock(&s->mutex);
    if (s->state >= 0)
        s->state = code;
    pthread_mutex_unlock(&s->mutex);
}
int32_t rcu_status(rcu *s, uint64_t *out) {
    pthread_mutex_lock(&s->mutex);
    out[0] = s->width;
    out[1] = s->height;
    out[2] = s->sequence;
    out[3] = s->geometry;
    int v = s->state;
    pthread_mutex_unlock(&s->mutex);
    return v;
}
int rcu_resize(rcu *s, uint32_t w, uint32_t h) {
    if (!w || !h || w > 8192 || h > 8192 || (uint64_t)w * h > RCU_MAX_PIXELS) {
        rcu_fail(s, RCU_INVALID);
        return 0;
    }
    uint8_t *p = calloc((size_t)w * h, 4), *coverage = calloc((size_t)w * h, 1);
    if (!p || !coverage) {
        free(p);
        free(coverage);
        rcu_fail(s, RCU_CONNECT);
        return 0;
    }
    pthread_mutex_lock(&s->mutex);
    free(s->pixels);
    free(s->coverage);
    s->pixels = p;
    s->coverage = coverage;
    s->width = w;
    s->height = h;
    s->geometry++;
    s->covered = 0;
    s->state = 0;
    pthread_mutex_unlock(&s->mutex);
    return 1;
}
// Publish only complete decoded rectangles. Initial coverage prevents returning
// uninitialized areas when a full framebuffer arrives as several rectangles.
void rcu_publish(rcu *s, const uint8_t *src, int stride, int x, int y, int w, int h) {
    pthread_mutex_lock(&s->mutex);
    if (s->state < 0 || x < 0 || y < 0 || w <= 0 || h <= 0 || (uint64_t)x + w > s->width ||
        (uint64_t)y + h > s->height) {
        pthread_mutex_unlock(&s->mutex);
        return;
    }
    for (int row = y; row < y + h; row++) {
        uint8_t *dst = s->pixels + ((size_t)row * s->width + x) * 4;
        memcpy(dst, src + (size_t)row * stride + (size_t)x * 4, (size_t)w * 4);
        for (int col = 0; col < w; col++)
            dst[col * 4 + 3] = 255;
        if (s->coverage)
            for (int col = x; col < x + w; col++) {
                size_t at = (size_t)row * s->width + col;
                if (!s->coverage[at]) {
                    s->coverage[at] = 1;
                    s->covered++;
                }
            }
    }
    if (s->covered == (uint64_t)s->width * s->height) {
        free(s->coverage);
        s->coverage = NULL;
        s->state = 1;
        s->sequence++;
    }
    pthread_mutex_unlock(&s->mutex);
}
uint8_t *rcu_snapshot(rcu *s, uint64_t *out) {
    pthread_mutex_lock(&s->mutex);
    uint8_t *p = NULL;
    if (s->state == 1) {
        size_t n = (size_t)s->width * s->height * 4;
        p = malloc(n);
        if (p) {
            memcpy(p, s->pixels, n);
            out[0] = s->width;
            out[1] = s->height;
            out[2] = s->sequence;
            out[3] = s->geometry;
            out[4] = n;
        }
    }
    pthread_mutex_unlock(&s->mutex);
    return p;
}
void rcu_free(void *p) {
    free(p);
}
uint64_t rcu_submit(rcu *s, int32_t kind, int32_t a, int32_t b, int32_t c, int32_t d) {
    pthread_mutex_lock(&s->mutex);
    uint64_t id = 0;
    if (s->state == 1 && !s->stop && !s->pending.id) {
        id = ++s->next_id;
        s->pending = (rcu_event){id, kind, a, b, c, d};
    }
    pthread_mutex_unlock(&s->mutex);
    return id;
}
int rcu_take(rcu *s, rcu_event *e) {
    pthread_mutex_lock(&s->mutex);
    *e = s->pending;
    s->pending.id = 0;
    pthread_mutex_unlock(&s->mutex);
    return e->id != 0;
}
void rcu_complete(rcu *s, uint64_t id, int result) {
    pthread_mutex_lock(&s->mutex);
    s->completed = id;
    s->result = result;
    pthread_mutex_unlock(&s->mutex);
}
int32_t rcu_result(rcu *s, uint64_t id) {
    pthread_mutex_lock(&s->mutex);
    int v = s->state < 0 ? s->state : (s->completed == id ? s->result : 0);
    pthread_mutex_unlock(&s->mutex);
    return v;
}
void rcu_cancel(rcu *s, uint64_t id) {
    pthread_mutex_lock(&s->mutex);
    if (s->pending.id == id)
        s->pending.id = 0;
    pthread_mutex_unlock(&s->mutex);
}
void rcu_close(rcu *s) {
    if (!s)
        return;
    pthread_mutex_lock(&s->mutex);
    s->stop = 1;
    pthread_mutex_unlock(&s->mutex);
    if (s->started) {
        backend_interrupt(s);
        pthread_join(s->thread, NULL);
    }
    if (s->password) {
        memset(s->password, 0, strlen(s->password));
        free(s->password);
    }
    free(s->host);
    free(s->user);
    free(s->domain);
    free(s->fingerprint);
    free(s->pixels);
    free(s->coverage);
    pthread_mutex_destroy(&s->mutex);
    free(s);
}

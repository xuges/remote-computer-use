#ifndef RCU_H
#define RCU_H
#include <pthread.h>
#include <stddef.h>
#include <stdint.h>

#ifdef _WIN32
#define RCU_API __declspec(dllexport)
#else
#define RCU_API __attribute__((visibility("default")))
#endif
#define RCU_MAX_PIXELS (16u * 1024u * 1024u)
enum {
    RCU_CONNECT = -1,
    RCU_AUTH = -2,
    RCU_TLS = -3,
    RCU_UNSUPPORTED = -4,
    RCU_LOST = -5,
    RCU_NOT_READY = -6,
    RCU_INVALID = -7
};
typedef struct {
    uint64_t id;
    int32_t kind, a, b, c, d;
} rcu_event;
typedef struct rcu {
    pthread_mutex_t mutex;
    pthread_t thread;
    int stop, state, started;
    char *host, *user, *password, *domain, *fingerprint;
    uint32_t port, requested_width, requested_height, allow_plain, skip_tls_verify;
    uint32_t width, height;
    uint64_t sequence, geometry, covered;
    uint8_t *pixels, *coverage;
    rcu_event pending;
    uint64_t next_id, completed;
    int result;
    void *impl;
} rcu;

RCU_API uint32_t rcu_abi_version(void);
RCU_API rcu *rcu_open(const char *, uint32_t, const char *, const char *, const char *,
                      const char *, uint32_t, uint32_t, uint32_t, uint32_t);
RCU_API int32_t rcu_status(rcu *, uint64_t *);
RCU_API uint8_t *rcu_snapshot(rcu *, uint64_t *);
RCU_API void rcu_free(void *);
RCU_API uint64_t rcu_submit(rcu *, int32_t, int32_t, int32_t, int32_t, int32_t);
RCU_API int32_t rcu_result(rcu *, uint64_t);
RCU_API void rcu_cancel(rcu *, uint64_t);
RCU_API void rcu_close(rcu *);

void *backend_run(void *);
void backend_interrupt(rcu *);
int rcu_stopping(rcu *);
void rcu_fail(rcu *, int);
int rcu_resize(rcu *, uint32_t, uint32_t);
void rcu_publish(rcu *, const uint8_t *, int, int, int, int, int);
int rcu_take(rcu *, rcu_event *);
void rcu_complete(rcu *, uint64_t, int);
#endif

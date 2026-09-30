/* Regression test: reject ARD horizontal scroll before it becomes a click. */
#include <assert.h>
#define vnc_connection_pointer_event capture_pointer
#include "../native/vnc.c"

static unsigned count;
static guint8 masks[16];
gboolean capture_pointer(VncConnection *c, guint8 mask, guint16 x, guint16 y) {
    (void)c;
    assert(x == 123 && y == 234);
    assert(count < sizeof(masks));
    masks[count++] = mask;
    return TRUE;
}
int main(void) {
    vnc_state apple = {.ard = 1};
    rcu_event wheel = {.kind = 4, .a = 2, .b = 2, .c = 123, .d = 234};
    assert(send_event(&apple, wheel) == RCU_UNSUPPORTED);
    assert(count == 0);
    wheel.a = -2;
    assert(send_event(&apple, wheel) == RCU_UNSUPPORTED);
    assert(count == 0);
    wheel.a = 0;
    assert(send_event(&apple, wheel) == 1);
    assert(count == 4 && masks[0] == 16 && masks[1] == 0 && masks[2] == 16 && masks[3] == 0);
    count = 0;
    vnc_state standard = {0};
    wheel.a = 2;
    wheel.b = 0;
    assert(send_event(&standard, wheel) == 1);
    assert(count == 4 && masks[0] == 64 && masks[1] == 0 && masks[2] == 64 && masks[3] == 0);
    puts("PASS: ARD horizontal scroll sends no clicks; standard VNC wheel remains supported");
    return 0;
}

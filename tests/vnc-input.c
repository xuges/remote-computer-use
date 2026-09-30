/* Regression tests for Apple ARD input quirks and standard VNC encoding. */
#include <assert.h>
#define vnc_connection_pointer_event capture_pointer
#define vnc_connection_key_event capture_key
#include "../native/vnc.c"

static unsigned count;
static guint8 masks[16];
static unsigned key_count, fail_key;
static struct { guint32 keysym; gboolean down; } keys[16];
gboolean capture_key(VncConnection *c, gboolean down, guint32 keysym, guint16 scancode) {
    (void)c;
    assert(scancode == 0);
    assert(key_count < sizeof(keys) / sizeof(keys[0]));
    keys[key_count].keysym = keysym;
    keys[key_count++].down = down;
    return !fail_key || key_count != fail_key;
}
gboolean capture_pointer(VncConnection *c, guint8 mask, guint16 x, guint16 y) {
    (void)c;
    assert(x == 123 && y == 234);
    assert(count < sizeof(masks));
    masks[count++] = mask;
    return TRUE;
}
static void text(vnc_state *v, int codepoint, guint32 first, guint32 second) {
    key_count = 0;
    assert(send_event(v, (rcu_event){.kind = 3, .a = codepoint}) == 1);
    assert(key_count == (second ? 4u : 2u));
    assert(keys[0].keysym == first && keys[1].keysym == first);
    for (unsigned i = 0; i < key_count; i++)
        assert(keys[i].down == (i % 2 == 0));
    if (second)
        assert(keys[2].keysym == second && keys[3].keysym == second);
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
    for (int apple_server = 0; apple_server < 2; apple_server++) {
        vnc_state *v = apple_server ? &apple : &standard;
        text(v, 'A', 'A', 0);
        text(v, 0xe9, 0xe9, 0);
        text(v, 0x4e2d, 0x01004e2d, 0);
        text(v, 0xffff, 0x0100ffff, 0);
    }
    text(&apple, 0x1f600, 0x0100d83d, 0x0100de00);
    text(&apple, 0x20000, 0x0100d840, 0x0100dc00);
    text(&apple, 0x10000, 0x0100d800, 0x0100dc00);
    text(&apple, 0x10ffff, 0x0100dbff, 0x0100dfff);
    text(&standard, 0x1f600, 0x0101f600, 0);
    text(&standard, 0x20000, 0x01020000, 0);
    text(&standard, 0x10000, 0x01010000, 0);
    text(&standard, 0x10ffff, 0x0110ffff, 0);
    // A transport failure partway through the pair must not report success.
    for (fail_key = 1; fail_key <= 4; fail_key++) {
        key_count = 0;
        assert(send_event(&apple, (rcu_event){.kind = 3, .a = 0x1f600}) == RCU_LOST);
        assert(key_count == fail_key);
    }
    puts("PASS: ARD text uses UTF-16 pairs, standard VNC uses scalar keysyms; wheel guards hold");
    return 0;
}

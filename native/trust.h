#ifndef RCU_TRUST_H
#define RCU_TRUST_H
#include <openssl/x509.h>
int rcu_system_trust(X509 *cert, STACK_OF(X509) *chain, const char *host);
#endif

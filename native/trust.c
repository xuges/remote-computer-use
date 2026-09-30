#include "trust.h"
#include <stdlib.h>
#include <openssl/x509v3.h>

#if defined(__APPLE__)
#include <Security/Security.h>
static SecCertificateRef apple_cert(X509 *cert) {
    unsigned char *der = NULL;
    int len = i2d_X509(cert, &der);
    if (len <= 0) return NULL;
    CFDataRef data = CFDataCreate(NULL, der, len);
    OPENSSL_free(der);
    SecCertificateRef result = data ? SecCertificateCreateWithData(NULL, data) : NULL;
    if (data) CFRelease(data);
    return result;
}
int rcu_system_trust(X509 *cert, STACK_OF(X509) *chain, const char *host) {
    int valid = 0;
    CFMutableArrayRef certificates = CFArrayCreateMutable(NULL, 0, &kCFTypeArrayCallBacks);
    CFStringRef name = CFStringCreateWithCString(NULL, host, kCFStringEncodingUTF8);
    SecPolicyRef policy = name ? SecPolicyCreateSSL(true, name) : NULL;
    SecTrustRef trust = NULL;
    if (!certificates || !policy) goto done;
    for (int i = -1; i < sk_X509_num(chain); i++) {
        SecCertificateRef c = apple_cert(i < 0 ? cert : sk_X509_value(chain, i));
        if (!c) goto done;
        CFArrayAppendValue(certificates, c);
        CFRelease(c);
    }
    if (SecTrustCreateWithCertificates(certificates, policy, &trust) == errSecSuccess)
        valid = SecTrustEvaluateWithError(trust, NULL);
done:
    if (trust) CFRelease(trust);
    if (policy) CFRelease(policy);
    if (name) CFRelease(name);
    if (certificates) CFRelease(certificates);
    return valid;
}
#elif defined(_WIN32)
#include <windows.h>
#include <wincrypt.h>
static PCCERT_CONTEXT windows_cert(HCERTSTORE store, X509 *cert) {
    unsigned char *der = NULL;
    int len = i2d_X509(cert, &der);
    if (len <= 0) return NULL;
    PCCERT_CONTEXT out = NULL;
    (void)CertAddEncodedCertificateToStore(store, X509_ASN_ENCODING, der, len, CERT_STORE_ADD_ALWAYS, &out);
    OPENSSL_free(der);
    return out;
}
int rcu_system_trust(X509 *cert, STACK_OF(X509) *chain, const char *host) {
    int valid = 0;
    HCERTSTORE store = CertOpenStore(CERT_STORE_PROV_MEMORY, 0, 0, 0, NULL);
    PCCERT_CONTEXT leaf = NULL;
    PCCERT_CHAIN_CONTEXT built = NULL;
    int n = MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, host, -1, NULL, 0);
    WCHAR *name = n ? calloc(n, sizeof(WCHAR)) : NULL;
    if (!store || !name || !MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, host, -1, name, n)) goto done;
    leaf = windows_cert(store, cert);
    if (!leaf) goto done;
    for (int i = 0; i < sk_X509_num(chain); i++) {
        PCCERT_CONTEXT c = windows_cert(store, sk_X509_value(chain, i));
        if (!c) goto done;
        CertFreeCertificateContext(c);
    }
    CERT_CHAIN_PARA parameters = {0};
    parameters.cbSize = sizeof(parameters);
    if (!CertGetCertificateChain(NULL, leaf, NULL, store, &parameters, 0, NULL, &built)) goto done;
    SSL_EXTRA_CERT_CHAIN_POLICY_PARA ssl = {0};
    ssl.cbSize = sizeof(ssl);
    ssl.dwAuthType = AUTHTYPE_SERVER;
    ssl.pwszServerName = name;
    CERT_CHAIN_POLICY_PARA policy = {0};
    policy.cbSize = sizeof(policy);
    policy.pvExtraPolicyPara = &ssl;
    CERT_CHAIN_POLICY_STATUS status = {0};
    status.cbSize = sizeof(status);
    valid = CertVerifyCertificateChainPolicy(CERT_CHAIN_POLICY_SSL, built, &policy, &status) && status.dwError == 0;
done:
    if (built) CertFreeCertificateChain(built);
    if (leaf) CertFreeCertificateContext(leaf);
    if (store) CertCloseStore(store, 0);
    free(name);
    return valid;
}
#else
#include <arpa/inet.h>
int rcu_system_trust(X509 *cert, STACK_OF(X509) *chain, const char *host) {
    int valid = 0;
    X509_STORE *store = X509_STORE_new();
    X509_STORE_CTX *ctx = X509_STORE_CTX_new();
    if (store && ctx && X509_STORE_set_default_paths(store) && X509_STORE_CTX_init(ctx, store, cert, chain)) {
        X509_VERIFY_PARAM *p = X509_STORE_CTX_get0_param(ctx);
        unsigned char ip[16];
        int numeric = inet_pton(AF_INET, host, ip) == 1 || inet_pton(AF_INET6, host, ip) == 1;
        int named = numeric ? X509_VERIFY_PARAM_set1_ip_asc(p, host) : X509_VERIFY_PARAM_set1_host(p, host, 0);
        X509_VERIFY_PARAM_set_purpose(p, X509_PURPOSE_SSL_SERVER);
        if (named) valid = X509_verify_cert(ctx) == 1;
    }
    X509_STORE_CTX_free(ctx);
    X509_STORE_free(store);
    return valid;
}
#endif

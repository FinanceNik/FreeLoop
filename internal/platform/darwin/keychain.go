//go:build darwin

// Package darwin holds every piece of macOS-only code (NFR-XPLAT-002).
package darwin

/*
#cgo LDFLAGS: -framework CoreFoundation -framework Security
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>
#include <string.h>

static CFStringRef fl_str(const char *p, long n) {
	return CFStringCreateWithBytes(kCFAllocatorDefault, (const UInt8 *)p, n, kCFStringEncodingUTF8, false);
}

// fl_query builds a generic-password query for service and account. The
// caller releases it.
static CFMutableDictionaryRef fl_query(const char *svc, long svcn, const char *acc, long accn) {
	CFMutableDictionaryRef q = CFDictionaryCreateMutable(kCFAllocatorDefault, 0,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFStringRef s = fl_str(svc, svcn);
	CFStringRef a = fl_str(acc, accn);
	CFDictionarySetValue(q, kSecClass, kSecClassGenericPassword);
	CFDictionarySetValue(q, kSecAttrService, s);
	CFDictionarySetValue(q, kSecAttrAccount, a);
	CFRelease(s);
	CFRelease(a);
	return q;
}

static OSStatus fl_set(const char *svc, long svcn, const char *acc, long accn, const UInt8 *val, long valn) {
	CFMutableDictionaryRef q = fl_query(svc, svcn, acc, accn);
	CFDataRef d = CFDataCreate(kCFAllocatorDefault, val, valn);
	CFMutableDictionaryRef upd = CFDictionaryCreateMutable(kCFAllocatorDefault, 0,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFDictionarySetValue(upd, kSecValueData, d);
	OSStatus st = SecItemUpdate(q, upd);
	if (st == errSecItemNotFound) {
		CFDictionarySetValue(q, kSecValueData, d);
		CFDictionarySetValue(q, kSecAttrAccessible, kSecAttrAccessibleWhenUnlocked);
		st = SecItemAdd(q, NULL);
	}
	CFRelease(upd);
	CFRelease(d);
	CFRelease(q);
	return st;
}

// fl_get stores the secret in *out; the caller releases it.
static OSStatus fl_get(const char *svc, long svcn, const char *acc, long accn, CFDataRef *out) {
	CFMutableDictionaryRef q = fl_query(svc, svcn, acc, accn);
	CFDictionarySetValue(q, kSecReturnData, kCFBooleanTrue);
	CFDictionarySetValue(q, kSecMatchLimit, kSecMatchLimitOne);
	CFTypeRef res = NULL;
	OSStatus st = SecItemCopyMatching(q, &res);
	CFRelease(q);
	*out = (CFDataRef)res;
	return st;
}

// fl_accounts stores in *out the accounts of a service's generic passwords, without their
// values, so no prompt comes of it; the caller releases it.
static OSStatus fl_accounts(const char *svc, long svcn, CFArrayRef *out) {
	CFMutableDictionaryRef q = CFDictionaryCreateMutable(kCFAllocatorDefault, 0,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFStringRef s = fl_str(svc, svcn);
	CFDictionarySetValue(q, kSecClass, kSecClassGenericPassword);
	CFDictionarySetValue(q, kSecAttrService, s);
	CFDictionarySetValue(q, kSecReturnAttributes, kCFBooleanTrue);
	CFDictionarySetValue(q, kSecMatchLimit, kSecMatchLimitAll);
	CFTypeRef res = NULL;
	OSStatus st = SecItemCopyMatching(q, &res);
	CFRelease(s);
	CFRelease(q);
	*out = (CFArrayRef)res;
	return st;
}

// fl_account returns the account of one item of what fl_accounts found, as UTF-8 in buf,
// or 0 if it does not fit.
static long fl_account(CFArrayRef items, long i, char *buf, long size) {
	CFDictionaryRef item = (CFDictionaryRef)CFArrayGetValueAtIndex(items, i);
	CFStringRef account = (CFStringRef)CFDictionaryGetValue(item, kSecAttrAccount);
	if (!account || !CFStringGetCString(account, buf, size, kCFStringEncodingUTF8)) return 0;
	return (long)strlen(buf);
}

static long fl_count(CFArrayRef items) { return (long)CFArrayGetCount(items); }

static OSStatus fl_delete(const char *svc, long svcn, const char *acc, long accn) {
	CFMutableDictionaryRef q = fl_query(svc, svcn, acc, accn);
	OSStatus st = SecItemDelete(q);
	CFRelease(q);
	return st;
}
*/
import "C"

import (
	"fmt"
	"unsafe"

	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// Keychain stores secrets as generic passwords in the user's login keychain.
type Keychain struct{}

var _ secrets.Store = Keychain{}

// cstr passes a Go string's bytes to C without copying. C must not keep the
// pointer, which none of the helpers above do.
func cstr(s string) (*C.char, C.long) {
	return (*C.char)(unsafe.Pointer(unsafe.StringData(s))), C.long(len(s))
}

func (Keychain) Get(ref secrets.Ref) (string, error) {
	svc, svcn := cstr(ref.Service)
	acc, accn := cstr(ref.Account)
	var d C.CFDataRef
	if err := status(C.fl_get(svc, svcn, acc, accn, &d), ref); err != nil {
		return "", err
	}
	defer C.CFRelease(C.CFTypeRef(d))
	return C.GoStringN((*C.char)(unsafe.Pointer(C.CFDataGetBytePtr(d))), C.int(C.CFDataGetLength(d))), nil
}

func (Keychain) Set(ref secrets.Ref, value string) error {
	svc, svcn := cstr(ref.Service)
	acc, accn := cstr(ref.Account)
	val, valn := cstr(value)
	return status(C.fl_set(svc, svcn, acc, accn, (*C.UInt8)(unsafe.Pointer(val)), valn), ref)
}

func (Keychain) Delete(ref secrets.Ref) error {
	svc, svcn := cstr(ref.Service)
	acc, accn := cstr(ref.Account)
	return status(C.fl_delete(svc, svcn, acc, accn), ref)
}

// Accounts lists the accounts held under a service, reading no secret (secrets.Lister).
func (Keychain) Accounts(service string) ([]string, error) {
	svc, svcn := cstr(service)
	var items C.CFArrayRef
	switch st := C.fl_accounts(svc, svcn, &items); st {
	case C.errSecSuccess:
	case C.errSecItemNotFound:
		return nil, nil
	default:
		return nil, fmt.Errorf("keychain: list %s: OSStatus %d", service, int(st))
	}
	defer C.CFRelease(C.CFTypeRef(items))
	var buf [4096]C.char
	var out []string
	for i := C.long(0); i < C.fl_count(items); i++ {
		// An account that cannot be read as text cannot be one of the app's references.
		if n := C.fl_account(items, i, &buf[0], C.long(len(buf))); n > 0 {
			out = append(out, C.GoStringN(&buf[0], C.int(n)))
		}
	}
	return out, nil
}

var _ secrets.Lister = Keychain{}

func status(st C.OSStatus, ref secrets.Ref) error {
	switch st {
	case C.errSecSuccess:
		return nil
	case C.errSecItemNotFound:
		return secrets.ErrNotFound
	default:
		return fmt.Errorf("keychain: %s: OSStatus %d", ref, int(st))
	}
}

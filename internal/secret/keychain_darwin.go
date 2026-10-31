package secret

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// The keychain keeps each secret as a generic password of this service,
// its name the account.
const service = "io.github.aronnaxlin.mygoify"

// Keychain keeps secrets in the listener's login keychain, through the
// Security framework: the secret stays in memory, never in an argument
// as with the security command (POC.md P9).
type Keychain struct{}

// The Security framework's status codes this package tells apart.
const (
	errSecItemNotFound = -25300
)

// cf is what the keychain calls of Core Foundation and Security, loaded
// as it is first used.
var cf struct {
	once sync.Once
	err  error

	release        func(ref uintptr)
	stringCreate   func(alloc uintptr, s string, encoding uint32) uintptr
	dataCreate     func(alloc uintptr, b *byte, n int) uintptr
	dataLength     func(data uintptr) int
	dataBytes      func(data uintptr) unsafe.Pointer
	dictCreate     func(alloc uintptr, keys, values *uintptr, n int, keyCallbacks, valueCallbacks uintptr) uintptr
	copyMatching   func(query uintptr, result *uintptr) int32
	add            func(attrs uintptr, result *uintptr) int32
	update         func(query, attrs uintptr) int32
	deleteItem     func(query uintptr) int32
	keyCallbacks   uintptr
	valueCallbacks uintptr

	class, genericPassword, service, account, valueData, returnData, matchLimit, matchLimitOne, yes uintptr
}

const utf8 = 0x08000100 // kCFStringEncodingUTF8

func loadCF() error {
	cf.once.Do(func() {
		core, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			cf.err = err
			return
		}
		sec, err := purego.Dlopen("/System/Library/Frameworks/Security.framework/Security", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			cf.err = err
			return
		}
		purego.RegisterLibFunc(&cf.release, core, "CFRelease")
		purego.RegisterLibFunc(&cf.stringCreate, core, "CFStringCreateWithCString")
		purego.RegisterLibFunc(&cf.dataCreate, core, "CFDataCreate")
		purego.RegisterLibFunc(&cf.dataLength, core, "CFDataGetLength")
		purego.RegisterLibFunc(&cf.dataBytes, core, "CFDataGetBytePtr")
		purego.RegisterLibFunc(&cf.dictCreate, core, "CFDictionaryCreate")
		purego.RegisterLibFunc(&cf.copyMatching, sec, "SecItemCopyMatching")
		purego.RegisterLibFunc(&cf.add, sec, "SecItemAdd")
		purego.RegisterLibFunc(&cf.update, sec, "SecItemUpdate")
		purego.RegisterLibFunc(&cf.deleteItem, sec, "SecItemDelete")
		// The callbacks are passed as their address; the constants are
		// read from theirs.
		for _, s := range []struct {
			v    *uintptr
			lib  uintptr
			name string
			addr bool
		}{
			{&cf.keyCallbacks, core, "kCFTypeDictionaryKeyCallBacks", true},
			{&cf.valueCallbacks, core, "kCFTypeDictionaryValueCallBacks", true},
			{&cf.yes, core, "kCFBooleanTrue", false},
			{&cf.class, sec, "kSecClass", false},
			{&cf.genericPassword, sec, "kSecClassGenericPassword", false},
			{&cf.service, sec, "kSecAttrService", false},
			{&cf.account, sec, "kSecAttrAccount", false},
			{&cf.valueData, sec, "kSecValueData", false},
			{&cf.returnData, sec, "kSecReturnData", false},
			{&cf.matchLimit, sec, "kSecMatchLimit", false},
			{&cf.matchLimitOne, sec, "kSecMatchLimitOne", false},
		} {
			p, err := purego.Dlsym(s.lib, s.name)
			if err != nil {
				cf.err = err
				return
			}
			if s.addr {
				*s.v = p
			} else {
				*s.v = **(**uintptr)(unsafe.Pointer(&p))
			}
		}
	})
	return cf.err
}

// query is a dictionary of the item named name, with the pairs of kv
// added; release frees what it made.
func query(name string, kv ...uintptr) (dict uintptr, release func()) {
	svc := cf.stringCreate(0, service, utf8)
	acct := cf.stringCreate(0, name, utf8)
	keys := []uintptr{cf.class, cf.service, cf.account}
	values := []uintptr{cf.genericPassword, svc, acct}
	for i := 0; i+1 < len(kv); i += 2 {
		keys = append(keys, kv[i])
		values = append(values, kv[i+1])
	}
	dict = cf.dictCreate(0, &keys[0], &values[0], len(keys), cf.keyCallbacks, cf.valueCallbacks)
	return dict, func() {
		cf.release(dict)
		cf.release(acct)
		cf.release(svc)
	}
}

func data(v []byte) uintptr {
	var p *byte
	if len(v) > 0 {
		p = &v[0]
	}
	return cf.dataCreate(0, p, len(v))
}

func statusError(op string, status int32) error {
	return fmt.Errorf("secret: keychain %s: OSStatus %d", op, status)
}

func (Keychain) Get(name string) ([]byte, error) {
	if err := loadCF(); err != nil {
		return nil, err
	}
	q, release := query(name, cf.returnData, cf.yes, cf.matchLimit, cf.matchLimitOne)
	defer release()
	var res uintptr
	switch status := cf.copyMatching(q, &res); status {
	case 0:
	case errSecItemNotFound:
		return nil, ErrNotFound
	default:
		return nil, statusError("read", status)
	}
	defer cf.release(res)
	n := cf.dataLength(res)
	if n == 0 {
		return []byte{}, nil
	}
	return append([]byte(nil), unsafe.Slice((*byte)(cf.dataBytes(res)), n)...), nil
}

func (Keychain) Set(name string, v []byte) error {
	if err := loadCF(); err != nil {
		return err
	}
	d := data(v)
	defer cf.release(d)
	q, release := query(name)
	defer release()
	vk, vv := []uintptr{cf.valueData}, []uintptr{d}
	attrs := cf.dictCreate(0, &vk[0], &vv[0], 1, cf.keyCallbacks, cf.valueCallbacks)
	defer cf.release(attrs)
	switch status := cf.update(q, attrs); status {
	case 0:
		return nil
	case errSecItemNotFound:
	default:
		return statusError("update", status)
	}
	item, releaseItem := query(name, cf.valueData, d)
	defer releaseItem()
	if status := cf.add(item, nil); status != 0 {
		return statusError("add", status)
	}
	return nil
}

func (Keychain) Delete(name string) error {
	if err := loadCF(); err != nil {
		return err
	}
	q, release := query(name)
	defer release()
	if status := cf.deleteItem(q); status != 0 && status != errSecItemNotFound {
		return statusError("delete", status)
	}
	return nil
}

func system() (Store, error) { return Keychain{}, loadCF() }

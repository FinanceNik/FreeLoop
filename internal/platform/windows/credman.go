//go:build windows

package windows

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// Credential Manager is Windows' keyring: each secret is a generic credential of the
// signed-in user, protected by Windows with the user's password (DPAPI), as the macOS
// Keychain and the Secret Service keep theirs. A reference's service and account name
// the credential "<service>/<account>". Nothing is ever kept in a file of the app's own.
//
// A credential holds at most 2,560 bytes, less than a vault of several keys and a
// sign-in token or two, so a longer value is split over numbered parts (parts).

// CredentialManager is a secrets.Store over the Windows Credential Manager.
type CredentialManager struct{}

var (
	_ secrets.Store  = CredentialManager{}
	_ secrets.Lister = CredentialManager{}
)

var (
	advapi32          = windows.NewLazySystemDLL("advapi32.dll")
	procCredReadW     = advapi32.NewProc("CredReadW")
	procCredWriteW    = advapi32.NewProc("CredWriteW")
	procCredDeleteW   = advapi32.NewProc("CredDeleteW")
	procCredEnumerate = advapi32.NewProc("CredEnumerateW")
	procCredFree      = advapi32.NewProc("CredFree")
)

const (
	credTypeGeneric         = 1
	credPersistLocalMachine = 2
)

// credential is CREDENTIALW.
type credential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

func target(ref secrets.Ref) string { return ref.Service + "/" + ref.Account }

// read returns a credential's blob and comment.
func read(name string) ([]byte, string, error) {
	n, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, "", err
	}
	var c *credential
	if r, _, err := procCredReadW.Call(uintptr(unsafe.Pointer(n)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&c))); r == 0 {
		if errors.Is(err, windows.ERROR_NOT_FOUND) {
			return nil, "", secrets.ErrNotFound
		}
		return nil, "", fmt.Errorf("credential manager: read %s: %w", name, err)
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(c)))
	blob := append([]byte(nil), unsafe.Slice(c.CredentialBlob, c.CredentialBlobSize)...)
	comment := ""
	if c.Comment != nil {
		comment = windows.UTF16PtrToString(c.Comment)
	}
	return blob, comment, nil
}

func write(name, account, comment string, blob []byte) error {
	n, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	u, err := windows.UTF16PtrFromString(account)
	if err != nil {
		return err
	}
	cm, err := windows.UTF16PtrFromString(comment)
	if err != nil {
		return err
	}
	c := credential{Type: credTypeGeneric, TargetName: n, Comment: cm, UserName: u, Persist: credPersistLocalMachine, CredentialBlobSize: uint32(len(blob))}
	if len(blob) > 0 {
		c.CredentialBlob = &blob[0]
	}
	if r, _, err := procCredWriteW.Call(uintptr(unsafe.Pointer(&c)), 0); r == 0 {
		return fmt.Errorf("credential manager: write %s: %w", name, err)
	}
	return nil
}

func remove(name string) error {
	n, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	if r, _, err := procCredDeleteW.Call(uintptr(unsafe.Pointer(n)), credTypeGeneric, 0); r == 0 {
		if errors.Is(err, windows.ERROR_NOT_FOUND) {
			return secrets.ErrNotFound
		}
		return fmt.Errorf("credential manager: delete %s: %w", name, err)
	}
	return nil
}

func (CredentialManager) Get(ref secrets.Ref) (string, error) {
	name := target(ref)
	first, comment, err := read(name)
	if err != nil {
		return "", err
	}
	count := partCount(comment)
	chunks := [][]byte{first}
	for i := 2; i <= count; i++ {
		b, _, err := read(partName(name, i))
		if err != nil {
			return "", fmt.Errorf("credential manager: %s: part %d of %d: %w", name, i, count, err)
		}
		chunks = append(chunks, b)
	}
	return string(join(chunks)), nil
}

// Set writes the parts after the first before the first, which says how many there
// are, so a reader never sees a count whose parts are not there yet; parts left from a
// longer value are removed after.
func (CredentialManager) Set(ref secrets.Ref, value string) error {
	name := target(ref)
	_, oldComment, err := read(name)
	if err != nil && !errors.Is(err, secrets.ErrNotFound) {
		return err
	}
	chunks := split([]byte(value))
	for i := len(chunks); i >= 2; i-- {
		if err := write(partName(name, i), ref.Account, partComment(0), chunks[i-1]); err != nil {
			return err
		}
	}
	if err := write(name, ref.Account, partComment(len(chunks)), chunks[0]); err != nil {
		return err
	}
	for i := len(chunks) + 1; i <= partCount(oldComment); i++ {
		remove(partName(name, i))
	}
	return nil
}

func (CredentialManager) Delete(ref secrets.Ref) error {
	name := target(ref)
	_, comment, err := read(name)
	if err != nil {
		return err
	}
	if err := remove(name); err != nil {
		return err
	}
	for i := 2; i <= partCount(comment); i++ {
		remove(partName(name, i))
	}
	return nil
}

// Accounts lists the accounts held under a service, reading no secret (secrets.Lister).
func (CredentialManager) Accounts(service string) ([]string, error) {
	filter, err := windows.UTF16PtrFromString(service + "/*")
	if err != nil {
		return nil, err
	}
	var count uint32
	var list **credential
	if r, _, err := procCredEnumerate.Call(uintptr(unsafe.Pointer(filter)), 0, uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&list))); r == 0 {
		if errors.Is(err, windows.ERROR_NOT_FOUND) {
			return nil, nil
		}
		return nil, fmt.Errorf("credential manager: list %s: %w", service, err)
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(list)))
	var accounts []string
	for _, c := range unsafe.Slice(list, count) {
		name := windows.UTF16PtrToString(c.TargetName)
		account, ok := strings.CutPrefix(name, service+"/")
		if !ok || isPart(name) {
			continue
		}
		accounts = append(accounts, account)
	}
	return accounts, nil
}

// partComment is the comment of a credential's first part, which says how many there
// are; 0 is a part after the first.
func partComment(n int) string {
	if n == 0 {
		return "Freeloop (part)"
	}
	return "Freeloop; parts=" + strconv.Itoa(n)
}

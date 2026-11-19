//go:build windows

package config

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// validateFileOwner uses the native security descriptor rather than Unix mode
// descriptor. A missing or null DACL is rejected because it would make the file
// broadly accessible; any non-owner, non-privileged ACE granting write-like
// access is rejected. LocalSystem and built-in Administrators are trusted OS
// principals, consistent with the boundary not defending against administrator
// or SYSTEM processes.
func validateFileOwner(path string, _ os.FileInfo) error {
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("read configuration security descriptor: %w", err)
	}
	if descriptor == nil {
		return errors.New("configuration security descriptor is missing")
	}
	owner, _, err := descriptor.Owner()
	if err != nil || owner == nil {
		return fmt.Errorf("read configuration owner: %w", err)
	}
	current := windows.GetCurrentProcessToken()
	user, err := current.GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil || !windows.EqualSid(owner, user.User.Sid) {
		return errors.New("configuration file is not owned by the current user")
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return fmt.Errorf("read configuration DACL: %w", err)
	}
	if dacl == nil {
		return errors.New("configuration DACL is absent or unrestricted")
	}
	if err := rejectNonOwnerWriteACE(dacl, owner); err != nil {
		return err
	}
	return nil
}

func rejectNonOwnerWriteACE(acl *windows.ACL, owner *windows.SID) error {
	getAce := windows.NewLazySystemDLL("advapi32.dll").NewProc("GetAce")
	const writeLike = uint32(windows.FILE_GENERIC_WRITE | windows.DELETE | windows.WRITE_DAC | windows.WRITE_OWNER)
	for index := uint32(0); index < uint32(acl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		result, _, callErr := getAce.Call(uintptr(unsafe.Pointer(acl)), uintptr(index), uintptr(unsafe.Pointer(&ace)))
		if result == 0 {
			if callErr == syscall.Errno(0) {
				callErr = windows.ERROR_INVALID_ACL
			}
			return fmt.Errorf("inspect configuration ACL entry: %w", callErr)
		}
		if ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || uint32(ace.Mask)&writeLike == 0 {
			continue
		}
		aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !windows.EqualSid(aceSID, owner) && !isTrustedWritePrincipal(aceSID) {
			return errors.New("configuration ACL grants write access beyond the owner")
		}
	}
	return nil
}

func isTrustedWritePrincipal(sid *windows.SID) bool {
	if sid == nil {
		return false
	}
	text := sid.String()
	return text == "S-1-5-18" || text == "S-1-5-32-544"
}

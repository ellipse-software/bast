//go:build windows

package platform

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func ReplaceFile(staged, destination string) error {
	return windows.Rename(staged, destination)
}

// Windows does not expose a portable directory fsync operation. Individual
// files are flushed before replacement, and MoveFileEx performs the replace.
func SyncDirectory(string) error { return nil }

func SecurePath(path string, _ os.FileMode) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	var inheritance uint32
	if info.IsDir() {
		inheritance = windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	system, err := windows.StringToSid("S-1-5-18")
	if err != nil {
		return err
	}
	administrators, err := windows.StringToSid("S-1-5-32-544")
	if err != nil {
		return err
	}
	entries := []windows.EXPLICIT_ACCESS{
		allowFullControl(user.User.Sid, windows.TRUSTEE_IS_USER, inheritance),
		allowFullControl(system, windows.TRUSTEE_IS_USER, inheritance),
		allowFullControl(administrators, windows.TRUSTEE_IS_GROUP, inheritance),
	}
	acl, err := windows.ACLFromEntries(entries, nil)
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		acl,
		nil,
	)
}

func allowFullControl(sid *windows.SID, trusteeType windows.TRUSTEE_TYPE, inheritance uint32) windows.EXPLICIT_ACCESS {
	return windows.EXPLICIT_ACCESS{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       inheritance,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  trusteeType,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}
}

func SupportsPOSIXPermissions() bool { return false }

func SamePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func PathContained(parent, child string) bool {
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)
	if strings.EqualFold(parent, child) {
		return true
	}
	if !strings.EqualFold(filepath.VolumeName(parent), filepath.VolumeName(child)) {
		return false
	}
	if !strings.HasSuffix(parent, `\`) {
		parent += `\`
	}
	return len(child) >= len(parent) && strings.EqualFold(child[:len(parent)], parent)
}

func HasPathSeparator(value string) bool {
	return strings.ContainsAny(value, `/\`)
}

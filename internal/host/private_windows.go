package host

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"unsafe"
)

func mkdirPrivate(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	s, err := SecurityDescriptor(true)
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString(s)
	if err != nil {
		return err
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	sa := windows.SecurityAttributes{SecurityDescriptor: sd}
	sa.Length = uint32(unsafe.Sizeof(sa))
	err = windows.CreateDirectory(p, &sa)
	if err == windows.ERROR_ALREADY_EXISTS {
		return nil
	}
	return err
}

func UserSID() (string, error) {
	t, e := windows.GetCurrentProcessToken().GetTokenUser()
	if e != nil {
		return "", e
	}
	return t.User.Sid.String(), nil
}
func SecurityDescriptor(directory bool) (string, error) {
	sid, e := UserSID()
	if e != nil {
		return "", e
	}
	inherit := ""
	if directory {
		inherit = "OICI"
	}
	return "O:" + sid + "D:P(A;" + inherit + ";FA;;;" + sid + ")(A;" + inherit + ";FA;;;SY)(A;" + inherit + ";FA;;;BA)", nil
}
func Protect(path string, directory bool) error {
	s, e := SecurityDescriptor(directory)
	if e != nil {
		return e
	}
	sd, e := windows.SecurityDescriptorFromString(s)
	if e != nil {
		return e
	}
	acl, _, e := sd.DACL()
	if e != nil {
		return e
	}
	owner, _, e := sd.Owner()
	if e != nil {
		return e
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, owner, nil, acl, nil)
}
func CheckPrivate(path string, directory bool) error {
	st, e := os.Lstat(path)
	if e != nil {
		return e
	}
	if st.IsDir() != directory || (!directory && !st.Mode().IsRegular()) || st.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		return errors.New("unsafe file type")
	}
	p, e := windows.UTF16PtrFromString(path)
	if e != nil {
		return e
	}
	attrs, e := windows.GetFileAttributes(p)
	if e != nil {
		return e
	}
	if attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("reparse point")
	}
	sd, e := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.OWNER_SECURITY_INFORMATION)
	if e != nil {
		return e
	}
	sid, e := UserSID()
	if e != nil {
		return e
	}
	owner, _, e := sd.Owner()
	if e != nil || owner == nil || owner.String() != sid {
		return errors.New("unexpected owner")
	}
	acl, _, e := sd.DACL()
	if e != nil || acl == nil {
		return errors.New("missing ACL")
	}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if e = windows.GetAce(acl, i, &ace); e != nil {
			return e
		}
		if ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 {
			continue
		}
		if ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return errors.New("unsupported ACL entry")
		}
		principal := (*windows.SID)(unsafe.Pointer(&ace.SidStart)).String()
		if ace.Mask != 0 && principal != sid && principal != "S-1-5-18" && principal != "S-1-5-32-544" {
			return errors.New("file accessible to other users")
		}
	}
	return nil
}

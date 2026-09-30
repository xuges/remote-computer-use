package host

import (
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateACLRejectsOtherUsers(t *testing.T) {
	p := filepath.Join(t.TempDir(), "private.txt")
	if e := os.WriteFile(p, []byte("fixture"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := Protect(p, false); e != nil {
		t.Fatal(e)
	}
	if e := CheckPrivate(p, false); e != nil {
		t.Fatal(e)
	}
	sd, e := windows.SecurityDescriptorFromString("D:P(A;;FA;;;WD)")
	if e != nil {
		t.Fatal(e)
	}
	acl, _, _ := sd.DACL()
	if e = windows.SetNamedSecurityInfo(p, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); e != nil {
		t.Fatal(e)
	}
	if e = CheckPrivate(p, false); e == nil {
		t.Fatal("accepted Everyone access")
	}
}

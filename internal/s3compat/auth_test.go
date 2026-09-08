package s3compat

import "testing"

func TestPermissionsAreIndependent(t *testing.T) {
	if !permissionOK("read,delete", "read") || !permissionOK("read,delete", "delete") {
		t.Fatal("explicit permissions were not granted")
	}
	if permissionOK("manage", "read") || permissionOK("read", "write") {
		t.Fatal("one permission incorrectly implied another")
	}
}

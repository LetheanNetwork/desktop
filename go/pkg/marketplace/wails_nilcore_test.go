// SPDX-Licence-Identifier: EUPL-1.2

package marketplace

import (
	"testing"
)

// A Service built without a Core fails gracefully like every other method in
// this package — it must never panic inside ServiceFor. Found by the coverage
// unit that had to route its tests around the panic.
func TestService_Installed_Ugly_NilCore(t *testing.T) {
	r := NewService(nil).Installed()
	if !r.OK {
		t.Fatalf("Installed with nil core should be an empty Ok, got fail: %v", r.Err())
	}
}

func TestService_InstallPlugin_Ugly_NilCore(t *testing.T) {
	r := NewService(nil).InstallPlugin("anything")
	if r.OK {
		t.Fatal("InstallPlugin with nil core must fail, not succeed")
	}
}

func TestService_Remove_Ugly_NilCore(t *testing.T) {
	r := NewService(nil).Remove("anything")
	if r.OK {
		t.Fatal("Remove with nil core must fail, not succeed")
	}
}

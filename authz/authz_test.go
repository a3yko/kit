package authz

import "testing"

type role int

const (
	viewer role = iota
	manager
	owner
)

const (
	view    Permission = "vehicles.view"
	edit    Permission = "vehicles.edit"
	destroy Permission = "vehicles.destroy"
)

func roles() Roles[role] {
	return NewRoles[role]().
		Grant(viewer, view).
		Grant(manager, view, edit).
		Grant(owner, view, edit, destroy)
}

func TestCan(t *testing.T) {
	r := roles()
	if !r.Can(manager, edit) {
		t.Error("manager should edit")
	}
	if r.Can(viewer, edit) {
		t.Error("viewer should not edit")
	}
	if r.Can(manager, destroy) {
		t.Error("manager should not destroy")
	}
	if !r.Can(owner, destroy) {
		t.Error("owner should destroy")
	}
}

func TestCanAnyAll(t *testing.T) {
	r := roles()
	if !r.CanAny(viewer, edit, view) {
		t.Error("viewer has view, so CanAny(edit,view) should be true")
	}
	if r.CanAll(manager, view, edit, destroy) {
		t.Error("manager lacks destroy, so CanAll should be false")
	}
	if !r.CanAll(owner, view, edit, destroy) {
		t.Error("owner has all three")
	}
}

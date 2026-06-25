// Package authz is a tiny role-based authorization core: declare which
// permissions each role grants, then ask whether a role has a permission. It is
// generic over your role type, so use your own role enum or string.
//
// Resource-scoped rules ("may edit THIS record because they own it") are domain
// logic and stay in your app — compose them with a permission check:
//
//	const EditVehicle authz.Permission = "vehicles.edit"
//
//	roles := authz.NewRoles[Role]().
//		Grant(Owner, EditVehicle, DeleteVehicle).
//		Grant(Manager, EditVehicle)
//
//	if roles.Can(user.Role, EditVehicle) && vehicle.TenantID == user.TenantID {
//		// allowed
//	}
package authz

// Permission is a named capability, e.g. "vehicles.edit".
type Permission string

// Roles maps each role to the set of permissions it grants. Build it with
// NewRoles().Grant(...). The zero value is not usable; use NewRoles.
type Roles[R comparable] map[R]map[Permission]bool

// NewRoles returns an empty role set.
func NewRoles[R comparable]() Roles[R] { return make(Roles[R]) }

// Grant adds permissions to a role and returns the receiver, so calls chain.
func (roles Roles[R]) Grant(role R, perms ...Permission) Roles[R] {
	set := roles[role]
	if set == nil {
		set = make(map[Permission]bool, len(perms))
		roles[role] = set
	}
	for _, p := range perms {
		set[p] = true
	}
	return roles
}

// Can reports whether role grants p.
func (roles Roles[R]) Can(role R, p Permission) bool {
	return roles[role][p]
}

// CanAny reports whether role grants at least one of perms.
func (roles Roles[R]) CanAny(role R, perms ...Permission) bool {
	for _, p := range perms {
		if roles[role][p] {
			return true
		}
	}
	return false
}

// CanAll reports whether role grants every one of perms.
func (roles Roles[R]) CanAll(role R, perms ...Permission) bool {
	for _, p := range perms {
		if !roles[role][p] {
			return false
		}
	}
	return true
}

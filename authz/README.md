# authz

A tiny **role → permission** authorization core. Declare which permissions each
role grants, then ask whether a role has a permission. Generic over your role
type, so use your own role enum or string.

## Install

```sh
go get github.com/a3yko/kit/authz
```

## Usage

```go
type Role int
const (
    Viewer Role = iota
    Manager
    Owner
)

const (
    ViewVehicle   authz.Permission = "vehicles.view"
    EditVehicle   authz.Permission = "vehicles.edit"
    DeleteVehicle authz.Permission = "vehicles.delete"
)

// Build once at startup.
var roles = authz.NewRoles[Role]().
    Grant(Viewer,  ViewVehicle).
    Grant(Manager, ViewVehicle, EditVehicle).
    Grant(Owner,   ViewVehicle, EditVehicle, DeleteVehicle)

// Check.
roles.Can(user.Role, EditVehicle)                 // bool
roles.CanAny(user.Role, EditVehicle, ViewVehicle) // any of
roles.CanAll(user.Role, ViewVehicle, EditVehicle) // all of
```

## Resource-scoped rules stay in your app

Ownership / tenant checks are domain logic, not part of this package — **compose**
them with a permission check:

```go
func canEdit(roles authz.Roles[Role], u User, v Vehicle) bool {
    return roles.Can(u.Role, EditVehicle) && v.TenantID == u.TenantID
}
```

That keeps `authz` a small, predictable capability table while your domain owns
the "whose record is it" decisions.

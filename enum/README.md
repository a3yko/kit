# enum

A closed set of string values that **cannot be forged**.

Go has no sum types. The usual stand-in enforces nothing:

```go
type Status string
const Active Status = "active"

Status("shipped")      // compiles
Status(row.Status)     // compiles, admits whatever the database holds
```

`enum` borrows two ideas from ML-family languages — **abstract types** (the
representation is hidden, so only the owning module can produce a value) and
**phantom types** (a type parameter carrying no data, purely to stop two
otherwise-identical types being interchangeable).

## Install

```sh
go get github.com/a3yko/kit/enum
```

## Usage

```go
type statusTag struct{}                    // unexported: only you can name it
type Status = enum.Value[statusTag]        // abstract; no literal produces one

var Statuses = enum.New[statusTag]("stub", "draft", "active")

var (
    Stub   = Statuses.MustParse("stub")    // panics at init if misspelled
    Draft  = Statuses.MustParse("draft")
    Active = Statuses.MustParse("active")
)
```

At every boundary where an untrusted string arrives — a database column, a URL
parameter, a JSON body — you have to parse:

```go
status, err := Statuses.Parse(row.Status)
```

That is the point. **Parse, don't validate**: past that line the value is known
good by construction and nothing downstream re-checks it.

`Value[a]` and `Value[b]` are different types, so a product status cannot be
passed where an order status is expected. A named string type cannot do that.

## Exhaustiveness

Go cannot check that a switch covers every case, and no library can add that.
`All()` lets a test stand in for the compiler:

```go
func TestEveryStatusHasRules(t *testing.T) {
    for _, s := range catalog.Statuses.All() {
        if _, ok := catalog.RulesFor(s); !ok {
            t.Errorf("%s has no rules - add it to the table", s)
        }
    }
}
```

Weaker than a compiler. Much better than nothing, and it fails the moment
somebody adds a member without handling it.

## Keeping SQL and Go in step

`SQLList()` renders the members for a CHECK constraint:

```go
fmt.Sprintf("CHECK (status IN (%s))", Statuses.SQLList())
// CHECK (status IN ('stub', 'draft', 'active'))
```

Assert it against the live constraint in a test and the two cannot drift.

## Notes

- The zero `Value` is a member of no set. An uninitialised field is detectably
  invalid rather than silently the first constant — check with `IsZero`.
- `Value` stays comparable, so it works as a map key for rules tables.
- `New` panics on duplicates or empty members. It runs at package init, where a
  panic is a failed build rather than a failed request.

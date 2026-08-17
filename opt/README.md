# opt

OCaml's `'a option`, in Go.

## Why

Go's usual answer to "this may be absent" is a pointer, or a pair of fields:

```go
Name    string
HasName bool
```

Both fail the same way: nothing stops the halves drifting apart. Set `Name`
without `HasName` and the value is invisible; set `HasName` without `Name` and
you render an empty cell as though it were data. The compiler helps with
neither, and the bug is silent.

A pointer has the same problem in costume: `*string` means "maybe absent" and
"maybe shared" and "maybe nil because someone forgot", and reading it wrong is a
panic rather than a compile error.

```go
Price opt.Optional[Money]
```

says exactly one thing, and the only honest way to read it forces you to
acknowledge absence at the point of use.

## Install

```sh
go get github.com/a3yko/kit/opt
```

## Usage

```go
price := opt.Some(money.FromCents(450))
none  := opt.None[Money]()

if v, ok := price.Get(); ok {
    fmt.Println(v)
}

price.Or(zero)   // fallback
price.OrZero()   // T's zero — only where that is genuinely sensible
```

## At the database boundary

sqlc emits `*T` for a nullable column. That is where NULL becomes an option:

```go
Name: opt.FromPtr(row.Name),          // *string  -> Optional[string]
```

and back, for a write:

```go
Name: product.Name.Ptr(),
```

`Optional` holds a copy, so `Ptr()` never aliases the value it came from.

## Notes

- The zero `Optional` is `None`, deliberately: a struct that gains an Optional
  field does not silently start claiming it has data.
- `Map` is a free function, not a method — Go methods cannot introduce a new
  type parameter.

package enum_test

import (
	"testing"

	"github.com/a3yko/kit/enum"
)

type statusTag struct{}
type reasonTag struct{}

type Status = enum.Value[statusTag]

var (
	statuses = enum.New[statusTag]("stub", "draft", "active", "discontinued", "archived")
	reasons  = enum.New[reasonTag]("receipt", "sale")

	Stub   = statuses.MustParse("stub")
	Active = statuses.MustParse("active")
)

func TestParseAcceptsMembers(t *testing.T) {
	v, err := statuses.Parse("draft")
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "draft" {
		t.Errorf("got %q", v.String())
	}
}

func TestParseRejectsEverythingElse(t *testing.T) {
	// The whole point: a value the database or a URL hands you is not a Status
	// until it has been through here.
	for _, bad := range []string{"", "shipped", "Active", "active "} {
		if _, err := statuses.Parse(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestErrorNamesTheAlternatives(t *testing.T) {
	_, err := statuses.Parse("shipped")
	if err == nil {
		t.Fatal("want an error")
	}
	// The message ends up in a log or a toast, so it has to be useful.
	if got := err.Error(); got == "" || !contains(got, "stub, draft") {
		t.Errorf("unhelpful message: %q", got)
	}
}

func TestZeroValueIsNotAMember(t *testing.T) {
	// An uninitialised field must be detectably invalid, not silently the first
	// constant.
	var s Status
	if !s.IsZero() {
		t.Error("the zero Value claims to be a member")
	}
	if s == Active || s == Stub {
		t.Error("the zero Value equals a real member")
	}
}

func TestDifferentTagsAreDifferentTypes(t *testing.T) {
	// This is the phantom-type property, and it is checked by the compiler, not
	// by this test. The test exists to document it: uncommenting the line below
	// must fail to build.
	//
	//   var s Status = reasons.MustParse("sale")
	//   ^ cannot use ... (Value[reasonTag]) as Value[statusTag]
	//
	// A plain `type Status string` would accept it happily.
	if reasons.MustParse("sale").String() != "sale" {
		t.Fatal("sanity")
	}
}

func TestSQLListMatchesTheSet(t *testing.T) {
	// So a CHECK constraint in a migration and the Go set can be asserted equal
	// rather than drifting the first time somebody edits one and not the other.
	want := "'stub', 'draft', 'active', 'discontinued', 'archived'"
	if got := statuses.SQLList(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestAllPreservesDeclarationOrder(t *testing.T) {
	// Order is usually meaningful: a lifecycle runs in one, a dropdown renders
	// in one.
	all := statuses.All()
	if len(all) != 5 || all[0] != Stub || all[2] != Active {
		t.Errorf("order not preserved: %v", statuses.Names())
	}
}

func TestDuplicateMemberPanicsAtInit(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("a duplicate member was accepted")
		}
	}()
	_ = enum.New[statusTag]("a", "a")
}

func TestValueIsUsableAsAMapKey(t *testing.T) {
	// Rules tables are keyed by state; Value must stay comparable.
	rules := map[Status]string{Stub: "placeholder", Active: "tradeable"}
	if rules[Active] != "tradeable" {
		t.Error("Value does not work as a map key")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

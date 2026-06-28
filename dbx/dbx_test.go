package dbx

import (
	"testing"

	"github.com/google/uuid"
)

func TestNullUUID(t *testing.T) {
	if NullUUID(uuid.Nil).Valid {
		t.Error("zero uuid should be invalid (NULL)")
	}
	id := uuid.New()
	v := NullUUID(id)
	if !v.Valid || v.Bytes != id {
		t.Error("non-zero uuid should be valid and carry its bytes")
	}
}

func TestPtrUUID(t *testing.T) {
	if PtrUUID(nil).Valid {
		t.Error("nil pointer should be NULL")
	}
	id := uuid.New()
	if v := PtrUUID(&id); !v.Valid || v.Bytes != id {
		t.Error("pointer should be valid and carry its bytes")
	}
}

func TestPtr(t *testing.T) {
	if Ptr("") != nil {
		t.Error(`Ptr("") should be nil`)
	}
	if Ptr(0) != nil {
		t.Error("Ptr(0) should be nil")
	}
	if p := Ptr("x"); p == nil || *p != "x" {
		t.Error(`Ptr("x") should point to "x"`)
	}
}

func TestDeref(t *testing.T) {
	if Deref[string](nil) != "" {
		t.Error("Deref(nil) should be zero value")
	}
	s := "hi"
	if Deref(&s) != "hi" {
		t.Error("Deref(&s) should be s")
	}
	if DerefOr[string](nil, "def") != "def" {
		t.Error("DerefOr(nil, def) should be def")
	}
}

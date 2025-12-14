package flux

import (
	"errors"
	"testing"
)

func TestErrorRing_NilSafe(t *testing.T) {
	var r *errorRing

	// All operations should be safe on nil
	r.push(errors.New("test"))
	r.clear()

	if r.all() != nil {
		t.Error("expected nil from nil ring")
	}
}

func TestErrorRing_ZeroSize(t *testing.T) {
	r := newErrorRing(0)
	if r != nil {
		t.Error("expected nil ring for size 0")
	}
}

func TestErrorRing_NegativeSize(t *testing.T) {
	r := newErrorRing(-1)
	if r != nil {
		t.Error("expected nil ring for negative size")
	}
}

func TestErrorRing_SingleError(t *testing.T) {
	r := newErrorRing(3)

	err := errors.New("error1")
	r.push(err)

	errs := r.all()
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if errs[0].Error() != "error1" {
		t.Error("expected same error instance")
	}
}

func TestErrorRing_FillsWithoutWrapping(t *testing.T) {
	r := newErrorRing(3)

	err1 := errors.New("error1")
	err2 := errors.New("error2")
	err3 := errors.New("error3")

	r.push(err1)
	r.push(err2)
	r.push(err3)

	errs := r.all()
	if len(errs) != 3 {
		t.Fatalf("expected 3 errors, got %d", len(errs))
	}

	// Oldest first
	if errs[0].Error() != "error1" {
		t.Error("expected err1 first")
	}
	if errs[1].Error() != "error2" {
		t.Error("expected err2 second")
	}
	if errs[2].Error() != "error3" {
		t.Error("expected err3 third")
	}
}

func TestErrorRing_WrapsAndEvictsOldest(t *testing.T) {
	r := newErrorRing(3)

	err1 := errors.New("error1")
	err2 := errors.New("error2")
	err3 := errors.New("error3")
	err4 := errors.New("error4")

	r.push(err1)
	r.push(err2)
	r.push(err3)
	r.push(err4) // Should evict err1

	errs := r.all()
	if len(errs) != 3 {
		t.Fatalf("expected 3 errors, got %d", len(errs))
	}

	// err1 should be gone, oldest is now err2
	if errs[0].Error() != "error2" {
		t.Error("expected err2 first after wrap")
	}
	if errs[1].Error() != "error3" {
		t.Error("expected err3 second")
	}
	if errs[2].Error() != "error4" {
		t.Error("expected err4 third")
	}
}

func TestErrorRing_MultipleWraps(t *testing.T) {
	r := newErrorRing(2)

	for i := 0; i < 10; i++ {
		r.push(errors.New("error"))
	}

	errs := r.all()
	if len(errs) != 2 {
		t.Errorf("expected 2 errors after multiple wraps, got %d", len(errs))
	}
}

func TestErrorRing_Clear(t *testing.T) {
	r := newErrorRing(3)

	r.push(errors.New("error1"))
	r.push(errors.New("error2"))

	r.clear()

	errs := r.all()
	if errs != nil {
		t.Errorf("expected nil after clear, got %v", errs)
	}
}

func TestErrorRing_ClearThenPush(t *testing.T) {
	r := newErrorRing(3)

	r.push(errors.New("error1"))
	r.push(errors.New("error2"))
	r.clear()

	newErr := errors.New("new error")
	r.push(newErr)

	errs := r.all()
	if len(errs) != 1 {
		t.Fatalf("expected 1 error after clear+push, got %d", len(errs))
	}
	if errs[0].Error() != "new error" {
		t.Error("expected new error")
	}
}

func TestErrorRing_EmptyAll(t *testing.T) {
	r := newErrorRing(3)

	errs := r.all()
	if errs != nil {
		t.Errorf("expected nil for empty ring, got %v", errs)
	}
}

func TestErrorRing_SizeOne(t *testing.T) {
	r := newErrorRing(1)

	err1 := errors.New("error1")
	err2 := errors.New("error2")

	r.push(err1)
	errs := r.all()
	if len(errs) != 1 || errs[0].Error() != "error1" {
		t.Error("expected err1")
	}

	r.push(err2)
	errs = r.all()
	if len(errs) != 1 || errs[0].Error() != "error2" {
		t.Error("expected err2 to replace err1")
	}
}

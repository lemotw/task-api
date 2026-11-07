package task

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

var errBoom = errors.New("boom")

type createCall struct {
	name   string
	status int
}

type updateCall struct {
	id     int
	name   string
	status int
}

type fakeRepo struct {
	listResult   []Task
	createResult Task
	updateResult Task
	updateErr    error
	deleteErr    error

	createCalls []createCall
	updateCalls []updateCall
	deleteCalls []int
}

func (r *fakeRepo) List() []Task { return r.listResult }

func (r *fakeRepo) Create(name string, status int) Task {
	r.createCalls = append(r.createCalls, createCall{name, status})
	return r.createResult
}

func (r *fakeRepo) Update(id int, name string, status int) (Task, error) {
	r.updateCalls = append(r.updateCalls, updateCall{id, name, status})
	return r.updateResult, r.updateErr
}

func (r *fakeRepo) Delete(id int) error {
	r.deleteCalls = append(r.deleteCalls, id)
	return r.deleteErr
}

// The boundaries below are the literal numbers from the spec (DOM-5), not
// MaxNameLength, so a wrongly edited constant cannot make them pass.
var validNames = map[string]string{
	"plain":              "buy milk",
	"surrounding spaces": "  buy milk  ",
	"255 ascii":          strings.Repeat("a", 255),
	"255 chinese":        strings.Repeat("任", 255),
	"255 emoji":          strings.Repeat("😀", 255),
}

var invalidNames = map[string]string{
	"empty":               "",
	"spaces":              "   ",
	"ideographic space":   "　",
	"tab and newline":     "\t\n",
	"256 ascii":           strings.Repeat("a", 256),
	"256 code points":     strings.Repeat("任", 256),
	"255 plus a space":    strings.Repeat("a", 255) + " ",
	"space plus 255 char": " " + strings.Repeat("任", 255),
}

var invalidStatuses = []int{2, -1}

var validStatuses = []int{StatusIncomplete, StatusComplete}

func TestMaxNameLength_MatchesSpec(t *testing.T) {
	if MaxNameLength != 255 {
		t.Errorf("MaxNameLength = %d, want 255", MaxNameLength)
	}
}

func assertValidationError(t *testing.T, err error) {
	t.Helper()
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
}

func TestValidate_AcceptsValidValues(t *testing.T) {
	for label, name := range validNames {
		t.Run(label, func(t *testing.T) {
			for _, status := range []int{StatusIncomplete, StatusComplete} {
				if err := Validate(name, status); err != nil {
					t.Errorf("status %d: %v", status, err)
				}
			}
		})
	}
}

func TestValidate_RejectsInvalidValues(t *testing.T) {
	for label, name := range invalidNames {
		t.Run(label, func(t *testing.T) {
			assertValidationError(t, Validate(name, StatusIncomplete))
		})
	}
	for _, status := range invalidStatuses {
		assertValidationError(t, Validate("a", status))
	}
}

func TestValidationError_ReturnsMessage(t *testing.T) {
	err := &ValidationError{Field: "status", Message: "status must be 0 or 1"}
	if got := err.Error(); got != "status must be 0 or 1" {
		t.Errorf("Error() = %q", got)
	}
}

func TestService_List_ReturnsRepositoryResult(t *testing.T) {
	want := []Task{{ID: 1, Name: "a"}, {ID: 2, Name: "b", Status: 1}}
	s := NewService(&fakeRepo{listResult: want})
	if got := s.List(); !reflect.DeepEqual(got, want) {
		t.Errorf("List = %+v", got)
	}
}

func TestService_Create_PassesValuesUnchanged(t *testing.T) {
	for label, name := range validNames {
		for _, status := range validStatuses {
			t.Run(fmt.Sprintf("%s/status %d", label, status), func(t *testing.T) {
				want := Task{ID: 7, Name: name, Status: status}
				repo := &fakeRepo{createResult: want}
				got, err := NewService(repo).Create(name, status)
				if err != nil {
					t.Fatalf("Create = %v", err)
				}
				if got != want {
					t.Errorf("task = %+v", got)
				}
				if !reflect.DeepEqual(repo.createCalls, []createCall{{name, status}}) {
					t.Errorf("calls = %+v", repo.createCalls)
				}
			})
		}
	}
}

func TestService_Create_AllowsDuplicateNames(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo)
	for range 2 {
		if _, err := s.Create("same", StatusIncomplete); err != nil {
			t.Fatalf("Create = %v", err)
		}
	}
	if len(repo.createCalls) != 2 {
		t.Errorf("repository called %d times", len(repo.createCalls))
	}
}

func TestService_Update_PassesValuesUnchanged(t *testing.T) {
	for label, name := range validNames {
		for _, status := range validStatuses { // includes complete -> incomplete (0)
			t.Run(fmt.Sprintf("%s/status %d", label, status), func(t *testing.T) {
				want := Task{ID: 3, Name: name, Status: status}
				repo := &fakeRepo{updateResult: want}
				got, err := NewService(repo).Update(3, name, status)
				if err != nil {
					t.Fatalf("Update = %v", err)
				}
				if got != want {
					t.Errorf("task = %+v", got)
				}
				if !reflect.DeepEqual(repo.updateCalls, []updateCall{{3, name, status}}) {
					t.Errorf("calls = %+v", repo.updateCalls)
				}
			})
		}
	}
}

func TestService_Delete_PassesID(t *testing.T) {
	repo := &fakeRepo{}
	if err := NewService(repo).Delete(4); err != nil {
		t.Fatalf("Delete = %v", err)
	}
	if !reflect.DeepEqual(repo.deleteCalls, []int{4}) {
		t.Errorf("calls = %v", repo.deleteCalls)
	}
}

func TestService_Create_RejectsInvalidValuesWithoutTouchingRepository(t *testing.T) {
	check := func(t *testing.T, name string, status int) {
		t.Helper()
		repo := &fakeRepo{}
		_, err := NewService(repo).Create(name, status)
		assertValidationError(t, err)
		if len(repo.createCalls) != 0 {
			t.Errorf("repository called %d times", len(repo.createCalls))
		}
	}
	for label, name := range invalidNames {
		t.Run(label, func(t *testing.T) { check(t, name, StatusIncomplete) })
	}
	for _, status := range invalidStatuses {
		check(t, "a", status)
	}
}

func TestService_Update_RejectsInvalidValuesWithoutTouchingRepository(t *testing.T) {
	check := func(t *testing.T, name string, status int) {
		t.Helper()
		repo := &fakeRepo{}
		_, err := NewService(repo).Update(1, name, status)
		assertValidationError(t, err)
		if len(repo.updateCalls) != 0 {
			t.Errorf("repository called %d times", len(repo.updateCalls))
		}
	}
	for label, name := range invalidNames {
		t.Run(label, func(t *testing.T) { check(t, name, StatusIncomplete) })
	}
	for _, status := range invalidStatuses {
		check(t, "a", status)
	}
}

func TestService_Update_PropagatesRepositoryErrors(t *testing.T) {
	for _, want := range []error{ErrNotFound, errBoom} {
		s := NewService(&fakeRepo{updateErr: want})
		if _, err := s.Update(1, "a", StatusIncomplete); !errors.Is(err, want) {
			t.Errorf("Update = %v, want %v", err, want)
		}
	}
}

func TestService_Delete_PropagatesRepositoryErrors(t *testing.T) {
	for _, want := range []error{ErrNotFound, errBoom} {
		s := NewService(&fakeRepo{deleteErr: want})
		if err := s.Delete(1); !errors.Is(err, want) {
			t.Errorf("Delete = %v, want %v", err, want)
		}
	}
}

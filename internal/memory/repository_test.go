package memory

import (
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/lemotw/task-api/internal/task"
)

func revision(r *Repository) uint64 {
	_, rev := r.Export()
	return rev
}

func nextID(r *Repository) int {
	state, _ := r.Export()
	return state.NextID
}

func TestList_EmptyIsNotNil(t *testing.T) {
	got := New().List()
	if got == nil || len(got) != 0 {
		t.Fatalf("List = %#v", got)
	}
}

func TestCreate_AssignsSequentialIDs(t *testing.T) {
	r := New()
	for want := 1; want <= 3; want++ {
		got := r.Create("a", task.StatusComplete)
		if got != (task.Task{ID: want, Name: "a", Status: task.StatusComplete}) {
			t.Fatalf("Create = %+v", got)
		}
	}
}

func TestCreate_KeepsDuplicateNames(t *testing.T) {
	r := New()
	r.Create("same", task.StatusIncomplete)
	r.Create("same", task.StatusIncomplete)
	if got := r.List(); len(got) != 2 {
		t.Fatalf("List = %+v", got)
	}
}

func TestList_OrderedByID(t *testing.T) {
	r := New()
	for range 50 {
		r.Create("a", task.StatusIncomplete)
	}
	list := r.List()
	if len(list) != 50 {
		t.Fatalf("List has %d tasks, want 50", len(list))
	}
	for i, got := range list {
		if got.ID != i+1 {
			t.Fatalf("position %d has id %d", i, got.ID)
		}
	}
}

func TestList_ReturnsCopy(t *testing.T) {
	r := New()
	r.Create("original", task.StatusIncomplete)
	r.List()[0].Name = "changed"
	if got := r.List()[0].Name; got != "original" {
		t.Errorf("Name = %q", got)
	}
}

func TestUpdate_ReplacesValuesAndKeepsID(t *testing.T) {
	r := New()
	created := r.Create("old", task.StatusIncomplete)
	got, err := r.Update(created.ID, "new", task.StatusComplete)
	if err != nil {
		t.Fatalf("Update = %v", err)
	}
	want := task.Task{ID: created.ID, Name: "new", Status: task.StatusComplete}
	if got != want {
		t.Errorf("Update returned %+v", got)
	}
	if list := r.List(); len(list) != 1 || list[0] != want {
		t.Errorf("List = %+v", list)
	}
}

func TestDelete_RemovesTask(t *testing.T) {
	r := New()
	a := r.Create("a", task.StatusIncomplete)
	b := r.Create("b", task.StatusIncomplete)
	if err := r.Delete(a.ID); err != nil {
		t.Fatalf("Delete = %v", err)
	}
	if list := r.List(); len(list) != 1 || list[0] != b {
		t.Errorf("List = %+v", list)
	}
}

func TestIDs_NotReusedAfterDeletingHighest(t *testing.T) {
	r := New()
	r.Create("a", task.StatusIncomplete)
	high := r.Create("b", task.StatusIncomplete)
	if err := r.Delete(high.ID); err != nil {
		t.Fatal(err)
	}
	if got := r.Create("c", task.StatusIncomplete); got.ID <= high.ID {
		t.Errorf("id %d reused or lower than %d", got.ID, high.ID)
	}
}

func TestIDs_NotReusedAfterDeletingAll(t *testing.T) {
	r := New()
	a := r.Create("a", task.StatusIncomplete)
	b := r.Create("b", task.StatusIncomplete)
	for _, id := range []int{a.ID, b.ID} {
		if err := r.Delete(id); err != nil {
			t.Fatal(err)
		}
	}
	if got := r.Create("c", task.StatusIncomplete); got.ID <= b.ID {
		t.Errorf("id %d reused or lower than %d", got.ID, b.ID)
	}
}

func TestUpdateAndDelete_UnknownIDReturnsNotFound(t *testing.T) {
	r := New()
	if _, err := r.Update(99, "a", task.StatusIncomplete); !errors.Is(err, task.ErrNotFound) {
		t.Errorf("Update = %v", err)
	}
	if err := r.Delete(99); !errors.Is(err, task.ErrNotFound) {
		t.Errorf("Delete = %v", err)
	}
}

func TestFailedOperations_ChangeNothing(t *testing.T) {
	ops := map[string]func(r *Repository) error{
		"update": func(r *Repository) error {
			_, err := r.Update(99, "x", task.StatusComplete)
			return err
		},
		"delete": func(r *Repository) error { return r.Delete(99) },
	}
	for name, op := range ops {
		t.Run(name, func(t *testing.T) {
			r := New()
			r.Create("a", task.StatusIncomplete)
			r.Create("b", task.StatusComplete)
			beforeState, beforeRev := r.Export()

			if err := op(r); !errors.Is(err, task.ErrNotFound) {
				t.Fatalf("error = %v, want ErrNotFound", err)
			}

			afterState, afterRev := r.Export()
			if afterRev != beforeRev {
				t.Errorf("revision changed from %d to %d", beforeRev, afterRev)
			}
			if !reflect.DeepEqual(afterState, beforeState) {
				t.Errorf("state changed\n before: %+v\n after:  %+v", beforeState, afterState)
			}
			if got := r.Create("c", task.StatusIncomplete); got.ID != beforeState.NextID {
				t.Errorf("new id = %d, want %d", got.ID, beforeState.NextID)
			}
		})
	}
}

func TestRevision_IncreasesOnEverySuccessfulMutation(t *testing.T) {
	r := New()
	last := revision(r)
	assertIncreased := func(op string) {
		t.Helper()
		got := revision(r)
		if got <= last {
			t.Errorf("%s: revision %d not above %d", op, got, last)
		}
		last = got
	}

	created := r.Create("a", task.StatusIncomplete)
	assertIncreased("create")
	if _, err := r.Update(created.ID, "b", task.StatusComplete); err != nil {
		t.Fatal(err)
	}
	assertIncreased("update")
	if err := r.Delete(created.ID); err != nil {
		t.Fatal(err)
	}
	assertIncreased("delete")
}

func TestExport_ReturnsIsolatedCopy(t *testing.T) {
	r := New()
	r.Create("original", task.StatusIncomplete)
	state, _ := r.Export()
	state.Tasks[0].Name = "changed"
	if got := r.List()[0].Name; got != "original" {
		t.Errorf("Name = %q", got)
	}
}

func TestExport_NotAffectedByLaterChanges(t *testing.T) {
	r := New()
	created := r.Create("original", task.StatusIncomplete)
	state, _ := r.Export()
	if _, err := r.Update(created.ID, "changed", task.StatusComplete); err != nil {
		t.Fatal(err)
	}
	r.Create("second", task.StatusIncomplete)
	if len(state.Tasks) != 1 || state.Tasks[0].Name != "original" {
		t.Errorf("snapshot changed: %+v", state.Tasks)
	}
}

func TestImport_RestoresTasksAndNextID(t *testing.T) {
	src := New()
	src.Create("a", task.StatusIncomplete)
	high := src.Create("b", task.StatusComplete)
	if err := src.Delete(high.ID); err != nil {
		t.Fatal(err)
	}
	state, _ := src.Export()

	dst := New()
	dst.Import(state)

	if got, want := dst.List(), src.List(); !reflect.DeepEqual(got, want) {
		t.Errorf("List = %+v, want %+v", got, want)
	}
	if got := dst.Create("c", task.StatusIncomplete); got.ID <= high.ID {
		t.Errorf("id %d reused or lower than %d", got.ID, high.ID)
	}
}

func TestImport_ReplacesExistingTasks(t *testing.T) {
	r := New()
	r.Create("old", task.StatusIncomplete)
	want := task.Task{ID: 5, Name: "new", Status: task.StatusComplete}
	r.Import(task.State{NextID: 6, Tasks: []task.Task{want}})
	if got := r.List(); len(got) != 1 || got[0] != want {
		t.Errorf("List = %+v", got)
	}
}

func TestImport_DoesNotShareInputSlice(t *testing.T) {
	r := New()
	tasks := []task.Task{{ID: 1, Name: "original"}}
	r.Import(task.State{NextID: 2, Tasks: tasks})
	tasks[0].Name = "changed"
	if got := r.List()[0].Name; got != "original" {
		t.Errorf("Name = %q", got)
	}
}

func TestConcurrentCreate_AssignsUniqueIDsAndKeepsEveryTask(t *testing.T) {
	r := New()
	const workers, perWorker = 8, 100
	ids := make(chan int, workers*perWorker)

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range perWorker {
				ids <- r.Create("a", task.StatusIncomplete).ID
			}
		}()
	}
	wg.Wait()
	close(ids)

	seen := make(map[int]bool)
	for id := range ids {
		if seen[id] {
			t.Fatalf("duplicate id %d", id)
		}
		seen[id] = true
	}
	if got := len(r.List()); got != workers*perWorker {
		t.Errorf("List has %d tasks", got)
	}
}

func TestConcurrentMixedOperations_AreSafe(t *testing.T) {
	r := New()
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				created := r.Create("a", task.StatusIncomplete)
				_ = r.List()
				_, _ = r.Update(created.ID, "b", task.StatusComplete)
				_, _ = r.Export()
				_ = r.Delete(created.ID)
			}
		}()
	}
	wg.Wait()
	if got := r.List(); len(got) != 0 {
		t.Errorf("List = %+v", got)
	}
}

func TestConcurrentUpdateAndDelete_NeverRestoresDeletedTask(t *testing.T) {
	r := New()
	for range 200 {
		created := r.Create("a", task.StatusIncomplete)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = r.Update(created.ID, "b", task.StatusComplete)
		}()
		go func() {
			defer wg.Done()
			if err := r.Delete(created.ID); err != nil {
				t.Errorf("Delete = %v", err)
			}
		}()
		wg.Wait()
		for _, got := range r.List() {
			if got.ID == created.ID {
				t.Fatalf("deleted task %d is back", got.ID)
			}
		}
	}
}

func TestExport_NextIDAboveEveryTaskDuringWrites(t *testing.T) {
	r := New()
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				created := r.Create("a", task.StatusIncomplete)
				_ = r.Delete(created.ID)
			}
		}
	}()
	for range 1000 {
		state, _ := r.Export()
		for _, got := range state.Tasks {
			if got.ID >= state.NextID {
				close(stop)
				wg.Wait()
				t.Fatalf("task id %d not below next id %d", got.ID, state.NextID)
			}
		}
	}
	close(stop)
	wg.Wait()
}

// The writers are paused halfway, so the intermediate export is guaranteed to
// see a partial state: neither empty nor final.
func TestExport_ObservesIntermediateStateWhileWritersArePaused(t *testing.T) {
	const writers, firstPhase, secondPhase = 4, 100, 150
	r := New()

	var phase1 sync.WaitGroup
	var all sync.WaitGroup
	release := make(chan struct{})
	for range writers {
		phase1.Add(1)
		all.Add(1)
		go func() {
			defer all.Done()
			for range firstPhase {
				r.Create("a", task.StatusIncomplete)
			}
			phase1.Done()
			<-release // paused until the main goroutine has exported
			for range secondPhase {
				r.Create("a", task.StatusIncomplete)
			}
		}()
	}

	phase1.Wait()
	mid, midRev := r.Export()
	if want := writers * firstPhase; len(mid.Tasks) != want || mid.NextID != want+1 {
		t.Fatalf("intermediate export: %d tasks, next id %d, want %d tasks", len(mid.Tasks), mid.NextID, want)
	}

	close(release)
	all.Wait()

	final, finalRev := r.Export()
	if want := writers * (firstPhase + secondPhase); len(final.Tasks) != want || final.NextID != want+1 {
		t.Fatalf("final export: %d tasks, next id %d, want %d tasks", len(final.Tasks), final.NextID, want)
	}
	if finalRev <= midRev {
		t.Errorf("revision %d did not advance past %d", finalRev, midRev)
	}
	if len(mid.Tasks) != writers*firstPhase {
		t.Errorf("intermediate snapshot changed to %d tasks", len(mid.Tasks))
	}
}

func TestExport_IsConsistentDuringCreates(t *testing.T) {
	const writers, perWriter = 4, 250
	r := New()

	var (
		wg      sync.WaitGroup
		started = make(chan struct{})
		once    sync.Once
	)
	for range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range perWriter {
				r.Create("a", task.StatusIncomplete)
				once.Do(func() { close(started) }) // a write has really happened
			}
		}()
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	check := func(state task.State) {
		t.Helper()
		maxID := 0
		for _, got := range state.Tasks {
			maxID = max(maxID, got.ID)
		}
		if len(state.Tasks) != maxID || state.NextID != maxID+1 {
			t.Fatalf("inconsistent state: %d tasks, max id %d, next id %d",
				len(state.Tasks), maxID, state.NextID)
		}
	}

	<-started
	state, _ := r.Export()
	if len(state.Tasks) == 0 {
		t.Fatal("export after the first write is empty")
	}
	check(state)
	for exporting := true; exporting; {
		select {
		case <-done:
			exporting = false
		default:
			state, _ := r.Export()
			check(state)
		}
	}

	final, _ := r.Export()
	check(final)
	if want := writers * perWriter; len(final.Tasks) != want {
		t.Errorf("final export has %d tasks, want %d", len(final.Tasks), want)
	}
}

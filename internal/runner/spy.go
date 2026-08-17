package runner

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Call is one recorded invocation.
type Call struct {
	Dir  string
	Name string
	Args []string
}

// String renders the call the way a shell would show it, which is what test
// assertions compare against.
func (c Call) String() string {
	if len(c.Args) == 0 {
		return c.Name
	}
	return c.Name + " " + strings.Join(c.Args, " ")
}

// Spy is a Runner that records calls instead of running anything. It lives in
// the non-test build so every package that takes a Runner can use it.
type Spy struct {
	mu    sync.Mutex
	calls []Call

	// ExitFor returns the exit code and error for a call. A nil ExitFor means
	// every call succeeds.
	ExitFor func(c Call) (int, error)
}

// Run records the call and consults ExitFor for its result.
func (s *Spy) Run(ctx context.Context, dir, name string, args ...string) (int, error) {
	s.mu.Lock()
	c := Call{Dir: dir, Name: name, Args: append([]string(nil), args...)}
	s.calls = append(s.calls, c)
	s.mu.Unlock()

	if s.ExitFor == nil {
		return 0, nil
	}
	return s.ExitFor(c)
}

// Calls returns the recorded calls in order.
func (s *Spy) Calls() []Call {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Call(nil), s.calls...)
}

// Count returns how many calls were recorded.
func (s *Spy) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

// ArgLines renders each call's arguments joined, for order assertions.
func (s *Spy) ArgLines() []string {
	calls := s.Calls()
	out := make([]string, 0, len(calls))
	for _, c := range calls {
		out = append(out, strings.Join(c.Args, " "))
	}
	return out
}

// FailOnArg makes any call whose arguments contain substr exit with code.
func FailOnArg(substr string, code int) func(Call) (int, error) {
	return func(c Call) (int, error) {
		if strings.Contains(strings.Join(c.Args, " "), substr) {
			return code, fmt.Errorf("exited %d", code)
		}
		return 0, nil
	}
}

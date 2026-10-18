package state

import "testing"

// The transition table below is the frozen contract of
// odd/tasks/presentation-client-ready.md §«Contrato congelado de prevention»,
// transcribed once and literally:
//
//	propuesta → asignada → en_curso → bloqueada → en_curso → enviada →
//	aceptada | devuelta
//	devuelta → en_curso (reapertura)
//	cancelada desde cualquier estado no terminal
//	aceptada y cancelada son terminales
//	toda otra transición es inválida
//
// It is deliberately written as the full state x state matrix instead of a list
// of "happy paths": the contract says every other transition is invalid, so the
// negative space is half the rule and has to be enumerated, not sampled.
var contractTransitions = map[State][]State{
	Propuesta: {Asignada},
	Asignada:  {EnCurso},
	EnCurso:   {Bloqueada, Enviada},
	Bloqueada: {EnCurso},
	Enviada:   {Aceptada, Devuelta},
	Devuelta:  {EnCurso},
	Aceptada:  {},
	Cancelada: {},
}

// contractStates is the wire vocabulary in the order the contract lists it.
var contractStates = []State{Propuesta, Asignada, EnCurso, Bloqueada, Enviada, Aceptada, Devuelta, Cancelada}

// nonTerminalStates are the states "cancelada desde cualquier estado no
// terminal" applies to.
var nonTerminalStates = []State{Propuesta, Asignada, EnCurso, Bloqueada, Enviada, Devuelta}

func allowed(from, to State) bool {
	for _, candidate := range contractTransitions[from] {
		if candidate == to {
			return true
		}
	}
	// Cancellation is reachable from every non-terminal state.
	if to == Cancelada {
		return !Terminal(from) && Valid(from)
	}
	return false
}

// TestEveryTransitionPairMatchesTheFrozenContract walks all 8 x 8 pairs so a
// transition that the contract omits can never be silently accepted and one it
// states can never be silently refused.
func TestEveryTransitionPairMatchesTheFrozenContract(t *testing.T) {
	if len(contractStates) != 8 {
		t.Fatalf("the contract defines eight states, the table has %d", len(contractStates))
	}
	allowedCount := 0
	for _, from := range contractStates {
		for _, to := range contractStates {
			want := allowed(from, to)
			if got := CanTransition(from, to); got != want {
				t.Errorf("CanTransition(%q, %q) = %v, want %v", from, to, got, want)
			}
			if want {
				allowedCount++
			}
		}
	}
	// Eight declared edges (propuesta→asignada, asignada→en_curso,
	// en_curso→bloqueada, bloqueada→en_curso, en_curso→enviada,
	// enviada→aceptada, enviada→devuelta, devuelta→en_curso) plus one
	// cancellation per non-terminal state. Cancelling an already accepted or
	// already cancelled action is NOT one of them: those are terminal.
	wantAllowed := 0
	for _, edges := range contractTransitions {
		wantAllowed += len(edges)
	}
	wantAllowed += len(nonTerminalStates)
	if wantAllowed != 14 {
		t.Fatalf("the table above describes %d transitions, the contract states 14", wantAllowed)
	}
	if allowedCount != wantAllowed {
		t.Fatalf("the contract allows %d transitions, the table produced %d", wantAllowed, allowedCount)
	}
}

// TestSelfTransitionsAreNeverValid pins that a no-op "transition" is invalid:
// re-sending the same state would otherwise write a history row claiming an
// event that did not happen.
func TestSelfTransitionsAreNeverValid(t *testing.T) {
	for _, s := range contractStates {
		if CanTransition(s, s) {
			t.Errorf("CanTransition(%q, %q) must be false", s, s)
		}
	}
}

func TestTerminalStatesAreExactlyAcceptedAndCancelled(t *testing.T) {
	for _, s := range contractStates {
		want := s == Aceptada || s == Cancelada
		if got := Terminal(s); got != want {
			t.Errorf("Terminal(%q) = %v, want %v", s, got, want)
		}
	}
}

// TestTerminalStatesHaveNoOutgoingTransition is the property that makes
// "terminal" mean something: neither an explicit edge nor the cancellation
// clause may move an action out of it.
func TestTerminalStatesHaveNoOutgoingTransition(t *testing.T) {
	for _, from := range contractStates {
		if !Terminal(from) {
			continue
		}
		for _, to := range contractStates {
			if CanTransition(from, to) {
				t.Errorf("terminal state %q must not transition to %q", from, to)
			}
		}
	}
}

// TestCancellationIsReachableFromEveryNonTerminalState encodes the second half
// of the contract sentence, independently of the explicit edges above.
func TestCancellationIsReachableFromEveryNonTerminalState(t *testing.T) {
	for _, from := range nonTerminalStates {
		if !CanTransition(from, Cancelada) {
			t.Errorf("cancelada must be reachable from %q", from)
		}
	}
}

// TestVocabularyRejectsUnknownStates keeps the service from accepting a status
// string that the wire contract never defined, such as one copied from another
// service or an English translation.
func TestVocabularyRejectsUnknownStates(t *testing.T) {
	for _, unknown := range []State{"", "PENDIENTE", "pending", "en curso", "EnCurso", "asignada ", "cerrada"} {
		if Valid(unknown) {
			t.Errorf("%q must not be part of the state vocabulary", unknown)
		}
		if CanTransition(Propuesta, unknown) {
			t.Errorf("CanTransition(propuesta, %q) must be false", unknown)
		}
		if CanTransition(unknown, EnCurso) {
			t.Errorf("CanTransition(%q, en_curso) must be false", unknown)
		}
		if Terminal(unknown) {
			t.Errorf("%q must not be reported as terminal", unknown)
		}
	}
}

func TestStatesListsTheWholeVocabularyOnce(t *testing.T) {
	got := States()
	if len(got) != len(contractStates) {
		t.Fatalf("States() returned %d states, want %d", len(got), len(contractStates))
	}
	seen := map[State]int{}
	for _, s := range got {
		seen[s]++
	}
	for _, want := range contractStates {
		if seen[want] != 1 {
			t.Errorf("States() contains %q %d times, want exactly 1", want, seen[want])
		}
	}
}

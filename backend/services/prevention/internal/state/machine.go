// Package state is the preventive-action state machine frozen in
// odd/tasks/presentation-client-ready.md §«Contrato congelado de prevention».
//
// It is deliberately free of storage, HTTP and clock dependencies: the rule that
// decides whether a transition may happen is one pure function, so the transport
// and the repository cannot drift apart about which transitions exist. The
// repository still re-checks it inside the transaction that owns the row, because
// a decision taken outside that transaction can always be overtaken.
package state

// State is a status of a preventive action as it appears on the wire and in
// prevention.actions.status. The literals are Spanish and lower-case because the
// contract fixes them; they are part of the API, not an internal detail.
type State string

const (
	Propuesta State = "propuesta"
	Asignada  State = "asignada"
	EnCurso   State = "en_curso"
	Bloqueada State = "bloqueada"
	Enviada   State = "enviada"
	Aceptada  State = "aceptada"
	Devuelta  State = "devuelta"
	Cancelada State = "cancelada"
)

// transitions is the frozen edge set, transcribed from the contract:
//
//	propuesta → asignada
//	asignada  → en_curso
//	en_curso  → bloqueada | enviada
//	bloqueada → en_curso
//	enviada   → aceptada | devuelta
//	devuelta  → en_curso
//
// Cancellation is not listed per state: it is the rule "cancelada desde
// cualquier estado no terminal" and is computed from Terminal instead, so adding
// a state cannot silently forget its cancellation edge. Aceptada and cancelada
// are terminal and have no outgoing edge at all.
var transitions = map[State][]State{
	Propuesta: {Asignada},
	Asignada:  {EnCurso},
	EnCurso:   {Bloqueada, Enviada},
	Bloqueada: {EnCurso},
	Enviada:   {Aceptada, Devuelta},
	Devuelta:  {EnCurso},
	Aceptada:  {},
	Cancelada: {},
}

// vocabulary is the ordered state list, kept separate from transitions so the
// documented order is stable for error messages and for the UI.
var vocabulary = []State{Propuesta, Asignada, EnCurso, Bloqueada, Enviada, Aceptada, Devuelta, Cancelada}

// States returns the wire vocabulary in contract order. A copy is returned so a
// caller cannot mutate the shared list.
func States() []State {
	all := make([]State, len(vocabulary))
	copy(all, vocabulary)
	return all
}

// Valid reports whether s is a state the contract defines. Exact comparison is
// deliberate: trimming or case-folding would invent aliases the contract does
// not have, and an alias is a second vocabulary that can drift.
func Valid(s State) bool {
	_, ok := transitions[s]
	return ok
}

// Terminal reports whether s is a state an action can never leave. These are
// exactly aceptada and cancelada, and this function is also what makes
// cancellation unreachable FROM a terminal state.
func Terminal(s State) bool {
	return s == Aceptada || s == Cancelada
}

// CanTransition reports whether the contract allows from → to. It is false for
// an unknown state on either side and for a self-transition: re-sending the
// current status is not an event and must never write a history row.
func CanTransition(from, to State) bool {
	if !Valid(from) || !Valid(to) {
		return false
	}
	if to == Cancelada {
		return !Terminal(from)
	}
	for _, candidate := range transitions[from] {
		if candidate == to {
			return true
		}
	}
	return false
}

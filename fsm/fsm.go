package fsm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrEventRejected is the error returned when the state machine cannot process
// an event in the state that it is in.
var (
	ErrEventRejected        = errors.New("event rejected")
	ErrWaitForStateTimedOut = errors.New("timed out while waiting for event")
	ErrInvalidContextType   = errors.New("invalid context")
)

const (
	// Default represents the default state of the system.
	Default StateType = ""

	// NoOp represents a no-op event.
	NoOp EventType = "NoOp"

	OnError EventType = "OnError"

	ContextValidationFailed EventType = "ContextValidationFailed"
)

// StateType represents an extensible state type in the state machine.
type StateType string

// EventType represents an extensible event type in the state machine.
type EventType string

// EventContext represents the context to be passed to the action
// implementation.
type EventContext interface{}

// Action represents the action to be executed in a given state.
type Action func(eventCtx EventContext) EventType

// Transitions represents a mapping of events and states.
type Transitions map[EventType]StateType

// State binds a state with an action and a set of events it can handle.
type State struct {
	EntryFunc   func()
	ExitFunc    func()
	Action      Action
	Transitions Transitions
}

// States represents a mapping of states and their implementations.
type States map[StateType]State

// Notification represents a notification sent to the state machine's
// notification channel.
type Notification struct {
	PreviousState StateType
	NextState     StateType
	Event         EventType
}

// StateMachine represents the state machine.
type StateMachine struct {
	// Context represents the state machine context.
	States States

	// Previous represents the previous state.
	Previous StateType

	// Current represents the current state.
	Current StateType

	// ActionEntryFunc is a function that is called before an action is executed.
	ActionEntryFunc func()

	// ActionExitFunc is a function that is called after an action is executed.
	ActionExitFunc func()

	// mutex ensures that only 1 event is processed by the state machine at
	// any given time.
	mutex sync.Mutex

	//LastActionError is an error set by the last action executed.
	LastActionError error

	// NotificationChan is a channel that receives notifications when the state
	// machine transitions between states.
	NotificationChan chan Notification
}

// getNextState returns the next state for the event given the machine's current
// state, or an error if the event can't be handled in the given state.
func (s *StateMachine) getNextState(event EventType) (State, error) {

	var state State
	var ok bool

	stateMap := s.States

	if state, ok = stateMap[s.Current]; !ok {
		return State{}, NewErrConfigError("current state not found")

	}
	if state.Transitions == nil {
		return State{}, NewErrConfigError("current state has no transitions")
	}

	var next StateType
	if next, ok = state.Transitions[event]; !ok {
		return State{}, NewErrConfigError(
			"event not found in current transitions",
		)
	}

	// Identify the state definition for the next state.
	state, ok = stateMap[next]
	if !ok {
		return State{}, NewErrConfigError("next state not found")
	}

	if state.Action == nil {
		return State{}, NewErrConfigError("next state has no action")
	}

	// Transition over to the next state.
	s.Previous = s.Current
	s.Current = next

	// Notify the state machine's notification channel if it exists.
	if s.NotificationChan != nil {
		s.NotificationChan <- Notification{
			PreviousState: s.Previous,
			NextState:     s.Current,
			Event:         event,
		}
	}

	return state, nil
}

// SendEvent sends an event to the state machine.
func (s *StateMachine) SendEvent(event EventType, eventCtx EventContext) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	fsmConfig := s.States
	if fsmConfig == nil {
		return NewErrConfigError("state machine config is nil")
	}

	for {
		// Determine the next state for the event given the machine's
		// current state.
		state, err := s.getNextState(event)
		if err != nil {
			return ErrEventRejected
		}

		// Execute the state machines ActionEntryFunc.
		if s.ActionEntryFunc != nil {
			s.ActionEntryFunc()
		}

		// Execute the current state's entry function
		if state.EntryFunc != nil {
			state.EntryFunc()
		}

		// Execute the next state's action and loop over again if the event
		// returned is not a no-op.
		nextEvent := state.Action(eventCtx)

		// Execute the current state's exit function
		if state.ExitFunc != nil {
			state.ExitFunc()
		}

		// Execute the state machines ActionExitFunc.
		if s.ActionExitFunc != nil {
			s.ActionExitFunc()
		}

		// If the next event is a no-op, we're done.
		if nextEvent == NoOp {
			return nil
		}

		event = nextEvent
	}
}

// HandleError is a helper function that can be used by actions to handle
// errors.
func (s *StateMachine) HandleError(err error) EventType {
	log.Errorf("StateMachine error: %s", err)
	s.LastActionError = err
	return OnError
}

// WaitForState waits for the state machine to reach the given state.
func (s *StateMachine) WaitForState(ctx context.Context,
	timeout time.Duration, state StateType) error {

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	var lastState StateType
	for {
		select {
		case <-timeoutCtx.Done():
			cancel()
			return NewErrWaitingForStateTimeout(state, lastState)
		default:
			if s.mutex.TryLock() {
				if s.Current == state {
					s.mutex.Unlock()
					cancel()
					return nil
				}
				lastState = s.Current
				s.mutex.Unlock()
			}
			time.Sleep(time.Millisecond * 10)

		}
	}
}

// NoOpAction is a no-op action that can be used by states that don't need to
// execute any action.
func NoOpAction(_ EventContext) EventType {
	return NoOp
}

// ErrConfigError is an error returned when the state machine is misconfigured.
type ErrConfigError error

// NewErrConfigError creates a new ErrConfigError.
func NewErrConfigError(msg string) ErrConfigError {
	return (ErrConfigError)(fmt.Errorf("config error: %s", msg))
}

// ErrWaitingForStateTimeout is an error returned when the state machine times
// out while waiting for a state.
type ErrWaitingForStateTimeout error

// NewErrWaitingForStateTimeout creates a new ErrWaitingForStateTimeout.
func NewErrWaitingForStateTimeout(expected,
	actual StateType) ErrWaitingForStateTimeout {

	return (ErrWaitingForStateTimeout)(fmt.Errorf(
		"waiting for state timeout: expected %s, actual: %s", expected, actual,
	))
}

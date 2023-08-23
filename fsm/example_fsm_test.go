package fsm

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type mockStore struct {
	storeErr error
}

func (m *mockStore) StoreStuff() error {
	return m.storeErr
}

type mockService struct {
	respondChan chan bool
	respondErr  error
}

func (m *mockService) WaitForStuffHappening() (<-chan bool, error) {
	return m.respondChan, m.respondErr
}

func newInitStuffRequest() *InitStuffRequest {
	return &InitStuffRequest{
		Stuff:       "stuff",
		respondChan: make(chan<- string, 1),
	}
}

func TestExampleFSM(t *testing.T) {

	testCases := []struct {
		name                    string
		expectedState           StateType
		eventCtx                EventContext
		expectedLastActionError error

		sendEvent    EventType
		sendEventErr error

		serviceErr error
		storeErr   error
	}{
		{
			name:          "success",
			expectedState: StuffSuccess,
			eventCtx:      newInitStuffRequest(),
			sendEvent:     OnRequestStuff,
		},
		{
			name:                    "service error",
			expectedState:           StuffFailed,
			eventCtx:                newInitStuffRequest(),
			sendEvent:               OnRequestStuff,
			serviceErr:              fmt.Errorf("service error"),
			expectedLastActionError: errors.New("service error"),
		},
		{
			name:                    "store error",
			expectedLastActionError: errors.New("store error"),
			storeErr:                errors.New("store error"),
			sendEvent:               OnRequestStuff,
			expectedState:           StuffFailed,
			eventCtx:                newInitStuffRequest(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			respondChan := make(chan string, 1)
			if req, ok := tc.eventCtx.(*InitStuffRequest); ok {
				req.respondChan = respondChan
			}

			serviceRespondChan := make(chan bool, 1)
			serviceRespondChan <- true

			service := &mockService{
				respondChan: serviceRespondChan,
				respondErr:  tc.serviceErr,
			}

			store := &mockStore{
				storeErr: tc.storeErr,
			}

			exampleContext := NewExampleFSMContext(service, store)

			err := exampleContext.SendEvent(tc.sendEvent, tc.eventCtx)
			require.Equal(t, tc.sendEventErr, err)
			require.Equal(t, tc.expectedLastActionError, exampleContext.LastActionError)
			err = exampleContext.WaitForState(context.Background(), time.Second, tc.expectedState)
			require.NoError(t, err)
		})
	}
}

func getTestContext() *ExampleFSM {
	service := &mockService{
		respondChan: make(chan bool, 1),
	}
	service.respondChan <- true

	store := &mockStore{}

	exampleContext := NewExampleFSMContext(service, store)

	return exampleContext
}
func TestExampleFSMFlow(t *testing.T) {
	testCases := []struct {
		name              string
		expectedStateFlow []StateType
		expectedEventFlow []EventType
		storeError        error
		serviceError      error
	}{
		{
			name: "success",
			expectedStateFlow: []StateType{
				InitFSM,
				StuffSentOut,
				StuffSuccess,
			},
			expectedEventFlow: []EventType{
				OnRequestStuff,
				OnStuffSentOut,
				OnStuffSuccess,
			},
		},
		{
			name: "failure on store",
			expectedStateFlow: []StateType{
				InitFSM,
				StuffFailed,
			},
			expectedEventFlow: []EventType{
				OnRequestStuff,
				OnError,
			},
			storeError: errors.New("store error"),
		},
		{
			name: "failure on service",
			expectedStateFlow: []StateType{
				InitFSM,
				StuffSentOut,
				StuffFailed,
			},
			expectedEventFlow: []EventType{
				OnRequestStuff,
				OnStuffSentOut,
				OnError,
			},
			serviceError: errors.New("service error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			exampleContext := getTestContext()
			exampleContext.NotificationChan = make(chan Notification)

			if tc.storeError != nil {
				exampleContext.store.(*mockStore).storeErr = tc.storeError
			}

			if tc.serviceError != nil {
				exampleContext.service.(*mockService).respondErr = tc.serviceError
			}

			go func() {
				err := exampleContext.SendEvent(OnRequestStuff, newInitStuffRequest())
				require.NoError(t, err)
			}()

			for index, expectedState := range tc.expectedStateFlow {
				notification := <-exampleContext.NotificationChan
				if index == 0 {
					require.Equal(t, Default, notification.PreviousState)
				} else {
					require.Equal(
						t, tc.expectedStateFlow[index-1], notification.PreviousState,
					)
				}
				require.Equal(t, expectedState, notification.NextState)
				require.Equal(t, tc.expectedEventFlow[index], notification.Event)
			}

		})
	}

}

func TestLightSwitchFSM(t *testing.T) {
	// Create a new light switch FSM.
	lightSwitch := NewLightSwitchFSM()

	// Expect the light to be off
	require.Equal(t, lightSwitch.Current, OffState)

	// Send the On Event
	err := lightSwitch.SendEvent(SwitchOn, nil)
	require.NoError(t, err)

	// Expect the light to be on
	require.Equal(t, lightSwitch.Current, OnState)

	// Send the Off Event
	err = lightSwitch.SendEvent(SwitchOff, nil)
	require.NoError(t, err)

	// Expect the light to be off
	require.Equal(t, lightSwitch.Current, OffState)
}

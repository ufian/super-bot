// Code generated by moq; DO NOT EDIT.
// github.com/matryer/moq

package mocks

import (
	"sync"
)

// Summarizer is a mock implementation of events.summarizer.
//
//	func TestSomethingThatUsessummarizer(t *testing.T) {
//
//		// make and configure a mocked events.summarizer
//		mockedsummarizer := &Summarizer{
//			GetSummariesByMessageFunc: func(remarkLink string) ([]string, error) {
//				panic("mock out the GetSummariesByMessage method")
//			},
//		}
//
//		// use mockedsummarizer in code that requires events.summarizer
//		// and then make assertions.
//
//	}
type Summarizer struct {
	// GetSummariesByMessageFunc mocks the GetSummariesByMessage method.
	GetSummariesByMessageFunc func(remarkLink string) ([]string, error)

	// calls tracks calls to the methods.
	calls struct {
		// GetSummariesByMessage holds details about calls to the GetSummariesByMessage method.
		GetSummariesByMessage []struct {
			// RemarkLink is the remarkLink argument value.
			RemarkLink string
		}
	}
	lockGetSummariesByMessage sync.RWMutex
}

// GetSummariesByMessage calls GetSummariesByMessageFunc.
func (mock *Summarizer) GetSummariesByMessage(remarkLink string) ([]string, error) {
	if mock.GetSummariesByMessageFunc == nil {
		panic("Summarizer.GetSummariesByMessageFunc: method is nil but summarizer.GetSummariesByMessage was just called")
	}
	callInfo := struct {
		RemarkLink string
	}{
		RemarkLink: remarkLink,
	}
	mock.lockGetSummariesByMessage.Lock()
	mock.calls.GetSummariesByMessage = append(mock.calls.GetSummariesByMessage, callInfo)
	mock.lockGetSummariesByMessage.Unlock()
	return mock.GetSummariesByMessageFunc(remarkLink)
}

// GetSummariesByMessageCalls gets all the calls that were made to GetSummariesByMessage.
// Check the length with:
//
//	len(mockedsummarizer.GetSummariesByMessageCalls())
func (mock *Summarizer) GetSummariesByMessageCalls() []struct {
	RemarkLink string
} {
	var calls []struct {
		RemarkLink string
	}
	mock.lockGetSummariesByMessage.RLock()
	calls = mock.calls.GetSummariesByMessage
	mock.lockGetSummariesByMessage.RUnlock()
	return calls
}

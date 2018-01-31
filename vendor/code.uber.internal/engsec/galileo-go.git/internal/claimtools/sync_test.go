package claimtools

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSyncMapStringError(t *testing.T) {
	db := newSyncMapStringError()
	fooErr := errors.New("foo")

	// initial lookup fails
	ok, _ := db.Load("foo")
	assert.False(t, ok)

	// store/load succeeds
	db.Store("foo", fooErr)
	ok, err := db.Load("foo")
	assert.True(t, ok)
	assert.Equal(t, fooErr, err)
}

func TestSyncMapStringErrorRace(t *testing.T) {
	var wg sync.WaitGroup
	db := newSyncMapStringError()

	storeSignal := make(chan struct{})
	loadSignal := make(chan struct{})

	errFoo := errors.New("foo error")
	errBar := errors.New("bar error")
	for i := 0; i < 1000; i++ {
		wg.Add(2)
		go func() {
			<-storeSignal
			db.Store("foo", errFoo)
			<-loadSignal
			db.Load("foo")
			wg.Done()
		}()
		go func() {
			<-storeSignal
			db.Store("foo", errBar)
			<-loadSignal
			db.Load("foo")
			wg.Done()
		}()
	}

	close(storeSignal)
	close(loadSignal)
	wg.Wait()
}

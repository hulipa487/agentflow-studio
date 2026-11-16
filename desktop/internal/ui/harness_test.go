package ui

import (
	"bytes"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// The app has one UI goroutine and every background completion reaches it
// through onUI. Fyne's headless test driver has none: fyne.Do there runs the
// callback on whichever goroutine called it. Left alone, a finished load
// touches widgets from its own goroutine while the test is reading or laying
// them out, and a goroutine that outlives its test does the same to the next
// one. Both are data races, and the first has crashed a test outright.
//
// So in this package's tests the running test is the UI goroutine. A test
// calls claimUI; from then on a callback from any other goroutine is queued,
// and runs only when the test pumps the queue. A callback raised on the test's
// own goroutine still runs at once, as it would on the real UI goroutine.
//
// Ownership passes from one claiming test to the next and is never released in
// between. A goroutine that outlives its test therefore still addresses that
// test's queue, which is marked dead when the test ends: its callbacks are
// dropped instead of running under the next test.

// Unlike the test driver's own fyne.Do, a queued callback is asynchronous, as
// it is in the app. Code that calls onUI and then reads what the callback was
// supposed to set is wrong in the app too, and here it shows.

// uiQueue is one test's claim on the UI.
type uiQueue struct {
	goid  uint64
	calls chan func()
	// dead is closed when the test ends. A callback arriving after that is
	// dropped, and a goroutine waiting for one stops waiting.
	dead chan struct{}
}

var uiOwner atomic.Pointer[uiQueue]

func init() {
	run := dispatchUI
	uiRunner.Store(&run)
}

func dispatchUI(fn func(), wait bool) {
	q := uiOwner.Load()
	switch {
	case q == nil:
		// No test has claimed the UI yet (a test that builds no widgets).
		fn()
	case goroutineID() == q.goid:
		fn()
	default:
		done := make(chan struct{})
		call := func() {
			defer close(done)
			fn()
		}
		select {
		case <-q.dead:
			return
		case q.calls <- call:
		default:
			return // a queue nobody is draining; treat it as dead
		}
		if wait {
			select {
			case <-done:
			case <-q.dead:
			}
		}
	}
}

// claimUI makes the calling test the UI goroutine. Call it before building any
// widget, from the goroutine the test runs on.
func claimUI(t testing.TB) *uiQueue {
	t.Helper()
	q := &uiQueue{goid: goroutineID(), calls: make(chan func(), 4096), dead: make(chan struct{})}
	uiOwner.Store(q)
	t.Cleanup(func() { close(q.dead) })
	return q
}

// pump runs every callback that has arrived.
func (q *uiQueue) pump() {
	for {
		select {
		case fn := <-q.calls:
			fn()
		default:
			return
		}
	}
}

// wait pumps for d: the replacement for sleeping while background work lands.
func (q *uiQueue) wait(d time.Duration) {
	deadline := time.Now().Add(d)
	for {
		q.pump()
		if !time.Now().Before(deadline) {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// waitFor pumps until cond holds or d has passed, and reports whether it held.
func (q *uiQueue) waitFor(d time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(d)
	for {
		q.pump()
		if cond() {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// settle pumps until no callback has arrived for quiet: enough for loads that
// finish at once (an unreachable or local test server) to have all landed.
func (q *uiQueue) settle(quiet time.Duration) {
	last := time.Now()
	for time.Since(last) < quiet {
		select {
		case fn := <-q.calls:
			fn()
			last = time.Now()
		default:
			time.Sleep(2 * time.Millisecond)
		}
	}
}

// goroutineID reads the current goroutine's id from its stack header
// ("goroutine 123 [running]:"). Go offers no API for this on purpose; a test
// harness deciding "is this the test's own goroutine" is the rare fair use.
func goroutineID() uint64 {
	var buf [64]byte
	b := buf[:runtime.Stack(buf[:], false)]
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	if i := bytes.IndexByte(b, ' '); i > 0 {
		b = b[:i]
	}
	id, _ := strconv.ParseUint(string(b), 10, 64)
	return id
}

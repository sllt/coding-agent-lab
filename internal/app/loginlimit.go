package app

import (
	"sync"
	"time"
)

const (
	loginWindow      = 15 * time.Minute
	loginMaxFailures = 5
	loginBaseLock    = time.Minute
	loginMaxLock     = 30 * time.Minute
	// loginGlobalMax bounds failures across all usernames, so guessing many
	// usernames from one loopback client is limited too.
	loginGlobalMax = 50
)

// loginLimiter locks a username after repeated failures. The lock doubles on
// each further failure up to loginMaxLock. State is in memory: a restart
// clears it, which is acceptable for a loopback-only single-admin service.
type loginLimiter struct {
	mu     sync.Mutex
	users  map[string]*loginState
	global []time.Time
	now    func() time.Time
}

type loginState struct {
	failures []time.Time
	until    time.Time
	lockN    int
}

func (l *loginLimiter) clock() time.Time {
	if l.now != nil {
		return l.now()
	}
	return time.Now()
}

func (l *loginLimiter) state(user string) *loginState {
	if l.users == nil {
		l.users = map[string]*loginState{}
	}
	st, ok := l.users[user]
	if !ok {
		st = &loginState{}
		l.users[user] = st
	}
	return st
}

func prune(list []time.Time, cutoff time.Time) []time.Time {
	out := list[:0]
	for _, t := range list {
		if t.After(cutoff) {
			out = append(out, t)
		}
	}
	return out
}

// locked returns how long the caller must wait, or zero.
func (l *loginLimiter) locked(user string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock()
	l.global = prune(l.global, now.Add(-loginWindow))
	if len(l.global) >= loginGlobalMax {
		return l.global[0].Add(loginWindow).Sub(now)
	}
	st := l.state(user)
	if st.until.After(now) {
		return st.until.Sub(now)
	}
	return 0
}

// fail records a failure and returns the new lock duration, if any.
func (l *loginLimiter) fail(user string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock()
	l.global = append(prune(l.global, now.Add(-loginWindow)), now)
	st := l.state(user)
	st.failures = append(prune(st.failures, now.Add(-loginWindow)), now)
	if len(st.failures) < loginMaxFailures {
		return 0
	}
	lock := loginBaseLock << st.lockN
	if lock > loginMaxLock || lock <= 0 {
		lock = loginMaxLock
	}
	st.lockN++
	st.until = now.Add(lock)
	return lock
}

func (l *loginLimiter) succeed(user string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.users, user)
}

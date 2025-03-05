package github_primary_ratelimit

import (
	"log"
	"sync/atomic"
	"time"
)

// XXX: Synchronization Notes
// We need to deal with weak consistency guarantees to start with,
// i.e., resources are allowed to be exausted by different clients concurrently.
// In addition, we allow internal concurrency of requests and responses.
// So, eventual consistency it is.
//
// Performance Considerations:
// 1. Clients assume low cpu utilization around network transmission.
// 2. Clients care about the latency of the overhead,
//	  in terms of both networking and CPU.
// 3. Clients are expected to stop sending requests once the limit is reached.
//	  As a result, overhead when usage>=limit is preferred over when usage<limit.
//
// Opinionated Descisions:
// 1. It is OK to leak (send) a few requests after the limit is reached. It is inevitable anyway.
// 2. It is BAD to leak a lot of requests after the limit, i.e., to avoid banning, spamming, etc.
//
// Effective Implications:
// 1. Prefer atomics over locking, albeit the implied complexity.
// 2. Otherwise, within reasonable limits, prefer simplicity over performance.
// 3. No need for optimizations of rare cases.
//
// Note:
// A lot of the cases where atomic is used below are tricky, meaning,
// there IS a race condition, but the race condition is OK. i.e.,
// it is eventually consistent and does not break the correctness of the module.
//
// Solution Summary:
// hold a map of category => atomic timestamp pointer (atomic.Pointer[SecondsSinceEpoch]).
// on request: block the request if timestamp != nil.
// on response: if limit is reached, set the timestamp and trigger a timer.
// on timer expiration: reset the timestamp back to nil.
// note: in principle, we could use an atomic bool instead of the atomic timestamp,
// 		 but we want to regenerate the bad response during the blockage time.

type SecondsSinceEpoch int64

func (s SecondsSinceEpoch) StartTimer() *time.Timer {
	timeLeft := time.Until(*s.AsTime())
	return time.NewTimer(timeLeft)
}

func (s SecondsSinceEpoch) AsTime() *time.Time {
	t := time.Unix(int64(s), 0)
	return &t
}

// -------------------------
type AtomicTime = atomic.Pointer[SecondsSinceEpoch]

// UpdateContainer is a simple abstraction over HTTP response,
// to isolate the perf-centric state management domain from the rest of the logic.
// It retains the wider-domain terminology of categories,
// but it is just a key-string as far as RateLimitState is concerned.
type UpdateContainer interface {
	GetCatgory() ResourceCategory
	GetResetTime() *SecondsSinceEpoch
}

type RateLimitState struct {
	resetTimeMap map[ResourceCategory]*AtomicTime
}

func NewRateLimitState(categories []ResourceCategory) *RateLimitState {
	resetTimeMap := make(map[ResourceCategory]*AtomicTime)
	for _, category := range categories {
		resetTimeMap[category] = &AtomicTime{}
	}
	return &RateLimitState{
		resetTimeMap: resetTimeMap,
	}
}

func (s *RateLimitState) GetResetTime(category ResourceCategory) *SecondsSinceEpoch {
	resetTime, exists := s.resetTimeMap[category]
	if !exists {
		log.Printf("unexpected category detected: %v. Please open an issue @ go-github-ratelimit", category)
		return nil
	}
	return resetTime.Load()
}

func (s *RateLimitState) Update(config *Config, update UpdateContainer, callbackContext *CallbackContext) *SecondsSinceEpoch {
	category := update.GetCatgory() // TODO detect req-resp category mismatch (and do what?)
	callbackContext.Category = category

	newResetTime := update.GetResetTime()
	if newResetTime == nil {
		// nothing to update on a successful request
		return nil
	}
	callbackContext.ResetTime = newResetTime.AsTime()

	sharedResetTime, exists := s.resetTimeMap[category]
	if !exists {
		// XXX: there is no point in adding it as a new category to the map,
		// 		because we will not detect it anyway. so just trigger and continue.
		config.TriggerUnknownCategory(callbackContext)
		return nil
	}

	// XXX: should hold a ref to the timer to free resources early on-demand.
	//      please open an issue if you actually need it.
	sharedResetTime.Store(newResetTime)
	timer := newResetTime.StartTimer()
	go func(timer *time.Timer, callbackContext CallbackContext) {
		<-timer.C
		sharedResetTime.Store(nil)
		cbContext := &CallbackContext{
			Category:  callbackContext.Category,
			ResetTime: callbackContext.ResetTime,
		}
		config.TriggerLimitReset(cbContext)
	}(timer, *callbackContext)

	return newResetTime
}

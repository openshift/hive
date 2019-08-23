/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"fmt"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/client-go/tools/cache"
)

const (
	// ExpectationsTimeout defines the length of time that a dormant
	// controller will wait for an expectation to be satisfied. It is
	// specifically targeted at the case where some problem prevents an update
	// of expectations, without it the controller could stay asleep forever. This should
	// be set based on the expected latency of watch events.
	ExpectationsTimeout = 5 * time.Minute
)

// Expectations are a way for controllers to tell the controller manager what they expect. eg:
//	Expectations: {
//		controller1: expects  2 adds in 2 minutes
//		controller2: expects  2 dels in 2 minutes
//		controller3: expects -1 adds in 2 minutes => controller3's expectations have already been met
//	}
//
// Implementation:
//	ControlleeExpectation = pair of atomic counters to track controllee's creation/deletion
//	ExpectationsStore = TTLStore + a ControlleeExpectation per controller
//
// * Once set expectations can only be lowered
// * A controller isn't synced till its expectations are either fulfilled, or expire
// * Controllers that don't set expectations will get woken up for every matching controllee

// ExpKeyFunc to parse out the key from a ControlleeExpectation
var ExpKeyFunc = func(obj interface{}) (string, error) {
	if e, ok := obj.(*ControlleeExpectations); ok {
		return e.key, nil
	}
	return "", fmt.Errorf("Could not find key for obj %#v", obj)
}

// ExpectationsInterface is an interface that allows users to set and wait on expectations.
// Only abstracted out for testing.
// Warning: if using KeyFunc it is not safe to use a single ExpectationsInterface with different
// types of controllers, because the keys might conflict across types.
type ExpectationsInterface interface {
	GetExpectations(controllerKey string) (*ControlleeExpectations, bool, error)
	SatisfiedExpectations(controllerKey string) bool
	DeleteExpectations(controllerKey string)
	SetExpectations(controllerKey string, add, del int) error
	ExpectCreations(controllerKey string, adds int) error
	ExpectDeletions(controllerKey string, dels int) error
	CreationObserved(controllerKey string)
	DeletionObserved(controllerKey string)
	RaiseExpectations(controllerKey string, add, del int)
	LowerExpectations(controllerKey string, add, del int)
}

// Expectations is a cache mapping controllers to what they expect to see before being woken up for a sync.
type Expectations struct {
	cache.Store
	logger log.FieldLogger
}

// GetExpectations returns the ControlleeExpectations of the given controller.
func (r *Expectations) GetExpectations(controllerKey string) (*ControlleeExpectations, bool, error) {
	exp, exists, err := r.GetByKey(controllerKey)
	if err == nil && exists {
		return exp.(*ControlleeExpectations), true, nil
	}
	return nil, false, err
}

// DeleteExpectations deletes the expectations of the given controller from the TTLStore.
func (r *Expectations) DeleteExpectations(controllerKey string) {
	if exp, exists, err := r.GetByKey(controllerKey); err == nil && exists {
		if err := r.Delete(exp); err != nil {
			r.logger.WithField("controllerKey", controllerKey).WithError(err).Warning("Error deleting expectations for controller")
		}
	}
}

// SatisfiedExpectations returns true if the required adds/dels for the given controller have been observed.
// Add/del counts are established by the controller at sync time, and updated as controllees are observed by the controller
// manager.
func (r *Expectations) SatisfiedExpectations(controllerKey string) bool {
	logger := r.logger.WithField("controllerKey", controllerKey)
	if exp, exists, err := r.GetExpectations(controllerKey); exists {
		if exp.Fulfilled() {
			logger.Debugf("Controller expectations fulfilled %#v", exp)
			return true
		} else if exp.isExpired() {
			logger.Debugf("Controller expectations expired %#v", exp)
			return true
		} else {
			logger.Debugf("Controller still waiting on expectations %#v", exp)
			return false
		}
	} else if err != nil {
		logger.WithError(err).Warning("Error encountered while checking expectations, forcing sync")
	} else {
		// When a new controller is created, it doesn't have expectations.
		// When it doesn't see expected watch events for > TTL, the expectations expire.
		//	- In this case it wakes up, creates/deletes controllees, and sets expectations again.
		// When it has satisfied expectations and no controllees need to be created/destroyed > TTL, the expectations expire.
		//	- In this case it continues without setting expectations till it needs to create/delete controllees.
		logger.Debug("Controller either never recorded expectations, or the ttl expired.")
	}
	// Trigger a sync if we either encountered and error (which shouldn't happen since we're
	// getting from local store) or this controller hasn't established expectations.
	return true
}

// SetExpectations registers new expectations for the given controller. Forgets existing expectations.
func (r *Expectations) SetExpectations(controllerKey string, add, del int) error {
	exp := &ControlleeExpectations{add: int64(add), del: int64(del), key: controllerKey, timestamp: clock.RealClock{}.Now()}
	r.logger.WithField("controllerKey", controllerKey).Debugf("Setting expectations %#v", exp)
	return r.Add(exp)
}

// ExpectCreations sets the expectations to expect the specified number of
// additions for the controller with the specified key.
func (r *Expectations) ExpectCreations(controllerKey string, adds int) error {
	return r.SetExpectations(controllerKey, adds, 0)
}

// ExpectDeletions sets the expectations to expect the specified number of
// deletions for the controller with the specified key.
func (r *Expectations) ExpectDeletions(controllerKey string, dels int) error {
	return r.SetExpectations(controllerKey, 0, dels)
}

// LowerExpectations decrements the expectation counts of the given
// controller.
func (r *Expectations) LowerExpectations(controllerKey string, add, del int) {
	if exp, exists, err := r.GetExpectations(controllerKey); err == nil && exists {
		exp.Add(int64(-add), int64(-del))
		// The expectations might've been modified since the update on the previous line.
		r.logger.WithField("controllerKey", controllerKey).Debugf("Lowered expectations %#v", exp)
	}
}

// RaiseExpectations increments the expectation counts of the given
// controller.
func (r *Expectations) RaiseExpectations(controllerKey string, add, del int) {
	if exp, exists, err := r.GetExpectations(controllerKey); err == nil && exists {
		exp.Add(int64(add), int64(del))
		// The expectations might've been modified since the update on the previous line.
		r.logger.WithField("controllerKey", controllerKey).Debugf("Raised expectations %#v", exp)
	}
}

// CreationObserved atomically decrements the `add` expectation count of the given controller.
func (r *Expectations) CreationObserved(controllerKey string) {
	r.LowerExpectations(controllerKey, 1, 0)
}

// DeletionObserved atomically decrements the `del` expectation count of the given controller.
func (r *Expectations) DeletionObserved(controllerKey string) {
	r.LowerExpectations(controllerKey, 0, 1)
}

// ControlleeExpectations track controllee creates/deletes.
type ControlleeExpectations struct {
	// Important: Since these two int64 fields are using sync/atomic, they have to be at the top of the struct due to a bug on 32-bit platforms
	// See: https://golang.org/pkg/sync/atomic/ for more information
	add       int64
	del       int64
	key       string
	timestamp time.Time
}

// Add increments the add and del counters.
func (e *ControlleeExpectations) Add(add, del int64) {
	atomic.AddInt64(&e.add, add)
	atomic.AddInt64(&e.del, del)
}

// Fulfilled returns true if this expectation has been fulfilled.
func (e *ControlleeExpectations) Fulfilled() bool {
	// TODO: think about why this line being atomic doesn't matter
	return atomic.LoadInt64(&e.add) <= 0 && atomic.LoadInt64(&e.del) <= 0
}

// GetExpectations returns the add and del expectations of the controllee.
func (e *ControlleeExpectations) GetExpectations() (int64, int64) {
	return atomic.LoadInt64(&e.add), atomic.LoadInt64(&e.del)
}

// TODO: Extend ExpirationCache to support explicit expiration.
// TODO: Make this possible to disable in tests.
// TODO: Support injection of clock.
func (e *ControlleeExpectations) isExpired() bool {
	return clock.RealClock{}.Since(e.timestamp) > ExpectationsTimeout
}

// NewExpectations returns a store for Expectations.
func NewExpectations(logger log.FieldLogger) *Expectations {
	return &Expectations{
		Store:  cache.NewStore(ExpKeyFunc),
		logger: logger,
	}
}

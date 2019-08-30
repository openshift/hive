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
	"sync"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/client-go/tools/cache"
)

// NewFakeExpectationsLookup creates a fake store for Expectations.
func NewFakeExpectationsLookup(ttl time.Duration) (*Expectations, *clock.FakeClock) {
	fakeTime := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	fakeClock := clock.NewFakeClock(fakeTime)
	ttlPolicy := &cache.TTLPolicy{Ttl: ttl, Clock: fakeClock}
	ttlStore := cache.NewFakeExpirationStore(
		ExpKeyFunc, nil, ttlPolicy, fakeClock)
	return &Expectations{ttlStore, log.StandardLogger()}, fakeClock
}

func TestExpectations(t *testing.T) {
	ttl := 30 * time.Second
	e, fakeClock := NewFakeExpectationsLookup(ttl)

	adds, dels := 10, 30
	ownerKey := "owner-key"
	e.SetExpectations(ownerKey, adds, dels)

	var wg sync.WaitGroup
	for i := 0; i < adds+1; i++ {
		wg.Add(1)
		go func() {
			e.CreationObserved(ownerKey)
			wg.Done()
		}()
	}
	wg.Wait()

	if e.SatisfiedExpectations(ownerKey) {
		t.Errorf("Expected expectations to not be satisfied as there are pending delete expectations")
	}

	for i := 0; i < dels+1; i++ {
		wg.Add(1)
		go func() {
			e.DeletionObserved(ownerKey)
			wg.Done()
		}()
	}
	wg.Wait()

	// Expectations have been surpassed
	if exp, exists, err := e.GetExpectations(ownerKey); err == nil && exists {
		add, del := exp.GetExpectations()
		if add != -1 || del != -1 {
			t.Errorf("Unexpected expectations %#v", exp)
		}
	} else {
		t.Errorf("Could not get expectations, exists %v and err %v", exists, err)
	}
	if !e.SatisfiedExpectations(ownerKey) {
		t.Errorf("Expected expecations to be satisfied")
	}

	// Next round of sync, old expectations are cleared
	e.SetExpectations(ownerKey, 1, 2)
	if exp, exists, err := e.GetExpectations(ownerKey); err == nil && exists {
		add, del := exp.GetExpectations()
		if add != 1 || del != 2 {
			t.Errorf("Unexpected expectations %#v", exp)
		}
	} else {
		t.Errorf("Could not get expectations, exists %v and err %v", exists, err)
	}

	// Expectations have expired because of ttl
	fakeClock.Step(ttl + 1)
	if !e.SatisfiedExpectations(ownerKey) {
		t.Errorf("Expectations should have expired but didn't")
	}
}

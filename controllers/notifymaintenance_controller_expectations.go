package controllers

import (
	"fmt"
	"sync"
	"time"
)

const ExpectationsTimeout = 1 * time.Millisecond

type NMControllerExpectationsInterface interface {
	SatisfiedExpectations(fromState, toState, nmName string) bool
	DeleteExpectations()
	SetExpectations(fromState, toState, nmName string)
}

type NMControllerExpectation struct {
	sync.Mutex
	expectations map[string]time.Time
}

func NewNMControllerExpectation() *NMControllerExpectation {
	return &NMControllerExpectation{expectations: make(map[string]time.Time)}
}

func (c *NMControllerExpectation) SetExpectations(fromState, toState, nmName string) {
	c.Lock()
	defer c.Unlock()

	key := fmt.Sprintf("%s/%s/%s", fromState, toState, nmName)
	c.expectations[key] = time.Now()
}

func (c *NMControllerExpectation) SatisfiedExpectations(fromState, toState, nmName string) bool {
	c.Lock()
	defer c.Unlock()

	key := fmt.Sprintf("%s/%s/%s", fromState, toState, nmName)
	exp, exists := c.expectations[key]

	if !exists {
		return false
	}

	if c.isFulfilled(exp) {
		return true
	} else {
		delete(c.expectations, key)
	}

	return false
}

func (c *NMControllerExpectation) isFulfilled(timestamp time.Time) bool {
	return time.Since(timestamp) < ExpectationsTimeout
}

func (c *NMControllerExpectation) isExpired(timestamp time.Time) bool {
	return time.Since(timestamp) > ExpectationsTimeout
}

func (c *NMControllerExpectation) DeleteExpectations() {
	c.Lock()
	defer c.Unlock()

	for key, lastTransition := range c.expectations {
		if c.isExpired(lastTransition) {
			delete(c.expectations, key)
		}
	}
}

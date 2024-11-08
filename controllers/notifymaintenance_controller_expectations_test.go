package controllers

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("NotifyMaintenance controller Expectations", func() {
	var (
		expectation *NMControllerExpectation
	)

	BeforeEach(func() {
		expectation = NewNMControllerExpectation()
	})

	Describe("NotifyMaintenance state transition control", func() {
		It("successful state transition", func() {
			expectation.SetExpectations("state-1", "state-2", "test-transition-1")
			Expect(expectation.SatisfiedExpectations("state-2", "state-3", "test-transition-1")).NotTo(BeTrue())
		})

		It("should prevent second transition", func() {
			expectation.SetExpectations("state-1", "state-2", "test-transition-2")
			Expect(expectation.SatisfiedExpectations("state-1", "state-2", "test-transition-2")).To(BeTrue())
		})

		It("delete expired expectations", func() {
			expectation.SetExpectations("state-1", "state-2", "test-transition-3")
			expectation.SetExpectations("state-2", "state-3", "test-transition-3")
			expectation.SetExpectations("state-3", "state-4", "test-transition-3")
			Expect(expectation.expectations).To(HaveLen(3))
			time.Sleep(1 * time.Millisecond)
			expectation.DeleteExpectations()
			Expect(expectation.expectations).To(HaveLen(0))
		})
	})
})

package utils

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8sv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Util", func() {
	var testNodeName = "test-node"
	var testMaintID = "test-maint-id"

	Context("IsFinal", func() {
		It("should return true if obj has deletionTimestamp", func() {
			n := k8sv1.Node{
				ObjectMeta: v1.ObjectMeta{
					DeletionTimestamp: &v1.Time{Time: time.Now()},
				},
			}
			Expect(IsFinal(&n)).To(BeTrue())
		})
		It("should return false if obj has no deletionTimestamp", func() {
			Expect(IsFinal(&k8sv1.Node{})).To(BeFalse())
		})
	})

	Context("Generate deterministic ID", func() {
		It("should return the same string", func() {
			uid1 := DeterministicUID(testNodeName + "-" + testMaintID)
			uid2 := DeterministicUID(testNodeName + "-" + testMaintID)
			Expect(uid1).To(Equal(uid2))
		})
		It("should return different uids", func() {
			uid1 := DeterministicUID(testNodeName + "-" + testMaintID)
			uid2 := DeterministicUID(testNodeName + "-" + testMaintID + "-" + "different")
			Expect(uid1).To(Not(Equal(uid2)))
		})
	})
})

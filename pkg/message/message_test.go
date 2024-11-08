package message

import (
	"github.com/NVIDIA/pika/api/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("HumanReadable", func() {
	var (
		nm        *v1alpha1.NotifyMaintenance
		namespace string
	)

	BeforeEach(func() {
		namespace = "testns"
		nm = validNotifyMaintenance(namespace)
	})

	Context("with event", func() {
		It("should output SLAStarted human readable event", func() {
			Expect(HumanReadable(nm, MessageEventSLAStarted)).Should(Equal("Maintenance test_id started on [test_node]"))
		})

		It("test MaintenanceComplete readable event", func() {
			Expect(HumanReadable(nm, MessageEventMaintenanceComplete)).Should(Equal("Maintenance test_id completed on [test_node]"))
		})

		It("test MaintenanceCancelled readable event", func() {
			Expect(HumanReadable(nm, MessageEventMaintenanceCancelled)).Should(Equal("Maintenance test_id cancelled on [test_node]"))
		})
	})
})

var _ = Describe("MachineReadable", func() {
	var (
		nm        *v1alpha1.NotifyMaintenance
		namespace string
	)

	BeforeEach(func() {
		namespace = "testns"
		nm = validNotifyMaintenance(namespace)
	})

	Context("with event", func() {
		It("should output valid JSON on ObjectsDrained event", func() {
			Expect(MachineReadable(nm, MessageEventObjectsDrained)).Should(Equal(`{"maintenanceID":"test_id","objects":["test_node"],"event":"ObjectsDrained","Message":"Maintenance test_id objects drained on [test_node]","namespace":"testns","maintenanceObjectName":"test","owner":"natalie_bandel"}`))
		})
		It("should produce a valid JSON on empty owner", func() {
			nm.Spec.Owner = ""
			Expect(MachineReadable(nm, MessageEventObjectsDrained)).Should(Equal(`{"maintenanceID":"test_id","objects":["test_node"],"event":"ObjectsDrained","Message":"Maintenance test_id objects drained on [test_node]","namespace":"testns","maintenanceObjectName":"test"}`))
		})
		It("should produce a valid JSON on non-empty MetadataConfigmap", func() {
			nm.Spec.MetadataConfigmap = "test-configmap"
			if nm.Annotations == nil {
				nm.Annotations = map[string]string{}
			}
			Expect(MachineReadable(nm, MessageEventObjectsDrained)).Should(Equal(`{"maintenanceID":"test_id","objects":["test_node"],"event":"ObjectsDrained","Message":"Maintenance test_id objects drained on [test_node]","namespace":"testns","maintenanceObjectName":"test","metadataConfigmap":"test-configmap","owner":"natalie_bandel"}`))
		})

		It("should produce an error on unsupported event", func() {
			str, err := MachineReadable(nm, "MaintenanceWaiting")
			Expect(str).Should(Equal(""))
			Expect(err).Should(MatchError("event type MaintenanceWaiting is not supported"))
		})
	})
})

func validNotifyMaintenance(namespace string) *v1alpha1.NotifyMaintenance {
	return &v1alpha1.NotifyMaintenance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: namespace,
		},
		Spec: v1alpha1.NotifyMaintenanceSpec{
			MaintenanceID: "test_id",
			NodeObject:    "test_node",
			Type:          v1alpha1.PlannedMaintenance,
			Owner:         "natalie_bandel",
		},
		Status: v1alpha1.NotifyMaintenanceStatus{},
	}
}

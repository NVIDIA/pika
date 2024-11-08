package v1alpha1

import (
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("NotifyMaintenance validation webhook", func() {

	var ns = "namespace"

	BeforeEach(func() {
		nmList := NotifyMaintenanceList{}
		err := k8sClient.List(ctx, &nmList)
		Expect(err).ToNot(HaveOccurred())
		Expect(nmList.Items).To(BeEmpty())
	})

	AfterEach(func() {
		err := k8sClient.DeleteAllOf(ctx, &NotifyMaintenance{}, client.InNamespace(ns))
		Expect(err).ToNot(HaveOccurred())
	})

	Context("validate update", func() {
		It("should reject change of object", func() {
			nmName := uuid.New().String()
			nm := NotifyMaintenance{
				ObjectMeta: v1.ObjectMeta{
					Name:      nmName,
					Namespace: ns,
				},
				Spec: NotifyMaintenanceSpec{
					NodeObject:    "node1a",
					MaintenanceID: "id",
					Type:          PlannedMaintenance,
				},
			}
			Expect(k8sClient.Create(ctx, &nm)).To(Succeed())

			nm.Spec.NodeObject = "node2a"
			Expect(k8sClient.Update(ctx, &nm)).ToNot(Succeed())
		})

		It("should reject change of annotation value from the final state", func() {
			nmName := uuid.New().String()
			nm := NotifyMaintenance{
				ObjectMeta: v1.ObjectMeta{
					Name:      nmName,
					Namespace: ns,
				},
				Spec: NotifyMaintenanceSpec{
					NodeObject:    "node1a",
					MaintenanceID: "id",
					Type:          PlannedMaintenance,
				},
			}
			nm.Annotations = map[string]string{}
			nm.Annotations[MaintenanceClientAnnotation.Key] = MaintenanceClientIncomplete
			Expect(k8sClient.Create(ctx, &nm)).To(Succeed())

			nm.Annotations[MaintenanceClientAnnotation.Key] = MaintenanceClientInprogress
			Expect(k8sClient.Update(ctx, &nm)).ToNot(Succeed())
		})

		It("should successfully update the annotation value from the non-final state", func() {
			nmName := uuid.New().String()
			nm := NotifyMaintenance{
				ObjectMeta: v1.ObjectMeta{
					Name:      nmName,
					Namespace: ns,
				},
				Spec: NotifyMaintenanceSpec{
					NodeObject:    "node1a",
					MaintenanceID: "id",
					Type:          PlannedMaintenance,
				},
			}
			nm.Annotations = map[string]string{}
			nm.Annotations["MyRandomAnnotationKey"] = "MyRandomAnnotationValue0"
			nm.Annotations[MaintenanceClientAnnotation.Key] = MaintenanceClientInprogress
			Expect(k8sClient.Create(ctx, &nm)).To(Succeed())

			nm.Annotations["MyRandomAnnotationKey"] = "MyRandomAnnotationValue001"
			nm.Annotations[ValidationClientAnnotation.Key] = ValidationClientInProgress
			nm.Annotations["MyRandomAnnotationKey2"] = "MyRandomAnnotationValue002"
			nm.Annotations[MaintenanceClientAnnotation.Key] = MaintenanceClientComplete
			nm.Annotations["MyRandomAnnotationKey3"] = "MyRandomAnnotationValue3"
			Expect(k8sClient.Update(ctx, &nm)).To(Succeed())
		})

		It("should reject change of annotation value from the MaintenanceClientIncomplete to MaintenanceClientComplete", func() {
			nmName := uuid.New().String()
			nm := NotifyMaintenance{
				ObjectMeta: v1.ObjectMeta{
					Name:      nmName,
					Namespace: ns,
				},
				Spec: NotifyMaintenanceSpec{
					NodeObject:    "node1a",
					MaintenanceID: "id",
					Type:          PlannedMaintenance,
				},
			}
			nm.Annotations = map[string]string{}
			nm.Annotations[MaintenanceClientAnnotation.Key] = MaintenanceClientIncomplete
			Expect(k8sClient.Create(ctx, &nm)).To(Succeed())
			nm.Annotations[MaintenanceClientAnnotation.Key] = MaintenanceClientComplete
			Expect(k8sClient.Update(ctx, &nm)).ToNot(Succeed())
		})
	})

	Context("validate create", func() {

		It("should reject creation of NotifyMaintenance object if another NM ojbect with the same MaintenanceID and same NodeObject already exist", func() {
			nmName := uuid.New().String()
			nm := NotifyMaintenance{
				ObjectMeta: v1.ObjectMeta{
					Name:      nmName,
					Namespace: ns,
				},
				Spec: NotifyMaintenanceSpec{
					NodeObject:    "node1a",
					MaintenanceID: "id",
					Type:          PlannedMaintenance,
				},
			}
			Expect(k8sClient.Create(ctx, &nm)).To(Succeed())

			nmName2 := uuid.New().String()
			nm2 := NotifyMaintenance{
				ObjectMeta: v1.ObjectMeta{
					Name:      nmName2,
					Namespace: ns,
				},
				Spec: NotifyMaintenanceSpec{
					NodeObject:    "node1a",
					MaintenanceID: "id",
					Type:          PlannedMaintenance,
				},
			}
			Expect(k8sClient.Create(ctx, &nm2)).NotTo(Succeed())
		})

		It("should succeed to create NotifyMaintenance object if another NM ojbect with the same MaintenanceID and different NodeObject already exist", func() {
			nmName := uuid.New().String()
			nm := NotifyMaintenance{
				ObjectMeta: v1.ObjectMeta{
					Name:      nmName,
					Namespace: ns,
				},
				Spec: NotifyMaintenanceSpec{
					NodeObject:    "node1a",
					MaintenanceID: "id",
					Type:          PlannedMaintenance,
				},
			}
			Expect(k8sClient.Create(ctx, &nm)).To(Succeed())

			nmName2 := uuid.New().String()
			nm2 := NotifyMaintenance{
				ObjectMeta: v1.ObjectMeta{
					Name:      nmName2,
					Namespace: ns,
				},
				Spec: NotifyMaintenanceSpec{
					NodeObject:    "node2a",
					MaintenanceID: "id",
					Type:          PlannedMaintenance,
				},
			}
			Expect(k8sClient.Create(ctx, &nm2)).To(Succeed())
		})

		It("should accept NotifyMaintenances with empty metadataConfigmap", func() {
			nmName := uuid.New().String()
			nm := NotifyMaintenance{
				ObjectMeta: v1.ObjectMeta{
					Name:      nmName,
					Namespace: ns,
				},
				Spec: NotifyMaintenanceSpec{
					NodeObject:    "node1a",
					MaintenanceID: "id",
					Type:          PlannedMaintenance,
				},
			}
			Expect(k8sClient.Create(ctx, &nm)).To(Succeed())
		})

		It("should reject NotifyMaintenances with invalid metadataConfigmap", func() {
			nmName := uuid.New().String()
			nm := NotifyMaintenance{
				ObjectMeta: v1.ObjectMeta{
					Name:      nmName,
					Namespace: ns,
				},
				Spec: NotifyMaintenanceSpec{
					NodeObject:        "node1a",
					MaintenanceID:     "id",
					Type:              PlannedMaintenance,
					MetadataConfigmap: "configmap-does-not-exist-in-namespace",
				},
			}
			Expect(k8sClient.Create(ctx, &nm)).NotTo(Succeed())
		})

		It("should reject NotifyMaintenances with empty .spec.objects", func() {
			nm := NotifyMaintenance{
				Spec: NotifyMaintenanceSpec{},
			}
			Expect(k8sClient.Create(ctx, &nm)).NotTo(Succeed())
		})
		It("should reject NotifyMaintenances with invalid nodes", func() {
			nm := NotifyMaintenance{
				Spec: NotifyMaintenanceSpec{
					NodeObject: "node-does-not-exist",
				},
			}
			Expect(k8sClient.Create(ctx, &nm)).NotTo(Succeed())
		})
		It("should validate maintenance ID", func() {
			nm := NotifyMaintenance{
				Spec: NotifyMaintenanceSpec{
					NodeObject:    "node-does-not-exist",
					MaintenanceID: "!nv4l1d",
				},
			}
			Expect(k8sClient.Create(ctx, &nm)).NotTo(Succeed())
		})
		It("should reject a NotifyMaintenance object with an invalid MaintenanceType", func() {
			nm := NotifyMaintenance{
				Spec: NotifyMaintenanceSpec{
					NodeObject:    "node1a",
					MaintenanceID: "id",
					Type:          "invalidType",
				},
			}
			Expect(k8sClient.Create(ctx, &nm)).ToNot(Succeed())
		})

		It("should allow the creation of NotifyMaintenances", func() {
			nm := NotifyMaintenance{
				ObjectMeta: v1.ObjectMeta{
					Name:      uuid.New().String(),
					Namespace: ns,
				},
				Spec: NotifyMaintenanceSpec{
					NodeObject:    "node3a",
					MaintenanceID: "id",
					Type:          PlannedMaintenance,
				},
			}
			Expect(k8sClient.Create(ctx, &nm)).To(Succeed())
		})

		It("should create a NotifyMaintenance object with validated and non-validated annotations", func() {
			nm := NotifyMaintenance{
				ObjectMeta: v1.ObjectMeta{
					Name:      uuid.New().String(),
					Namespace: ns,
				},
				Spec: NotifyMaintenanceSpec{
					NodeObject:    "node3a",
					MaintenanceID: "id",
					Type:          PlannedMaintenance,
				},
			}
			nm.Annotations = map[string]string{}
			nm.Annotations["MyRandomAnnotationKey"] = "MyRandomAnnotationValue1"
			nm.Annotations[ValidationClientAnnotation.Key] = ValidationClientInProgress
			nm.Annotations["MyRandomAnnotationKey2"] = "MyRandomAnnotationValue020"
			nm.Annotations[MaintenanceClientAnnotation.Key] = MaintenanceClientIncomplete
			nm.Annotations["MyRandomAnnotationKey3"] = "MyRandomAnnotationValue3"
			Expect(k8sClient.Create(ctx, &nm)).To(Succeed())
		})

		It("should reject a NotifyMaintenance object with an invalid Annotation value for MaintenanceClient annotation key", func() {
			nm := NotifyMaintenance{
				ObjectMeta: v1.ObjectMeta{
					Name:      uuid.New().String(),
					Namespace: ns,
				},
				Spec: NotifyMaintenanceSpec{
					NodeObject:    "node3a",
					MaintenanceID: "id",
					Type:          PlannedMaintenance,
				},
			}
			nm.Annotations = map[string]string{}
			nm.Annotations["MyRandomAnnotationKey"] = "MyRandomAnnotationValue01"
			nm.Annotations[MaintenanceClientAnnotation.Key] = "invalidValue"
			Expect(k8sClient.Create(ctx, &nm)).ToNot(Succeed())
		})

		It("should reject a NotifyMaintenance object with an invalid Annotation value for ValidationClient annotation key", func() {
			nm := NotifyMaintenance{
				ObjectMeta: v1.ObjectMeta{
					Name:      uuid.New().String(),
					Namespace: ns,
				},
				Spec: NotifyMaintenanceSpec{
					NodeObject:    "node3a",
					MaintenanceID: "id",
					Type:          PlannedMaintenance,
				},
			}
			nm.Annotations = map[string]string{}
			nm.Annotations["MyRandomAnnotationKey"] = "MyRandomAnnotationValue011"
			nm.Annotations[ValidationClientAnnotation.Key] = "invalidValue"
			nm.Annotations["MyRandomAnnotationKey2"] = "MyRandomAnnotationValue200"
			Expect(k8sClient.Create(ctx, &nm)).ToNot(Succeed())
		})
	})

})

var _ = Describe("Notifymaintenance", func() {
	nm := NotifyMaintenance{
		ObjectMeta: v1.ObjectMeta{
			Name:      "foo",
			Namespace: "bar",
		},
		Status: NotifyMaintenanceStatus{
			SLAExpires: &v1.Time{
				Time: time.Now().Add(-time.Hour),
			},
		},
	}
	It("should report if SLA is expired", func() {
		Expect(nm.Status.IsSLAExpired()).To(BeTrue())

		nm.Status.SLAExpires = &v1.Time{
			Time: time.Now().Add(time.Hour),
		}
		Expect(nm.Status.IsSLAExpired()).To(BeFalse())
	})

	It("should report it's name in string", func() {
		Expect(nm.String()).To(Equal("bar/foo"))
	})
})

package controllers

import (
	"context"
	"errors"
	"fmt"
	"time"

	ngn2v1alpha1 "github.com/NVIDIA/pika/api/v1alpha1"
	"github.com/NVIDIA/pika/pkg/utils"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8sv1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("NotifyMaintenanceReconciler", func() {
	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("SLATimer", func() {
		var (
			slatimer *slaTimer
			objName  string
			nm2      *ngn2v1alpha1.NotifyMaintenance
		)
		BeforeEach(func() {
			slatimer = newSlaTimer(nmRec)
			objName = uuid.New().String()
			nm = newNotifyMaintenance(objName)
			nm2 = newNotifyMaintenance(objName)
			nm.Status.SLAExpires = &metav1.Time{Time: time.Now().Add(time.Hour)}
			nm2.Status.SLAExpires = nm.Status.SLAExpires.DeepCopy()
			slatimer.StartCountDown(nm)
			slatimer.StartCountDown(nm2)
		})

		It("should start countdown", func() {
			nodeInfo, registered := slatimer.nodeInfoMap[objName]
			Expect(registered).To(BeTrue())
			Expect(nodeInfo.nmSet.Contains(nm.String(), nm2.String())).To(BeTrue())
			Expect(slatimer.HasCountDownBegun(nm)).To(BeTrue())
			Expect(slatimer.HasCountDownBegun(nm2)).To(BeTrue())
		})

		It("should abort countdown", func() {
			slatimer.unregisterCountDown(nm)
			nodeInfo, registered := slatimer.nodeInfoMap[objName]
			Expect(registered).To(BeTrue())
			Expect(nodeInfo.nmSet.Contains(nm.String())).To(BeFalse())

			slatimer.unregisterCountDown(nm2)
			_, registered = slatimer.nodeInfoMap[objName]
			Expect(registered).To(BeFalse())

			Expect(slatimer.HasCountDownBegun(nm)).To(BeFalse())
			Expect(slatimer.HasCountDownBegun(nm2)).To(BeFalse())
		})
	})

	Context("Start Maintenance", func() {
		BeforeEach(func() {
			testNode = newFakeNode()
			Expect(k8sClient.Create(ctx, testNode)).Should(Succeed())

			object = testNode.Name
			nm = newNotifyMaintenance(object)
			namespaceName = types.NamespacedName{
				Namespace: testNamespace,
				Name:      nm.Name,
			}
			nm2 = newNotifyMaintenance(object)
			namespaceName2 = types.NamespacedName{
				Namespace: testNamespace,
				Name:      nm2.Name,
			}
			nm2.Spec.MaintenanceID = newMaintenanceID

			notifier.EXPECT().GetNotifierName().AnyTimes()
			notifier.EXPECT().NotifySLAStart(gomock.Any(), gomock.Any()).MaxTimes(2)
		})

		AfterEach(func() {
			nmRec.config.SLAPeriod = 8 * time.Hour
		})

		ctx := context.Background()

		var waitForNMInit = func(nm *ngn2v1alpha1.NotifyMaintenance) *ngn2v1alpha1.NotifyMaintenance {
			createdNotify := &ngn2v1alpha1.NotifyMaintenance{}
			nsName := types.NamespacedName{
				Namespace: testNamespace,
				Name:      nm.Name,
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, nsName, createdNotify)
				if err != nil {
					return false
				}
				_, labelExists := createdNotify.Labels[utils.MAINTENANCE_LABEL_PREFIX+nm.Spec.MaintenanceID]
				return labelExists &&
					(createdNotify.Status.MaintenanceStatus == ngn2v1alpha1.MaintenanceScheduled || createdNotify.Status.MaintenanceStatus == ngn2v1alpha1.MaintenanceStarted)
			}, timeout, interval).Should(BeTrue())

			return createdNotify
		}

		var addAndVerifyAnnotation = func(nm *ngn2v1alpha1.NotifyMaintenance, annotationKey string, conditionKey string, val string) {
			// add an annotation to NotifyMaintenance which triggers reconcile.
			if nm.Annotations == nil {
				nm.Annotations = map[string]string{}
			}
			nm.Annotations[annotationKey] = val

			err = k8sClient.Update(context.Background(), nm)
			Expect(err).NotTo(HaveOccurred())

			SleepForReconcile()
			err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(nm), nm)
			Expect(err).NotTo(HaveOccurred())

			// Expect one condition to exist
			Expect(nm.Status.Conditions).ShouldNot(BeEmpty())
			condStatus := ""
			for _, cond := range nm.Status.Conditions {
				if cond.Type == conditionKey {
					condStatus = cond.Status
				}
			}

			Expect(condStatus).To(Equal(val))
		}

		var addMaintenanceClientInProgressAnnotation = func(nm *ngn2v1alpha1.NotifyMaintenance) {
			addAndVerifyAnnotation(nm, ngn2v1alpha1.MaintenanceClientAnnotation.Key, ngn2v1alpha1.MaintenanceClientConditionType, ngn2v1alpha1.MaintenanceClientInprogress)
		}

		var addValidationClientInProgressAnnotation = func(nm *ngn2v1alpha1.NotifyMaintenance) {
			addAndVerifyAnnotation(nm, ngn2v1alpha1.ValidationClientAnnotation.Key, ngn2v1alpha1.ValidationClientConditionType, ngn2v1alpha1.ValidationClientInProgress)
		}

		var fullMaintenanceCycle = func(annotationKey, annotationVal string) {
			nmRec.config.SLAPeriod = 3 * time.Second

			By("ensuring .status and label updates")
			Expect(k8sClient.Create(ctx, nm)).Should(Succeed())
			createdNotify := waitForNMInit(nm)

			By("tainting the node")
			Expect(taintNode(testNode)).To(Succeed())
			createdNode := k8sv1.Node{}
			Eventually(func() bool {
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(testNode), &createdNode)
				Expect(err).ToNot(HaveOccurred())
				if createdNode.Spec.Unschedulable {
					return true
				}
				for _, taint := range createdNode.Spec.Taints {
					if taint.Effect == k8sv1.TaintEffectNoSchedule {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			By("ensuring .status.maintenanceStatus changes to MaintenanceStarted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, namespaceName, createdNotify)
				if err != nil {
					return false
				}
				return createdNotify.Status.MaintenanceStatus == ngn2v1alpha1.MaintenanceStarted
			}, timeout, interval).Should(BeTrue())

			By("ensuring .status.maintenanceStatus changes to SLAExpired")
			notifier.EXPECT().NotifySLAExpire(gomock.Any(), gomock.Any()).AnyTimes()
			Eventually(func() bool {
				err := k8sClient.Get(ctx, namespaceName, createdNotify)
				if err != nil {
					return false
				}
				return createdNotify.Status.MaintenanceStatus == ngn2v1alpha1.SLAExpired
			}, timeout, interval).Should(BeTrue())

			By("ensuring .status.MaintenanceStatus changes to ObjectsDrained")
			notifier.EXPECT().NotifyNodeDrain(gomock.Any(), gomock.Any()).AnyTimes()

			MarkNMReadyForMaintenance(createdNotify)

			Eventually(func() bool {
				err := k8sClient.Get(ctx, namespaceName, createdNotify)
				if err != nil {
					return false
				}
				return createdNotify.Status.MaintenanceStatus == ngn2v1alpha1.ObjectsDrained
			}, timeout, interval).Should(BeTrue())

			By("adding Maintenance Client In Progress annotation on the NotifyMaintenance object")
			addMaintenanceClientInProgressAnnotation(createdNotify)

			Consistently(func() bool {
				err := k8sClient.Get(ctx, namespaceName, createdNotify)
				if err != nil {
					return false
				}
				return createdNotify.Status.MaintenanceStatus == ngn2v1alpha1.ObjectsDrained
			}, timeout, interval).Should(BeTrue())

			notifier.EXPECT().NotifyValidating(gomock.Any(), gomock.Any()).AnyTimes()

			if annotationKey == ngn2v1alpha1.MaintenanceClientAnnotation.Key {
				By("adding a Maintenance Client annotation on the NotifyMaintenance object")
				notifier.EXPECT().NotifyMaintenanceIncomplete(gomock.Any(), gomock.Any())

				AddAnnotation(createdNotify, annotationKey, annotationVal)

				Eventually(func() bool {
					err := k8sClient.Get(ctx, namespaceName, createdNotify)
					if err != nil {
						return false
					}
					return createdNotify.Status.MaintenanceStatus == ngn2v1alpha1.MaintenanceIncomplete
				}, timeout, interval).Should(BeTrue())

				Expect(createdNotify.Status.TransitionTimestamps).To(HaveLen(5))
				Expect(createdNotify.Status.TransitionTimestamps[0].MaintenanceStatus).To(Equal(ngn2v1alpha1.MaintenanceScheduled))
				Expect(createdNotify.Status.TransitionTimestamps[1].MaintenanceStatus).To(Equal(ngn2v1alpha1.MaintenanceStarted))
				Expect(createdNotify.Status.TransitionTimestamps[2].MaintenanceStatus).To(Equal(ngn2v1alpha1.SLAExpired))
				Expect(createdNotify.Status.TransitionTimestamps[3].MaintenanceStatus).To(Equal(ngn2v1alpha1.ObjectsDrained))
				Expect(createdNotify.Status.TransitionTimestamps[4].MaintenanceStatus).To(Equal(ngn2v1alpha1.MaintenanceIncomplete))

			}
			if annotationKey == ngn2v1alpha1.ValidationClientAnnotation.Key {
				By("adding a Maintenance Client complete annotation on the NotifyMaintenance object")
				notifier.EXPECT().NotifyValidating(gomock.Any(), gomock.Any()).AnyTimes()
				AddAnnotation(createdNotify, ngn2v1alpha1.MaintenanceClientAnnotation.Key, ngn2v1alpha1.MaintenanceClientComplete)

				Eventually(func() bool {
					err := k8sClient.Get(ctx, namespaceName, createdNotify)
					if err != nil {
						return false
					}
					return createdNotify.Status.MaintenanceStatus == ngn2v1alpha1.Validating
				}, timeout, interval).Should(BeTrue())

				By("adding a Validation Client In Progress annotation on the NotifyMaintenance object")
				addValidationClientInProgressAnnotation(createdNotify)

				By("adding a Validation Client " + annotationVal + " annotation on the NotifyMaintenance object")
				notifier.EXPECT().NotifyMaintenanceIncomplete(gomock.Any(), gomock.Any())
				AddAnnotation(createdNotify, annotationKey, annotationVal)

				Eventually(func() bool {
					err := k8sClient.Get(ctx, namespaceName, createdNotify)
					if err != nil {
						return false
					}
					return createdNotify.Status.MaintenanceStatus == ngn2v1alpha1.MaintenanceIncomplete
				}, timeout, interval).Should(BeTrue())

				Expect(createdNotify.Status.TransitionTimestamps).To(HaveLen(6))
				Expect(createdNotify.Status.TransitionTimestamps[0].MaintenanceStatus).To(Equal(ngn2v1alpha1.MaintenanceScheduled))
				Expect(createdNotify.Status.TransitionTimestamps[1].MaintenanceStatus).To(Equal(ngn2v1alpha1.MaintenanceStarted))
				Expect(createdNotify.Status.TransitionTimestamps[2].MaintenanceStatus).To(Equal(ngn2v1alpha1.SLAExpired))
				Expect(createdNotify.Status.TransitionTimestamps[3].MaintenanceStatus).To(Equal(ngn2v1alpha1.ObjectsDrained))
				Expect(createdNotify.Status.TransitionTimestamps[4].MaintenanceStatus).To(Equal(ngn2v1alpha1.Validating))
				Expect(createdNotify.Status.TransitionTimestamps[5].MaintenanceStatus).To(Equal(ngn2v1alpha1.MaintenanceIncomplete))
			}

			By("ensuring tenants are notified and the NotifyMaintenance is deleted when the node is uncordoned")
			notifier.EXPECT().NotifyMaintenanceEnded(gomock.Any(), gomock.Any()).AnyTimes()
			Expect(k8sClient.Delete(ctx, createdNotify)).To(Succeed())

			SleepForReconcile()

			notifier.EXPECT().NotifyMaintenanceCompleted(gomock.Any(), gomock.Any()).Times(1)
			Expect(uncordonNode(testNode)).To(Succeed())

			// ensure the NotifyMaintenance is deleted
			Eventually(func() bool {
				err := k8sClient.Get(ctx, namespaceName, createdNotify)
				return k8serrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			// ensure labels are removed
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testNode), &createdNode)
				if err != nil {
					return false
				}
				_, maintenanceLabelExists := createdNode.Labels[utils.MAINTENANCE_LABEL_PREFIX+maintenanceID]
				_, newMaintenanceLabelExists := createdNode.Labels[utils.MAINTENANCE_LABEL_PREFIX+newMaintenanceID]
				return !maintenanceLabelExists && !newMaintenanceLabelExists
			}, timeout, interval).Should(BeTrue())
		}

		var completeCycleWithTwoNms = func() {
			nmRec.config.SLAPeriod = 3 * time.Second

			By("ensuring .status and label updates")
			Expect(k8sClient.Create(ctx, nm)).Should(Succeed())
			createdNotify := waitForNMInit(nm)

			By("tainting the node")
			Expect(taintNode(testNode)).To(Succeed())
			createdNode := k8sv1.Node{}
			Eventually(func() bool {
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(testNode), &createdNode)
				Expect(err).ToNot(HaveOccurred())
				if createdNode.Spec.Unschedulable {
					return true
				}
				for _, taint := range createdNode.Spec.Taints {
					if taint.Effect == k8sv1.TaintEffectNoSchedule {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			By("ensuring .status.maintenanceStatus changes to MaintenanceStarted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, namespaceName, createdNotify)
				if err != nil {
					return false
				}
				return createdNotify.Status.MaintenanceStatus == ngn2v1alpha1.MaintenanceStarted
			}, timeout, interval).Should(BeTrue())

			By("ensuring .status.maintenanceStatus changes to SLAExpired")
			notifier.EXPECT().NotifySLAExpire(gomock.Any(), gomock.Any()).AnyTimes()
			Eventually(func() bool {
				err := k8sClient.Get(ctx, namespaceName, createdNotify)
				if err != nil {
					return false
				}
				return createdNotify.Status.MaintenanceStatus == ngn2v1alpha1.SLAExpired
			}, timeout, interval).Should(BeTrue())

			By("creating a maintenance window")
			By("creating a second nm")
			Expect(k8sClient.Create(ctx, nm2)).Should(Succeed())
			createdNotify2 := waitForNMInit(nm2)

			By("labeling the node with the second nm")
			notifier.EXPECT().NotifySLAExpire(gomock.Any(), gomock.Any()).AnyTimes()
			Expect(labelNode(testNode, nm2)).To(Succeed())

			By("ensuring .status.maintenanceStatus changes to SLAExpired for the second nm")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, namespaceName2, createdNotify2)
				if err != nil {
					return false
				}
				return createdNotify2.Status.MaintenanceStatus == ngn2v1alpha1.SLAExpired
			}, timeout, interval).Should(BeTrue())

			By("ensuring .status.MaintenanceStatus changes to ObjectsDrained")
			notifier.EXPECT().NotifyNodeDrain(gomock.Any(), gomock.Any()).Times(2)

			MarkNMReadyForMaintenance(createdNotify)

			Eventually(func() bool {
				err := k8sClient.Get(ctx, namespaceName, createdNotify)
				if err != nil {
					return false
				}
				return createdNotify.Status.MaintenanceStatus == ngn2v1alpha1.ObjectsDrained
			}, timeout, interval).Should(BeTrue())

			By("adding Maintenance Client In Progress annotation on the first NotifyMaintenance object")
			addMaintenanceClientInProgressAnnotation(createdNotify)
			By("adding the same annotation one more time on the first NotifyMaintenance object")
			addMaintenanceClientInProgressAnnotation(createdNotify)

			By("ensuring notification when the first Maintenance has Ended")
			notifier.EXPECT().NotifyMaintenanceEnded(gomock.Any(), gomock.Any()).AnyTimes()
			Expect(k8sClient.Delete(ctx, createdNotify)).To(Succeed())

			SleepForReconcile()

			MarkNMReadyForMaintenance(createdNotify2)

			Eventually(func() bool {
				err := k8sClient.Get(ctx, namespaceName, createdNotify2)
				if err != nil {
					return false
				}
				return createdNotify2.Status.MaintenanceStatus == ngn2v1alpha1.ObjectsDrained
			}, timeout, interval).Should(BeTrue())

			By("adding Maintenance Client In Progress annotation on the second NotifyMaintenance object")
			addMaintenanceClientInProgressAnnotation(createdNotify2)
			By("adding the same annotation one more time on the second NotifyMaintenance object")
			addMaintenanceClientInProgressAnnotation(createdNotify2)

			By("ensuring notification when the second Maintenance has Ended")
			notifier.EXPECT().NotifyMaintenanceEnded(gomock.Any(), gomock.Any()).AnyTimes()
			Expect(k8sClient.Delete(ctx, createdNotify2)).To(Succeed())
			notifier.EXPECT().NotifyMaintenanceCompleted(gomock.Any(), gomock.Any()).Times(2)

			Expect(uncordonNode(testNode)).To(Succeed())
			SleepForReconcile()

			// ensure the NotifyMaintenances is deleted
			Eventually(func() bool {
				err := k8sClient.Get(ctx, namespaceName, createdNotify)
				return k8serrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, namespaceName, createdNotify2)
				return k8serrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			SleepForReconcile()

			// ensure labels are removed
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testNode), &createdNode)
				if err != nil {
					return false
				}
				_, maintenanceLabelExists := createdNode.Labels[utils.MAINTENANCE_LABEL_PREFIX+maintenanceID]
				_, newMaintenanceLabelExists := createdNode.Labels[utils.MAINTENANCE_LABEL_PREFIX+newMaintenanceID]
				return !maintenanceLabelExists && !newMaintenanceLabelExists
			}, timeout, interval).Should(BeTrue())
		}

		It("should rotate certs and continue", func() {
			Expect(taintNode(testNode)).To(Succeed())

			By("creating a nm")
			Expect(k8sClient.Create(ctx, nm)).Should(Succeed())
			createdNotify := waitForNMInit(nm)

			By("skiping the SLA period")
			createdNotify.Status.MaintenanceStatus = ngn2v1alpha1.MaintenanceStarted
			Expect(k8sClient.Status().Update(ctx, createdNotify)).To(Succeed())

			By("rotating certs")
			notifier.EXPECT().NotifyNodeDrain(gomock.Any(), gomock.Any()).Return(errors.New("403"))
			notifier.EXPECT().RotateCert().Times(1)
			notifier.EXPECT().NotifyNodeDrain(gomock.Any(), gomock.Any()).Times(2)

			MarkNMReadyForMaintenance(nm)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, namespaceName, createdNotify)
				if err != nil {
					return false
				}
				return createdNotify.Status.MaintenanceStatus == ngn2v1alpha1.ObjectsDrained
			}, timeout, interval).Should(BeTrue())
		})

		It("should resume SLA countdown after restart", func() {
			nm1 := ngn2v1alpha1.NotifyMaintenance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foobar",
					Namespace: testNamespace,
				},
				Spec: ngn2v1alpha1.NotifyMaintenanceSpec{
					NodeObject: testNode.Name,
				},
				Status: ngn2v1alpha1.NotifyMaintenanceStatus{
					MaintenanceStatus: ngn2v1alpha1.MaintenanceStarted,
					SLAExpires: &metav1.Time{
						Time: time.Now().Add(time.Hour),
					},
				},
			}

			By("ensuring SLA countdown is resumed")
			Expect(nmRec.resumeSLACountdown(&nm1, testNode)).To(Succeed())
			Eventually(func() bool {
				return nmRec.slaTimer.HasCountDownBegun(&nm1)
			}, timeout, interval).Should(BeTrue())

			By("ensuring SLAExpired notification is sent")
			nm1.Status.SLAExpires = &metav1.Time{
				Time: time.Now().Add(time.Hour),
			}
			notifier.EXPECT().NotifySLAExpire(gomock.Any(), gomock.Any()).MaxTimes(1)
			Expect(nmRec.resumeSLACountdown(&nm1, testNode)).To(Succeed())
		})

		It("should not block a nm deletion before maintenance begins", func() {
			By("creating a nm")
			Expect(k8sClient.Create(ctx, nm)).Should(Succeed())
			createdNotify := waitForNMInit(nm)

			Expect(k8sClient.Delete(ctx, nm)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, namespaceName, createdNotify)
				return k8serrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			Eventually(func() bool {
				newNode := k8sv1.Node{}
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testNode), &newNode)
				if err != nil {
					return false
				}
				_, mntLblExists := newNode.Labels[utils.MAINTENANCE_LABEL_PREFIX+maintenanceID+createdNotify.Spec.MaintenanceID]
				return !mntLblExists
			}).Should(BeTrue())
		})

		It("should block a nm deletion after maintenance begins until node is uncordoned", func() {
			Expect(taintNode(testNode)).To(Succeed())

			By("creating a nm")
			Expect(k8sClient.Create(ctx, nm)).Should(Succeed())
			createdNotify := waitForNMInit(nm)

			By("deleting a nm before uncordoning")
			createdNotify.Status.MaintenanceStatus = ngn2v1alpha1.SLAExpired
			Expect(k8sClient.Status().Update(ctx, createdNotify)).To(Succeed())

			notifier.EXPECT().NotifyMaintenanceCancellation(gomock.Any(), gomock.Any()).AnyTimes()
			Expect(k8sClient.Delete(ctx, nm)).To(Succeed())

			// ensure that nm deletion is blocked
			Consistently(func() bool {
				err := k8sClient.Get(ctx, namespaceName, createdNotify)
				return err == nil
			}).Should(BeTrue())

			// ensure nm is marked as terminal
			Eventually(func() bool {
				err = k8sClient.Get(ctx, namespaceName, createdNotify)
				return err == nil && createdNotify.IsTerminal()
			}, timeout, interval).Should(BeTrue())

			By("uncordoning the node")
			notifier.EXPECT().NotifyMaintenanceCompleted(gomock.Any(), gomock.Any()).AnyTimes()
			Expect(uncordonNode(testNode)).To(Succeed())

			// ensure the NotifyMaintenance is deleted
			Eventually(func() bool {
				err := k8sClient.Get(ctx, namespaceName, createdNotify)
				return k8serrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("should skip SLA period when node is annotated with objects drained", func() {
			Expect(taintNode(testNode)).To(Succeed())

			By("creating a nm")
			Expect(k8sClient.Create(ctx, nm)).Should(Succeed())
			createdNotify := waitForNMInit(nm)

			By("skip SLA period")
			createdNotify.Status.MaintenanceStatus = ngn2v1alpha1.MaintenanceStarted
			Expect(k8sClient.Status().Update(ctx, createdNotify)).To(Succeed())

			notifier.EXPECT().NotifyNodeDrain(gomock.Any(), gomock.Any()).MaxTimes(2)

			MarkNMReadyForMaintenance(nm)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, namespaceName, createdNotify)
				if err != nil {
					return false
				}
				return createdNotify.Status.MaintenanceStatus == ngn2v1alpha1.ObjectsDrained
			}, timeout, interval).Should(BeTrue())
		})

		It("should transition properly to MaintenanceIncomplete after ObjectDrain and stay on MaintenanceIncomplete", func() {
			Expect(taintNode(testNode)).To(Succeed())

			By("creating a nm")
			Expect(k8sClient.Create(ctx, nm)).Should(Succeed())
			createdNotify := waitForNMInit(nm)

			By("skip SLA period")
			createdNotify.Status.MaintenanceStatus = ngn2v1alpha1.MaintenanceStarted
			Expect(k8sClient.Status().Update(ctx, createdNotify)).To(Succeed())

			notifier.EXPECT().NotifyNodeDrain(gomock.Any(), gomock.Any()).AnyTimes()

			MarkNMReadyForMaintenance(nm)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, namespaceName, createdNotify)
				if err != nil {
					return false
				}
				return createdNotify.Status.MaintenanceStatus == ngn2v1alpha1.ObjectsDrained
			}, timeout, interval).Should(BeTrue())

			By("adding a Client incomplete annotation on the NotifyMaintenance object")
			notifier.EXPECT().NotifyMaintenanceIncomplete(gomock.Any(), gomock.Any()).AnyTimes()

			AddAnnotation(createdNotify, ngn2v1alpha1.MaintenanceClientAnnotation.Key, ngn2v1alpha1.MaintenanceClientIncomplete)

			Eventually(func() bool {
				err := k8sClient.Get(ctx, namespaceName, createdNotify)
				if err != nil {
					return false
				}
				return createdNotify.Status.MaintenanceStatus == ngn2v1alpha1.MaintenanceIncomplete
			}, timeout, interval).Should(BeTrue())

			// touch to trigger reconciliation
			changed := nm.DeepCopy()
			changed.Labels = map[string]string{"foo": "bar"}
			err = k8sClient.Patch(context.Background(), changed, client.MergeFrom(nm))
			Expect(err).NotTo(HaveOccurred())

			notifier.EXPECT().NotifyNodeDrain(gomock.Any(), gomock.Any()).Times(0)

			SleepForReconcile()

			Consistently(func() bool {
				err := k8sClient.Get(ctx, namespaceName, createdNotify)
				if err != nil {
					return false
				}
				return createdNotify.Status.MaintenanceStatus == ngn2v1alpha1.MaintenanceIncomplete
			}, timeout, interval).Should(BeTrue())
		})

		It("should skip SLA period when node is annotated with objects drained before NM is in SLAStarted", func() {
			By("creating a nm")
			Expect(k8sClient.Create(ctx, nm)).Should(Succeed())
			createdNotify := waitForNMInit(nm)

			By("draining the node")
			notifier.EXPECT().NotifyNodeDrain(gomock.Any(), gomock.Any()).Times(1)

			MarkNMReadyForMaintenance(nm)
			createdNode := k8sv1.Node{}

			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testNode), testNode)
			Expect(err).ToNot(HaveOccurred())

			Expect(taintNode(testNode)).To(Succeed())
			Eventually(func() bool {
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(testNode), &createdNode)
				Expect(err).ToNot(HaveOccurred())
				if createdNode.Spec.Unschedulable {
					return true
				}
				for _, taint := range createdNode.Spec.Taints {
					if taint.Effect == k8sv1.TaintEffectNoSchedule {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			By("skip SLA period")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, namespaceName, createdNotify)
				if err != nil {
					return false
				}
				return createdNotify.Status.MaintenanceStatus == ngn2v1alpha1.ObjectsDrained
			}, timeout, interval).Should(BeTrue())
		})

		It("should notify for a complete maintenance cycle doing uncordon before nm delete", func() {
			nmRec.config.SLAPeriod = 3 * time.Second

			By("ensuring .status and label updates")
			Expect(k8sClient.Create(ctx, nm)).Should(Succeed())
			createdNotify := waitForNMInit(nm)

			By("tainting the node")
			Expect(taintNode(testNode)).To(Succeed())
			createdNode := k8sv1.Node{}
			Eventually(func() bool {
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(testNode), &createdNode)
				Expect(err).ToNot(HaveOccurred())
				if createdNode.Spec.Unschedulable {
					return true
				}
				for _, taint := range createdNode.Spec.Taints {
					if taint.Effect == k8sv1.TaintEffectNoSchedule {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			By("ensuring .status.maintenanceStatus changes to MaintenanceStarted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, namespaceName, createdNotify)
				if err != nil {
					return false
				}
				return createdNotify.Status.MaintenanceStatus == ngn2v1alpha1.MaintenanceStarted
			}, timeout, interval).Should(BeTrue())

			By("ensuring .status.maintenanceStatus changes to SLAExpired")
			notifier.EXPECT().NotifySLAExpire(gomock.Any(), gomock.Any()).AnyTimes()
			Eventually(func() bool {
				err := k8sClient.Get(ctx, namespaceName, createdNotify)
				if err != nil {
					return false
				}
				return createdNotify.Status.MaintenanceStatus == ngn2v1alpha1.SLAExpired
			}, timeout, interval).Should(BeTrue())

			By("creating a maintenance window")
			By("creating a second nm")
			Expect(k8sClient.Create(ctx, nm2)).Should(Succeed())
			createdNotify2 := waitForNMInit(nm2)

			By("labeling the node with the second nm")
			notifier.EXPECT().NotifySLAExpire(gomock.Any(), gomock.Any()).AnyTimes()
			Expect(labelNode(testNode, nm2)).To(Succeed())

			By("ensuring .status.maintenanceStatus changes to SLAExpired for the second nm")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, namespaceName2, createdNotify2)
				if err != nil {
					return false
				}
				return createdNotify2.Status.MaintenanceStatus == ngn2v1alpha1.SLAExpired
			}, timeout, interval).Should(BeTrue())

			By("ensuring .status.MaintenanceStatus changes to ObjectsDrained")
			notifier.EXPECT().NotifyNodeDrain(gomock.Any(), gomock.Any()).Times(2)

			MarkNMReadyForMaintenance(createdNotify)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, namespaceName, createdNotify)
				if err != nil {
					return false
				}
				return createdNotify.Status.MaintenanceStatus == ngn2v1alpha1.ObjectsDrained
			}, timeout, interval).Should(BeTrue())

			By("ensuring notification when the node is uncordoned then NotifyMaintenance is deleted")
			notifier.EXPECT().NotifyMaintenanceCompleted(gomock.Any(), gomock.Any()).Times(2)
			Expect(uncordonNode(testNode)).To(Succeed())

			SleepForReconcile()

			notifier.EXPECT().NotifyMaintenanceEnded(gomock.Any(), gomock.Any()).AnyTimes()
			Expect(k8sClient.Delete(ctx, createdNotify)).To(Succeed())
			Expect(k8sClient.Delete(ctx, createdNotify2)).To(Succeed())

			SleepForReconcile()

			// ensure the NotifyMaintenance is deleted
			Eventually(func() bool {
				err := k8sClient.Get(ctx, namespaceName, createdNotify)
				return k8serrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			// ensure labels are removed
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testNode), &createdNode)
				if err != nil {
					return false
				}
				_, maintenanceLabelExists := createdNode.Labels[utils.MAINTENANCE_LABEL_PREFIX+maintenanceID]
				_, newMaintenanceLabelExists := createdNode.Labels[utils.MAINTENANCE_LABEL_PREFIX+newMaintenanceID]
				return !maintenanceLabelExists && !newMaintenanceLabelExists
			}, timeout, interval).Should(BeTrue())

		})

		It("should notify for a complete maintenance cycle doing uncordon after restart and before nm delete", func() {
			nmRec.config.SLAPeriod = 3 * time.Second

			By("ensuring .status and label updates")
			Expect(k8sClient.Create(ctx, nm)).Should(Succeed())
			createdNotify := waitForNMInit(nm)

			By("tainting the node")
			Expect(taintNode(testNode)).To(Succeed())
			createdNode := k8sv1.Node{}
			Eventually(func() bool {
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(testNode), &createdNode)
				Expect(err).ToNot(HaveOccurred())
				if createdNode.Spec.Unschedulable {
					return true
				}
				for _, taint := range createdNode.Spec.Taints {
					if taint.Effect == k8sv1.TaintEffectNoSchedule {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			By("ensuring .status.maintenanceStatus changes to MaintenanceStarted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, namespaceName, createdNotify)
				if err != nil {
					return false
				}
				return createdNotify.Status.MaintenanceStatus == ngn2v1alpha1.MaintenanceStarted
			}, timeout, interval).Should(BeTrue())

			By("ensuring .status.maintenanceStatus changes to SLAExpired")
			notifier.EXPECT().NotifySLAExpire(gomock.Any(), gomock.Any()).AnyTimes()
			Eventually(func() bool {
				err := k8sClient.Get(ctx, namespaceName, createdNotify)
				if err != nil {
					return false
				}
				return createdNotify.Status.MaintenanceStatus == ngn2v1alpha1.SLAExpired
			}, timeout, interval).Should(BeTrue())

			By("ensuring .status.MaintenanceStatus changes to ObjectsDrained")
			notifier.EXPECT().NotifyNodeDrain(gomock.Any(), gomock.Any()).Times(2)

			MarkNMReadyForMaintenance(createdNotify)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, namespaceName, createdNotify)
				if err != nil {
					return false
				}
				return createdNotify.Status.MaintenanceStatus == ngn2v1alpha1.ObjectsDrained
			}, timeout, interval).Should(BeTrue())

			By("adding Maintenance Client In Progress annotation on the NotifyMaintenance object")
			addMaintenanceClientInProgressAnnotation(createdNotify)

			By("creating a test panic for reconciler restart")
			notifier.EXPECT().RotateCert().MinTimes(1)
			notifier.EXPECT().NotifyMaintenanceEnded(gomock.Any(), gomock.Any()).AnyTimes()
			counter := 0

			notifier.EXPECT().NotifyMaintenanceCompleted(gomock.Any(), gomock.Any()).DoAndReturn(func(_, _ interface{}) error {
				// the idea here is to insert a mock like cert rotation for significant amount of time
				// once the controller has recovered from the error, it should be able to reconcile state
				// properly
				if counter < 5 {
					counter += 1
					return fmt.Errorf("mock error")
				}
				return nil
			}).MinTimes(1)

			Expect(k8sClient.Delete(ctx, createdNotify)).To(Succeed())

			Expect(uncordonNode(testNode)).To(Succeed())

			SleepForReconcile()

			// ensure the NotifyMaintenance is deleted
			Eventually(func() bool {
				err := k8sClient.Get(ctx, namespaceName, createdNotify)
				return k8serrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			// ensure labels are removed
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testNode), &createdNode)
				if err != nil {
					return false
				}
				_, maintenanceLabelExists := createdNode.Labels[utils.MAINTENANCE_LABEL_PREFIX+maintenanceID]
				_, newMaintenanceLabelExists := createdNode.Labels[utils.MAINTENANCE_LABEL_PREFIX+newMaintenanceID]
				return !maintenanceLabelExists && !newMaintenanceLabelExists
			}, timeout, interval).Should(BeTrue())

		})

		It("should notify for a complete maintenance cycle", func() {
			completeCycleWithTwoNms()
		})

		It("should transition correctly to MaintenanceIncomplete status after adding annotation maintenance.ngn2.nvidia.com/maintenanceClient=incomplete", func() {
			fullMaintenanceCycle(ngn2v1alpha1.MaintenanceClientAnnotation.Key, ngn2v1alpha1.MaintenanceClientIncomplete)
		})

		It("should transition correctly to MaintenanceIncomplete status after adding annotation maintenance.ngn2.nvidia.com/validationClient=failed", func() {
			fullMaintenanceCycle(ngn2v1alpha1.ValidationClientAnnotation.Key, ngn2v1alpha1.ValidationClientFailed)
		})
	})

	Context("Proper reconcile", Label("NID-5894"), func() {
		reconcileTest := func(nodeUpdate func(*k8sv1.Node), expectedStatus ngn2v1alpha1.MaintenanceStatus) {
			node := newFakeNode()
			nodeUpdate(node)
			nm := newNotifyMaintenance(node.Name)

			err := k8sClient.Create(context.Background(), node)
			Expect(err).NotTo(HaveOccurred())
			err = k8sClient.Create(context.Background(), nm)
			Expect(err).NotTo(HaveOccurred())

			SleepForReconcile()

			// Touch NotifyMaintenance object to trigger reconcile.
			changed := nm.DeepCopy()
			changed.Labels = map[string]string{"foo": "bar"}
			err = k8sClient.Patch(context.Background(), changed, client.MergeFrom(nm))
			Expect(err).NotTo(HaveOccurred())

			SleepForReconcile()

			err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(nm), nm)
			Expect(err).NotTo(HaveOccurred())
			Expect(nm.Status.MaintenanceStatus).To(Equal(expectedStatus))
			Expect(nm.Status.Conditions).To(BeEmpty())
		}

		It("should stay at MaintenanceScheduled when node is not cordoned and does not have maintenance label", func() {
			reconcileTest(
				func(node *k8sv1.Node) {
					node.Labels = nil
				},
				ngn2v1alpha1.MaintenanceScheduled,
			)
		})

		It("should stay at MaintenanceScheduled when node is cordoned but does not have maintenance label", func() {
			reconcileTest(
				func(node *k8sv1.Node) {
					node.Labels = nil
					node.Spec.Unschedulable = true
				},
				ngn2v1alpha1.MaintenanceScheduled,
			)
		})

		It("should transition to MaintenanceStarted when node has been cordoned and has maintenance label", func() {
			reconcileTest(
				func(node *k8sv1.Node) {
					node.Labels = map[string]string{utils.MAINTENANCE_LABEL_PREFIX + maintenanceID: ""}
					node.Spec.Unschedulable = true
				},
				ngn2v1alpha1.MaintenanceStarted,
			)
		})
	})
})

func newNotifyMaintenance(object string) *ngn2v1alpha1.NotifyMaintenance {
	return &ngn2v1alpha1.NotifyMaintenance{
		TypeMeta: metav1.TypeMeta{
			Kind:       ngn2v1alpha1.NotifyMaintenanceKind,
			APIVersion: ngn2v1alpha1.Version,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      uuid.New().String(),
			Namespace: testNamespace,
		},
		Spec: ngn2v1alpha1.NotifyMaintenanceSpec{
			NodeObject:                object,
			MaintenanceID:             maintenanceID,
			AdditionalMessageChannels: ngn2v1alpha1.MessageChannels{},
			Type:                      ngn2v1alpha1.PlannedMaintenance,
		},
	}
}

func newFakeNode() *k8sv1.Node {
	return &k8sv1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: uuid.New().String(),
			Labels: map[string]string{
				utils.MAINTENANCE_LABEL_PREFIX + maintenanceID: "",
			},
			Annotations: map[string]string{},
		},
	}
}

func labelNode(node *k8sv1.Node, nm *ngn2v1alpha1.NotifyMaintenance) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		newNode := node.DeepCopy()
		newNode.Labels[utils.MAINTENANCE_LABEL_PREFIX+nm.Spec.MaintenanceID] = ""

		return k8sClient.Patch(ctx, newNode, client.MergeFrom(node))
	})
}

func taintNode(node *k8sv1.Node) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		newNode := node.DeepCopy()
		newNode.Spec.Taints = append(newNode.Spec.Taints, k8sv1.Taint{
			Key:    "foobar",
			Value:  "bar",
			Effect: k8sv1.TaintEffectNoSchedule,
		})
		newNode.Spec.Unschedulable = true

		return k8sClient.Patch(ctx, newNode, client.MergeFrom(node))
	})
}

func removeNoScheduleTaint(node *k8sv1.Node) {
	var newTaint []k8sv1.Taint
	for _, taint := range node.Spec.Taints {
		if taint.Effect != k8sv1.TaintEffectNoSchedule {
			newTaint = append(newTaint, taint)
		}
	}
	node.Spec.Taints = newTaint
}

func uncordonNode(node *k8sv1.Node) error {
	err := k8sClient.Get(ctx, client.ObjectKeyFromObject(node), node)
	if err != nil {
		return err
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		newNode := node.DeepCopy()
		removeNoScheduleTaint(newNode)
		newNode.Spec.Unschedulable = false

		return k8sClient.Patch(ctx, newNode, client.MergeFrom(node))
	})
}

func SleepForReconcile() {
	time.Sleep(2 * time.Second)
}

func AddAnnotation(nm *ngn2v1alpha1.NotifyMaintenance, key, val string) {
	// add an annotation to NotifyMaintenance.
	changed := nm.DeepCopy()
	if changed.Annotations == nil {
		changed.Annotations = map[string]string{}
	}
	changed.Annotations[key] = val
	err = k8sClient.Patch(context.Background(), changed, client.MergeFrom(nm))
	Expect(err).NotTo(HaveOccurred())
}

func MarkNMReadyForMaintenance(nm *ngn2v1alpha1.NotifyMaintenance) {
	AddAnnotation(nm, ngn2v1alpha1.ReadyForMaintenanceAnnotation.Key, "true")
	SleepForReconcile()
}

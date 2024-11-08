package controllers

import (
	"context"

	ngn2v1alpha1 "github.com/NVIDIA/pika/api/v1alpha1"
	"github.com/NVIDIA/pika/pkg/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8sv1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("HandlerMapper", func() {

	var (
		nodeMapper *NodeMapper
	)
	BeforeEach(func() {
		nodeMapper = &NodeMapper{
			Mapper: Mapper{
				nmRec: nmRec,
			},
		}
	})

	It("should gather NotifyMaintenance objects on node creation", func() {
		node := newFakeNode()
		node.Labels = map[string]string{}
		Expect(k8sClient.Create(context.Background(), node)).Should(Succeed())
		Eventually(func() bool {
			tempNode := &k8sv1.Node{}
			return k8sClient.Get(context.Background(), client.ObjectKeyFromObject(node), tempNode) == nil
		}, timeout, interval).Should(BeTrue())

		mapperNM := newNotifyMaintenance(node.Name)
		Expect(k8sClient.Create(context.Background(), mapperNM)).Should(Succeed())

		SleepForReconcile()
		// wait for the object to be in scheduled
		Eventually(func() bool {
			tempNM := &ngn2v1alpha1.NotifyMaintenance{}
			err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(mapperNM), tempNM)
			if err != nil {
				return false
			}
			return tempNM.Status.MaintenanceStatus == ngn2v1alpha1.MaintenanceScheduled
		}, timeout, interval).Should(BeTrue())

		Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(mapperNM), mapperNM)).To(Succeed())
		mapperNM.Status = ngn2v1alpha1.NotifyMaintenanceStatus{
			MaintenanceStatus: ngn2v1alpha1.ObjectsDrained,
		}
		Expect(k8sClient.Status().Update(context.Background(), mapperNM)).To(Succeed())
		// wait for the update to be in cache before running nodeMapper
		Eventually(func() bool {
			tempNM := &ngn2v1alpha1.NotifyMaintenance{}
			err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(mapperNM), tempNM)
			if err != nil {
				return false
			}
			return tempNM.Status.MaintenanceStatus == ngn2v1alpha1.ObjectsDrained
		}, timeout, interval).Should(BeTrue())

		reqs := nodeMapper.MapNodeCreate(node)
		Expect(reqs).To(HaveLen(1))
		Expect(reqs[0].NamespacedName).To(Equal(client.ObjectKeyFromObject(mapperNM)))
	})

	It("should gather NotifyMaintenance objects on node update", func() {
		node := newFakeNode()
		Expect(k8sClient.Create(context.Background(), node)).Should(Succeed())
		Eventually(func() bool {
			tempNode := &k8sv1.Node{}
			return k8sClient.Get(context.Background(), client.ObjectKeyFromObject(node), tempNode) == nil
		}, timeout, interval).Should(BeTrue())

		mapperNM := newNotifyMaintenance(node.Name)
		Expect(k8sClient.Create(context.Background(), mapperNM)).Should(Succeed())
		// wait for the nmobject to be in cache for the list call to succeed in nodeMapper
		Eventually(func() bool {
			tempNM := &ngn2v1alpha1.NotifyMaintenance{}
			return k8sClient.Get(context.Background(), client.ObjectKeyFromObject(mapperNM), tempNM) == nil
		}, timeout, interval).Should(BeTrue())

		node.Labels[utils.MAINTENANCE_LABEL_PREFIX+mapperNM.Spec.MaintenanceID] = ""
		Expect(k8sClient.Update(context.Background(), node)).To(Succeed())

		reqs := nodeMapper.MapNodeUpdate(node)
		Expect(reqs).To(HaveLen(1))
		Expect(reqs[0].NamespacedName).To(Equal(client.ObjectKeyFromObject(mapperNM)))
	})
})

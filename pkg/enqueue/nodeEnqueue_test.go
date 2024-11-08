package enqueue

import (
	"github.com/NVIDIA/pika/pkg/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8sv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("NodeEnqueue", func() {
	It("should get new maintenances", func() {
		n1 := k8sv1.Node{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{
					utils.MAINTENANCE_LABEL_PREFIX + "foo": "",
				},
			},
		}
		n2 := n1.DeepCopy()
		n2.Labels[utils.MAINTENANCE_LABEL_PREFIX+"bar"] = ""

		res := getNewMaintenanceIDs(&n1, n2)
		Expect(res).To(Equal([]string{"bar"}))
	})

	Context("Update", func() {
		var handler *nodeEnqueueRequestsFromMapFunc

		var enqueuedNode client.Object

		BeforeEach(func() {
			enqueuedNode = nil
			handler = &nodeEnqueueRequestsFromMapFunc{
				EnqueueRequestsFromMapFunc: EnqueueRequestsFromMapFunc{},
				updateMapper: func(o client.Object) []reconcile.Request {
					enqueuedNode = o
					return GenericMapperFunc(o)
				},
			}
		})

		It("should enqueue on node cordon", func() {
			evt := event.UpdateEvent{
				ObjectOld: &k8sv1.Node{
					ObjectMeta: v1.ObjectMeta{
						Name:            "foo",
						ResourceVersion: "foo",
					},
					Spec: k8sv1.NodeSpec{
						Unschedulable: false,
					},
				},
				ObjectNew: &k8sv1.Node{
					ObjectMeta: v1.ObjectMeta{
						Name:            "foo",
						ResourceVersion: "bar",
					},
					Spec: k8sv1.NodeSpec{
						Unschedulable: true,
					},
				},
			}
			handler.Update(evt, workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()))
			Expect(enqueuedNode.GetName()).To(Equal("foo"))
		})
	})
})

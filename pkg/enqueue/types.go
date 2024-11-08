package enqueue

import (
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var log = logf.Log.WithName("enqueue")

type empty struct{}

type EnqueueRequestsFromMapFunc struct {
	c client.Client
}

// Create no-op EventHandlers.
func (e *EnqueueRequestsFromMapFunc) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
}

func (e *EnqueueRequestsFromMapFunc) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
}

func (e *EnqueueRequestsFromMapFunc) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

func (e *EnqueueRequestsFromMapFunc) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
}

func GenericMapperFunc(o client.Object) []reconcile.Request {
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      o.GetName(),
			Namespace: o.GetNamespace(),
		},
	}
	return []reconcile.Request{req}
}

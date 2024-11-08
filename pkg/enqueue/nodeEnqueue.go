package enqueue

import (
	"context"
	"strings"

	ngn2v1alpha1 "github.com/NVIDIA/pika/api/v1alpha1"
	"github.com/NVIDIA/pika/pkg/utils"

	k8sv1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type WithReconciler struct {
	C client.Client
}

type nodeEnqueueRequestsFromMapFunc struct {
	EnqueueRequestsFromMapFunc

	createMapper handler.MapFunc
	updateMapper handler.MapFunc
}

func (w *WithReconciler) NodeEventHandler(nodeUpdateMapper, nodeCreateMapper handler.MapFunc) handler.EventHandler {
	return &nodeEnqueueRequestsFromMapFunc{
		EnqueueRequestsFromMapFunc: EnqueueRequestsFromMapFunc{
			c: w.C,
		},
		createMapper: nodeCreateMapper,
		updateMapper: nodeUpdateMapper,
	}
}

// Create no-op EventHandlers.
func (e *nodeEnqueueRequestsFromMapFunc) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	if evt.Object == nil {
		log.Error(nil, "Create event has no object")
		return
	}

	node, ok := evt.Object.(*k8sv1.Node)
	if !ok {
		return
	}

	log.Info("Node was created", "nodeName", node.ObjectMeta.Name)
	reqs := map[reconcile.Request]empty{}
	e.mapAndEnqueue(e.createMapper, q, evt.Object, reqs)
}

func (e *nodeEnqueueRequestsFromMapFunc) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	reqs := map[reconcile.Request]empty{}
	if evt.ObjectOld == nil {
		log.Error(nil, "Update event has no old object to update")
		return
	}
	if evt.ObjectNew == nil {
		log.Error(nil, "Update event has no new object to update")
		return
	}

	newNode, ok := evt.ObjectNew.(*k8sv1.Node)
	if !ok {
		return
	}
	oldNode, ok := evt.ObjectOld.(*k8sv1.Node)
	if !ok {
		return
	}

	if newNode.GetResourceVersion() == oldNode.GetResourceVersion() {
		return
	}

	if newNode.Spec.Unschedulable != oldNode.Spec.Unschedulable {
		// transition from/to unschedulable state
		if newNode.Spec.Unschedulable {
			log.Info("Node was cordoned", "nodeName", newNode.ObjectMeta.Name)
		} else if oldNode.Spec.Unschedulable {
			log.Info("Node was uncordoned", "nodeName", newNode.ObjectMeta.Name)
		}
		e.mapAndEnqueue(e.updateMapper, q, evt.ObjectNew, reqs)
	} else if newMaintenanceIDs := getNewMaintenanceIDs(oldNode, newNode); len(newMaintenanceIDs) > 0 && newNode.Spec.Unschedulable {
		// parallel maintenances
		// When a new maintenance label is added to a Node, only reconcile
		// the new maintenance object to align it with the other NotifyMaintenance object's state.
		log.Info("Node has new Maintenance IDs", "IDs", newMaintenanceIDs, "nodeName", newNode.ObjectMeta.Name)

		// Find if a Node with active maintenance(s) was labeled with a new NotifyMaintenance object
		nmList := ngn2v1alpha1.NotifyMaintenanceList{}
		err := e.c.List(context.Background(), &nmList, client.MatchingFields{"spec.nodeObject": newNode.Name})
		if err != nil {
			log.Info("Failed to List NotifyMaintenances", "IDs", newMaintenanceIDs, "nodeName", newNode.ObjectMeta.Name, "error", err)
			return
		}
		for _, nm := range nmList.Items {
			for _, newMaintenanceID := range newMaintenanceIDs {
				if nm.Spec.MaintenanceID == newMaintenanceID {
					e.enqueue(q, &nm, reqs)
				}
			}
		}
	}
}

func (e *nodeEnqueueRequestsFromMapFunc) mapAndEnqueue(mapper handler.MapFunc, q workqueue.RateLimitingInterface, object client.Object, reqs map[reconcile.Request]empty) {
	log.Info("starting to mapAndEnqueue")
	for _, req := range mapper(object) {
		_, ok := reqs[req]
		if !ok {
			q.Add(req)
			reqs[req] = empty{}
		}
	}
}

func (e *nodeEnqueueRequestsFromMapFunc) enqueue(q workqueue.RateLimitingInterface, object client.Object, reqs map[reconcile.Request]empty) {
	req := reconcile.Request{
		NamespacedName: client.ObjectKeyFromObject(object),
	}
	_, ok := reqs[req]
	if !ok {
		q.Add(req)
		reqs[req] = empty{}
	}
}

func getNewMaintenanceIDs(nodeOld *k8sv1.Node, nodeNew *k8sv1.Node) []string {
	res := []string{}

	for key := range nodeNew.Labels {
		if strings.HasPrefix(key, utils.MAINTENANCE_LABEL_PREFIX) {
			if _, inOld := nodeOld.Labels[key]; !inOld {
				maintID := strings.TrimPrefix(key, utils.MAINTENANCE_LABEL_PREFIX)
				res = append(res, maintID)
			}
		}
	}
	return res
}

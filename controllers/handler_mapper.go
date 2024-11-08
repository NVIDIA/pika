package controllers

import (
	"context"
	"strings"

	ngn2v1alpha1 "github.com/NVIDIA/pika/api/v1alpha1"
	"github.com/NVIDIA/pika/pkg/utils"

	k8sv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type MapperInterface interface {
	Map(client.Object) []reconcile.Request
}

type Mapper struct {
	nmRec *NotifyMaintenanceReconciler
}

type NodeMapper struct {
	Mapper
}

func (m NodeMapper) MapNodeUpdate(obj client.Object) []reconcile.Request {
	var requests []reconcile.Request
	if changedNode, ok := obj.(*k8sv1.Node); ok {
		log.WithValues("NodeName", changedNode.Name, "Verb", "Update").Info("Mapping Node to NM")

		// Find NotifyMaintenance object(s) that are watching events on changedNode
		notifyMaintenances, err := m.getActiveMaintenances(changedNode)
		if err != nil {
			log.Error(err, "failed to get active maintenances")
			return requests
		}

		for _, notify := range notifyMaintenances {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      notify.Name,
					Namespace: notify.Namespace,
				},
			})
		}
	}
	return requests
}

func (m NodeMapper) MapNodeCreate(obj client.Object) []reconcile.Request {
	var requests []reconcile.Request
	if node, ok := obj.(*k8sv1.Node); ok {
		log.WithValues("NodeName", node.Name, "Verb", "Create").Info("Mapping Node to NM")
		// Find NotifyMaintenance object(s) that are watching events on changedNode
		// ensure that the node should have no maintenance labels
		for k := range node.Labels {
			if strings.HasPrefix(k, utils.MAINTENANCE_LABEL_PREFIX) {
				return requests
			}
		}

		notifyMaintenances, err := m.getMaintenanceOnNodeCreate(node)
		if err != nil {
			log.Error(err, "failed to get active maintenances")
			return requests
		}

		for _, notify := range notifyMaintenances {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      notify.Name,
					Namespace: notify.Namespace,
				},
			})
		}
	}
	return requests
}

func (m NodeMapper) getMaintenanceOnNodeCreate(node *k8sv1.Node) ([]ngn2v1alpha1.NotifyMaintenance, error) {
	maintenances := []ngn2v1alpha1.NotifyMaintenance{}
	nmList := ngn2v1alpha1.NotifyMaintenanceList{}
	err := m.nmRec.List(context.Background(), &nmList)
	if err != nil {
		return maintenances, err
	}

	for _, nm := range nmList.Items {
		if nm.Spec.NodeObject == node.Name && nm.Status.MaintenanceStatus == ngn2v1alpha1.ObjectsDrained {
			maintenances = append(maintenances, nm)
		}
	}
	return maintenances, nil
}

func (m NodeMapper) getActiveMaintenances(node *k8sv1.Node) ([]ngn2v1alpha1.NotifyMaintenance, error) {
	activeMaintenanceIDs := map[string]struct{}{}
	activeNMs := []ngn2v1alpha1.NotifyMaintenance{}

	for labelKey := range node.Labels {
		if strings.HasPrefix(labelKey, utils.MAINTENANCE_LABEL_PREFIX) {
			maintenanceID := strings.TrimPrefix(labelKey, utils.MAINTENANCE_LABEL_PREFIX)
			activeMaintenanceIDs[maintenanceID] = struct{}{}
		}
	}

	nmList := ngn2v1alpha1.NotifyMaintenanceList{}
	err := m.nmRec.List(context.Background(), &nmList)
	if err != nil {
		return activeNMs, err
	}

	for _, nm := range nmList.Items {
		_, isActive := activeMaintenanceIDs[nm.Spec.MaintenanceID]
		if isActive && nm.Spec.NodeObject == node.Name {
			if !node.Spec.Unschedulable {
				// node is schedulable and uncordoned, requeue every active nm
				activeNMs = append(activeNMs, nm)
			} else if !utils.IsFinal(&nm) {
				// node is unschedulable, so don't requeue objects in the terminal state.
				activeNMs = append(activeNMs, nm)
			}
		}
	}
	return activeNMs, nil
}

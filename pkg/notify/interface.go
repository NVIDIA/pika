package notify

import (
	ngn2v1alpha1 "github.com/NVIDIA/pika/api/v1alpha1"
	"github.com/NVIDIA/pika/pkg/config"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type NewNotifierFunc func(config.Config, logr.Logger) (Notifier, error)

type Notifier interface {
	NotifySLAStart(client.Object, ngn2v1alpha1.NotifyMaintenance) error
	NotifySLAExpire(client.Object, ngn2v1alpha1.NotifyMaintenance) error
	NotifyMaintenanceCancellation(client.Object, ngn2v1alpha1.NotifyMaintenance) error
	NotifyMaintenanceEnded(client.Object, ngn2v1alpha1.NotifyMaintenance) error
	NotifyMaintenanceCompleted(client.Object, ngn2v1alpha1.NotifyMaintenance) error
	NotifyNodeDrain(client.Object, ngn2v1alpha1.NotifyMaintenance) error
	NotifyValidating(client.Object, ngn2v1alpha1.NotifyMaintenance) error
	NotifyMaintenanceIncomplete(client.Object, ngn2v1alpha1.NotifyMaintenance) error
	GetNotifierName() string
	RotateCert() error
}

type NoopNotifier struct{}

func (n NoopNotifier) NotifySLAStart(obj client.Object, nm ngn2v1alpha1.NotifyMaintenance) error {
	return nil
}

func (n NoopNotifier) NotifySLAExpire(obj client.Object, nm ngn2v1alpha1.NotifyMaintenance) error {
	return nil
}

func (n NoopNotifier) NotifyMaintenanceCompleted(obj client.Object, nm ngn2v1alpha1.NotifyMaintenance) error {
	return nil
}

func (n NoopNotifier) NotifyNodeDrain(obj client.Object, nm ngn2v1alpha1.NotifyMaintenance) error {
	return nil
}

func (n NoopNotifier) NotifyValidating(obj client.Object, nm ngn2v1alpha1.NotifyMaintenance) error {
	return nil
}

func (n NoopNotifier) NotifyMaintenanceIncomplete(obj client.Object, nm ngn2v1alpha1.NotifyMaintenance) error {
	return nil
}

func (n NoopNotifier) NotifyMaintenanceCancellation(obj client.Object, nm ngn2v1alpha1.NotifyMaintenance) error {
	return nil
}

func (n NoopNotifier) NotifyMaintenanceEnded(obj client.Object, nm ngn2v1alpha1.NotifyMaintenance) error {
	return nil
}

func (n NoopNotifier) GetNotifierName() string {
	return ""
}

func (n NoopNotifier) RotateCert() error {
	return nil
}

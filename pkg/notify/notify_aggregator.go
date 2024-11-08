package notify

import (
	ngn2v1alpha1 "github.com/NVIDIA/pika/api/v1alpha1"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	aggregatorName = "NotificationAggregator"
)

type notifiers_t []Notifier

type NotifyAggregator struct {
	notifiers notifiers_t
}

func NewNotifyAggregator(notifiers ...Notifier) *NotifyAggregator {
	return &NotifyAggregator{
		notifiers: notifiers,
	}
}

func (na *NotifyAggregator) NotifySLAStart(obj client.Object, nm ngn2v1alpha1.NotifyMaintenance) error {
	errors := []error{}
	for _, notifier := range na.notifiers {
		err := notifier.NotifySLAStart(obj, nm)
		if err != nil {
			errors = append(errors, err)

			// In the future, we can have this only run on 403s
			if err = na.RotateCert(); err != nil {
				errors = append(errors, err)
			}
		}
	}
	if len(errors) > 0 {
		return utilerrors.NewAggregate(errors)
	}
	return nil
}

func (na *NotifyAggregator) NotifySLAExpire(obj client.Object, nm ngn2v1alpha1.NotifyMaintenance) error {
	errors := []error{}
	for _, notifier := range na.notifiers {
		err := notifier.NotifySLAExpire(obj, nm)
		if err != nil {
			errors = append(errors, err)

			// In the future, we can have this only run on 403s
			if err = na.RotateCert(); err != nil {
				errors = append(errors, err)
			}
		}
	}
	if len(errors) > 0 {
		return utilerrors.NewAggregate(errors)
	}
	return nil
}

func (na *NotifyAggregator) NotifyMaintenanceEnded(obj client.Object, nm ngn2v1alpha1.NotifyMaintenance) error {
	errors := []error{}
	for _, notifier := range na.notifiers {
		err := notifier.NotifyMaintenanceEnded(obj, nm)
		if err != nil {
			errors = append(errors, err)

			// In the future, we can have this only run on 403s
			if err = na.RotateCert(); err != nil {
				errors = append(errors, err)
			}
		}
	}
	if len(errors) > 0 {
		return utilerrors.NewAggregate(errors)
	}
	return nil
}

func (na *NotifyAggregator) NotifyMaintenanceCompleted(obj client.Object, nm ngn2v1alpha1.NotifyMaintenance) error {
	errors := []error{}
	for _, notifier := range na.notifiers {
		err := notifier.NotifyMaintenanceCompleted(obj, nm)
		if err != nil {
			errors = append(errors, err)

			// In the future, we can have this only run on 403s
			if err = na.RotateCert(); err != nil {
				errors = append(errors, err)
			}
		}
	}
	if len(errors) > 0 {
		return utilerrors.NewAggregate(errors)
	}
	return nil
}

func (na *NotifyAggregator) NotifyValidating(obj client.Object, nm ngn2v1alpha1.NotifyMaintenance) error {
	errors := []error{}
	for _, notifier := range na.notifiers {
		err := notifier.NotifyValidating(obj, nm)
		if err != nil {
			errors = append(errors, err)
		}
	}
	if len(errors) > 0 {
		return utilerrors.NewAggregate(errors)
	}
	return nil
}

func (na *NotifyAggregator) NotifyMaintenanceIncomplete(obj client.Object, nm ngn2v1alpha1.NotifyMaintenance) error {
	errors := []error{}
	for _, notifier := range na.notifiers {
		err := notifier.NotifyMaintenanceIncomplete(obj, nm)
		if err != nil {
			errors = append(errors, err)
		}
	}
	if len(errors) > 0 {
		return utilerrors.NewAggregate(errors)
	}
	return nil
}

func (na *NotifyAggregator) NotifyNodeDrain(obj client.Object, nm ngn2v1alpha1.NotifyMaintenance) error {
	errors := []error{}
	for _, notifier := range na.notifiers {
		err := notifier.NotifyNodeDrain(obj, nm)
		if err != nil {
			errors = append(errors, err)

			// In the future, we can have this only run on 403s
			if err = na.RotateCert(); err != nil {
				errors = append(errors, err)
			}
		}
	}
	if len(errors) > 0 {
		return utilerrors.NewAggregate(errors)
	}
	return nil
}

func (na *NotifyAggregator) NotifyMaintenanceCancellation(obj client.Object, nm ngn2v1alpha1.NotifyMaintenance) error {
	errors := []error{}
	for _, notifier := range na.notifiers {
		err := notifier.NotifyMaintenanceCancellation(obj, nm)
		if err != nil {
			errors = append(errors, err)

			// In the future, we can have this only run on 403s
			if err = na.RotateCert(); err != nil {
				errors = append(errors, err)
			}
		}
	}
	if len(errors) > 0 {
		return utilerrors.NewAggregate(errors)
	}
	return nil
}

func (na *NotifyAggregator) GetNotifierName() string {
	return aggregatorName
}

func (na *NotifyAggregator) RotateCert() error {
	errors := []error{}
	for _, notifier := range na.notifiers {
		klog.Infof("rotating cert for %+v", na.GetNotifierName)
		err := notifier.RotateCert()
		if err != nil {
			errors = append(errors, err)
		}
	}
	if len(errors) > 0 {
		return utilerrors.NewAggregate(errors)
	}
	return nil
}

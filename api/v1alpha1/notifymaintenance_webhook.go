/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"context"
	"fmt"

	k8sv1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var notifymaintenancelog = logf.Log.WithName("notifymaintenance-resource")
var k8sclient client.Client

func (r *NotifyMaintenance) SetupWebhookWithManager(mgr ctrl.Manager) error {
	k8sclient = mgr.GetClient()

	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

var _ webhook.Defaulter = &NotifyMaintenance{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *NotifyMaintenance) Default() {
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-ngn2-nvidia-com-v1alpha1-notifymaintenance,mutating=false,failurePolicy=fail,sideEffects=None,groups=ngn2.nvidia.com,resources=notifymaintenances,verbs=create;update,versions=v1alpha1,name=vnotifymaintenance.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &NotifyMaintenance{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *NotifyMaintenance) ValidateCreate() (err error) {
	notifymaintenancelog.Info("validate create", "NotifyMaintenance", r.String())

	if err = r.validateSpec(); err != nil {
		return err
	}
	if err = r.validateAnnotationsInput(); err != nil {
		return err
	}

	if err = r.ValidateNonDuplication(); err != nil {
		return err
	}

	if err = r.validateConfigmap(); err != nil {
		return err
	}

	return r.validateNode()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *NotifyMaintenance) ValidateUpdate(old runtime.Object) (err error) {
	notifymaintenancelog.Info("validate update", "NotifyMaintenance", r.String())

	if err = r.validateSpec(); err != nil {
		return err
	}

	oldNotifyMaintenance, ok := old.(*NotifyMaintenance)
	if !ok {
		return fmt.Errorf("failed to read NotifyMaintenance object")
	}
	if err = r.validateAnnotationsInput(); err != nil {
		return err
	}
	if err = r.validateAnnotationsValueTransition(oldNotifyMaintenance); err != nil {
		return err
	}

	return r.validateNoObjectChange(oldNotifyMaintenance)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *NotifyMaintenance) ValidateDelete() error {
	notifymaintenancelog.Info("validate delete", "NotifyMaintenance", r.String())

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}

// validate .spec
func (r *NotifyMaintenance) validateSpec() error {
	notifymaintenancelog.Info("validate spec", "NotifyMaintenance", r.String())
	// ensure there are at least one entry in .spec.objects
	if r.Spec.MaintenanceID == "" {
		return fmt.Errorf("please specify a MaintenanceID in .spec.MaintenanceID")
	}
	if r.Spec.NodeObject == "" {
		return fmt.Errorf("please specify an object in .spec.objects")
	}
	if r.Spec.Type != PlannedMaintenance && r.Spec.Type != UnplannedMaintenance {
		return fmt.Errorf("MaintenanceType can only be %s or %s in .spec.MaintenanceType", PlannedMaintenance, UnplannedMaintenance)
	}
	return nil
}

func (r *NotifyMaintenance) ValidateNonDuplication() (err error) {
	notifymaintenancelog.Info("ValidateNonDuplication", "NotifyMaintenance", r.String())

	nmList := NotifyMaintenanceList{}

	// find ALL NM objects with the same maintenanceID. For each check that the node is different from the given one.
	if r.Spec.MaintenanceID != "" {
		err = k8sclient.List(context.Background(), &nmList, client.MatchingFields{"spec.maintenanceID": r.Spec.MaintenanceID})
		if err != nil && !k8serrors.IsNotFound(err) {
			notifymaintenancelog.Error(err, "Failed to List NotifyMaintenances", "maintenanceID", r.Spec.MaintenanceID, "error", err)
			return err
		}

		for _, nm := range nmList.Items {
			if nm.Spec.NodeObject == r.Spec.NodeObject {
				err = fmt.Errorf("nofityMainenance objects with the same maintenanceID (%s) cannot have the same nodeObject (%s)", r.Spec.MaintenanceID, r.Spec.NodeObject)
				notifymaintenancelog.Error(err, "ValidateNonDuplication")
				return err
			}
		}
	}
	return nil
}

// validate whether the specified configmap exists in the NM's namespace
func (r *NotifyMaintenance) validateConfigmap() error {
	if r.Spec.MetadataConfigmap == "" {
		return nil
	}

	cm := k8sv1.ConfigMap{}
	err := k8sclient.Get(context.Background(), types.NamespacedName{
		Name:      r.Spec.MetadataConfigmap,
		Namespace: r.Namespace,
	}, &cm)

	if err != nil {
		return err
	}
	return nil
}

// validate whether the specified node exists
func (r *NotifyMaintenance) validateNode() error {
	node := k8sv1.Node{}
	err := k8sclient.Get(context.Background(), types.NamespacedName{
		Name: r.Spec.NodeObject,
	}, &node)

	if err != nil {
		return err
	}
	return nil
}

// validate there are no change of object when updating a NotifyMaintenance object
// pre-reqs:
//   - validateSpec
//   - validateNodes
func (r *NotifyMaintenance) validateNoObjectChange(old *NotifyMaintenance) (err error) {
	if r.Spec.NodeObject != old.Spec.NodeObject {
		return fmt.Errorf("change of .spec.object in NotifyMaintenance is not allowed")
	}

	if r.Spec.MaintenanceID != old.Spec.MaintenanceID {
		return fmt.Errorf("change of .spec.maintenanceID in NotifyMaintenance is not allowed")
	}

	return nil
}

func (r *NotifyMaintenance) validateAnnotationsInput() error {
	for _, a := range ValidatedAnnotations {
		notifymaintenancelog.Info("validate Annotation", "NotifyMaintenance", r.String(), "Annotation key", a.Key)
		val, exists := r.Annotations[a.Key]
		if !exists {
			continue
		}
		found := false

		// Check the value is valid.
		for _, v := range a.ValidValues {
			if v == val {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("annotation value %s is not allowed for key %s", val, a.Key)
		}
	}

	return nil
}

func (r *NotifyMaintenance) validateAnnotationsValueTransition(old *NotifyMaintenance) error {
	for _, a := range ValidatedAnnotations {
		val, exists := r.Annotations[a.Key]
		if !exists {
			continue
		}
		oldVal, oexist := old.Annotations[a.Key]
		if !oexist {
			continue
		}
		if val != oldVal {
			// If old value is in the final state, it shouldn't be modified backwards or to any other value.
			found := false
			for _, v := range a.FinalStateValues {
				if v == oldVal {
					found = true
				}
			}
			if found {
				return fmt.Errorf("annotation value for %s cannot be modified from a final state value. Final State value: %s. Attempted to set value: %s", a.Key, oldVal, val)
			}
		}
	}
	return nil
}

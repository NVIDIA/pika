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

package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	ngn2v1alpha1 "github.com/NVIDIA/pika/api/v1alpha1"
	"github.com/NVIDIA/pika/pkg/config"
	"github.com/NVIDIA/pika/pkg/enqueue"
	"github.com/NVIDIA/pika/pkg/metrics"
	"github.com/NVIDIA/pika/pkg/notify"
	"github.com/NVIDIA/pika/pkg/utils"

	"github.com/go-logr/logr"
	k8sv1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("notifymaintenance_controller")
var nmRetentionDuration = 7 * 24
var afterStatus ngn2v1alpha1.MaintenanceStatus

// NotifyMaintenanceReconciler reconciles a NotifyMaintenance object
type NotifyMaintenanceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger

	Notifiers        []notify.NewNotifierFunc
	notifyAggregator notify.Notifier
	slaTimer         *slaTimer
	config           config.Config
	expectations     NMControllerExpectationsInterface
}

type reconcileReason string

const (
	reconcileFinalizer                   reconcileReason = "finalizer"
	reconcileInit                        reconcileReason = "init"
	reconcileTimer                       reconcileReason = "timer"
	reconcileNode                        reconcileReason = "node"
	reconcileMaintenanceClientAnnotation reconcileReason = "maintenanceClientAnnotation"
	reconcileValidationClientAnnotation  reconcileReason = "validationClientAnnotation"
	reconcileReadyForMaintenance         reconcileReason = "readyForMaintenance"
)

//+kubebuilder:rbac:groups=ngn2.nvidia.com,resources=notifymaintenances,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ngn2.nvidia.com,resources=notifymaintenances/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ngn2.nvidia.com,resources=notifymaintenances/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;watch;list;update
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;watch;list

// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *NotifyMaintenanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Reconcile will only occur when:
	//   1) A Node being watched by NotifyMaintenance is cordoned
	//   2) A NotifyMaintenance is CREATEd
	//   3) A NotifyMaintenance failed to Reconcile
	//   4) Add a finalizer to NotifyMaintenance
	//   5) SLA timer has expired on active NotifyMaintenances
	//   6) A Node was labeled with a new NotifyMaintenance

	recLog := r.Log.WithValues("NotifyMaintenance", req.NamespacedName)
	recLog.Info("Fetching NotifyMaintenance")

	nm := &ngn2v1alpha1.NotifyMaintenance{}
	node := &k8sv1.Node{}

	err := r.Client.Get(ctx, req.NamespacedName, nm)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		recLog.Error(err, "Error fetching NotifyMaintenance")
		return ctrl.Result{}, err
	}

	err = r.Client.Get(ctx, client.ObjectKeyFromObject(nm.GetNode()), node)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			recLog.Info("Node in .spec.nodeObject not found!", ".spec.nodeObject", nm.Spec.NodeObject)
			return ctrl.Result{}, nil
		}
		recLog.Error(err, "Error fetching node", ".spec.nodeObject", nm.Spec.NodeObject)
		return ctrl.Result{}, err
	}

	creationTime := nm.CreationTimestamp.Time
	initialStatus := nm.Status.MaintenanceStatus

	if time.Since(creationTime).Hours() > float64(nmRetentionDuration) {
		metrics.RecordStaleMaintenanceObject(nm.Name, nm.Namespace)
	}

	reason := r.getReconcileReason(nm)
	recLog.Info("Reconciling NotifyMaintenance", "reason", reason)
	switch reason {
	case reconcileMaintenanceClientAnnotation:
		err = r.onMaintenanceClientAnnotation(nm, node)
		if err != nil {
			return ctrl.Result{}, err
		}
	case reconcileValidationClientAnnotation:
		err = r.onValidationClientAnnotation(nm, node)
		if err != nil {
			return ctrl.Result{}, err
		}
	case reconcileFinalizer:
		err = r.onNMDeletion(nm, node)
		if err != nil {
			recLog.Error(err, "Error removing notify maintenance finalizer")
			return ctrl.Result{}, err
		}
	case reconcileInit:
		err = r.initNotifyMaintenance(nm)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to initialize NotifyMaintenance %s: %v", nm.String(), err)
		}
	case reconcileTimer:
		err = r.resumeSLACountdown(nm, node)
		if err != nil {
			return ctrl.Result{}, err
		}
	case reconcileReadyForMaintenance:
		err = r.onReadyForMaintenanceAnnotation(nm, node)
		if err != nil {
			return ctrl.Result{}, err
		}
	case reconcileNode:
		// Only reconcile when node has corresponding maintenance label set.
		if _, found := node.Labels[utils.MAINTENANCE_LABEL_PREFIX+nm.Spec.MaintenanceID]; !found && !nm.IsTerminal() && !utils.IsFinal(nm) {
			break
		}
		if node.Spec.Unschedulable &&
			nm.Status.MaintenanceStatus != ngn2v1alpha1.MaintenanceIncomplete &&
			nm.Status.MaintenanceStatus != ngn2v1alpha1.Validating {
			if nm.Status.MaintenanceStatus == ngn2v1alpha1.MaintenanceScheduled {
				// node is cordoned and labeled for maintenance but the NM object isn't in MaintenanceStarted
				// Notify SLA Start event
				err = r.onSLAStart(nm, node)
				if err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to notify SLAStart for NotifyMaintenance %s: %v", nm.String(), err)
				}
				return ctrl.Result{Requeue: true}, nil
			} else if nm.Status.MaintenanceStatus != ngn2v1alpha1.ObjectsDrained &&
				isAnnotationUpdated(nm, ngn2v1alpha1.ReadyForMaintenanceAnnotation.Key) {
				// node is drained.
				err = r.onNodeDrain(nm, node)
				if err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to notify NodeDrain for NotifyMaintenance %s: %v", nm.String(), err)
				}
			}
		}
		if !node.Spec.Unschedulable && controllerutil.ContainsFinalizer(nm, ngn2v1alpha1.NotifyMaintenanceFinalState) {
			// Node has been uncordoned and NM still contains finalizer, so we
			// should notify that maintenance has completed.
			err = r.onNodeUncordon(nm, node)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to notify NodeUncordon for NotifyMaintenance %s: %v", nm.String(), err)
			}
		}
	}

	r.expectations.DeleteExpectations()
	r.expectations.SetExpectations(string(initialStatus), string(afterStatus), nm.Name)
	return ctrl.Result{}, nil
}

func (r *NotifyMaintenanceReconciler) getReconcileReason(nm *ngn2v1alpha1.NotifyMaintenance) reconcileReason {
	if utils.IsFinal(nm) && !nm.IsTerminal() {
		return reconcileFinalizer
	}

	if nm.Status.MaintenanceStatus == ngn2v1alpha1.MaintenanceUnknown {
		return reconcileInit
	}

	if nm.Status.MaintenanceStatus == ngn2v1alpha1.MaintenanceStarted &&
		!nm.Status.SLAExpires.IsZero() && !nm.IsTerminal() &&
		(nm.Status.IsSLAExpired() || !r.slaTimer.HasCountDownBegun(nm)) {
		return reconcileTimer
	}

	if nm.Status.MaintenanceStatus == ngn2v1alpha1.ObjectsDrained && !nm.IsTerminal() {
		r.Log.Info("Maintenance in ObjectsDrained.  Check annotations", "Annotations", nm.Annotations)
		if isAnnotationUpdated(nm, ngn2v1alpha1.MaintenanceClientAnnotation.Key) {
			return reconcileMaintenanceClientAnnotation
		}
	}

	if nm.Status.MaintenanceStatus == ngn2v1alpha1.Validating && !nm.IsTerminal() {
		if isAnnotationUpdated(nm, ngn2v1alpha1.ValidationClientAnnotation.Key) {
			return reconcileValidationClientAnnotation
		}
	}

	if (nm.Status.MaintenanceStatus == ngn2v1alpha1.MaintenanceStarted || nm.Status.IsSLAExpired()) && !nm.IsTerminal() {
		if isAnnotationUpdated(nm, ngn2v1alpha1.ReadyForMaintenanceAnnotation.Key) {
			return reconcileReadyForMaintenance
		}
	}

	return reconcileNode
}

func isAnnotationUpdated(nm *ngn2v1alpha1.NotifyMaintenance, clientAnnotationType string) bool {
	annotationVal, aExists := nm.Annotations[clientAnnotationType]
	if aExists {
		cond, cExists := nm.Status.GetCondition(clientAnnotationType)
		if !cExists || (cExists && cond.Status != annotationVal) {
			return true
		}
	}
	return false
}

// initialize nm, expect .status.maintenanceStatus to be MaintenanceUnknown
func (r *NotifyMaintenanceReconciler) initNotifyMaintenance(nm *ngn2v1alpha1.NotifyMaintenance) (err error) {
	r.Log.Info("initializing NotifyMaintenance", "NotifyMaintenance", nm.String())
	update := false
	statusUpdate := false

	if !controllerutil.ContainsFinalizer(nm, ngn2v1alpha1.NotifyMaintenanceFinalState) {
		controllerutil.AddFinalizer(nm, ngn2v1alpha1.NotifyMaintenanceFinalState)
		update = true
	}

	label := utils.MAINTENANCE_LABEL_PREFIX + nm.Spec.MaintenanceID
	if _, labeled := nm.Labels[label]; !labeled {
		if nm.Labels == nil {
			nm.Labels = map[string]string{}
		}
		nm.Labels[label] = ""
		update = true
	}

	if update {
		err = r.Update(context.Background(), nm)
		if err != nil {
			return err
		}
	}

	newStatus := nm.Status.DeepCopy()
	if nm.Status.MaintenanceStatus == ngn2v1alpha1.MaintenanceUnknown {
		newStatus.MaintenanceStatus = ngn2v1alpha1.MaintenanceScheduled
		afterStatus = ngn2v1alpha1.MaintenanceScheduled
		statusUpdate = true
	}

	if statusUpdate {
		if r.expectations.SatisfiedExpectations(string(nm.Status.MaintenanceStatus), string(newStatus.MaintenanceStatus), nm.Name) {
			return nil
		}
		err = r.patchStatus(nm, newStatus)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *NotifyMaintenanceReconciler) setCondition(nm *ngn2v1alpha1.NotifyMaintenance, conditionType string, conditionStatus string) error {
	r.Log.Info("SetCondition", "NotifyMaintenance", nm.String(), "Condition type", conditionType, "New status", conditionStatus)

	newStatus := nm.Status.DeepCopy()
	cond, exists := newStatus.GetCondition(conditionType)
	if !exists {
		newCondition := ngn2v1alpha1.MaintenanceCondition{
			Type:               conditionType,
			Status:             conditionStatus,
			Reason:             "Annotation",
			LastTransitionTime: metav1.NewTime(time.Now()),
		}
		r.Log.Info("Adding condition", "NotifyMaintenance", nm.String())

		// Update the CR's status with the modified conditions.
		newStatus.Conditions = append(newStatus.Conditions, newCondition)
	} else if cond.Status != conditionStatus {
		r.Log.Info("Updating the existing condition", "NotifyMaintenance", nm.String(), "Condition type", conditionType, "Existing status", cond.Status, "New status", conditionStatus)
		newStatus.SetCondition(conditionType, conditionStatus)
	}

	err := r.patchStatus(nm, newStatus)
	if err != nil {
		return fmt.Errorf("failed to patch status for NotifyMaintenance %s: %v", nm.String(), err)
	}
	nm.Status = *newStatus
	return nil
}

func (r *NotifyMaintenanceReconciler) resumeSLACountdown(nm *ngn2v1alpha1.NotifyMaintenance, node *k8sv1.Node) error {
	if nm.Status.IsSLAExpired() {
		// SLA is expired
		r.Log.Info("resuming SLA expired notification delivery for NotifyMaintenance", "NotifyMaintenance", nm.String())
		err := r.onSLAExpire(node, nm)
		if err != nil {
			return err
		}
	} else if !r.slaTimer.HasCountDownBegun(nm) {
		// SLA is yet to be expired and countdown has not been registered, start the countdown
		r.Log.Info("resuming SLA countdown for NotifyMaintenance", "NotifyMaintenance", nm.String())
		r.slaTimer.StartCountDown(nm)
	}
	return nil
}

func (r *NotifyMaintenanceReconciler) patchStatus(nm *ngn2v1alpha1.NotifyMaintenance, newStatus *ngn2v1alpha1.NotifyMaintenanceStatus) (err error) {
	log.Info("Patch Status", "NotifyMaintenance", nm.String(), "new status", newStatus)

	changed := nm.DeepCopy()
	oldStatus := changed.Status.MaintenanceStatus

	if newStatus.MaintenanceStatus != oldStatus {
		now := metav1.NewTime(time.Now())
		log.Info("Patch Status", "NotifyMaintenance", nm.String(), "added new timetamp", now)
		ts := ngn2v1alpha1.StatusTransitionTimestamp{MaintenanceStatus: newStatus.MaintenanceStatus,
			TransitionTimestamp: now}
		newStatus.TransitionTimestamps = append(newStatus.TransitionTimestamps, ts)
	}

	changed.Status = *newStatus
	err = r.Status().Patch(context.Background(), changed, client.MergeFrom(nm))
	if err != nil {
		return fmt.Errorf("failed to patch status for NotifyMaintenance %s: %v", nm.String(), err)
	}

	if newStatus.MaintenanceStatus != oldStatus {
		log.Info("Patch Status", "NotifyMaintenance", nm.String(), "Recording in metrics status changed to", changed.Status.MaintenanceStatus)
		metrics.RecordPhaseTransitionTime(changed.Spec.MaintenanceID, changed.Status, oldStatus, changed.DeletionTimestamp)
		metrics.RecordMaintenanceStatusChange(changed, oldStatus)
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NotifyMaintenanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	var (
		err        error
		nodeMapper = &NodeMapper{
			Mapper: Mapper{
				nmRec: r,
			},
		}
		enqueueWithReconciler = &enqueue.WithReconciler{
			C: r.Client,
		}
	)

	r.slaTimer = newSlaTimer(r)
	r.expectations = NewNMControllerExpectation()
	r.config, err = config.ReadConfig(config.DefaultConfigPath) // TODO: This should be provided from command-line arguments.
	if err != nil {
		return fmt.Errorf("failed to read config, err: %w", err)
	}

	notifiers := make([]notify.Notifier, 0, len(r.Notifiers))
	for _, f := range r.Notifiers {
		n, err := f(r.config, r.Log)
		if err != nil {
			return fmt.Errorf("failed to initialize notifier, err: %w", err)
		}
		notifiers = append(notifiers, n)
	}

	r.notifyAggregator = notify.NewNotifyAggregator(notifiers...)

	err = mgr.GetFieldIndexer().IndexField(
		context.TODO(), &ngn2v1alpha1.NotifyMaintenance{}, "spec.nodeObject",
		func(rawObj client.Object) []string {
			nm := rawObj.(*ngn2v1alpha1.NotifyMaintenance)
			return []string{nm.Spec.NodeObject}
		},
	)

	if err != nil {
		return err
	}

	err = mgr.GetFieldIndexer().IndexField(
		context.TODO(), &ngn2v1alpha1.NotifyMaintenance{}, "spec.maintenanceID",
		func(rawObj client.Object) []string {
			nm := rawObj.(*ngn2v1alpha1.NotifyMaintenance)
			return []string{nm.Spec.MaintenanceID}
		},
	)

	if err != nil {
		return err
	}

	err = mgr.GetFieldIndexer().IndexField(
		context.TODO(), &ngn2v1alpha1.NotifyMaintenance{}, "status.maintenanceStatus",
		func(rawObj client.Object) []string {
			nm := rawObj.(*ngn2v1alpha1.NotifyMaintenance)
			return []string{string(nm.Status.MaintenanceStatus)}
		},
	)

	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&ngn2v1alpha1.NotifyMaintenance{}).
		Watches(
			&source.Kind{Type: &k8sv1.Node{}},
			enqueueWithReconciler.NodeEventHandler(nodeMapper.MapNodeUpdate, nodeMapper.MapNodeCreate),
		).
		Watches(
			&source.Channel{Source: r.slaTimer.eventChan},
			handler.EnqueueRequestsFromMapFunc(enqueue.GenericMapperFunc),
		).
		Complete(r)
}

// change .status.maintenanceStatus to SLAExpired and deliver notification
func (r *NotifyMaintenanceReconciler) onSLAExpire(obj client.Object, nm *ngn2v1alpha1.NotifyMaintenance) (err error) {
	if utils.IsFinal(nm) {
		r.Log.Info("NotifyMaintenance is been deleted, aborting notification delivery", "NotifyMaintenance", nm.String())
		return nil
	}

	if nm.Status.MaintenanceStatus == ngn2v1alpha1.SLAExpired {
		// SLAExpire notification was already delivered, nothing to do.
		return nil
	}

	if nm.Status.MaintenanceStatus != ngn2v1alpha1.MaintenanceStarted {
		// Incorrect precondition to send SLA expire.
		msg := fmt.Sprintf("aborting SLA expire notification: unexpected maintenance status, expecting %s, found %s",
			ngn2v1alpha1.MaintenanceStarted, nm.Status.MaintenanceStatus)
		r.Log.Info(msg, "NotifyMaintenance", nm.String())
		return nil
	}

	if r.expectations.SatisfiedExpectations(string(nm.Status.MaintenanceStatus), string(ngn2v1alpha1.SLAExpired), nm.Name) {
		return nil
	}

	err = r.notifyAggregator.NotifySLAExpire(obj, *nm)
	if err != nil {
		return fmt.Errorf("failed to notify SLA expiration for NotifyMaintenance %s: %v", nm.String(), err)
	}

	err = r.setMaintenanceStatus(nm, ngn2v1alpha1.SLAExpired)
	afterStatus = ngn2v1alpha1.SLAExpired
	return err
}

// Change `.status.maintenanceStatus` to MaintenanceStarted and deliver notification.
func (r *NotifyMaintenanceReconciler) onSLAStart(nm *ngn2v1alpha1.NotifyMaintenance, obj client.Object) (err error) {
	r.Log.Info("Start maintenance", "NotifyMaintenance", nm.String())

	if utils.IsFinal(nm) {
		r.Log.Info("NotifyMaintenance is been deleted, aborting notification delivery", "NotifyMaintenance", nm.String())
		return nil
	}

	if nm.Status.MaintenanceStatus == ngn2v1alpha1.MaintenanceStarted {
		// Maintenance has already started, nothing to do.
		return nil
	}

	// check if the state is changing from MaintenanceScheduled
	if nm.Status.MaintenanceStatus != ngn2v1alpha1.MaintenanceScheduled {
		msg := fmt.Sprintf("aborting SLA Start notification: unexpected maintenance status, expecting %s, found %s",
			ngn2v1alpha1.MaintenanceScheduled, nm.Status.MaintenanceStatus)
		r.Log.Info(msg, "NotifyMaintenance", nm.String())
		return nil
	}

	if r.expectations.SatisfiedExpectations(string(nm.Status.MaintenanceStatus), string(ngn2v1alpha1.MaintenanceStarted), nm.Name) {
		return nil
	}

	newNM := nm.DeepCopy()
	newNM.Status.MaintenanceStatus = ngn2v1alpha1.MaintenanceStarted

	// POST .Status.SLA on a newly created NotifyMaintenance
	if nm.Status.SLAExpires.IsZero() {
		r.Log.Info("Setting notify.Status.SLA", "NotifyMaintenance", nm.String())
		// start the SLA coutndown and set .status.slaExpires
		r.slaTimer.StartCountDown(newNM)
	}

	err = r.notifyAggregator.NotifySLAStart(obj, *newNM)
	if err != nil {
		return fmt.Errorf("failed to notify SLA start for NotifyMaintenance %s: %v", nm.String(), err)
	}

	err = r.patchStatus(nm, &newNM.Status)
	if err != nil {
		return err
	}

	afterStatus = ngn2v1alpha1.MaintenanceStarted
	return nil
}

func (r *NotifyMaintenanceReconciler) onNodeUncordon(nm *ngn2v1alpha1.NotifyMaintenance, node *k8sv1.Node) (err error) {
	// Maintenance is finished, remove finalizer and let NotifyMaintenance object to be deleted.

	// check and make sure NM is in appropriate states
	// if it is in MaintenanceUnknown or MaintenanceScheduled, the node is yet to be cordoned off
	// skip
	switch nm.Status.MaintenanceStatus { //nolint:exhaustive
	case ngn2v1alpha1.MaintenanceUnknown, ngn2v1alpha1.MaintenanceScheduled:
		return nil
	}

	shouldUpdate := false
	// Remove all maintenance labels by passing in a `nil` nm object
	_ = r.processMaintenanceLabel(node, nil, func(s string) error {
		delete(node.Labels, s)
		shouldUpdate = true
		return nil
	})

	if shouldUpdate {
		err = r.Client.Update(context.Background(), node)
		if err != nil {
			return err
		}
	}

	err = r.notifyAggregator.NotifyMaintenanceCompleted(node, *nm)
	if err != nil {
		return fmt.Errorf("failed to notify maintenance completed for NotifyMaintenance %s: %v", nm.String(), err)
	}

	metrics.RecordCompletedMaintenance(nm, nm.DeletionTimestamp)

	// Remove the finalizer.
	err = r.removeFinalizer(nm)
	if err != nil {
		return err
	}

	r.slaTimer.CancelCountDown(nm)

	return nil
}

func (r *NotifyMaintenanceReconciler) onMaintenanceClientAnnotation(nm *ngn2v1alpha1.NotifyMaintenance, node *k8sv1.Node) (err error) {
	r.Log.Info("MaintenanceClientAnnotation", "NotifyMaintenance", nm.String())
	annotationVal, exists := nm.Annotations[ngn2v1alpha1.MaintenanceClientAnnotation.Key]
	if !exists {
		r.Log.Error(err, "Error getting annotation for maintenance client")
		return err
	}

	cond, _ := nm.Status.GetCondition(ngn2v1alpha1.MaintenanceClientConditionType)
	if cond.Status != annotationVal {
		// Set condition Status to the new annotation value.
		err = r.setCondition(nm, ngn2v1alpha1.MaintenanceClientConditionType, annotationVal)
		if err != nil {
			r.Log.Error(err, "Error setting condition for maintenance client")
			return err
		}
	}

	switch annotationVal {
	case ngn2v1alpha1.MaintenanceClientComplete:
		if r.expectations.SatisfiedExpectations(string(nm.Status.MaintenanceStatus), string(ngn2v1alpha1.Validating), nm.Name) {
			return nil
		}
		err = r.setMaintenanceStatus(nm, ngn2v1alpha1.Validating)
		if err != nil {
			return fmt.Errorf("failed to set Validating status for NotifyMaintenance %s: %v", nm.String(), err)
		}
		err = r.notifyAggregator.NotifyValidating(node, *nm)
		if err != nil {
			return fmt.Errorf("failed to notify Validating status for NotifyMaintenance %s: %v", nm.String(), err)
		}

		afterStatus = ngn2v1alpha1.Validating
	case ngn2v1alpha1.MaintenanceClientIncomplete:
		if r.expectations.SatisfiedExpectations(string(nm.Status.MaintenanceStatus), string(ngn2v1alpha1.MaintenanceIncomplete), nm.Name) {
			return nil
		}
		err = r.setMaintenanceStatus(nm, ngn2v1alpha1.MaintenanceIncomplete)
		if err != nil {
			return fmt.Errorf("failed to set Maintenance Incomplete status for NotifyMaintenance %s: %v", nm.String(), err)
		}
		err = r.notifyAggregator.NotifyMaintenanceIncomplete(node, *nm)
		if err != nil {
			return fmt.Errorf("failed to notify Maintenance Incomplete status for NotifyMaintenance %s: %v", nm.String(), err)
		}

		afterStatus = ngn2v1alpha1.MaintenanceIncomplete
	}
	return nil
}

func (r *NotifyMaintenanceReconciler) onValidationClientAnnotation(nm *ngn2v1alpha1.NotifyMaintenance, node *k8sv1.Node) (err error) {
	r.Log.Info("ValidationClientAnnotation", "NotifyMaintenance", nm.String())
	annotationVal, exists := nm.Annotations[ngn2v1alpha1.ValidationClientAnnotation.Key]
	if !exists {
		r.Log.Error(err, "Error getting annotation for validation client")
		return err
	}

	cond, _ := nm.Status.GetCondition(ngn2v1alpha1.ValidationClientConditionType)
	if cond.Status != annotationVal {
		// Set condition Status to the new annotation value.
		err = r.setCondition(nm, ngn2v1alpha1.ValidationClientConditionType, annotationVal)
		if err != nil {
			return err
		}
	}

	if annotationVal == ngn2v1alpha1.ValidationClientFailed {
		if r.expectations.SatisfiedExpectations(string(nm.Status.MaintenanceStatus), string(ngn2v1alpha1.MaintenanceIncomplete), nm.Name) {
			return nil
		}
		err = r.setMaintenanceStatus(nm, ngn2v1alpha1.MaintenanceIncomplete)
		if err != nil {
			return fmt.Errorf("failed to set Maintenance Incomplete status for NotifyMaintenance %s: %v", nm.String(), err)
		}
		err = r.notifyAggregator.NotifyMaintenanceIncomplete(node, *nm)
		if err != nil {
			return fmt.Errorf("failed to notify Maintenance Incomplete status for NotifyMaintenance %s: %v", nm.String(), err)
		}

		afterStatus = ngn2v1alpha1.MaintenanceIncomplete
	}
	return nil
}

// change .status.maintenanceStatus from SLAExpired or MaintenanceStarted to ObjectsDrained and deliver notification
func (r *NotifyMaintenanceReconciler) onReadyForMaintenanceAnnotation(nm *ngn2v1alpha1.NotifyMaintenance, node *k8sv1.Node) (err error) {
	r.Log.Info("onReadyForMaintenanceAnnotation", "NotifyMaintenance", nm.String())

	if utils.IsFinal(nm) {
		r.Log.Info("NotifyMaintenance is been deleted, aborting notification delivery", "NotifyMaintenance", nm.String())
		return nil
	}

	annotationVal, exists := nm.Annotations[ngn2v1alpha1.ReadyForMaintenanceAnnotation.Key]
	if !exists {
		r.Log.Error(err, "Error getting annotation value for: ", "key", ngn2v1alpha1.ReadyForMaintenanceAnnotation.Key)
		return err
	}

	cond, _ := nm.Status.GetCondition(ngn2v1alpha1.ReadyForMaintenanceConditionType)
	if cond.Status != annotationVal && annotationVal == "true" {
		if r.expectations.SatisfiedExpectations(string(nm.Status.MaintenanceStatus), string(ngn2v1alpha1.ObjectsDrained), nm.Name) {
			return nil
		}

		err = r.notifyAggregator.NotifyNodeDrain(node, *nm)
		if err != nil {
			return fmt.Errorf("failed to notify node drain for NotifyMaintenance %s: %v", nm.String(), err)
		}

		// Set condition Status to the new annotation value.
		err = r.setCondition(nm, ngn2v1alpha1.ReadyForMaintenanceConditionType, annotationVal)
		if err != nil {
			return err
		}

		err = r.setMaintenanceStatus(nm, ngn2v1alpha1.ObjectsDrained)
		if err == nil {
			afterStatus = ngn2v1alpha1.ObjectsDrained
		}

		return err
	}
	return nil
}

// change .status.maintenanceStatus from SLAExpire or MaintenanceStarted to ObjectsDrained and deliver notification
func (r *NotifyMaintenanceReconciler) onNodeDrain(nm *ngn2v1alpha1.NotifyMaintenance, obj client.Object) (err error) {
	r.Log.Info("Drained node", "NotifyMaintenance", nm.String())

	if utils.IsFinal(nm) {
		r.Log.Info("NotifyMaintenance is been deleted, aborting notification delivery", "NotifyMaintenance", nm.String())
		return nil
	}

	// We are reconciling the NM state based on the Node "state".
	// Since NM state and Node "state" are held on different objects, what normally would be a 409 error turns into a race.
	// In these scenarios, requeue on failures and reconcile should get us to the correct state.
	//    race: Node Drain Annotation vs transitioning to ngn2v1alpha1.MaintenanceStarted
	if nm.Status.MaintenanceStatus == ngn2v1alpha1.MaintenanceScheduled || nm.Status.MaintenanceStatus == ngn2v1alpha1.MaintenanceUnknown {
		msg := fmt.Sprintf("requeuing node drain notification: unexpected maintenance status, expecting either %s or %s, found %s",
			ngn2v1alpha1.MaintenanceStarted, ngn2v1alpha1.SLAExpired, nm.Status.MaintenanceStatus)
		r.Log.Info(msg, "NotifyMaintenance", nm.String())
		return utils.NM_STATE_OLD
	}

	if r.expectations.SatisfiedExpectations(string(nm.Status.MaintenanceStatus), string(ngn2v1alpha1.ObjectsDrained), nm.Name) {
		return nil
	}

	err = r.notifyAggregator.NotifyNodeDrain(obj, *nm)
	if err != nil {
		return fmt.Errorf("failed to notify node drain for NotifyMaintenance %s: %v", nm.String(), err)
	}

	err = r.setMaintenanceStatus(nm, ngn2v1alpha1.ObjectsDrained)
	if err == nil {
		afterStatus = ngn2v1alpha1.ObjectsDrained
	}

	return err
}

func (r NotifyMaintenanceReconciler) onNMDeletion(nm *ngn2v1alpha1.NotifyMaintenance, node *k8sv1.Node) (err error) {
	// Notify Node Maintenance completed
	log.Info("Maintenance ended for object", "object", nm.Spec.NodeObject)
	r.slaTimer.CancelCountDown(nm)

	oldNM := nm.DeepCopy()
	oldStatus := oldNM.Status.MaintenanceStatus

	updateTerminal := false
	switch nm.Status.MaintenanceStatus {
	case ngn2v1alpha1.MaintenanceUnknown, ngn2v1alpha1.MaintenanceScheduled:
		// if the maintenance is yet to be started, do not block on the uncordon event and delete nm immediately
		err = r.removeFinalizer(nm)
		if err != nil {
			return fmt.Errorf("failed to remove finalizer on NotifyMaintenance %s: %v", nm.String(), err)
		}

		metrics.DeleteStaleMaintenaceObject(nm.Name, nm.Namespace)

		err = r.processMaintenanceLabel(node, nm, func(s string) error {
			delete(node.Labels, s)
			return r.Update(context.Background(), node)
		})
		if err != nil {
			return fmt.Errorf("failed to remove maintenance label on node %s: %v", node.Name, err)
		}
	case ngn2v1alpha1.MaintenanceStarted, ngn2v1alpha1.SLAExpired:
		err = r.notifyAggregator.NotifyMaintenanceCancellation(node, *nm)
		if err != nil {
			return fmt.Errorf("failed to notify maintenance cancellation for NotifyMaintenance %s: %v", nm.String(), err)
		}
		updateTerminal = true
	case ngn2v1alpha1.ObjectsDrained, ngn2v1alpha1.MaintenanceIncomplete, ngn2v1alpha1.Validating:
		err = r.notifyAggregator.NotifyMaintenanceEnded(node, *nm)
		if err != nil {
			return fmt.Errorf("failed to notify end of maintenance for NotifyMaintenance %s: %v", nm.String(), err)
		}
		updateTerminal = true
	}

	if updateTerminal {
		if nm.Annotations == nil {
			nm.Annotations = map[string]string{}
		}
		if _, exist := nm.Annotations[ngn2v1alpha1.NMTerminalAnnotation]; !exist {
			nm.Annotations[ngn2v1alpha1.NMTerminalAnnotation] = ""
			err = r.Update(context.Background(), nm)
			if err != nil {
				return err
			}

			// The DeletionTimestamp will always equal the MaintenanceEnded transition.
			// Call metrics so that we properly set the pika_maintenance_status gauge edge case.
			metrics.RecordPhaseTransitionTime(nm.Spec.MaintenanceID, nm.Status, oldStatus, nm.DeletionTimestamp)
			metrics.RecordMaintenanceStatusChange(nm, oldStatus)
		}
	}

	return nil
}

func (r *NotifyMaintenanceReconciler) removeFinalizer(nm *ngn2v1alpha1.NotifyMaintenance) error {
	nmNew := ngn2v1alpha1.NotifyMaintenance{}
	err := r.Get(context.Background(), client.ObjectKeyFromObject(nm), &nmNew)
	if err != nil {
		return err
	}

	controllerutil.RemoveFinalizer(&nmNew, ngn2v1alpha1.NotifyMaintenanceFinalState)
	return r.Update(context.Background(), &nmNew)
}

func (r *NotifyMaintenanceReconciler) processMaintenanceLabel(node *k8sv1.Node, nm *ngn2v1alpha1.NotifyMaintenance,
	processor func(string) error) error {
	if nm == nil {
		r.Log.Info("Removing all maintenance labels on Node", "Node", node.Name)
		for labelKey := range node.Labels {
			if strings.HasPrefix(labelKey, utils.MAINTENANCE_LABEL_PREFIX) || strings.HasPrefix(labelKey, utils.CORDONED_LABEL_KEY) {
				r.Log.Info("Removing label", "Label", labelKey)
				err := processor(labelKey)
				if err != nil {
					return err
				}
			}
		}
	} else {
		for labelKey := range node.Labels {
			if strings.HasPrefix(labelKey, utils.MAINTENANCE_LABEL_PREFIX) {
				maintenanceID := strings.TrimPrefix(labelKey, utils.MAINTENANCE_LABEL_PREFIX)
				if maintenanceID == nm.Spec.MaintenanceID {
					return processor(labelKey)
				}
			}
		}
	}
	return nil
}

func (r *NotifyMaintenanceReconciler) setMaintenanceStatus(nm *ngn2v1alpha1.NotifyMaintenance, status ngn2v1alpha1.MaintenanceStatus) error {
	newStatus := nm.Status.DeepCopy()
	newStatus.MaintenanceStatus = status
	err := r.patchStatus(nm, newStatus)
	if err != nil {
		return err
	}
	return nil
}

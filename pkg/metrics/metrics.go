package metrics

import (
	"fmt"
	"time"

	ngn2v1alpha1 "github.com/NVIDIA/pika/api/v1alpha1"

	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var log = logf.Log.WithName("notifymaintenance_controller")
var recordedStaleNotifyMaintenances map[string]bool = make(map[string]bool)

const (
	subsystem = "pika"
)

var (
	notificationSuccessCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: subsystem,
			Name:      "notification_success_total",
			Help:      "",
		},
		[]string{"notifier"},
	)

	notificationFailCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: subsystem,
			Name:      "notification_fail_total",
			Help:      "",
		},
		[]string{"notifier"},
	)

	nmMaintenanceStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: subsystem,
			Name:      "maintenance_status",
			Help:      "",
		},
		[]string{"maintenance_status", "maintenance_id", "node", "owner", "maintenance_type"},
	)

	nmOrphanedMaintenanceObjs = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: subsystem,
			Name:      "orphaned_maintenance_objects",
			Help:      "",
		},
		[]string{"nm_name", "nm_namespace"},
	)

	nmMaintenanceStatusTransitionLatency = newMaintenanceStatusTransitionTimeHistogramVec()

	plannedMaintenanceStateGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Subsystem: subsystem,
			Name:      "planned_maintenance_state",
			Help:      "",
		})
)

func NotificationSuccessCount(notifier string) {
	notificationSuccessCount.With(prometheus.Labels{"notifier": notifier}).Inc()
}

func NotificationFailCount(notifier string) {
	notificationFailCount.With(prometheus.Labels{"notifier": notifier}).Inc()
}

func RecordStaleMaintenanceObject(nmName string, nmNamespace string) {
	key := nmName + "/" + nmNamespace

	_, ok := recordedStaleNotifyMaintenances[key]
	if !ok {
		nmOrphanedMaintenanceObjs.With(prometheus.Labels{
			"nm_name":      nmName,
			"nm_namespace": nmNamespace,
		}).Inc()

		recordedStaleNotifyMaintenances[key] = true
	}
}

func DeleteStaleMaintenaceObject(nmName string, nmNamespace string) {
	key := nmName + "/" + nmNamespace

	val, ok := recordedStaleNotifyMaintenances[key]
	if ok && val {
		nmOrphanedMaintenanceObjs.With(prometheus.Labels{
			"nm_name":      nmName,
			"nm_namespace": nmNamespace,
		}).Dec()

		delete(recordedStaleNotifyMaintenances, key)
	}
}

func RecordMaintenanceStatusChange(nm *ngn2v1alpha1.NotifyMaintenance, oldStatus ngn2v1alpha1.MaintenanceStatus) {
	log.Info("RecordMaintenanceStatusChange", "NotifyMaintenance", nm.Spec.MaintenanceID, "old status", oldStatus, "new status", nm.Status.MaintenanceStatus)
	// Only occurs when we're deleting an object:
	//   MaintenanceEnded and MaintenanceCompleted are not API states.
	//   So the transition from <current_state> to MaintenanceEnded is actually
	//   a transition from <current_state> to <current_state>.
	if nm.Status.MaintenanceStatus == oldStatus {
		nmMaintenanceStatus.With(prometheus.Labels{
			"maintenance_status": string(oldStatus),
			"maintenance_id":     nm.Spec.MaintenanceID,
			"node":               nm.Spec.NodeObject,
			"owner":              nm.Spec.Owner,
			"maintenance_type":   nm.Spec.Type,
		}).Dec()

		log.Info("RecordMaintenanceEndedStatusChange", "NotifyMaintenance", nm.Spec.MaintenanceID, "old status", oldStatus, "new status", nm.Status.MaintenanceStatus)
		return
	}

	// TODO: There's room for us to expand these labels to include annotations.
	//       This would allow us to track metrics on Maintenance Clients and
	//       Validation Clients.
	nmMaintenanceStatus.With(prometheus.Labels{
		"maintenance_status": string(nm.Status.MaintenanceStatus),
		"maintenance_id":     nm.Spec.MaintenanceID,
		"node":               nm.Spec.NodeObject,
		"owner":              nm.Spec.Owner,
		"maintenance_type":   nm.Spec.Type,
	}).Inc()

	// ngn2v1alpha1.MaintenanceUnknown is a starting phase, so don't track it.
	if oldStatus != ngn2v1alpha1.MaintenanceUnknown {
		nmMaintenanceStatus.With(prometheus.Labels{
			"maintenance_status": string(oldStatus),
			"maintenance_id":     nm.Spec.MaintenanceID,
			"node":               nm.Spec.NodeObject,
			"owner":              nm.Spec.Owner,
			"maintenance_type":   nm.Spec.Type,
		}).Dec()
	}
}

func RecordPhaseTransitionTime(nmID string, newStatus ngn2v1alpha1.NotifyMaintenanceStatus, oldStatus ngn2v1alpha1.MaintenanceStatus, deletionTimestamp *metav1.Time) {
	log.Info("RecordPhaseTransitionTime", "NotifyMaintenance", nmID, "old status", oldStatus, "new status", newStatus.MaintenanceStatus)

	if newStatus.MaintenanceStatus == ngn2v1alpha1.MaintenanceUnknown {
		return
	}

	prevTimeStamp := getPrevTimeStamp(newStatus, deletionTimestamp)
	if prevTimeStamp == nil {
		log.Info("RecordPhaseTransitionTime. prevTimeStamp is nil or oldStatus is MaintenanceUnknown", "NotifyMaintenance", nmID)
		return
	}

	diffMinutes, err := getTransitionTimeMinutes(newStatus, deletionTimestamp)
	if err != nil {
		log.Info("RecordPhaseTransitionTime. getTransitionTimeMinutes returned error", "Error: ", err)
		return
	}

	nmMaintenanceStatusTransitionLatency.With(prometheus.Labels{
		"status":         string(prevTimeStamp.MaintenanceStatus),
		"maintenance_id": nmID,
	}).Observe(diffMinutes)

	log.Info("RecordPhaseTransitionTime. Called to observe with", "prevTimeStamp.MaintenanceStatus:", prevTimeStamp.MaintenanceStatus, "diffMinutes: ", diffMinutes)
}

// Record phase transiton time from MaintenanceEnded to MaintenanceCompleted and the mainenance details.
func RecordCompletedMaintenance(nm *ngn2v1alpha1.NotifyMaintenance, deletionTimestamp *metav1.Time) {
	l := len(nm.Status.TransitionTimestamps)
	currTime := time.Now()
	var prevTime time.Time

	if deletionTimestamp.IsZero() {
		if l < 1 {
			return
		}
		prevTime = nm.Status.TransitionTimestamps[l-1].TransitionTimestamp.Time
	} else {
		prevTime = deletionTimestamp.Time
	}

	diffMinutes := currTime.Sub(prevTime).Minutes()

	nmMaintenanceStatusTransitionLatency.With(prometheus.Labels{
		"status":         "MaintenanceCompleted",
		"maintenance_id": nm.Spec.MaintenanceID,
	}).Observe(diffMinutes)

	// Record the completed maintenances.
	nmMaintenanceStatus.With(prometheus.Labels{
		"maintenance_status": "MaintenanceCompleted",
		"maintenance_id":     nm.Spec.MaintenanceID,
		"node":               nm.Spec.NodeObject,
		"owner":              nm.Spec.Owner,
		"maintenance_type":   nm.Spec.Type,
	}).Inc()

	log.Info("RecordCompletedMaintenance. From MaintenanceEnded to MaintenanceCompleted:", "NotifyMaintenance", nm.Spec.MaintenanceID, "diffMinutes: ", diffMinutes)
}

func init() {
	metrics.Registry.MustRegister(
		notificationSuccessCount,
		notificationFailCount,
		nmMaintenanceStatus,
		nmOrphanedMaintenanceObjs,
		nmMaintenanceStatusTransitionLatency,
		plannedMaintenanceStateGauge,
	)
}

func nmStatusTransitionTimeBuckets() []float64 {
	return []float64{
		(1 * time.Second.Minutes()),    // 1 Seconds
		(10 * time.Second.Minutes()),   // 10 Seconds
		(30 * time.Second.Minutes()),   // 30 Seconds
		(1.0 * time.Minute.Minutes()),  // 1 minute
		(2.0 * time.Minute.Minutes()),  // 2 minutes
		(3.0 * time.Minute.Minutes()),  // 3 minutes
		(4.0 * time.Minute.Minutes()),  // 4 minutes
		(5.0 * time.Minute.Minutes()),  // 5 minutes
		(7.0 * time.Minute.Minutes()),  // 7 minutes
		(8.0 * time.Minute.Minutes()),  // 8 minutes
		(10.0 * time.Minute.Minutes()), // 10 minutes
		(12.0 * time.Minute.Minutes()), // 12 minutes
		(15.0 * time.Minute.Minutes()), // 15 minutes
		(20.0 * time.Minute.Minutes()), // 20 minutes
		(22.0 * time.Minute.Minutes()), // 22 minutes
		(25.0 * time.Minute.Minutes()), // 25 minutes
		(30.0 * time.Minute.Minutes()), // 30 minutes
		(35.0 * time.Minute.Minutes()), // 35 minutes
		(40.0 * time.Minute.Minutes()), // 40 minutes
		(50.0 * time.Minute.Minutes()), // 50 minutes
		(1.0 * time.Hour.Minutes()),    // 1 hour
		(2.0 * time.Hour.Minutes()),    // 2 hours
		(3.0 * time.Hour.Minutes()),    // 3 hours
		(4.0 * time.Hour.Minutes()),    // 4 hours
		(5.0 * time.Hour.Minutes()),    // 5 hours
		(6.0 * time.Hour.Minutes()),    // 6 hours
		(12.0 * time.Hour.Minutes()),   // 12 hours
		(18.0 * time.Hour.Minutes()),   // 18 hours
		(24.0 * time.Hour.Minutes()),   // 24 hours (1 day)
		(48.0 * time.Hour.Minutes()),   // 48 hours (2 days)
		(72.0 * time.Hour.Minutes()),   // 72 hours (3 days)
		(120.0 * time.Hour.Minutes()),  // 120 hours (5 days)
	}
}

func newMaintenanceStatusTransitionTimeHistogramVec() *prometheus.HistogramVec {
	histogramVec := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{ //nolint:promlinter // It should be `_seconds` but it is already used in dashboards.
			Subsystem: subsystem,
			Name:      "maintenance_status_transition_time_minutes",
			Buckets:   nmStatusTransitionTimeBuckets(),
			Help:      "",
		},
		[]string{"status", "maintenance_id"},
	)
	return histogramVec
}

func getTransitionTimeMinutes(newStatus ngn2v1alpha1.NotifyMaintenanceStatus, deletionTimestamp *metav1.Time) (float64, error) {
	l := len(newStatus.TransitionTimestamps)
	var currTime, prevTime metav1.Time

	if !deletionTimestamp.IsZero() {
		if l < 1 {
			return 0.0, fmt.Errorf("missing status transition timestamp value")
		}
		currTime = *deletionTimestamp
		prevTime = newStatus.TransitionTimestamps[l-1].TransitionTimestamp
	} else if l < 2 {
		return 0.0, fmt.Errorf("missing status transition timestamp value")
	} else {
		currTime = newStatus.TransitionTimestamps[l-1].TransitionTimestamp
		prevTime = newStatus.TransitionTimestamps[l-2].TransitionTimestamp
	}

	if currTime.IsZero() || prevTime.IsZero() || currTime.Before(&prevTime) {
		log.Info("getTransitionTimeMinutes. invalid times", "Current time:", currTime, "Previous time:", prevTime)
		return 0.0, fmt.Errorf("invalid status transition timestamp values")
	}

	// Calculate the difference in minutes
	return currTime.Sub(prevTime.Time).Minutes(), nil
}

func getPrevTimeStamp(newStatus ngn2v1alpha1.NotifyMaintenanceStatus, deletionTimestamp *metav1.Time) (prevTimeStamp *ngn2v1alpha1.StatusTransitionTimestamp) {
	if !deletionTimestamp.IsZero() {
		if len(newStatus.TransitionTimestamps) >= 1 {
			return &newStatus.TransitionTimestamps[len(newStatus.TransitionTimestamps)-1]
		}
	}

	if len(newStatus.TransitionTimestamps) < 2 {
		return nil
	}
	return &newStatus.TransitionTimestamps[len(newStatus.TransitionTimestamps)-2]
}

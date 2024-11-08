package controllers

import (
	"sync"
	"time"

	ngn2v1alpha1 "github.com/NVIDIA/pika/api/v1alpha1"

	"github.com/emirpasic/gods/sets/hashset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

type timerNodeInfo struct {
	cancelCh        *stopCh
	expiryTimestamp time.Time
	nmSet           *hashset.Set
}

type slaTimer struct {
	nmRec    *NotifyMaintenanceReconciler
	dirMutex sync.Mutex

	eventChan   chan event.GenericEvent
	nodeInfoMap map[string]*timerNodeInfo
}

type (
	// StopCh is specialized channel for stopping things.
	stopCh struct {
		once sync.Once
		ch   chan struct{}
	}
)

func newStopCh() *stopCh {
	return &stopCh{ch: make(chan struct{})}
}

func (sc *stopCh) Listen() <-chan struct{} {
	return sc.ch
}

func (sc *stopCh) Close() {
	sc.once.Do(func() {
		close(sc.ch)
	})
}

func newSlaTimer(nmRec *NotifyMaintenanceReconciler) *slaTimer {
	return &slaTimer{
		eventChan:   make(chan event.GenericEvent),
		nmRec:       nmRec,
		nodeInfoMap: make(map[string]*timerNodeInfo),
	}
}

func (t *slaTimer) StartCountDown(nm *ngn2v1alpha1.NotifyMaintenance) {
	if t.HasCountDownBegun(nm) {
		log.Info("SLA timer for Node already exists, skipping SLA countdown", "Node", nm.Spec.NodeObject)
		return
	}

	if nm.Status.IsSLAExpired() {
		log.Info("SLA timer for Node has already expired, skipping SLA countdown", "Node", nm.Spec.NodeObject)
		return
	}

	done := t.registerCountDown(nm)
	timer := time.NewTimer(time.Until(nm.Status.SLAExpires.Time))

	go func() {
		defer t.unregisterCountDown(nm)

		select {
		case <-done.Listen():
			log.Info("SLA countdown cancelled", "Node", nm.Spec.NodeObject)
			return
		case <-timer.C:
			// queue for reconciliation
			t.eventChan <- event.GenericEvent{
				Object: nm,
			}
		}
	}()
}

func (t *slaTimer) registerCountDown(nm *ngn2v1alpha1.NotifyMaintenance) *stopCh {
	t.dirMutex.Lock()
	defer t.dirMutex.Unlock()

	nodeInfo, nodeRegistered := t.nodeInfoMap[nm.Spec.NodeObject]
	if !nodeRegistered || nodeInfo == nil {
		newNodeInfo := &timerNodeInfo{
			cancelCh:        newStopCh(),
			expiryTimestamp: metav1.Now().Add(t.nmRec.config.SLAPeriod),
			nmSet:           hashset.New(nm.String()),
		}
		t.nodeInfoMap[nm.Spec.NodeObject] = newNodeInfo
		nm.Status.SLAExpires = &metav1.Time{Time: newNodeInfo.expiryTimestamp}
		return newNodeInfo.cancelCh
	} else {
		nodeInfo.nmSet.Add(nm.String())
		if nm.Status.SLAExpires.IsZero() {
			nm.Status.SLAExpires = &metav1.Time{Time: nodeInfo.expiryTimestamp}
		}
		return nodeInfo.cancelCh
	}
}

func (t *slaTimer) unregisterCountDown(nm *ngn2v1alpha1.NotifyMaintenance) {
	t.dirMutex.Lock()
	defer t.dirMutex.Unlock()

	if nodeInfo, nodeRegistered := t.nodeInfoMap[nm.Spec.NodeObject]; nodeRegistered && nodeInfo != nil {
		nodeInfo.nmSet.Remove(nm.String())
		if nodeInfo.nmSet.Empty() {
			delete(t.nodeInfoMap, nm.Spec.NodeObject)
		}
	}
}

func (t *slaTimer) CancelCountDown(nm *ngn2v1alpha1.NotifyMaintenance) {
	if nodeInfo, exists := t.nodeInfoMap[nm.Spec.NodeObject]; exists && nodeInfo != nil {
		nodeInfo.cancelCh.Close()
	}
}

func (t *slaTimer) HasCountDownBegun(nm *ngn2v1alpha1.NotifyMaintenance) bool {
	t.dirMutex.Lock()
	defer t.dirMutex.Unlock()

	if nodeInfo, nodeRegistered := t.nodeInfoMap[nm.Spec.NodeObject]; nodeRegistered && nodeInfo != nil {
		return nodeInfo.nmSet.Contains(nm.String())
	} else {
		return false
	}
}

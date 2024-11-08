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
	"github.com/NVIDIA/pika/pkg/utils"

	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	NotifyMaintenanceFinalState = "ngn2.nvidia.com/finalNotice"
	PodNMAnnotation             = "ngn2.nvidia.com/notify"
	NMTerminalAnnotation        = "maintenance.ngn2.nvidia.com/terminated"

	// MaintenanceClientAnnotation values.
	MaintenanceClientIncomplete = "incomplete"
	MaintenanceClientInprogress = "inprogress"
	MaintenanceClientComplete   = "complete"

	// ValidationClientAnnotation values.
	ValidationClientInProgress = "inprogress"
	ValidationClientValidated  = "validated"
	ValidationClientFailed     = "failed"

	// Condition types.
	MaintenanceClientConditionType   = "maintenanceClient"
	ValidationClientConditionType    = "validationClient"
	ReadyForMaintenanceConditionType = "readyForMaintenance"

	PlannedMaintenance   = "planned"
	UnplannedMaintenance = "unplanned"
)

type Annotation struct {
	Key              string
	ValidValues      []string
	FinalStateValues []string
	ConditionKey     string
}

var MaintenanceClientAnnotation = Annotation{
	Key:              utils.MAINTENANCE_LABEL_PREFIX + MaintenanceClientConditionType,
	ValidValues:      []string{MaintenanceClientIncomplete, MaintenanceClientInprogress, MaintenanceClientComplete},
	FinalStateValues: []string{MaintenanceClientIncomplete, MaintenanceClientComplete},
	ConditionKey:     MaintenanceClientConditionType,
}

var ValidationClientAnnotation = Annotation{
	Key:              utils.MAINTENANCE_LABEL_PREFIX + ValidationClientConditionType,
	ValidValues:      []string{ValidationClientInProgress, ValidationClientValidated, ValidationClientFailed},
	FinalStateValues: []string{ValidationClientValidated, ValidationClientFailed},
	ConditionKey:     ValidationClientConditionType,
}

var ReadyForMaintenanceAnnotation = Annotation{
	Key:              utils.MAINTENANCE_LABEL_PREFIX + ReadyForMaintenanceConditionType,
	ValidValues:      []string{"true", "false"},
	FinalStateValues: []string{"true"},
	ConditionKey:     ReadyForMaintenanceConditionType,
}

var ValidatedAnnotations = []Annotation{
	MaintenanceClientAnnotation,
	ValidationClientAnnotation,
	ReadyForMaintenanceAnnotation,
}

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// NotifyMaintenanceSpec defines the desired state of NotifyMaintenance.
type NotifyMaintenanceSpec struct {
	// Node object headed for Maintenance.
	NodeObject string `json:"nodeObject"`
	// MaintenanceID is a unique ID for a maintenance task.
	// +kubebuilder:validation:Pattern=`([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]`
	MaintenanceID             string          `json:"maintenanceID"`
	AdditionalMessageChannels MessageChannels `json:"additionalMessageChannels,omitempty"`

	// Planned or Unplanned
	Type string `json:"maintenanceType"`

	// Metadata for Maintenance Clients
	MetadataConfigmap string `json:"metadataConfigmap,omitempty"`

	// Owner (PIC) for Maintenance
	Owner string `json:"owner,omitempty"`
}

type MessageChannels struct {
	// AmazonSNSTopicARN is the Amazon SNS topic that events for this object will be published to.
	AmazonSNSTopicARN string `json:"amazonSNSTopicARN,omitempty"`
}

// NotifyMaintenanceStatus defines the observed state of NotifyMaintenance.
type NotifyMaintenanceStatus struct {
	SLAExpires           *metav1.Time                `json:"slaExpires,omitempty"`
	MaintenanceStatus    MaintenanceStatus           `json:"maintenanceStatus,omitempty"`
	Conditions           []MaintenanceCondition      `json:"conditions,omitempty"`
	TransitionTimestamps []StatusTransitionTimestamp `json:"transitionTimestamps,omitempty"`
}

// StatusTransitionTimestamp defines the timestamp when the transition to the specified MaintenanceStatus occurred.
type StatusTransitionTimestamp struct {
	MaintenanceStatus   MaintenanceStatus `json:"maintenanceStatus,omitempty"`
	TransitionTimestamp metav1.Time       `json:"transitionTimestamp,omitempty"`
}

// MaintenanceCondition defines custom conditions.
type MaintenanceCondition struct {
	Type               string      `json:"type"`
	Status             string      `json:"status"`
	Reason             string      `json:"reason,omitempty"`
	Message            string      `json:"message,omitempty"`
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
}

type MaintenanceStatus string

const (
	MaintenanceUnknown    MaintenanceStatus = ""
	MaintenanceScheduled  MaintenanceStatus = "MaintenanceScheduled"
	MaintenanceStarted    MaintenanceStatus = "MaintenanceStarted"
	SLAExpired            MaintenanceStatus = "SLAExpired"
	ObjectsDrained        MaintenanceStatus = "ObjectsDrained"
	MaintenanceIncomplete MaintenanceStatus = "MaintenanceIncomplete"
	Validating            MaintenanceStatus = "Validating"
)

var (
	InProgressStatuses = []MaintenanceStatus{ObjectsDrained, MaintenanceStarted, SLAExpired, Validating}
)

func IsNotifyMaintenanceStatusInProgress(status MaintenanceStatus) bool {
	// Can't use slices.Contains function, because it works only for string arrays.
	for _, s := range InProgressStatuses {
		if status == s {
			return true
		}
	}
	return false
}

func GetInProgressMaintenanceStatuses() []MaintenanceStatus {
	return InProgressStatuses
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:singular=notifymaintenance
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="MaintenanceID",type="string",JSONPath=".spec.maintenanceID"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.maintenanceStatus"
// +kubebuilder:printcolumn:name="Node",type="string",JSONPath=".spec.nodeObject"
// +kubebuilder:printcolumn:name="SLAExpiry",type="string",JSONPath=`.status.slaExpires`
// +kubebuilder:printcolumn:name="ReadyForMaintenance",type="string",JSONPath=".status.conditions[?(@.type=='ObjectsDrained')].status"
// NotifyMaintenance is the Schema for the notifymaintenances API
type NotifyMaintenance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NotifyMaintenanceSpec   `json:"spec,omitempty"`
	Status NotifyMaintenanceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NotifyMaintenanceList contains a list of NotifyMaintenance.
type NotifyMaintenanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NotifyMaintenance `json:"items"`
}

func (nm NotifyMaintenance) String() string {
	return nm.Namespace + "/" + nm.Name
}

// returns .spec.nodeObject as a k8sv1.Node object
func (nm NotifyMaintenance) GetNode() *k8sv1.Node {
	return &k8sv1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nm.Spec.NodeObject,
		},
	}
}

func (nm NotifyMaintenance) IsTerminal() bool {
	_, exists := nm.Annotations[NMTerminalAnnotation]
	return exists
}

func (nmStatus NotifyMaintenanceStatus) IsSLAExpired() bool {
	if nmStatus.SLAExpires.IsZero() {
		return false
	}

	now := metav1.Now()
	return nmStatus.SLAExpires.Before(&now)
}

func (nm NotifyMaintenance) IsInProgress() bool {
	return IsNotifyMaintenanceStatusInProgress(nm.Status.MaintenanceStatus)
}

func init() {
	SchemeBuilder.Register(&NotifyMaintenance{}, &NotifyMaintenanceList{})
}

func (nmStatus NotifyMaintenanceStatus) GetCondition(conditionType string) (MaintenanceCondition, bool) {
	emptyCondition := MaintenanceCondition{
		Status: "",
	}
	if nmStatus.Conditions != nil {
		for _, cond := range nmStatus.Conditions {
			if cond.Type == conditionType && cond.Status != "" {
				return cond, true
			}
		}
	}
	return emptyCondition, false
}

func (nmStatus *NotifyMaintenanceStatus) SetCondition(conditionType string, newStatus string) {
	for k, cond := range nmStatus.Conditions {
		if cond.Type == conditionType {
			nmStatus.Conditions[k].Status = newStatus
			return
		}
	}
}

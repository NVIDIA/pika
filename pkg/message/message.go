package message

import (
	"encoding/json"
	"fmt"

	"github.com/NVIDIA/pika/api/v1alpha1"
)

/*

Message format on the wire for reference

{
    "maintenanceID": "123",
    "zone": "nd-zrsjc6y",
    "objects": [spec.objects],
    "tenant": <name>,
    "event": "MaintenanceScheduled",
    "Message": "Maintenance scheduled on <spec.Objects> in <zone> for <tenant>."
}
*/

type Message struct {
	MaintenanceID               string       `json:"maintenanceID"`
	Objects                     []string     `json:"objects"`
	Event                       MessageEvent `json:"event"`
	Message                     string       `json:"Message,omitempty"`
	Namespace                   string       `json:"namespace"`
	MaintenanceObjectName       string       `json:"maintenanceObjectName"`
	MetadataConfigmap           string       `json:"metadataConfigmap,omitempty"`
	ValidationMetadataOverrides string       `json:"validationMetadataOverrides,omitempty"`
	Owner                       string       `json:"owner,omitempty"`
}

type MessageEvent string

func newMessage(nm *v1alpha1.NotifyMaintenance, event MessageEvent) (*Message, error) {
	m := &Message{
		MaintenanceID:         nm.Spec.MaintenanceID,
		Objects:               []string{nm.Spec.NodeObject},
		Event:                 event,
		Namespace:             nm.Namespace,
		MaintenanceObjectName: nm.Name,
		MetadataConfigmap:     nm.Spec.MetadataConfigmap,
		Owner:                 nm.Spec.Owner,
	}
	msg, err := m.generateMessage()
	if err != nil {
		return nil, err
	}
	m.Message = msg
	return m, nil
}

const (
	MessageEventScheduled             = MessageEvent("Scheduled")
	MessageEventSLAStarted            = MessageEvent("SLAStarted")
	MessageEventSLAExpired            = MessageEvent("SLAExpired")
	MessageEventObjectsDrained        = MessageEvent("ObjectsDrained")
	MessageEventValidating            = MessageEvent("Validating")
	MessageEventMaintenanceIncomplete = MessageEvent("MaintenanceIncomplete")
	MessageEventMaintenanceComplete   = MessageEvent("MaintenanceComplete")
	MessageEventMaintenanceEnded      = MessageEvent("MaintenanceEnded")
	MessageEventMaintenanceCancelled  = MessageEvent("MaintenanceCancelled")
)

// MachineReadable returns a json string from the nm objects and the event that triggered this message
func MachineReadable(nm *v1alpha1.NotifyMaintenance, event MessageEvent) (string, error) {
	m, err := newMessage(nm, event)
	if err != nil {
		return "", err
	}

	mBytes, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(mBytes), nil
}

// HumanReadable returns a human-readable string from the nm objects and the event that triggered this message
func HumanReadable(nm *v1alpha1.NotifyMaintenance, event MessageEvent) string {
	m, err := newMessage(nm, event)
	if err != nil {
		return ""
	}
	return m.Message
}

func (m *Message) generateMessage() (eventString string, err error) {
	switch m.Event {
	case MessageEventScheduled:
		eventString = "scheduled"
	case MessageEventSLAStarted:
		eventString = "started"
	case MessageEventSLAExpired:
		eventString = "SLA expired"
	case MessageEventObjectsDrained:
		eventString = "objects drained"
	case MessageEventValidating:
		eventString = "validating"
	case MessageEventMaintenanceIncomplete:
		eventString = "incomplete"
	case MessageEventMaintenanceComplete:
		eventString = "completed"
	case MessageEventMaintenanceCancelled:
		eventString = "cancelled"
	case MessageEventMaintenanceEnded:
		eventString = "ended"
	default:
		return "", fmt.Errorf("event type %s is not supported", m.Event)
	}
	return fmt.Sprintf("Maintenance %s %s on %s", m.MaintenanceID, eventString, m.Objects), nil
}

package notify

import (
	"fmt"

	ngn2v1alpha1 "github.com/NVIDIA/pika/api/v1alpha1"
	"github.com/NVIDIA/pika/pkg/config"
	"github.com/NVIDIA/pika/pkg/message"
	"github.com/NVIDIA/pika/pkg/metrics"
	snsclient "github.com/NVIDIA/pika/pkg/notify/sns"
	"github.com/NVIDIA/pika/pkg/utils"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/go-logr/logr"
	k8sv1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type snsNotifier struct {
	NoopNotifier

	log    logr.Logger
	client snsclient.Client
}

func NewSNSNotifier(_ config.Config, log logr.Logger) (Notifier, error) {
	c, err := snsclient.NewSNSClient()
	if err != nil {
		return nil, err
	}

	return &snsNotifier{
		log:    log.WithName("sns-notifier"),
		client: c,
	}, nil
}

func (s *snsNotifier) publishAndLog(event message.MessageEvent, nm *ngn2v1alpha1.NotifyMaintenance, node *k8sv1.Node) error {
	if nm == nil {
		return nil
	}
	if nm.Spec.AdditionalMessageChannels.AmazonSNSTopicARN == "" {
		s.log.Info("noop for sns notifier", "event", event, "reason", ".spec.additionalMessageChannels.amazonSNSTopicARN is empty")
		return nil
	}

	msg, err := message.MachineReadable(nm, event)
	if err != nil {
		metrics.NotificationFailCount(s.GetNotifierName())
		return err
	}

	var maintClient string
	if nm.Labels != nil {
		maintClient = nm.Labels[utils.MAINTENANCE_LABEL_PREFIX+utils.MAINTENANCE_CLIENT_LABEL]
	}
	if maintClient == "" {
		maintClient = utils.DefaultMaintenanceClient
	}

	var attributes = make(map[string]*sns.MessageAttributeValue)
	attributes["MaintenanceEvent"] = s.maintenanceEventAttribute(event)
	attributes["MaintenanceClient"] = s.maintenanceClientAttribute(maintClient)

	groupID := utils.Md5Hash(nm.Spec.NodeObject)
	messageID := utils.Md5Hash(fmt.Sprintf("%s/%s/%s/%s/%s", nm.Namespace, nm.Name, nm.Spec.NodeObject, string(event), nm.CreationTimestamp.String()))

	result, err := s.client.Publish(&sns.PublishInput{
		Message:                &msg,
		TopicArn:               &nm.Spec.AdditionalMessageChannels.AmazonSNSTopicARN,
		MessageGroupId:         &groupID,
		MessageDeduplicationId: &messageID,
		MessageAttributes:      attributes,
	})
	if err != nil {
		s.log.Error(err, fmt.Sprintf("error publishing %s event", event))
		metrics.NotificationFailCount(s.GetNotifierName())
		return err
	}

	metrics.NotificationSuccessCount(s.GetNotifierName())

	if result != nil {
		s.log.Info(fmt.Sprintf("published %s event", event), "messageID", result.MessageId, "MessageGroupID", groupID, "MessageDuplicationID", messageID, "sequenceNumber", result.SequenceNumber)
		return nil
	} else {
		s.log.Info("response from sns publish even empty", "MessageGroupID", groupID, "MessageDuplicationID", messageID)
	}
	return nil
}

func (s *snsNotifier) maintenanceClientAttribute(maintenanceClient string) *sns.MessageAttributeValue {
	s.log.Info("MaintenanceClient SNS message attribute added", "MaintenanceClient", maintenanceClient)

	return &sns.MessageAttributeValue{
		DataType:    aws.String("String"),
		StringValue: aws.String(maintenanceClient),
	}
}

func (s *snsNotifier) maintenanceEventAttribute(status message.MessageEvent) *sns.MessageAttributeValue {
	s.log.Info("MaintenanceEvent SNS message attribute added", "MaintenanceEvent", status)

	return &sns.MessageAttributeValue{
		DataType:    aws.String("String"),
		StringValue: aws.String(string(status)),
	}
}

func (s *snsNotifier) NotifyMaintenanceScheduled(node client.Object, nm ngn2v1alpha1.NotifyMaintenance) error {
	return s.publishAndLog(message.MessageEventScheduled, &nm, node.(*k8sv1.Node))
}

func (s *snsNotifier) NotifySLAStart(node client.Object, nm ngn2v1alpha1.NotifyMaintenance) error {
	return s.publishAndLog(message.MessageEventSLAStarted, &nm, node.(*k8sv1.Node))
}

func (s *snsNotifier) NotifySLAExpire(node client.Object, nm ngn2v1alpha1.NotifyMaintenance) error {
	return s.publishAndLog(message.MessageEventSLAExpired, &nm, node.(*k8sv1.Node))
}

func (s *snsNotifier) NotifyNodeDrain(node client.Object, nm ngn2v1alpha1.NotifyMaintenance) error {
	return s.publishAndLog(message.MessageEventObjectsDrained, &nm, node.(*k8sv1.Node))
}

func (s *snsNotifier) NotifyValidating(node client.Object, nm ngn2v1alpha1.NotifyMaintenance) error {
	return s.publishAndLog(message.MessageEventValidating, &nm, node.(*k8sv1.Node))
}

func (s *snsNotifier) NotifyMaintenanceIncomplete(node client.Object, nm ngn2v1alpha1.NotifyMaintenance) error {
	return s.publishAndLog(message.MessageEventMaintenanceIncomplete, &nm, node.(*k8sv1.Node))
}

func (s *snsNotifier) NotifyMaintenanceCompleted(node client.Object, nm ngn2v1alpha1.NotifyMaintenance) error {
	return s.publishAndLog(message.MessageEventMaintenanceComplete, &nm, node.(*k8sv1.Node))
}

func (s *snsNotifier) NotifyMaintenanceCancellation(node client.Object, nm ngn2v1alpha1.NotifyMaintenance) error {
	return s.publishAndLog(message.MessageEventMaintenanceCancelled, &nm, node.(*k8sv1.Node))
}

func (s *snsNotifier) GetNotifierName() string {
	return "SNSNotifier"
}

func (s *snsNotifier) RotateCert() error {
	c, err := snsclient.NewSNSClient()
	if err != nil {
		return err
	}
	s.client = c
	return nil
}

package notify

import (
	"fmt"
	"time"

	ngn2v1alpha1 "github.com/NVIDIA/pika/api/v1alpha1"
	"github.com/NVIDIA/pika/pkg/message"
	snsclient "github.com/NVIDIA/pika/pkg/notify/sns"
	"github.com/NVIDIA/pika/pkg/utils"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = Describe("SNS Mocks", func() {
	var (
		mockCtrl       *gomock.Controller
		snsClient      *snsclient.MockClient
		sNoti          *snsNotifier
		snsTopicARN    string
		node           k8sv1.Node
		testNodeName   string
		creationTime   metav1.Time
		nm             ngn2v1alpha1.NotifyMaintenance
		testAttributes map[string]*sns.MessageAttributeValue
		input          func(event message.MessageEvent, maintenance ngn2v1alpha1.NotifyMaintenance, node *k8sv1.Node) *sns.PublishInput
	)

	type notiFunc func(client.Object, ngn2v1alpha1.NotifyMaintenance) error

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		snsClient = snsclient.NewMockClient(mockCtrl)
		sNoti = &snsNotifier{
			log:    zap.New().WithName("sns-mock-logger"),
			client: snsClient,
		}
		snsTopicARN = "test-arn.fifo"
		testNodeName = "test-node"
		creationTime = metav1.NewTime(time.Now())
		nm = ngn2v1alpha1.NotifyMaintenance{
			ObjectMeta: metav1.ObjectMeta{CreationTimestamp: creationTime},
			Spec: ngn2v1alpha1.NotifyMaintenanceSpec{
				NodeObject: testNodeName,
				AdditionalMessageChannels: ngn2v1alpha1.MessageChannels{
					AmazonSNSTopicARN: snsTopicARN,
				},
				MetadataConfigmap: "kubernetes-upgrade",
			},
		}
		node = k8sv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{},
				Name:   testNodeName,
			},
		}
		nm.Annotations = map[string]string{}
		nm.Annotations[utils.MAINTENANCE_METADATA_OVERRIDES_KEY] = "{      test: sanity,    }"

		testAttributes = make(map[string]*sns.MessageAttributeValue)

		input = func(event message.MessageEvent, nm ngn2v1alpha1.NotifyMaintenance, node *k8sv1.Node) *sns.PublishInput {
			msg, _ := message.MachineReadable(&nm, event)
			groupID := utils.Md5Hash(nm.Spec.NodeObject)
			messageID := utils.Md5Hash(fmt.Sprintf("%s/%s/%s/%s/%s", nm.Namespace, nm.Name, nm.Spec.NodeObject, string(event), nm.CreationTimestamp.String()))

			testAttributes["MaintenanceEvent"] = &sns.MessageAttributeValue{
				DataType:    aws.String("String"),
				StringValue: aws.String(string(event)),
			}

			testAttributes["MaintenanceClient"] = &sns.MessageAttributeValue{
				DataType:    aws.String("String"),
				StringValue: aws.String(utils.DefaultMaintenanceClient),
			}

			return &sns.PublishInput{
				Message:                &msg,
				MessageDeduplicationId: &messageID,
				MessageGroupId:         &groupID,
				TopicArn:               &snsTopicARN,
				MessageAttributes:      testAttributes,
			}
		}
	})

	DescribeTable("should send sns notification on maintenance events", func(notiFunc func() (notiFunc, message.MessageEvent)) {
		eventFunc, event := notiFunc()
		publishInput := input(event, nm, &node)
		snsClient.EXPECT().Publish(publishInput).Times(1)
		Expect(eventFunc(&node, nm)).To(Succeed())
	},
		Entry("NotifyMaintenanceScheduled", func() (notiFunc, message.MessageEvent) {
			return sNoti.NotifyMaintenanceScheduled, message.MessageEventScheduled
		}),
		Entry("NotifySLAStart", func() (notiFunc, message.MessageEvent) {
			return sNoti.NotifySLAStart, message.MessageEventSLAStarted
		}),
		Entry("NotifySLAExpire", func() (notiFunc, message.MessageEvent) {
			return sNoti.NotifySLAExpire, message.MessageEventSLAExpired
		}),
		Entry("NotifyNodeDrain", func() (notiFunc, message.MessageEvent) {
			return sNoti.NotifyNodeDrain, message.MessageEventObjectsDrained
		}),
		Entry("NotifyValidating", func() (notiFunc, message.MessageEvent) {
			return sNoti.NotifyValidating, message.MessageEventValidating
		}),
		Entry("NotifyMaintenanceIncomplete", func() (notiFunc, message.MessageEvent) {
			return sNoti.NotifyMaintenanceIncomplete, message.MessageEventMaintenanceIncomplete
		}),
		Entry("NotifyMaintenanceCompleted", func() (notiFunc, message.MessageEvent) {
			return sNoti.NotifyMaintenanceCompleted, message.MessageEventMaintenanceComplete
		}),
		Entry("NotifyMaintenanceCancellation", func() (notiFunc, message.MessageEvent) {
			return sNoti.NotifyMaintenanceCancellation, message.MessageEventMaintenanceCancelled
		}),
	)

	DescribeTable("should be a no-op and print info log on maintenance events", func(notiFunc func() (notiFunc, message.MessageEvent)) {
		eventFunc, _ := notiFunc()
		nm.Spec.AdditionalMessageChannels.AmazonSNSTopicARN = ""
		snsClient.EXPECT().Publish(gomock.Any()).Times(0)
		Expect(eventFunc(&node, nm)).To(Succeed())
	},
		Entry("NotifyMaintenanceScheduled", func() (notiFunc, message.MessageEvent) {
			return sNoti.NotifyMaintenanceScheduled, message.MessageEventScheduled
		}),
		Entry("NotifySLAStart", func() (notiFunc, message.MessageEvent) {
			return sNoti.NotifySLAStart, message.MessageEventSLAStarted
		}),
		Entry("NotifySLAExpire", func() (notiFunc, message.MessageEvent) {
			return sNoti.NotifySLAExpire, message.MessageEventSLAExpired
		}),
		Entry("NotifyNodeDrain", func() (notiFunc, message.MessageEvent) {
			return sNoti.NotifyNodeDrain, message.MessageEventObjectsDrained
		}),
		Entry("NotifyValidating", func() (notiFunc, message.MessageEvent) {
			return sNoti.NotifyValidating, message.MessageEventValidating
		}),
		Entry("NotifyMaintenanceIncomplete", func() (notiFunc, message.MessageEvent) {
			return sNoti.NotifyMaintenanceIncomplete, message.MessageEventMaintenanceIncomplete
		}),
		Entry("NotifyMaintenanceCompleted", func() (notiFunc, message.MessageEvent) {
			return sNoti.NotifyMaintenanceCompleted, message.MessageEventMaintenanceComplete
		}),
		Entry("NotifyMaintenanceCancellation", func() (notiFunc, message.MessageEvent) {
			return sNoti.NotifyMaintenanceCancellation, message.MessageEventMaintenanceCancelled
		}),
	)
})

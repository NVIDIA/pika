package sns

import (
	"errors"
	"os"
	"path"

	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/pointer"
)

var _ = Describe("SNSClient", func() {
	Context("create a SNS client", func() {
		var (
			err    error
			tmpDir string
		)
		BeforeEach(func() {
			tmpDir, err = os.MkdirTemp("", "")
			Expect(err).To(Succeed())

		})

		AfterEach(func() {
			Expect(os.RemoveAll(tmpDir)).To(Succeed())
		})

		It("should return error if aws config and credentials are not defined", func() {
			awsCredPath = path.Join(tmpDir, "fake-cred")

			_, err := NewSNSClient()
			Expect(err).NotTo(Succeed())
		})
	})
	Context("lookup a topic from SNS", func() {
		var (
			mockCtrl              *gomock.Controller
			mockSNSInternalClient *MockclientInterface
			lookupTopic           string
			snsClient             client
		)
		BeforeEach(func() {
			mockCtrl = gomock.NewController(GinkgoT())
			mockSNSInternalClient = NewMockclientInterface(mockCtrl)
			lookupTopic = "test-lookup-topic-name"
			snsClient = client{mockSNSInternalClient}
		})

		It("should return error if topic is not found", func() {
			mockSNSInternalClient.EXPECT().ListTopics(nil).Return(&sns.ListTopicsOutput{}, nil)
			_, err := snsClient.Lookup(lookupTopic)
			Expect(err.Error()).To(ContainSubstring("NotFound"))
		})
		It("should return error if ListTopics errors", func() {
			mockSNSInternalClient.EXPECT().ListTopics(nil).Return(&sns.ListTopicsOutput{}, errors.New("unreachable"))
			_, err := snsClient.Lookup(lookupTopic)
			Expect(err.Error()).To(ContainSubstring("unreachable"))
		})
		It("should return 2nd element from list", func() {
			t := sns.Topic{
				TopicArn: pointer.String("foo:bar:" + lookupTopic),
			}
			mockSNSInternalClient.EXPECT().ListTopics(nil).Return(&sns.ListTopicsOutput{Topics: []*sns.Topic{
				{
					TopicArn: pointer.String("foo:bar:test-topic"),
				},
				&t,
				{
					TopicArn: pointer.String("foo:bar:test-topic-1"),
				},
			}}, nil)
			topic, err := snsClient.Lookup(lookupTopic)
			Expect(err).ToNot(HaveOccurred())
			Expect(topic.TopicArn).To(BeIdenticalTo(t.TopicArn))
		})
	})
})

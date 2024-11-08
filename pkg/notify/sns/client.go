package sns

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sns"
)

type Client interface {
	Publish(input *sns.PublishInput) (*sns.PublishOutput, error)
	Lookup(topic string) (*sns.Topic, error)
	CreateTopic(input *sns.CreateTopicInput) (*sns.CreateTopicOutput, error)
}

type client struct {
	clientInterface
}

type clientInterface interface {
	Publish(input *sns.PublishInput) (*sns.PublishOutput, error)
	CreateTopic(input *sns.CreateTopicInput) (*sns.CreateTopicOutput, error)
	ListTopics(input *sns.ListTopicsInput) (*sns.ListTopicsOutput, error)
	GetTopicAttributes(input *sns.GetTopicAttributesInput) (*sns.GetTopicAttributesOutput, error)
}

var (
	homeDir            = os.Getenv("HOME")
	awsCredPath        = path.Join(homeDir, ".aws", "credentials")
	awsAccessKeyID     = os.Getenv("AWS_ACCESS_KEY_ID")
	awsSecretAccessKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
)

func NewSNSClient() (Client, error) {
	var sess *session.Session

	if awsAccessKeyID != "" && awsSecretAccessKey != "" {
		fmt.Println("Using env vars to create AWS session")
		sess = session.Must(session.NewSession())
	} else if _, err := os.Stat(awsCredPath); err == nil {
		fmt.Println("Using config file to create AWS session")
		sess = session.Must(session.NewSessionWithOptions(session.Options{
			SharedConfigState: session.SharedConfigEnable,
		}))
	} else {
		return nil, fmt.Errorf("AWS env vars 'AWS_ACCESS_KEY_ID' & 'AWS_SECRET_ACCESS_KEY' and AWS creds file %s do not exist", awsCredPath)
	}

	return client{sns.New(sess)}, nil
}

func (l client) Lookup(topicName string) (*sns.Topic, error) {
	topics, err := l.ListTopics(nil)
	if err != nil {
		return nil, err
	}
	for _, topic := range topics.Topics {
		topic := topic
		arnSplit := strings.Split(*topic.TopicArn, ":")
		if arnSplit[len(arnSplit)-1] == topicName {
			return topic, nil
		}
	}
	return nil, errors.New("NotFound")
}

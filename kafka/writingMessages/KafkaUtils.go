package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/google/uuid"
)

// TopicName holds the name of the topic, for kafka registry this is the "subject"
// convention naming should be like so: topicName-key or topicName-value
// const TopicName string = "Person-value"
const TopicName string = "person-new"

// PropsFile holds the filename with config
const PropsFile string = "ccloud.properties"

// CreateTopic is a utility function that
// creates the topic if it doesn't exist.
func CreateTopic(props map[string]string) {

	adminClient, err := kafka.NewAdminClient(&kafka.ConfigMap{
		"bootstrap.servers":       props["bootstrap.servers"],
		"broker.version.fallback": "0.10.0.0",
		"api.version.fallback.ms": 0,
		"sasl.mechanisms":         "PLAIN",
		"security.protocol":       "SASL_SSL",
		"sasl.username":           props["sasl.username"],
		"sasl.password":           props["sasl.password"]})

	if err != nil {
		fmt.Printf("Failed to create Admin client: %s\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	maxDuration, err := time.ParseDuration("60s")
	if err != nil {
		panic("time.ParseDuration(60s)")
	}

	results, err := adminClient.CreateTopics(ctx,
		[]kafka.TopicSpecification{{
			Topic:             TopicName,
			NumPartitions:     4,
			ReplicationFactor: 3}},
		kafka.SetAdminOperationTimeout(maxDuration))

	if err != nil {
		fmt.Printf("Problem during the topic creation: %v\n", err)
		os.Exit(1)
	}

	for _, result := range results {
		if result.Error.Code() != kafka.ErrNoError &&
			result.Error.Code() != kafka.ErrTopicAlreadyExists {
			fmt.Printf("Topic creation failed for %s: %v",
				result.Topic, result.Error.String())
			os.Exit(1)
		}
	}

	adminClient.Close()
}

// LoadProperties read the properties file
// containing the Confluent Cloud config
// so the apps can connect to the service.
func LoadProperties() map[string]string {
	props := make(map[string]string)
	file, err := os.Open(PropsFile)
	if err != nil {
		panic(fmt.Sprintf("Failed to load the '%s' file", PropsFile))
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) > 0 {
			if !strings.HasPrefix(line, "//") &&
				!strings.HasPrefix(line, "#") {
				parts := strings.Split(line, "=")
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				props[key] = value
			}
		}
	}
	return props
}

// NewUUID is a simpler way
// to retrieve a new UUID.
func NewUUID() string {
	newUUID, _ := uuid.NewUUID()
	return newUUID.String()
}

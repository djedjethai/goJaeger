package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	pb "getting-started-with-ccloud-golang/api/v1/proto"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/golang/protobuf/proto"
	"github.com/riferrei/srclient"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	producerMode string = "producer"
	consumerMode string = "consumer"
	schemaFile   string = "./api/v1/proto/Person.proto"
	// schemaFile   string = "./api/v1/proto/SensorReading.proto"
	// messageFile  string = "./api/v1/proto/Message.proto"
	partitionCount int32 = 3
	service              = "trace-kfk"
	environment          = "development"
	id                   = 1
	jaegerEndpoint       = "http://localhost:14268/api/traces"
)

type Message struct {
	Text string
}

// const topicMessage = "message"

func tracerProvider(url string) (*tracesdk.TracerProvider, error) {
	// Create the Jaeger exporter
	exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(url)))
	if err != nil {
		return nil, err
	}
	tp := tracesdk.NewTracerProvider(
		// Always be sure to batch in production.
		tracesdk.WithBatcher(exp),
		// Record information about this application in a Resource.
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(service),
			attribute.String("environment", environment),
			attribute.Int64("ID", id),
		)),
		// tracesdk.WithSampler(tracesdk.AlwaysSample()), does not do anything...
	)
	return tp, nil
}

func testJaegger() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tr := otel.Tracer("from-testJaegger")
	// fmt.Println("The context: ", ctx)

	_, sp := tr.Start(ctx, "produce")
	// defer sp.End()

	fmt.Println("In the func testJaegger .............. ", sp)

	// ctxSp := trace.ContextWithSpan(ctx, sp)

	sp.End()
}

func main() {

	// ============== here OK set jaeger ===========
	tp, err := tracerProvider("http://127.0.0.1:14268/api/traces")
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Register our TracerProvider as the global so any imported
	// instrumentation in the future will default to using it.
	otel.SetTracerProvider(tp)

	// Cleanly shutdown and flush telemetry when the application exits.
	defer func(ctx context.Context) {
		// Do not make the application hang when it is shutdown.
		ctx, cancel := context.WithTimeout(ctx, time.Second*5)
		defer cancel()
		if err := tp.Shutdown(ctx); err != nil {
			log.Fatal(err)
		}
	}(ctx)

	// testJaegger()
	// ==============================

	clientMode := os.Args[1]
	// props := LoadProperties()
	topic := "partitioning-strategy"

	if strings.Compare(clientMode, producerMode) == 0 {
		// producer(props, topic)
		// NOTE if producer jaeger get over sampling then it does not record the span
		producer(topic)
	} else if strings.Compare(clientMode, consumerMode) == 0 {
		consumer(topic)
	} else {
		fmt.Printf("Invalid option. Valid options are '%s' and '%s'.",
			producerMode, consumerMode)
	}
}

// implement a custom partitioner function that returns a specific partition for the "banana" key
// and uses the default hash-based partitioning for all other keys.
func partitioner(key []byte, partitionCount int32) int32 {

	// Use the default hash-based partitioning for all keys except "banana"
	fmt.Println("see the key: ", string(key))
	if string(key) != "banane" {
		h := fnv.New32a()
		h.Write(key)
		partition := int32(h.Sum32()) % partitionCount
		// some partition number will be negatif, set them as positif
		if partition < 0 {
			partition += partitionCount
		}
		// partition may be == 0 which is the one reserved for "banane"
		if partition == 0 {
			partition = 1
		}
		fmt.Println("see key: ", string(key), " to partition: ", partition)
		return partition
	}

	// Assign the "banana" key to partition 0
	return 0
}

type KafkaHeadersCarrier map[string]string

func (c KafkaHeadersCarrier) Get(key string) string {
	return c[key]
}

func (c KafkaHeadersCarrier) Set(key string, value string) {
	c[key] = value
}

func (c KafkaHeadersCarrier) Keys() []string {
	keys := make([]string, len(c))
	i := 0
	for key := range c {
		keys[i] = key
		i++
	}
	return keys
}

func injectOpenTelemetrySpanToKafka(span trace.Span, message *kafka.Message) error {
	propagator := propagation.TraceContext{}

	ctx := context.Background()
	ctx = trace.ContextWithSpan(ctx, span)

	carrier := KafkaHeadersCarrier{}

	propagator.Inject(ctx, carrier)

	for key, value := range carrier {
		message.Headers = append(message.Headers, kafka.Header{
			Key:   key,
			Value: []byte(value),
		})
	}

	return nil
}

/**************************************************/
/******************** Producer ********************/
/**************************************************/

// func producer(props map[string]string, topic string) {
func producer(topic string) {

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// testJaegger()

	// log.Println("See the context with span: ", ctxSp)

	config := &kafka.ConfigMap{
		"bootstrap.servers": "localhost:29092",
		"client.id":         "my-producer",
		"acks":              "all",
		"retries":           5,
		// even the key is the same the messages will be distributed randomly among partitions
		// "partitioner":         "random",
		"go.delivery.reports": true,
	}

	// Create producer instance
	producer, err := kafka.NewProducer(config)

	if err != nil {
		log.Panic("err connecting to kafka: ", err)
	}

	defer producer.Close()

	schemaRegistryClient := srclient.CreateSchemaRegistryClient("http://127.0.0.1:8081")

	schema, err := schemaRegistryClient.GetLatestSchema(topic)

	fmt.Println("The schema: ", schema)

	schema = nil
	if schema == nil {
		// var b bool = false
		schemaBytes, _ := ioutil.ReadFile(schemaFile)

		// Test mytest = 6;
		schema, err = schemaRegistryClient.CreateSchema(topic, string(schemaBytes), "PROTOBUF")
		if err != nil {
			panic(fmt.Sprintf("Error creating the schema %s", err))
		}
		fmt.Println("look like schema has been created...")
	}

	// bananaKey := "banana"
	otherKeys := []string{"apple", "banane", "pear", "banane", "carotte", "banane", "citron", "banane"}

	fmt.Println("See otherKeys: ", otherKeys)

	// Listen for delivery reports on the Events channel
	go func() {
		for ev := range producer.Events() {
			switch e := ev.(type) {
			case *kafka.Message:
				if e.TopicPartition.Error != nil {
					fmt.Printf("Delivery failed: %v\n", e.TopicPartition)
				} else {
					fmt.Printf("Delivered message to %v\n", e.TopicPartition)
					fmt.Printf("Delivered message to topic %s, partition [%d] at offset %v\n",
						*e.TopicPartition.Topic,
						e.TopicPartition.Partition,
						e.TopicPartition.Offset)
				}
			}
		}
	}()

	// // ==== Jaeger staff ====

	tr := otel.Tracer("from-producer")
	// fmt.Println("The context: ", ctx)

	// for {
	// TODO use with partitioner function
	var message *kafka.Message

	fmt.Println("The message: ", message)

	for i, k := range otherKeys {

		ctx, cancel := context.WithCancel(context.Background())
		// defer cancel()

		fmt.Println("Key: ", k)

		_, sp := tr.Start(ctx, fmt.Sprintf("produce-%v", i))
		// defer sp.End()

		msg := &pb.Message{Text: "this a is a good test"}

		recordValue := []byte{}
		// recordValue := []byte("that is a test de fou")

		recordValue = append(recordValue, byte(0))
		schemaIDBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(schemaIDBytes, uint32(schema.ID()))
		recordValue = append(recordValue, schemaIDBytes...)
		messageIndexBytes := []byte{byte(2), byte(0)}
		recordValue = append(recordValue, messageIndexBytes...)

		valueBytes, _ := proto.Marshal(msg)
		recordValue = append(recordValue, valueBytes...)

		message = &kafka.Message{
			TopicPartition: kafka.TopicPartition{
				Topic:     &topic,
				Partition: kafka.PartitionAny,
			},
			Key:   []byte(k),
			Value: recordValue,
		}

		// // Use the custom partitioner function to determine the partition to send the message to
		message.TopicPartition.Partition = partitioner(message.Key, int32(partitionCount))

		// inject span into message header
		injectOpenTelemetrySpanToKafka(sp, message)

		err = producer.Produce(message, nil)
		if err != nil {
			fmt.Printf("Failed to produce message: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("sent....")
		cancel()
		sp.End()
		time.Sleep(1000 * time.Millisecond)
	}
	// }
}

/**************************************************/
/******************** Consumer ********************/
/**************************************************/

func consumeMessage(message kafka.Message) {
	propagator := propagation.TraceContext{}

	carrier := KafkaHeadersCarrier{}
	for _, header := range message.Headers {
		carrier.Set(header.Key, string(header.Value))
	}

	ctx := context.Background()
	ctx = propagator.Extract(ctx, carrier)

	tracer := otel.Tracer("consumer-tracer")
	_, span := tracer.Start(ctx, "consume")

	// Use the span for tracing within the consumer logic

	span.End()
}

// func consumer(props map[string]string, topic string) {
func consumer(topic string) {

	// setTopic(topic)

	schemaRegistryClient := srclient.CreateSchemaRegistryClient("http://127.0.0.1:8081")

	schemaRegistryClient.CodecCreationEnabled(false)

	c, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers": "127.0.0.1:29092",
		"group.id":          "myGroup",
		"auto.offset.reset": "earliest",
	})
	defer c.Close()

	if err != nil {
		panic(err)
	}

	// Handle unexpected shutdown
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	// Start a goroutine to handle the signals
	go func(*kafka.Consumer) {
		// Block until a signal is received
		sig := <-signals
		fmt.Println("Received signal:", sig)

		// Handle the cleanup tasks before shut down
		// like close connection to the db

		// Exit the program gracefully
		os.Exit(0)
	}(c)

	// c.SubscribeTopics([]string{"myTopic", "^aRegex.*[Tt]opic"}, nil)

	fmt.Println("bf the subscribe topic: ", topic)

	c.SubscribeTopics([]string{topic}, nil)

	run := true

	for run {
		// record, err := c.ReadMessage(-1)
		// if err == nil {
		// 	// sensorReading := &pb.SensorReading{}
		// 	msg := &pb.Message{}
		// 	// err = proto.Unmarshal(record.Value[7:], sensorReading)
		// 	err = proto.Unmarshal(record.Value[7:], msg)
		// 	if err != nil {
		// 		panic(fmt.Sprintf("Error deserializing the record: %s", err))
		// 	}
		// 	fmt.Printf("Message on %s: %s\n", record.TopicPartition, string(record.Value))
		// 	// fmt.Println("seeeee: ", msg)
		// } else if err.(kafka.Error).IsFatal() {
		// 	// fmt.Println(err)
		// 	log.Printf("Consumer error: %v \n", err)
		// }

		select {
		case <-signals:
			// Termination signal received. exit the loop.
			break

		default:

			// fmt.Printf("Received message !!!!!!!!! : %s\n", string(e.Value)
			ev := c.Poll(1000)

			switch e := ev.(type) {
			case *kafka.Message:

				msg := &pb.Message{}
				// err = proto.Unmarshal(record.Value[7:], sensorReading)
				err = proto.Unmarshal(e.Value[7:], msg)
				if err != nil {
					panic(fmt.Sprintf("Error deserializing the record: %s", err))
				}
				fmt.Printf("Message on %s: %s\n", e.TopicPartition, string(e.Value))
				consumeMessage(*e)
			case kafka.Error:
				fmt.Printf("Error: %v\n", e)
				// Handle the error here

			default:
				// Ignore other event types
			}

		}
	}
}

func setTopic(topic string) {
	fmt.Println("in the set topic")

	// admin create a topic
	a, err := kafka.NewAdminClient(&kafka.ConfigMap{"bootstrap.servers": "127.0.0.1:29092"})
	if err != nil {
		fmt.Printf("Failed to create Admin client: %s\n", err)
		os.Exit(1)
	}

	// Contexts are used to abort or limit the amount of time
	// the Admin call blocks waiting for a result.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create topics on cluster.
	// Set Admin options to wait for the operation to finish (or at most 60s)
	maxDur, err := time.ParseDuration("60s")
	if err != nil {
		panic("ParseDuration(60s)")
	}
	results, err := a.CreateTopics(
		ctx,
		// Multiple topics can be created simultaneously
		// by providing more TopicSpecification structs here.
		[]kafka.TopicSpecification{{
			Topic:             topic,
			NumPartitions:     4,
			ReplicationFactor: 1,
			Config:            map[string]string{"replication.factor": "1"},
		}},
		// Admin options
		kafka.SetAdminOperationTimeout(maxDur))
	if err != nil {
		fmt.Printf("Failed to create topic: %v\n", err)
		os.Exit(1)
	} else {
		fmt.Println("topic created: ", topic)
	}

	// Print results
	for _, result := range results {
		fmt.Printf("%s\n", result)
	}

	a.Close()
}

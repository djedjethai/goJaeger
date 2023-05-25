package main

// clean the cache if needed
// go clean -cache -modcache -i -r

// to import jaeger necessary libraries
// git clone the opentelemetry project into this project
// https://github.com/open-telemetry/opentelemetry-go

// and set the imports like so
// https://github.com/open-telemetry/opentelemetry-go/blob/main/example/jaeger/go.mod

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	pb "communication/api"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/wrapperspb"

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
	service        = "trace-demo"
	environment    = "production"
	id             = 1
	grpcPort       = "50001"
	jaegerEndpoint = "http://jaeger:14268/api/traces"
)

type Order struct {
	ID          string
	Items       []string
	Description string
	Price       float32
	Destination string
}

var orderMap = make(map[string]pb.Order)

// type Tracer interface {
// 	Start(ctx context.Context, spanName string, opts ...trace.SpanOption) (context.Context, trace.Span)
// }

// tracerProvider returns an OpenTelemetry TracerProvider configured to use
// the Jaeger exporter that will send spans to the provided url. The returned
// TracerProvider will also use a Resource configured with all the information
// about the application.
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
	)
	return tp, nil
}

func main() {

	flg := os.Args[1]

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	traceDialOption := grpc.WithUnaryInterceptor(traceInterceptor)

	conn, err := grpc.DialContext(
		ctx,
		fmt.Sprintf("127.0.0.1:%v", grpcPort),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		traceDialOption,
	)
	if err != nil {
		log.Fatalln("Err dialing: ", err)
	}

	client := pb.NewOrderManagementClient(conn)

	// set jaeger
	tp, err := tracerProvider("http://localhost:14268/api/traces")
	if err != nil {
		log.Fatal(err)
	}

	// Register our TracerProvider as the global so any imported
	// instrumentation in the future will default to using it.
	// means that anywhere in the service I can do tr := otel.Tracer("myNewTracer")
	otel.SetTracerProvider(tp)

	// Cleanly shutdown and flush telemetry when the application exits.
	defer func(ctx context.Context) {
		// Do not make the application hang when it is shutdown.
		ctx, cancel = context.WithTimeout(ctx, time.Second*5)
		defer cancel()
		if err := tp.Shutdown(ctx); err != nil {
			log.Fatal(err)
		}
	}(ctx)

	tr := tp.Tracer("component-main")

	_, span := tr.Start(ctx, "foo")
	defer span.End()

	fmt.Println("see the ctx: ", ctx)

	ctxSp := trace.ContextWithSpan(ctx, span)

	// fmt.Println("see the ctxSp: ", ctxSp)

	// span is here OK

	if strings.Compare(flg, "order") == 0 {

		res, err := client.GetOrder(ctxSp, &wrapperspb.StringValue{Value: "1"})
		if err != nil {
			log.Fatalln("Err from client: ", err)
		}

		ord := Order{
			ID:          res.Id,
			Items:       res.Items,
			Description: res.Description,
			Price:       res.Price,
			Destination: res.Destination,
		}

		log.Println("the ord: ", ord)

	} else if strings.Compare(flg, "stream") == 0 {
		fmt.Println("let start....")
		res, err := client.SearchOrders(ctx, &wrapperspb.StringValue{Value: "0"})
		if err != nil {
			log.Fatalln("Err from stream client: ", err)
		}

		for {
			orders, err := res.Recv()
			if err == io.EOF {
				break
			}
			log.Println("See the result: ", orders)
		}
		// client send a stream to server
	} else if strings.Compare(flg, "add") == 0 {
		orderMap["0"] = pb.Order{
			Id:          "aaa",
			Items:       []string{"firyyyyy", "secondyyyy"},
			Description: "YUI",
			Price:       233456.4,
			Destination: "Cambodge",
		}

		orderMap["1"] = pb.Order{
			Id:          "hjk",
			Items:       []string{"thYYY", "forthYYY"},
			Description: "from where",
			Price:       9,
			Destination: "Netherland",
		}

		updateStream, err := client.UpdateOrders(ctx)
		if err != nil {
			log.Fatalln("Err from stream client: ", err)
		}

		for _, v := range orderMap {
			log.Println("see the data sent: ", v)

			if err := updateStream.Send(&v); err != nil {
				log.Println("Err client stream sending datas: ", err)
			}
		}

		updateRes, err := updateStream.CloseAndRecv()
		if err != nil {
			log.Fatalln("Err close stream client: ", err)
		}

		log.Printf("Update Orders Res : %s", updateRes)

	} else if strings.Compare(flg, "concat") == 0 {
		streamProcOrder, err := client.ProcessOrders(ctx)
		if err != nil {
			log.Fatalln("Err from stream client: ", err)
		}

		ids := []string{"0", "1", "0", "0", "1", "1", "0", "1"}

		for _, v := range ids {
			if err := streamProcOrder.Send(&wrapperspb.StringValue{Value: v}); err != nil {
				log.Println("Err client stream sending ids: ", err)
			}
		}

		channel := make(chan struct{})
		go asyncBidirectionalRPC(streamProcOrder, channel)

		time.Sleep(time.Millisecond * 3000)

		// time.Sleep(time.Millisecond * 1000)

		ids = []string{"0", "1", "0", "0"}
		for _, v := range ids {
			if err := streamProcOrder.Send(&wrapperspb.StringValue{Value: v}); err != nil {
				log.Println("Err client stream sending ids: ", err)
			}
		}

		<-channel

	} else {
		log.Println("Invalid flag.....")
		// just for ex how to set another func within the same project
		// bar(ctx)
	}
}

// // just for ex how to set another func within the same project
// func bar(ctx context.Context) {
// 	// Use the global TracerProvider.
// 	tr := otel.Tracer("component-bar")
// 	_, span := tr.Start(ctx, "bar")
// 	span.SetAttributes(attribute.Key("testset").String("value"))
// 	defer span.End()
//
// 	// Do bar...
// }

type contextKey string

const spanContextKey = contextKey("span")

func traceInterceptor(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	log.Println("See the context in grpc: ", ctx)

	tracer := otel.Tracer(method)
	_, span := tracer.Start(ctx, method)
	defer span.End()

	// You can add attributes to the span if needed
	span.SetAttributes(attribute.String("grpc.method", method))

	propagator := propagation.TraceContext{}
	// NOTE that works
	// headers := metadata.New(map[string]string{})
	// propagator.Inject(ctx, propagation.HeaderCarrier(headers))

	// same goMicro
	md := metadata.MD{}
	propagator.Inject(ctx, metadataCarrier(md))

	// Invoke the gRPC method
	ctx = metadata.NewOutgoingContext(ctx, md)

	log.Println("See NewOutgoingContext: ", ctx)

	err := invoker(ctx, method, req, reply, cc, opts...)

	return err
}

// Define a custom carrier type that implements the TextMapCarrier interface
type metadataCarrier metadata.MD

func (c metadataCarrier) Get(key string) string {
	if v := metadata.MD(c).Get(key); len(v) > 0 {
		return v[0]
	}
	return ""
}

func (c metadataCarrier) Set(key string, value string) {
	metadata.MD(c).Set(key, value)
}

func (c metadataCarrier) Keys() []string {
	var keys []string
	for key := range metadata.MD(c) {
		keys = append(keys, key)
	}
	return keys
}

func asyncBidirectionalRPC(
	streamProcOrder pb.OrderManagement_ProcessOrdersClient,
	c chan struct{}) {
	for {
		combinedShipment, err := streamProcOrder.Recv()
		if err == io.EOF {
			log.Println("Combined shipment err: ", err)
			break
		}
		// TODO pass the datas throught the channel
		log.Println("received datas: ", combinedShipment)
	}

	<-c
}

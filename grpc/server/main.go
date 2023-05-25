package main

import (
	"fmt"
	"io"
	"log"
	"net"
	// "reflect"
	"strconv"
	"time"

	// "strings"

	pb "communication/api"
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	// "go.opentelemetry.io/otel/trace"
)

const (
	grpcPort       = 50001
	service        = "trace-server"
	environment    = "production"
	id             = 1
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

// var tr *trace.tracer

func main() {
	ctx := context.Background()

	// // set jaeger
	tp, err := tracerProvider("http://localhost:14268/api/traces")
	if err != nil {
		log.Fatal(err)
	}

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

	orderMap["0"] = pb.Order{
		Id:          "123",
		Items:       []string{"first", "second"},
		Description: "decribeeee",
		Price:       23,
		Destination: "Thailand",
	}

	orderMap["1"] = pb.Order{
		Id:          "456",
		Items:       []string{"third", "forth"},
		Description: "decribeeee again",
		Price:       23,
		Destination: "France",
	}

	fmt.Println("vim-go")
	grpcListen(tp)
}

// func grpcListen(tp *tracesdk.TracerProvider) {
func grpcListen(tp *tracesdk.TracerProvider) {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%v", grpcPort))
	if err != nil {
		log.Fatal("err creating the listener: ", err)
	}

	opt := []grpc.ServerOption{}

	serv, err := NewGrpcServer(opt...)
	if err != nil {
		log.Fatal("err creating the server: ", err)
	}

	log.Println("Server is listening")
	err = serv.Serve(lis)
	if err != nil {
		log.Println("err server listen: ", err)
	}

}

type Server struct {
	pb.UnimplementedOrderManagementServer
	// tp *tracesdk.TracerProvider
}

// func NewGrpcServer(tp *tracesdk.TracerProvider, opt ...grpc.ServerOption) (*grpc.Server, error) {
func NewGrpcServer(opt ...grpc.ServerOption) (*grpc.Server, error) {
	gsrv := grpc.NewServer(
		grpc.UnaryInterceptor(traceInterceptor),
	)

	// srv := &Server{tp: tp}
	srv := &Server{}

	pb.RegisterOrderManagementServer(gsrv, srv)

	return gsrv, nil
}

type contextKey string

const spanContextKey = contextKey("span")

func traceInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	// from client to server

	method := info.FullMethod
	log.Println("Received gRPC request for method:", method)

	// Extract the span context from the gRPC metadata
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		propagator := propagation.TraceContext{}
		ctx = propagator.Extract(ctx, metadataCarrier(md))
		// ctx = propagator.Extract(ctx, propagation.NewCarrier(md))
		log.Println("see the ctx from carrier: ", ctx)

	}

	// Call the gRPC handler with the modified context
	resp, err = handler(ctx, req)

	// response to the client

	// initiate a trace but trace nothing...
	// tracer := otel.Tracer("back to client")
	// ctx, span := tracer.Start(ctx, "back to client")
	// defer span.End()

	// does not do any thing
	// md = metadata.MD{}
	// propagator := propagation.TraceContext{}
	// propagator.Inject(ctx, metadataCarrier(md))

	// // Invoke the gRPC method
	// ctxSp := trace.ContextWithSpan(ctx, span)
	// ctx = metadata.NewOutgoingContext(ctxSp, md)

	return resp, err
}

func (s *Server) GetOrder(ctx context.Context, orderID *wrapperspb.StringValue) (*pb.Order, error) {
	log.Println("See the contextttt: ", ctx)

	tr := otel.Tracer("component-server")
	// fmt.Println("The context: ", ctx)

	_, sp := tr.Start(ctx, "inTheServer")
	defer sp.End()

	// srv implementation
	ord := orderMap[orderID.Value]
	return &ord, nil
}

// return a stream with found datas
func (s *Server) SearchOrders(orderID *wrapperspb.StringValue, stream pb.OrderManagement_SearchOrdersServer) error {
	for key, order := range orderMap {
		// for _, item := range order.Items {
		// if strings.Contains(item, orderID.Value) {
		err := stream.Send(&order)
		if err != nil {
			return fmt.Errorf("error sending msg to stream: %v", err)
		}
		log.Print("Matching Order Found : " + key)
		// break
		// }
		// }
	}
	return nil
}

// receive a stream from the client and update its datas
func (s *Server) UpdateOrders(stream pb.OrderManagement_UpdateOrdersServer) error {

	ordersStr := "Updated Order IDs : "

	ordersLGT := len(orderMap)

	// pbOrders := pb.Order{}

	for {

		order, err := stream.Recv()
		if err == io.EOF {
			// finish reading the order stream
			return stream.SendAndClose(
				&wrapperspb.StringValue{Value: "Orders processed" + ordersStr})
		}

		log.Println("see datas: ", order)

		orderMap[strconv.Itoa(ordersLGT)] = *order
		ordersLGT++
	}
}

// stream bidirectional
func (s *Server) ProcessOrders(stream pb.OrderManagement_ProcessOrdersServer) error {
	var batchMarker = 3
	var orderBatchSize = 0

	var combinedShipmentMap = make(map[string]pb.CombinedShipment)
	pbt := pb.CombinedShipment{
		Id:     "1",
		Status: "SendingToThailand",
	}
	combinedShipmentMap["Thai"] = pbt
	pbf := pb.CombinedShipment{
		Id:     "2",
		Status: "SendingToFrance",
	}
	combinedShipmentMap["France"] = pbf

	var ordersFr = []*pb.Order{}
	var ordersTh = []*pb.Order{}
	// var newOrder Order

	for {
		orderID, err := stream.Recv()
		if err == io.EOF {

			for _, comb := range combinedShipmentMap {
				stream.Send(&comb)
			}
			return nil
		}
		if err != nil {
			return err
		}

		// Logic to organize orders into shipments,
		newOrder := orderMap[orderID.Value]

		if newOrder.Destination == "France" {
			ordersFr = append(ordersFr, &newOrder)
			pbf.OrdersList = ordersFr
			orderBatchSize++
		} else if newOrder.Destination == "Thailand" {
			ordersTh = append(ordersTh, &newOrder)
			pbt.OrdersList = ordersTh
			orderBatchSize++
		} else {
			log.Println("unknow Destination: ", newOrder.Destination)
		}

		// based on the destination.

		combinedShipmentMap["France"] = pbf
		combinedShipmentMap["Thai"] = pbt

		if batchMarker == orderBatchSize {
			for _, comb := range combinedShipmentMap {
				stream.Send(&comb)
			}
			// reset all var
			orderBatchSize = 0
			combinedShipmentMap = make(map[string]pb.CombinedShipment)
			ordersFr = []*pb.Order{}
			ordersTh = []*pb.Order{}
		}
	}
}

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

package apinotrace

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/trace"
	// "google.golang.org/grpc/codes"
	"gopkg.in/yaml.v2"

	// "go.opentelemetry.io/otel/propagation"
	httptrace "go.opentelemetry.io/contrib/instrumentation/net/http/httptrace/otelhttptrace"
	othttp "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	// "go.opentelemetry.io/otel/trace"
)

// go get go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp

const (
	// jaegerEndpoint string = "http://127.0.0.1:14268/api/traces"
	// jaegerAgentEndpoint string = "localhost:6832"
	jaegerAgentEndpoint string = "0.0.0.0:6831"
	service             string = "api"
	environment         string = "development"
	id                         = 0
)

func tracerProvider(url string) (*tracesdk.TracerProvider, error) {
	// Create the Jaeger exporter
	exp, err := jaeger.New(jaeger.WithAgentEndpoint(
		jaeger.WithAgentHost("127.0.0.1"),
		jaeger.WithAgentPort("6831")))
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

var services Config

func Run() {

	tp, err := tracerProvider(jaegerAgentEndpoint)
	// tp, err := tracerProvider(jaegerEndpoint)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	otel.SetTracerProvider(tp)

	defer func(ctx context.Context) {
		ctx, cancel := context.WithTimeout(ctx, time.Second*5)
		defer cancel()
		if err := tp.Shutdown(ctx); err != nil {
			log.Fatal(err)
		}
	}(ctx)

	mux := http.NewServeMux()
	// othttp.WithPublicEndpoint—this will slightly modify
	// how the trace context from the client is flowed to our backend services. Rather than
	// persisting the same TraceID from the client, the incoming context will be associated
	// with a new trace as a link
	mux.Handle("/", othttp.NewHandler(http.HandlerFunc(rootHandler), "root", othttp.WithPublicEndpoint()))
	mux.Handle("/calculate", othttp.NewHandler(http.HandlerFunc(calcHandler), "calculate", othttp.WithPublicEndpoint()))
	services = GetServices()

	log.Println("Initializing server...")
	err = http.ListenAndServe(":3000", mux)
	if err != nil {
		log.Fatalf("Could not initialize server: %s", err)
	}
}

func rootHandler(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w, "%v", services)
}

func enableCors(w *http.ResponseWriter, req *http.Request) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
	(*w).Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
	(*w).Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, X-B3-SpanId, X-B3-TraceId, X-B3-Sampled, traceparent")
}

func calcHandler(w http.ResponseWriter, req *http.Request) {

	enableCors(&w, req)
	if (*req).Method == "OPTIONS" {
		return
	}

	// Retrieve the span from the request context
	// Create a new outgoing trace with the parent span
	tr := otel.Tracer("in-calcHandler")
	// Create a new context and span
	// this will be a child from calculate,
	// and doing like so will show a specific event/span "generateRequest"
	ctx, span := tr.Start(req.Context(), "generateRequest")
	defer span.End()

	calcRequest, err := ParseCalcRequest(req.Body, span)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var url string

	for _, n := range services.Services {
		if strings.ToLower(calcRequest.Method) == strings.ToLower(n.Name) {
			j, _ := json.Marshal(calcRequest.Operands)
			url = fmt.Sprintf("http://%s:%d/%s?o=%s", n.Host, n.Port, strings.ToLower(n.Name), strings.Trim(string(j), "[]"))
		}
	}

	if url == "" {
		http.Error(w, "could not find requested calculation method", http.StatusBadRequest)
	}

	client := http.DefaultClient
	request, _ := http.NewRequest("GET", url, nil)

	// Create a new outgoing trace
	// ctx := req.Context()
	ctx, request = httptrace.W3C(ctx, request)
	// Inject the context into the outgoing request
	httptrace.Inject(ctx, request)

	res, err := client.Do(request)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	body, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp, err := strconv.Atoi(string(body))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "%d", resp)
}

type CalcRequest struct {
	Method   string `json:"method"`
	Operands []int  `json:"operands"`
}

func ParseCalcRequest(body io.Reader, span trace.Span) (CalcRequest, error) {
	var parsedRequest CalcRequest

	// Add event: attempting to parse body
	span.AddEvent("attempting to parse body")
	span.AddEvent(fmt.Sprintf("%s", body))
	err := json.NewDecoder(body).Decode(&parsedRequest)
	if err != nil {
		// span.SetStatus(codes.InvalidArgument)
		span.SetStatus(codes.Error, "the description")
		span.AddEvent(err.Error())
		span.End()
		return parsedRequest, err
	}
	span.End()

	return parsedRequest, nil
}

type Config struct {
	Services []struct {
		Name string `yaml:"name"`
		Host string `yaml:"host"`
		Port int    `yaml:"port"`
	} `yaml:"services"`
}

func GetServices() Config {
	wd, err := os.Getwd()
	if err != nil {
		log.Println("Error:", err)
	}

	relativePath := "../../services.yaml"

	absolutePath := filepath.Join(wd, relativePath)

	f, err := os.Open(absolutePath)
	if err != nil {
		log.Fatal("could not open config")
	}
	defer f.Close()

	var cfg Config
	decoder := yaml.NewDecoder(f)
	err = decoder.Decode(&cfg)
	if err != nil {
		log.Fatal("could not process config")
	}
	return cfg
}

// func GetServices() Config {
// 	f, err := os.Open("services.yaml")
// 	if err != nil {
// 		log.Fatal("could not open config")
// 	}
// 	defer f.Close()
//
// 	var cfg Config
// 	decoder := yaml.NewDecoder(f)
// 	err = decoder.Decode(&cfg)
// 	if err != nil {
// 		log.Fatal("could not process config")
// 	}
// 	return cfg
// }

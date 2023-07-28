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
	"runtime"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
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
	jaegerEndpoint string = "http://127.0.0.1:14268/api/traces"
	service        string = "api"
	environment    string = "development"
	id                    = 0
	tracingLibrary        = "go.opentelemetry.io/otel/trace"
)

// test like verbose mode, which ideally I would setup a end-point
// which would allow me to dynamically change the mode
// var traceVerbose = os.Getenv("TRACE_LEVEL") == "verbose"
var traceVerbose = true

func withLocalSpan(ctx context.Context, spanName string) (context.Context, trace.Span) {
	if traceVerbose {
		tr := otel.Tracer(tracingLibrary)
		pc, _, _, ok := runtime.Caller(1)
		callerFn := runtime.FuncForPC(pc)
		if ok && callerFn != nil {
			ctx, span := tr.Start(ctx, callerFn.Name())
			return ctx, span
		}
	}
	return ctx, nil
}

func finishLocalSpan(span trace.Span) {
	if traceVerbose {
		span.End()
	}
}

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

var services Config

func TracingMiddleware(h http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		tr := otel.Tracer(service)

		_, span := tr.Start(r.Context(), "FromRequest-ToResponse")
		span.SetAttributes(attribute.String("service", service))
		defer span.End()

		span.SetAttributes(attribute.String("route", r.URL.EscapedPath()))

		h(w, r)

		// the status does not show up but the idea is there
		span.SetAttributes(attribute.String("status", w.Header().Get("Status")))
	})
}

func Run() {

	tp, err := tracerProvider(jaegerEndpoint)
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

	// Wrap your handlers with the TracingMiddleware
	rootHandlerWithMiddleware := TracingMiddleware(rootHandler)
	calcHandlerWithMiddleware := TracingMiddleware(calcHandler)

	// mux.Handle("/", othttp.NewHandler(http.HandlerFunc(rootHandler), "root", othttp.WithPublicEndpoint()))
	mux.Handle("/", othttp.NewHandler(rootHandlerWithMiddleware, "root", othttp.WithPublicEndpoint()))
	// mux.Handle("/calculate", othttp.NewHandler(http.HandlerFunc(calcHandler), "calculate", othttp.WithPublicEndpoint()))
	mux.Handle("/calculate", othttp.NewHandler(calcHandlerWithMiddleware, "calculate", othttp.WithPublicEndpoint()))
	services = GetServices()

	log.Println("Initializing server...")
	err = http.ListenAndServe(":3000", mux)
	if err != nil {
		log.Fatalf("Could not initialize server: %s", err)
	}
}

func rootHandler(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w, "%v", services)
	getFoo(req.Context())
}

// the span will be different if verbose or not verbose mode.
func getFoo(ctx context.Context) {
	ctx, localSpan := withLocalSpan(ctx, "GetFoo datas")
	localSpan.SetAttributes(attribute.String("method", "getFoo"))
	// Do stuff
	time.Sleep(1 * time.Second)
	finishLocalSpan(localSpan)
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

	// as parseCalcRequest is an important func we like to always trace it
	// if it would not have been an important on then use the verbose
	tr := otel.Tracer(tracingLibrary)
	ctx, span := tr.Start(req.Context(), "Handle calcul requests")
	span.SetAttributes(attribute.String("method", "calcHandler"))
	defer span.End()

	calcRequest, err := ParseCalcRequest(req.Body, span)
	if err != nil {
		span.RecordError(err)
		span.AddEvent("error-encountered", trace.WithAttributes(
			// Additional attributes related to the error, if needed.
			// For example:
			attribute.String("error.code", "E123"),
			attribute.String("error.message", err.Error()),
		))

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
		span.RecordError(err)
		span.AddEvent("error-encountered", trace.WithAttributes(
			// Additional attributes related to the error, if needed.
			// For example:
			attribute.String("error.code", "E123"),
			attribute.String("error.message", err.Error()),
		))

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
		span.RecordError(err)
		span.AddEvent("error-encountered", trace.WithAttributes(
			// Additional attributes related to the error, if needed.
			// For example:
			attribute.String("error.code", "E123"),
			attribute.String("error.message", err.Error()),
		))

		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	body, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		span.RecordError(err)
		span.AddEvent("error-encountered", trace.WithAttributes(
			// Additional attributes related to the error, if needed.
			// For example:
			attribute.String("error.code", "E123"),
			attribute.String("error.message", err.Error()),
		))

		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp, err := strconv.Atoi(string(body))
	if err != nil {
		span.RecordError(err)
		span.AddEvent("error-encountered", trace.WithAttributes(
			// Additional attributes related to the error, if needed.
			// For example:
			attribute.String("error.code", "E123"),
			attribute.String("error.message", err.Error()),
		))

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

	// Add tags: attempting to parse body
	span.AddEvent("attempting to parse body")
	span.AddEvent(fmt.Sprintf("%s", body))
	err := json.NewDecoder(body).Decode(&parsedRequest)
	if err != nil {
		// span.SetStatus(codes.InvalidArgument)
		span.RecordError(err)
		span.AddEvent("error-encountered", trace.WithAttributes(
			// Additional attributes related to the error, if needed.
			// For example:
			attribute.String("error.code", "E123"),
			attribute.String("error.message", err.Error()),
		))
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

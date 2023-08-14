package addnotrace

import (
	"context"
	"fmt"
	"log"
	"net/http"
	// "os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/trace"
	// "go.opentelemetry.io/otel/propagation"
	othttp "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	// "go.opentelemetry.io/otel/trace"
)

const (
	jaegerEndpoint string = "http://127.0.0.1:14268/api/traces"
	service        string = "add"
	environment    string = "development"
	id                    = 1
	tracingLibrary        = "go.opentelemetry.io/otel/trace"
	samplingRatio         = 0.5
)

// test like verbose mode, which ideally I would setup a end-point
// which would allow me to dynamically change the mode
// var traceVerbose = os.Getenv("TRACE_LEVEL") == "verbose"
var traceVerb = true

type traceVerbose struct {
	ctx       context.Context
	span      trace.Span
	isVerbose bool
}

func NewTraceVerbose(iv bool) *traceVerbose {
	return &traceVerbose{
		isVerbose: iv,
	}
}

func (tv *traceVerbose) createLocalSpan(ctx context.Context, spanName string) func() {
	if tv.isVerbose {
		spanContext := trace.SpanContextFromContext(ctx)

		tr := otel.Tracer(tracingLibrary)
		// retrieves information about the calling function,
		// 1 means we want info about the current func
		pc, _, _, ok := runtime.Caller(1)
		// converts the function PC (program counter) to a *runtime.Func
		// This gives you information about the calling function.
		callerFn := runtime.FuncForPC(pc)
		if ok && callerFn != nil {
			ctx, span := tr.Start(
				trace.ContextWithRemoteSpanContext(ctx, spanContext),
				callerFn.Name())
			tv.ctx = ctx
			tv.span = span
		}
		return func() {
			tv.span.End()
		}

	}
	return func() {}
}

func (tv *traceVerbose) setAttributes(key, val string) {
	if tv.isVerbose {
		tv.span.SetAttributes(attribute.String(key, val))
	}
}

func (tv *traceVerbose) addEvent(evt string) {
	if tv.isVerbose {
		tv.span.AddEvent(evt)
	}
}

func (tv *traceVerbose) recordError(err error) {
	if tv.isVerbose {
		tv.span.RecordError(err)
	}
}

func (tv *traceVerbose) addEventError(key, code string, err error) {
	if tv.isVerbose {
		tv.span.AddEvent(key, trace.WithAttributes(
			// Additional attributes related to the error, if needed.
			// For example:
			attribute.String("error.code", code),
			attribute.String("error.message", err.Error()),
		))
	}
}

func tracerProvider(url string) (*tracesdk.TracerProvider, error) {
	// Create the Jaeger exporter
	exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(url)))
	if err != nil {
		return nil, err
	}

	// Create the root sampler (e.g., AlwaysSample for dev, TraceIDRatioBased for prod)
	// rootSampler := tracesdk.TraceIDRatioBased(samplingRatio)

	// Configure the ParentBased sampler using the root sampler
	// parentBasedSampler := tracesdk.ParentBased(rootSampler)

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
		// useless as I pass the sample info via the req header...
		// tracesdk.WithSampler(tracesdk.ParentBased(parentBasedSampler)), // sample half, good for prod
		// tracesdk.WithSampler(tracesdk.AlwaysSample()), // ok for dev
	)
	return tp, nil
}

// var samp sampler

// sampler make sure only sampled traces from the upstream service is sampled here
// note that verbose and sample may be combined.....
type sampler struct {
	sample    bool
	span      trace.Span
	available bool
}

func NewSampler() *sampler {
	return &sampler{}
}

func (s *sampler) createTrace(r *http.Request, name string) func() {
	isSampledStr := r.Header.Get("Is-Sampled")
	isSampled, _ := strconv.ParseBool(isSampledStr)
	if isSampled {
		s.sample = true
		tr := otel.Tracer(tracingLibrary)

		_, span := tr.Start(
			r.Context(),
			name,
		)
		s.span = span

		return func() { s.span.End() }
	}
	return func() {}
}

func (s *sampler) setAttributes(key string, x interface{}) {
	if s.sample {
		s.span.SetAttributes(attribute.String("service", service))
	}
}

// func TracingMiddleware(tr trace.Tracer, h http.HandlerFunc) http.HandlerFunc {
func TracingMiddleware(h http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		isSampledStr := r.Header.Get("Is-Sampled")
		fmt.Println("the isSampleStr: ", isSampledStr)

		samp := NewSampler()

		// isSamp := trace.SpanContextFromContext(r.Context()).IsSampled() // return true is sampled

		sp := samp.createTrace(r, "FromRequest-ToResponse")
		defer sp()
		samp.setAttributes("service", service)

		samp.setAttributes("route", r.URL.EscapedPath())

		// to inspect
		// Extract the trace context
		spanContext := trace.SpanContextFromContext(r.Context())

		// Print traceparent and tracestate headers
		fmt.Println("Traceparent:", spanContext.SpanID().String())
		fmt.Println("Tracestate:", spanContext.TraceState().String())

		h(w, r)

		// the status does not show up but the idea is there
		samp.setAttributes("status", w.Header().Get("Status"))
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

	addHandlerWithMiddleware := TracingMiddleware(addHandler)

	// mux.Handle("/", othttp.NewHandler(http.HandlerFunc(addHandler), "add", othttp.WithPublicEndpoint()))
	mux.Handle("/", othttp.NewHandler(addHandlerWithMiddleware, "add", othttp.WithPublicEndpoint()))

	log.Println("Initializing server...")
	err = http.ListenAndServe(":3001", mux)
	if err != nil {
		log.Fatalf("Could not initialize server: %s", err)
	}
}

func addHandler(w http.ResponseWriter, req *http.Request) {
	tv := NewTraceVerbose(traceVerb)

	sp := tv.createLocalSpan(req.Context(), "Handle addition")
	defer sp()

	tv.setAttributes("method", "addHandler")
	tv.addEvent("start addition")

	values := strings.Split(req.URL.Query()["o"][0], ",")
	var res int
	for _, n := range values {
		i, err := strconv.Atoi(n)
		if err != nil {
			tv.recordError(err)
			tv.addEventError("error-encountered", "E123", err)

			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		res += i
	}
	fmt.Fprintf(w, "%d", res)
}

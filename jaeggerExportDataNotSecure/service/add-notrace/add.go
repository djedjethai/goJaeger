package addnotrace

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	// "go.opentelemetry.io/otel/propagation"
	othttp "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	// "go.opentelemetry.io/otel/trace"
)

const (
	// jaegerEndpoint string = "http://127.0.0.1:14268/api/traces"
	jaegerAgentEndpoint string = "http://localhost:6831"
	service             string = "add"
	environment         string = "development"
	id                         = 1
)

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

func Run() {

	tp, err := tracerProvider(jaegerAgentEndpoint)
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
	mux.Handle("/", othttp.NewHandler(http.HandlerFunc(addHandler), "add", othttp.WithPublicEndpoint()))

	log.Println("Initializing server...")
	err = http.ListenAndServe(":3001", mux)
	if err != nil {
		log.Fatalf("Could not initialize server: %s", err)
	}
}

func addHandler(w http.ResponseWriter, req *http.Request) {
	values := strings.Split(req.URL.Query()["o"][0], ",")
	var res int
	for _, n := range values {
		i, err := strconv.Atoi(n)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		res += i
	}
	fmt.Fprintf(w, "%d", res)
}

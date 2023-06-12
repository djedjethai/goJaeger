module getting-started-with-ccloud-golang

go 1.20

replace (
	go.opentelemetry.io/otel => ./opentelemetry-go
	go.opentelemetry.io/otel/exporters/jaeger => ./opentelemetry-go/exporters/jaeger
	go.opentelemetry.io/otel/sdk => ./opentelemetry-go/sdk
)

require (
	github.com/confluentinc/confluent-kafka-go/v2 v2.1.1
	github.com/golang/protobuf v1.5.3
	github.com/google/uuid v1.3.0
	github.com/riferrei/srclient v0.5.4
	go.opentelemetry.io/otel v1.16.0
	go.opentelemetry.io/otel/exporters/jaeger v0.0.0-00010101000000-000000000000
	go.opentelemetry.io/otel/sdk v1.16.0
	google.golang.org/protobuf v1.30.0
)

require (
	github.com/go-logr/logr v1.2.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/linkedin/goavro/v2 v2.11.1 // indirect
	github.com/santhosh-tekuri/jsonschema/v5 v5.2.0 // indirect
	go.opentelemetry.io/otel/metric v1.16.0 // indirect
	go.opentelemetry.io/otel/trace v1.16.0 // indirect
	golang.org/x/sync v0.1.0 // indirect
	golang.org/x/sys v0.8.0 // indirect
)

replace go.opentelemetry.io/otel/trace => ./opentelemetry-go/trace

replace go.opentelemetry.io/otel/metric => ./opentelemetry-go/metric

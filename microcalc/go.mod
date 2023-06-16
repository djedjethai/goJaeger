module github.com/austinlparker/microcalc

go 1.13

replace (
	go.opentelemetry.io/otel => ./opentelemetry-go
	go.opentelemetry.io/otel/exporters/jaeger => ./opentelemetry-go/exporters/jaeger
	go.opentelemetry.io/otel/sdk => ./opentelemetry-go/sdk
)

require (
	go.opentelemetry.io/contrib/instrumentation/net/http/httptrace/otelhttptrace v0.42.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.42.0 // indirect
	go.opentelemetry.io/otel v1.16.0
	go.opentelemetry.io/otel/exporters/jaeger v0.0.0-00010101000000-000000000000
	go.opentelemetry.io/otel/sdk v1.16.0
	go.opentelemetry.io/otel/trace v1.16.0
	gopkg.in/yaml.v2 v2.2.7
)

replace go.opentelemetry.io/otel/trace => ./opentelemetry-go/trace

replace go.opentelemetry.io/otel/metric => ./opentelemetry-go/metric

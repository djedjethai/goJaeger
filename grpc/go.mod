module communication

go 1.20


replace (
	go.opentelemetry.io/otel => /home/jerome/Documents/code/goJaeger/grpc/opentelemetry-go
	go.opentelemetry.io/otel/exporters/jaeger => /home/jerome/Documents/code/goJaeger/grpc/opentelemetry-go/exporters/jaeger
	go.opentelemetry.io/otel/sdk => /home/jerome/Documents/code/goJaeger/grpc/opentelemetry-go/sdk
)

require (
	go.opentelemetry.io/otel v1.16.0-rc.1
	go.opentelemetry.io/otel/exporters/jaeger v0.0.0-00010101000000-000000000000
	go.opentelemetry.io/otel/sdk v1.16.0-rc.1
	google.golang.org/grpc v1.54.0
	google.golang.org/protobuf v1.30.0
)

require (
	github.com/go-logr/logr v1.2.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	go.opentelemetry.io/otel/metric v1.16.0-rc.1 // indirect
	go.opentelemetry.io/otel/trace v1.16.0-rc.1 // indirect
	golang.org/x/net v0.8.0 // indirect
	golang.org/x/sys v0.8.0 // indirect
	golang.org/x/text v0.8.0 // indirect
	google.golang.org/genproto v0.0.0-20230125152338-dcaf20b6aeaa // indirect
)

replace go.opentelemetry.io/otel/trace => /home/jerome/Documents/code/goJaeger/grpc/opentelemetry-go/trace

replace go.opentelemetry.io/otel/metric => /home/jerome/Documents/code/goJaeger/grpc/opentelemetry-go/metric

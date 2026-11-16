// The backend for frontend: a Go module of its own, so the backend's
// ./... leaves it out and it never shares the backend's dependencies.
module github.com/raveesh-me/dedo-todo/bff

go 1.26.4

require (
	buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go v1.36.12-20260825204119-511051f7f437.2
	connectrpc.com/connect/v2 v2.0.2
	github.com/GoHighLevel/dedo v0.43.5
	github.com/raveesh-me/dedo-todo v0.0.0
	go.opentelemetry.io/otel v1.47.0
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.47.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.47.0
	go.opentelemetry.io/otel/sdk v1.47.0
	go.opentelemetry.io/otel/sdk/metric v1.47.0
	google.golang.org/genproto/googleapis/api v0.0.0-20261026165753-2ffab32bce7e
	google.golang.org/genproto/googleapis/rpc v0.0.0-20261019180522-ce908d29299b
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/GoHighLevel/dedo/mcp v0.43.5 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.30.0 // indirect
	github.com/modelcontextprotocol/go-sdk v1.8.0 // indirect
	github.com/raveesh-me/dedo-todo/gen/options v0.0.0
	github.com/segmentio/asm v1.2.1 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.47.0 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
	go.opentelemetry.io/proto/otlp v1.11.0 // indirect
	golang.org/x/net v0.61.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	google.golang.org/grpc v1.84.0 // indirect
)

// The backend's generated clients, from this repository's working tree:
// the frontend, this BFF and the backend they call build from one commit.
replace github.com/raveesh-me/dedo-todo => ../

// dedo: the MCP Go SDK with in-place JSON decoding, until a release has it (https://github.com/modelcontextprotocol/go-sdk/pull/1454). dedo upgrade maintains this line.
replace github.com/modelcontextprotocol/go-sdk v1.8.0 => github.com/raveesh-me/go-sdk v1.8.0-dedo.1

replace github.com/raveesh-me/dedo-todo/gen/options => ../gen/options

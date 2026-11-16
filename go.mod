module github.com/raveesh-me/dedo-todo

go 1.26.4

// dedo: the MCP Go SDK with in-place JSON decoding, until a release has it (https://github.com/modelcontextprotocol/go-sdk/pull/1454). dedo upgrade maintains this line.
replace github.com/modelcontextprotocol/go-sdk v1.8.0 => github.com/raveesh-me/go-sdk v1.8.0-dedo.1

tool (
	connectrpc.com/connect/v2/cmd/protoc-gen-connect-go
	github.com/GoHighLevel/commerce-auth/go/cmd/protoc-gen-commerce-iam
	github.com/GoHighLevel/dedo/cmd/dedo
	github.com/GoHighLevel/dedo/mcp/cmd/protoc-gen-dedo-mcp
	github.com/GoHighLevel/dedo/openapi/cmd/protoc-gen-dedo-openapi
	google.golang.org/protobuf/cmd/protoc-gen-go
)

require (
	buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go v1.36.12-20260825204119-511051f7f437.2
	connectrpc.com/connect/v2 v2.0.2
	connectrpc.com/validate v0.9.0
	connectrpc.com/vanguard v0.5.0
	github.com/GoHighLevel/commerce-auth/go v0.4.0
	github.com/GoHighLevel/dedo v0.43.5
	github.com/GoHighLevel/dedo/mcp v0.43.5
	github.com/jackc/pgx/v5 v5.13.0
	github.com/modelcontextprotocol/go-sdk v1.8.0
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
	buf.build/go/protovalidate v1.4.0 // indirect
	cel.dev/cel-go v0.32.0 // indirect
	cel.dev/expr v0.25.3 // indirect
	connectrpc.com/connect v1.21.0 // indirect
	github.com/GoHighLevel/dedo/cmd/dedo v0.43.5 // indirect
	github.com/GoHighLevel/dedo/lint v0.43.5 // indirect
	github.com/GoHighLevel/dedo/openapi v0.43.5 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.30.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.3 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/segmentio/asm v1.2.1 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.47.0 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
	go.opentelemetry.io/proto/otlp v1.11.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/exp v0.0.0-20260820142414-ca536658362e // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/net v0.61.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	golang.org/x/tools v0.52.0 // indirect
	google.golang.org/grpc v1.84.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

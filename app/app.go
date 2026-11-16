// Package app is the composition root: the only code, beside main, that
// imports several modules. It builds each module's dependencies, applies
// their migrations in dependency order, registers their handlers on one
// connect.Server behind commerce-auth's guard and dedo's interceptors, and
// serves that server as Connect, gRPC, REST, OpenAPI docs and MCP tools.
package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"
	"connectrpc.com/connect/v2/connectinprocess"
	"connectrpc.com/validate"
	"connectrpc.com/vanguard"
	"github.com/GoHighLevel/commerce-auth/go/iamguard"
	"github.com/GoHighLevel/dedo/events"
	"github.com/GoHighLevel/dedo/flowid"
	"github.com/GoHighLevel/dedo/interceptor"
	"github.com/GoHighLevel/dedo/mcp/mcpserver"
	"github.com/GoHighLevel/dedo/policy"
	"github.com/GoHighLevel/dedo/telemetry"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/raveesh-me/dedo-todo/docs"
	tasksv1 "github.com/raveesh-me/dedo-todo/gen/go/commerce/todo/tasks/v1"
	"github.com/raveesh-me/dedo-todo/gen/go/commerce/todo/tasks/v1/tasksv1connect"
	time_trackingv1 "github.com/raveesh-me/dedo-todo/gen/go/commerce/todo/time_tracking/v1"
	"github.com/raveesh-me/dedo-todo/gen/go/commerce/todo/time_tracking/v1/time_trackingv1connect"
	"github.com/raveesh-me/dedo-todo/internal/shared/db"
	"github.com/raveesh-me/dedo-todo/internal/tasks"
	"github.com/raveesh-me/dedo-todo/internal/time_tracking"
)

// APIVersion is the Version header platform IAM requires on every call: the
// version the REST reference documents and the MCP tools were built for.
const APIVersion = "v4"

// Source names this service in every event it publishes.
const Source = "//todo.commerce.example.com"

// Services are the services the server serves, by full name.
var Services = []string{
	tasksv1connect.TaskServiceName,
	time_trackingv1connect.TimerServiceName,
	time_trackingv1connect.TimeEntryServiceName,
}

// Config is what the server is built from.
type Config struct {
	// Pool reaches Postgres as the owner of the modules' schemas.
	Pool *pgxpool.Pool
	// Authorizer is platform IAM: iambackend in every real environment.
	Authorizer iamguard.Authorizer
	// Authenticated reports whether an MCP request carries a token the
	// service accepts, for MCP_ACCESS=authenticated.
	Authenticated func(*http.Request) bool
	// MCPAccess and MCPAllowedHosts are MCP_ACCESS and MCP_ALLOWED_HOSTS.
	MCPAccess       string
	MCPAllowedHosts string
	// Now is the clock; nil is time.Now. Tests move it.
	Now func() time.Time
	Log *slog.Logger
}

// Migrate applies the modules' migrations, Tasks before Time Tracking.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	return db.Migrate(ctx, pool, tasks.Migrations(), time_tracking.Migrations())
}

// New builds the service's HTTP handler.
func New(cfg Config) (http.Handler, error) {
	if cfg.Pool == nil || cfg.Authorizer == nil {
		return nil, errors.New("app: Pool and Authorizer are required")
	}
	log := cfg.Log
	if log == nil {
		log = slog.Default()
	}
	// The API: scopes and policy come from the .proto, not from handlers.
	table, err := policy.Build(Services...)
	if err != nil {
		return nil, err
	}
	guard, err := interceptor.New(interceptor.Config{
		Table:         table,
		Scopes:        iamguard.Scopes,
		EnforceScopes: true,
		Log:           log,
	})
	if err != nil {
		return nil, err
	}
	// commerce-auth: one IAM AuthZ call per request, against the route key
	// built from the proto and the tenant each service declares.
	iam, err := iamguard.New(iamguard.Config{
		Services:   descriptors(),
		Authorizer: cfg.Authorizer,
	})
	if err != nil {
		return nil, err
	}
	observe, err := telemetry.NewServerInterceptor(telemetry.Config{Table: table})
	if err != nil {
		return nil, err
	}
	// The interceptors run for every transport, MCP tool calls included, in
	// order: flow IDs, telemetry, IAM, the declared contract (scopes,
	// headers, error audit), then the buf.validate rules.
	server := connect.NewServer(flowid.Interceptor(), observe, iam.Interceptor(), guard, validate.NewServerInterceptor())

	// Calls the service makes to itself, in-process, on its caller's
	// behalf: the caller's token forwarded, its location named, the API
	// version set, the trace and flow ID continued.
	traced, err := telemetry.NewClientInterceptor(telemetry.Config{})
	if err != nil {
		return nil, err
	}
	self := connect.NewClient(connectinprocess.New(server),
		flowid.ClientInterceptor(), traced, apiVersion(APIVersion), callerLocation(), iamguard.ForwardToken())

	// Tasks announces completions and deletions on the bus; the reactions
	// stop a timer running on that task, through Time Tracking's API.
	bus := newBus(log, newReactions(time_trackingv1connect.NewTimerServiceClient(self), log))
	pub, err := events.New(events.Config{Driver: bus, Source: Source, Log: log})
	if err != nil {
		return nil, err
	}

	// Each module's handlers, from internal/<module>.
	taskReader := tasksACL{client: tasksv1connect.NewTaskServiceClient(self)}
	tasksv1connect.RegisterTaskServiceHandler(server, tasks.NewTaskService(cfg.Pool, pub, cfg.Now))
	time_trackingv1connect.RegisterTimerServiceHandler(server, time_tracking.NewTimerService(cfg.Pool, taskReader, cfg.Now))
	time_trackingv1connect.RegisterTimeEntryServiceHandler(server, time_tracking.NewTimeEntryService(cfg.Pool, taskReader, cfg.Now))

	mux := http.NewServeMux()
	// Connect, gRPC and gRPC-Web, then REST from the google.api.http bindings.
	connecthttp.Mount(mux, server)
	if err := vanguard.Mount(mux, server); err != nil {
		return nil, err
	}

	// MCP: every RPC is a tool, called in-process on the same server with
	// the caller's token, Version and location-id.
	tools := mcp.NewServer(&mcp.Implementation{Name: "dedo-todo", Version: "v1"}, nil)
	toolClient := connect.NewClient(connectinprocess.New(server), apiVersion(APIVersion))
	if err := mcpserver.AddToolsWith(tools, mcpserver.Options{
		Client:         toolClient,
		ForwardHeaders: []string{"Version", iamguard.HeaderLocationID}, // a caller's own Version wins
	}, append(append(tasksv1.TaskServiceMCPTools(), time_trackingv1.TimerServiceMCPTools()...), time_trackingv1.TimeEntryServiceMCPTools()...)...); err != nil {
		return nil, err
	}
	authenticated := cfg.Authenticated
	if authenticated == nil {
		authenticated = func(r *http.Request) bool { return r.Header.Get("Authorization") != "" }
	}
	endpoint, err := mcpserver.Guard(mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return tools },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true}),
		mcpserver.Access(cfg.MCPAccess), authenticated)
	if err != nil {
		return nil, err
	}

	mux.Handle("GET /openapi.json", docs.Spec())
	mux.Handle("GET /docs", docs.Page())
	// Health is the server's, never the API's: outside the interceptors, so
	// a probe needs no token (DEDO_NO_HEALTH_RPC). Readiness checks the
	// database the modules need.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := cfg.Pool.Ping(ctx); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	// Cross-origin protection: the MCP transport requires an Origin check.
	// AllowHosts lets a bridge in a container (Claude Desktop's) reach a
	// service on localhost as host.containers.internal.
	endpoint = mcpserver.AllowHosts(endpoint, mcpserver.AllowedHosts(cfg.MCPAllowedHosts))
	endpoint = telemetry.Middleware(telemetry.Config{}, "/mcp", http.NewCrossOriginProtection().Handler(endpoint))
	mux.Handle("/mcp", flowid.Middleware(endpoint))
	return mux, nil
}

// descriptors are the served services' descriptors, for the guard.
func descriptors() []protoreflect.ServiceDescriptor {
	return []protoreflect.ServiceDescriptor{
		tasksv1.File_commerce_todo_tasks_v1_tasks_proto.Services().ByName("TaskService"),
		time_trackingv1.File_commerce_todo_time_tracking_v1_timer_proto.Services().ByName("TimerService"),
		time_trackingv1.File_commerce_todo_time_tracking_v1_time_entries_proto.Services().ByName("TimeEntryService"),
	}
}

// apiVersion sets the Version header platform IAM requires on a call that
// lacks one: the API version the service's MCP tools were generated from.
func apiVersion(v string) connect.ClientInterceptor {
	return func(next connect.ClientFunc) connect.ClientFunc {
		return func(ctx context.Context, spec connect.Spec) (connect.ClientStream, error) {
			info, ok := connect.CallInfoForClientContext(ctx)
			if !ok {
				ctx, info = connect.NewClientContext(ctx)
			}
			if info.RequestHeader().Get("Version") == "" {
				info.RequestHeader().Set("Version", v)
			}
			return next(ctx, spec)
		}
	}
}

// callerLocation names, on a call made on a caller's behalf, the location
// IAM verified the caller in: the forwarded token is judged there too.
func callerLocation() connect.ClientInterceptor {
	return func(next connect.ClientFunc) connect.ClientFunc {
		return func(ctx context.Context, spec connect.Spec) (connect.ClientStream, error) {
			caller, ok := iamguard.FromContext(ctx)
			if ok && strings.EqualFold(caller.Target.Class, "Location") {
				info, found := connect.CallInfoForClientContext(ctx)
				if !found {
					ctx, info = connect.NewClientContext(ctx)
				}
				info.RequestHeader().Set(iamguard.HeaderLocationID, caller.Target.ID)
			}
			return next(ctx, spec)
		}
	}
}

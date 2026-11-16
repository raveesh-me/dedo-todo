<!-- dedo:begin guidance (dedo init and dedo upgrade rewrite this block: change its source, not this copy) -->
# Building an API with dedo: guidance for coding agents

`dedo init` adds this file to your repository's `AGENTS.md` and `CLAUDE.md`, followed by [domains.md](https://github.com/GoHighLevel/dedo/blob/main/guidance/domains.md) in its own `dedo:begin domains` block, and `dedo upgrade` keeps both current. dedo's full guidance: [install.md](https://github.com/GoHighLevel/dedo/blob/main/guidance/install.md) sets a service up, [tutorial.md](https://github.com/GoHighLevel/dedo/blob/main/guidance/tutorial.md) builds one from scratch, [guideline.md](https://github.com/GoHighLevel/dedo/blob/main/guidance/guideline.md) explains the reasoning, [domains.md](https://github.com/GoHighLevel/dedo/blob/main/guidance/domains.md) maps domain-driven design onto packages and services, and [RULES.md](https://github.com/GoHighLevel/dedo/blob/main/guidance/RULES.md) documents every lint rule. It tells a coding agent how a service built with [dedo](https://github.com/GoHighLevel/dedo) works and which mistakes to avoid.

**Your organisation's guidance wins.** An organisation can add its own guidance after this section: the annexures that `dedo.yaml`'s `annexure` names, each in its own `dedo:begin annexure:` block. Where an annexure and this section conflict, follow the annexure; where it is silent, this section holds. Never edit the text between a `<!-- dedo:begin … -->` line and its `<!-- dedo:end … -->` line: `dedo upgrade` rewrites those blocks from their sources. Change `dedo.yaml` or the annexure's source instead, and keep your own notes outside the blocks.

## The proto is the source of truth

- One `.proto` defines the API. dedo serves it as Connect/gRPC, REST (`google.api.http`, through Vanguard), an OpenAPI 3.2 reference and MCP tools. The reference and the tools leave out services whose `audience` is narrower than the generators' `min_audience`: by default only `AUDIENCE_COMPONENT_INTERNAL` ones. Change the proto, never the generated output.
- Never edit anything under `gen/` or `docs/openapi.json`. Run `make generate` and commit the result; CI fails on stale output. Use the `make` targets (`run`, `generate`, `check`, `test`, `tools`; `docs` and `docs-tunnel` to show the REST reference, never by tunnelling the service, and `docs-static` to write it as one file that opens offline) and `dedo` rather than calling `buf` or `go` directly; where the repository's own `Makefile` already had one of those names, dedo's is `api-<name>`.
- Set dedo up with `dedo init` and upgrade it with `dedo update && dedo upgrade`, never by hand-editing pins: the options module's label in `buf.yaml` (which also names the lint plugin's release) and the generators and runtime in `go.mod` are one release and move together. After a fresh clone, run `dedo tools`: it copies the lint plugin at `buf.yaml`'s release into `.tools/`, installing it into `$GOBIN` first if needed, then fetches the house rules and checks their overrides. A `url` import's or annexure's `sha256` pins the published file: change it only to move to another release, copying the pin that release publishes, never to silence a mismatch, which means the file changed. `dedo upgrade -lint` moves only the options module and the lint plugin to a new release. Leave the `replace` of `github.com/modelcontextprotocol/go-sdk` in `go.mod`, and in a BFF's `go.mod`, alone: dedo maintains it (a build of the SDK with a fix not yet released upstream) and removes it when upstream has the fix.
- Before you finish a change to `proto/`, run `make check` (format, lint, breaking). Fix findings in the proto. Suppress one only with `// buf:lint:ignore <RULE_ID>` and a written reason.
- Scopes, cache policy, deprecation and error reasons are declared in the proto (`dedo.v1` options) and enforced by the runtime. Do not write scope checks, cache headers or deprecation headers in handlers.
- Return errors with a declared reason in a `google.rpc.ErrorInfo` detail. Undeclared reasons are logged as audit findings. The service uses connect-go v2: handlers take and return plain messages, errors are `connect.NewError(code, message)`, and any other error reaches callers as `UNKNOWN` with no message.
- A server stream that clients wait on (a watch, a feed) sends a first message saying it is open, such as a `CHANGE_WATCHING` value, rather than relying on headers: buffering proxies hold headers back, and connect-go v2.0.1's HTTP transport sends none until the first message (fixed upstream after v2.0.1).
- In a multi-tenant service (`tenancy` in `dedo.v1.service`), every resource name starts with the tenant (`locations/1/contacts/2`) and every request names it in the field its HTTP path binds. The runtime rejects a request before the handler runs if that field, or any other field with a `google.api.resource_reference`, names a tenant the caller may not act in. Give every field that holds a resource name a `resource_reference`: one without is not checked. Handlers still scope their storage queries to the tenant. Never mark an RPC `cross_tenant` to get past the check: that is only for RPCs that genuinely span tenants, such as listing the caller's own.
- A service never subscribes to or watches anything; it only answers calls. A command that announces what it did declares each event type in `emits` and publishes it with `events.Publisher` after the change commits, never through an outbox, a server stream of events or webhooks. Events carry no payload: a type and a resource name. Return `Publish`'s error so the caller retries, and declare `EVENT_NOT_PUBLISHED` with `UNAVAILABLE` in the RPC's errors.
- Health is the server's, not the API's. `main.go` serves `GET /healthz` beside `/docs`, outside the interceptors, so a probe needs no token. Never declare a health service, RPC or message in the proto, or import `grpc/health/v1/health.proto` (`DEDO_NO_HEALTH_RPC`). A readiness check of the server's dependencies is a route beside `/healthz` too.
- Give every scalar field an example, in protovalidate's rule of the field's own type: `(buf.validate.field).string.example = "products/01HZX3QK4M"`, `int64.example = 240`, `timestamp.example = {seconds: 1767225600}`, `repeated.items.string.example`, `map.keys`/`map.values` for a map. Never invent a `dedo.v1` option or a comment convention for it. Output-only fields need one too; enums and messages don't (`DEDO_FIELD_EXAMPLE`), nor do the fields of `google.type` messages, which the generators give dedo's examples. Write a realistic value a real caller would send or see, in the format the service really issues; never a secret, a token or real personal data (use `example.com` addresses and `555` numbers). A resource name's example is a full name in its pattern's form (`shelves/main/books/01HZX3`), since the reference derives path parameter examples from its segments. Match the declared type (an `int64` takes `int64.example = 240`, not a string) and, for an enum's `enum.example`, a number the enum defines. [guideline.md](https://github.com/GoHighLevel/dedo/blob/main/guidance/guideline.md#examples) has the details.
- Put cross-cutting checks (auth, logging) in `connect.ServerInterceptor`s on the one `connect.Server`, not in HTTP middleware: MCP tool calls reach the server in-process and skip HTTP middleware.

## Packages and services follow the domain

The `dedo:begin domains` block after this one, from [domains.md](https://github.com/GoHighLevel/dedo/blob/main/guidance/domains.md), maps each building block of domain-driven design to a layer of the API: the repository is the bounded context, a proto package a module, a service an aggregate, and a service that manages no resource a domain service. It also lays the code and the database out by module: one Go package per proto module at `internal/<module>/`, one Postgres schema and role per module, and only `main` and `app/` importing several modules. Shape new packages, services and Go code by it, and check them against its checklist.

- **A backend for a frontend (BFF) is a nested Go module with an unnamed buf module**, such as `bff/` with `bff/proto`, written by `dedo init -bff`. Follow [domains.md's Backends for frontends](https://github.com/GoHighLevel/dedo/blob/main/guidance/domains.md#backends-for-frontends) before changing one: its services are `AUDIENCE_COMPONENT_INTERNAL`; it owns no data, never imports the backend's packages and forwards the caller's credentials; what only the frontend reads becomes an internal-audience service of the backend; and the frontend and BFF are paired by commit SHA. `dedo modules` checks the layout.

## Lint classes and precedence

`buf-plugin-dedo-lint` reports these classes of rules:

| Prefix | Source |
| --- | --- |
| `AIP_*` | Google's AIPs, through api-linter |
| `ZALANDO_*` | The Zalando RESTful API guidelines dedo adopts |
| `DEDO_*` | dedo's own conventions |
| `HOUSE_*` | Your organisation's own rules, as CEL checks in `dedo.yaml`'s `lint` and `breaking` sections (and the shared files it imports) |
| `DEDO_MULTI_TENANT_*` | dedo's opt-in rules for multi-tenant services, the `DEDO_MULTI_TENANT` set |

`dedo.yaml`'s `lint` section chooses which lint rules run: `use` names the rule sets (`AIP_NOLANG`, `ZALANDO`, `DEDO`, `HOUSE`, and `DEDO_MULTI_TENANT` for multi-tenant services), `disable` turns rules off by ID, and its `rules`, with the house rules its top-level `imports` bring, build on those sets and override them where they say so. Its `breaking` section does the same for the breaking rules `buf breaking` runs (`DEDO` and `HOUSE`): those check a change against the earlier version, such as an RPC dropping a scope, a REST path or an event type, which Buf's own breaking rules cannot see; a house breaking rule reads the earlier element as `against`. `buf.yaml` lists only `DEDO` in `lint.use` and `DEDO_BREAKING` beside `FILE` in `breaking.use`, which hold every rule of the plugin; never name another of its rules there, in `use` or `except`. A breaking finding means the change breaks callers: keep what it names, or make the change in a new major version; never silence it to merge.

Rules rank in layers: **the repository's own `dedo.yaml` > the house rules it imports > DEDO > AIP > ZALANDO**. Nothing resolves a conflict automatically: a rule that replaces others names them in its `overrides`, and only rules of a lower layer, never a final one: dedo's `ZALANDO_SERVICE_RULES_REQUIRED`, `DEDO_METHOD_RULES_REQUIRED` and `DEDO_MULTI_TENANT_REQUEST_TENANT`, and any house rule marked `final: true`, which cannot be disabled either. Run `dedo tools` after every change to `dedo.yaml`'s rules, of either kind, or to an import: it fetches changed imports, checks `use`, `disable`, every override and final rule, and lists what runs and the overrides in force. Read that list before assuming a rule runs. Without a house override, dedo's conventions hold:

- Every RPC returns its own `<Rpc>Response`; REST unwraps it with `response_body`. Do not "fix" this to AIP's bare-resource returns.
- The major version is in the package and the path (`/<root>/v1/...`). Path segments are lowerCamelCase, never kebab-case.
- JSON field names are the protojson default. Never set `json_name`.
- Errors are `google.rpc.Status` with `ErrorInfo`, not Problem JSON. Pagination is `page_token`/`next_page_token`.

[RULES.md](https://github.com/GoHighLevel/dedo/blob/main/guidance/RULES.md#skipped-rules) lists every rule dedo skips on purpose, by source. Do not re-enable them.

## Go rules

`dedo vet` lints the Go code that serves the contract, and `make check` runs it (`make vet` alone). Run it before you finish any change to Go code, and fix every finding in the code. [RULES.md](https://github.com/GoHighLevel/dedo/blob/main/guidance/RULES.md#go-rules) explains each rule; a finding links to its section.

- `DEDO_GO_SERVER_INTERCEPTORS`: every `connect.NewServer` outside tests has `flowid.Interceptor()` and the policy interceptor from `interceptor.New`, the flow ID one first. Never drop either to make a test or a feature work.
- `DEDO_GO_ERROR_REASON_DECLARED`: a handler returns only the `ErrorInfo` reasons its RPC declares. When a handler needs a new reason, declare it in the proto (`dedo.v1.method` errors, with its code) and regenerate; never rename the reason in Go to dodge the finding.
- `DEDO_GO_EVENT_DECLARED`: a handler publishes only the event types in its RPC's `emits`. Add the type to the proto first.
- `HOUSE_GO_*`: your organisation's rules from `dedo.yaml`'s `go` section, such as an import it has ruled out. The finding's message says what to do instead; follow it rather than wrapping the package somewhere it is allowed.
- Suppress a finding only when it does not apply, with `//dedo:ignore <RULE_ID> <reason>` on or above its line, and say why in the reason: a directive without one suppresses nothing and is itself a finding (`DEDO_GO_IGNORE_REASON`). Name every suppression you add in your summary.

## Dependencies

Qualify a dependency before adding it, as [dependencies.md](https://github.com/GoHighLevel/dedo/blob/main/guidance/dependencies.md) describes. Your organisation's annexure says which licenses it permits.

- Add a dependency only when the task needs it, and only after checking that the standard library or an existing dependency can't do the job. Name every one you add in your summary, with its version, its license and the modules it brings with it (`go list -m all`, before and after).
- Never add a thin wrapper: a package that only renames, re-exports or sets defaults on another library, or turns a service's API into a few functions. Use the library directly, or call the API and keep its request and response types in this repository.
- Pin exact versions: `go get <module>@vX.Y.Z`, `go get -tool <module>@vX.Y.Z`, `go run <module>@vX.Y.Z`, GitHub Actions by commit SHA, images by digest, npm packages without `^` or `~`. Never `@latest`, a floating tag or `go get -u ./...`. Update one dependency per change, except dedo's own pins, which `dedo update && dedo upgrade` moves together.
- Check the license of the dependency and of every module it brings: `go run github.com/google/go-licenses/v2@v2.0.1 report ./...`. A license the annexure doesn't list as allowed outright needs a person's approval: stop and ask before adding it.
- Run `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...` after adding or updating a dependency, and fix what it reports before you finish.

## Observability

A dedo service reports through OpenTelemetry and W3C Trace Context, never a vendor's SDK or headers. `main.go` sets up the SDK and OTLP exporters in `setupTelemetry`, configured only by the standard `OTEL_*` environment variables; the runtime (package `telemetry`) imports only OpenTelemetry's API.

- Keep `telemetry.NewServerInterceptor` right after `flowid.Interceptor()` in `connect.NewServer`, and `telemetry.Middleware` around `/mcp`. They give every RPC a span and `rpc.server.call.duration`, on every transport, following OpenTelemetry's semantic conventions for Connect RPC. Never instrument a handler by hand for what they record, and never add HTTP tracing middleware for the API routes: it would duplicate the spans.
- Give every Connect client to another service `telemetry.NewClientInterceptor` beside `flowid.ClientInterceptor()`, so the trace continues there.
- Log with the call's context, `log.InfoContext(ctx, ...)`, through the `slog` logger `main.go` builds: `telemetry.NewLogHandler` adds `trace_id` and `span_id` only from the context.
- Configure telemetry with `OTEL_*` variables (`OTEL_SERVICE_NAME`, `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_TRACES_SAMPLER`, ...), never with code or vendor variables. Add a vendor's exporter only to `setupTelemetry`, and only when OTLP cannot reach the backend.
- To record a request field on the span and its logs, declare it in the proto: `(dedo.v1.field) = {attribute: "<module>.<name>"}`, with `attribute_mode` `ATTRIBUTE_MODE_PRESENCE` or `ATTRIBUTE_MODE_ITEM_COUNT` where the value does not fit. Never set it from handler code. A key is in your own namespace, never a semantic-conventions one (`rpc.`, `http.`, `server.`, `error.`, ...) or `dedo.`, and once released it is kept (`DEDO_FIELD_ATTRIBUTE_KEPT`). A sensitive field records only its presence. Never put a token or a whole request on a span, a metric or a log record, and never a value with many distinct values, such as an ID, on a metric.
- Service level objectives are declared in the proto (`service_level` in `dedo.v1.service`, and `{exempt: true}` or objectives of its own in `dedo.v1.method`), and `make generate` writes them into `slo/` as OpenSLO. Never edit `slo/` by hand, and never write SLO queries by hand for what the contract can declare.
- `error.type` and an error status mean the server failed (`INTERNAL`, `UNAVAILABLE`, ...). Return `NOT_FOUND`, `INVALID_ARGUMENT` and the like for the caller's mistakes, with a declared reason, so they do not count against the service. When a declared reason counts wrongly by its code, declare `error_classifications` on the RPC rather than changing the code.

## MCP tools

Every unary, non-deprecated RPC becomes an MCP tool named after it in snake_case (`GetTodo` becomes `get_todo`), unless its service's `audience` is narrower than `protoc-gen-dedo-mcp`'s `min_audience` (by default, only `AUDIENCE_COMPONENT_INTERNAL` services are left out). `protoc-gen-dedo-mcp` generates them as `<Service>MCPTools()`, and `main.go` serves them with `mcpserver.AddTools(tools, client, <pkg>.<Service>MCPTools()...)`, `client` being `connect.NewClient(connectinprocess.New(server))`: calls go in-process to the service's `connect.Server`. For a service it leaves out, `<Service>MCPTools()` returns no tools, so `main.go` keeps compiling. Never write tool definitions by hand: change the proto and regenerate. Calls go through the same auth, scopes and validation as REST and Connect.

- Who may reach `/mcp` is configuration, not code: `MCP_ACCESS=public` (the default; anyone may list the tools, every call is still scope-checked) or `authenticated` (requests without a token the service accepts get 401, tool discovery included). Keep `mcpserver.Guard` wired with the service's real token check.
- A tool call reaches the service with only the MCP request's `Authorization` and `X-Flow-ID` headers. When the service's interceptors need another header, such as an API version, list it in `mcpserver.AddToolsWith(tools, mcpserver.Options{Client: client, ForwardHeaders: []string{"<Header>"}}, ...)` (`Config.ForwardHeaders` for `mcpserver.New`). MCP clients rarely send such headers, so when the service requires one, also give the in-process client a `connect.ClientInterceptor` that sets the value the tools were built for when the caller sent none.
- A Connect client to another service over HTTP needs its own `http.Client` with `Transport: &http.Transport{MaxIdleConnsPerHost: <your concurrency>}`, reused for every call. Never `http.DefaultClient`: it keeps two idle connections per host, and under load the service runs out of local ports (`cannot assign requested address`).

### What the model sees

- **Only `name`, `description` and `inputSchema` reach the model.** Hosts such as Claude Desktop and claude.ai turn MCP tools into model tool definitions with just those three fields. `title`, `annotations` (`readOnlyHint`, `destructiveHint`, `idempotentHint`, `openWorldHint`) and `outputSchema` reach the client application but not the model. That is by design: the MCP specification makes annotations hints for clients, untrusted when the server is.
- So **put everything the model must know in `method.tool.description`**: what the tool does, whether it is irreversible ("cannot be recovered"), and when to use it instead of a similar tool. dedo appends the declared error reasons to it.
- Still declare `method.tool.annotations` accurately. Clients use them, for example to ask the user before a destructive call, and `DEDO_TOOL_ANNOTATIONS` checks them against `idempotency_level`.
- Keep spec citations such as "AIP-134" out of comments on request fields and messages: those comments become schema descriptions the model reads.

### Where the input schema comes from

`protoc-gen-dedo-mcp` builds each tool's schemas at generation time:

| Declaration | In the input schema |
| --- | --- |
| Leading field comment | The field's `description`. Write it for a model. |
| `dedo.v1.message` description | The object's `description` |
| `google.api.field_behavior` | `REQUIRED` is required; `OUTPUT_ONLY` is left out. A create omits the resource name; an update with a `FieldMask` requires only the name and lists the accepted field names. |
| `google.api.resource`, `resource_reference` | A `pattern` for resource names, such as `^todos/[^/]+$` |
| `buf.validate` rules | `minimum`, `maximum`, `minLength`, `maxLength`, `pattern`, `enum`, `minItems`, `maxItems`, and `format` for email, uri and uuid |
| `google.type.Money` | The currency code pattern and the `nanos` range |

State a limit as a `buf.validate` rule, not only in prose, and add `connectrpc.com/validate`'s interceptor so the server enforces it on every transport. Rules the schema cannot express (CEL, such as Money's sign rule) are still enforced by the server; describe them in the field comment.

### Behaviour of the served tools

- Arguments and results use proto field names (`budget_amount`). Field masks take the same names; `mcpserver` converts them to the JSON mapping, and lowerCamelCase also works.
- Results write default values, so an empty list is `{"todos": [], "next_page_token": ""}`, never `{}`.
- 64-bit integers: inputs accept a JSON number or a decimal string; outputs are strings.
- A failed call, including a schema validation failure, is a `CallToolResult` with `isError: true` and a JSON body `{"code", "message", "reason"}`. JSON-RPC errors are only for protocol faults such as an unknown tool.
- Wrap the MCP handler in `http.NewCrossOriginProtection()`. The MCP transport requires servers to reject requests from other web origins (DNS rebinding); MCP clients send no `Origin` and pass.

### Checking a tool on the wire

Do not judge the server from what a model reports: it cannot see annotations, titles or output schemas. Check the raw response instead:

```sh
curl -s http://localhost:8080/mcp -H 'Authorization: Bearer <token>' \
  -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

The official suite, `@modelcontextprotocol/conformance server --url <url>`, mostly calls fixture tools (`test_simple_text` and the like) that only its reference server has; those failures do not apply to a dedo service. Its DNS-rebinding scenario needs a `localhost` URL. Run Node tools in a container.

### Connecting a local client

- Claude Code: `claude mcp add --transport http <name> http://localhost:<port>/mcp --header "Authorization: Bearer <token>"`
- Claude Desktop: `dedo local-mcp-claude -token <token>` (or `make kind-claude`), then restart Claude Desktop. Its config file takes only servers it launches as a local command, and Settings › Connectors needs HTTPS with OAuth, so neither reaches `http://localhost` directly. The command adds an `mcpServers` entry that runs `mcp-remote` (pinned) in a podman container (docker if podman is missing), bridging stdio to the running service's `/mcp`, with `--allow-http` and the token as a header. It points at the kind cluster's port from `zarf/k8s/dev/kind-config.yaml`, or `make run`'s 8080 without one; `-url` points elsewhere, `-print` prints the entry, `-remove` removes it. It keeps the rest of the config and a `.bak` of it.
- Do not give Claude Desktop the service over stdio instead: a stdio server is a second process with its own state, apart from the one serving REST and the UI. The bridge keeps one server, and each MCP call stays stateless.
- A container reaches the host as `host.containers.internal` (`host.docker.internal` under docker), which the MCP SDK's DNS rebinding protection refuses on a loopback connection. `main.go` wraps the endpoint in `mcpserver.AllowHosts`, which serves those two names as `localhost` and still refuses every other name; `MCP_ALLOWED_HOSTS` (comma-separated) replaces the list. In kind the check never fires: traffic reaches the pod on its cluster address. A server written before this release adds the line itself.
- A static bearer token is for local development only; production MCP authorization is OAuth 2.1.
<!-- dedo:end guidance -->

<!-- dedo:begin domains (dedo init and dedo upgrade rewrite this block: change its source, not this copy) -->
# Modelling the domain: DDD and dedo's layers

`dedo init` adds this file to your repository's `AGENTS.md` and `CLAUDE.md`, after [claude.md](https://github.com/GoHighLevel/dedo/blob/main/guidance/claude.md), and `dedo upgrade` keeps it current.

An API built with dedo has layers: a repository, its proto packages, their services, the resources they manage, and the RPCs on those resources. Each layer is one of the building blocks of domain-driven design (DDD). Shape the API along that mapping, and the contract, the REST reference and the MCP tools all read in the language of the domain. Readers then navigate it the way the business thinks about it, not the way the code happens to be organised.

The terms come from Eric Evans' *Domain-Driven Design* (bounded context, module, aggregate, entity, value object, ubiquitous language, context map) and Vaughn Vernon's *Effective Aggregate Design* (small aggregates, reference by identity). The resource terms come from Google's AIPs, which dedo lints against.

## The layers

| DDD | dedo | Where it shows |
| --- | --- | --- |
| Domain | The organisation's business as a whole | The shared house rules and the root every path starts with (`/shop`) |
| Bounded context | One repository: one API module on the BSR, and at most one unnamed module beside it for a [backend for frontend](#backends-for-frontends) | One REST root (`/shop/v1`), one OpenAPI document, one MCP server |
| Module | One proto package: `acme.orders.v1` | A navigation tag in the reference, holding the package's services; in the service, one Go package (`internal/orders/`) and one Postgres schema |
| Aggregate | One service: `OrderService` | A tag in the reference, nested under its package's |
| Aggregate root | The resource the service manages: a message with `google.api.resource` | Its resource name (`orders/{order}`) is its identity and its REST path |
| Entity inside an aggregate | A child resource, its pattern nested under the root's (`orders/{order}/lines/{line}`) | A nested REST path |
| Value object | A message without `google.api.resource`: `google.type.Money`, an address | A schema, never a path |
| Domain service | A service that manages no resource, holding only custom methods: `AddressService` | A tag in the reference; paths that start at the tenant (`locations/{location}:resolveAddress`) or at a collection noun (`addresses:resolveAddress`) |
| Command | An RPC that changes state: a standard method or a custom method (`:cancel`) | A `POST`, `PATCH` or `DELETE`; an MCP tool with `read_only_hint: false` |
| Query | `Get` and `List`, declared `NO_SIDE_EFFECTS` | A `GET`; an MCP tool with `read_only_hint: true` |
| Domain event | An event type a command declares in `emits` (`acme.orders.order.cancelled`), published outward to a bus | A line in the operation's description and `x-emits`; not a REST path |
| Ubiquitous language | Names, `title`s and leading comments | Every heading and description in the reference, and every MCP tool description |

## Bounded context: one repository, one root

A bounded context is the boundary inside which every term has one meaning. In dedo it is one repository.

- One repository holds one context: one API module on the BSR, one REST root (`/<root>/v1/...`), one OpenAPI document and one MCP server. Beside it, the repository may hold one unnamed module, which is never published: the contract of a [backend for frontend](#backends-for-frontends). Two contexts that share a repository share a module and a lint configuration, and their terms start to leak into each other.
- The context owns its storage. No other context reads its tables; they call its API.
- Within the context, every name means one thing. `Order` cannot mean a basket in one package and a binding sale in another. Keep the glossary where the product decisions live, and use exactly those words in the proto.

## Modules: one package each

A module groups the parts of the model that belong together, so that the model can be understood one part at a time. In dedo a module is a proto package, `<context>.<module>.v<major>`.

- **Name the package with a noun from the domain**: `orders`, `catalog`, `payments`. Never name it after a technical layer: `api`, `common`, `models`, `handlers` and `v1` alone say nothing about the domain.
- **Open the package with a comment that says what the module is responsible for**, directly above the `package` statement with no blank line between them. The reference shows the comment as the module's description:

  ```proto
  // Orders is what buyers commit to buy, and what they send back.
  package acme.orders.v1;
  ```

- **Group by cohesion, not by size.** The services of one package change together, for the same reasons. A package that holds unrelated services is two modules.
- **Keep modules acyclic.** One module may refer to another's resources by name, and import its messages where it must. A call to another module's service counts as a dependency, and so does an internal port or repository that reads its data, the same as an import. A cycle through any of them means what an import cycle means: the two packages are one module, or a third holds what both need.

  Buf catches only import cycles. A `resource_reference` field is a string, so it adds no import, and cycles through calls pass the build. In the Go layout below, a port is an import, so Go rejects a cycle through ports, and `dedo modules` checks calls through generated clients. If `catalog` calls `inventory` to show stock, and `inventory` calls `catalog` to refuse a discontinued product, buf passes, but the two depend on each other. To break the cycle, keep one direction and let the client or a later call combine the two; merge the modules; or move what both need into a third. An export or report that reads every module belongs in a module of its own, which nothing else depends on.
- **Put shared value objects in a package of their own**, such as `acme.type`, with no services and no version. It gets no tag in the reference.
- **A context small enough for one module has one package.** Its reference lists its services without a package level.
- **The package is not in the REST path.** Paths keep one root per context, so regrouping modules does not move a REST path. It does rename the Connect and gRPC procedures, which carry the package, so `buf breaking` reports it as a breaking change. Settle the modules before the first release.

### Code and data follow the modules

The service keeps the modules of its protos in its Go code and in its database. Each module's dependencies are then explicit, so the cycles that the bullet on keeping modules acyclic warns about fail a build, a check or a query, not only a review. A context `acme` with the packages `acme.orders.v1`, `acme.orders.v2` and `acme.catalog.v1` looks like this:

```text
internal/
  orders/              # module orders: acme.orders.v1 and acme.orders.v2
    handlers_v1.go     # adapters for acme.orders.v1
    handlers_v2.go     # adapters for acme.orders.v2
    orders.go          # the domain: aggregates and their rules
    port.go            # Port: what other modules may read
    store.go           # the only code that reads the orders schema
    migrations/
    queries/
  catalog/             # module catalog: acme.catalog.v1
    ...
  shared/
    money/             # shared helpers
app/                   # the composition root, with main
main.go
```

- **One Go package per proto module.** The packages `<ctx>.<m>.vN` of a context `<ctx>` put module `m`'s code at `internal/<m>/`: its handlers, domain, store, migrations and queries. A service with one module uses the same layout, so every service reads the same way.
- **Shared helpers go at `internal/shared/<x>/`.** `shared` is reserved, so no proto module may be called `shared`: `DEDO_PACKAGE_SHARED_RESERVED` rejects `acme.shared.v1`. Value objects that the protos share go in `<ctx>.type`, as above.
- **Everything stays under `internal/`**, so no other repository can import it. Other contexts call the API.
- **The composition root is the only code that imports several modules.** `main` and the `app/` package build each module's dependencies, run the migrations in dependency order and register the handlers.
- **Every version of a module shares one package.** During an AIP-185 migration, `<ctx>.<m>.v1` and `<ctx>.<m>.v2` are two API surfaces over one domain and one store. Their handlers are adapters in `internal/<m>/`, such as `handlers_v1.go` and `handlers_v2.go`, and moving from v1 to v2 is not a cycle. The module boundary is the domain, not the API version: both versions serve the same aggregates, keep the same rules and own the same data. Split into two packages, they would have to share all three.
- **Ports belong to their provider.** A module that reads another depends on the provider's own `Port` interface, such as `filing.Port`, which returns the provider's generated messages. The dependency is then a Go import, and Go rejects an import cycle at compile time. Never put ports in a shared package: it hides every edge, and a cycle through it compiles. Modules never subscribe to each other's events: one reads another through its `Port`.
- **Generated clients and messages are edges too.** A call through another module's generated client (`<m>v1connect`) or an import of its messages is a dependency. Go cannot see a cycle through them, since every module imports only generated code, so `dedo modules` checks them.
- **Each module owns a Postgres schema named after it.** Only its migrations and queries touch it. Shared infrastructure, such as migration bookkeeping, lives in a `shared` schema.
- **No foreign keys or views across modules.** A reference to another module's data is a resource name, as in the protos.
- **Each module runs as its own database role**, with privileges only on its own schema. A query that reaches into another module's tables then fails at runtime.

## Aggregates: one service each

An aggregate is a cluster of objects that changes as one, with one root that guards its rules. In dedo an aggregate is one service and the resource it manages.

- **One service per aggregate root**, named after it: `OrderService` manages `Order`. Give the service the plural noun readers look for as its `title` (`Orders`); the reference uses it as the service's heading. dedo doesn't lint that the service is named after its root, but a house rule on a service's `roots` can ([RULES.md](RULES.md#house-rules)). It does lint that the heading says no *service*, *module* or *aggregate* ([`DEDO_DOCS_NO_CODE_STRUCTURE`](RULES.md#dedo_docs_no_code_structure)).
- **Every RPC of the service acts on that aggregate.** An RPC is one transaction, so the aggregate is the consistency boundary: what must always hold true is enforced inside one RPC. A rule that spans two aggregates holds eventually, through a later call to the other aggregate, not inside one transaction: the caller's, or that of an orchestrator an event notified.
- **Keep aggregates small.** Put only what must be consistent at once inside the boundary. Large aggregates make for large requests, conflicting writes and slow pages.
- **Refer to another aggregate by its resource name, never by embedding its message.** Declare the field with `google.api.resource_reference`. Embedding couples two lifecycles and leaks the other aggregate's rules into this one. A multi-tenant service also checks the tenant of every referenced name.
- **Entities inside the aggregate are child resources**, with patterns nested under the root's. They change through the root's service, so the root can keep its rules.

## Domain services: operations with no aggregate

Some operations belong to no aggregate and keep no state: resolving a free-text address, suggesting tariff codes for a product description, converting between currencies. DDD calls them domain services. In dedo a domain service is a service that manages no resource.

- **Name it after the noun readers look for.** `AddressService`, titled `Addresses`; `TariffCodeService`, titled `Tariff codes`. It declares no `google.api.resource` and has no standard methods, only custom methods named with the domain's verbs: `ResolveAddress`, `SuggestTariffCodes`.
- **In a multi-tenant service, hang its methods off the tenant.** AIP-136 attaches a stateless method to the resource that scopes it, with the whole RPC name after the `:`. Here that resource is the tenant. The request names the tenant in a field of its own, so the runtime checks the tenant as it does for any other call:

  ```proto
  rpc ResolveAddress(ResolveAddressRequest) returns (ResolveAddressResponse) {
    option idempotency_level = NO_SIDE_EFFECTS;
    option (google.api.http) = {get: "/shop/v1/{location=locations/*}:resolveAddress"};
  }

  message ResolveAddressRequest {
    // The location asking, such as locations/1.
    string location = 1 [
      (google.api.field_behavior) = REQUIRED,
      (google.api.resource_reference).type = "shop.example.com/Location"
    ];
    // The address as a person typed it.
    string text = 2 [(google.api.field_behavior) = REQUIRED];
  }
  ```

  A path without the tenant, such as `/shop/v1/addresses:resolveAddress`, fails `DEDO_MULTI_TENANT_REQUEST_TENANT`. Don't make the method `cross_tenant` to get past the rule: then only its scopes would guard it.
- **With one tenant, put its methods after a collection noun.** Such a service has no resource to scope the method, and a path straight after the version (`/shop/v1:resolveAddress`) fails `DEDO_HTTP_PATH_VERSION`. Use the plural noun of what the method is about. The custom verb in the path follows `AIP_0136_HTTP_URI_SUFFIX`: just the verb when the RPC name ends with the collection noun (`SuggestTariffCodes` at `/shop/v1/tariffCodes:suggest`), and the whole RPC name when it doesn't (`ResolveAddress` at `/shop/v1/addresses:resolveAddress`, not `addresses:resolve`).
- **One that only answers is a query.** Declare it `idempotency_level = NO_SIDE_EFFECTS`, with `read_only_hint: true`. Prefer `GET`, with the input in query parameters: Connect can then call it over GET too, and the method can declare a cache policy (`VISIBILITY_PRIVATE` in a multi-tenant service). Use `POST` with `body: "*"` when the input may not fit in a URL, such as a product description. The method stays `NO_SIDE_EFFECTS`.
- **Say whether it reaches outside the context.** A domain service often wraps a vendor behind an anti-corruption layer. One that writes to the vendor, for example to send a message or file a return, is a command, and always sets `open_world_hint: true`. One that only reads from a vendor, such as a geocoder, usually sets it too, since its answer comes from data the context does not own. It may set `false` when the answer is reference data the context could hold itself, such as codes from a published tariff schedule.
- **It never changes an aggregate.** In DDD a domain service may also coordinate a change across aggregates, such as moving money between two accounts. In dedo an RPC is one aggregate's transaction, so that change is not a domain service. Put the command on the aggregate it changes and let an orchestrator, notified by its event, call the other, or make the process a resource of its own (`transfers/{transfer}`) whose service drives the steps.
- **It lives in the module whose language it speaks**, beside the aggregates that use it. When several modules use it, it gets a module of its own, and they depend on it through its generated client or its `Port`, like any other module.

## Commands and queries

- **Commands are named with the domain's verbs.** Use the standard methods (`Create`, `Update`, `Delete`) where they fit, and AIP-136 custom methods for transitions the business names: `CancelOrder` at `:cancel`, not `UpdateOrderStatus`. The RPC name becomes the MCP tool's name, so a model reads the same verb the business uses.
- **Queries have no side effects.** Declare `Get` and `List` with `idempotency_level = NO_SIDE_EFFECTS`, and their tools' `read_only_hint: true`; `DEDO_TOOL_ANNOTATIONS` checks that the two agree. The runtime sets their cache headers from the cache policy they declare.
- **Domain events leave the aggregate after the transaction.** A command may announce what it did, and whoever reacts does so in a transaction of its own ([below](#domain-events-outward-only)).

## Domain events: outward only

A core service waits to be called. It never subscribes to a bus, polls another service or watches a stream. When it takes part in orchestration, it may announce what happened, outward, to a bus. Whatever reacts, an orchestrator or another context, runs on its own and calls APIs, this one's included.

- **An event is a notification, not a document.** It says that something happened to a resource, and nothing more: a type (`acme.orders.order.cancelled`), the resource's name as its subject (`orders/123`), an ID and a time. It has no payload. Consumers fetch the resource through the API, with their own token and scopes, so an event never carries data past a scope check. A duplicate or an event out of order costs a consumer one more fetch, since the fetch returns the current state. The ID stays the same when the bus redelivers an event, but a command the caller retries publishes a new one. Give `Publish` the command's `request_id` (AIP-155) as the event's idempotency key, and the events of every retry carry the same key, which a consumer that must not act twice keeps.
- **The command declares what it may announce**, in `emits`. The type is the package without its version, then the resource and the verb: `<context>.<module>.<resource>.<past-tense verb>`. Name the transition the business names (`cancelled`, `shipped`), not `updated`. `DEDO_EVENT_TYPES` checks the form, and the REST reference lists each operation's events. Most commands emit nothing: add an event when a consumer needs one. `buf breaking` does not compare option values, so renaming or dropping a type passes it and silently stops its consumers: treat a released type like a field, and add new ones rather than rename one.

  ```proto
  rpc CancelOrder(CancelOrderRequest) returns (Order) {
    option (dedo.v1.method) = {
      scopes: "orders.order.write"
      errors: {reason: "ORDER_NOT_FOUND", code: NOT_FOUND}
      errors: {reason: "EVENT_NOT_PUBLISHED", code: UNAVAILABLE}
      emits: "acme.orders.order.cancelled"
    };
  }
  ```

- **The handler publishes after the change commits**, through `events.Publisher` ([install.md](install.md#publishing-events)). The runtime refuses a type the RPC does not declare. The bus is a driver the service injects, as `database/sql` takes a database driver: dedo has no opinion on Pub/Sub, Kafka or anything else. Once the driver acknowledges an event, the bus delivers it at least once.
- **Until then, `Publish` retries within the request's deadline**, with exponential backoff. If the bus does not acknowledge in time, the handler returns the error, `UNAVAILABLE` with reason `EVENT_NOT_PUBLISHED`, and the caller retries the command. That closes the gap between the commit and the acknowledgement without the service holding any state, and needs the command to be safe to retry: idempotent, or a create that takes a `request_id` (AIP-155). A handler that may lose an event can log the error and succeed instead.
- **Nothing else carries events.** No outbox, no server stream of events, no webhooks or registered clients: each makes the core service hold state about its consumers. A consumer that needs a live feed, such as a browser, gets it from a gateway that reads the bus.

## Between contexts: the context map

- **Published language.** The API module on the BSR, with the event types its commands declare, is the language other contexts speak to this one. Consumers depend on a released label, and `buf breaking` keeps released versions compatible.
- **Open host service.** REST, Connect, gRPC and MCP are one host service, all generated from the same contract.
- **Anti-corruption layer.** When the context calls a vendor or another context with a model of its own, an adapter inside the service translates between the two models. The vendor's model never appears in the proto: the contract speaks only this context's language.
- **Shared kernel.** dedo's options module and the `google.type` messages are shared by every context. Keep what contexts share this small: anything else shared between them belongs to one context, and the others refer to it.

## Backends for frontends

A backend for frontend (BFF) serves exactly one frontend. It shapes what the frontend's pages need from the context's API, and its contract changes with those pages, not with the domain. In dedo a BFF lives in the service's repository, as a Go module and a binary of its own, above the domain:

```text
go.mod                 # the backend: module example.com/shop
buf.yaml               # modules: proto (named), bff/proto (no name)
proto/                 # the published API
internal/
app/
main.go
bff/
  go.mod               # the BFF: module example.com/shop/bff
  buf.gen.yaml         # the BFF's Go and Connect code, the frontend's client
  proto/shop/web/v1/   # the frontend's contract
  main.go
```

- **A BFF is a nested Go module.** It is a directory with its own `go.mod`, such as `bff/`, that holds a module path of `buf.yaml`, such as `bff/proto`. `dedo modules` finds it from that layout, with no setting, and leaves its protos out of the backend's modules. `dedo init -bff` writes one ([install.md](install.md#adding-a-bff-dedo-init--bff)). The backend's `go test ./...` stops at the nested module, so `make check`, `make test` and the CI stage run the tests in `bff/` too, and check that its generated code is current ([guideline.md](guideline.md#9-ci)).
- **Nothing about it is published.** Its buf module has no `name`, so `buf push --exclude-unnamed` never pushes it. Its own `buf.gen.yaml` generates only the BFF's Go and Connect code and the frontend's client, such as `protoc-gen-es`: never an OpenAPI document or MCP tools. Its services declare `AUDIENCE_COMPONENT_INTERNAL`, and the services of a named module never do; `dedo modules` checks both.
- **One rule set, no breaking check.** The repository's `dedo.yaml` lints the BFF's protos as it lints the published API, so there is no second rule set to maintain. Its entry in `buf.yaml` ignores the module for `buf breaking`: the frontend and the BFF are built from one commit and paired exactly, so no other version of the frontend ever calls it.
- **It owns no data.** No data-store client anywhere in its dependencies (SQL, MongoDB, Redis, Firestore, Spanner, BigQuery, DynamoDB, Elasticsearch and the rest [install.md](install.md#keeping-modules-apart-dedo-modules) lists), no database credentials, no schema. It never depends on the backend's own packages, anything of its Go module but the code generated from its protos (`internal/`, `app/`, `cmd/`, the root's `main`), directly or through another module: a Go module nested under the backend's path may import its `internal/`, so `dedo modules` follows every import. It reads this context only through its Connect API, and other contexts only through their published SDKs. The backend never imports the BFF.
- **What only the frontend reads becomes an internal service of the backend.** A read the pages need and no other caller does, such as the choices a form offers, is a service in the backend's API with an internal audience, such as `AUDIENCE_BUSINESS_UNIT_INTERNAL`. It keeps its breaking check, since several BFF versions call it at once while one retires: change it additively. A public reference leaves it out with the generators' `min_audience` ([install.md](install.md#3-code-generators)).
- **It holds no authority.** A BFF has no identity or credentials of its own. It forwards the caller's credentials on every call it makes, the `Authorization` header, the flow ID and the headers it is configured with, through `bfftools.Forward`. Every access decision stays in the services it calls, and they check the token and scopes on internal APIs too, never trusting a caller for being inside the network.
- **It changes no dependency direction.** A BFF may read several contexts, each through its own API. They still never depend on it, nor, through it, on each other: if the `shop` context must never depend on `payments`, a BFF that reads both is fine.
- **The frontend and its BFF are paired by commit.** Both are built from the same commit, and the first 12 characters of its SHA pair them, printed by `git rev-parse HEAD | cut -c1-12` (never `--short=12`, which prints more when 12 are ambiguous): the frontend calls `<prefix>/<sha>/...`, such as `/shop/bff/3f2a9c1d7e4b/shop.web.v1.CartService/GetCart`, and a BFF answers only its own SHA (`bfftools.Pin`). Any other SHA gets `FAILED_PRECONDITION` with a `google.rpc.ErrorInfo` whose reason is `BFF_VERSION_RETIRED` (`bfftools.ReasonVersionRetired`), and the frontend reloads the whole page, which loads the current frontend. There is no `latest` route, so a frontend never reaches a BFF built from another commit.
- **Release the BFF first, and retire the previous one after a window.** Each release runs in this order:
  1. Deploy the new BFF, routed at `<prefix>/<sha>/` under a name that carries its SHA, such as `shop-bff-<sha>`, so two versions run side by side. It must answer before its frontend is published, or that frontend reloads into itself.
  2. Publish the frontend, then purge the frontend's entry file (the HTML or remote entry that names the versioned assets) from every cache in front of it, or serve that file with `Cache-Control: no-cache`. A cached entry file keeps loading the previous frontend: once the previous BFF retires, every reload gets `BFF_VERSION_RETIRED` and loads the same stale file again, in a loop. Hashed assets can stay cached for good.
  3. Keep the previous BFF serving its frontend for a fixed retention window, at its smallest size: 7 days is the recommended window, so a tab left open over a weekend still works. It logs its activity per SHA through `bfftools.Activity`: one structured line per request with its SHA, procedure and a caller key, which the consumer derives from the verified token or tenancy headers, such as user and location. The log is the durable record; `GET /activity` only counts what one process has seen since it started.
  4. At the end of the window, a scheduled job writes a final summary of that SHA's log lines (requests, distinct callers, last request), then deletes the deployment. A request for a SHA with no BFF goes to the current one, which answers `BFF_VERSION_RETIRED`.

  As a guard, the frontend reloads on `BFF_VERSION_RETIRED` at most once per SHA in a session, and shows an error after that.

## How the reference shows it

`protoc-gen-dedo-openapi` turns the layers into OpenAPI 3.2 tags. When the documented services span several packages, each package becomes a navigation tag (`kind: nav`) named after the package. Its `summary` is the package's last segment before the version, in sentence case (`acme.orders.v1` reads "Orders", `shop.tax_profile.v1` "Tax profile"), and its description is the package comment. Each service's tag names its package as its `parent`:

```json
"tags": [
  {"name": "acme.orders.v1", "summary": "Orders", "kind": "nav",
   "description": "Orders is what buyers commit to buy, and what they send back."},
  {"name": "Orders", "parent": "acme.orders.v1", "description": "Reads orders."},
  {"name": "Returns", "parent": "acme.orders.v1", "description": "Reads returns."}
]
```

Operations name their tag by name alone, so every tag name must be unique across the document: two services titled `Orders` in different packages fail generation, and one of them needs another title. A package's comment comes from the first of its files that has one. A service whose tag repeats its package's heading, so that the reference reads "Calculation › Calculation", draws a warning when it is not named after the one root it manages: it is probably named after its module. The warning names the root to call it after (`TaxQuoteService` titled "Tax quotes"). `OrderService` titled `Orders` in `acme.orders.v1` draws none: that is the layout this guide describes.

The layers are how the code is built; the reader sees only the domain. Headings are domain nouns, and descriptions say what the API does for its caller, never which service, module or aggregate does it: `Reads orders.`, not `OrderService reads orders.` The proto names keep their `Service` suffix, which buf requires and the Connect and gRPC paths carry, but the reference never shows it. [`DEDO_DOCS_NO_CODE_STRUCTURE`](RULES.md#dedo_docs_no_code_structure) checks both.

A reference that supports 3.2 shows context, module, aggregate and operation as four levels. The docs page that `dedo init` writes uses Scalar, which does. A viewer that ignores `parent` still lists every service, without the package level.

Documentation sites that build their navigation from `x-tagGroups` instead, the extension Redoc introduced, such as Docusaurus with `docusaurus-plugin-openapi-docs`, find the same grouping there: the document lists one group per package, named after its title (the nav tag's `summary`), with the tags of its services in order. A document of one package gets one group too, since those sites put a tag that no group lists under "UNTAGGED"; when that package has no name, the group takes the document's title. Two packages with one title, such as `acme.orders.v1` and `shop.orders.v1`, are told apart by their full names. The nav tags themselves are in no group: they hold no operations.

```json
"x-tagGroups": [
  {"name": "Orders", "tags": ["Orders", "Returns"]}
]
```

`no_tag_groups` in the generator's `opt` leaves `x-tagGroups` out. Scalar reads it too, as a heading over the services of each package, so a single-package reference shows that heading above its services.

## Checklist

- One repository, one context, one REST root.
- One package per module, named with a domain noun and opened with a comment that says what it is responsible for. No module is called `shared`.
- One Go package per module at `internal/<m>/`, holding every version's handlers, its domain, store, migrations and queries; shared helpers at `internal/shared/<x>/`; only `main` and `app/` import several modules.
- A module reads another only through the provider's `Port` or its generated client, and the modules stay acyclic.
- One Postgres schema and one database role per module; no foreign keys or views across modules.
- One service per aggregate, titled with the noun readers look for; every RPC acts on that aggregate.
- Headings and descriptions speak the domain's language: no *service*, *module* or *aggregate* in a heading, and no proto service name in a comment.
- An operation with no aggregate is a domain service: no resource, only custom methods, hung off the tenant in a multi-tenant service and off a collection noun otherwise. It changes no aggregate, and one that only answers is a `NO_SIDE_EFFECTS` query, over `GET` where the input fits in a URL.
- References to other aggregates are resource names with `resource_reference`, never embedded messages.
- Commands are named with the domain's verbs; queries are `NO_SIDE_EFFECTS`.
- A command that announces what it did declares each event type in `emits` and publishes it after the commit, outward only; events carry no payload, and the service never subscribes.
- Vendor and other contexts' models stay behind an adapter, out of the proto.
- A BFF is a nested Go module whose buf module has no name: linted with the repository's rules, never checked for breaking changes, and generated only for its own Go and Connect code and its frontend's client. Its services are `AUDIENCE_COMPONENT_INTERNAL`; no published service is.
- A BFF owns no data and holds no credentials: no data-store client, no dependency on the backend's packages, and the caller's credentials forwarded on every call. It reads its context through internal-audience services, which check tokens and scopes and change additively; the backend never imports it.
- A frontend and its BFF are built from one commit and paired by its first 12 characters (`git rev-parse HEAD | cut -c1-12`); any other SHA gets `BFF_VERSION_RETIRED`. Each release deploys the BFF first, purges the frontend's entry file (or serves it `no-cache`), and retires the previous BFF after a fixed window, 7 days recommended, from its per-SHA activity log.
<!-- dedo:end domains -->

<!-- dedo:begin annexure:https://gohighlevel.github.io/commerce-dedo-rules/v0.17.0/annexure.md (dedo init and dedo upgrade rewrite this block: change its source, not this copy) -->
# Commerce services on dedo: guidance for coding agents

This is HighLevel commerce's guidance for coding agents working on a service built with dedo. It comes from [commerce-dedo-rules](https://gohighlevel.github.io/commerce-dedo-rules/), whose `annexure.md` a service names in its `dedo.yaml`. Where it conflicts with dedo's guidance above, follow this. Where it says nothing, dedo's guidance holds.

## The rules come from the platform

- The `HOUSE_*` rules make a contract produce an API that passes HighLevel's `public_api_rules` quality gate. When a house rule and an AIP, ZALANDO or DEDO rule disagree, the house rule is the decision. Never `disable` a house rule or override it in the service to get a lint clean; change the proto. The [rules' documentation](https://gohighlevel.github.io/commerce-dedo-rules/), linked from each finding, explains each rule.
- Commerce departs from the gate in a few agreed places, listed in the rules' README under *Where commerce departs from the gate*: the major version in the path after one root per service (`/commerce/<service>/v1/...`), and AIP-136 `:verb` custom methods. Do not "fix" those to what the gate reports.
- The location (tenant) comes from the caller's token, or for a user token from the `location-id` header that IAM verifies, never from the URL. Do not add `DEDO_MULTI_TENANT` to `use`, and do not put `locations/{id}` in a path: `HOUSE_HTTP_PATH_NO_LOCATION` forbids it. The service then keeps every call inside that location itself: see *The token's tenant scopes every call*.

## Every service carries commerce-auth's and commerce-obs's annexures

Every commerce service declares its auth with commerce-auth (`HOUSE_TENANT_DECLARED`) and its observability with commerce-obs (`HOUSE_OBS_TIER_DECLARED`), so it needs both of their annexures, not only this one: commerce-auth's says how to wire `iamguard` and gate with conditions, and commerce-obs's how to choose a tier, a route and dependency reasons and generate the alerts. Nothing checks that `dedo.yaml` names them, so you do. Whenever you work in a commerce service, read the `annexure` list in its `dedo.yaml`, and if either is missing:

1. Open its index, [commerce-auth](https://gohighlevel.github.io/commerce-auth/) or [commerce-obs](https://gohighlevel.github.io/commerce-obs/), and copy the newest release's `url` and `sha256` exactly as listed. Never compute or guess a `sha256`: dedo checks it against the published file.
2. Add each entry after this annexure's, commerce-auth's first:

   ```yaml
   annexure:
     - url: https://gohighlevel.github.io/commerce-dedo-rules/<tag>/annexure.md
       sha256: <from commerce-dedo-rules' index>
     - url: https://gohighlevel.github.io/commerce-auth/annexure/<tag>/annexure.md
       sha256: <from commerce-auth's index>
     - url: https://gohighlevel.github.io/commerce-obs/annexure/<tag>/annexure.md
       sha256: <from commerce-obs's index>
   ```

3. Run `dedo upgrade`, which adds them to `CLAUDE.md` and `AGENTS.md` after this one, then read them and follow them.
4. Commit `dedo.yaml`, `CLAUDE.md` and `AGENTS.md` together, and say in the pull request that you added them.

Never remove either one. Move one to a newer release only when asked to.

## The public spec is in docs/public

The `public_api_rules` gate checks only the OpenAPI documents under `docs/public/` that a pull request changes (`docs/how-the-gate-runs.md` in GoHighLevel/commerce-dedo-rules traces how). A service's public reference lives there, so the gate checks every contract change:

- Point `protoc-gen-dedo-openapi` at `docs/public/<root>/<root>.json`, where `<root>` is the service's path root (`/commerce/<root>/v1/...`): `out: docs/public/<root>` with `filename=<root>.json`, `server=https://services.leadconnectorhq.com` and `min_audience=EXTERNAL_PUBLIC`. A second output, such as `docs/openapi.json` with a local `server`, can stay for local tools.
- Regenerate it in the same pull request as the `.proto` change and commit it: the gate never sees a contract change whose spec is not in the diff, and it compares the spec with the merge base for breaking changes (gate rules 32–42), which `buf breaking` does not cover for REST paths.
- Until the gate's exception framework lands, it still reports the agreed departures (the version in the path, rule 9, and `:verb` custom methods, rule 7) on every pull request that changes the spec. Leave them: they are deliberate. Fix every other finding in the proto, never in the generated file.
- The same document is what HighLevel's developer docs publish (GoHighLevel/api-documentation, `specs/apps/v4/<root>-v4.json`): the API CoE's *How to Deploy API Documentation on Staging* in platform-docs has the steps.

## The token's tenant scopes every call

Tenancy is a chain: the platform, an agency (company), a location. Tenants are absolutely isolated: no call reads or writes another tenant's data, and there is no override. dedo's guidance says a multi-tenant service checks the tenant of every referenced name. dedo's runtime does that only for names that carry the tenant (`locations/{id}/...`), which commerce forbids, so with the tenant in the token nothing in dedo does it. Every commerce service guarantees it in storage:

- **Every resource has one owner, at one level of the chain.** A location owns its merchant data, such as tax rates. An agency owns its agency features, such as the configuration it sets for its locations. The platform owns what everyone shares, such as jurisdictions or tax codes. Ownership is the service's business logic, not the contract's: the proto declares which scopes a method needs, and the table a resource lives in, with its tenant column, says which level owns it.
- **A token changes only what its own level owns.** A location token writes its location's resources. An agency token writes its agency's resources; to change a location's resource it mints a token for that location and makes the call as the location. The platform writes platform resources through a migration or a method no agency or location token can call. The platform is a tenant like the others, not a superuser: it has no access to an agency's or a location's data.
- **Reads and references point up the chain, never down or across.** A location reads and references its own resources, its agency's and the platform's. An agency reads and references its own and the platform's. A platform resource names no agency or location resource, and an agency resource names no location resource. Something a tenant owns is never shared sideways; if it has to be shared, the level above owns it.
- **Storage scopes every read and write, not each query.** Every table holds one level's resources. A location's table has a `location_id` that is never null, an agency's a `company_id`, and every key and unique constraint starts with that column, so IDs are per tenant. On Postgres, enable and force row-level security on the table, with a policy on a transaction setting:

  ```sql
  ALTER TABLE taxes.tax_rates ENABLE ROW LEVEL SECURITY;
  ALTER TABLE taxes.tax_rates FORCE ROW LEVEL SECURITY;
  CREATE POLICY tenant ON taxes.tax_rates
    USING (location_id = current_setting('app.location_id'))
    WITH CHECK (location_id = current_setting('app.location_id'));
  ```

  An agency's table lets its locations read and only the agency write. A location's call sets `app.location_id`, while an agency's call leaves it empty:

  ```sql
  CREATE POLICY agency_read ON taxes.agency_settings FOR SELECT
    USING (company_id = current_setting('app.company_id'));
  CREATE POLICY agency_write ON taxes.agency_settings FOR ALL
    USING (company_id = current_setting('app.company_id') AND current_setting('app.location_id') = '')
    WITH CHECK (company_id = current_setting('app.company_id') AND current_setting('app.location_id') = '');
  ```

  An agency's call then sees no location's rows, since no `location_id` is empty. Platform tables have no tenant column, and the service's role may only read them. Run each RPC's statements in one transaction that first sets the tenant from the token (a location token gives its location and that location's agency, an agency token its agency and an empty location) and switches to a role that owns no tables, since owners and superusers skip the policy:

  ```go
  // set_config(..., true) is SET LOCAL: both end with the transaction, so a
  // pooled connection never carries a tenant to the next caller.
  _, err := tx.Exec(ctx,
  	"SELECT set_config('app.location_id', $1, true), set_config('app.company_id', $2, true), set_config('role', $3, true)",
  	location, company, appRole)
  ```

  A query that forgets `WHERE location_id = ...` then still sees only the caller's rows, and a write for another tenant fails. Reach the database only through the function that opens this transaction. A store without row-level security, such as MongoDB or Firestore, gets the same from one repository layer that adds the tenant from the context to every filter and every document it writes; handlers never build a query or document themselves.
- **A referenced name resolves in the caller's tenant.** Look up a resource a request names, such as `tax_zone: "tax-zones/eu"` on a new tax rate, inside the same scoped transaction, or through a foreign key that includes `location_id`. Another tenant's resource is then not found. Return `NOT_FOUND` for it, never `PERMISSION_DENIED`, so the reply doesn't reveal that it exists elsewhere. The same holds for `Get`, `Update` and `Delete` on another tenant's name.
- **No tenant, no storage.** A call that reaches storage without a tenant is refused, never run unscoped. The auth interceptor should already have rejected it.
- **Nothing crosses tenants: no RPC, no job.** Work over many locations runs once per location, each in that location's scoped transaction, so it runs in parallel and a failure retries one location alone. Work a write will need later, such as a reaper, is scheduled by that write, as a task that carries its location, rather than found by a query across locations. A sweep over every location takes the list from the platform's registry of locations. Support acts with a token for one location. What seems to need two tenants at once either belongs to the level above or runs once per tenant.
- **The superuser is infrastructure, outside the application.** Migrations and operations run as the database owner, never through the API and never from service code. Analytics reads an export, not the service.
- **Tests prove it.** Each service tests that, after writing as location A, location B reads, lists, updates and deletes nothing of A's, gets `NOT_FOUND` when it references one of A's names, and fails to insert a row for A; that a location reads its agency's and the platform's resources and can change neither; and that an agency token changes no location's resource. It also tests that a pooled connection carries no tenant after a transaction ends.

commerce-taxes-next does this in `internal/shared/db`: `InTenantTx`, the `platform.tenant_table` migration helper, and their tests in `internal/shared/db/db_test.go`. `app/migrate/schema_test.go` checks every migration's tables against these rules. Start from it instead of writing it again.

## Vendors stay behind the adapter

dedo's guidance puts a vendor's model in an adapter inside the service. For commerce:

- No field is named after a vendor, such as `avalara_document_code`: name it for what it means to commerce. Comments may name vendors.
- An enum value may name the vendor when it records provenance, such as `ENGINE_AVALARA` for the engine that calculated a tax. Provenance says where a result came from. The vendor's own states, errors and options are still translated in the adapter.
- The vendor's identifiers, and any mapping from commerce's identifiers to them, stay in the adapter. The exception is a vendor so dominant in its domain that its code system is the domain's language: commerce then publishes that code system as its own identifiers, and needs no mapping. Avalara's tax codes are that case (`tax-codes/DA060000`).

## Every call carries a Version header

Platform IAM (`go-platform-core/iam`) rejects a request without a `Version` header: a date such as `2021-07-28`, or `v3` and later. That holds for calls that reach the service in-process too, MCP tool calls among them.

Commerce services document and send **`v4`**, the version HighLevel's developer docs publish new APIs under (GoHighLevel/api-documentation, `specs/apps/v4/`). Declare the header once, in a header file that `protoc-gen-dedo-openapi` adds to every operation (`headers=docs/headers.json`, dedo v0.42.0 or later), in the form the developer docs' v4 specs use:

```json
{
  "headers": [
    {
      "name": "Version",
      "description": "API Version. Send `v4`.",
      "required": true,
      "schema": {"type": "string", "enum": ["v4"], "example": "v4"},
      "example": "v4"
    }
  ]
}
```

Pass it to both outputs, the local `docs/openapi.json` and the public `docs/public/<root>/<root>.json`, and send `Version: v4` everywhere the service sets one itself: the MCP default below, a frontend's transport, smoke tests and curl examples.

dedo's MCP server forwards only `Authorization` and `X-Flow-ID` to the service unless told otherwise, and MCP clients rarely send `Version`. Wire both halves in `main.go`, with dedo v0.20.0 or later:

```go
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

client := connect.NewClient(connectinprocess.New(server), apiVersion("v4"))
err := mcpserver.AddToolsWith(tools, mcpserver.Options{
	Client:         client,
	ForwardHeaders: []string{"Version", "location-id"}, // a caller's own Version wins
}, <pkg>.<Service>MCPTools()...)
```

Use the version the service's REST API documents, `v4`, not a new one per call site. Any other client the service builds to call itself in-process needs the same interceptor.

## Auth is declared in the contract

A service states its auth posture in its protos with the `commerce.auth.v2` options from `buf.build/lc-commerce/commerce-auth`, and IAM's config and the runtime guard are generated from them. The rules' `docs/auth.md` explains the design; these are the rules to follow, and commerce-auth's own annexure, which every service carries, says how to wire `iamguard`.

- **Every method is an API, authorized by IAM.** There are no internal services, no anonymous or public methods and no service identity: every caller presents a credential IAM understands, and every service is `AUDIENCE_EXTERNAL_PUBLIC`, except a backend for frontends (a service on `/commerce/<root>/bff/v<n>/...` paths), which is `AUDIENCE_COMPONENT_INTERNAL` (`HOUSE_AUDIENCE`). The audience says where a service is published, not who may call it. Never write an endpoint that skips authorization, and never authorize on `channel: ISTIO_MESH`, `istio-workload-name` or `source-id`: they attribute a call, they do not prove who made it.
- **One API scope per method**, in `(dedo.v1.method).scopes`: kebab-case, `<root>/<collection>.<readonly|write>`, named after the method's own path (`/commerce/taxes/v1/tax-rates` takes `taxes/tax-rates.readonly` or `.write`; on a BFF path, `/commerce/taxes/bff/v1/...`, the scope of the backend call behind it). Platform IAM maps each route to exactly one scope. Never add a second.
- **Every service declares its tenant**, `(commerce.auth.v2.service).tenant`: `TENANT_LOCATION`, `TENANT_AGENCY` or `TENANT_PLATFORM`.
- **Permissions bundle scopes** for Team Management, in `(commerce.auth.v2.file).permissions`: an `id` such as `taxes/rates.manage`, a `label`, a `module`, the `features` it needs, the API scopes it `grants`, and its `seed_roles`. Never change a released permission's `grants`: users granted it explicitly would never receive the new scope (`HOUSE_PERMISSION_GRANTS_SAME`). Add a new permission instead. Keep `seed_roles` conservative and leave out `ROLE_ACCOUNT_USER` unless every sub-account user may have it.
- **The documents are right when written.** Compute derived state in the request that changes a document, react to other services through the events their methods declare in `emits`, and keep time-based work in the owning service, scheduled by the write. No service calls another without a user behind the call.
- **A dependent call forwards the caller's token** in `x-forwarded-authorization`, not `Authorization`, and the dependent service authorizes it against its own scope. Declare the calls a method makes in `(commerce.auth.v2.method).calls`, by full name, and import the file that declares each one (with `// buf:lint:ignore IMPORT_USED` and the call it serves, when no type of it is used), so the house rules can check that every permission granting the method's scope also grants the dependency's.
- **Callers without a GHL account**, such as a buyer on a payment link, need a platform-issued client session, which commerce has proposed to platform with commerce-auth's `clientsession` as the reference implementation. Until the platform supports it, do not serve such callers.
- **Authorize through IAM once per request**, in one server interceptor ahead of dedo's: decode the token's claims to choose the `authClass` and its id, call AuthZ, and check the verified token in the reply matches. Build the route key from the proto, the same on every transport. Do not cache decisions. Hand the method's scope to dedo's `Scopes` once IAM allows. Return `Unauthenticated` for a missing or bad token, `PermissionDenied` for a denial and `Unavailable` when IAM cannot be reached, and fail closed.
- **A permission's `features` are not enforced by IAM.** Gate a capability that only some tenants have, such as an agency plan or a location feature, with conditions instead: list their ids in the permission's `requires`, take shared ones such as `founders` from `commerce/auth/v2/conditions.proto`, and declare `{reason: "CONDITION_NOT_MET" code: PERMISSION_DENIED}` on every method the permission gates. commerce-auth's `iamguard` enforces them; write no gate in a handler, and no refusal reason of the service's own (the rules' `docs/auth.md`, *Conditional permissions*).
- **Do not use `auth.v1`** (`guard_profile` and the rest). It is deprecated.

## Dependencies

dedo's [dependencies.md](https://github.com/GoHighLevel/dedo/blob/main/guidance/dependencies.md) says how to qualify and pin a dependency. These are HighLevel's licenses and commerce's choices on top of it.

- **Licenses**, HighLevel's policy for every dependency, including every module it brings with it:

  | License | Policy |
  | --- | --- |
  | MIT, BSD, ISC | Allowed |
  | Apache 2.0 | Allowed when we consume the package, as a library linked into a service is. Not allowed when we would resell it directly or compete with it: stop and ask. |
  | LGPL, MPL | Needs Legal review: stop and ask, and don't add it until Legal approves. Record the approval in the pull request. |
  | GPL, AGPL | Blocked, even when another dependency brings it in |
  | Anything else, or a license the tools can't detect | Not listed in the policy, so treat it as needing Legal review |

  Check a service's dependencies with:

  ```sh
  go run github.com/google/go-licenses/v2@v2.0.1 check ./... --allowed_licenses=MIT,BSD-2-Clause,BSD-3-Clause,ISC,Apache-2.0 --ignore github.com/GoHighLevel
  ```

  Apache 2.0 is on that list because a service consumes its libraries. A failure names the module and its license: remove the module or take it to Legal; never widen the list to pass.
- **No go-platform-core.** Commerce services use upstream libraries and call platform services directly: OpenTelemetry's SDK, `log/slog`, and iam-backend through commerce-auth's `iamguard`. They never depend on `github.com/GoHighLevel/go-platform-core`. commerce-auth's `iambackend` Authorizer calls iam-backend directly; its deprecated `platformiam` adapter, which wraps go-platform-core, is not for new code.
- **No thin wrappers from the platform either.** A shared package that only renames or re-exports a library, or wraps one service call, is a dependency to avoid, wherever it comes from. Copy the few lines of glue into the service, which has an owner.

## Observability is declared in the contract

A service states how it is watched in its contract, and its alerts are generated from it by commerce-obs: vmalert rule files for the platform's alert tree today, and HyperDX alerts once HyperDX can fire them. dedo's options say what it promises and which errors count, and the `commerce.obs.v1` options from `buf.build/lc-commerce/commerce-obs` add what is commerce's. Never write an alert by hand for a commerce service, and never edit a generated one: change the contract and generate again, or the alert drifts from what the service promises. commerce-obs's own annexure, which every service carries (*Every service carries commerce-auth's and commerce-obs's annexures*), says how to choose a tier, a route and dependency reasons, and how to generate the alerts.

- **Every service declares its tier**, `(commerce.obs.v1.service).tier`: `TIER_1` for money paths (checkout, payments, invoicing, tax), which page on both of dedo's burn rates; `TIER_2` for what a merchant notices within minutes; `TIER_3` for the rest, which never pages.
- **Every service names its alert route**, `(commerce.obs.v1.service).alert_route`: the kebab-case name of the HyperDX webhook its alerts notify, such as `commerce-taxes-oncall`. Never a URL or a token: the webhook holds those.
- **Every service states its objectives** in `(dedo.v1.service).service_level`, with an availability of at least its tier's floor: 0.999 for `TIER_1`, 0.995 for `TIER_2`, 0.99 for `TIER_3` (`HOUSE_OBS_TIER_MIN_AVAILABILITY`). `dedo slo` writes them as OpenSLO, and commerce-obs turns that into alerts.
- **Say which errors count in dedo.** A call is a server error, and spends the budget, when its code means the server failed (`UNKNOWN`, `DEADLINE_EXCEEDED`, `UNIMPLEMENTED`, `INTERNAL`, `UNAVAILABLE`, `DATA_LOSS`). Override that per declared reason with `(dedo.v1.method).error_classifications`.
- **Mark a dependency's failures**, in `(commerce.obs.v1.method).dependency_reasons`: the declared reasons whose errors are a vendor's, such as a tax engine that is down. They are taken out of the service's budget and raise an alert of their own. Return such a failure under a reason of its own, such as `TAX_ENGINE_UNAVAILABLE`, rather than a bare `UNAVAILABLE`: only a reason can be marked.
<!-- dedo:end annexure:https://gohighlevel.github.io/commerce-dedo-rules/v0.17.0/annexure.md -->

<!-- dedo:begin annexure:https://gohighlevel.github.io/commerce-auth/annexure/v0.1.0/annexure.md (dedo init and dedo upgrade rewrite this block: change its source, not this copy) -->
# commerce-auth: guidance for coding agents

This is guidance for coding agents working on a commerce service that uses commerce-auth: the `commerce.auth.v2` options (`buf.build/lc-commerce/commerce-auth`), `protoc-gen-commerce-iam` and the `iamguard` runtime guard (Go module `github.com/GoHighLevel/commerce-auth/go`). It adds to commerce's own annexure from commerce-dedo-rules. That annexure's *Auth is declared in the contract* and *The token's tenant scopes every call* sections set the rules: a tenant per service, one scope per method, permissions that never change their grants, no anonymous or service-to-service methods, and storage scoped to the token's tenant. Follow them. This annexure says how to meet them with commerce-auth. The rules' `docs/auth.md` explains the design.

## The contract decides, handlers don't

- Declare auth only in the `.proto` files: `(commerce.auth.v2.service).tenant` on every service, the one scope in `(dedo.v1.method).scopes`, `(commerce.auth.v2.method).calls` on a method that calls another service, and `permissions` and `conditions` in `(commerce.auth.v2.file)`. Import `commerce/auth/v2/options.proto`. Keep `buf.build/lc-commerce/commerce-auth` in `buf.yaml` at the release that matches the `github.com/GoHighLevel/commerce-auth/go` version in `go.mod`, because the generator and the guard read the options that release defines.
- Don't write auth code in a handler: no scope check, no token parsing, no tenant, plan or feature gate that refuses a call. The guard and dedo's interceptor enforce the contract. A check in a handler is invisible to IAM's config, Team Management, the API reference and the MCP tools.
- There is no anonymous or service-only method to declare. `MethodRules` fields 1 (`anonymous`) and 2 (`callers`) are reserved. Don't try to work around that with an unscoped route, an HTTP handler outside the `connect.Server`, or trust in `channel: ISTIO_MESH`, `source-id` or `istio-workload-name`. The guard refuses a call that claims the mesh or `CLOUD_TASK` channel.
- Don't use `auth.v1` (`guard_profile` and the rest). It is deprecated.

## Gate a capability with a condition

When only some tenants may use a capability, such as one plan's perk or a location feature, declare that on the permission. Don't check it in code:

- Name conditions in the permission's `requires`. Every listed condition must hold, and a condition holds when any of its values matches. Take a shared condition, such as `founders`, from the catalog: `import "commerce/auth/v2/conditions.proto";`, preceded by `// buf:lint:ignore IMPORT_USED` and a comment that names the condition. Never redeclare a catalog condition. If a second service needs one of your conditions, move it to the catalog with a pull request to commerce-auth.
- Declare a condition of your own in `conditions`, in the same file as the permission that requires it. Give it a kebab-case `id` that no other service would take, a plain-English `label` (the docs show only the id and label), values of exactly one kind (`agency_plans` or `location_features`) and `on_unknown`. Use `ON_UNKNOWN_ALLOW` for a commercial limit and `ON_UNKNOWN_DENY` for a boundary that protects data. Don't use `agency_features`: no platform API serves it yet, so `on_unknown` decides every call. An `agency_plans` value must be a tier (`starter`, `unlimited`, `pro`) or a platform plan name (`agency_<interval>_<price>`). Nothing else ever matches.
- The guard gates per scope, because tokens carry scopes, not permission ids. Every permission that grants a scope must require the same conditions. To gate only some of a permission's scopes, split it into new permissions, since a released permission's `grants` never change. Keep a conditioned permission in the same file as the methods whose scopes it grants: the guard reads the permissions of the files it serves. Adding or changing `requires` doesn't change `grants`.
- Every method whose scope is conditioned declares `errors: {reason: "CONDITION_NOT_MET" code: PERMISSION_DENIED}` in `(dedo.v1.method)`. That is the only refusal reason for a plan or feature gate. Don't invent one for the service.
- A permission's `features` only hide it in Team Management. Neither IAM nor the guard enforces them, so don't rely on `features` for gating.
- To *limit* by plan rather than refuse, such as a quota per tier, use `iamguard.NewAgencyPlans` with a `platformplan` source, from the handlers that need it. `Lookup` never fails. An unknown plan has `Known` false and `Excludes` false, so the limit fails open. Never return the plan to the caller.

## A dependent call forwards the caller's token

- Name each RPC a method calls in `(commerce.auth.v2.method).calls`, in full (`commerce.payments.v1.PaymentsService.CreatePayment`). Add the dependency's module to `buf.yaml`'s `deps` and import its file. A permission that grants the method's scope must also grant the scope of each call. Otherwise a user who holds only that permission is refused halfway through.
- On a Connect client, add `iamguard.ForwardToken()` to its interceptors. On a `net/http` request, call `iamguard.SetForwardedAuthorization(ctx, req.Header)` and fail the call when it returns false. Name the tenant yourself (`iamguard.HeaderLocationID` or `iamguard.HeaderCompanyID`). Never copy `Authorization`, never set `x-forwarded-authorization` by hand, and never log either header.
- Only an enforcing guard gives a handler a token to forward. In audit mode, and for a client session, nothing is forwarded. Keep a service's existing outbound credentials until it enforces.
- commerce-auth's guard reads tenant facts for conditions over the mesh, without a token. That exception belongs to commerce-auth alone. Service code never sends mesh headers and never calls a platform service as itself.

## Wire the guard ahead of dedo's

The guard replaces the `auth` interceptor, `scopesFromContext` and `devTokens` that `dedo init` writes into `main.go`:

```go
cfg, err := iambackend.ConfigFromEnv(os.Getenv) // IAM_BACKEND_URL, IAM_BACKEND_TIMEOUT
authz, err := iambackend.New(cfg)
iam, err := iamguard.New(iamguard.Config{
	Services:   services, // every protoreflect.ServiceDescriptor this server serves
	Authorizer: authz,
	Facts:      facts, // *iamguard.FactConfig: required once a served scope is conditioned
})
guard, err := interceptor.New(interceptor.Config{Table: table, Scopes: iamguard.Scopes, EnforceScopes: true, Log: log})
server := connect.NewServer(flowid.Interceptor(), observe, iam.Interceptor(), guard, validate.NewServerInterceptor())
```

- Use the `iambackend` Authorizer. `platformiam` is deprecated: it is the go-platform-core adapter that commerce's annexure allowed only until `iambackend` existed. Never import go-platform-core.
- `iamguard.New` fails on an invalid contract or missing `Facts`. Let startup fail and fix the proto. Never skip a service to get it to start.
- In `iamguard.FactConfig`, read `CompaniesURL` and `LocationsURL` (for `agency_plans`) and `FeaturesURL` (for `location_features`) from configuration. Never hard-code a host. `SourceID` is the service's name and is required. Count `OnDecision` outcomes, especially `unknown_allowed`, so an outage shows up.
- `Config.Conditions` overrides a condition's values by id from configuration, such as when a plan joins the Founders. It never changes a condition's kind, and an id that the contract doesn't declare fails `New`.
- Leave `Config.Sessions` nil. Client sessions (`clientsession`, `examples/payer-links`) are a proposal to platform, not a production path.
- Every transport, MCP tool calls included, goes through these server interceptors. Don't add HTTP middleware for auth, and don't cache decisions.
- Keep the guard's errors as they are: `Unauthenticated`, `PermissionDenied`, `InvalidArgument` (no tenant named), `Unavailable` and `Internal`. Never list `IAM_UNAVAILABLE` in `dependency_reasons`: commerce-obs already counts it as a dependency failure on every method.

## Use what IAM verified

- `iamguard.FromContext(ctx)` returns the `Caller`. `Caller.Target` is the tenant to scope storage to, and `Caller.Principal` is the actor to record on a change. Never take a tenant or an actor from the request body.
- `iamguard.Company(ctx)` returns the agency that IAM verified. It is often empty for a location's caller. The service decides what an empty agency means, and never reads it from a `company-id` or `location-id` header instead.

## Generate IAM's config from the contract

- Run `protoc-gen-commerce-iam` from the service's buf generation with `strategy: all` and `opt: out=iam-config.json`. Install it at the same version as the `github.com/GoHighLevel/commerce-auth/go` requirement in `go.mod`, never `@latest`. To document conditions in the API reference and the MCP tools, add `access=access.json`, generate that file in a run of its own before dedo's generators, and pass it to them with `access=` (the rules' `docs/auth.md`, *Documenting conditions*). Never edit the generated files.
- The plugin fails the build on a contract that IAM or the guard can't honour. Fix the proto. It also prints warnings, listed under `warnings` in `iam-config.json`: a scope that no permission grants, a permission missing a call's scope, or a plan value that can't match. Resolve each warning, or explain it in the pull request.
- Route keys are `POST /commerce/<root>/<package.Service>/<Method>`. Renaming a package, service or method, or changing the path root, moves a key. Register every new key with IAM (`commerce-iam-render` renders the platform-backend config) before the service deploys. Otherwise IAM refuses the route with `E-101`.

## Roll out an existing service in audit mode

Before an existing service enforces, build an auditor with `iam.NewAuditor(iamguard.AuditConfig{Report: ..., Sample: ..., Async: true})` and put `auditor.Interceptor()` after the service's current auth. It evaluates each sampled call exactly as the guard would and never refuses one. Count records where `!r.Evaluated()` apart from would-be refusals. Switch to `iam.Interceptor()` once nothing unexpected would be refused and every route is in IAM's config.

## Verify

- Run `make check` (it runs the house rules on the options) and `make generate`, then commit the regenerated output with the `.proto` change.
- Test the server as `main.go` builds it, so `iamguard.New` validates the contract. Use a fake `iamguard.Authorizer` that returns `iamguard.Decision{Allow: true, Token: &iamguard.Token{AuthClass: "Location", AuthClassID: "loc-1"}}`, and send an opaque `Authorization: Bearer` token with `location-id: loc-1`. Then assert:
  - no token gets `Unauthenticated`;
  - a denial gets `PermissionDenied`;
  - an `ErrUnavailable` from the Authorizer gets `Unavailable`;
  - for a conditioned scope, with `FactConfig`'s URLs pointed at an `httptest` server, a tenant without the condition gets `PermissionDenied` with reason `CONDITION_NOT_MET`.
- Test that each outbound client forwards the caller's token, and that it sends no token when the context has no `Caller`.
<!-- dedo:end annexure:https://gohighlevel.github.io/commerce-auth/annexure/v0.1.0/annexure.md -->

<!-- dedo:begin annexure:https://gohighlevel.github.io/commerce-obs/annexure/v0.1.0/annexure.md (dedo init and dedo upgrade rewrite this block: change its source, not this copy) -->
# Commerce observability on dedo: guidance for coding agents

This is HighLevel commerce's guidance for coding agents on how a commerce service built with dedo is watched. It comes from [commerce-obs](https://gohighlevel.github.io/commerce-obs/), whose `annexure.md` a service names in its `dedo.yaml` next to commerce-dedo-rules' annexure. That annexure's *Observability is declared in the contract* ([commerce-dedo-rules](https://gohighlevel.github.io/commerce-dedo-rules/)) says what every service declares: its tier, its alert route, objectives at its tier's floor, which errors count, and its dependencies' reasons. This one says how to get those right and how the alerts are made from them. It adds to that section and never replaces it: if the two ever disagree, stop and ask. Where this conflicts with dedo's guidance, follow this.

commerce-obs has not yet had a review by an owner of commerce observability. Follow this guidance, and flag what looks wrong rather than working around it.

## The contract is the only source of alerts

- A service's alerts come from two inputs: the SLOs `dedo slo` writes into `slo/` from `(dedo.v1.service).service_level`, and the `commerce.obs.v1` options in the contract. `commerce-obs rules` turns them into vmalert rule files for the platform's alert tree, the alerting path today. `commerce-obs alerts` and `apply` write HyperDX alerts, which cannot fire until HyperDX has a ClickHouse metric source.
- To change an alert, change the contract and generate again. Never edit `slo/` or a rule file that starts with `# Code generated by commerce-obs. DO NOT EDIT.`, and never write a vmalert rule or a HyperDX alert by hand for what the contract can declare.
- The windows and thresholds are dedo's burn-rate policies: page at 14.4x the budget over 1h, ticket at 6x over 6h. Commerce decides only how loudly they ring, through the tier. A 3-day or other extra policy belongs in dedo, not in a service.
- A service with a `service_level` but no `(commerce.obs.v1.service)` makes `commerce-obs rules` fail. A service without an availability objective has no availability alert, and fails `HOUSE_OBS_TIER_MIN_AVAILABILITY`.

## Choose the tier by the worst outage

The tier sets the platform severity of each of dedo's alerts in production:

| Alert | `TIER_1` | `TIER_2` | `TIER_3` |
| --- | --- | --- | --- |
| fast burn (dedo's page) | `p0` | `p1` | `p2` |
| slow burn (dedo's ticket) | `p1` | `p2` | `p2` |
| dependency burn | `p1` | `p2` | `p2` |

`p0` and `p1` page the on-call; `p2` never pages. A service that takes, computes or moves money is `TIER_1`, even if only one of its RPCs does. Never lower a tier, or an objective, to quiet an alert: that is the owners' decision, not a code change. Fix the cause, or ask.

## The alert route is a name

- `(commerce.obs.v1.service).alert_route` is the kebab-case name of a HyperDX webhook, at most 63 characters, such as `commerce-taxes-oncall`. commerce-obs refuses a contract whose route is missing or malformed, for every target, although the vmalert rules do not use it.
- Never put a URL, a token, a channel or an email address in it: every reader of the contract sees it. If one was ever committed, say so in the pull request so the secret is rotated; deleting it from the proto does not unleak it.
- The vmalert rules route by the platform's `team` and `sub_team`, which the pipeline passes as `-team` and `-sub-team`. They are not in the contract: do not derive them from `alert_route`, or from dedo's `owner_team`, which names a team in the team registry, not an on-call route.

## Mark a dependency's failures, and only those

- A dependency is something the service's own team cannot fix: a vendor such as a tax engine or a payment provider, or a platform service another team runs. The service's own database, cache or queue is not: when they fail, the service failed, and that spends its budget.
- Give each dependency's failure a reason of its own, such as `TAX_ENGINE_UNAVAILABLE`, declared in `(dedo.v1.method).errors` with a code dedo counts as a server error (`UNAVAILABLE` when it is down, `DEADLINE_EXCEEDED` when it timed out), and list it in `(commerce.obs.v1.method).dependency_reasons` on every method that returns it. The marking is per method: a reason listed on `CalculateTax` does not cover `PreviewTax`.
- Return it from the adapter that calls the dependency, with `connect.NewError` and a `google.rpc.ErrorInfo` detail carrying the reason. Any other error reaches the caller as `UNKNOWN` with no reason: it spends the service's budget, and no marking can move it.
- A reason dedo does not count (a client code, or `error_classifications` with `server_error: false`) spends no budget, so marking it only adds a dependency alert for calls that never failed. For a dependency's outage use `dependency_reasons`, never `server_error: false`: the classification hides the outage from every alert, while the dependency reason moves it to an alert of its own.
- Never invent a reason. List only reasons the method's code really returns for that dependency, never declare a reason just to list it, never reuse one dependency's reason for another, and never give the service's own failures a dependency's reason to protect its budget. Each reason is `UPPER_SNAKE_CASE`, declared, and listed once (`HOUSE_OBS_DEPENDENCY_DECLARED`, `HOUSE_OBS_DEPENDENCY_UNIQUE`); commerce-obs refuses the same.

## IAM_UNAVAILABLE is already a dependency

- When iam-backend, the platform's IAM service, does not answer, commerce-auth's `iamguard` fails the call with `UNAVAILABLE` and the reason `IAM_UNAVAILABLE`. commerce-obs counts it as a dependency's failure on every method of every watched service: out of the budget, into the dependency alert. Never list it in `dependency_reasons`.
- Never return `IAM_UNAVAILABLE` from the service's own code, for instance when another service is down: it would read as an IAM outage. Give that failure a reason of its own.
- Install `iamguard`'s interceptor where dedo's `main.go` puts `auth`: after dedo's telemetry interceptor and before its contract interceptor. Ahead of telemetry, refused calls are never measured, IAM outages included.

## Failures must count

- dedo decides what spends the budget: a call carries `error.type`, and is a server error, only for `UNKNOWN`, `DEADLINE_EXCEEDED`, `UNIMPLEMENTED`, `INTERNAL`, `UNAVAILABLE` and `DATA_LOSS`, unless `(dedo.v1.method).error_classifications` says otherwise for a declared reason. commerce-obs adds nothing to that; it only moves dependency reasons out of the budget.
- Never hide a failure so it does not count: no `OK` with an empty or partial result, no client code (`FAILED_PRECONDITION`, `NOT_FOUND`, `ABORTED`) for something the server failed to do, no error caught, logged and dropped. A failure that does not count pages no one.
- Classify a reason with `server_error: false` only when it is never a failure by design, such as a watch that ends with `UNAVAILABLE` on every deploy, and say why in the contract. Never classify to quiet an alert.
- Return a caller's mistake with a client code and a declared reason, so it does not count against the service.
- Keep dedo's telemetry: never remove its interceptor, and never set `OTEL_METRICS_EXPORTER=none` or `OTEL_SDK_DISABLED=true` where the service runs. The alerts count `rpc.server.call.duration` by `rpc.method`, and availability by `error.type` and `dedo.error.reason`. An RPC with `service_level: {exempt: true}` is in no alert: exempt only what cannot meet the objectives by design.

## From SLO to rule files

```sh
make generate                  # dedo slo writes slo/<service>.yaml
buf build -o image.binpb       # the contract, with the options it imports
commerce-obs rules -image image.binpb -slo slo -env prod \
  -team <team> -sub-team <sub_team> -out <checkout of the alert tree>
```

- `-env` has no default. `dev` is a sink: it builds and checks every rule and writes none. `staging` writes test alerts (`environment: itr`), all at `p2`, so they never page. `prod` writes `environment: prd` at the tier's severity. Run `prod` only for production's alert tree.
- Each alert is one file under `-out`, at `alerts/<team>/<sub_team>/custom/<AlertName>.yaml`, such as `TaxServiceAvailabilityFastBurn.yaml`. The files reach the platform by a pull request to its alert tree (`platform-shared-changes`), never by hand. commerce-obs never deletes a file: it prints `stale:` for each file it wrote earlier for the same services that this run no longer writes. Delete those in the same pull request.
- `skipped SLO` lines name a policy of dedo's that cannot fire for that SLO, such as either burn rate on a p50 latency objective. They are not errors; read them, and change the objective if the alert was wanted.
- For HyperDX, `commerce-obs alerts -image image.binpb -slo slo -source <metric source> -out alerts.json`, then `commerce-obs apply -manifest alerts.json -url <HyperDX API> -dry-run` before the same without `-dry-run`. The API key comes from `HYPERDX_API_KEY`, set from the pipeline's secrets: never in the repository, a flag or a log. `-prune` deletes the managed alerts of the manifest's services that it no longer has.

## Check it before you push

1. `buf lint` is clean, `HOUSE_OBS_*` included.
2. `make generate`, then read the diff of `slo/`: every objective that changed is one you meant to change.
3. `buf build -o image.binpb && commerce-obs rules -image image.binpb -slo slo -env dev` succeeds. It builds and checks every rule a production run would, and writes nothing.
4. A handler test for each dependency: when the dependency fails, the method returns the code and the `ErrorInfo` reason its `dependency_reasons` names. Another for each caller's mistake: a client code and a declared reason.
5. When the change adds or removes an SLO, an objective, a method or a dependency reason, the rule files change too: say so in the pull request, so the alert tree is regenerated with it.
<!-- dedo:end annexure:https://gohighlevel.github.io/commerce-obs/annexure/v0.1.0/annexure.md -->

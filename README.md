# Todo: tasks with deadlines and time tracking

One small service where tasks, their optional deadlines and the time spent on them stay consistent, for people through REST and for AI agents through MCP. Built with [dedo](https://github.com/GoHighLevel/dedo) and commerce's house rules ([dedo.yaml](dedo.yaml)).

## The domain

| Term | Meaning |
| --- | --- |
| **Task** | Something a person intends to do: a title, optional notes, a state (open or done) and an optional deadline. |
| **Deadline** | When a task should be done by. A task without one is never overdue. |
| **Overdue** | Derived on every read, never stored: an open task whose deadline has passed. |
| **Timer** | A running measurement of time on one task. A person has at most one. |
| **Time entry** | A finished, immutable record of time spent on a task. Stopping a timer writes one. |
| **Tracked time** | The sum of a task's time entries. |

Two modules, one bounded context (`commerce.todo`), with no edge between them in Go or SQL (`make modules`):

```
 ┌───────────────┐   commerce.todo.tasks.task.completed / .deleted   ┌────────────────────┐
 │ tasks         │ ────────────────────────────────────────────────► │ time_tracking      │
 │ (core)        │ ◄──────────────────────────────────────────────── │ (supporting)       │
 └───────────────┘   TaskService.GetTask, its published contract     └────────────────────┘
```

| | Tasks | Time Tracking |
| --- | --- | --- |
| Contract | [`proto/commerce/todo/tasks/v1`](proto/commerce/todo/tasks/v1/tasks.proto) | [`proto/commerce/todo/time_tracking/v1`](proto/commerce/todo/time_tracking/v1) |
| Services | `TaskService` | `TimerService`, `TimeEntryService` |
| Go | [`internal/tasks`](internal/tasks) | [`internal/time_tracking`](internal/time_tracking) |
| Postgres | schema `tasks`, role `tasks_app` | schema `time_tracking`, role `time_tracking_app` |

- **Tasks never knows Time Tracking exists.** `CompleteTask` and `DeleteTask` publish their events after the commit. A reaction in the composition root ([`app/reactions.go`](app/reactions.go)) stops a timer running on that task through Time Tracking's own API, as the caller, so exactly one time entry is written.
- **Time Tracking asks Tasks only through Tasks' contract.** It owns a port, `time_tracking.Tasks`, and the composition root implements it over `TaskService.GetTask` ([`app/tasks_acl.go`](app/tasks_acl.go)), forwarding the caller's token. The contract declares the call in `(commerce.auth.v2.method).calls`.
- **Every call is scoped to the caller.** commerce-auth's `iamguard` authorizes each call through IAM. Each module's queries run in one transaction as the module's own role, under row-level security on the caller's location and person ([`internal/shared/db`](internal/shared/db)).

`bff/` is the backend for the frontend in `frontend/`: it reads the board and a task's page from the API as the caller, and is paired with its frontend by commit SHA.

## Run it

Needs Go, buf ≥ 1.75 (the protovalidate `field_mask` example rules), podman (or docker), kind and kubectl.

```sh
make kind-all        # kind cluster + Postgres + service + BFF + frontend
make test            # go test ./..., integration tests on the cluster's Postgres (127.0.0.1:15432)
make check           # format, lint, breaking, modules, dedo vet, the BFF's generated code and tests
make docs            # the REST reference
```

| What | Where |
| --- | --- |
| App | http://localhost:8543 |
| REST reference | http://localhost:8542/docs |
| Connect, gRPC, REST, MCP | http://localhost:8542 (`/mcp`) |

Locally, outside kind: `AUTH_MODE=dev DATABASE_URL=postgres://postgres:postgres@127.0.0.1:15432/todo?sslmode=disable make run`.

### Development tokens

`AUTH_MODE=dev`, set only in the kind overlay, replaces iam-backend with [`devauth`](internal/shared/devauth): a fixed set of people, whose route-to-scope map comes from the generated IAM config. Every other environment sets `IAM_BACKEND_URL`, and the service refuses to start with neither.

| Token | Locations | Scopes |
| --- | --- | --- |
| `alice` | `loc-demo`, `loc-other` | all |
| `bob` | `loc-demo` | all |
| `reader` | `loc-demo` | read-only |

Every call sends `Authorization: Bearer <token>`, `location-id: <location>` and `Version: v4`:

```sh
curl -s localhost:8542/commerce/todo/v1/tasks -H 'Authorization: Bearer alice' -H 'location-id: loc-demo' -H 'Version: v4' \
  -H 'Content-Type: application/json' -d '{"title": "Write the report"}'
```

### Claude Desktop

`make kind-claude-entry KIND_CLAUDE_TOKEN=alice` prints the `mcpServers` entry: dedo's `mcp-remote` bridge with the `location-id` and `Version` headers. Add it to Claude Desktop's config and restart it. Then ask: "start a timer on my report task, then mark it done."

// Package devauth is a stand-in for platform IAM in local development and
// the kind cluster: an iamguard.Authorizer that knows a few people by an
// opaque token, the locations each belongs to and the scopes each holds,
// and maps a route key to its scope with the urlScopeMap protoc-gen-commerce-iam
// generates from the contract, exactly as IAM is configured from it.
//
// It is for development only. main.go uses it only when AUTH_MODE=dev and
// IAM_BACKEND_URL is not set; everywhere else the guard calls iam-backend.
package devauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/GoHighLevel/commerce-auth/go/iamguard"
)

// Person is one developer identity: a user who belongs to some locations
// and holds some scopes.
type Person struct {
	// User is the user's ID, which IAM's verified token names.
	User string `json:"user"`
	// Locations are the locations the user may act in.
	Locations []string `json:"locations"`
	// Scopes are the API scopes the user holds; "*" holds every scope and
	// "*.readonly" every read-only one.
	Scopes []string `json:"scopes"`
}

// DefaultPeople are the identities `make run` and the kind cluster accept,
// by bearer token. Every call also names its location in the location-id
// header, as a user's call to IAM does.
var DefaultPeople = map[string]Person{
	"alice":  {User: "alice", Locations: []string{"loc-demo", "loc-other"}, Scopes: []string{"*"}},
	"bob":    {User: "bob", Locations: []string{"loc-demo"}, Scopes: []string{"*"}},
	"reader": {User: "reader", Locations: []string{"loc-demo"}, Scopes: []string{"*.readonly"}},
}

// Authorizer decides calls as IAM would, from a static table.
type Authorizer struct {
	routes map[string]string // route key -> scope
	people map[string]Person // bearer token -> person
}

// New returns an Authorizer that maps route keys to scopes with iamConfig,
// the iam-config.json protoc-gen-commerce-iam writes, and knows people by
// their bearer tokens.
func New(iamConfig []byte, people map[string]Person) (*Authorizer, error) {
	var cfg struct {
		URLScopeMap map[string]string `json:"urlScopeMap"`
	}
	if err := json.Unmarshal(iamConfig, &cfg); err != nil {
		return nil, fmt.Errorf("devauth: iam config: %w", err)
	}
	if len(cfg.URLScopeMap) == 0 {
		return nil, errors.New("devauth: iam config has no urlScopeMap: run make generate")
	}
	return &Authorizer{routes: cfg.URLScopeMap, people: people}, nil
}

// Authorize implements iamguard.Authorizer.
func (a *Authorizer) Authorize(_ context.Context, req iamguard.Request) (iamguard.Decision, error) {
	token, _ := strings.CutPrefix(req.Header.Get("Authorization"), "Bearer ")
	person, ok := a.people[strings.TrimSpace(token)]
	if !ok {
		return iamguard.Decision{}, &iamguard.CredentialError{Code: "E-202", Message: "unknown development token"}
	}
	scope, ok := a.routes[req.RouteKey]
	if !ok {
		return iamguard.Decision{Allow: false, Code: "E-101", Message: "route is not in IAM's config: " + req.RouteKey}, nil
	}
	if req.Target.Class != "Location" || !slices.Contains(person.Locations, req.Target.ID) {
		return iamguard.Decision{Allow: false, Code: "FORBIDDEN", Message: person.User + " does not belong to " + req.Target.Class + " " + req.Target.ID}, nil
	}
	if !holds(person.Scopes, scope) {
		return iamguard.Decision{Allow: false, Message: "The token is not authorized for this scope: " + scope}, nil
	}
	return iamguard.Decision{
		Allow: true,
		Token: &iamguard.Token{
			Channel:     orDefault(req.Header.Get("channel"), "APP"),
			Source:      orDefault(req.Header.Get("source"), "WEB_USER"),
			AuthClass:   "User",
			AuthClassID: person.User,
		},
	}, nil
}

// Known reports whether token is a development token, for MCP_ACCESS=authenticated.
func (a *Authorizer) Known(token string) bool {
	_, ok := a.people[token]
	return ok
}

func holds(scopes []string, scope string) bool {
	for _, s := range scopes {
		if s == "*" || s == scope || (s == "*.readonly" && strings.HasSuffix(scope, ".readonly")) {
			return true
		}
	}
	return false
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

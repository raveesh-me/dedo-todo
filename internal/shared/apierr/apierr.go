// Package apierr builds the errors handlers return: a Connect error with a
// google.rpc.ErrorInfo detail carrying a reason the RPC declares in its
// dedo.v1.method errors.
package apierr

import (
	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connectproto"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
)

// Domain is the ErrorInfo domain of every error the service returns.
const Domain = "todo.commerce.example.com"

// New returns a Connect error with code, a google.rpc.ErrorInfo whose reason
// is reason, and message for the caller. reason must be one the RPC
// declares; dedo vet checks it at each call site.
func New(code connect.Code, reason, message string) error {
	err := connect.NewError(code, message)
	detail, derr := connectproto.NewErrorDetail(&errdetails.ErrorInfo{Reason: reason, Domain: Domain})
	if derr != nil {
		return err
	}
	return err.WithDetail(detail)
}

// Reason returns the ErrorInfo reason an error carries, or "".
func Reason(err error) string {
	var cerr *connect.Error
	if !asConnect(err, &cerr) {
		return ""
	}
	for _, d := range cerr.Details() {
		msg, err := connectproto.UnmarshalErrorDetail(d)
		if info, ok := msg.(*errdetails.ErrorInfo); err == nil && ok && info.GetReason() != "" {
			return info.GetReason()
		}
	}
	return ""
}

func asConnect(err error, target **connect.Error) bool {
	for err != nil {
		if c, ok := err.(*connect.Error); ok {
			*target = c
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

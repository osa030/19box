// Package connect provides Connect RPC service implementations.
package connect

import (
	"context"

	"connectrpc.com/connect"
	"github.com/osa030/19box/internal/infra/config"
)

const (
	// AdminTokenHeader is the header name for admin authentication token.
	AdminTokenHeader = "X-Admin-Token"
)

// NewAdminAuthInterceptor creates an interceptor that validates admin tokens
// from request metadata for AdminService methods.
func NewAdminAuthInterceptor(cfg *config.Config) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Extract token from metadata
			token := req.Header().Get(AdminTokenHeader)
			if token == "" {
				return nil, connect.NewError(connect.CodeUnauthenticated, nil)
			}

			// Validate token
			if token != cfg.Admin.Token {
				return nil, connect.NewError(connect.CodeUnauthenticated, nil)
			}

			// Call next handler
			return next(ctx, req)
		}
	}
}

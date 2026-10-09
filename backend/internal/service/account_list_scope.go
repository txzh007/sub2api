package service

import "context"

type accountListScopeKey struct{}

const (
	AccountListScopeGeneral        = "general"
	AccountListScopeImageProviders = "image_providers"
)

// WithImageProviderAccountList selects the separately managed image channels.
// It applies only to admin list queries; scheduler group queries are unaffected.
func WithImageProviderAccountList(ctx context.Context) context.Context {
	return context.WithValue(ctx, accountListScopeKey{}, AccountListScopeImageProviders)
}

func AccountListScopeFromContext(ctx context.Context) string {
	value, _ := ctx.Value(accountListScopeKey{}).(string)
	return value
}

func withDefaultAdminAccountListScope(ctx context.Context) context.Context {
	if AccountListScopeFromContext(ctx) != "" {
		return ctx
	}
	return context.WithValue(ctx, accountListScopeKey{}, AccountListScopeGeneral)
}

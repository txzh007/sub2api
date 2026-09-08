package service

import "context"

type externalImageToolContextKey struct{}

// WithExternalImageTool prevents the OpenAI adapter from injecting a second,
// upstream-hosted image tool while the gateway executes Gemini image calls.
func WithExternalImageTool(ctx context.Context) context.Context {
	return context.WithValue(ctx, externalImageToolContextKey{}, true)
}

func hasExternalImageTool(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(externalImageToolContextKey{}).(bool)
	return v
}

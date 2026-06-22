package template

import "context"

type rendererOptions struct {
	exprFunctions     map[string]RenderFunction
	templateFunctions map[string]RenderFunction
}

// RenderFunction is a caller-supplied helper callable during rendering.
type RenderFunction func(ctx context.Context, params ...any) (any, error)

// Option configures a renderer.
type Option func(*rendererOptions)

// WithExprFunction registers a command-specific Expr helper.
func WithExprFunction(name string, function RenderFunction) Option {
	return func(options *rendererOptions) {
		if options.exprFunctions == nil {
			options.exprFunctions = make(map[string]RenderFunction)
		}

		options.exprFunctions[name] = function
	}
}

// WithTemplateFunction registers a command-specific text/template helper.
func WithTemplateFunction(name string, function RenderFunction) Option {
	return func(options *rendererOptions) {
		if options.templateFunctions == nil {
			options.templateFunctions = make(map[string]RenderFunction)
		}

		options.templateFunctions[name] = function
	}
}

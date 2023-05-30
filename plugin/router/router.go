package router

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

var replArgRegex = regexp.MustCompile(`\{([^\}^\:]+):?([^\}^\:]*)\}`)

// DefaultNotFoundHandler is the handler
// that is used for handling requests when a handler can't be found for a given route.
// This can be overridden in the Router.
var DefaultNotFoundHandler HandlerFunc = func(
	ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender,
) {
	_ = sender.Send(&backend.CallResourceResponse{
		Status: http.StatusNotFound,
		Body:   []byte(fmt.Sprintf("no route match for path %s", req.Path)),
	})
}

// NewRouter returns a new Router
func NewRouter() *Router {
	return &Router{
		NotFoundHandler: DefaultNotFoundHandler,
		subrouters:      make([]*Subrouter, 0),
		routes:          make([]*RouteHandler, 0),
	}
}

// Use adds middlewares to the router.
func (r *Router) Use(middlewares ...middleware) {
	r.middlewares = append(r.middlewares, middlewares...)
}

// HandlerFunc defines the signatures handlers need to have in order to be used in this router.
type HandlerFunc func(context.Context, *backend.CallResourceRequest, backend.CallResourceResponseSender)

// Router is a simple request router specific to the grafana plugin SDK backend.CallResourceRequest HTTP calls.
// It allows the user to treat the grafana plugin backend as a traditional HTTP server,
// registering routes and using path parameters as normal.
type Router struct {
	// Handler called when there's no route match.
	NotFoundHandler HandlerFunc

	// Nested routers with a matching path.
	subrouters []*Subrouter

	// Routes to be matched.
	routes []*RouteHandler

	// Slice of middlewares to be called after a match is found.
	middlewares []middleware
}

// Subrouter is a slightly-extended router
// meant for being registered a with Subrouter() on either a Router or Subrouter.
type Subrouter struct {
	Router
	matcher      *regexp.Regexp
	pathArgNames []string
}

// Subrouter creates and returns a Subrouter for the given path prefix.
// All handlers registered with the Subrouter will have the prefix added implicitly.
func (r *Router) Subrouter(path string) *Subrouter {
	pathArgNames := make([]string, 0)
	// Look for path replacement vars ({...})
	matches := replArgRegex.FindAllStringSubmatch(path, -1)
	if len(matches) > 0 {
		// Check each match, replace it with regex, and add the match to our list of pathArgNames
		for _, match := range matches {
			if len(match) != 3 {
				continue
			}
			// Element 0 is the whole matched string which needs replacing in the path
			// Element 1 is the variable name
			// Element 2 is the match expression (may be empty, in which case it should be `([^\/]+)`)
			repl := `([^\/]+)`
			if match[2] != "" {
				repl = fmt.Sprintf("(%s)", match[2])
			}
			path = strings.ReplaceAll(path, match[0], repl)
			pathArgNames = append(pathArgNames, match[1])
		}
	}
	regex, err := regexp.Compile(fmt.Sprintf(`^%s`, path))
	if err != nil {
		// TODO
		return nil
	}

	sr := &Subrouter{
		Router: Router{
			NotFoundHandler: r.NotFoundHandler,
			subrouters:      make([]*Subrouter, 0),
			routes:          make([]*RouteHandler, 0),
		},
		pathArgNames: pathArgNames,
		matcher:      regex,
	}

	r.subrouters = append(r.subrouters, sr)
	return sr
}

// Handle registers a handler to a given path and method(s). If no method(s) are specified, GET is implicitly used.
func (r *Router) Handle(path string, handler HandlerFunc, methods ...string) *RouteHandler {
	// Normalize empty path to root.
	if path == "" {
		path = "/"
	}

	pathArgNames := make([]string, 0)
	// Look for path replacement vars ({...})
	matches := replArgRegex.FindAllStringSubmatch(path, -1)
	if len(matches) > 0 {
		// Check each match, replace it with regex, and add the match to our list of pathArgNames
		for _, match := range matches {
			if len(match) != 3 {
				continue
			}
			// Element 0 is the whole matched string which needs replacing in the path
			// Element 1 is the variable name
			// Element 2 is the match expression (may be empty, in which case it should be `([^\/]+)`)
			repl := `([^\/]+)`
			if match[2] != "" {
				repl = fmt.Sprintf("(%s)", match[2])
			}
			path = strings.ReplaceAll(path, match[0], repl)
			pathArgNames = append(pathArgNames, match[1])
		}
	}
	regex, err := regexp.Compile(fmt.Sprintf(`^%s$`, path))
	if err != nil {
		// TODO
		return nil
	}

	// Methods
	m := make(map[string]struct{})
	if len(methods) == 0 {
		m["GET"] = struct{}{}
	}
	for _, method := range methods {
		m[strings.ToUpper(method)] = struct{}{}
	}

	h := &RouteHandler{
		matcher:      regex,
		handleFunc:   handler,
		pathArgNames: pathArgNames,
		methods:      m,
	}

	r.routes = append(r.routes, h)

	return h
}

// RouteByName gets a RouteHandler by its name, if assigned.
// If multiple routes have the same name, the first registered one will be returned.
func (r *Router) RouteByName(name string) *RouteHandler {
	for _, h := range r.routes {
		if h.name == name {
			return h
		}
	}
	return nil
}

//nolint:lll
func (r *Router) getHandler(ctx context.Context, path string, method string, applyMiddlewares ...middleware) (context.Context, HandlerFunc) {
	// Check subrouters
	for _, h := range r.subrouters {
		if matches := h.matcher.FindStringSubmatch(path); len(matches) > 0 {
			for i := 1; i < len(matches); i++ {
				if i > len(h.pathArgNames) {
					break
				}

				ctx = CtxWithVar(ctx, h.pathArgNames[i-1], matches[i])
			}

			return h.getHandler(ctx, path[len(matches[0]):], method, append(applyMiddlewares, r.middlewares...)...)
		}
	}

	// Look for a matching handler
	for _, routeHandler := range r.routes {
		if _, ok := routeHandler.methods[method]; !ok {
			continue
		}

		if matches := routeHandler.matcher.FindStringSubmatch(path); len(matches) > 0 {
			for i := 1; i < len(matches); i++ {
				if i > len(routeHandler.pathArgNames) {
					break
				}

				ctx = CtxWithVar(ctx, routeHandler.pathArgNames[i-1], matches[i])
			}

			// handler found, apply middleware chain
			var handler HandlerFunc = routeHandler.handleFunc
			// middlewares attached to this router first
			for i := len(r.middlewares) - 1; i >= 0; i-- {
				handler = r.middlewares[i].Middleware(handler)
			}
			// middlewares from parent routers next
			for i := len(applyMiddlewares) - 1; i >= 0; i-- {
				handler = applyMiddlewares[i].Middleware(handler)
			}

			return ctx, handler
		}
	}

	return ctx, nil
}

// CallResource implements backend.CallResourceHandler, allowing the Router to route resource API requests
func (r *Router) CallResource(
	ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender,
) error {
	// Get the appropriate handler (if one exists)
	ctx, handler := r.getHandler(ctx, req.Path, strings.ToUpper(req.Method))
	if handler == nil {
		if r.NotFoundHandler == nil {
			return errors.New("no handler found for the request")
		}

		// Return not found
		r.NotFoundHandler(ctx, req, sender)
		return nil
	}

	handler(ctx, req, sender)
	return nil
}

// ListenAndServe hooks into the backend of the plugin SDK to handle and serve resource API requests
func (r *Router) ListenAndServe() error {
	return backend.Serve(backend.ServeOpts{
		CallResourceHandler: r,
	})
}

// RouteHandler is a Handler function assigned to a route
type RouteHandler struct {
	matcher      *regexp.Regexp
	name         string
	handleFunc   func(ctx context.Context, req *backend.CallResourceRequest, res backend.CallResourceResponseSender)
	pathArgNames []string
	methods      map[string]struct{}
}

// Methods sets the methods the handler function will be called for
func (h *RouteHandler) Methods(methods []string) *RouteHandler {
	m := make(map[string]struct{})
	for _, method := range methods {
		m[strings.ToUpper(method)] = struct{}{}
	}
	h.methods = m
	return h
}

// Name sets the name of the RouteHandler.
// Names should be unique for retrieval purposes, but uniqueness is not enforced.
func (h *RouteHandler) Name(name string) *RouteHandler {
	h.name = name
	return h
}

package k8s

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/logging"
	"github.com/grafana/grafana-app-sdk/resource"
)

const apisBasePath = "/apis"

// CustomRouteCaller is the subset of app.App that the CustomRouteHandler depends on. Any
// app.App implementation satisfies it, but tests and adapters can supply a narrower implementation.
type CustomRouteCaller interface {
	CallCustomRoute(ctx context.Context, w app.CustomRouteResponseWriter, r *app.CustomRouteRequest) error
}

// CustomRouteHandlerConfig is the configuration for a CustomRouteHandler.
type CustomRouteHandlerConfig struct {
	// Caller handles inbound requests via CallCustomRoute. An app.App satisfies this interface.
	Caller CustomRouteCaller
	// Manifest is validated at construction time and used by Register to install one
	// literal-plural pattern per kind (namespaced or cluster-scoped according to the kind's
	// Scope), plus two version-level fallback patterns per version. Only Group and
	// Versions[*].Kinds[*] (Kind, Plural, Scope) are read; admission and conversion fields are
	// ignored.
	Manifest app.ManifestData
}

// CustomRouteHandler installs ServeMux patterns that expose the custom routes of an app.App
// on the same URL grammar used by the aggregated Kubernetes API server:
//
//	/apis/<group>/<version>/<plural>/<name>/<subresource...>                   (cluster-scoped subresource)
//	/apis/<group>/<version>/namespaces/<ns>/<plural>/<name>/<subresource...>   (namespaced subresource)
//	/apis/<group>/<version>/<path...>                                          (cluster-scoped version-level)
//	/apis/<group>/<version>/namespaces/<ns>/<path...>                          (namespaced version-level)
//
// Inbound requests are translated into an app.CustomRouteRequest and dispatched via
// Caller.CallCustomRoute. Patterns are installed onto a parent http.ServeMux via Register.
type CustomRouteHandler struct {
	caller   CustomRouteCaller
	manifest app.ManifestData
}

// NewCustomRouteHandler creates a new CustomRouteHandler. Caller is required. The Manifest is
// validated for duplicate plurals; pattern registration itself is deferred to Register.
func NewCustomRouteHandler(cfg CustomRouteHandlerConfig) (*CustomRouteHandler, error) {
	if cfg.Caller == nil {
		return nil, errors.New("config.Caller is required")
	}

	// Validate that no group/version has two kinds with the same plural — that would create
	// duplicate ServeMux patterns at Register time, which would panic.
	registered := make(map[string]string)
	for _, v := range cfg.Manifest.Versions {
		for _, k := range v.Kinds {
			if k.Plural == "" || k.Kind == "" {
				continue
			}
			key := fmt.Sprintf("%s/%s/%s", cfg.Manifest.Group, v.Name, k.Plural)
			if existing, ok := registered[key]; ok {
				return nil, fmt.Errorf("duplicate plural %q for group %q version %q: kinds %q and %q",
					k.Plural, cfg.Manifest.Group, v.Name, existing, k.Kind)
			}
			registered[key] = k.Kind
		}
	}

	return &CustomRouteHandler{
		caller:   cfg.Caller,
		manifest: cfg.Manifest,
	}, nil
}

// Register installs the custom route handler on the provided mux.
func (h *CustomRouteHandler) Register(mux *http.ServeMux) {
	for _, v := range h.manifest.Versions {
		versionPath := fmt.Sprintf("%s/%s/%s", apisBasePath, h.manifest.Group, v.Name)

		for _, k := range v.Kinds {
			if k.Plural == "" || k.Kind == "" {
				continue
			}
			pattern := fmt.Sprintf("%s/namespaces/{namespace}/%s/{name}/{path...}", versionPath, k.Plural)
			if resource.SchemaScope(k.Scope) == resource.ClusterScope {
				pattern = fmt.Sprintf("%s/%s/{name}/{path...}", versionPath, k.Plural)
			}
			mux.HandleFunc(pattern, h.dispatchRoute(h.manifest.Group, v.Name, k.Kind, k.Plural))
		}

		// Version-level routes leave kind/plural empty.
		mux.HandleFunc(
			versionPath+"/namespaces/{namespace}/{path...}",
			h.dispatchRoute(h.manifest.Group, v.Name, "", ""),
		)
		mux.HandleFunc(
			versionPath+"/{path...}",
			h.dispatchRoute(h.manifest.Group, v.Name, "", ""),
		)
	}

	// Catch-all under /apis/ for any URL that didn't match the patterns above. Scoped to
	// /apis/ rather than / so it doesn't shadow other handlers the caller has installed.
	mux.HandleFunc(apisBasePath+"/", func(w http.ResponseWriter, r *http.Request) {
		writeStatusError(w, http.StatusNotFound, metav1.StatusReasonNotFound,
			fmt.Sprintf("path %q is not a valid /apis/<group>/<version>/... URL", r.URL.Path))
	})
}

func (h *CustomRouteHandler) dispatchRoute(group, version, kind, plural string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := resource.FullIdentifier{
			Group:     group,
			Version:   version,
			Namespace: r.PathValue("namespace"),
			Kind:      kind,
			Plural:    plural,
			Name:      r.PathValue("name"),
		}
		routeReq := &app.CustomRouteRequest{
			ResourceIdentifier: id,
			Path:               r.PathValue("path"),
			URL:                r.URL,
			Method:             r.Method,
			Headers:            r.Header,
			Body:               r.Body,
		}
		err := h.caller.CallCustomRoute(r.Context(), w, routeReq)
		if err == nil {
			logging.FromContext(r.Context()).Debug("custom route handler returned success",
				"method", r.Method, "path", r.URL.Path)
			return
		}
		logging.FromContext(r.Context()).Error("custom route handler returned error",
			"error", err, "method", r.Method, "path", r.URL.Path)
		writeError(w, r, err)
	}
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, app.ErrCustomRouteNotFound) {
		writeStatusError(w, http.StatusNotFound, metav1.StatusReasonNotFound,
			fmt.Sprintf("custom route %s %s not found", r.Method, r.URL.Path))
		return
	}

	code := int32(http.StatusInternalServerError)
	reason := metav1.StatusReasonInternalError
	var apiStatus apierrors.APIStatus
	if errors.As(err, &apiStatus) {
		st := apiStatus.Status()
		if st.Code != 0 {
			code = st.Code
		}
		if st.Reason != "" {
			reason = st.Reason
		}
	}
	writeStatusError(w, code, reason, err.Error())
}

func writeStatusError(w http.ResponseWriter, code int32, reason metav1.StatusReason, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(int(code))
	_ = json.NewEncoder(w).Encode(metav1.Status{
		TypeMeta: metav1.TypeMeta{Kind: "Status", APIVersion: "v1"},
		Status:   metav1.StatusFailure,
		Code:     code,
		Reason:   reason,
		Message:  message,
	})
}

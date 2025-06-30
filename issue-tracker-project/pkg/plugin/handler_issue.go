package plugin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/grafana/grafana-app-sdk/plugin"
	"github.com/grafana/grafana-app-sdk/plugin/router"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/backend/tracing"

	issue "github.com/grafana/issue-tracker-project/pkg/generated/issue/v1"
)

type IssueService interface {
	List(ctx context.Context, opts resource.StoreListOptions) (*resource.TypedList[*issue.Issue], error)
	Get(ctx context.Context, id resource.Identifier) (*issue.Issue, error)
	Add(ctx context.Context, obj *issue.Issue) (*issue.Issue, error)
	Update(ctx context.Context, id resource.Identifier, obj *issue.Issue) (*issue.Issue, error)
	Delete(ctx context.Context, id resource.Identifier) error
}

func (p *Plugin) handleIssueList(ctx context.Context, req router.JSONRequest) (router.JSONResponse, error) {
	ctx, span := tracing.DefaultTracer().Start(ctx, "issue-list")
	defer span.End()
	filtersRaw := req.URL.Query().Get("filters")
	filters := make([]string, 0)
	if len(filtersRaw) > 0 {
		filters = strings.Split(filtersRaw, ",")
	}
	svc, err := p.service.GetIssueService(ctx)
	if err != nil {
		log.DefaultLogger.With("traceID", span.SpanContext().TraceID()).Error("Error getting IssueService: "+err.Error(), "error", err)
		return nil, plugin.NewError(http.StatusInternalServerError, err.Error())
	}
	return svc.List(ctx, resource.StoreListOptions{Namespace: p.namespace, PerPage: 500, Filters: filters})
}

func (p *Plugin) handleIssueGet(ctx context.Context, req router.JSONRequest) (router.JSONResponse, error) {
	ctx, span := tracing.DefaultTracer().Start(ctx, "issue-get")
	defer span.End()
	svc, err := p.service.GetIssueService(ctx)
	if err != nil {
		log.DefaultLogger.With("traceID", span.SpanContext().TraceID()).Error("Error getting IssueService: "+err.Error(), "error", err)
		return nil, plugin.NewError(http.StatusInternalServerError, err.Error())
	}
	obj, err := svc.Get(ctx, resource.Identifier{
		Namespace: p.namespace,
		Name:      req.Vars.MustGet("name"),
	})
	if err != nil {
		if e, ok := err.(errWithStatusCode); ok {
			return nil, plugin.NewError(e.StatusCode(), e.Error())
		} else {
			log.DefaultLogger.With("traceID", span.SpanContext().TraceID()).Error("Error getting Issue '"+req.Vars.MustGet("name")+"': "+err.Error(), "error", err)
		}
	}
	return obj, err
}

func (p *Plugin) handleIssueCreate(ctx context.Context, req router.JSONRequest) (router.JSONResponse, error) {
	ctx, span := tracing.DefaultTracer().Start(ctx, "issue-create")
	defer span.End()
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, plugin.NewError(http.StatusBadRequest, err.Error())
	}

	t := issue.Issue{}
	// TODO: this should eventually be unmarshalled via a method in the Object itself, so Thema can handle it
	err = json.Unmarshal(body, &t)
	if err != nil {
		return nil, plugin.NewError(http.StatusBadRequest, err.Error())
	}

	svc, err := p.service.GetIssueService(ctx)
	if err != nil {
		log.DefaultLogger.Error("Error getting IssueService: " + err.Error())
		return nil, plugin.NewError(http.StatusInternalServerError, err.Error())
	}
	t.SetNamespace(p.namespace)
	obj, err := svc.Add(ctx, &t)
	if err != nil {
		if e, ok := err.(errWithStatusCode); ok {
			return nil, plugin.NewError(e.StatusCode(), e.Error())
		} else {
			log.DefaultLogger.With("traceID", span.SpanContext().TraceID()).Error("Error creating new Issue: "+err.Error(), "error", err)
		}
	}
	return obj, err
}

func (p *Plugin) handleIssueUpdate(ctx context.Context, req router.JSONRequest) (router.JSONResponse, error) {
	ctx, span := tracing.DefaultTracer().Start(ctx, "issue-update")
	defer span.End()
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, plugin.NewError(http.StatusBadRequest, err.Error())
	}

	t := issue.Issue{}
	// TODO: this should eventually be unmarshalled via a method in the Object itself, so Thema can handle it
	err = json.Unmarshal(body, &t)
	if err != nil {
		return nil, plugin.NewError(http.StatusBadRequest, err.Error())
	}

	svc, err := p.service.GetIssueService(ctx)
	if err != nil {
		log.DefaultLogger.With("traceID", span.SpanContext().TraceID()).Error("Error getting IssueService: "+err.Error(), "error", err)
		return nil, plugin.NewError(http.StatusInternalServerError, err.Error())
	}
	obj, err := svc.Update(ctx, resource.Identifier{
		Namespace: p.namespace,
		Name:      req.Vars.MustGet("name"),
	}, &t)
	if err != nil {
		if e, ok := err.(errWithStatusCode); ok {
			return nil, plugin.NewError(e.StatusCode(), e.Error())
		} else {
			log.DefaultLogger.With("traceID", span.SpanContext().TraceID()).Error("Error updating Issue '"+req.Vars.MustGet("name")+"': "+err.Error(), "error", err)
		}
	}
	return obj, err
}

func (p *Plugin) handleIssueDelete(ctx context.Context, req router.JSONRequest) (router.JSONResponse, error) {
	ctx, span := tracing.DefaultTracer().Start(ctx, "issue-delete")
	defer span.End()
	svc, err := p.service.GetIssueService(ctx)
	if err != nil {
		log.DefaultLogger.With("traceID", span.SpanContext().TraceID()).Error("Error getting IssueService: "+err.Error(), "error", err)
		return nil, plugin.NewError(http.StatusInternalServerError, err.Error())
	}
	err = svc.Delete(ctx, resource.Identifier{
		Namespace: p.namespace,
		Name:      req.Vars.MustGet("name"),
	})
	if err != nil {
		if e, ok := err.(errWithStatusCode); ok {
			return nil, plugin.NewError(e.StatusCode(), e.Error())
		} else {
			log.DefaultLogger.With("traceID", span.SpanContext().TraceID()).Error("Error deleting Issue '"+req.Vars.MustGet("name")+"': "+err.Error(), "error", err)
		}
	}
	return nil, err
}

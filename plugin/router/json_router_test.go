package router_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/grafana/grafana-app-sdk/plugin"
	"github.com/grafana/grafana-app-sdk/plugin/router"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONRouter_JSONErrorHandler(t *testing.T) {
	tests := []struct {
		name       string
		errHandler router.JSONErrorHandler
		err        error
		wantStatus int
		wantBody   []byte
	}{
		{
			name:       "when no custom error handler is used and no error is returned",
			errHandler: nil,
			err:        nil,
			wantStatus: http.StatusNoContent,
			wantBody:   nil,
		},
		{
			name: "when a custom error handler is used and no error is returned",
			errHandler: func(err plugin.Error) (int, router.JSONResponse) {
				return http.StatusInternalServerError, router.JSONErrorResponse{
					Code:  http.StatusInternalServerError,
					Error: "internal server error",
				}
			},
			err:        nil,
			wantStatus: http.StatusNoContent,
			wantBody:   nil,
		},
		{
			name:       "when no custom error handler is used and an error is returned",
			errHandler: nil,
			err:        plugin.NewError(http.StatusInternalServerError, "custom message"),
			wantStatus: http.StatusInternalServerError,
			wantBody:   []byte(`{"code":500,"error":"internal server error"}`),
		},
		{
			name: "when a custom error handler is used and an error is returned",
			errHandler: func(err plugin.Error) (int, router.JSONResponse) {
				return http.StatusBadRequest, router.JSONErrorResponse{
					Code:  http.StatusBadRequest,
					Error: "some message",
				}
			},
			err:        plugin.NewError(http.StatusInternalServerError, "whoops"),
			wantStatus: http.StatusBadRequest,
			wantBody:   []byte(`{"code":400,"error":"some message"}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := router.NewJSONRouterWithErrorHandler(tt.errHandler)

			h := r.WrapHandlerFunc(func(ctx context.Context, req router.JSONRequest) (router.JSONResponse, error) {
				return nil, tt.err
			}, http.StatusOK)

			send := fakeSender{
				sendFunc: func(res *backend.CallResourceResponse) error {
					// This is a hack around the fact that JSON encoder appends a newline to each `.Encode()` call.
					wantBody := tt.wantBody
					if wantBody != nil {
						wantBody = append(wantBody, '\n')
					}

					assert.Equal(t, wantBody, res.Body)
					assert.Equal(t, tt.wantStatus, res.Status)

					return nil
				},
			}

			h(context.Background(), &backend.CallResourceRequest{}, send)
		})
	}
}

func TestJSONRouter_SubrouteWithErrorHandler(t *testing.T) {
	tests := []struct {
		name       string
		errHandler router.JSONErrorHandler
		err        error
		wantStatus int
		wantBody   []byte
	}{
		{
			name:       "when no custom error handler is used and no error is returned",
			errHandler: nil,
			err:        nil,
			wantStatus: http.StatusNoContent,
			wantBody:   nil,
		},
		{
			name: "when a custom error handler is used and no error is returned",
			errHandler: func(err plugin.Error) (int, router.JSONResponse) {
				return http.StatusInternalServerError, router.JSONErrorResponse{
					Code:  http.StatusInternalServerError,
					Error: "internal server error",
				}
			},
			err:        nil,
			wantStatus: http.StatusNoContent,
			wantBody:   nil,
		},
		{
			name:       "when no custom error handler is used and an error is returned",
			errHandler: nil,
			err:        plugin.NewError(http.StatusInternalServerError, "custom message"),
			wantStatus: http.StatusInternalServerError,
			wantBody:   []byte(`{"code":500,"error":"internal server error"}`),
		},
		{
			name: "when a custom error handler is used and an error is returned",
			errHandler: func(err plugin.Error) (int, router.JSONResponse) {
				return http.StatusBadRequest, router.JSONErrorResponse{
					Code:  http.StatusBadRequest,
					Error: "some message",
				}
			},
			err:        plugin.NewError(http.StatusInternalServerError, "whoops"),
			wantStatus: http.StatusBadRequest,
			wantBody:   []byte(`{"code":400,"error":"some message"}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := router.NewJSONRouterWithErrorHandler(func(err plugin.Error) (int, router.JSONResponse) {
				t.Fatal("this should not be called")

				return http.StatusInternalServerError, nil
			})
			subRoute := r.SubrouteWithErrorHandler("/path", tt.errHandler)

			h := subRoute.WrapHandlerFunc(func(ctx context.Context, req router.JSONRequest) (router.JSONResponse, error) {
				return nil, tt.err
			}, http.StatusOK)

			send := fakeSender{
				sendFunc: func(res *backend.CallResourceResponse) error {
					// This is a hack around the fact that JSON encoder appends a newline to each `.Encode()` call.
					wantBody := tt.wantBody
					if wantBody != nil {
						wantBody = append(wantBody, '\n')
					}

					assert.Equal(t, wantBody, res.Body)
					assert.Equal(t, tt.wantStatus, res.Status)

					return nil
				},
			}

			h(context.Background(), &backend.CallResourceRequest{}, send)
		})
	}
}

func TestJSONRouter_WrapHandlerFunc(t *testing.T) {
	r := router.NewJSONRouter()

	method := http.MethodGet
	rawURL := "http://some.url.com/some/path?param1=val1&param2=val2"
	headers := map[string][]string{
		"header1": {"val1", "val2"},
	}
	pluginCtx := backend.PluginContext{
		OrgID:    1,
		PluginID: "some-plugin",
	}
	body := []byte("some-body")

	u, err := url.Parse(rawURL)
	require.NoError(t, err)
	url := *u

	req := backend.CallResourceRequest{
		Method:        method,
		URL:           rawURL,
		Headers:       headers,
		Body:          body,
		PluginContext: pluginCtx,
	}

	vars := router.NewVars(map[string]string{
		"some":  "var",
		"other": "vars",
	})

	type response struct {
		Message string `json:"message"`
	}

	tests := []struct {
		name           string
		res            router.JSONResponse
		err            error
		okStatus       int
		wantStatus     int
		wantBody       []byte
		wantJSONHeader bool
	}{
		{
			name: "when a 200 response is returned",
			res: &response{
				Message: "some",
			},
			err:            nil,
			okStatus:       http.StatusOK,
			wantStatus:     http.StatusOK,
			wantBody:       []byte(`{"message":"some"}`),
			wantJSONHeader: true,
		},
		{
			name: "when a 201 response is returned",
			res: &response{
				Message: "some",
			},
			err:            nil,
			okStatus:       http.StatusCreated,
			wantStatus:     http.StatusCreated,
			wantBody:       []byte(`{"message":"some"}`),
			wantJSONHeader: true,
		},
		{
			name:           "when no content is returned",
			res:            nil,
			err:            nil,
			okStatus:       http.StatusOK,
			wantStatus:     http.StatusNoContent,
			wantBody:       nil,
			wantJSONHeader: false,
		},
		{
			name:           "when a bad request error is returned",
			res:            nil,
			err:            plugin.NewError(http.StatusBadRequest, "bad request"),
			okStatus:       http.StatusOK,
			wantStatus:     http.StatusBadRequest,
			wantBody:       []byte(`{"code":400,"error":"bad request"}`),
			wantJSONHeader: true,
		},
		{
			name:           "when a generic error is returned",
			res:            nil,
			err:            errors.New("something bad happened"),
			okStatus:       http.StatusOK,
			wantStatus:     http.StatusInternalServerError,
			wantBody:       []byte(`{"code":500,"error":"internal server error"}`),
			wantJSONHeader: true,
		},
		{
			name: "when both response and an error are returned",
			res: response{
				Message: "some",
			},
			err:            errors.New("something bad happened"),
			okStatus:       http.StatusOK,
			wantStatus:     http.StatusInternalServerError,
			wantBody:       []byte(`{"code":500,"error":"internal server error"}`),
			wantJSONHeader: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := r.WrapHandlerFunc(func(ctx context.Context, req router.JSONRequest) (router.JSONResponse, error) {
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)

				assert.Equal(t, method, req.Method)
				assert.Equal(t, url, req.URL)
				assert.Equal(t, vars, req.Vars)
				assert.Equal(t, http.Header(headers), req.Headers)
				assert.Equal(t, pluginCtx, req.Context)
				assert.Equal(t, body, body)

				return tt.res, tt.err
			}, tt.okStatus)

			send := fakeSender{
				sendFunc: func(res *backend.CallResourceResponse) error {
					assert.Equal(t, tt.wantStatus, res.Status)

					// This is a hack around the fact that JSON encoder appends a newline to each `.Encode()` call.
					wantBody := tt.wantBody
					if wantBody != nil {
						wantBody = append(wantBody, '\n')
					}

					assert.Equal(t, wantBody, res.Body)
					if tt.wantJSONHeader {
						assert.Equal(t, map[string][]string{router.HeaderContentType: {router.ContentTypeJSON}}, res.Headers)
					}
					return nil
				},
			}

			h(router.CtxWithVars(context.Background(), vars), &req, send)
		})
	}
}

func TestJSONRouter_HandleWithCode(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		method     string
		wantPath   string
		wantMethod string
	}{
		{
			name:       "when no path is specified",
			path:       "",
			method:     http.MethodGet,
			wantPath:   "/",
			wantMethod: http.MethodGet,
		},
		{
			name:       "when no method is specified",
			path:       "/foo",
			method:     "",
			wantPath:   "/foo",
			wantMethod: http.MethodGet,
		},
		{
			name:       "when path and method are specified",
			path:       "/foo",
			method:     http.MethodGet,
			wantPath:   "/foo",
			wantMethod: http.MethodGet,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := router.NewJSONRouter()

			if tt.method == "" {
				r.HandleWithCode(tt.path, func(ctx context.Context, req router.JSONRequest) (router.JSONResponse, error) {
					return nil, nil
				}, http.StatusOK)
			} else {
				r.HandleWithCode(tt.path, func(ctx context.Context, req router.JSONRequest) (router.JSONResponse, error) {
					return nil, nil
				}, http.StatusOK, tt.method)
			}

			send := fakeSender{
				sendFunc: func(res *backend.CallResourceResponse) error {
					assert.Equal(t, http.StatusNoContent, res.Status)
					assert.EqualValues(t, []byte(nil), res.Body)
					return nil
				},
			}

			assert.NoError(t, r.CallResource(context.Background(), &backend.CallResourceRequest{
				Path:   tt.wantPath,
				Method: tt.wantMethod,
			}, send))
		})
	}
}

func TestJSONRouter_HandleResource(t *testing.T) {
	r := router.NewJSONRouter()
	r.NotFoundHandler = nil

	s := fakeSender{
		sendFunc: func(res *backend.CallResourceResponse) error {
			return nil
		},
	}

	r.HandleResource("resource", router.JSONResourceHandler{
		Create: func(context.Context, router.JSONRequest) (router.JSONResponse, error) { return nil, nil },
		Read:   func(context.Context, router.JSONRequest) (router.JSONResponse, error) { return nil, nil },
		Update: func(context.Context, router.JSONRequest) (router.JSONResponse, error) { return nil, nil },
		Delete: func(context.Context, router.JSONRequest) (router.JSONResponse, error) { return nil, nil },
		List:   func(context.Context, router.JSONRequest) (router.JSONResponse, error) { return nil, nil },
	})

	assert.NoError(t, r.CallResource(context.Background(), &backend.CallResourceRequest{
		Path:   "/resource",
		Method: http.MethodPost,
	}, s))

	assert.NoError(t, r.CallResource(context.Background(), &backend.CallResourceRequest{
		Path:   "/resource/1234",
		Method: http.MethodGet,
	}, s))

	assert.NoError(t, r.CallResource(context.Background(), &backend.CallResourceRequest{
		Path:   "/resource/1234",
		Method: http.MethodPut,
	}, s))

	assert.NoError(t, r.CallResource(context.Background(), &backend.CallResourceRequest{
		Path:   "/resource/1234",
		Method: http.MethodDelete,
	}, s))

	assert.NoError(t, r.CallResource(context.Background(), &backend.CallResourceRequest{
		Path:   "/resource",
		Method: http.MethodGet,
	}, s))

	assert.Error(t, r.CallResource(context.Background(), &backend.CallResourceRequest{
		Path:   "/some-other",
		Method: http.MethodPatch,
	}, s))
}

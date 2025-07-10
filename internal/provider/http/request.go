package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	xcontext "github.com/krateoplatformops/plumbing/context"
	"github.com/krateoplatformops/plumbing/http/request"
	"github.com/krateoplatformops/plumbing/http/response"
	"github.com/krateoplatformops/plumbing/ptr"
)

const maxUnstructuredResponseTextBytes = 2048

// This is a copy of: plumbing/http/request/request.go
// with a line change below
func Do(ctx context.Context, opts request.RequestOptions) *response.Status {
	uri := strings.TrimSuffix(opts.Endpoint.ServerURL, "/")
	if len(opts.Path) > 0 {
		uri = fmt.Sprintf("%s/%s", uri, strings.TrimPrefix(opts.Path, "/"))
	}

	u, err := url.Parse(uri)
	if err != nil {
		return response.New(http.StatusInternalServerError, err)
	}

	verb := ptr.Deref(opts.Verb, http.MethodGet)

	var body io.Reader
	if s := ptr.Deref(opts.Payload, ""); len(s) > 0 {
		body = strings.NewReader(s)
	}

	call, err := http.NewRequestWithContext(ctx, verb, u.String(), body)
	if err != nil {
		return response.New(http.StatusInternalServerError, err)
	}
	call.Header.Set(xcontext.LabelKrateoTraceId, xcontext.TraceId(ctx, true))

	if len(opts.Headers) > 0 {
		for _, el := range opts.Headers {
			idx := strings.Index(el, ":")
			if idx <= 0 {
				continue
			}
			call.Header.Set(el[:idx], el[idx+1:])
		}
	}

	cli, err := request.HTTPClientForEndpoint(opts.Endpoint)
	if err != nil {
		return response.New(http.StatusInternalServerError,
			fmt.Errorf("unable to create HTTP Client for endpoint: %w", err))
	}

	respo, err := cli.Do(call)
	if err != nil {
		return response.New(http.StatusInternalServerError, err)
	}
	defer respo.Body.Close()

	statusOK := respo.StatusCode >= 200 && respo.StatusCode < 300
	if !statusOK {
		dat, err := io.ReadAll(io.LimitReader(respo.Body, maxUnstructuredResponseTextBytes))
		if err != nil {
			return response.New(http.StatusInternalServerError, err)
		}

		res := &response.Status{}
		if err := json.Unmarshal(dat, res); err != nil {
			res = response.New(respo.StatusCode, fmt.Errorf("%s", string(dat)))
			return res
		}

		return res
	}

	// These lines are not needed for this project

	// if ct := respo.Header.Get("Content-Type"); !strings.Contains(ct, "json") {
	// 	return response.New(http.StatusNotAcceptable, fmt.Errorf("content type %q is not allowed", ct))
	// }

	if opts.ResponseHandler != nil {
		if err := opts.ResponseHandler(respo.Body); err != nil {
			return response.New(http.StatusInternalServerError, err)
		}
		return response.New(http.StatusOK, nil)
	}

	return response.New(http.StatusNoContent, nil)
}

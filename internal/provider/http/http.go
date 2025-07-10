package http

import (
	"context"
	api "database-handler-uploader/api/v1"
	"fmt"
	"io"

	"github.com/krateoplatformops/plumbing/endpoints"
	"github.com/krateoplatformops/plumbing/http/request"

	ctrl "sigs.k8s.io/controller-runtime"
)

type HttpProvider struct{}

func (r *HttpProvider) Resolve(managed *api.Notebook) (string, error) {
	cfg := ctrl.GetConfigOrDie()

	endpoint, err := endpoints.FromSecret(context.Background(), cfg, managed.Spec.API.EndpointRef.Name, managed.Spec.API.EndpointRef.Namespace)
	if err != nil {
		return "", fmt.Errorf("could not get endpoint secret for API %s: %v", managed.Name, err)
	}

	var bodyData []byte

	options := request.RequestOptions{
		Path:     managed.Spec.API.Path,
		Verb:     &managed.Spec.API.Verb,
		Payload:  &managed.Spec.API.Payload,
		Headers:  managed.Spec.API.Headers,
		Endpoint: &endpoint,
		ResponseHandler: func(rc io.ReadCloser) error {
			bodyData, _ = io.ReadAll(rc)
			return nil
		},
	}

	res := Do(context.Background(), options)

	if res.Code != 200 {
		return "", fmt.Errorf("resolver error while calling endpoint: %v - body: %s", res.Message, string(bodyData))
	}

	return string(bodyData), nil
}

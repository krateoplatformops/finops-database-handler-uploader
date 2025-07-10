package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/krateoplatformops/provider-runtime/pkg/controller"
	"github.com/krateoplatformops/provider-runtime/pkg/event"
	"github.com/krateoplatformops/provider-runtime/pkg/logging"
	"github.com/krateoplatformops/provider-runtime/pkg/ratelimiter"
	"github.com/krateoplatformops/provider-runtime/pkg/reconciler"
	"github.com/krateoplatformops/provider-runtime/pkg/resource"
	"k8s.io/client-go/tools/record"

	prv1 "github.com/krateoplatformops/provider-runtime/apis/common/v1"

	api "database-handler-uploader/api/v1"
	"database-handler-uploader/internal/helpers"
	"database-handler-uploader/internal/helpers/config"

	"github.com/krateoplatformops/plumbing/endpoints"
	"github.com/krateoplatformops/plumbing/http/request"
)

func Setup(mgr ctrl.Manager, o controller.Options, config config.Configuration) error {
	name := reconciler.ControllerName(api.GroupKind)

	log := o.Logger.WithValues("controller", name)
	log.Info("controller", "name", name)

	recorder := mgr.GetEventRecorderFor(name)

	r := reconciler.NewReconciler(mgr,
		resource.ManagedKind(api.GroupVersionKind),
		reconciler.WithExternalConnecter(&connector{
			log:          log,
			recorder:     recorder,
			pollInterval: o.PollInterval,
			config:       config,
		}),
		reconciler.WithPollInterval(o.PollInterval),
		reconciler.WithLogger(log),
		reconciler.WithRecorder(event.NewAPIRecorder(recorder)))

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		For(&api.Notebook{}).
		Complete(ratelimiter.New(name, r, o.GlobalRateLimiter))
}

type connector struct {
	pollInterval time.Duration
	log          logging.Logger
	recorder     record.EventRecorder
	config       config.Configuration
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (reconciler.ExternalClient, error) {
	return &external{
		pollInterval: c.pollInterval,
		log:          c.log,
		rec:          c.recorder,
		config:       c.config,
	}, nil
}

type external struct {
	pollInterval time.Duration
	log          logging.Logger
	rec          record.EventRecorder
	config       config.Configuration
}

func (c *external) Disconnect(_ context.Context) error {
	return nil // NOOP
}

func (e *external) Observe(ctx context.Context, mg resource.Managed) (reconciler.ExternalObservation, error) {
	managed, ok := mg.(*api.Notebook)
	if !ok {
		return reconciler.ExternalObservation{}, fmt.Errorf("cannot cast to api.Notebook")
	}

	cfg := ctrl.GetConfigOrDie()

	e.log.Debug("getting finops-database-handler endpoint", "name", managed.Name, "operation", "observe")
	dbHandlerEndpoint, err := endpoints.FromSecret(ctx, cfg, e.config.DBHandlerEndpoint.Name, e.config.DBHandlerEndpoint.Namespace)
	if err != nil {
		return reconciler.ExternalObservation{}, fmt.Errorf("could not get endpoint secret for finops-database-handler: %v", err)
	}
	if e.config.DBHandlerURLOverride != nil {
		dbHandlerEndpoint.ServerURL = *e.config.DBHandlerURLOverride
	}

	var bodyData []byte

	e.log.Debug("listing notebooks", "name", managed.Name, "operation", "observe")
	res := request.Do(ctx, request.RequestOptions{
		Path:     "/compute/list",
		Verb:     helpers.GetStringPointer("GET"),
		Endpoint: &dbHandlerEndpoint,
		ResponseHandler: func(rc io.ReadCloser) error {
			bodyData, _ = io.ReadAll(rc)
			return nil
		},
	})

	if res.Code != 200 {
		return reconciler.ExternalObservation{}, fmt.Errorf("could not get notebook list from finops-database-handler: %d %v - body: %s", res.Code, res.Message, string(bodyData))
	}

	var bodyValues [][]string
	err = json.Unmarshal(bodyData, &bodyValues)
	if err != nil {
		return reconciler.ExternalObservation{}, fmt.Errorf("could not unmarshal notebook list from finops-database-handler: %d %v", res.Code, res.Message)
	}

	// The data structure is: [["allcosts"],["costs"],["metrics"],["pricing_parser"],["costsbreakdown"],["cyclic"],["pricing_frontend"]]
	notebooks := []string{}
	for _, s := range bodyValues {
		notebooks = append(notebooks, s[0])
	}
	e.log.Debug("received notebook list", "list", notebooks, "name", managed.Name, "operation", "observe")

	bodyData = []byte{}
	if slices.Contains(notebooks, managed.Name) {
		managed.SetConditions(prv1.Available())
		e.log.Debug("getting notebook code", "name", managed.Name)
		res = request.Do(ctx, request.RequestOptions{
			Path:     fmt.Sprintf("/compute/%s/info", managed.Name),
			Verb:     helpers.GetStringPointer("GET"),
			Endpoint: &dbHandlerEndpoint,
			ResponseHandler: func(rc io.ReadCloser) error {
				bodyData, _ = io.ReadAll(rc)
				return nil
			},
		})

		if res.Code != 200 {
			return reconciler.ExternalObservation{}, fmt.Errorf("could not get notebook code from finops-database-handler: %d %v - body: %s", res.Code, res.Message, string(bodyData))
		}

		e.log.Debug("received notebook data", "data", string(bodyData), "name", managed.Name, "operation", "observe")
		var notebookValues helpers.NotebookInfo
		err = json.Unmarshal(bodyData, &notebookValues)
		if err != nil {
			return reconciler.ExternalObservation{}, fmt.Errorf("could not unmarshal notebook code from finops-database-handler: %d %v", res.Code, res.Message)
		}

		e.log.Debug("getting new code from managed", "name", managed.Name, "operation", "observe")
		code, err := helpers.GetProvider(managed.Spec.Type).Resolve(managed)
		if err != nil {
			return reconciler.ExternalObservation{}, fmt.Errorf("could not resolve notebook code from custom resource: %v", err)
		}

		if code != notebookValues.Code {
			e.log.Debug("code does not match", "name", managed.Name, "operation", "observe")
			e.log.Debug("managed", "code", code)
			e.log.Debug("notebook", "code", notebookValues.Code)
			return reconciler.ExternalObservation{
				ResourceExists:   true,
				ResourceUpToDate: false,
			}, nil
		}
		e.log.Debug("code does match, up-to-date", "name", managed.Name, "operation", "observe")
		return reconciler.ExternalObservation{
			ResourceExists:   true,
			ResourceUpToDate: true,
		}, nil
	}
	e.log.Debug("notebook does not exist", "name", managed.Name, "operation", "observe")
	return reconciler.ExternalObservation{
		ResourceExists: false,
	}, nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) error {
	managed, ok := mg.(*api.Notebook)
	if !ok {
		return fmt.Errorf("cannot cast to api.Notebook")
	}

	e.log.Debug("getting new code from managed", "name", managed.Name, "operation", "create")
	code, err := helpers.GetProvider(managed.Spec.Type).Resolve(managed)
	if err != nil {
		return fmt.Errorf("could not resolve notebook code from custom resource: %v", err)
	}

	cfg := ctrl.GetConfigOrDie()
	e.log.Debug("getting finops-database-handler endpoint", "name", managed.Name, "operation", "create")
	dbHandlerEndpoint, err := endpoints.FromSecret(ctx, cfg, e.config.DBHandlerEndpoint.Name, e.config.DBHandlerEndpoint.Namespace)
	if err != nil {
		return fmt.Errorf("could not get endpoint secret for finops-database-handler: %v", err)
	}
	if e.config.DBHandlerURLOverride != nil {
		dbHandlerEndpoint.ServerURL = *e.config.DBHandlerURLOverride
	}

	var bodyData []byte
	e.log.Debug("creating notebook", "name", managed.Name, "operation", "create")
	res := request.Do(ctx, request.RequestOptions{
		Path:     fmt.Sprintf("/compute/%s/upload", managed.Name),
		Verb:     helpers.GetStringPointer("POST"),
		Endpoint: &dbHandlerEndpoint,
		Payload:  &code,
		ResponseHandler: func(rc io.ReadCloser) error {
			bodyData, _ = io.ReadAll(rc)
			return nil
		},
	})

	if res.Code != 200 {
		return fmt.Errorf("could not upload notebook %s: %d %v - body: %s", managed.Name, res.Code, res.Message, string(bodyData))
	}

	managed.SetConditions(prv1.Creating())
	return nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) error {
	managed, ok := mg.(*api.Notebook)
	if !ok {
		return fmt.Errorf("cannot cast to api.Notebook")
	}

	cfg := ctrl.GetConfigOrDie()

	e.log.Debug("getting finops-database-handler endpoint", "name", managed.Name, "operation", "update")
	dbHandlerEndpoint, err := endpoints.FromSecret(ctx, cfg, e.config.DBHandlerEndpoint.Name, e.config.DBHandlerEndpoint.Namespace)
	if err != nil {
		return fmt.Errorf("could not get endpoint secret for finops-database-handler: %v", err)
	}
	if e.config.DBHandlerURLOverride != nil {
		dbHandlerEndpoint.ServerURL = *e.config.DBHandlerURLOverride
	}

	e.log.Debug("getting new code from managed", "name", managed.Name, "operation", "update")
	code, err := helpers.GetProvider(managed.Spec.Type).Resolve(managed)
	if err != nil {
		return fmt.Errorf("could not resolve notebook code from custom resource: %v", err)
	}

	e.log.Debug("uploading notebook", "name", managed.Name, "operation", "update")
	res := request.Do(ctx, request.RequestOptions{
		Path:     fmt.Sprintf("/compute/%s/upload?overwrite=true", managed.Name),
		Verb:     helpers.GetStringPointer("POST"),
		Endpoint: &dbHandlerEndpoint,
		Payload:  &code,
	})

	if res.Code != 200 {
		return fmt.Errorf("could not upload notebook %s: %v", managed.Name, res.Message)
	}

	return nil
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) error {
	managed, ok := mg.(*api.Notebook)
	if !ok {
		return fmt.Errorf("cannot cast to api.Notebook")
	}

	cfg := ctrl.GetConfigOrDie()

	e.log.Debug("getting finops-database-handler endpoint", "name", managed.Name, "operation", "delete")
	dbHandlerEndpoint, err := endpoints.FromSecret(ctx, cfg, e.config.DBHandlerEndpoint.Name, e.config.DBHandlerEndpoint.Namespace)
	if err != nil {
		return fmt.Errorf("could not get endpoint secret for finops-database-handler: %v", err)
	}
	if e.config.DBHandlerURLOverride != nil {
		dbHandlerEndpoint.ServerURL = *e.config.DBHandlerURLOverride
	}

	e.log.Debug("deleting notebook", "name", managed.Name, "operation", "delete")
	res := request.Do(ctx, request.RequestOptions{
		Path:     fmt.Sprintf("/compute/%s", managed.Name),
		Verb:     helpers.GetStringPointer("DELETE"),
		Endpoint: &dbHandlerEndpoint,
	})

	if res.Code != 200 {
		return fmt.Errorf("could not upload notebook %s: %v", managed.Name, res.Message)
	}

	managed.SetConditions(prv1.Deleting())

	return nil
}

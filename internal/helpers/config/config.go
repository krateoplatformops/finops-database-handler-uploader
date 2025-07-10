package config

import (
	"flag"
	"fmt"

	"github.com/krateoplatformops/plumbing/env"

	finopstypes "github.com/krateoplatformops/finops-data-types/api/v1"
)

type Configuration struct {
	WatchNamespace       string
	PollingInterval      string
	MaxReconcileRate     string
	DBHandlerEndpoint    *finopstypes.ObjectRef
	DBHandlerURLOverride *string
}

func (r *Configuration) String() string {
	return fmt.Sprintf("WATCH_NAMESPACE: %s - MAX_RECONCILE_RATE: %s - POLLING_INTERVAL: %s - FINOPS_DATABASE_HANDLER_ENDPOINT_NAME: %s - FINOPS_DATABASE_HANDLER_ENDPOINT_NAMESPACE: %s - FINOPS_DATABASE_HANDLER_URL_OVERRIDE: %s", r.WatchNamespace, r.MaxReconcileRate, r.PollingInterval, r.DBHandlerEndpoint.Name, r.DBHandlerEndpoint.Namespace, *r.DBHandlerURLOverride)
}

func ParseConfig() Configuration {
	watchNamespace := flag.String("watchnamespace",
		env.String("WATCH_NAMESPACE", ""), "Default namespace to watch")
	maxReconcileRate := flag.String("maxreconcilerate",
		env.String("MAX_RECONCILE_RATE", "1"), "Maximum reconcile rate (default: 1)")
	pollingInterval := flag.String("pollinginterval",
		env.String("POLLING_INTERVAL", "300"), "Polling interval in seconds (default: 300)")

	dbHandlerEndpointName := flag.String("dbhandlerendpointname",
		env.String("FINOPS_DATABASE_HANDLER_ENDPOINT_NAME", "finops-database-handler-endpoint"), "Name of the secret with the finops-database-handler endpoint")
	dbHandlerEndpointNamespace := flag.String("dbhandlerendpointnamespace",
		env.String("FINOPS_DATABASE_HANDLER_ENDPOINT_NAMESPACE", "krateo-system"), "Namespace of the secret with the finops-database-handler endpoint")

	dbHandlerURLOverride := flag.String("dbhandlerurloverride",
		env.String("FINOPS_DATABASE_HANDLER_URL_OVERRIDE", ""), "finops-database-handler URL override")

	flag.Parse()

	return Configuration{
		WatchNamespace:   *watchNamespace,
		PollingInterval:  *pollingInterval,
		MaxReconcileRate: *maxReconcileRate,
		DBHandlerEndpoint: &finopstypes.ObjectRef{
			Name:      *dbHandlerEndpointName,
			Namespace: *dbHandlerEndpointNamespace,
		},
		DBHandlerURLOverride: dbHandlerURLOverride,
	}
}

package provider

import (
	api "database-handler-uploader/api/v1"
)

type Provider interface {
	Resolve(notebook *api.Notebook) (string, error)
}

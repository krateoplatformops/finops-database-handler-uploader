package helpers

import (
	providers "database-handler-uploader/internal/provider"
	fileprovider "database-handler-uploader/internal/provider/file"
	httpprovider "database-handler-uploader/internal/provider/http"
	"strings"
)

type NotebookInfo struct {
	Name string `yaml:"name" json:"name"`
	Code string `yaml:"code" json:"code"`
}

func GetStringPointer(value string) *string {
	return &value
}

func GetProvider(name string) providers.Provider {
	providers := map[string]providers.Provider{
		"inline": &fileprovider.FileProvider{},
		"api":    &httpprovider.HttpProvider{},
	}
	return providers[strings.ToLower(name)]
}

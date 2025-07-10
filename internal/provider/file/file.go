package file

import api "database-handler-uploader/api/v1"

type FileProvider struct{}

func (r *FileProvider) Resolve(notebook *api.Notebook) (string, error) {
	return notebook.Spec.Inline, nil
}

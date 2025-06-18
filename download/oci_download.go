//go:build !opa_no_oci

package download

import (
	v1 "github.com/IUAD1IY7/opa/v1/download"

	"github.com/IUAD1IY7/opa/plugins/rest"
)

// NewOCI returns a new Downloader that can be started.
func NewOCI(config Config, client rest.Client, path, storePath string) *OCIDownloader {
	return v1.NewOCI(config, client, path, storePath)
}

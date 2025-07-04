package identifier_test

import (
	"crypto/x509"
	"net/http"

	"github.com/IUAD1IY7/opa/v1/server/identifier"
)

type mockHandler struct {
	identity        string
	identityDefined bool

	clientCertificates        []*x509.Certificate
	clientCertificatesDefined bool
}

func (h *mockHandler) ServeHTTP(_ http.ResponseWriter, r *http.Request) {
	h.identity, h.identityDefined = identifier.Identity(r)
	h.clientCertificates, h.clientCertificatesDefined = identifier.ClientCertificates(r)
}

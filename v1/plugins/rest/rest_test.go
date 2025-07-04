// Copyright 2018 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package rest

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/IUAD1IY7/opa/internal/jwx/jwa"
	"github.com/IUAD1IY7/opa/internal/jwx/jws"
	"github.com/IUAD1IY7/opa/internal/providers/aws"
	"github.com/IUAD1IY7/opa/v1/bundle"
	"github.com/IUAD1IY7/opa/v1/keys"
	"github.com/IUAD1IY7/opa/v1/logging"
	"github.com/IUAD1IY7/opa/v1/tracing"

	"github.com/IUAD1IY7/opa/internal/version"
	"github.com/IUAD1IY7/opa/v1/util/test"

	testlogger "github.com/IUAD1IY7/opa/v1/logging/test"
)

const keyID = "key1"

func TestAuthPluginWithNoAuthPluginLookup(t *testing.T) {
	t.Parallel()

	authPlugin := "anything"
	cfg := Config{
		Credentials: struct {
			Bearer               *bearerAuthPlugin                  `json:"bearer,omitempty"`
			OAuth2               *oauth2ClientCredentialsAuthPlugin `json:"oauth2,omitempty"`
			ClientTLS            *clientTLSAuthPlugin               `json:"client_tls,omitempty"`
			S3Signing            *awsSigningAuthPlugin              `json:"s3_signing,omitempty"`
			GCPMetadata          *gcpMetadataAuthPlugin             `json:"gcp_metadata,omitempty"`
			AzureManagedIdentity *azureManagedIdentitiesAuthPlugin  `json:"azure_managed_identity,omitempty"`
			Plugin               *string                            `json:"plugin,omitempty"`
		}{
			Plugin: &authPlugin,
		},
	}
	_, err := cfg.AuthPlugin(nil)
	if err == nil {
		t.Error("Expected error but got nil")
	}
	if want, have := "missing auth plugin lookup function", err.Error(); want != have {
		t.Errorf("Unexpected error, want %q, have %q", want, have)
	}
}

// Note(philipc): Cannot run this test in parallel, due to the t.Setenv calls
// from one of its helper methods.
func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		env     map[string]string
	}{
		{
			name: "BadScheme",
			input: `{
				"name": "foo",
				"url": "bad scheme://authority",
			}`,
			wantErr: true,
		},
		{
			name: "ValidUrl",
			input: `{
				"name": "foo",
				"url", "http://localhost/some/path",
			}`,
		},
		{
			name: "Token",
			input: `{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"bearer": {
						"token": "secret",
					}
				}
			}`,
		},
		{
			name: "TokenWithScheme",
			input: `{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"bearer": {
						"scheme": "Acmecorp-Token",
						"token": "secret"
					}
				}
			}`,
		},
		{
			name: "MissingTlsOptions",
			input: `{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"client_tls": {}
				}
			}`,
			wantErr: true,
		},
		{
			name: "IncompleteTlsOptions",
			input: `{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"client_tls": {
						"cert": "cert.pem"
					}
				}
			}`,
			wantErr: true,
		},
		{
			name: "EmptyS3Options",
			input: `{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"s3_signing": {
					}
				}
			}`,
			wantErr: true,
		},
		{
			name: "ValidS3EnvCreds",
			input: `{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"s3_signing": {
						"environment_credentials": {}
					}
				}
			}`,
		},
		{
			name: "ValidApiGatewayEnvCreds",
			input: `{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"s3_signing": {
						"service": "execute-api",
						"environment_credentials": {}
					}
				}
			}`,
		},
		{
			name: "ValidS3MetadataCredsWithRole",
			input: `{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"s3_signing": {
						"metadata_credentials": {
							"aws_region": "us-east-1",
							"iam_role": "my_iam_role"
						}
					}
				}
			}`,
		},
		{
			name: "ValidS3MetadataCreds",
			input: `{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"s3_signing": {
						"metadata_credentials": {
							"aws_region": "us-east-1",
						}
					}
				}
			}`,
		},
		{
			name: "MissingS3MetadataCredOptions",
			input: `{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"s3_signing": {
						"metadata_credentials": {}
					}
				}
			}`,
			wantErr: true,
		},
		{
			name: "MultipleS3CredOptions/metadata+environment",
			input: `{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"s3_signing": {
						"metadata_credentials": {
							"aws_region": "us-east-1",
							"iam_role": "my_iam_role"
						},
						"environment_credentials": {}
					}
				}
			}`,
			wantErr: false,
		},
		{
			name: "MultipleS3CredOptions/metadata+profile+environment+webidentity",
			input: `{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"s3_signing": {
						"profile_credentials": {},
						"environment_credentials": {},
						"web_identity_credentials": {},
						"metadata_credentials": {
							"aws_region": "us-east-1",
							"iam_role": "my_iam_role"
						}
					}
				}
			}`,
			env: map[string]string{
				awsRoleArnEnvVar:              "TEST",
				awsWebIdentityTokenFileEnvVar: "TEST",
				awsRegionEnvVar:               "us-west-2",
			},
			wantErr: false,
		},
		{
			name: "MultipleCredentialsOptions",
			input: `{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"s3_signing": {
						"environment_credentials": {}
					},
					"bearer": {
						"scheme": "Acmecorp-Token",
						"token": "secret"
					}
				}
			}`,
			wantErr: true,
		},
		{
			name: "Oauth2NoTokenUrl",
			input: `{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"oauth2": {
						"token_url": ""
					}

				}
			}`,
			wantErr: true,
		},
		{
			name: "Oauth2MissingScopes",
			input: `{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"oauth2": {
						"token_url": "https://localhost",
						"client_id": "client_one",
						"client_secret": "super_secret"
					}
				}
			}`,
		},
		{
			name: "Oauth2MissingClientId",
			input: `{
				"name": "foo",
				"url": "https://localhost",
				"credentials": {
					"oauth2": {
						"token_url": "https://localhost",
						"client_id": ""
					}
				}
			}`,
			wantErr: true,
		},
		{
			name: "Oauth2MissingSecret",
			input: `{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"oauth2": {
						"token_url": "https://localhost",
						"client_id": "client_one"
					}
				}
			}`,
			wantErr: true,
		},
		{
			name: "Oauth2Creds",
			input: `{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"oauth2": {
						"token_url": "https://localhost",
						"client_id": "client_one",
						"client_secret": "super_secret"
					}
				}
			}`,
		},
		{
			name: "Oauth2GetCredScopes",
			input: `{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"oauth2": {
						"token_url": "https://localhost",
						"client_id": "client_one",
						"client_secret": "super_secret",
						"scopes": ["profile", "opa"]
					}
				}
			}`,
		},
		{
			name: "Oauth2JwtBearerMissingSigningKey",
			input: fmt.Sprintf(`{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"oauth2": {
						"grant_type": %q,
						"token_url": "https://localhost",
						"scopes": ["profile", "opa"],
						"additional_claims": {
							"aud": "some audience"
						}
					}
				}
			}`, grantTypeJwtBearer),
			wantErr: true,
		},
		{
			name: "Oauth2JwtBearerSigningKeyWithoutCorrespondingKey",
			input: fmt.Sprintf(`{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"oauth2": {
						"grant_type": %q,
						"signing_key": "key2",
						"token_url": "https://localhost",
						"scopes": ["profile", "opa"],
						"additional_claims": {
							"aud": "some audience"
						}
					}
				}
			}`, grantTypeJwtBearer),
			wantErr: true,
		},
		{
			name: "Oauth2JwtBearerSigningKeyWithCorrespondingKey",
			input: fmt.Sprintf(`{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"oauth2": {
						"grant_type": %q,
						"signing_key": "key1",
						"token_url": "https://localhost",
						"scopes": ["profile", "opa"],
						"additional_claims": {
							"aud": "some audience"
						}
					}
				}
			}`, grantTypeJwtBearer),
		},
		{
			name: "Oauth2JwtBearerSigningKeyPublicKeyReference",
			input: fmt.Sprintf(`{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"oauth2": {
						"grant_type": %q,
						"signing_key": "pub_key",
						"token_url": "https://localhost",
						"scopes": ["profile", "opa"],
						"additional_claims": {
							"aud": "some audience"
						}
					}
				}
			}`, grantTypeJwtBearer),
			wantErr: true,
		},
		{
			name: "Oauth2WrongGrantType",
			input: `{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"oauth2": {
						"grant_type": "authorization_code",
						"token_url": "https://localhost",
						"scopes": ["profile", "opa"],
						"additional_claims": {
							"aud": "some audience"
						}
					}
				}
			}`,
			wantErr: true,
		},
		{
			name: "Oauth2ClientCredentialsMissingCredentials",
			input: fmt.Sprintf(`{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"oauth2": {
						"grant_type": %q,
						"token_url": "https://localhost",
						"scopes": ["profile", "opa"],
						"additional_claims": {
							"aud": "some audience"
						}
					}
				}
			}`, grantTypeClientCredentials),
			wantErr: true,
		},
		{
			name: "Oauth2ClientCredentialsJwtNoAdditionalClaims",
			input: fmt.Sprintf(`{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"oauth2": {
						"grant_type": %q,
						"signing_key": "key1",
						"token_url": "https://localhost",
						"scopes": ["profile", "opa"]
					}
				}
			}`, grantTypeClientCredentials),
		},
		{
			name: "Oauth2ClientCredentialsJwtThumbprint",
			input: fmt.Sprintf(`{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"oauth2": {
						"grant_type": %q,
						"signing_key": "key1",
						"token_url": "https://localhost",
						"scopes": ["profile", "opa"],
						"thumbprint": "8F1BDDDE9982299E62749C20EDDBAAC57F619D04"
					}
				}
			}`, grantTypeClientCredentials),
		},
		{
			name: "Oauth2ClientCredentialsTooManyCredentials",
			input: fmt.Sprintf(`{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"oauth2": {
						"grant_type": %q,
						"signing_key": "key1",
						"client_id": "client-one",
						"client_secret": "supersecret",
						"token_url": "https://localhost",
						"scopes": ["profile", "opa"],
						"additional_claims": {
							"aud": "some audience"
						}
					}
				}
			}`, grantTypeClientCredentials),
			wantErr: true,
		},
		{
			name: "Oauth2ClientCredentialsJWTAuthentication",
			input: fmt.Sprintf(`{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"oauth2": {
						"grant_type": %q,
						"signing_key": "key1",
						"token_url": "https://localhost",
						"scopes": ["profile", "opa"],
						"additional_claims": {
							"aud": "some audience"
						}
					}
				}
			}`, grantTypeClientCredentials),
		},
		{
			name: "Oauth2ClientCredentialsJWTAuthentication_with_AWS_KMS",
			input: fmt.Sprintf(`{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"oauth2": {
						"grant_type": %q,
						"aws_kms": {
							"name": "arn:aws:kms:eu-west-1:account_no:key/key_id",
							"algorithm": "ECDSA_SHA_256"
						},
						"aws_signing": {
							"service": "kms",
							"environment_credentials": {
								"aws_default_region": "eu-west-1"
							}
						},
						"token_url": "https://localhost",
						"scopes": ["profile", "opa"],
						"additional_claims": {
							"aud": "some audience"
						}
					}
				}
			}`, grantTypeClientCredentials),
		},
		{
			name: "Oauth2ClientCredentialsJWTAuthentication_with_AWS_KMS_missing_credentials",
			input: fmt.Sprintf(`{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"oauth2": {
						"grant_type": %q,
						"aws_kms": {
							"name": "arn:aws:kms:eu-west-1:account_no:key/key_id",
							"algorithm": "ECDSA_SHA_256"
						},
						"token_url": "https://localhost",
						"scopes": ["profile", "opa"],
						"additional_claims": {
							"aud": "some audience"
						}
					}
				}
			}`, grantTypeClientCredentials),
			wantErr: true,
		},
		{
			name: "Oauth2ClientCredentialsJWTAuthentication with Azure KeyVault",
			input: fmt.Sprintf(`{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"oauth2": {
						"grant_type": %q,
						"azure_keyvault": {
            	"key": "tester-key",
          		"key_algorithm": "ES256",
          		"vault": "my-secret-kv"
          	},
            "azure_signing": {
            	"service": "keyvault",
              "azure_managed_identity": {}
            },
            "token_url": "https://localhost",
						"scopes": ["profile", "opa"],
						"additional_claims": {
							"aud": "some audience"
						}
					}
				}
			}`, grantTypeClientCredentials),
		},
		{
			name: "Oauth2ClientCredentialsJWTAuthentication with Azure KeyVault and missing managed identity",
			input: fmt.Sprintf(`{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"oauth2": {
						"grant_type": %q,
						"azure_keyvault": {
            	"key": "tester-key",
          		"key_algorithm": "ES256",
          		"vault": "my-secret-kv"
          	},
            "azure_signing": {
            	"service": "keyvault",
            },
            "token_url": "https://localhost",
						"scopes": ["profile", "opa"],
						"additional_claims": {
							"aud": "some audience"
						}
					}
				}
			}`, grantTypeClientCredentials),
			wantErr: true,
		},
		{
			name: "S3WebIdentityMissingEnvVars",
			input: `{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"s3_signing": {
						"web_identity_credentials": {}
					},
				}
			}`,
			wantErr: true,
		},
		{
			name: "S3WebIdentityCreds",
			input: `{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"s3_signing": {
						"web_identity_credentials": {}
					},
				}
			}`,
			env: map[string]string{
				awsRoleArnEnvVar:              "TEST",
				awsWebIdentityTokenFileEnvVar: "TEST",
				awsRegionEnvVar:               "us-west-1",
			},
		},
		{
			name: "S3AssumeRoleMissingEnvVars",
			input: `{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"s3_signing": {
						"assume_role_credentials": {}
					},
				}
			}`,
			wantErr: true,
		},
		{
			name: "S3AssumeRoleCredsMissingSigningPlugin",
			input: `{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"s3_signing": {
						"assume_role_credentials": {}
					},
				}
			}`,
			env: map[string]string{
				awsRoleArnEnvVar: "TEST",
				accessKeyEnvVar:  "TEST",
				secretKeyEnvVar:  "TEST",
				awsRegionEnvVar:  "us-west-1",
			},
			wantErr: true,
		},
		{
			name: "S3AssumeRoleCreds",
			input: `{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"s3_signing": {
						"assume_role_credentials": {"aws_signing": {"environment_credentials": {}}}
					},
				}
			}`,
			env: map[string]string{
				awsRoleArnEnvVar: "TEST",
				accessKeyEnvVar:  "TEST",
				secretKeyEnvVar:  "TEST",
				awsRegionEnvVar:  "us-west-1",
			},
		},
		{
			name: "ValidGCPMetadataIDTokenOptions",
			input: `{
				"name": "foo",
				"url": "https://localhost",
				"credentials": {
					"gcp_metadata": {
						"audience": "https://localhost"
					}
				}
			}`,
		},
		{
			name: "ValidGCPMetadataAccessTokenOptions",
			input: `{
				"name": "foo",
				"url": "https://localhost",
				"credentials": {
					"gcp_metadata": {
						"scopes": ["storage.read_only"]
					}
				}
			}`,
		},
		{
			name: "EmptyGCPMetadataOptions",
			input: `{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"gcp_metadata": {
					}
				}
			}`,
			wantErr: true,
		},
		{
			name: "EmptyGCPMetadataIDTokenAudienceOption",
			input: `{
				"name": "foo",
				"url": "https://localhost",
				"credentials": {
					"gcp_metadata": {
						"audience": ""
					}
				}
			}`,
			wantErr: true,
		},
		{
			name: "EmptyGCPMetadataAccessTokenScopesOption",
			input: `{
				"name": "foo",
				"url": "https://localhost",
				"credentials": {
					"gcp_metadata": {
						"scopes": []
					}
				}
			}`,
			wantErr: true,
		},
		{
			name: "InvalidGCPMetadataOptions",
			input: `{
				"name": "foo",
				"url": "https://localhost",
				"credentials": {
					"gcp_metadata": {
						"audience": "https://localhost",
						"scopes": ["storage.read_only"]
					}
				}
			}`,
			wantErr: true,
		},
		{
			name: "Plugin",
			input: `{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"plugin": "my_plugin"
        }
			}`,
		},
		{
			name: "Unknown plugin",
			input: `{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"plugin": "unknown_plugin"
        }
			}`,
			wantErr: true,
		},
		{
			name: "Oauth2CredsClientAssertionPath",
			input: fmt.Sprintf(`{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"oauth2": {
						"grant_type": %q,
						"token_url": "https://localhost",
						"client_id": "client_one",
						"client_assertion_path": "/some/file",
						"scopes": ["profile", "opa"]
					}
				}
			}`, grantTypeClientCredentials),
		},
		{
			name: "Oauth2CredsClientAssertion",
			input: fmt.Sprintf(`{
				"name": "foo",
				"url": "http://localhost",
				"credentials": {
					"oauth2": {
						"grant_type": %q,
						"token_url": "https://localhost",
						"client_id": "client_one",
						"client_assertion": "assertive",
						"scopes": ["profile", "opa"]
					}
				}
			}`, grantTypeClientCredentials),
		},
	}

	var results []Client

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
	keyPem := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	pubKeyPem := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&key.PublicKey),
	})

	ks := map[string]*keys.Config{
		keyID: {
			PrivateKey: string(keyPem),
			Algorithm:  "RS256",
		},
		"pub_key": {
			Key:       string(pubKeyPem),
			Algorithm: "RS256",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for key, val := range tc.env {
				t.Setenv(key, val)
			}

			client, err := New([]byte(tc.input), ks, AuthPluginLookup(mockAuthPluginLookup))
			if err != nil {
				// We never want an error here and cannot proceed if there is one.
				t.Fatalf("Unexpected error: %v", err)
			}

			plugin, err := client.config.AuthPlugin(mockAuthPluginLookup)
			if err != nil {
				if tc.wantErr {
					return
				}
				t.Fatalf("Unexpected error: %v", err)
			}

			_, err = plugin.NewClient(client.config)
			if err != nil && !tc.wantErr {
				t.Fatalf("Unexpected error: %v", err)
			} else if err == nil && tc.wantErr {
				t.Fatalf("Expected error for input %v", tc.input)
			}

			if *client.config.ResponseHeaderTimeoutSeconds != defaultResponseHeaderTimeoutSeconds {
				t.Fatalf("Expected default response header timeout but got %v seconds", *client.config.ResponseHeaderTimeoutSeconds)
			}

			results = append(results, client)
		})
	}

	if results[3].config.Credentials.Bearer.Scheme != "Acmecorp-Token" {
		t.Fatalf("Expected custom token but got: %v", results[3].config.Credentials.Bearer.Scheme)
	}
}

func TestNewWithResponseHeaderTimeout(t *testing.T) {
	t.Parallel()

	input := `{
				"name": "foo",
				"url": "http://localhost",
				"response_header_timeout_seconds": 20
			}`

	client, err := New([]byte(input), map[string]*keys.Config{})
	if err != nil {
		t.Fatal("Unexpected error")
	}

	if *client.config.ResponseHeaderTimeoutSeconds != 20 {
		t.Fatalf("Expected response header timeout %v seconds but got %v seconds", 20, *client.config.ResponseHeaderTimeoutSeconds)
	}
}

func TestDoWithResponseHeaderTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("too slow for testing.Short")
	}

	t.Parallel()

	ctx := context.Background()

	tests := map[string]struct {
		d                     time.Duration
		responseHeaderTimeout string
		wantErr               bool
		errMsg                string
	}{
		"response_headers_timeout_not_met": {1, "2", false, ""},
		"response_headers_timeout_met":     {2, "1", true, "net/http: timeout awaiting response headers"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {

			baseURL, teardown := getTestServerWithTimeout(tc.d)
			defer teardown()

			config := fmt.Sprintf(`{
				"name": "foo",
				"url": %q,
				"response_header_timeout_seconds": %v,
			}`, baseURL, tc.responseHeaderTimeout)
			ks := map[string]*keys.Config{}
			client, err := New([]byte(config), ks)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			_, err = client.Do(ctx, "GET", "/v1/test")
			if tc.wantErr {
				if err == nil {
					t.Fatal("Expected error but got nil")
				}

				if !strings.Contains(err.Error(), tc.errMsg) {
					t.Fatalf("Expected error %v but got %v", tc.errMsg, err.Error())
				}
			} else if err != nil {
				t.Fatalf("Unexpected error %v", err)
			}
		})
	}
}

type tracemock struct {
	called int
}

func (m *tracemock) NewTransport(rt http.RoundTripper, _ tracing.Options) http.RoundTripper {
	m.called++
	return rt
}

func (*tracemock) NewHandler(http.Handler, string, tracing.Options) http.Handler {
	panic("unreachable")
}

func TestDoWithDistributedTracingOpts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mock := tracemock{}
	tracing.RegisterHTTPTracing(&mock)

	body := "Some Bad Request was received"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, body)
	}))
	defer ts.Close()

	buf := bytes.Buffer{}
	logger := logging.New()
	logger.SetOutput(&buf)
	logger.SetLevel(logging.Debug)

	config := fmt.Sprintf(`{
				"name": "foo",
				"url": %q,
			}`, ts.URL)
	ks := map[string]*keys.Config{}
	client, err := New([]byte(config), ks, DistributedTracingOpts(tracing.Options{"testoption"}))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	_, err = client.Do(ctx, "GET", ts.URL)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if exp, act := 1, mock.called; exp != act {
		t.Errorf("calls to NewTransport: expected %d, got %d", exp, act)
	}
}

func TestDoWithResponseInClientLog(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	body := "Some Bad Request was received"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, body)
	}))
	defer ts.Close()

	buf := bytes.Buffer{}
	logger := logging.New()
	logger.SetOutput(&buf)
	logger.SetLevel(logging.Debug)

	config := fmt.Sprintf(`{
				"name": "foo",
				"url": %q,
			}`, ts.URL)
	ks := map[string]*keys.Config{}
	client, err := New([]byte(config), ks, Logger(logger))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	_, err = client.Do(ctx, "GET", ts.URL)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), body) {
		t.Errorf("expected string %q not found in client logs", body)
	}
}

func TestDoWithTruncatedResponseInClientLog(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, strings.Repeat("Some Bad Request was received", 50))
	}))
	defer ts.Close()

	buf := bytes.Buffer{}
	logger := logging.New()
	logger.SetOutput(&buf)
	logger.SetLevel(logging.Debug)

	config := fmt.Sprintf(`{
				"name": "foo",
				"url": %q,
			}`, ts.URL)
	ks := map[string]*keys.Config{}
	client, err := New([]byte(config), ks, Logger(logger))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	_, err = client.Do(ctx, "GET", ts.URL)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	exp := "Some Bad Request was recei..."
	if !strings.Contains(buf.String(), exp) {
		t.Errorf("expected string %q not found in client logs", exp)
	}
}

func TestValidUrl(t *testing.T) {
	t.Parallel()

	ts := testServer{
		t:         t,
		expMethod: "GET",
		expPath:   "/test",
	}
	ts.start()
	defer ts.stop()
	config := fmt.Sprintf(`{
		"name": "foo",
		"url": %q,
	}`, ts.server.URL)
	client, err := New([]byte(config), map[string]*keys.Config{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	ctx := context.Background()
	if _, err := client.Do(ctx, "GET", "test"); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
}

func testBearerToken(t *testing.T, scheme, token string) {
	ts := testServer{
		t:               t,
		expBearerScheme: scheme,
		expBearerToken:  token,
	}
	ts.start()
	defer ts.stop()
	config := fmt.Sprintf(`{
		"name": "foo",
		"url": %q,
		"credentials": {
			"bearer": {
				"scheme": %q,
				"token": %q
			}
		}
	}`, ts.server.URL, scheme, token)
	ks := map[string]*keys.Config{}
	client, err := New([]byte(config), ks)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	ctx := context.Background()
	if _, err := client.Do(ctx, "GET", "test"); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
}

func TestBearerTokenDefaultScheme(t *testing.T) {
	t.Parallel()

	testBearerToken(t, "", "secret")
}

func TestBearerTokenCustomScheme(t *testing.T) {
	t.Parallel()

	testBearerToken(t, "Acmecorp-Token", "secret")
}

func TestBearerTokenPath(t *testing.T) {
	t.Parallel()

	ts := testServer{
		t:                  t,
		expBearerScheme:    "",
		expBearerToken:     "secret",
		expBearerTokenPath: true,
	}
	ts.start()
	defer ts.stop()

	files := map[string]string{
		"token.txt": "secret",
	}

	test.WithTempFS(files, func(path string) {
		tokenPath := filepath.Join(path, "token.txt")

		client := newTestBearerClient(t, &ts, tokenPath)

		ctx := context.Background()
		if _, err := client.Do(ctx, "GET", "test"); err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Stop server and update the token
		ts.stop()
		ts.expBearerToken = "newsecret"
		ts.start()

		// check client cannot access the server
		client = newTestBearerClient(t, &ts, tokenPath)

		if resp, err := client.Do(ctx, "GET", "test"); err == nil {
			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("Expected http status %v but got %v", http.StatusUnauthorized, resp.StatusCode)
			}

			expectedErrMsg := "Expected bearer token \"newsecret\", got authorization header \"Bearer secret\""

			if string(bodyBytes) != expectedErrMsg {
				t.Fatalf("Expected error message %v but got %v", expectedErrMsg, string(bodyBytes))
			}
		} else {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Update the token file and try again
		if err := os.WriteFile(filepath.Join(path, "token.txt"), []byte("newsecret"), 0600); err != nil {
			t.Fatalf("Unexpected error: %s", err)
		}

		if _, err := client.Do(ctx, "GET", "test"); err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
	})
}

func TestBearerWithCustomCACert(t *testing.T) {
	t.Parallel()

	ts := testServer{
		t:                  t,
		tls:                true,
		expBearerScheme:    "",
		expBearerToken:     "secret",
		expBearerTokenPath: true,
	}
	ts.start()
	defer ts.stop()

	files := map[string]string{
		"token.txt": "secret",
		"ca.pem":    string(ts.rootCertPEM),
	}

	test.WithTempFS(files, func(path string) {
		tokenPath := filepath.Join(path, "token.txt")
		ts.caCert = filepath.Join(path, "ca.pem")

		client := newTestBearerClient(t, &ts, tokenPath)

		ctx := context.Background()
		if _, err := client.Do(ctx, "GET", "test"); err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
	})
}

func TestBearerWithCustomCACertAndSystemCA(t *testing.T) {
	t.Parallel()

	ts := testServer{
		t:                  t,
		tls:                true,
		expBearerScheme:    "",
		expBearerToken:     "secret",
		expBearerTokenPath: true,
		expectSystemCA:     true,
	}
	ts.start()
	defer ts.stop()

	files := map[string]string{
		"token.txt": "secret",
		"ca.pem":    string(ts.rootCertPEM),
	}

	test.WithTempFS(files, func(path string) {
		tokenPath := filepath.Join(path, "token.txt")
		ts.caCert = filepath.Join(path, "ca.pem")

		client := newTestBearerClient(t, &ts, tokenPath)

		ctx := context.Background()
		if _, err := client.Do(ctx, "GET", "test"); err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
	})
}

func TestBearerTokenInvalidConfig(t *testing.T) {
	t.Parallel()

	ts := testServer{
		t:               t,
		expBearerScheme: "",
		expBearerToken:  "secret",
	}
	ts.start()
	defer ts.stop()

	config := fmt.Sprintf(`{
		"name": "foo",
		"url": %q,
		"credentials": {
			"bearer": {
				"token_path": %q,
				"token": %q
			}
		}
	}`, ts.server.URL, "token.txt", "secret")
	client, err := New([]byte(config), map[string]*keys.Config{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	ctx := context.Background()

	_, err = client.Do(ctx, "GET", "test")

	if err == nil {
		t.Fatalf("Expected error but got nil")
	}

	if !strings.HasPrefix(err.Error(), "invalid config") {
		t.Fatalf("Unexpected error message %v\n", err)
	}
}

func TestBearerTokenIsEncodedForOCI(t *testing.T) {
	t.Parallel()

	config := `{
		"name": "foo",
		"type": "oci",
		"credentials": {
			"bearer": {
				"token": "secret",
				"scheme": "Bearer"
			}
		}
	}`

	client, err := New([]byte(config), map[string]*keys.Config{})
	if err != nil {
		t.Fatalf("New() = %q", err)
	}

	if _, err := client.config.Credentials.Bearer.NewClient(client.config); err != nil {
		t.Errorf("Bearer.NewClient() = %q", err)
	}

	req := httptest.NewRequest("", "http://somewhere.com", nil)
	if err := client.config.Credentials.Bearer.Prepare(req); err != nil {
		t.Errorf("Bearer.Prepare() = %q", err)
	}

	token := base64.StdEncoding.EncodeToString([]byte("secret"))

	want := "Bearer " + token
	got := req.Header.Get("Authorization")
	if got != want {
		t.Errorf("req.Header.Get(\"Authorization\") = %q, want = %q", got, want)
	}
}

func newTestBearerClient(t *testing.T, ts *testServer, tokenPath string) *Client {
	config := fmt.Sprintf(`{
			"name": "foo",
			"url": %q,
			"tls": {"ca_cert": %q, system_ca_required: %v},
			"credentials": {
				"bearer": {
					"token_path": %q
				}
			}
		}`, ts.server.URL, ts.caCert, ts.expectSystemCA, tokenPath)
	client, err := New([]byte(config), map[string]*keys.Config{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	return &client
}

func TestClientCert(t *testing.T) {
	t.Parallel()

	ts := testServer{
		t:                t,
		tls:              true,
		expectClientCert: true,
	}
	ts.start()
	defer ts.stop()

	files := map[string]string{
		"client.pem": string(ts.clientCertPem),
		"client.key": string(ts.clientCertKey),
	}

	test.WithTempFS(files, func(path string) {
		certPath := filepath.Join(path, "client.pem")
		keyPath := filepath.Join(path, "client.key")

		client := newTestClient(t, &ts, certPath, keyPath)

		ctx := context.Background()
		if _, err := client.Do(ctx, "GET", "test"); err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Scramble the keys in the server
		ts.stop()
		ts.start()

		// Ensure the keys don't work anymore, make a new client as the url will have changed
		client = newTestClient(t, &ts, certPath, keyPath)
		_, err := client.Do(ctx, "GET", "test")
		expectedErrMsg := func(s string) bool {
			switch {
			case strings.Contains(s, "tls: unknown certificate authority"):
			case strings.Contains(s, "tls: bad certificate"):
			default:
				return false
			}
			return true
		}
		if err == nil || !expectedErrMsg(err.Error()) {
			t.Fatalf("Unexpected error %v", err)
		}

		// Update the key files and try again..
		if err := os.WriteFile(filepath.Join(path, "client.pem"), ts.clientCertPem, 0600); err != nil {
			t.Fatalf("Unexpected error: %s", err)
		}
		if err := os.WriteFile(filepath.Join(path, "client.key"), ts.clientCertKey, 0600); err != nil {
			t.Fatalf("Unexpected error: %s", err)
		}
		if _, err := client.Do(ctx, "GET", "test"); err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
	})
}

func TestClientCertPassword(t *testing.T) {
	t.Parallel()

	ts := testServer{
		t:                  t,
		tls:                true,
		expectClientCert:   true,
		clientCertPassword: "password",
	}
	ts.start()
	defer ts.stop()

	files := map[string]string{
		"client.pem": string(ts.clientCertPem),
		"client.key": string(ts.clientCertKey),
	}

	test.WithTempFS(files, func(path string) {
		certPath := filepath.Join(path, "client.pem")
		keyPath := filepath.Join(path, "client.key")

		client := newTestClient(t, &ts, certPath, keyPath)

		ctx := context.Background()
		if _, err := client.Do(ctx, "GET", "test"); err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
	})
}

func TestClientTLSWithCustomCACert(t *testing.T) {
	t.Parallel()

	ts := testServer{
		t:                t,
		tls:              true,
		expectClientCert: true,
	}
	ts.start()
	defer ts.stop()

	files := map[string]string{
		"client.pem": string(ts.clientCertPem),
		"client.key": string(ts.clientCertKey),
		"ca.pem":     string(ts.rootCertPEM),
	}

	test.WithTempFS(files, func(path string) {
		certPath := filepath.Join(path, "client.pem")
		keyPath := filepath.Join(path, "client.key")
		ts.caCert = filepath.Join(path, "ca.pem")

		client := newTestClient(t, &ts, certPath, keyPath)

		ctx := context.Background()
		if _, err := client.Do(ctx, "GET", "test"); err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
	})
}

func TestClientTLSWithCustomCACertAndSystemCA(t *testing.T) {
	t.Parallel()

	ts := testServer{
		t:                t,
		tls:              true,
		expectClientCert: true,
		expectSystemCA:   true,
	}
	ts.start()
	defer ts.stop()

	files := map[string]string{
		"client.pem": string(ts.clientCertPem),
		"client.key": string(ts.clientCertKey),
		"ca.pem":     string(ts.rootCertPEM),
	}

	test.WithTempFS(files, func(path string) {
		certPath := filepath.Join(path, "client.pem")
		keyPath := filepath.Join(path, "client.key")
		ts.caCert = filepath.Join(path, "ca.pem")

		client := newTestClient(t, &ts, certPath, keyPath)

		ctx := context.Background()
		if _, err := client.Do(ctx, "GET", "test"); err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
	})
}

func TestOauth2ClientCredentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ts      *testServer
		ots     *oauth2TestServer
		options testPluginCustomizer
		wantErr bool
	}{
		{
			ts:  &testServer{t: t, expBearerToken: "token_1"},
			ots: &oauth2TestServer{t: t},
		},
		{
			ts:      &testServer{t: t, expBearerToken: "token_1"},
			ots:     &oauth2TestServer{t: t, tokenType: "unknown"},
			wantErr: true,
		},
		{
			ts:  &testServer{t: t},
			ots: &oauth2TestServer{t: t},
			options: func(c *Config) {
				c.Credentials.OAuth2.ClientSecret = "not_super_secret"
			},
			wantErr: true,
		},
		{
			ts:  &testServer{t: t},
			ots: &oauth2TestServer{t: t, expScope: &[]string{"read", "opa"}},
			options: func(c *Config) {
				c.Credentials.OAuth2.Scopes = []string{"read", "opa"}
			},
		},
		{
			ts:  &testServer{t: t},
			ots: &oauth2TestServer{t: t, expHeaders: map[string]string{"x-custom-header": "custom-value"}},
			options: func(c *Config) {
				c.Credentials.OAuth2.AdditionalHeaders = map[string]string{"x-custom-header": "custom-value"}
			},
		},
		{
			ts:  &testServer{t: t},
			ots: &oauth2TestServer{t: t, expBody: map[string]string{"custom_field": "custom-value"}},
			options: func(c *Config) {
				c.Credentials.OAuth2.AdditionalParameters = map[string]string{"custom_field": "custom-value"}
			},
		},
	}

	for _, tc := range tests {
		func() {
			tc.ts.start()
			defer tc.ts.stop()
			tc.ots.start()
			defer tc.ots.stop()

			if tc.options == nil {
				tc.options = func(_ *Config) {}
			}

			client := newOauth2TestClient(t, tc.ts, tc.ots, tc.options)
			ctx := context.Background()
			_, err := client.Do(ctx, "GET", "test")
			if err != nil && !tc.wantErr {
				t.Fatalf("Unexpected error: %v", err)
			} else if err == nil && tc.wantErr {
				t.Fatalf("Expected error: %v", err)
			}
		}()
	}
}

func TestOauth2ClientCredentialsExpiringTokenIsRefreshed(t *testing.T) {
	t.Parallel()

	ts := testServer{
		t:              t,
		expBearerToken: "token_1",
	}
	ts.start()
	ots := oauth2TestServer{
		t: t,
		// Issue tokens with a TTL below our considered minimum - this should force the client to fetch a new one the
		// second time the credentials are used rather than reusing the token it has
		tokenTTL: 9,
	}
	ots.start()
	defer ots.stop()

	client := newOauth2TestClient(t, &ts, &ots)
	ctx := context.Background()
	_, err := client.Do(ctx, "GET", "test")
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	ts.stop()
	ts = testServer{
		t:              t,
		expBearerToken: "token_2",
	}
	ts.start()
	defer ts.stop()

	client = newOauth2TestClient(t, &ts, &ots)
	ctx = context.Background()
	_, err = client.Do(ctx, "GET", "test")
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
}

func TestOauth2ClientCredentialsNonExpiringTokenIsReused(t *testing.T) {
	t.Parallel()

	ts := testServer{
		t:              t,
		expBearerToken: "token_1",
	}
	ts.start()
	defer ts.stop()

	ots := oauth2TestServer{
		t:        t,
		tokenTTL: 300,
	}
	ots.start()
	defer ots.stop()

	client := newOauth2TestClient(t, &ts, &ots)
	ctx := context.Background()
	_, err := client.Do(ctx, "GET", "test")
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	_, err = client.Do(ctx, "GET", "test")
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
}

func TestOauth2JwtBearerGrantType(t *testing.T) {
	t.Parallel()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
	keyPem := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	ks := map[string]*keys.Config{
		keyID: {
			PrivateKey: string(keyPem),
			Algorithm:  "RS256",
		},
	}

	ts := testServer{t: t, expBearerToken: "token_1"}
	ts.start()
	defer ts.stop()

	ots := oauth2TestServer{
		t:                t,
		tokenTTL:         300,
		expGrantType:     "urn:ietf:params:oauth:grant-type:jwt-bearer",
		expScope:         &[]string{"scope1", "scope2"},
		expJwtCredential: true,
		expAlgorithm:     jwa.RS256,
		verificationKey:  &key.PublicKey,
	}
	ots.start()
	defer ots.stop()

	client := newOauth2JwtBearerTestClient(t, ks, &ts, &ots, func(c *Config) {
		c.Credentials.OAuth2.SigningKeyID = keyID
	})
	ctx := context.Background()
	_, err = client.Do(ctx, "GET", "test")
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
}

func TestOauth2JwtBearerGrantTypePKCS8EncodedPrivateKey(t *testing.T) {
	t.Parallel()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	privateKey, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	keyPem := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privateKey})
	ks := map[string]*keys.Config{
		keyID: {
			PrivateKey: string(keyPem),
			Algorithm:  "RS256",
		},
	}

	ts := testServer{t: t, expBearerToken: "token_1"}
	ts.start()
	defer ts.stop()

	ots := oauth2TestServer{
		t:                t,
		tokenTTL:         300,
		expGrantType:     "urn:ietf:params:oauth:grant-type:jwt-bearer",
		expScope:         &[]string{"scope1", "scope2"},
		expJwtCredential: true,
		expAlgorithm:     jwa.RS256,
		verificationKey:  &key.PublicKey,
	}
	ots.start()
	defer ots.stop()

	client := newOauth2JwtBearerTestClient(t, ks, &ts, &ots, func(c *Config) {
		c.Credentials.OAuth2.SigningKeyID = keyID
	})
	ctx := context.Background()
	_, err = client.Do(ctx, "GET", "test")
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
}

func TestOauth2JwtBearerGrantTypeEllipticCurveAlgorithm(t *testing.T) {
	t.Parallel()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
	privateKey, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	keyPem := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privateKey})
	ks := map[string]*keys.Config{
		keyID: {
			PrivateKey: string(keyPem),
			Algorithm:  "ES256",
		},
	}

	ts := testServer{t: t, expBearerToken: "token_1"}
	ts.start()
	defer ts.stop()

	ots := oauth2TestServer{
		t:                t,
		tokenTTL:         300,
		expGrantType:     "urn:ietf:params:oauth:grant-type:jwt-bearer",
		expScope:         &[]string{"scope1", "scope2"},
		expJwtCredential: true,
		expAlgorithm:     jwa.ES256,
		verificationKey:  &key.PublicKey,
	}
	ots.start()
	defer ots.stop()

	client := newOauth2JwtBearerTestClient(t, ks, &ts, &ots, func(c *Config) {
		c.Credentials.OAuth2.SigningKeyID = keyID
		c.Credentials.OAuth2.IncludeJti = true
	})
	ctx := context.Background()
	_, err = client.Do(ctx, "GET", "test")
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	_, err = client.Do(ctx, "GET", "test")
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
}

func TestOauth2ClientCredentialsJwtAuthentication(t *testing.T) {
	t.Parallel()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
	keyPem := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	ks := map[string]*keys.Config{
		keyID: {
			PrivateKey: string(keyPem),
			Algorithm:  "RS256",
		},
	}

	ts := testServer{t: t, expBearerToken: "token_1"}
	ts.start()
	defer ts.stop()

	ots := oauth2TestServer{
		t:                t,
		tokenTTL:         300,
		expGrantType:     grantTypeClientCredentials,
		expScope:         &[]string{"scope1", "scope2"},
		expX5t:           "jxvd3pmCKZ5idJwg7duqxX9hnQQ=",
		expJwtCredential: true,
		expAlgorithm:     jwa.RS256,
		verificationKey:  &key.PublicKey,
	}
	ots.start()
	defer ots.stop()

	client := newOauth2ClientCredentialsJwtAuthClient(t, ks, &ts, &ots, func(c *Config) {
		c.Credentials.OAuth2.SigningKeyID = keyID
	})
	ctx := context.Background()
	_, err = client.Do(ctx, "GET", "test")
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	_, err = client.Do(ctx, "GET", "test")
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
}

// https://github.com/IUAD1IY7/opa/issues/3255
func TestS3SigningInstantiationInitializesLogger(t *testing.T) {
	t.Parallel()

	config := `{
			"name": "foo",
			"url": "https://bundles.example.com",
			"credentials": {
				"s3_signing": {
					"environment_credentials": {}
				}
			}
		}`

	authPlugin := &awsSigningAuthPlugin{
		AWSEnvironmentCredentials: &awsEnvironmentCredentialService{},
	}
	client, err := New([]byte(config), map[string]*keys.Config{}, AuthPluginLookup(func(_ string) HTTPAuthPlugin {
		return authPlugin
	}))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	plugin := client.authPluginLookup("s3_signing")
	if _, err = plugin.NewClient(client.config); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if authPlugin.logger == nil {
		t.Errorf("Expected logger to be initialized")
	}
}

func TestS3SigningMultiCredentialProvider(t *testing.T) {
	t.Parallel()

	credentialProviderCount := 4
	config := `{
		"name": "foo",
		"url": "https://bundles.example.com",
		"credentials": {
			"s3_signing": {
				"environment_credentials": {},
				"profile_credentials": {},
				"metadata_credentials": {},
				"web_identity_credentials": {}
			}
		}
	}`

	client, err := New([]byte(config), map[string]*keys.Config{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	awsPlugin := client.config.Credentials.S3Signing
	if awsPlugin == nil {
		t.Fatalf("Client config S3 signing credentials setup unexpected")
	}

	awsCredentialServiceChain, ok := awsPlugin.awsCredentialService().(*awsCredentialServiceChain)
	if !ok {
		t.Fatalf("Unexpected AWS credential service:%v is not a chain",
			reflect.TypeOf(awsCredentialServiceChain))
	}

	if len(awsCredentialServiceChain.awsCredentialServices) != credentialProviderCount {
		t.Fatalf("Credential provider count mismatch %d != %d", credentialProviderCount,
			len(awsCredentialServiceChain.awsCredentialServices))
	}

	expectedOrder := []awsCredentialService{
		&awsEnvironmentCredentialService{},
		&awsWebIdentityCredentialService{},
		&awsProfileCredentialService{},
		&awsMetadataCredentialService{},
	}

	if !reflect.DeepEqual(awsCredentialServiceChain.awsCredentialServices,
		expectedOrder) {
		t.Fatalf("Ordering is unexpected")
	}
}

func TestAWSCredentialServiceChain(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		env     map[string]string
		errMsg  string
	}{
		{
			name: "Fallback to Environment Credential",
			input: `{
				"name": "foo",
				"url": "https://bundles.example.com",
				"credentials": {
					"s3_signing": {
						"web_identity_credentials": {},
						"environment_credentials": {},
						"profile_credentials": {},
						"metadata_credentials": {}
					}
				}
			}`,
			wantErr: false,
			env: map[string]string{
				accessKeyEnvVar: "a",
				secretKeyEnvVar: "a",
				awsRegionEnvVar: "us-east-1",
			},
		},
		{
			name: "No provider is successful",
			input: `{
				"name": "foo",
				"url": "https://bundles.example.com",
				"credentials": {
					"s3_signing": {
						"web_identity_credentials": {},
						"environment_credentials": {},
						"profile_credentials": {},
						"metadata_credentials": {}
					}
				}
			}`,
			wantErr: true,
			errMsg:  "all AWS credential providers failed: 4 errors occurred",
			env:     map[string]string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for key, val := range tc.env {
				t.Setenv(key, val)
			}

			t.Cleanup(func() {
				for key := range tc.env {
					_ = os.Unsetenv(key)
				}
			})

			client, err := New([]byte(tc.input), map[string]*keys.Config{})
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			awsPlugin := client.config.Credentials.S3Signing
			if awsPlugin == nil {
				t.Fatalf("Client config S3 signing credentials setup unexpected")
			}

			req, err := http.NewRequest("GET", "/example/bundle.tar.gz", nil)
			if err != nil {
				t.Fatalf("Failed to create HTTP request: %v", err)
			}

			awsPlugin.logger = client.logger
			err = awsPlugin.Prepare(req)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("Expected error for input %v", tc.input)
				}

				if !strings.Contains(err.Error(), tc.errMsg) {
					t.Fatalf("Expected error message %v but got %v", tc.errMsg, err.Error())
				}
			} else if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
		})
	}
}

func TestDebugLoggingRequestMaskAuthorizationHeader(t *testing.T) {
	t.Parallel()

	token := "secret"
	plaintext := "plaintext"
	ts := testServer{t: t, expBearerToken: token}
	ts.start()
	defer ts.stop()

	config := fmt.Sprintf(`{
		"name": "foo",
		"url": %q,
		"credentials": {
			"bearer": {
				"token": %q
			}
		},
		"headers": {
			"X-AMZ-SECURITY-TOKEN": %q,
			"remains-unmasked": %q
		}
	}`, ts.server.URL, token, token, plaintext)
	client, err := New([]byte(config), map[string]*keys.Config{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	logger := testlogger.New()
	logger.SetLevel(logging.Debug)
	client.logger = logger

	ctx := context.Background()
	if _, err := client.Do(ctx, "GET", "test"); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	entries := logger.Entries()
	if len(entries) != 2 {
		t.Fatalf("Expected 2 log entries, got %d", len(entries))
	}

	requestEntry := entries[0]
	headers := requestEntry.Fields["headers"].(http.Header)
	for k := range headers {
		v := headers.Get(k)
		if _, ok := maskedHeaderKeys[k]; ok {
			if v != "REDACTED" {
				t.Errorf("Expected redacted %q header value, got %v", k, v)
			}
		} else if k == "Remains-Unmasked" && v != plaintext {
			t.Errorf("Expected %q header to have value %q, got %v", k, plaintext, v)
		}
	}
}

func newTestClient(t *testing.T, ts *testServer, certPath string, keypath string) *Client {
	config := fmt.Sprintf(`{
			"name": "foo",
			"url": %q,
			"allow_insecure_tls": true,
			"tls": {"ca_cert": %q, system_ca_required: %v},
			"credentials": {
				"client_tls": {
					"cert": %q,
					"private_key": %q
				}
			}
		}`, ts.server.URL, ts.caCert, ts.expectSystemCA, certPath, keypath)
	client, err := New([]byte(config), map[string]*keys.Config{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if ts.clientCertPassword != "" {
		client.Config().Credentials.ClientTLS.PrivateKeyPassphrase = ts.clientCertPassword
	}

	return &client
}

type testPluginCustomizer func(c *Config)

type testServer struct {
	t                  *testing.T
	server             *httptest.Server
	expPath            string
	expMethod          string
	expBearerToken     string
	expBearerScheme    string
	expBearerTokenPath bool
	tls                bool
	clientCertPem      []byte
	clientCertKey      []byte
	clientCertPassword string
	expectClientCert   bool
	rootCertPEM        []byte
	caCert             string
	expectSystemCA     bool
	serverCertPool     *x509.CertPool
	certificates       []tls.Certificate
}

type oauth2TestServer struct {
	t                *testing.T
	server           *httptest.Server
	expGrantType     string
	expClientID      string
	expClientSecret  string
	expHeaders       map[string]string
	expBody          map[string]string
	expJwtCredential bool
	expScope         *[]string
	expAlgorithm     jwa.SignatureAlgorithm
	expX5t           string
	expSignature     string
	tokenType        string
	tokenTTL         int64
	invocations      int32
	verificationKey  any
}

func newOauth2TestClient(t *testing.T, ts *testServer, ots *oauth2TestServer, options ...testPluginCustomizer) *Client {
	config := fmt.Sprintf(`{
			"name": "foo",
			"url": %q,
			"allow_insecure_tls": true,
			"credentials": {
				"oauth2": {
					"token_url": "%v/token",
					"client_id": "client_one",
					"client_secret": "super_secret"
				}
			}
		}`, ts.server.URL, ots.server.URL)
	client, err := New([]byte(config), map[string]*bundle.KeyConfig{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	for _, option := range options {
		option(client.Config())
	}

	return &client
}

// Create client to test JWT authorization grant as described in https://tools.ietf.org/html/rfc7523
func newOauth2JwtBearerTestClient(t *testing.T, keys map[string]*keys.Config, ts *testServer, ots *oauth2TestServer, options ...testPluginCustomizer) *Client {
	config := fmt.Sprintf(`{
			"name": "foo",
			"url": %q,
			"allow_insecure_tls": true,
			"credentials": {
				"oauth2": {
					"token_url": "%v/token",
					"grant_type": %q,
					"scopes": ["scope1", "scope2"],
					"additional_claims": {
						"aud": "test-audience",
						"iss": "client-one"
					}
				}
			}
		}`, ts.server.URL, ots.server.URL, grantTypeJwtBearer)

	client, err := New([]byte(config), keys)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	for _, option := range options {
		option(client.Config())
	}

	return &client
}

// Create client to test JWT client authentication as described in https://tools.ietf.org/html/rfc7523
func newOauth2ClientCredentialsJwtAuthClient(t *testing.T, keys map[string]*keys.Config, ts *testServer, ots *oauth2TestServer, options ...testPluginCustomizer) *Client {
	config := fmt.Sprintf(`{
			"name": "foo",
			"url": %q,
			"allow_insecure_tls": true,
			"credentials": {
				"oauth2": {
					"token_url": "%v/token",
					"grant_type": %q,
					"signing_key": "key1",
					"client_id": "client-one",
					"scopes": ["scope1", "scope2"],
					"thumbprint": "8F1BDDDE9982299E62749C20EDDBAAC57F619D04",
					"additional_claims": {
						"aud": "test-audience",
						"iss": "client-one"
					}
				}
			}
		}`, ts.server.URL, ots.server.URL, grantTypeClientCredentials)

	client, err := New([]byte(config), keys)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	for _, option := range options {
		option(client.Config())
	}

	return &client
}

func (t *oauth2TestServer) start() {
	if t.tokenTTL == 0 {
		t.tokenTTL = 3600
	}
	if t.expScope == nil {
		t.expScope = &[]string{}
	}
	if t.tokenType == "" {
		t.tokenType = "bearer"
	}
	t.expClientID = "client_one"
	t.expClientSecret = "super_secret"

	t.server = httptest.NewUnstartedServer(http.HandlerFunc(t.handle))

	rootKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.t.Fatalf("generating random key: %v", err)
	}
	_, rootCertPem, err := createRootCert(rootKey)
	if err != nil {
		t.t.Fatalf("creating root cert: %v", err)
	}

	serverCertPool := x509.NewCertPool()
	serverCertPool.AppendCertsFromPEM(rootCertPem)
	t.server.TLS = &tls.Config{
		RootCAs: serverCertPool,
	}
	t.server.StartTLS()
}

func (t *oauth2TestServer) stop() {
	t.server.Close()
}

func (t *oauth2TestServer) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		t.t.Fatalf("Expected method POST, got %v", r.Method)
	}
	if r.URL.Path != "/token" {
		t.t.Fatalf("Expected path /token got %q", r.URL.Path)
	}

	if err := r.ParseForm(); err != nil {
		t.t.Fatal(err)
	}

	if t.expGrantType == "" {
		t.expGrantType = grantTypeClientCredentials
	}
	if r.Form["grant_type"][0] != t.expGrantType {
		t.t.Fatalf("Expected grant_type=%v", t.expGrantType)
	}

	for k, v := range t.expBody {
		if r.Form[k][0] != v {
			t.t.Fatalf("Expected header %s=%s got %s", k, v, r.Form[k][0])
		}
	}

	for k, v := range t.expHeaders {
		if r.Header.Get(k) != v {
			t.t.Fatalf("Expected header %s=%s got %s", k, v, r.Header.Get(k))
		}
	}

	if len(r.Form["scope"]) > 0 {
		scope := strings.Split(r.Form["scope"][0], " ")
		if !slices.Equal(*t.expScope, scope) {
			t.t.Fatalf("Expected scope %v, got %v", *t.expScope, scope)
		}
	} else if t.expScope != nil && len(*t.expScope) > 0 {
		t.t.Fatal("Expected scope to be provided")
	}

	if !t.expJwtCredential {
		authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
		split := strings.Split(authHeader, " ")
		credentials := split[len(split)-1]

		decoded, err := base64.StdEncoding.DecodeString(credentials)
		if err != nil {
			t.t.Fatal(err)
		}

		pair := strings.SplitN(string(decoded), ":", 2)
		if len(pair) != 2 || pair[0] != t.expClientID || pair[1] != t.expClientSecret {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error"": "invalid_client"}`))
			return
		}
	} else {
		var token string
		if t.expGrantType == "urn:ietf:params:oauth:grant-type:jwt-bearer" {
			token = r.Form["assertion"][0]
		} else {
			token = r.Form["client_assertion"][0]
		}
		if t.expSignature != "" {
			signature := strings.Split(token, ".")[2]
			if t.expSignature != signature {
				t.t.Errorf("Expected expSignature %v, got %v", t.expSignature, signature)
			}
		} else {
			_, err := jws.Verify([]byte(token), t.expAlgorithm, t.verificationKey)
			if err != nil {
				t.t.Fatalf("Unexpected signature verification error %v", err)
			}
		}
		if t.expX5t != "" {
			headerRaw, _ := base64.RawURLEncoding.DecodeString(strings.Split(token, ".")[0])
			var headers map[string]string
			_ = json.Unmarshal(headerRaw, &headers)
			x5t := headers["x5t"]

			if t.expX5t != x5t {
				t.t.Errorf("Expected expX5t %v, got %v", t.expX5t, x5t)
			}
		}
	}

	t.invocations++
	token := fmt.Sprintf("token_%v", t.invocations)

	w.WriteHeader(http.StatusOK)
	body := fmt.Sprintf(`{"token_type": "%v", "access_token": "%v", "expires_in": %v}`, t.tokenType, token, t.tokenTTL)
	_, _ = w.Write([]byte(body))
}

func (t *testServer) handle(w http.ResponseWriter, r *http.Request) {
	if t.expMethod != "" && t.expMethod != r.Method {
		t.t.Fatalf("Expected method %v, got %v", t.expMethod, r.Method)
	}
	if t.expPath != "" && t.expPath != r.URL.Path {
		t.t.Fatalf("Expected path %q, got %q", t.expPath, r.URL.Path)
	}
	if (t.expBearerToken != "" || t.expBearerScheme != "") && len(r.Header["Authorization"]) == 0 {
		t.t.Fatal("Expected bearer token, but didn't get any")
	}
	if len(r.Header["Authorization"]) > 0 {
		auth := r.Header["Authorization"][0]
		if t.expBearerScheme != "" && !strings.HasPrefix(auth, t.expBearerScheme) {
			errMsg := fmt.Sprintf("Expected bearer scheme %q, got authorization header %q", t.expBearerScheme, auth)
			if t.expBearerTokenPath {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(errMsg))
				return
			}
			t.t.Fatal(errMsg)
		}
		if t.expBearerToken != "" && !strings.HasSuffix(auth, t.expBearerToken) {
			errMsg := fmt.Sprintf("Expected bearer token %q, got authorization header %q", t.expBearerToken, auth)
			if t.expBearerTokenPath {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(errMsg))
				return
			}
			t.t.Fatal(errMsg)
		}
	}
	if t.expectClientCert {
		if len(r.TLS.PeerCertificates) == 0 {
			t.t.Fatal("Expected client certificate but didn't get any")
		}
	}
	ua := r.Header.Get("user-Agent")
	if ua != version.UserAgent {
		t.t.Errorf("Unexpected User-Agent string: %s", ua)
	}

	w.WriteHeader(200)
}

func (t *testServer) generateClientKeys() {
	// generate a new set of root key+cert objects
	rootKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.t.Fatalf("generating random key: %v", err)
	}
	rootCert, rootCertPEM, err := createRootCert(rootKey)
	if err != nil {
		t.t.Fatalf("error creating cert: %v", err)
	}

	keyPEMBlock := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rootKey)})
	cert, err := tls.X509KeyPair(rootCertPEM, keyPEMBlock)
	if err != nil {
		t.t.Fatalf("error creating tls.X509KeyPair: %v", err)
	}

	// save a copy of the root certificate for clients to use
	t.serverCertPool = x509.NewCertPool()
	t.serverCertPool.AppendCertsFromPEM(rootCertPEM)
	t.rootCertPEM = rootCertPEM
	t.certificates = []tls.Certificate{cert}

	// create a key-pair for the client
	clientKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.t.Fatalf("generating random key: %v", err)
	}

	// create a template for the client
	clientCertTmpl, err := certTemplate()
	if err != nil {
		t.t.Fatalf("creating cert template: %v", err)
	}
	clientCertTmpl.KeyUsage = x509.KeyUsageDigitalSignature
	clientCertTmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}

	// the root cert signs the client cert
	_, t.clientCertPem, err = createCert(clientCertTmpl, rootCert, &clientKey.PublicKey, rootKey)
	if err != nil {
		t.t.Fatalf("error creating cert: %v", err)
	}

	var pemBlock *pem.Block
	if t.clientCertPassword != "" {
		// nolint: staticcheck // We don't want to forbid users from using this encryption.
		pemBlock, err = x509.EncryptPEMBlock(rand.Reader, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(clientKey),
			[]byte(t.clientCertPassword), x509.PEMCipherAES128)
		if err != nil {
			t.t.Fatalf("error encrypting pem block: %v", err)
		}
	} else {
		pemBlock = &pem.Block{
			Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(clientKey),
		}
	}

	// encode and load the cert and private key for the client
	t.clientCertKey = pem.EncodeToMemory(pemBlock)
}

func (t *testServer) start() {
	t.server = httptest.NewUnstartedServer(http.HandlerFunc(t.handle))

	if t.tls {
		t.generateClientKeys()
		t.server.TLS = &tls.Config{
			ClientAuth:   tls.VerifyClientCertIfGiven,
			ClientCAs:    t.serverCertPool,
			Certificates: t.certificates,
		}
		t.server.StartTLS()
	} else {
		t.server.Start()
	}
}

func (t *testServer) stop() {
	t.server.Close()
}

// helper function to create a cert template with a serial number and other required fields
func certTemplate() (*x509.Certificate, error) {
	// generate a random serial number (a real cert authority would have some logic behind this)
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, errors.New("failed to generate serial number: " + err.Error())
	}

	tmpl := x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               pkix.Name{Organization: []string{"OPA"}},
		SignatureAlgorithm:    x509.SHA256WithRSA,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour), // valid for an hour
		BasicConstraintsValid: true,
	}
	return &tmpl, nil
}

func createCert(template, parent *x509.Certificate, pub any, parentPriv any) (
	cert *x509.Certificate, certPEM []byte, err error) {

	certDER, err := x509.CreateCertificate(rand.Reader, template, parent, pub, parentPriv)
	if err != nil {
		return
	}
	// parse the resulting certificate so we can use it again
	cert, err = x509.ParseCertificate(certDER)
	if err != nil {
		return
	}
	// PEM encode the certificate (this is a standard TLS encoding)
	b := pem.Block{Type: "CERTIFICATE", Bytes: certDER}
	certPEM = pem.EncodeToMemory(&b)
	return
}

func createRootCert(rootKey *rsa.PrivateKey) (cert *x509.Certificate, certPEM []byte, err error) {
	rootCertTmpl, err := certTemplate()
	if err != nil {
		return nil, nil, err
	}
	rootCertTmpl.IsCA = true
	rootCertTmpl.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature
	rootCertTmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
	rootCertTmpl.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}

	return createCert(rootCertTmpl, rootCertTmpl, &rootKey.PublicKey, rootKey)
}

func getTestServerWithTimeout(d time.Duration) (baseURL string, teardownFn func()) {
	mux := http.NewServeMux()
	ts := httptest.NewServer(mux)

	mux.HandleFunc("/v1/test", func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(d * time.Second)
		w.WriteHeader(http.StatusOK)
	})
	return ts.URL, ts.Close
}

func mockAuthPluginLookup(name string) HTTPAuthPlugin {
	if name == "my_plugin" {
		return &myPluginMock{}
	}
	return nil
}

type myPluginMock struct{}

func (*myPluginMock) NewClient(c Config) (*http.Client, error) {
	tlsConfig, err := DefaultTLSConfig(c)
	if err != nil {
		return nil, err
	}
	return DefaultRoundTripperClient(
		tlsConfig,
		defaultResponseHeaderTimeoutSeconds,
	), nil
}
func (*myPluginMock) Prepare(*http.Request) error {
	return nil
}

// Note(philipc): Cannot run this test in parallel, due to the t.Setenv calls
// from one of its helper methods.
func TestOauth2ClientCredentialsGrantTypeWithKms(t *testing.T) {

	// DER-encoded object from KMS as explained here: https://docs.aws.amazon.com/kms/latest/APIReference/API_Sign.html#API_Sign_ResponseSyntax
	derEncodeSignature := []byte{48, 68, 2, 32, 84, 124, 17, 255, 68, 181, 189, 159, 77, 235, 242, 88, 85, 139, 84, 111, 204, 108, 235, 90, 128, 220, 247, 176, 215, 28, 188, 110, 19, 158, 137, 30, 2, 32, 88, 17, 176, 72, 157, 42, 1, 223, 69, 41, 225, 77, 121, 13, 117, 132, 146, 243, 45, 208, 207, 119, 233, 156, 96, 94, 192, 174, 136, 218, 206, 84}
	// The signature representing the above object
	jwtSignature := "VHwR_0S1vZ9N6_JYVYtUb8xs61qA3Pew1xy8bhOeiR5YEbBInSoB30Up4U15DXWEkvMt0M936ZxgXsCuiNrOVA"

	ts := testServer{t: t, expBearerToken: "token_1"}
	ts.start()
	defer ts.stop()

	ots := oauth2TestServer{
		t:                t,
		tokenTTL:         300,
		expScope:         &[]string{"scope1", "scope2"},
		expJwtCredential: true,
		expAlgorithm:     jwa.ES256,
		expGrantType:     grantTypeClientCredentials,
		expSignature:     jwtSignature,
	}
	ots.start()
	defer ots.stop()

	kmsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var signRequest = &aws.KMSSignRequest{}
		if r.Body != nil {
			bodyBytes, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("failed to read kms sign request = %v", err)
			}
			defer r.Body.Close()
			err = json.Unmarshal(bodyBytes, signRequest)
			if err != nil {
				t.Fatalf("failed to unmarshall kms sign request = %v", err)
			}
		}
		responseFmt := `{"KeyId": "%s", "Signature": "%s", "SigningAlgorithm": "%s"}`
		responsePayload := fmt.Sprintf(responseFmt, signRequest.KeyID, base64.StdEncoding.EncodeToString(derEncodeSignature), signRequest.SigningAlgorithm)
		if _, err := io.WriteString(w, responsePayload); err != nil {
			t.Fatalf("io.WriteString(w, payload) = %v", err)
		}
	}))
	defer kmsServer.Close()

	logger := logging.New()
	logger.SetLevel(logging.Debug)

	kms := aws.NewKMSWithURLClient(kmsServer.URL, kmsServer.Client(), logger)
	client := newOauth2KmsClientCredentialsTestClient(t, &ts, &ots, kms)
	ctx := context.Background()
	_, err := client.Do(ctx, "GET", "test")
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	_, err = client.Do(ctx, "GET", "test")
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
}

// Create client to test ClientCredentials grant using KMS
func newOauth2KmsClientCredentialsTestClient(t *testing.T, ts *testServer, ots *oauth2TestServer, kms *aws.KMS) *Client {
	config := fmt.Sprintf(`{
			"name": "foo",
			"url": %q,
			"allow_insecure_tls": true,
			"credentials": {
				"oauth2": {
					"token_url": "%v/token",
					"grant_type": %q,
					"scopes": ["scope1", "scope2"],
					"additional_claims": {
						"aud": "test-audience",
						"iss": "client-one"
					},
					"aws_kms": {
						"name": "arn:aws:kms:eu-west-1:account_no:key/key_id",
						"algorithm": "ECDSA_SHA_256"
					},
					"aws_signing": {
						"service": "kms",
						"environment_credentials": {
							"aws_default_region": "eu-west-1"
						}
					}
				}
			}
		}`, ts.server.URL, ots.server.URL, grantTypeClientCredentials)

	// Setup variables for environment_credentials{}
	t.Setenv(accessKeyEnvVar, accessKeyEnvVar)
	t.Setenv(secretKeyEnvVar, secretKeyEnvVar)
	t.Setenv(awsRegionEnvVar, awsRegionEnvVar)

	client, err := New([]byte(config), map[string]*keys.Config{})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if _, err := client.config.Credentials.OAuth2.NewClient(client.config); err != nil {
		t.Fatalf("OAuth2.NewClient() = %q", err)
	}

	if client.config.Credentials.OAuth2.AWSSigningPlugin.kmsSignPlugin == nil {
		t.Errorf("OAuth2.AWSSigningPlugin.kmsSignPlugin isn't setup")
	}

	// setup fake KMS signer
	client.config.Credentials.OAuth2.AWSSigningPlugin.kmsSignPlugin.kms = kms
	return &client
}

func TestOauth2ClientCredentialsGrantTypeWithKeyVault(t *testing.T) {
	sign := "KMUFsIDTnFmyG3nMiGM6H9FNFUROf3wh7SmqJp-QV30"
	ts := testServer{t: t, expBearerToken: "token_1"}
	ts.start()
	defer ts.stop()

	ots := oauth2TestServer{
		t:                t,
		tokenTTL:         300,
		expScope:         &[]string{"scope1", "scope2"},
		expJwtCredential: true,
		expAlgorithm:     "ES256",
		expGrantType:     grantTypeClientCredentials,
		expSignature:     sign,
	}
	ots.start()
	defer ots.stop()

	kvServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body == nil {
			t.Fatal("got keyvault sign request but body is missing")
		}
		defer r.Body.Close()
		var signRequest kvRequest
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read keyvault sign request = %v", err)
		}

		err = json.Unmarshal(bodyBytes, &signRequest)
		if err != nil {
			t.Fatalf("failed to unmarshal keyvault sign request = %v", err)
		}

		resp, err := json.Marshal(kvResponse{KID: "some-KID", Value: sign})
		if err != nil {
			t.Fatalf("json.Marshal(kvResponse{}) = %v", err)
		}
		w.WriteHeader(200)
		_, err = w.Write(resp)
		if err != nil {
			t.Fatalf("w.Write(resp) = %v", err)
		}
	}))
	defer kvServer.Close()

	tokenerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := json.Marshal(azureManagedIdentitiesToken{AccessToken: "token1"})
		if err != nil {
			t.Fatalf("json.Marshal(azureManagedIdentitiesToken{}) = %v", err)
		}
		w.WriteHeader(200)
		_, err = w.Write(b)
		if err != nil {
			t.Fatalf("w.Write(b) = %v", err)
		}
	}))

	kvURL, err := url.Parse(kvServer.URL)
	if err != nil {
		t.Fatalf("url.Parse(kvServer.URL) = %v", err)
	}
	tokenerServerURL, err := url.Parse(tokenerServer.URL)
	if err != nil {
		t.Fatalf("url.Parse(tokenerServer.URL) = %v", err)
	}
	fakeCfg := azureKeyVaultConfig{
		URL: kvURL,
		Alg: "ES256",
	}

	fakePlugin := &azureSigningAuthPlugin{
		MIAuthPlugin:   &azureManagedIdentitiesAuthPlugin{Endpoint: tokenerServerURL.String()},
		Service:        "keyvault",
		keyVaultConfig: &fakeCfg,
		keyVaultSignPlugin: &azureKeyVaultSignPlugin{
			tokener: func() (string, error) { return "azure_tkn", nil },
			config:  fakeCfg,
		},
	}

	client := newOauth2AzureKVClient(t, &ts, &ots, tokenerServer, fakePlugin)
	_, err = client.Do(context.Background(), http.MethodGet, "test")
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
}

func newOauth2AzureKVClient(t *testing.T, ts *testServer, ots *oauth2TestServer, tkn *httptest.Server, azureSign *azureSigningAuthPlugin) *Client {
	cfg := fmt.Sprintf(`{
				"name": "foo",
				"url": %q,
				"allow_insecure_tls": true,
				"credentials": {
					"oauth2": {
            "token_url": "%v/token",
						"grant_type": %q,
						"azure_keyvault": {
            	"key": "tester-key",
          		"key_algorithm": "ES256",
          		"vault": "my-secret-kv"
          	},
            "azure_signing": {
            	"service": "keyvault",
              "azure_managed_identity": {
								"endpoint": "%v"
							}
            },
						"scopes": ["scope1", "scope2"],
						"additional_claims": {
							"aud": "test-audience",
							"iss": "client-one"
						}
					}
				}
			}`, ts.server.URL, ots.server.URL, grantTypeClientCredentials, tkn.URL)
	client, err := New([]byte(cfg), map[string]*keys.Config{})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if _, err := client.config.Credentials.OAuth2.NewClient(client.config); err != nil {
		t.Fatalf("Oauth2.NewClient() = %q", err)
	}

	if client.config.Credentials.OAuth2.AzureSigningPlugin.keyVaultSignPlugin == nil {
		t.Errorf("Oauth2.AzureSigningPlugin.keyVaultSignPlugin isn't setup")
	}

	// setup fake KV config and signer
	client.config.Credentials.OAuth2.AzureKeyVault = azureSign.keyVaultConfig
	client.config.Credentials.OAuth2.AzureSigningPlugin = azureSign
	return &client
}

// Copyright 2019 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package rest

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/IUAD1IY7/opa/internal/providers/aws"
	"github.com/IUAD1IY7/opa/v1/logging"
	"github.com/IUAD1IY7/opa/v1/util/test"
)

// this is usually private; but we need it here
type metadataPayload struct {
	Code            string
	AccessKeyID     string `json:"AccessKeyId"`
	SecretAccessKey string
	Token           string
	Expiration      time.Time
}

// quick and dirty assertions
func assertEq(expected string, actual string, t *testing.T) {
	t.Helper()
	if actual != expected {
		t.Error("expected: ", expected, " but got: ", actual)
	}
}
func assertIn(candidates []string, actual string, t *testing.T) {
	t.Helper()
	if slices.Contains(candidates, actual) {
		return
	}
	t.Error("value: '", actual, "' not found in: ", candidates)
}

func assertErr(expected string, actual error, t *testing.T) {
	t.Helper()
	if !strings.Contains(actual.Error(), expected) {
		t.Errorf("Expected error to contain %s, got: %s", expected, actual.Error())
	}
}

func TestEnvironmentCredentialService(t *testing.T) {
	cs := &awsEnvironmentCredentialService{}

	// wrong path: some required environment is missing
	_, err := cs.credentials(context.Background())
	assertErr("no AWS_ACCESS_KEY_ID set in environment", err, t)

	t.Setenv("AWS_ACCESS_KEY_ID", "MYAWSACCESSKEYGOESHERE")
	_, err = cs.credentials(context.Background())
	assertErr("no AWS_SECRET_ACCESS_KEY set in environment", err, t)

	t.Setenv("AWS_SECRET_ACCESS_KEY", "MYAWSSECRETACCESSKEYGOESHERE")
	_, err = cs.credentials(context.Background())
	assertErr("no AWS_REGION set in environment", err, t)

	t.Setenv("AWS_REGION", "us-east-1")

	expectedCreds := aws.Credentials{
		AccessKey:    "MYAWSACCESSKEYGOESHERE",
		SecretKey:    "MYAWSSECRETACCESSKEYGOESHERE",
		RegionName:   "us-east-1",
		SessionToken: ""}

	testCases := []struct {
		tokenEnv   string
		tokenValue string
	}{
		// happy path: all required environment is present
		{"", ""},
		// happy path: all required environment is present including security token
		{"AWS_SECURITY_TOKEN", "MYSECURITYTOKENGOESHERE"},
		// happy path: all required environment is present including session token that is preferred over security token
		{"AWS_SESSION_TOKEN", "MYSESSIONTOKENGOESHERE"},
	}

	for _, testCase := range testCases {
		if testCase.tokenEnv != "" {
			t.Setenv(testCase.tokenEnv, testCase.tokenValue)
		}
		expectedCreds.SessionToken = testCase.tokenValue

		envCreds, err := cs.credentials(context.Background())
		if err != nil {
			t.Error("unexpected error: " + err.Error())
		}

		if envCreds != expectedCreds {
			t.Error("expected: ", expectedCreds, " but got: ", envCreds)
		}
	}
}

func TestProfileCredentialService(t *testing.T) {

	defaultKey := "AKIAIOSFODNN7EXAMPLE"
	defaultSecret := "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	defaultSessionToken := "AQoEXAMPLEH4aoAH0gNCAPy"
	defaultRegion := "us-west-2"

	fooKey := "AKIAI44QH8DHBEXAMPLE"
	fooSecret := "je7MtGbClwBF/2Zp9Utk/h3yCo8nvbEXAMPLEKEY"
	fooRegion := "us-east-1"

	config := fmt.Sprintf(`
[default]
aws_access_key_id=%v
aws_secret_access_key=%v
aws_session_token=%v

[foo]
aws_access_key_id=%v
aws_secret_access_key=%v
`, defaultKey, defaultSecret, defaultSessionToken, fooKey, fooSecret)

	files := map[string]string{
		"example.ini": config,
	}

	test.WithTempFS(files, func(path string) {
		cfgPath := filepath.Join(path, "example.ini")
		cs := &awsProfileCredentialService{
			Path:       cfgPath,
			Profile:    "foo",
			RegionName: fooRegion,
		}
		creds, err := cs.credentials(context.Background())
		if err != nil {
			t.Fatal(err)
		}

		expected := aws.Credentials{
			AccessKey:    fooKey,
			SecretKey:    fooSecret,
			RegionName:   fooRegion,
			SessionToken: "",
		}

		if expected != creds {
			t.Fatalf("Expected credentials %v but got %v", expected, creds)
		}

		// "default" profile
		cs = &awsProfileCredentialService{
			Path:       cfgPath,
			Profile:    "",
			RegionName: defaultRegion,
		}

		creds, err = cs.credentials(context.Background())
		if err != nil {
			t.Fatal(err)
		}

		expected = aws.Credentials{
			AccessKey:    defaultKey,
			SecretKey:    defaultSecret,
			RegionName:   defaultRegion,
			SessionToken: defaultSessionToken,
		}

		if expected != creds {
			t.Fatalf("Expected credentials %v but got %v", expected, creds)
		}
	})
}

func TestProfileCredentialServiceWithEnvVars(t *testing.T) {
	defaultKey := "AKIAIOSFODNN7EXAMPLE"
	defaultSecret := "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	defaultSessionToken := "AQoEXAMPLEH4aoAH0gNCAPy"
	defaultRegion := "us-east-1"
	profile := "profileName"
	config := fmt.Sprintf(`
[%s]
aws_access_key_id=%s
aws_secret_access_key=%s
aws_session_token=%s
`, profile, defaultKey, defaultSecret, defaultSessionToken)

	files := map[string]string{
		"example.ini": config,
	}

	test.WithTempFS(files, func(path string) {
		cfgPath := filepath.Join(path, "example.ini")

		t.Setenv(awsCredentialsFileEnvVar, cfgPath)
		t.Setenv(awsProfileEnvVar, profile)
		t.Setenv(awsRegionEnvVar, defaultRegion)

		cs := &awsProfileCredentialService{}
		creds, err := cs.credentials(context.Background())
		if err != nil {
			t.Fatal(err)
		}

		expected := aws.Credentials{
			AccessKey:    defaultKey,
			SecretKey:    defaultSecret,
			RegionName:   defaultRegion,
			SessionToken: defaultSessionToken,
		}

		if expected != creds {
			t.Fatalf("Expected credentials %v but got %v", expected, creds)
		}
	})
}

func TestProfileCredentialServiceWithDefaultPath(t *testing.T) {
	defaultKey := "AKIAIOSFODNN7EXAMPLE"
	defaultSecret := "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	defaultSessionToken := "AQoEXAMPLEH4aoAH0gNCAPy"
	defaultRegion := "us-west-22"

	config := fmt.Sprintf(`
[default]
aws_access_key_id=%s
aws_secret_access_key=%s
aws_session_token=%s
`, defaultKey, defaultSecret, defaultSessionToken)

	files := map[string]string{}

	test.WithTempFS(files, func(path string) {

		t.Setenv("USERPROFILE", path)
		t.Setenv("HOME", path)

		cfgDir := filepath.Join(path, ".aws")
		err := os.MkdirAll(cfgDir, os.ModePerm)
		if err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(cfgDir, "credentials"), []byte(config), 0600); err != nil {
			t.Fatalf("Unexpected error: %s", err)
		}

		cs := &awsProfileCredentialService{RegionName: defaultRegion}
		creds, err := cs.credentials(context.Background())
		if err != nil {
			t.Fatal(err)
		}

		expected := aws.Credentials{
			AccessKey:    defaultKey,
			SecretKey:    defaultSecret,
			RegionName:   defaultRegion,
			SessionToken: defaultSessionToken,
		}

		if expected != creds {
			t.Fatalf("Expected credentials %v but got %v", expected, creds)
		}
	})
}

func TestProfileCredentialServiceWithError(t *testing.T) {
	configNoAccessKeyID := `
[default]
aws_secret_access_key = secret
`

	configNoSecret := `
[default]
aws_access_key_id=accessKey
`
	tests := []struct {
		note   string
		config string
		err    string
	}{
		{
			note:   "no aws_access_key_id",
			config: configNoAccessKeyID,
			err:    "does not contain \"aws_access_key_id\"",
		},
		{
			note:   "no aws_secret_access_key",
			config: configNoSecret,
			err:    "does not contain \"aws_secret_access_key\"",
		},
	}

	for _, tc := range tests {
		t.Run(tc.note, func(t *testing.T) {

			files := map[string]string{
				"example.ini": tc.config,
			}

			test.WithTempFS(files, func(path string) {
				cfgPath := filepath.Join(path, "example.ini")
				cs := &awsProfileCredentialService{
					Path: cfgPath,
				}
				_, err := cs.credentials(context.Background())
				if err == nil {
					t.Fatal("Expected error but got nil")
				}
				if !strings.Contains(err.Error(), tc.err) {
					t.Errorf("expected error to contain %v, got %v", tc.err, err.Error())
				}
			})
		})
	}
}

func TestMetadataCredentialService(t *testing.T) {
	ts := ec2CredTestServer{}
	ts.start()
	defer ts.stop()

	// wrong path: cred service path not well formed
	cs := awsMetadataCredentialService{
		RoleName:        "my_iam_role",
		RegionName:      "us-east-1",
		credServicePath: "this is not a URL", // malformed
		tokenPath:       ts.server.URL + "/latest/api/token",
		logger:          logging.Get(),
	}
	_, err := cs.credentials(context.Background())
	assertErr("unsupported protocol scheme \"\"", err, t)

	// wrong path: no role set but no ECS URI in environment
	os.Unsetenv(ecsRelativePathEnvVar)
	cs = awsMetadataCredentialService{
		RegionName: "us-east-1",
		logger:     logging.Get(),
	}
	_, err = cs.credentials(context.Background())
	assertErr("metadata endpoint cannot be determined from settings and environment", err, t)

	// wrong path: missing token
	t.Setenv(ecsFullPathEnvVar, "fullPath")
	os.Unsetenv(ecsAuthorizationTokenEnvVar)
	_, err = cs.credentials(context.Background())
	assertErr("unable to get ECS metadata authorization token", err, t)
	os.Unsetenv(ecsFullPathEnvVar)

	test.WithTempFS(nil, func(path string) {
		// wrong path: bad file token
		t.Setenv(ecsFullPathEnvVar, "fullPath")
		t.Setenv(ecsAuthorizationTokenFileEnvVar, filepath.Join(path, "bad-file"))
		_, err = cs.credentials(context.Background())
		assertErr("failed to read ECS metadata authorization token from file", err, t)
		os.Unsetenv(ecsFullPathEnvVar)
		os.Unsetenv(ecsAuthorizationTokenFileEnvVar)
	})

	// wrong path: creds not found
	cs = awsMetadataCredentialService{
		RoleName:        "not_my_iam_role", // not present
		RegionName:      "us-east-1",
		credServicePath: ts.server.URL + "/latest/meta-data/iam/security-credentials/",
		tokenPath:       ts.server.URL + "/latest/api/token",
		logger:          logging.Get(),
	}
	_, err = cs.credentials(context.Background())
	assertErr("metadata HTTP request returned unexpected status: 404 Not Found", err, t)

	// wrong path: malformed JSON body
	cs = awsMetadataCredentialService{
		RoleName:        "my_bad_iam_role", // not good
		RegionName:      "us-east-1",
		credServicePath: ts.server.URL + "/latest/meta-data/iam/security-credentials/",
		tokenPath:       ts.server.URL + "/latest/api/token",
		logger:          logging.Get(),
	}
	_, err = cs.credentials(context.Background())
	assertErr("failed to parse credential response from metadata service: invalid character 'T' looking for beginning of value", err, t)

	// wrong path: token service error
	cs = awsMetadataCredentialService{
		RoleName:        "my_iam_role",
		RegionName:      "us-east-1",
		credServicePath: ts.server.URL + "/latest/meta-data/iam/security-credentials/",
		tokenPath:       ts.server.URL + "/latest/api/missing_token",
		logger:          logging.Get(),
	} // will 404
	_, err = cs.credentials(context.Background())
	assertErr("metadata token HTTP request returned unexpected status: 404 Not Found", err, t)

	// wrong path: token service returns bad token
	cs = awsMetadataCredentialService{
		RoleName:        "my_iam_role",
		RegionName:      "us-east-1",
		credServicePath: ts.server.URL + "/latest/meta-data/iam/security-credentials/",
		tokenPath:       ts.server.URL + "/latest/api/bad_token",
		logger:          logging.Get(),
	} // not good
	_, err = cs.credentials(context.Background())
	assertErr("metadata HTTP request returned unexpected status: 401 Unauthorized", err, t)

	// wrong path: bad result code from EC2 metadata service
	ts.payload = metadataPayload{
		AccessKeyID:     "MYAWSACCESSKEYGOESHERE",
		SecretAccessKey: "MYAWSSECRETACCESSKEYGOESHERE",
		Code:            "Failure", // this is bad
		Token:           "MYAWSSECURITYTOKENGOESHERE",
		Expiration:      time.Now().UTC().Add(time.Minute * 30)}
	cs = awsMetadataCredentialService{
		RoleName:        "my_iam_role",
		RegionName:      "us-east-1",
		credServicePath: ts.server.URL + "/latest/meta-data/iam/security-credentials/",
		tokenPath:       ts.server.URL + "/latest/api/token",
		logger:          logging.Get(),
	}
	_, err = cs.credentials(context.Background())
	assertErr("metadata service query did not succeed: Failure", err, t)

	// happy path: base case
	ts.payload = metadataPayload{
		AccessKeyID:     "MYAWSACCESSKEYGOESHERE",
		SecretAccessKey: "MYAWSSECRETACCESSKEYGOESHERE",
		Code:            "Success",
		Token:           "MYAWSSECURITYTOKENGOESHERE",
		Expiration:      time.Now().UTC().Add(time.Minute * 300)}
	cs = awsMetadataCredentialService{
		RoleName:        "my_iam_role",
		RegionName:      "us-east-1",
		credServicePath: ts.server.URL + "/latest/meta-data/iam/security-credentials/",
		tokenPath:       ts.server.URL + "/latest/api/token",
		logger:          logging.Get(),
	}
	var creds aws.Credentials
	creds, err = cs.credentials(context.Background())
	if err != nil {
		// Cannot proceed with test if unable to fetch credentials.
		t.Fatal(err)
	}

	assertEq(creds.AccessKey, ts.payload.AccessKeyID, t)
	assertEq(creds.SecretKey, ts.payload.SecretAccessKey, t)
	assertEq(creds.RegionName, cs.RegionName, t)
	assertEq(creds.SessionToken, ts.payload.Token, t)

	// happy path: verify credentials are cached based on expiry
	ts.payload.AccessKeyID = "ICHANGEDTHISBUTWEWONTSEEIT"
	creds, err = cs.credentials(context.Background())
	if err != nil {
		// Cannot proceed with test if unable to fetch credentials.
		t.Fatal(err)
	}

	assertEq(creds.AccessKey, "MYAWSACCESSKEYGOESHERE", t) // the original value
	assertEq(creds.SecretKey, ts.payload.SecretAccessKey, t)
	assertEq(creds.RegionName, cs.RegionName, t)
	assertEq(creds.SessionToken, ts.payload.Token, t)

	// happy path: with refresh
	// first time through
	cs = awsMetadataCredentialService{
		RoleName:        "my_iam_role",
		RegionName:      "us-east-1",
		credServicePath: ts.server.URL + "/latest/meta-data/iam/security-credentials/",
		tokenPath:       ts.server.URL + "/latest/api/token",
		logger:          logging.Get(),
	}
	ts.payload = metadataPayload{
		AccessKeyID:     "MYAWSACCESSKEYGOESHERE",
		SecretAccessKey: "MYAWSSECRETACCESSKEYGOESHERE",
		Code:            "Success",
		Token:           "MYAWSSECURITYTOKENGOESHERE",
		Expiration:      time.Now().UTC().Add(time.Minute * 2)} // short time

	creds, err = cs.credentials(context.Background())
	if err != nil {
		// Cannot proceed with test if unable to fetch credentials.
		t.Fatal(err)
	}

	assertEq(creds.AccessKey, ts.payload.AccessKeyID, t)
	assertEq(creds.SecretKey, ts.payload.SecretAccessKey, t)
	assertEq(creds.RegionName, cs.RegionName, t)
	assertEq(creds.SessionToken, ts.payload.Token, t)

	// second time through, with changes
	ts.payload.AccessKeyID = "ICHANGEDTHISANDWEWILLSEEIT"
	creds, err = cs.credentials(context.Background())
	if err != nil {
		// Cannot proceed with test if unable to fetch credentials.
		t.Fatal(err)
	}

	assertEq(creds.AccessKey, ts.payload.AccessKeyID, t) // the new value
	assertEq(creds.SecretKey, ts.payload.SecretAccessKey, t)
	assertEq(creds.RegionName, cs.RegionName, t)
	assertEq(creds.SessionToken, ts.payload.Token, t)

	// happy path: credentials fetched from full path var
	cs = awsMetadataCredentialService{
		RegionName:      "us-east-1",
		credServicePath: "", // not set as we want to test env var resolution
		logger:          logging.Get(),
	}
	ts.payload = metadataPayload{
		AccessKeyID:     "MYAWSACCESSKEYGOESHERE",
		SecretAccessKey: "MYAWSSECRETACCESSKEYGOESHERE",
		Code:            "Success",
		Token:           "MYAWSSECURITYTOKENGOESHERE",
		Expiration:      time.Now().UTC().Add(time.Minute * 2)} // short time
	t.Setenv(ecsFullPathEnvVar, ts.server.URL+"/fullPath")
	t.Setenv(ecsAuthorizationTokenEnvVar, "THIS_IS_A_GOOD_TOKEN")

	creds, err = cs.credentials(context.Background())
	if err != nil {
		// Cannot proceed with test if unable to fetch credentials.
		t.Fatal(err)
	}

	assertEq(creds.AccessKey, ts.payload.AccessKeyID, t)
	assertEq(creds.SecretKey, ts.payload.SecretAccessKey, t)
	assertEq(creds.RegionName, cs.RegionName, t)
	assertEq(creds.SessionToken, ts.payload.Token, t)
	os.Unsetenv(ecsFullPathEnvVar)
	os.Unsetenv(ecsAuthorizationTokenEnvVar)

	// happy path: credentials fetched from full path var using token from filesystem
	files := map[string]string{
		"good_token_file": "THIS_IS_A_GOOD_TOKEN",
	}
	test.WithTempFS(files, func(path string) {
		// happy path: credentials fetched from full path var
		cs = awsMetadataCredentialService{
			RegionName:      "us-east-1",
			credServicePath: "", // not set as we want to test env var resolution
			logger:          logging.Get(),
		}
		ts.payload = metadataPayload{
			AccessKeyID:     "MYAWSACCESSKEYGOESHERE",
			SecretAccessKey: "MYAWSSECRETACCESSKEYGOESHERE",
			Code:            "Success",
			Token:           "MYAWSSECURITYTOKENGOESHERE",
			Expiration:      time.Now().UTC().Add(time.Minute * 2)} // short time
		t.Setenv(ecsFullPathEnvVar, ts.server.URL+"/fullPath")
		t.Setenv(ecsAuthorizationTokenFileEnvVar, filepath.Join(path, "good_token_file"))
		creds, err = cs.credentials(context.Background())
		if err != nil {
			// Cannot proceed with test if unable to fetch credentials.
			t.Fatal(err)
		}

		assertEq(creds.AccessKey, ts.payload.AccessKeyID, t)
		assertEq(creds.SecretKey, ts.payload.SecretAccessKey, t)
		assertEq(creds.RegionName, cs.RegionName, t)
		assertEq(creds.SessionToken, ts.payload.Token, t)
		os.Unsetenv(ecsFullPathEnvVar)
		os.Unsetenv(ecsAuthorizationTokenFileEnvVar)
	})
}

func TestMetadataServiceErrorHandled(t *testing.T) {
	ts := ec2CredTestServer{}
	ts.start()
	defer ts.stop()

	// wrong path: handle errors from credential service
	cs := &awsMetadataCredentialService{
		RoleName:        "not_my_iam_role", // not present
		RegionName:      "us-east-1",
		credServicePath: ts.server.URL + "/latest/meta-data/iam/security-credentials/",
		tokenPath:       ts.server.URL + "/latest/api/token",
		logger:          logging.Get(),
	}

	_, err := cs.credentials(context.Background())
	assertErr("metadata HTTP request returned unexpected status: 404 Not Found", err, t)
}

func TestV4Signing(t *testing.T) {
	ts := ec2CredTestServer{}
	ts.start()
	defer ts.stop()

	// happy path: sign correctly
	cs := &awsMetadataCredentialService{
		RoleName:        "my_iam_role", // not present
		RegionName:      "us-east-1",
		credServicePath: ts.server.URL + "/latest/meta-data/iam/security-credentials/",
		tokenPath:       ts.server.URL + "/latest/api/token",
		logger:          logging.Get(),
	}
	ts.payload = metadataPayload{
		AccessKeyID:     "MYAWSACCESSKEYGOESHERE",
		SecretAccessKey: "MYAWSSECRETACCESSKEYGOESHERE",
		Code:            "Success",
		Token:           "MYAWSSECURITYTOKENGOESHERE",
		Expiration:      time.Now().UTC().Add(time.Minute * 2)}
	req, _ := http.NewRequest("GET", "https://mybucket.s3.amazonaws.com/bundle.tar.gz", strings.NewReader(""))

	// force a non-random source so that we can predict the v4a signing key and, thus, signature
	aws.SetRandomSource(test.NewZeroReader())
	defer func() { aws.SetRandomSource(rand.Reader) }()

	tests := []struct {
		sigVersion            string
		expectedAuthorization []string
	}{
		{
			sigVersion: "4",
			expectedAuthorization: []string{
				"AWS4-HMAC-SHA256 Credential=MYAWSACCESSKEYGOESHERE/20190424/us-east-1/s3/aws4_request," +
					"SignedHeaders=host;x-amz-content-sha256;x-amz-date;x-amz-security-token," +
					"Signature=d3f0561abae5e35d9ee2c15e678bb7acacc4b4743707a8f7fbcbfdb519078990",
			},
		},
		{
			sigVersion: "4a",
			expectedAuthorization: []string{
				// this signature is for go 1.24+
				"AWS4-ECDSA-P256-SHA256 Credential=MYAWSACCESSKEYGOESHERE/20190424/s3/aws4_request, " +
					"SignedHeaders=host;x-amz-content-sha256;x-amz-date;x-amz-region-set;x-amz-security-token, " +
					"Signature=3045022061d172d81cb118e1b1fbc321aa83b622eadbe8cc602d27d7e107cbf9caf42c09022100b3b0d9c52a382eea199b260f7aa69c658d63f78bd459da46ed94f30f66553389",
				// this signature is for go 1.20+, which changed crypto/ecdsa so signatures differ from go 1.18
				"AWS4-ECDSA-P256-SHA256 Credential=MYAWSACCESSKEYGOESHERE/20190424/s3/aws4_request, " +
					"SignedHeaders=host;x-amz-content-sha256;x-amz-date;x-amz-region-set;x-amz-security-token, " +
					"Signature=3046022100b5b0a90b1739a67315b53b5ac93164e2a511723f76e29bf5396b7e55cb5db75a0221008702a757055fe397997d279fabfd73d162e4cae38111e806e87f4500076f3de0",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.sigVersion, func(t *testing.T) {
			creds, err := cs.credentials(context.Background())
			if err != nil {
				t.Fatal("unexpected error getting credentials")
			}

			if err := aws.SignRequest(req, "s3", creds, time.Unix(1556129697, 0), test.sigVersion); err != nil {
				t.Fatal("unexpected error during signing", err)
			}

			// expect mandatory headers
			assertEq("mybucket.s3.amazonaws.com", req.Header.Get("Host"), t)
			assertIn(test.expectedAuthorization, req.Header.Get("Authorization"), t)
			assertEq("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
				req.Header.Get("X-Amz-Content-Sha256"), t)
			assertEq("20190424T181457Z", req.Header.Get("X-Amz-Date"), t)
			assertEq("MYAWSSECURITYTOKENGOESHERE", req.Header.Get("X-Amz-Security-Token"), t)
		})
	}
}

func TestV4SigningUnsignedPayload(t *testing.T) {
	ts := ec2CredTestServer{}
	ts.start()
	defer ts.stop()

	// happy path: sign correctly
	cs := &awsMetadataCredentialService{
		RoleName:        "my_iam_role", // not present
		RegionName:      "us-east-1",
		credServicePath: ts.server.URL + "/latest/meta-data/iam/security-credentials/",
		tokenPath:       ts.server.URL + "/latest/api/token",
		logger:          logging.Get(),
	}
	ts.payload = metadataPayload{
		AccessKeyID:     "MYAWSACCESSKEYGOESHERE",
		SecretAccessKey: "MYAWSSECRETACCESSKEYGOESHERE",
		Code:            "Success",
		Token:           "MYAWSSECURITYTOKENGOESHERE",
		Expiration:      time.Now().UTC().Add(time.Minute * 2)}

	// force a non-random source so that we can predict the v4a signing key and, thus, signature
	aws.SetRandomSource(test.NewZeroReader())
	defer func() { aws.SetRandomSource(rand.Reader) }()

	tests := []struct {
		disablePayloadSigning bool
		expectedAuthorization []string
		expectedShaHeaderVal  string
	}{
		{
			disablePayloadSigning: true,
			expectedAuthorization: []string{
				"AWS4-HMAC-SHA256 Credential=MYAWSACCESSKEYGOESHERE/20190424/us-east-1/s3/aws4_request," +
					"SignedHeaders=host;x-amz-content-sha256;x-amz-date;x-amz-security-token," +
					"Signature=3682e55f6d86d3372003b3d28c74aa960f076d91fce833b129ae76415a12e5e4",
			},
			expectedShaHeaderVal: "UNSIGNED-PAYLOAD",
		},
		{
			disablePayloadSigning: false,
			expectedAuthorization: []string{
				"AWS4-HMAC-SHA256 Credential=MYAWSACCESSKEYGOESHERE/20190424/us-east-1/s3/aws4_request," +
					"SignedHeaders=host;x-amz-content-sha256;x-amz-date;x-amz-security-token," +
					"Signature=d3f0561abae5e35d9ee2c15e678bb7acacc4b4743707a8f7fbcbfdb519078990",
			},
			expectedShaHeaderVal: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
	}
	for _, test := range tests {
		creds, err := cs.credentials(context.Background())
		if err != nil {
			t.Fatal("unexpected error getting credentials")
		}
		req, _ := http.NewRequest("GET", "https://mybucket.s3.amazonaws.com/bundle.tar.gz", strings.NewReader(""))
		var body []byte
		if req.Body == nil {
			body = []byte("")
		} else {
			body, _ = io.ReadAll(req.Body)
			req.Body = io.NopCloser(bytes.NewReader(body))
		}

		authHeader, awsHeaders := aws.SignV4(req.Header, req.Method, req.URL, body, "s3", creds, time.Unix(1556129697, 0).UTC(), test.disablePayloadSigning)
		req.Header.Set("Authorization", authHeader)
		for k, v := range awsHeaders {
			req.Header.Add(k, v)
		}

		// expect mandatory headers
		assertEq("mybucket.s3.amazonaws.com", req.Header.Get("Host"), t)
		assertIn(test.expectedAuthorization, req.Header.Get("Authorization"), t)
		assertEq(test.expectedShaHeaderVal, req.Header.Get("X-Amz-Content-Sha256"), t)
		assertEq("20190424T181457Z", req.Header.Get("X-Amz-Date"), t)
		assertEq("MYAWSSECURITYTOKENGOESHERE", req.Header.Get("X-Amz-Security-Token"), t)
	}
}

func TestV4SigningForApiGateway(t *testing.T) {
	ts := ec2CredTestServer{}
	ts.start()
	defer ts.stop()

	cs := &awsMetadataCredentialService{
		RoleName:        "my_iam_role", // not present
		RegionName:      "us-east-1",
		credServicePath: ts.server.URL + "/latest/meta-data/iam/security-credentials/",
		tokenPath:       ts.server.URL + "/latest/api/token",
		logger:          logging.Get(),
	}
	ts.payload = metadataPayload{
		AccessKeyID:     "MYAWSACCESSKEYGOESHERE",
		SecretAccessKey: "MYAWSSECRETACCESSKEYGOESHERE",
		Code:            "Success",
		Token:           "MYAWSSECURITYTOKENGOESHERE",
		Expiration:      time.Now().UTC().Add(time.Minute * 2)}
	req, _ := http.NewRequest("POST", "https://myrestapi.execute-api.us-east-1.amazonaws.com/prod/logs",
		strings.NewReader("{ \"payload\": 42 }"))
	req.Header.Set("Content-Type", "application/json")

	creds, err := cs.credentials(context.Background())
	if err != nil {
		t.Fatal("unexpected error getting credentials")
	}

	if err := aws.SignRequest(req, "execute-api", creds, time.Unix(1556129697, 0), "4"); err != nil {
		t.Fatal("unexpected error during signing")
	}

	// expect mandatory headers
	assertEq(req.Header.Get("Host"), "myrestapi.execute-api.us-east-1.amazonaws.com", t)
	assertEq(req.Header.Get("Authorization"),
		"AWS4-HMAC-SHA256 Credential=MYAWSACCESSKEYGOESHERE/20190424/us-east-1/execute-api/aws4_request,"+
			"SignedHeaders=content-type;host;x-amz-date;x-amz-security-token,"+
			"Signature=c8ee72cc45050b255bcbf19defc693f7cd788959b5380fa0985de6e865635339", t)
	// no content sha should be set, since this is specific to s3 and glacier
	assertEq(req.Header.Get("X-Amz-Content-Sha256"), "", t)
	assertEq(req.Header.Get("X-Amz-Date"), "20190424T181457Z", t)
	assertEq(req.Header.Get("X-Amz-Security-Token"), "MYAWSSECURITYTOKENGOESHERE", t)
}

func TestV4SigningOmitsIgnoredHeaders(t *testing.T) {
	ts := ec2CredTestServer{}
	ts.start()
	defer ts.stop()

	cs := &awsMetadataCredentialService{
		RoleName:        "my_iam_role", // not present
		RegionName:      "us-east-1",
		credServicePath: ts.server.URL + "/latest/meta-data/iam/security-credentials/",
		tokenPath:       ts.server.URL + "/latest/api/token",
		logger:          logging.Get(),
	}
	ts.payload = metadataPayload{
		AccessKeyID:     "MYAWSACCESSKEYGOESHERE",
		SecretAccessKey: "MYAWSSECRETACCESSKEYGOESHERE",
		Code:            "Success",
		Token:           "MYAWSSECURITYTOKENGOESHERE",
		Expiration:      time.Now().UTC().Add(time.Minute * 2)}
	req, _ := http.NewRequest("POST", "https://myrestapi.execute-api.us-east-1.amazonaws.com/prod/logs",
		strings.NewReader("{ \"payload\": 42 }"))
	req.Header.Set("Content-Type", "application/json")

	// These are headers that should never be included in the signed headers
	req.Header.Set("User-Agent", "Unit Tests!")
	req.Header.Set("Authorization", "Auth header will be overwritten, and shouldn't be signed")
	req.Header.Set("X-Amzn-Trace-Id", "Some trace id")

	// force a non-random source so that we can predict the v4a signing key and, thus, signature
	aws.SetRandomSource(test.NewZeroReader())
	defer func() { aws.SetRandomSource(rand.Reader) }()

	tests := []struct {
		sigVersion            string
		expectedAuthorization []string
	}{
		{
			sigVersion: "4",
			expectedAuthorization: []string{"AWS4-HMAC-SHA256 Credential=MYAWSACCESSKEYGOESHERE/20190424/us-east-1/execute-api/aws4_request," +
				"SignedHeaders=content-type;host;x-amz-date;x-amz-security-token," +
				"Signature=c8ee72cc45050b255bcbf19defc693f7cd788959b5380fa0985de6e865635339",
			},
		},
		{
			sigVersion: "4a",
			expectedAuthorization: []string{
				// this signature is for go 1.20+, which changed crypto/ecdsa so signatures differ from go 1.18
				"AWS4-ECDSA-P256-SHA256 Credential=MYAWSACCESSKEYGOESHERE/20190424/execute-api/aws4_request, " +
					"SignedHeaders=content-length;content-type;host;x-amz-content-sha256;x-amz-date;x-amz-region-set;x-amz-security-token, " +
					"Signature=30440220030e9ef5a174354265b33cb57e43ed15cf418d90954d1c6061d99bca709ff0bd02204f11c90715131161bc65040dd11bc761471ccd230888750a12dbeaaf40541c20",
				// this signature is for go 1.24+
				"AWS4-ECDSA-P256-SHA256 Credential=MYAWSACCESSKEYGOESHERE/20190424/execute-api/aws4_request, " +
					"SignedHeaders=content-length;content-type;host;x-amz-content-sha256;x-amz-date;x-amz-region-set;x-amz-security-token, " +
					"Signature=30450220652e9b5e04bb50cc27a599a05e4755719c25a6a93c4da71784ea681e951c5e9b022100c9e090654a5077946478fcd35b4b60d34899961b604ab57e6dd13ebcb49fc672",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.sigVersion, func(t *testing.T) {
			creds, err := cs.credentials(context.Background())
			if err != nil {
				t.Fatal("unexpected error getting credentials")
			}

			if err := aws.SignRequest(req, "execute-api", creds, time.Unix(1556129697, 0), test.sigVersion); err != nil {
				t.Fatal("unexpected error during signing")
			}

			// Check the signed headers doesn't include user-agent, authorization or x-amz-trace-id
			assertIn(test.expectedAuthorization, req.Header.Get("Authorization"), t)
			// The headers omitted from signing should still be present in the request
			assertEq(req.Header.Get("User-Agent"), "Unit Tests!", t)
			assertEq(req.Header.Get("X-Amzn-Trace-Id"), "Some trace id", t)
		})
	}

}

func TestV4SigningCustomPort(t *testing.T) {
	ts := ec2CredTestServer{}
	ts.start()
	defer ts.stop()

	cs := &awsMetadataCredentialService{
		RoleName:        "my_iam_role", // not present
		RegionName:      "us-east-1",
		credServicePath: ts.server.URL + "/latest/meta-data/iam/security-credentials/",
		tokenPath:       ts.server.URL + "/latest/api/token",
		logger:          logging.Get(),
	}
	ts.payload = metadataPayload{
		AccessKeyID:     "MYAWSACCESSKEYGOESHERE",
		SecretAccessKey: "MYAWSSECRETACCESSKEYGOESHERE",
		Code:            "Success",
		Token:           "MYAWSSECURITYTOKENGOESHERE",
		Expiration:      time.Now().UTC().Add(time.Minute * 2)}
	req, _ := http.NewRequest("GET", "https://custom.s3.server:9000/bundle.tar.gz", strings.NewReader(""))

	creds, err := cs.credentials(context.Background())
	if err != nil {
		t.Fatal("unexpected error getting credentials")
	}

	if err := aws.SignRequest(req, "s3", creds, time.Unix(1556129697, 0), "4"); err != nil {
		t.Fatal("unexpected error during signing")
	}

	// expect mandatory headers
	assertEq(req.Header.Get("Host"), "custom.s3.server:9000", t)
	assertEq(req.Header.Get("Authorization"),
		"AWS4-HMAC-SHA256 Credential=MYAWSACCESSKEYGOESHERE/20190424/us-east-1/s3/aws4_request,"+
			"SignedHeaders=host;x-amz-content-sha256;x-amz-date;x-amz-security-token,"+
			"Signature=765b67c6b136f99d9b769171c9939fc444021f7d17e4fbe6e1ab8b1926713c2b", t)
	assertEq(req.Header.Get("X-Amz-Content-Sha256"),
		"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", t)
	assertEq(req.Header.Get("X-Amz-Date"), "20190424T181457Z", t)
	assertEq(req.Header.Get("X-Amz-Security-Token"), "MYAWSSECURITYTOKENGOESHERE", t)
}

func TestV4SigningDoesNotMutateBody(t *testing.T) {
	ts := ec2CredTestServer{}
	ts.start()
	defer ts.stop()

	cs := &awsMetadataCredentialService{
		RoleName:        "my_iam_role", // not present
		RegionName:      "us-east-1",
		credServicePath: ts.server.URL + "/latest/meta-data/iam/security-credentials/",
		tokenPath:       ts.server.URL + "/latest/api/token",
		logger:          logging.Get(),
	}
	ts.payload = metadataPayload{
		AccessKeyID:     "MYAWSACCESSKEYGOESHERE",
		SecretAccessKey: "MYAWSSECRETACCESSKEYGOESHERE",
		Code:            "Success",
		Token:           "MYAWSSECURITYTOKENGOESHERE",
		Expiration:      time.Now().UTC().Add(time.Minute * 2)}

	// force a non-random source so that we can predict the v4a signing key and, thus, signature
	aws.SetRandomSource(test.NewZeroReader())
	defer func() { aws.SetRandomSource(rand.Reader) }()

	tests := []struct {
		sigVersion string
	}{
		{sigVersion: "4"},
		{sigVersion: "4a"},
	}

	for _, test := range tests {
		req, _ := http.NewRequest("POST", "https://myrestapi.execute-api.us-east-1.amazonaws.com/prod/logs",
			strings.NewReader("{ \"payload\": 42 }"))

		creds, err := cs.credentials(context.Background())
		if err != nil {
			t.Fatal("unexpected error getting credentials")
		}

		if err := aws.SignRequest(req, "execute-api", creds, time.Unix(1556129697, 0), test.sigVersion); err != nil {
			t.Fatal("unexpected error during signing")
		}

		// Read the body and check that it was not mutated
		body, _ := io.ReadAll(req.Body)
		assertEq(string(body), "{ \"payload\": 42 }", t)
	}
}

func TestV4SigningWithMultiValueHeaders(t *testing.T) {
	ts := ec2CredTestServer{}
	ts.start()
	defer ts.stop()

	cs := &awsMetadataCredentialService{
		RoleName:        "my_iam_role", // not present
		RegionName:      "us-east-1",
		credServicePath: ts.server.URL + "/latest/meta-data/iam/security-credentials/",
		tokenPath:       ts.server.URL + "/latest/api/token",
		logger:          logging.Get(),
	}
	ts.payload = metadataPayload{
		AccessKeyID:     "MYAWSACCESSKEYGOESHERE",
		SecretAccessKey: "MYAWSSECRETACCESSKEYGOESHERE",
		Code:            "Success",
		Token:           "MYAWSSECURITYTOKENGOESHERE",
		Expiration:      time.Now().UTC().Add(time.Minute * 2)}
	req, _ := http.NewRequest("POST", "https://myrestapi.execute-api.us-east-1.amazonaws.com/prod/logs",
		strings.NewReader("{ \"payload\": 42 }"))
	req.Header.Add("Accept", "text/plain")
	req.Header.Add("Accept", "text/html")

	// force a non-random source so that we can predict the v4a signing key and, thus, signature
	// The mock rand reader returns an endless stream of zeros
	aws.SetRandomSource(test.NewZeroReader())
	defer func() { aws.SetRandomSource(rand.Reader) }()

	tests := []struct {
		sigVersion            string
		expectedAuthorization []string
	}{
		{
			sigVersion: "4",
			expectedAuthorization: []string{
				"AWS4-HMAC-SHA256 Credential=MYAWSACCESSKEYGOESHERE/20190424/us-east-1/execute-api/aws4_request," +
					"SignedHeaders=accept;host;x-amz-date;x-amz-security-token," +
					"Signature=0237b0c789cad36212f0efba70c02549e1f659ab9caaca16423930cc7236c046",
			},
		},
		{
			sigVersion: "4a",
			expectedAuthorization: []string{
				// this signature is for go 1.20+, which changed crypto/ecdsa so signatures differ from go 1.18
				"AWS4-ECDSA-P256-SHA256 Credential=MYAWSACCESSKEYGOESHERE/20190424/execute-api/aws4_request, " +
					"SignedHeaders=accept;content-length;host;x-amz-content-sha256;x-amz-date;x-amz-region-set;x-amz-security-token, " +
					"Signature=3045022047fc8a4a842fdaf8ca538580d8a7ef13da72b06f3b2953b2cb105c6ad86a9a41022100bb54877f27a58cf73a812af45bf82c94dd9f25d41a65645cb35f012cd1591589",
				// this signature is for go 1.24+
				"AWS4-ECDSA-P256-SHA256 Credential=MYAWSACCESSKEYGOESHERE/20190424/execute-api/aws4_request, " +
					"SignedHeaders=accept;content-length;host;x-amz-content-sha256;x-amz-date;x-amz-region-set;x-amz-security-token, " +
					"Signature=304502204f38b116e49b743307141797f04c8610ed035c54d06acbeb4f33ab48c8ec578e022100bd5995cb1eaecb5aa8c3062dcfcae7af62b6f32cc578cd42268165e259be46a0",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.sigVersion, func(t *testing.T) {
			creds, err := cs.credentials(context.Background())
			if err != nil {
				t.Fatal("unexpected error getting credentials")
			}

			if err := aws.SignRequest(req, "execute-api", creds, time.Unix(1556129697, 0), test.sigVersion); err != nil {
				t.Fatal("unexpected error during signing")
			}

			if len(req.Header.Values("Authorization")) != 1 {
				t.Fatal("Authorization header is multi-valued. This will break AWS v4 signing.")
			}
			// Check the signed headers includes our multi-value 'accept' header
			assertIn(test.expectedAuthorization, req.Header.Get("Authorization"), t)
			// The multi-value headers are preserved
			assertEq("text/plain", req.Header.Values("Accept")[0], t)
			assertEq("text/html", req.Header.Values("Accept")[1], t)
		})
	}
}

// simulate EC2 metadata service
type ec2CredTestServer struct {
	server  *httptest.Server
	payload metadataPayload // must set before use
}

func (t *ec2CredTestServer) handle(w http.ResponseWriter, r *http.Request) {
	goodPath := "/latest/meta-data/iam/security-credentials/my_iam_role"
	badPath := "/latest/meta-data/iam/security-credentials/my_bad_iam_role"
	goodPathFull := "/fullPath"

	goodTokenPath := "/latest/api/token"
	badTokenPath := "/latest/api/bad_token"

	tokenValue := "THIS_IS_A_GOOD_TOKEN"
	jsonBytes, _ := json.Marshal(t.payload)

	switch r.URL.Path {
	case goodTokenPath:
		// a valid token
		w.WriteHeader(200)
		_, _ = w.Write([]byte(tokenValue))
	case badTokenPath:
		// an invalid token
		w.WriteHeader(200)
		_, _ = w.Write([]byte("THIS_IS_A_BAD_TOKEN"))
	case goodPath:
		// validate token...
		if r.Header.Get("X-aws-ec2-metadata-token") == tokenValue {
			// a metadata response that's well-formed
			w.WriteHeader(200)
			_, _ = w.Write(jsonBytes)
		} else {
			// an unauthorized response
			w.WriteHeader(401)
		}
	case badPath:
		// a metadata response that's not well-formed
		w.WriteHeader(200)
		_, _ = w.Write([]byte("This isn't a JSON payload"))
	case goodPathFull:
		// validate token...
		if r.Header.Get("Authorization") == tokenValue {
			w.WriteHeader(200)
			_, _ = w.Write(jsonBytes)
		} else {
			// AWS returns a 404 if the token is wrong
			w.WriteHeader(404)
		}
	default:
		// something else that we won't be able to find
		w.WriteHeader(404)
	}
}

func (t *ec2CredTestServer) start() {
	t.server = httptest.NewServer(http.HandlerFunc(t.handle))
}

func (t *ec2CredTestServer) stop() {
	t.server.Close()
}

func TestWebIdentityCredentialService(t *testing.T) {
	t.Setenv("AWS_REGION", "us-west-1")

	testAccessKey := "ASgeIAIOSFODNN7EXAMPLE"
	ts := stsTestServer{
		t:                         t,
		accessKey:                 testAccessKey,
		assumeRoleWithWebIdentity: true,
	}
	ts.start()
	defer ts.stop()
	cs := awsWebIdentityCredentialService{
		stsURL: ts.server.URL,
		logger: logging.Get(),
	}

	files := map[string]string{
		"good_token_file": "good-token",
		"bad_token_file":  "bad-token",
	}

	test.WithTempFS(files, func(path string) {
		goodTokenFile := filepath.Join(path, "good_token_file")
		badTokenFile := filepath.Join(path, "bad_token_file")

		// wrong path: no AWS_ROLE_ARN set
		err := cs.populateFromEnv()
		assertErr("no AWS_ROLE_ARN set in environment", err, t)
		t.Setenv("AWS_ROLE_ARN", "role:arn")

		// wrong path: no AWS_WEB_IDENTITY_TOKEN_FILE set
		err = cs.populateFromEnv()
		assertErr("no AWS_WEB_IDENTITY_TOKEN_FILE set in environment", err, t)
		t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "/nonsense")

		// happy path: both env vars set
		err = cs.populateFromEnv()
		if err != nil {
			t.Fatalf("Error while getting env vars: %s", err)
		}

		// wrong path: refresh with invalid web token file
		err = cs.refreshFromService(context.Background())
		assertErr("unable to read web token for sts HTTP request: open /nonsense: no such file or directory", err, t)

		// wrong path: refresh with "bad token"
		t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", badTokenFile)
		_ = cs.populateFromEnv()
		err = cs.refreshFromService(context.Background())
		assertErr("STS HTTP request returned unexpected status: 401 Unauthorized", err, t)

		// happy path: refresh with "good token"
		t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", goodTokenFile)
		_ = cs.populateFromEnv()
		err = cs.refreshFromService(context.Background())
		if err != nil {
			t.Fatalf("Unexpected err: %s", err)
		}

		// happy path: refresh and get credentials
		creds, _ := cs.credentials(context.Background())
		assertEq(creds.AccessKey, testAccessKey, t)

		// happy path: refresh with session and get credentials
		cs.expiration = time.Now()
		cs.SessionName = "TEST_SESSION"
		creds, _ = cs.credentials(context.Background())
		assertEq(creds.AccessKey, testAccessKey, t)

		// happy path: don't refresh, but get credentials
		ts.accessKey = "OTHERKEY"
		creds, _ = cs.credentials(context.Background())
		assertEq(creds.AccessKey, testAccessKey, t)

		// happy/wrong path: refresh with "bad token" but return previous credentials
		t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", badTokenFile)
		_ = cs.populateFromEnv()
		cs.expiration = time.Now()
		creds, err = cs.credentials(context.Background())
		assertEq(creds.AccessKey, testAccessKey, t)
		assertErr("STS HTTP request returned unexpected status: 401 Unauthorized", err, t)

		// wrong path: refresh with "bad token" but return previous credentials
		t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", goodTokenFile)
		t.Setenv("AWS_ROLE_ARN", "BrokenRole")
		_ = cs.populateFromEnv()
		cs.expiration = time.Now()
		creds, err = cs.credentials(context.Background())
		assertErr("failed to parse credential response from STS service: EOF", err, t)
	})
}

func TestAssumeRoleCredentialServiceUsingWrongSigningProvider(t *testing.T) {
	t.Setenv("AWS_REGION", "us-west-1")

	testAccessKey := "ASgeIAIOSFODNN7EXAMPLE"
	ts := stsTestServer{
		t:                         t,
		accessKey:                 testAccessKey,
		assumeRoleWithWebIdentity: false,
	}
	ts.start()
	defer ts.stop()
	cs := awsAssumeRoleCredentialService{
		stsURL: ts.server.URL,
		logger: logging.Get(),
	}

	// wrong path: no AWS signing plugin
	err := cs.populateFromEnv()
	assertErr("a AWS signing plugin must be specified when AssumeRole credential provider is enabled", err, t)

	// wrong path: unsupported AWS signing plugin
	cs.AWSSigningPlugin = &awsSigningAuthPlugin{AWSWebIdentityCredentials: &awsWebIdentityCredentialService{}}
	err = cs.populateFromEnv()
	assertErr("unsupported AWS signing plugin with AssumeRole credential provider", err, t)
}

func TestAssumeRoleCredentialServiceUsingEnvCredentialsProvider(t *testing.T) {
	t.Setenv("AWS_REGION", "us-west-1")

	testAccessKey := "ASgeIAIOSFODNN7EXAMPLE"
	ts := stsTestServer{
		t:                         t,
		accessKey:                 testAccessKey,
		assumeRoleWithWebIdentity: false,
	}
	ts.start()
	defer ts.stop()
	cs := awsAssumeRoleCredentialService{
		stsURL:           ts.server.URL,
		logger:           logging.Get(),
		AWSSigningPlugin: &awsSigningAuthPlugin{AWSEnvironmentCredentials: &awsEnvironmentCredentialService{}},
	}

	// wrong path: no AWS IAM Role ARN set in environment or config
	err := cs.populateFromEnv()
	assertErr("no AWS_ROLE_ARN set in environment or configuration", err, t)
	t.Setenv("AWS_ROLE_ARN", "role:arn")

	// happy path: set AWS IAM Role ARN as env var
	err = cs.populateFromEnv()
	if err != nil {
		t.Fatalf("Error while getting env vars: %s", err)
	}

	// happy path: set AWS IAM Role ARN in config
	os.Unsetenv("AWS_ROLE_ARN")
	cs.RoleArn = "role:arn"

	err = cs.populateFromEnv()
	if err != nil {
		t.Fatalf("Error while getting env vars: %s", err)
	}

	// wrong path: refresh and get credentials but signing credentials not set via env variables
	_, err = cs.credentials(context.Background())
	assertErr("no AWS_ACCESS_KEY_ID set in environment", err, t)

	t.Setenv("AWS_ACCESS_KEY_ID", "MYAWSACCESSKEYGOESHERE")

	_, err = cs.credentials(context.Background())
	assertErr("no AWS_SECRET_ACCESS_KEY set in environment", err, t)

	t.Setenv("AWS_SECRET_ACCESS_KEY", "MYAWSSECRETACCESSKEYGOESHERE")

	// happy path: refresh and get credentials
	creds, _ := cs.credentials(context.Background())
	assertEq(creds.AccessKey, testAccessKey, t)

	// happy path: refresh with session and get credentials
	cs.expiration = time.Now()
	cs.SessionName = "TEST_SESSION"
	creds, _ = cs.credentials(context.Background())
	assertEq(creds.AccessKey, testAccessKey, t)

	// happy path: don't refresh as credentials not expired so STS not called
	// verify existing credentials haven't changed
	ts.accessKey = "OTHERKEY"
	creds, _ = cs.credentials(context.Background())
	assertEq(creds.AccessKey, testAccessKey, t)

	// happy path: refresh expired credentials
	// verify new credentials are set
	cs.expiration = time.Now()
	creds, _ = cs.credentials(context.Background())
	assertEq(creds.AccessKey, ts.accessKey, t)
}

func TestAssumeRoleCredentialServiceUsingProfileCredentialsProvider(t *testing.T) {
	t.Setenv("AWS_REGION", "us-west-1")

	testAccessKey := "ASgeIAIOSFODNN7EXAMPLE"
	ts := stsTestServer{
		t:                         t,
		accessKey:                 testAccessKey,
		assumeRoleWithWebIdentity: false,
	}
	ts.start()
	defer ts.stop()

	defaultKey := "MYAWSACCESSKEYGOESHERE"
	defaultSecret := "MYAWSSECRETACCESSKEYGOESHERE"
	defaultSessionToken := "AQoEXAMPLEH4aoAH0gNCAPy"

	config := fmt.Sprintf(`
[foo]
aws_access_key_id=%v
aws_secret_access_key=%v
aws_session_token=%v
`, defaultKey, defaultSecret, defaultSessionToken)

	files := map[string]string{
		"example.ini": config,
	}

	test.WithTempFS(files, func(path string) {
		cfgPath := filepath.Join(path, "example.ini")

		cs := awsAssumeRoleCredentialService{
			stsURL:           ts.server.URL,
			logger:           logging.Get(),
			AWSSigningPlugin: &awsSigningAuthPlugin{AWSProfileCredentials: &awsProfileCredentialService{Path: cfgPath, Profile: "foo"}},
		}

		// wrong path: no AWS IAM Role ARN set in environment or config
		err := cs.populateFromEnv()
		assertErr("no AWS_ROLE_ARN set in environment or configuration", err, t)
		t.Setenv("AWS_ROLE_ARN", "role:arn")

		// happy path: set AWS IAM Role ARN as env var
		err = cs.populateFromEnv()
		if err != nil {
			t.Fatalf("Error while getting env vars: %s", err)
		}

		// happy path: set AWS IAM Role ARN in config
		os.Unsetenv("AWS_ROLE_ARN")
		cs.RoleArn = "role:arn"

		err = cs.populateFromEnv()
		if err != nil {
			t.Fatalf("Error while getting env vars: %s", err)
		}

		// happy path: refresh and get credentials
		creds, _ := cs.credentials(context.Background())
		assertEq(creds.AccessKey, testAccessKey, t)

		// happy path: refresh with session and get credentials
		cs.expiration = time.Now()
		cs.SessionName = "TEST_SESSION"
		creds, _ = cs.credentials(context.Background())
		assertEq(creds.AccessKey, testAccessKey, t)

		// happy path: don't refresh as credentials not expired so STS not called
		// verify existing credentials haven't changed
		ts.accessKey = "OTHERKEY"
		creds, _ = cs.credentials(context.Background())
		assertEq(creds.AccessKey, testAccessKey, t)

		// happy path: refresh expired credentials
		// verify new credentials are set
		cs.expiration = time.Now()
		creds, _ = cs.credentials(context.Background())
		assertEq(creds.AccessKey, ts.accessKey, t)
	})
}

func TestAssumeRoleCredentialServiceUsingMetadataCredentialsProvider(t *testing.T) {
	t.Setenv("AWS_REGION", "us-west-1")

	testAccessKey := "ASgeIAIOSFODNN7EXAMPLE"
	ts := stsTestServer{
		t:                         t,
		accessKey:                 testAccessKey,
		assumeRoleWithWebIdentity: false,
	}
	ts.start()
	defer ts.stop()

	tsMetadata := ec2CredTestServer{}
	tsMetadata.payload = metadataPayload{
		AccessKeyID:     "MYAWSACCESSKEYGOESHERE",
		SecretAccessKey: "MYAWSSECRETACCESSKEYGOESHERE",
		Code:            "Success",
		Token:           "MYAWSSECURITYTOKENGOESHERE",
		Expiration:      time.Now().UTC().Add(time.Minute * 300)}
	tsMetadata.start()
	defer tsMetadata.stop()

	cs := awsAssumeRoleCredentialService{
		stsURL: ts.server.URL,
		logger: logging.Get(),
		AWSSigningPlugin: &awsSigningAuthPlugin{AWSMetadataCredentials: &awsMetadataCredentialService{RoleName: "my_iam_role", credServicePath: tsMetadata.server.URL + "/latest/meta-data/iam/security-credentials/",
			tokenPath: tsMetadata.server.URL + "/latest/api/token"}},
	}

	// wrong path: no AWS IAM Role ARN set in environment or config
	err := cs.populateFromEnv()
	assertErr("no AWS_ROLE_ARN set in environment or configuration", err, t)
	t.Setenv("AWS_ROLE_ARN", "role:arn")

	// happy path: set AWS IAM Role ARN as env var
	err = cs.populateFromEnv()
	if err != nil {
		t.Fatalf("Error while getting env vars: %s", err)
	}

	// happy path: set AWS IAM Role ARN in config
	os.Unsetenv("AWS_ROLE_ARN")
	cs.RoleArn = "role:arn"

	err = cs.populateFromEnv()
	if err != nil {
		t.Fatalf("Error while getting env vars: %s", err)
	}

	// happy path: refresh and get credentials
	creds, _ := cs.credentials(context.Background())
	assertEq(creds.AccessKey, testAccessKey, t)

	// happy path: refresh with session and get credentials
	cs.expiration = time.Now()
	cs.SessionName = "TEST_SESSION"
	creds, _ = cs.credentials(context.Background())
	assertEq(creds.AccessKey, testAccessKey, t)

	// happy path: don't refresh as credentials not expired so STS not called
	// verify existing credentials haven't changed
	ts.accessKey = "OTHERKEY"
	creds, _ = cs.credentials(context.Background())
	assertEq(creds.AccessKey, testAccessKey, t)

	// happy path: refresh expired credentials
	// verify new credentials are set
	cs.expiration = time.Now()
	creds, _ = cs.credentials(context.Background())
	assertEq(creds.AccessKey, ts.accessKey, t)
}

func TestStsPath(t *testing.T) {
	cs := awsWebIdentityCredentialService{}

	defaultPath := fmt.Sprintf(stsDefaultPath, stsDefaultDomain)
	assertEq(defaultPath, cs.stsPath(), t)

	cs.RegionName = "us-east-2"
	assertEq("https://sts.us-east-2.amazonaws.com", cs.stsPath(), t)

	cs.Domain = "example.com"
	assertEq("https://sts.us-east-2.example.com", cs.stsPath(), t)

	cs.stsURL = "http://test.com"
	assertEq("http://test.com", cs.stsPath(), t)
}

func TestStsPathFromEnv(t *testing.T) {
	t.Setenv(awsRoleArnEnvVar, "role:arn")
	t.Setenv(awsWebIdentityTokenFileEnvVar, "/nonsense")

	tests := []struct {
		note string
		env  map[string]string
		cs   awsWebIdentityCredentialService
		want string
	}{
		{
			note: "region set in config",
			cs: awsWebIdentityCredentialService{
				RegionName: "us-east-2",
			},
			want: "https://sts.us-east-2.amazonaws.com",
		},
		{
			note: "region set in env",
			env: map[string]string{
				awsRegionEnvVar: "us-east-1",
			},
			want: "https://sts.us-east-1.amazonaws.com",
		},
		{
			note: "region set in env and config (config wins)",
			env: map[string]string{
				awsRegionEnvVar: "us-east-1",
			},
			cs: awsWebIdentityCredentialService{
				RegionName: "us-east-2",
			},
			want: "https://sts.us-east-2.amazonaws.com",
		},
		{
			note: "domain set in config",
			cs: awsWebIdentityCredentialService{
				RegionName: "us-east-2",
				Domain:     "foo.example.com",
			},
			want: "https://sts.us-east-2.foo.example.com",
		},
		{
			note: "domain set in env",
			env: map[string]string{
				awsDomainEnvVar: "bar.example.com",
			},
			cs: awsWebIdentityCredentialService{
				RegionName: "us-east-2", // Region must always be set
			},
			want: "https://sts.us-east-2.bar.example.com",
		},
		{
			note: "domain set in env and config (config wins)",
			env: map[string]string{
				awsDomainEnvVar: "bar.example.com",
			},
			cs: awsWebIdentityCredentialService{
				RegionName: "us-east-2", // Region must always be set
				Domain:     "foo.example.com",
			},
			want: "https://sts.us-east-2.foo.example.com",
		},
	}

	for _, tc := range tests {
		t.Run(tc.note, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if err := tc.cs.populateFromEnv(); err != nil {
				t.Fatalf("Unexpected err: %s", err)
			}
			assertEq(tc.want, tc.cs.stsPath(), t)
		})
	}
}

// simulate AWS Security Token Service (AWS STS)
type stsTestServer struct {
	t                         *testing.T
	server                    *httptest.Server
	accessKey                 string
	assumeRoleWithWebIdentity bool
}

func (t *stsTestServer) handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" || r.Method != http.MethodPost {
		w.WriteHeader(404)
		return
	}

	if err := r.ParseForm(); err != nil {
		w.WriteHeader(400)
		return
	}

	if r.PostForm.Get("Action") != "AssumeRoleWithWebIdentity" && r.PostForm.Get("Action") != "AssumeRole" {
		w.WriteHeader(400)
		return
	}

	if r.PostForm.Get("RoleArn") == "BrokenRole" {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("{}"))
		return
	}

	if t.assumeRoleWithWebIdentity {
		token := r.PostForm.Get("WebIdentityToken")
		if token != "good-token" {
			w.WriteHeader(401)
			return
		}
		w.WriteHeader(200)
	}

	sessionName := r.PostForm.Get("RoleSessionName")

	var xmlResponse string

	if t.assumeRoleWithWebIdentity {
		// Taken from STS docs: https://docs.aws.amazon.com/STS/latest/APIReference/API_AssumeRoleWithWebIdentity.html
		xmlResponse = `<AssumeRoleWithWebIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
	<AssumeRoleWithWebIdentityResult>
	  <SubjectFromWebIdentityToken>amzn1.account.AF6RHO7KZU5XRVQJGXK6HB56KR2A</SubjectFromWebIdentityToken>
	  <Audience>client.5498841531868486423.1548@apps.example.com</Audience>
	  <AssumedRoleUser>
		<Arn>arn:aws:sts::123456789012:assumed-role/FederatedWebIdentityRole/%[1]s</Arn>
		<AssumedRoleId>AROACLKWSDQRAOEXAMPLE:%[1]s</AssumedRoleId>
	  </AssumedRoleUser>
	  <Credentials>
		<SessionToken>AQoDYXdzEE0a8ANXXXXXXXXNO1ewxE5TijQyp+IEXAMPLE</SessionToken>
		<SecretAccessKey>wJalrXUtnFEMI/K7MDENG/bPxRfiCYzEXAMPLEKEY</SecretAccessKey>
		<Expiration>%s</Expiration>
		<AccessKeyId>%s</AccessKeyId>
	  </Credentials>
	  <Provider>www.amazon.com</Provider>
	</AssumeRoleWithWebIdentityResult>
	<ResponseMetadata>
	  <RequestId>ad4156e9-bce1-11e2-82e6-6b6efEXAMPLE</RequestId>
	</ResponseMetadata>
  </AssumeRoleWithWebIdentityResponse>`
	} else {
		// Taken from STS docs: https://docs.aws.amazon.com/STS/latest/APIReference/API_AssumeRole.html
		xmlResponse = `<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
<AssumeRoleResult>
<SourceIdentity>DevUser123</SourceIdentity>
<AssumedRoleUser>
  <Arn>arn:aws:sts::123456789012:assumed-role/demo/John</Arn>
  <AssumedRoleId>ARO123EXAMPLE123:%[1]s</AssumedRoleId>
</AssumedRoleUser>
<Credentials>
  <SessionToken>
   AQoDYXdzEPT//////////wEXAMPLEtc764bNrC9SAPBSM22wDOk4x4HIZ8j4FZTwdQW
   LWsKWHGBuFqwAeMicRXmxfpSPfIeoIYRqTflfKD8YUuwthAx7mSEI/qkPpKPi/kMcGd
   QrmGdeehM4IC1NtBmUpp2wUE8phUZampKsburEDy0KPkyQDYwT7WZ0wq5VSXDvp75YU
   9HFvlRd8Tx6q6fE8YQcHNVXAkiY9q6d+xo0rKwT38xVqr7ZD0u0iPPkUL64lIZbqBAz
   +scqKmlzm8FDrypNC9Yjc8fPOLn9FX9KSYvKTr4rvx3iSIlTJabIQwj2ICCR/oLxBA==
  </SessionToken>
  <SecretAccessKey>
   wJalrXUtnFEMI/K7MDENG/bPxRfiCYzEXAMPLEKEY
  </SecretAccessKey>
  <Expiration>%s</Expiration>
  <AccessKeyId>%s</AccessKeyId>
</Credentials>
<PackedPolicySize>8</PackedPolicySize>
</AssumeRoleResult>
<ResponseMetadata>
<RequestId>c6104cbe-af31-11e0-8154-cbc7ccf896c7</RequestId>
</ResponseMetadata>
</AssumeRoleResponse>`
	}

	_, _ = w.Write(fmt.Appendf(nil, xmlResponse, sessionName, time.Now().Add(time.Hour).Format(time.RFC3339), t.accessKey))
}

func (t *stsTestServer) start() {
	t.server = httptest.NewServer(http.HandlerFunc(t.handle))
}

func (t *stsTestServer) stop() {
	t.server.Close()
}

func TestECRAuthPluginFailsWithoutAWSAuthPlugins(t *testing.T) {
	ap := newECRAuthPlugin(&awsSigningAuthPlugin{
		logger: logging.NewNoOpLogger(),
	})

	req := httptest.NewRequest("", "http://somewhere.com", nil)

	err := ap.Prepare(req)
	if err == nil {
		t.Error("ecrAuthPlugin.Prepare(): expected and error")
	}
}

func TestECRAuthPluginRequestsAuthorizationToken(t *testing.T) {
	// Environment credentials to sign the ecr get authorization token request
	t.Setenv(accessKeyEnvVar, "blablabla")
	t.Setenv(secretKeyEnvVar, "tatata")
	t.Setenv(awsRegionEnvVar, "us-east-1")
	t.Setenv(sessionTokenEnvVar, "lalala")

	awsAuthPlugin := awsSigningAuthPlugin{
		logger:                    logging.NewNoOpLogger(),
		AWSEnvironmentCredentials: &awsEnvironmentCredentialService{},
	}

	ap := newECRAuthPlugin(&awsAuthPlugin)
	ap.ecr = &ecrStub{token: aws.ECRAuthorizationToken{
		AuthorizationToken: "secret",
	}}

	req := httptest.NewRequest("", "http://somewhere.com", nil)

	if err := ap.Prepare(req); err != nil {
		t.Errorf("ecrAuthPlugin.Prepare() = %q", err)
	}

	got := req.Header.Get("Authorization")
	want := "Basic secret"
	if got != want {
		t.Errorf("req.Header.Get(\"Authorization\") = %q, want %q", got, want)
	}
}

func TestECRAuthPluginRequestsRedirection(t *testing.T) {
	// Environment credentials to sign the ecr get authorization token request
	t.Setenv(accessKeyEnvVar, "blablabla")
	t.Setenv(secretKeyEnvVar, "tatata")
	t.Setenv(awsRegionEnvVar, "us-east-1")
	t.Setenv(sessionTokenEnvVar, "lalala")

	ap := awsSigningAuthPlugin{
		logger:                    logging.NewNoOpLogger(),
		AWSEnvironmentCredentials: &awsEnvironmentCredentialService{},
		host:                      "somewhere.com",
		AWSService:                "ecr",
	}

	apECR := newECRAuthPlugin(&ap)
	apECR.ecr = &ecrStub{token: aws.ECRAuthorizationToken{
		AuthorizationToken: "secret",
	}}

	ap.ecrAuthPlugin = apECR

	// Request to the host specified in the plugin configuration
	req := httptest.NewRequest("", "http://somewhere.com", nil)

	if err := ap.Prepare(req); err != nil {
		t.Errorf("ecrAuthPlugin.Prepare() = %q", err)
	}

	got := req.Header.Get("Authorization")
	want := "Basic secret"
	if got != want {
		t.Errorf("req.Header.Get(\"Authorization\") = %q, want %q", got, want)
	}

	// Redirection to another host
	req = httptest.NewRequest("", "http://somewhere-else.com", nil)

	if err := ap.Prepare(req); err != nil {
		t.Errorf("ecrAuthPlugin.Prepare() = %q", err)
	}

	got = req.Header.Get("Authorization")
	want = ""
	if got != want {
		t.Errorf("req.Header.Get(\"Authorization\") = %q, want %q", got, want)
	}
}

type ecrStub struct {
	token aws.ECRAuthorizationToken
}

func (es *ecrStub) GetAuthorizationToken(context.Context, aws.Credentials, string) (aws.ECRAuthorizationToken, error) {
	return es.token, nil
}

func TestECRAuthPluginReusesCachedToken(t *testing.T) {
	logger := logging.NewNoOpLogger()
	ap := ecrAuthPlugin{
		token: aws.ECRAuthorizationToken{
			AuthorizationToken: "secret",
			ExpiresAt:          time.Now().Add(time.Hour),
		},
		awsAuthPlugin: &awsSigningAuthPlugin{
			logger: logger,
		},
		logger: logger,
	}

	req := httptest.NewRequest("", "http://somewhere.com", nil)

	if err := ap.Prepare(req); err != nil {
		t.Errorf("ecrAuthPlugin.Prepare() = %q", err)
	}

	got := req.Header.Get("Authorization")
	want := "Basic secret"
	if got != want {
		t.Errorf("req.Header.Get(\"Authorization\") = %q, want %q", got, want)
	}
}

func TestSSOCredentialService(t *testing.T) {
	// Create a temporary directory for test files
	tempDir := t.TempDir()

	// Create test config file
	configPath := filepath.Join(tempDir, "config")
	configContent := `
[profile test-profile]
sso_account_id = 123456789012
sso_role_name = TestRole
sso_session = test-session
region = us-east-1

[sso-session test-session]
sso_start_url = https://test.awsapps.com/start
sso_region = us-east-1
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Create test cache file
	cachePath := filepath.Join(tempDir, "sso", "cache")
	if err := os.MkdirAll(cachePath, 0755); err != nil {
		t.Fatalf("Failed to create cache dir: %v", err)
	}

	// Calculate the cache file name using the session name
	sessionName := "test-session"
	hash := sha1.New()
	hash.Write([]byte(sessionName))
	cacheKey := fmt.Sprintf("%x.json", hash.Sum(nil))
	cacheFile := filepath.Join(cachePath, cacheKey)

	// Set up test times
	futureTime := time.Now().Add(24 * time.Hour)
	expiredTime := time.Now().Add(-1 * time.Hour)

	cacheContent := fmt.Sprintf(`{
		"startUrl": "https://test.awsapps.com/start",
		"region": "us-east-1",
		"accessToken": "test-access-token",
		"expiresAt": %q,
		"registrationExpiresAt": %q,
		"refreshToken": "test-refresh-token",
		"clientId": "test-client-id",
		"clientSecret": "test-client-secret"
	}`, futureTime.Format(time.RFC3339), futureTime.Format(time.RFC3339))

	if err := os.WriteFile(cacheFile, []byte(cacheContent), 0644); err != nil {
		t.Fatalf("Failed to write cache file: %v", err)
	}

	tests := []struct {
		name          string
		configPath    string
		profile       string
		cacheFile     string
		modifySession func(*ssoSessionDetails)
		modifyCreds   func(*aws.Credentials)
		expectError   bool
		errorContains string
		setupTest     func()
	}{
		{
			name:          "missing config path",
			configPath:    "/nonexistent/path",
			profile:       "test-profile",
			expectError:   true,
			errorContains: "failed to load config file",
		},
		{
			name:          "invalid profile",
			configPath:    configPath,
			profile:       "nonexistent-profile",
			expectError:   true,
			errorContains: "failed to find profile",
		},
		{
			name:       "successful credentials",
			configPath: configPath,
			profile:    "test-profile",
			cacheFile:  cacheFile,
			modifyCreds: func(creds *aws.Credentials) {
				creds.AccessKey = "test-access-key"
				creds.SecretKey = "test-secret-key"
				creds.SessionToken = "test-session-token"
				creds.RegionName = "us-east-1"
			},
		},
		{
			name:       "expired access token",
			configPath: configPath,
			profile:    "test-profile",
			cacheFile:  cacheFile,
			modifySession: func(session *ssoSessionDetails) {
				session.ExpiresAt = expiredTime
				session.RegistrationExpiresAt = futureTime
			},
			modifyCreds: func(creds *aws.Credentials) {
				creds.AccessKey = "test-access-key"
				creds.SecretKey = "test-secret-key"
				creds.SessionToken = "test-session-token"
				creds.RegionName = "us-east-1"
			},
		},
		{
			name:       "expired registration",
			configPath: configPath,
			profile:    "test-profile",
			cacheFile:  cacheFile,
			modifySession: func(session *ssoSessionDetails) {
				session.ExpiresAt = expiredTime
				session.RegistrationExpiresAt = expiredTime
			},
			expectError:   true,
			errorContains: "cannot refresh token, registration expired",
		},
		{
			name:          "missing refresh token",
			configPath:    configPath,
			profile:       "test-profile",
			cacheFile:     cacheFile,
			expectError:   true,
			errorContains: "failed to refresh token",
			setupTest: func() {
				// Create a session with expired access token and missing refresh token
				session := &ssoSessionDetails{
					StartUrl:              "https://test.awsapps.com/start",
					Region:                "us-east-1",
					AccessToken:           "test-access-token",
					ExpiresAt:             expiredTime,
					RegistrationExpiresAt: futureTime,
					ClientId:              "test-client-id",
					ClientSecret:          "test-client-secret",
				}
				modifiedContent, err := json.Marshal(session)
				if err != nil {
					t.Fatalf("Failed to marshal session: %v", err)
				}
				if err := os.WriteFile(cacheFile, modifiedContent, 0644); err != nil {
					t.Fatalf("Failed to write modified cache file: %v", err)
				}
			},
		},
		{
			name:       "expired credentials",
			configPath: configPath,
			profile:    "test-profile",
			cacheFile:  cacheFile,
			modifyCreds: func(creds *aws.Credentials) {
				creds.AccessKey = "test-access-key"
				creds.SecretKey = "test-secret-key"
				creds.SessionToken = "test-session-token"
				creds.RegionName = "us-east-1"
			},
		},
		{
			name:          "invalid cache file",
			configPath:    configPath,
			profile:       "test-profile",
			cacheFile:     cacheFile,
			expectError:   true,
			errorContains: "failed to unmarshal cache file",
			setupTest: func() {
				// Write invalid JSON to cache file before running the test
				if err := os.WriteFile(cacheFile, []byte("invalid json"), 0644); err != nil {
					t.Fatalf("Failed to write invalid cache file: %v", err)
				}
			},
		},
		{
			name:          "missing cache file",
			configPath:    configPath,
			profile:       "test-profile",
			cacheFile:     filepath.Join(cachePath, "nonexistent.json"),
			expectError:   true,
			errorContains: "failed to load cache file",
			setupTest: func() {
				// Remove the cache file if it exists
				if err := os.Remove(cacheFile); err != nil && !os.IsNotExist(err) {
					t.Fatalf("Failed to remove cache file: %v", err)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Reset files to initial state before each test
			if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
				t.Fatalf("Failed to reset config file: %v", err)
			}
			if err := os.WriteFile(cacheFile, []byte(cacheContent), 0644); err != nil {
				t.Fatalf("Failed to reset cache file: %v", err)
			}

			// Run any test-specific setup
			if tc.setupTest != nil {
				tc.setupTest()
			}

			// Create service with test configuration
			service := &awsSSOCredentialsService{
				Path:         tc.configPath,
				SSOCachePath: filepath.Dir(tc.cacheFile),
				Profile:      tc.profile,
				logger:       logging.NewNoOpLogger(),
			}

			// Modify session details if needed
			if tc.modifySession != nil {
				session := &ssoSessionDetails{}
				if err := json.Unmarshal([]byte(cacheContent), session); err != nil {
					t.Fatalf("Failed to unmarshal session: %v", err)
				}
				tc.modifySession(session)
				modifiedContent, err := json.Marshal(session)
				if err != nil {
					t.Fatalf("Failed to marshal modified session: %v", err)
				}
				if err := os.WriteFile(tc.cacheFile, modifiedContent, 0644); err != nil {
					t.Fatalf("Failed to write modified cache file: %v", err)
				}
			}

			// Get credentials
			creds, err := service.credentials(context.Background())

			// Verify results
			if tc.expectError {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				if !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("expected error to contain %q, got %q", tc.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if tc.modifyCreds != nil {
					tc.modifyCreds(&creds)
				}
				if creds.AccessKey == "" {
					t.Error("expected non-empty access key")
				}
				if creds.SecretKey == "" {
					t.Error("expected non-empty secret key")
				}
				if creds.SessionToken == "" {
					t.Error("expected non-empty session token")
				}
				if creds.RegionName != "us-east-1" {
					t.Errorf("expected region us-east-1, got %q", creds.RegionName)
				}
			}
		})
	}
}

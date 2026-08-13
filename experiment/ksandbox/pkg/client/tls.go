/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package client

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	"k8s.io/client-go/util/keyutil"
)

// buildTLS constructs our single-use PKI infrastructure (CA, server cert, client cert)
func (c *AgentClient) buildTLS() error {
	now := time.Now()

	{
		key, err := ecdsa.GenerateKey(elliptic.P384(), cryptorand.Reader)
		if err != nil {
			return fmt.Errorf("failed to generate key: %w", err)
		}
		tmpl := x509.Certificate{
			SerialNumber: big.NewInt(1),
			Subject: pkix.Name{
				CommonName: "buildagent-ca",
			},
			NotBefore:             now.Add(-1 * time.Hour).UTC(),
			NotAfter:              now.Add(24 * time.Hour).UTC(),
			KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
			BasicConstraintsValid: true,
			IsCA:                  true,
		}

		certDERBytes, err := x509.CreateCertificate(cryptorand.Reader, &tmpl, &tmpl, key.Public(), key)
		if err != nil {
			return err
		}
		c.caCertBytes = certDERBytes
		cert, err := x509.ParseCertificate(certDERBytes)
		if err != nil {
			return err
		}
		c.caCert = cert
		c.caKey = key
	}

	{
		key, err := ecdsa.GenerateKey(elliptic.P384(), cryptorand.Reader)
		if err != nil {
			return fmt.Errorf("failed to generate key: %w", err)
		}
		tmpl := x509.Certificate{
			SerialNumber: big.NewInt(2),
			DNSNames:     []string{ClientCertificateDNSName},
			NotBefore:    now.Add(-1 * time.Hour).UTC(),
			NotAfter:     now.Add(24 * time.Hour).UTC(),

			BasicConstraintsValid: true,
			IsCA:                  false,

			KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		}

		certDERBytes, err := x509.CreateCertificate(cryptorand.Reader, &tmpl, c.caCert, key.Public(), c.caKey)
		if err != nil {
			return err
		}
		c.clientCertBytes = certDERBytes
		c.clientKey = key
	}

	{
		key, err := ecdsa.GenerateKey(elliptic.P384(), cryptorand.Reader)
		if err != nil {
			return fmt.Errorf("failed to generate key: %w", err)
		}
		tmpl := x509.Certificate{
			SerialNumber: big.NewInt(3),
			DNSNames:     []string{ServerCertificateDNSName},
			NotBefore:    now.Add(-1 * time.Hour).UTC(),
			NotAfter:     now.Add(24 * time.Hour).UTC(),

			BasicConstraintsValid: true,
			IsCA:                  false,

			KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		}

		certDERBytes, err := x509.CreateCertificate(cryptorand.Reader, &tmpl, c.caCert, key.Public(), c.caKey)
		if err != nil {
			return err
		}
		c.serverCertBytes = certDERBytes
		c.serverKey = key
	}

	return nil
}

// keyToPEM converts a PrivateKey to PEM format
func keyToPEM(key *ecdsa.PrivateKey) ([]byte, error) {
	var b bytes.Buffer
	asn, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	if err := pem.Encode(&b, &pem.Block{Type: keyutil.ECPrivateKeyBlockType, Bytes: asn}); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// certToPEM converts a certificate to PEM format
func certToPEM(certBytes []byte) ([]byte, error) {
	var b bytes.Buffer
	if err := pem.Encode(&b, &pem.Block{Type: "CERTIFICATE", Bytes: certBytes}); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

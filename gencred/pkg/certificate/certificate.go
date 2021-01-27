/*
Copyright 2019 The Kubernetes Authors.

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

package certificate

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	certificates "k8s.io/api/certificates/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api/v1"
	"k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/keyutil"
	"k8s.io/test-infra/gencred/pkg/kubeconfig"
)

const (
	// systemPrivilegedGroup is a superuser by default (i.e. bound to the cluster-admin ClusterRole).
	systemPrivilegedGroup = "system:masters"
	// waitInterval request poll interval.
	waitInterval = time.Second
	// waitTimeout request poll timeout.
	waitTimeout = 20 * time.Second
)

// generateCSR generates a certificate signing request (CSR).
func generateCSR() (*certificates.CertificateSigningRequest, []byte, error) {

	// Generate a new private key.
	pk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate key: %v", err)
	}

	// Marshal pk -> der.
	der, err := x509.MarshalECPrivateKey(pk)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal key to DER: %v", err)
	}

	// Generate PEM key.
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: keyutil.ECPrivateKeyBlockType, Bytes: der})

	// Generate a x509 certificate signing request.
	csrPEM, err := cert.MakeCSR(pk, &pkix.Name{CommonName: "client", Organization: []string{systemPrivilegedGroup}}, nil, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("create CSR from key: %v", err)
	}

	// Generate a Kubernetes CSR object.
	csrObj := &certificates.CertificateSigningRequest{
		// Username, UID, Groups will be injected by API server.
		ObjectMeta: metav1.ObjectMeta{
			Name:         "",
			GenerateName: "csr-",
		},
		Spec: certificates.CertificateSigningRequestSpec{
			Request: csrPEM,
			Usages: []certificates.KeyUsage{
				certificates.UsageDigitalSignature,
				certificates.UsageKeyEncipherment,
				certificates.UsageClientAuth,
			},
		},
	}

	return csrObj, keyPEM, nil
}

// requestCSR requests a certificate signing request (CSR).
func requestCSR(clientset kubernetes.Interface, csrObj *certificates.CertificateSigningRequest) ([]byte, error) {
	client := clientset.CertificatesV1beta1().CertificateSigningRequests()

	// Create CSR.
	csrObj, err := client.Create(context.TODO(), csrObj, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("create CSR: %v", err)
	}

	csrName := csrObj.Name
	appendApprovalCondition(csrObj)

	// Approve CSR.
	err = wait.Poll(waitInterval, waitTimeout, func() (bool, error) {
		_, err = client.UpdateApproval(context.TODO(), csrObj, metav1.UpdateOptions{})
		if err != nil {
			return false, err
		}

		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("approve CSR: %v", err)
	}

	// Get CSR.
	err = wait.Poll(waitInterval, waitTimeout, func() (bool, error) {
		csrObj, err = client.Get(context.TODO(), csrName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("get CSR: %v", err)
	}

	return csrObj.Status.Certificate, nil
}

// getRootCA fetches the service account root certificate authority (CA).
func getRootCA(clientset kubernetes.Interface) ([]byte, error) {
	secrets, err := clientset.CoreV1().Secrets(metav1.NamespaceSystem).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	if len(secrets.Items) == 0 {
		return nil, errors.New("locate secrets")
	}

	caPEM, ok := secrets.Items[0].Data[corev1.ServiceAccountRootCAKey]
	if !ok {
		return nil, errors.New("locate root CA")
	}

	return caPEM, nil
}

// appendApprovalCondition appends the approval condition to the certificate signing request (CSR).
func appendApprovalCondition(csr *certificates.CertificateSigningRequest) {
	csr.Status.Conditions = append(csr.Status.Conditions, certificates.CertificateSigningRequestCondition{
		Type:           certificates.CertificateApproved,
		Reason:         "GenCredApprove",
		Message:        "This CSR was approved by gencred.",
		LastUpdateTime: metav1.Now(),
	})
}

// CreateClusterCertificateCredentials creates a client certificate and key to authenticate to a cluster API server.
func CreateClusterCertificateCredentials(clientset kubernetes.Interface) (certPEM []byte, keyPEM []byte, caPEM []byte, err error) {
	csrObj, keyPEM, err := generateCSR()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("generate CSR: %v", err)
	}

	certPEM, err = requestCSR(clientset, csrObj)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("request CSR: %v", err)
	}

	caPEM, err = getRootCA(clientset)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("get root CA: %v", err)
	}

	return certPEM, keyPEM, caPEM, nil
}

// CreateKubeConfigWithCertificateCredentials creates a kube config containing a certificate and key to authenticate to a Kubernetes cluster API server.
func CreateKubeConfigWithCertificateCredentials(clientset kubernetes.Interface, name string) ([]byte, error) {
	certPEM, keyPEM, caPEM, err := CreateClusterCertificateCredentials(clientset)
	if err != nil {
		return nil, err
	}

	authInfo := clientcmdapi.AuthInfo{
		ClientCertificateData: certPEM,
		ClientKeyData:         keyPEM,
	}

	return kubeconfig.CreateKubeConfig(clientset, name, caPEM, authInfo)
}

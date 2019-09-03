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

package gencred

import (
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
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/keyutil"
)

const (
	// clusterRoleBindingName is the name for the cluster administrator ClusterRoleBinding.
	clusterRoleBindingName = "serviceaccount-cluster-admin-crb"
	// serviceAccountName is the name for the cluster administrator ServiceAccount.
	serviceAccountName = "serviceaccount-cluster-admin"
	// systemPrivilegedGroup is a superuser by default (i.e. bound to the cluster-admin ClusterRole).
	systemPrivilegedGroup = "system:masters"
	// waitInterval request poll interval.
	waitInterval = time.Second
	// waitTimeout request poll timeout.
	waitTimeout = 20 * time.Second
)

// getOrCreateSA gets existing or creates new service account (SA).
func getOrCreateSA(clientset kubernetes.Interface) ([]byte, []byte, error) {
	client := clientset.CoreV1().ServiceAccounts(corev1.NamespaceDefault)

	// Get ServiceAccount if exists.
	if saObj, err := client.Get(serviceAccountName, metav1.GetOptions{}); err == nil {
		return getSASecrets(clientset, saObj)
	}

	// Generate a Kubernetes ServiceAccount object.
	saObj := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: serviceAccountName,
		},
	}

	// Create ServiceAccount.
	saObj, err := client.Create(saObj)
	if err != nil {
		return nil, nil, fmt.Errorf("create SA: %v", err)
	}

	// Get/Create ClusterRoleBinding.
	err = getOrCreateCRB(clientset)
	if err != nil {
		return nil, nil, fmt.Errorf("get or create CRB: %v", err)
	}

	// Get ServiceAccount.
	saObj, err = client.Get(serviceAccountName, metav1.GetOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("get SA: %v", err)
	}

	return getSASecrets(clientset, saObj)
}

// getOrCreateCRB gets existing or creates new cluster role binding (CRB).
func getOrCreateCRB(clientset kubernetes.Interface) error {
	client := clientset.RbacV1().ClusterRoleBindings()

	// Get ClusterRoleBinding if exists.
	if _, err := client.Get(clusterRoleBindingName, metav1.GetOptions{}); err == nil {
		return nil
	}

	// Generate a Kubernetes ClusterRoleBinding object.
	crbObj := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: clusterRoleBindingName},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: serviceAccountName, Namespace: corev1.NamespaceDefault}},
		RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "cluster-admin"},
	}

	// Create ClusterRoleBinding.
	_, err := client.Create(crbObj)
	if err != nil {
		return fmt.Errorf("create CRB: %v", err)
	}

	return nil
}

// getSASecrets gets service account token and root CA secrets.
func getSASecrets(clientset kubernetes.Interface, saObj *corev1.ServiceAccount) ([]byte, []byte, error) {
	client := clientset.CoreV1().Secrets(corev1.NamespaceDefault)

	if len(saObj.Secrets) == 0 {
		return nil, nil, errors.New("locate secrets")
	}

	// Get Secret.
	secretObj, err := client.Get(saObj.Secrets[0].Name, metav1.GetOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("get secret: %v", err)
	}

	token, ok := secretObj.Data[corev1.ServiceAccountTokenKey]
	if !ok {
		return nil, nil, errors.New("locate token")
	}

	caPEM, ok := secretObj.Data[corev1.ServiceAccountRootCAKey]
	if !ok {
		return nil, nil, errors.New("locate root CA")
	}

	return token, caPEM, nil
}

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
	csrObj, err := client.Create(csrObj)
	if err != nil {
		return nil, fmt.Errorf("create CSR: %v", err)
	}

	csrName := csrObj.Name

	// Approve CSR.
	err = wait.Poll(waitInterval, waitTimeout, func() (bool, error) {
		appendApprovalCondition(csrObj)
		csrObj, err = client.UpdateApproval(csrObj)
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
		csrObj, err = client.Get(csrName, metav1.GetOptions{})
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
	secrets, err := clientset.CoreV1().Secrets(metav1.NamespaceSystem).List(metav1.ListOptions{})
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
		Reason:         "GenCertApprove",
		Message:        "This CSR was approved by gencert.",
		LastUpdateTime: metav1.Now(),
	})
}

// CreateClusterCertificateCredentials creates a client certificate and key to authenticate to a cluster API server
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

func CreateClusterServiceAccountCredentials(clientset kubernetes.Interface) (token []byte, caPEM []byte, err error) {
	token, caPEM, err = getOrCreateSA(clientset)
	if err != nil {
		return nil, nil, fmt.Errorf("get or create SA: %v", err)
	}

	return token, caPEM, nil
}

// TODO(clarketm): implement `Create` method that returns a kube config file as string.
// CreateKubeConfigCredentials creates a kube config containing a certificate and key to authenticate to a Kubernetes cluster API server
//func CreateKubeConfigCredentials(clientset kubernetes.Interface) (string, error) {}

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

package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"google.golang.org/grpc/credentials"
	"k8s.io/klog/v2"
	"k8s.io/test-infra/experiment/ksandbox/pkg/server"
)

func main() {
	ctx := context.Background()
	err := run(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	klog.InitFlags(nil)

	listen := ":7007"
	flag.StringVar(&listen, "listen", listen, "port on which to listen for requests")
	tlsDir := "tls"
	flag.StringVar(&tlsDir, "tls-dir", tlsDir, "directory for tls credentials")
	deleteTLS := true
	flag.BoolVar(&deleteTLS, "delete-tls", deleteTLS, "automatically delete tls credentials after reading")

	installDir := ""
	flag.StringVar(&installDir, "install", installDir, "copy into this directory and exit")

	flag.Parse()

	if installDir != "" {
		return installTo(installDir)
	}

	// TODO: Auto-shutdown after 1 hour?

	s, err := server.NewAgentServer()
	if err != nil {
		return err
	}

	var creds credentials.TransportCredentials

	if tlsDir != "" {
		// Load our serving certificate and enforce client certificates
		certFile := filepath.Join(tlsDir, "server.crt")
		keyFile := filepath.Join(tlsDir, "server.key")

		serverKeypair, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return fmt.Errorf("failed to load TLS credentials from %q: %w", tlsDir, err)
		}

		clientCACertPath := filepath.Join(tlsDir, "client-ca.crt")
		clientCACertBytes, err := os.ReadFile(clientCACertPath)
		if err != nil {
			return fmt.Errorf("failed to read %q: %w", clientCACertPath, err)
		}
		clientCACertPool := x509.NewCertPool()
		if !clientCACertPool.AppendCertsFromPEM(clientCACertBytes) {
			return fmt.Errorf("failed to parse any certificates from %q: %w", clientCACertPath, err)
		}

		creds = credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{serverKeypair},
			ClientCAs:    clientCACertPool,
			ClientAuth:   tls.RequireAndVerifyClientCert,
		})

		if deleteTLS {
			// We delete TLS certificates so that they aren't sitting on disk.
			// This isn't perfect, but it prevents trival and accidental leakage.
			// The credentials aren't particular high-value anyway - they are single-use (and we connect _to_ the pod)
			// TODO: Should we delete the ksandbox-agent binary, just so we don't have anything else obviously on the disk?
			if err := os.RemoveAll(tlsDir); err != nil {
				return fmt.Errorf("unable to delete tls credentials: %w", err)
			}
		}
	}

	if err := s.ListenAndServe(listen, creds); err != nil {
		return err
	}

	return nil
}

// copyFile copies the file from src to dest, setting the mode of the created file
func copyFile(src, dest string, mode os.FileMode) error {
	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("unable to open %q: %w", src, err)
	}
	defer f.Close()

	out, err := os.OpenFile(dest, os.O_RDWR|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("unable to create %q: %w", dest, err)
	}
	if _, err := io.Copy(out, f); err != nil {
		out.Close()
		return fmt.Errorf("error writing %q: %w", dest, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("error closing %q: %w", dest, err)
	}
	return nil
}

// installTo copies the agent and PKI keys to the specified
// directory.
//
// installDir will normally be a shared volume mount which is then
// used as the entrypoint for the main container.
func installTo(installDir string) error {
	installBin := filepath.Join(installDir, "ksandbox-agent")

	if err := copyFile(os.Args[0], installBin, os.FileMode(0755)); err != nil {
		return fmt.Errorf("error copying file %q: %w", os.Args[0], err)
	}

	// Also copy TLS material (so we can delete it, since https://github.com/kubernetes/kubernetes/pull/58720)
	{
		copySrcDir := "/tls"
		copyDestDir := filepath.Join(installDir, "tls")
		if err := os.MkdirAll(copyDestDir, 0700); err != nil {
			return fmt.Errorf("error creating %q: %w", copyDestDir, err)
		}

		files, err := os.ReadDir(copySrcDir)
		if err != nil {
			return fmt.Errorf("error reading %q directory: %w", copySrcDir, err)
		}

		for _, f := range files {
			if f.IsDir() {
				continue
			}

			src := filepath.Join(copySrcDir, f.Name())
			if filepath.Ext(src) != ".crt" && filepath.Ext(src) != ".key" {
				klog.Infof("skipping copy of %q; isn't .crt or .key", src)
				continue
			}

			out := filepath.Join(copyDestDir, f.Name())
			if err := copyFile(src, out, os.FileMode(0600)); err != nil {
				return fmt.Errorf("error copying file %q: %w", src, err)
			}
		}
	}

	return nil
}

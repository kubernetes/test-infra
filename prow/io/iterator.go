/*
Copyright 2020 The Kubernetes Authors.

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

package io

import (
	"context"
	"io"
	"strings"

	"cloud.google.com/go/storage"
	"gocloud.dev/blob"
	"google.golang.org/api/iterator"
)

type ObjectAttributes struct {
	// Name is the full path of the object or directory
	Name string
	// ObjName is the last segment of the name in case of an object
	ObjName string
	// IsDir is true if the object is a directory
	IsDir bool
}

// ObjectIterator iterates through storage objects
// It returns attr as long as it finds objects. When no
// objects can be found anymore io.EOF error is returned
type ObjectIterator interface {
	Next(ctx context.Context) (attr ObjectAttributes, err error)
}

// gcsObjectIterator implements ObjectIterator for GCS
type gcsObjectIterator struct {
	Iterator *storage.ObjectIterator
}

func (g gcsObjectIterator) Next(_ context.Context) (ObjectAttributes, error) {
	oAttrs, err := g.Iterator.Next()
	// oAttrs object has only 'Name' or 'Prefix' field set.
	if err == iterator.Done {
		return ObjectAttributes{}, io.EOF
	}
	if err != nil {
		return ObjectAttributes{}, err
	}
	var attr ObjectAttributes
	if oAttrs.Prefix == "" {
		// object
		attr.Name = oAttrs.Name
		nameSplit := strings.Split(oAttrs.Name, "/")
		attr.ObjName = nameSplit[len(nameSplit)-1]
	} else {
		// directory
		attr.Name = oAttrs.Prefix
		attr.IsDir = true
	}
	return attr, nil
}

// gcsObjectIterator implements ObjectIterator via the gocloud/blob package
type openerObjectIterator struct {
	Iterator *blob.ListIterator
}

func (g openerObjectIterator) Next(ctx context.Context) (ObjectAttributes, error) {
	oAttrs, err := g.Iterator.Next(ctx)
	if err == io.EOF {
		return ObjectAttributes{}, io.EOF
	}
	if err != nil {
		return ObjectAttributes{}, err
	}
	attr := ObjectAttributes{
		Name:  oAttrs.Key,
		IsDir: oAttrs.IsDir,
	}
	if !oAttrs.IsDir {
		// object
		nameSplit := strings.Split(oAttrs.Key, "/")
		attr.ObjName = nameSplit[len(nameSplit)-1]
	}
	return attr, nil
}

package main

import (
	"context"
	"io"

	"cloud.google.com/go/storage"
	gzip "github.com/klauspost/pgzip"
	"google.golang.org/api/option"
)

type gcsReader struct{}

func (r *gcsReader) downloadAndDecompress(objectPath string) (io.Reader, error) {
	reader, err := download(objectPath)
	if err != nil {
		return nil, err
	}

	decompressed, err := decompress(reader)
	if err != nil {
		return nil, err
	}
	return decompressed, nil
}

func download(objectPath string) (io.Reader, error) {
	context := context.Background()
	client, err := storage.NewClient(context, option.WithoutAuthentication())
	if err != nil {
		return nil, err
	}

	bucket := client.Bucket(bucketName)

	remoteFile := bucket.Object(objectPath).ReadCompressed(true)
	reader, err := remoteFile.NewReader(context)
	if err != nil {
		return nil, err
	}

	return reader, err
}

func decompress(reader io.Reader) (io.Reader, error) {
	newReader, err := gzip.NewReader(reader)
	if err != nil {
		return nil, err
	}
	return newReader, nil
}

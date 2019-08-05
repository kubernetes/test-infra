package qiniu

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/qiniu/api.v7/auth"
	"github.com/qiniu/api.v7/client"
	"github.com/qiniu/api.v7/storage"
)

// ObjectHandle provides operations on an object in a qiniu cloud bucket
type ObjectHandle struct {
	key string

	cfg *Config

	bm *storage.BucketManager

	mac *auth.Credentials

	client *client.Client
}

type Config struct {
	Bucket    string `json:"bucket"`
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`

	// domain used to download files from qiniu cloud
	Domain string `json:"domain"`
}

func NewQiniuObject(cfg *Config, key string, bm *storage.BucketManager) *ObjectHandle {
	return &ObjectHandle{
		key:    key,
		cfg:    cfg,
		bm:     bm,
		mac:    auth.New(cfg.AccessKey, cfg.SecretKey),
		client: &client.Client{Client: http.DefaultClient},
	}
}

func (o *ObjectHandle) Attrs(ctx context.Context) (storage.FileInfo, error) {
	//TODO(CarlJi): need retry when errors
	return o.bm.Stat(o.cfg.Bucket, o.key)
}

// NewReader creates a reader to read the contents of the object.
// ErrObjectNotExist will be returned if the object is not found.
// The caller must call Close on the returned Reader when done reading.
func (o *ObjectHandle) NewReader(ctx context.Context) (io.ReadCloser, error) {
	return o.NewRangeReader(ctx, 0, -1)
}

// NewRangeReader reads parts of an object, reading at most length bytes starting
// from the given offset. If length is negative, the object is read until the end.
func (o *ObjectHandle) NewRangeReader(ctx context.Context, offset, length int64) (io.ReadCloser, error) {
	deadline := time.Now().Add(time.Second * 60 * 10).Unix()
	accessUrl := storage.MakePrivateURL(o.mac, o.cfg.Domain, o.key, deadline)

	//TODO(CarlJi): need do more enhancement works
	res, err := o.client.DoRequest(ctx, "GET", accessUrl, nil)
	if err != nil {
		return nil, err
	}

	return res.Body, nil
}

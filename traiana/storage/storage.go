package storage

import (
	"cloud.google.com/go/storage"
	"context"
	"k8s.io/test-infra/traiana"
	"k8s.io/test-infra/traiana/awsapi"
	"k8s.io/test-infra/traiana/storage/option"
)

// Client wrapper - AWS or GCS
type Client struct {
	gcs *storage.Client
	aws *awsapi.Client
}

func NewClient(ctx context.Context, opt ...option.ClientOption) (*Client, error) {
	if traiana.Aws {
		aws, err := awsapi.NewClient(option.GetAws(opt))

		return &Client{
			aws: aws,
		}, err
	} else {
		gcs, err := storage.NewClient(ctx, option.GetGcs(opt)...)

		return &Client{
			gcs: gcs,
		}, err
	}
}

type ObjectHandle struct {
	gcs *storage.ObjectHandle
	aws *awsapi.ObjectHandle
}

func (c *Client) Bucket(name string) *BucketHandle {
	if traiana.Aws {
		return &BucketHandle{
			aws: c.aws.Bucket(name),
		}
	} else {
		return &BucketHandle{
			gcs: c.gcs.Bucket(name),
		}
	}
}

type StorageWriter struct {
	gcs *storage.Writer
	aws *awsapi.Writer2Reader

	// You must call CopyFields() after setting the following fields:
	Metadata     map[string]string
	SendCRC32C   bool
	ProgressFunc func(int64)
	ObjectAttrs	 *ObjectAttrs
}

func (sw *StorageWriter) Write(p []byte) (n int, err error) {
	if traiana.Aws {
		return sw.aws.Write(p)
	} else {
		return sw.gcs.Write(p)
	}
}
func (sw *StorageWriter) Close() error {
	if traiana.Aws {
		return sw.aws.Close()
	} else {
		return sw.gcs.Close()

	}
}
func (sw *StorageWriter) CopyFields() {
	if !traiana.Aws {
		sw.gcs.Metadata = sw.Metadata
	}
}

func (o *ObjectHandle) NewWriter(ctx context.Context) *StorageWriter {
	if traiana.Aws {
		return &StorageWriter{
			aws: o.aws.NewWriter(ctx),
		}
	} else {
		return &StorageWriter{
			gcs: o.gcs.NewWriter(ctx),
		}
	}
}

type Reader struct {
	gcs      *storage.Reader
	//AbugovTODO
	aws      interface{} //*awsapi.Writer2Reader
}

func (*Reader) Read(p []byte) (n int, err error) {
	panic("AbugovTODO")
}

func (*Reader) Close() error {
	panic("AbugovTODO")
}

func (o *ObjectHandle) NewReader(ctx context.Context) (r *Reader, err error) {
	if traiana.Aws {
		r.aws = o.aws.NewReader(ctx)
	} else {
		r.gcs, err = o.gcs.NewReader(ctx)
	}

	return r, err
}

func (o *ObjectHandle) NewRangeReader(ctx context.Context, offset, length int64) (r *Reader, err error) {
	if traiana.Aws {
		o.aws = o.aws.NewRangeReader(ctx, offset, length) //AbugovTODO
	} else {
		r.gcs, err = o.gcs.NewRangeReader(ctx, offset, length)
	}

	return r, err
}

func (o *ObjectHandle) Attrs(ctx context.Context) (*ObjectAttrs, error) {
	panic("implement me")
}

type Query struct {
	// Delimiter returns results in a directory-like fashion.
	// Results will contain only objects whose names, aside from the
	// prefix, do not contain delimiter. Objects whose names,
	// aside from the prefix, contain delimiter will have their name,
	// truncated after the delimiter, returned in prefixes.
	// Duplicate prefixes are omitted.
	// Optional.
	Delimiter string

	// Prefix is the prefix filter to query objects
	// whose names begin with this prefix.
	// Optional.
	Prefix string

	// Versions indicates whether multiple versions of the same
	// object will be included in the results.
	Versions bool
}

//AbugovTODO
type ObjectAttrs struct {
	// Bucket is the name of the bucket containing this GCS object.
	// This field is read-only.
	//Bucket string

	// Name is the name of the object within the bucket.
	// This field is read-only.
	Name string

	// ContentType is the MIME type of the object's content.
	//ContentType string

	// ContentLanguage is the content language of the object's content.
	//ContentLanguage string

	// CacheControl is the Cache-Control header to be sent in the response
	// headers when serving the object data.
	//CacheControl string

	// ACL is the list of access control rules for the object.
	//ACL []ACLRule

	// Owner is the owner of the object. This field is read-only.
	//
	// If non-zero, it is in the form of "user-<userId>".
	//Owner string

	// Size is the length of the object's content. This field is read-only.
	Size int64

	// ContentEncoding is the encoding of the object's content.
	ContentEncoding string

	// ContentDisposition is the optional Content-Disposition header of the object
	// sent in the response headers.
	//ContentDisposition string

	// MD5 is the MD5 hash of the object's content. This field is read-only,
	// except when used from a Writer. If set on a Writer, the uploaded
	// data is rejected if its MD5 hash does not match this field.
	//MD5 []byte

	// CRC32C is the CRC32 checksum of the object's content using
	// the Castagnoli93 polynomial. This field is read-only, except when
	// used from a Writer. If set on a Writer and Writer.SendCRC32C
	// is true, the uploaded data is rejected if its CRC32c hash does not
	// match this field.
	CRC32C uint32

	// MediaLink is an URL to the object's content. This field is read-only.
	//MediaLink string

	// Metadata represents user-provided metadata, in key/value pairs.
	// It can be nil if no metadata is provided.
	//Metadata map[string]string

	// Generation is the generation number of the object's content.
	// This field is read-only.
	//Generation int64

	// Metageneration is the version of the metadata for this
	// object at this generation. This field is used for preconditions
	// and for detecting changes in metadata. A metageneration number
	// is only meaningful in the context of a particular generation
	// of a particular object. This field is read-only.
	//Metageneration int64

	// StorageClass is the storage class of the object.
	// This value defines how objects in the bucket are stored and
	// determines the SLA and the cost of storage. Typical values are
	// "MULTI_REGIONAL", "REGIONAL", "NEARLINE", "COLDLINE", "STANDARD"
	// and "DURABLE_REDUCED_AVAILABILITY".
	// It defaults to "STANDARD", which is equivalent to "MULTI_REGIONAL"
	// or "REGIONAL" depending on the bucket's location settings.
	//StorageClass string

	// Created is the time the object was created. This field is read-only.
	//Created time.Time

	// Deleted is the time the object was deleted.
	// If not deleted, it is the zero value. This field is read-only.
	//Deleted time.Time

	// Updated is the creation or modification time of the object.
	// For buckets with versioning enabled, changing an object's
	// metadata does not change this property. This field is read-only.
	//Updated time.Time

	// CustomerKeySHA256 is the base64-encoded SHA-256 hash of the
	// customer-supplied encryption key for the object. It is empty if there is
	// no customer-supplied encryption key.
	// See // https://cloud.google.com/storage/docs/encryption for more about
	// encryption in Google Cloud Storage.
	//CustomerKeySHA256 string

	// Cloud KMS key name, in the form
	// projects/P/locations/L/keyRings/R/cryptoKeys/K, used to encrypt this object,
	// if the object is encrypted by such a key.
	//
	// Providing both a KMSKeyName and a customer-supplied encryption key (via
	// ObjectHandle.Key) will result in an error when writing an object.
	//KMSKeyName string

	// Prefix is set only for ObjectAttrs which represent synthetic "directory
	// entries" when iterating over buckets using Query.Delimiter. See
	// ObjectIterator.Next. When set, no other fields in ObjectAttrs will be
	// populated.
	Prefix string
}
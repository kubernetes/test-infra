package awsapi

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"google.golang.org/api/iterator"
)

// S3ObjectIterator is an adapter between AWS SDK that takes a function and calls
// it multiple times in order to implement pagination, and prow that takes an
// iterator in order to list storage objects

type S3ObjectIterator struct {
	item     chan S3ObjectIteratorItem
	putError error
}

type S3ObjectIteratorItem struct {
	item    *s3.ListObjectsV2Output
	gotMore bool
}

func NewS3ObjectIterator(b *BucketHandle, q *Query) *S3ObjectIterator {
	it := &S3ObjectIterator{
		item:     make(chan S3ObjectIteratorItem),
	}

	fn := func() {
		err := S3ListObjects(b, q.Prefix, q.Delimiter, it.put)
		it.putError = err
		close(it.item)
	}

	go fn()

	return it
}

func (it *S3ObjectIterator) put(item *s3.ListObjectsV2Output, lastPage bool) bool {
	it.item <- S3ObjectIteratorItem{ item: item, gotMore: !lastPage }

	// always continue, this is a limitation of this adapter
	return true
}

func (it *S3ObjectIterator) take() (item *s3.ListObjectsV2Output, err error) {
	err = nil

	defer func() {
		// S3ListObjects exited with error without putting the last item
		recover()
		item = nil
		err = it.putError
	}()

	// read or block
	i, ok := <-it.item

	if ok {
		item = i.item

		if !i.gotMore {
			err = iterator.Done
		}
	} else {
		item = nil
		err = iterator.Done
	}

	return item, err
}

func (it *S3ObjectIterator) Next() (att *ObjectAttrs, err error) {
	item, err := it.take()

	return s3ObjectToObjectAttrs(item), err
}

func s3ObjectToObjectAttrs(o *s3.ListObjectsV2Output) *ObjectAttrs {
	if o == nil {
		return nil
	}

	return &ObjectAttrs {
		Name: aws.StringValue(o.Name),
		Prefix: aws.StringValue(o.Prefix),
	}
}
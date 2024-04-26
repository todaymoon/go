package datastore

import (
	"context"
	"fmt"
	"io"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"

	"github.com/stellar/go/support/errors"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/url"
)

// DataStore defines an interface for interacting with data storage
type DataStore interface {
	GetFile(ctx context.Context, path string) (io.ReadCloser, error)
	PutFile(ctx context.Context, path string, in io.WriterTo) error
	PutFileIfNotExists(ctx context.Context, path string, in io.WriterTo) (bool, error)
	Exists(ctx context.Context, path string) (bool, error)
	Size(ctx context.Context, path string) (int64, error)
	Close() error
	// TODO: Remove when binary search code is added
	//ListDirectoryNames(ctx context.Context) ([]string, error)
	//ListFileNames(ctx context.Context, path string) ([]string, error)
}

// NewDataStore creates a new DataStore based on the destination URL.
// Currently, only accepts GCS URLs.
func NewDataStore(ctx context.Context, destinationURL string) (DataStore, error) {
	parsed, err := url.Parse(destinationURL)
	if err != nil {
		return nil, err
	}

	pth := parsed.Path
	if parsed.Scheme != "gcs" {
		return nil, errors.Errorf("Invalid destination URL %s. Expected GCS URL ", destinationURL)
	}

	// Inside gcs, all paths start _without_ the leading /
	pth = strings.TrimPrefix(pth, "/")
	bucketName := parsed.Host
	prefix := pth

	log.Infof("creating GCS client for bucket: %s, prefix: %s", bucketName, prefix)

	var options []option.ClientOption
	client, err := storage.NewClient(ctx, options...)
	if err != nil {
		return nil, err
	}

	// Check the bucket exists
	bucket := client.Bucket(bucketName)
	if _, err := bucket.Attrs(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to retrieve bucket attributes")
	}

	return &GCSDataStore{client: client, bucket: bucket, prefix: prefix}, nil
}

// GetObjectKeyFromSequenceNumber generates the file name from the ledger sequence number.
func GetObjectKeyFromSequenceNumber(ledgerSeq uint32, ledgersPerFile uint32, filesPerPartition uint32, fileSuffix string) (string, error) {
	var objectKey string

	if ledgersPerFile < 1 {
		return "", errors.Errorf("Invalid ledgers per file (%d): must be at least 1", ledgersPerFile)
	}

	if filesPerPartition > 1 {
		partitionSize := ledgersPerFile * filesPerPartition
		partitionStart := (ledgerSeq / partitionSize) * partitionSize
		partitionEnd := partitionStart + partitionSize - 1
		objectKey = fmt.Sprintf("%d-%d/", partitionStart, partitionEnd)
	}

	fileStart := (ledgerSeq / ledgersPerFile) * ledgersPerFile
	fileEnd := fileStart + ledgersPerFile - 1
	objectKey += fmt.Sprintf("%d", fileStart)

	// Multiple ledgers per file
	if fileStart != fileEnd {
		objectKey += fmt.Sprintf("-%d", fileEnd)
	}
	objectKey += fileSuffix

	return objectKey, nil
}

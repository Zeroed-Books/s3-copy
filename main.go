package main

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

func main() {
	var appVersion, bucket, endpoint, region string

	flag.StringVar(&appVersion, "app-version", "", "Application version to tag files with.")
	flag.StringVar(&bucket, "bucket", "", "Bucket name")
	flag.StringVar(&endpoint, "endpoint", "", "AWS endpoint")
	flag.StringVar(&region, "region", "us-east-1", "AWS region")
	flag.Parse()

	key := os.Getenv("AWS_ACCESS_KEY_ID")
	secret := os.Getenv("AWS_SECRET_ACCESS_KEY")
	if key == "" || secret == "" {
		log.Fatal("Both 'AWS_ACCESS_KEY_ID' and 'AWS_SECRET_ACCESS_KEY' must be provided as environment variables.")
	}

	sessionConfig := &aws.Config{
		Credentials: credentials.NewStaticCredentials(key, secret, ""),
		Region:      aws.String(region),
	}

	if endpoint != "" {
		sessionConfig.Endpoint = aws.String(endpoint)
	}

	sess := session.Must(session.NewSession(sessionConfig))
	baseS3Uploader := s3manager.NewUploader(sess)

	s3Uploader := newS3Uploader(baseS3Uploader, bucket, "public-read")
	if appVersion != "" {
		s3Uploader.Tags["x-amz-meta-app-version"] = aws.String(appVersion)
	}

	if err := filepath.WalkDir("./", createUploadFunc(os.DirFS("./"), &s3Uploader)); err != nil {
		log.Fatal("Upload failed:", err)
	}
}

// uploadObject contains information about a file to upload.
type uploadObject struct {
	Path        string
	Body        io.Reader
	ContentType string
}

// An uploader allows for uploading a file to a remote location.
type uploader interface {
	// Upload stores the provided information in the remote location.
	Upload(*uploadObject) error
}

// s3Uploader implements file uploading to an S3-compatible storage backend.
type s3Uploader struct {
	// base is the client used to perform the uploads
	base *s3manager.Uploader
	// bucket is the storage bucket to upload files to
	bucket string
	// fileACL is the default ACL to apply to files.
	fileACL string

	Tags map[string]*string
}

func newS3Uploader(client *s3manager.Uploader, bucket, fileACL string) s3Uploader {
	return s3Uploader{
		base:    client,
		bucket:  bucket,
		fileACL: fileACL,
		Tags:    map[string]*string{},
	}
}

func (s *s3Uploader) Upload(object *uploadObject) error {
	_, err := s.base.Upload(&s3manager.UploadInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(object.Path),
		ACL:         aws.String(s.fileACL),
		Body:        object.Body,
		ContentType: aws.String(object.ContentType),
		Metadata:    s.Tags,
	})

	if err != nil {
		return fmt.Errorf("failed to upload to S3: %v", err)
	}

	return nil
}

// createUploadFunc creates a callback for `filepath.WalkDir` that uploads files from the given
// filesystem using a specific upload client.
func createUploadFunc(fsys fs.FS, client uploader) fs.WalkDirFunc {
	return func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("could not walk %s: %v", path, err)
		}

		// S3 does not have the concept of directories. Objects are stored under keys, which may
		// happen to look like directory-based file paths. Because of this, we don't have to handle
		// directories.
		if entry.IsDir() {
			log.Printf("Found directory: %s\n", path)
			return nil
		}

		extension := filepath.Ext(path)
		contentType := mime.TypeByExtension(extension)

		file, err := fsys.Open(path)
		if err != nil {
			return fmt.Errorf("could not open %s for reading: %v", path, err)
		}
		defer file.Close()

		err = client.Upload(&uploadObject{
			Path:        path,
			Body:        file,
			ContentType: contentType,
		})
		if err != nil {
			return fmt.Errorf("failed to upload %s: %v", path, err)
		}

		log.Printf("Uploaded %s\n", path)

		return nil
	}
}

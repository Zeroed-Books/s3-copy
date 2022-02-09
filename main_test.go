package main

import (
	"errors"
	"io"
	"io/fs"
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"
)

type mockFile struct {
	body io.Reader
	err  error
}

func (f *mockFile) Stat() (os.FileInfo, error) {
	return nil, errors.New("unimplemented")
}

func (f *mockFile) Read(target []byte) (int, error) {
	return f.body.Read(target)
}

func (f *mockFile) Close() error {
	return nil
}

type mockFileInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
}

func (i mockFileInfo) Name() string {
	return i.name
}

func (i mockFileInfo) IsDir() bool {
	return i.mode.IsDir()
}

func (i mockFileInfo) Type() fs.FileMode {
	return i.mode
}

func (i mockFileInfo) Info() (fs.FileInfo, error) {
	return i, nil
}

func (i mockFileInfo) Size() int64 {
	return i.size
}

func (i mockFileInfo) ModTime() time.Time {
	return i.modTime
}

func (i mockFileInfo) Mode() fs.FileMode {
	return i.mode
}

func (i mockFileInfo) Sys() interface{} {
	return nil
}

type mockFS struct {
	files map[string]mockFile
}

func (f *mockFS) Open(path string) (fs.File, error) {
	file, ok := f.files[path]
	if !ok {
		return nil, errors.New("file not found")
	}

	return &file, file.err
}

type mockUploader struct {
	uploadErr      error
	uploadedObject *uploadObject
}

func (u *mockUploader) Upload(object *uploadObject) error {
	if u.uploadErr != nil {
		return u.uploadErr
	}

	u.uploadedObject = object

	return nil
}

func Test_createUploadFunc(t *testing.T) {
	testCases := []struct {
		desc    string
		fsys    mockFS
		client  mockUploader
		path    string
		entry   fs.DirEntry
		walkErr error
		want    *uploadObject
		wantErr bool
	}{
		{
			desc:    "walk error no uploads",
			client:  mockUploader{},
			path:    "foo.txt",
			walkErr: errors.New("some error"),
			want:    nil,
			wantErr: true,
		},
		{
			desc:   "skip directory",
			client: mockUploader{},
			path:   "foo/bar",
			entry: mockFileInfo{
				name:    "foo/bar",
				size:    12,
				mode:    fs.ModeDir,
				modTime: time.Time{},
			},
			want:    nil,
			wantErr: false,
		},
		{
			desc: "error opening file",
			fsys: mockFS{
				files: map[string]mockFile{
					"foo.txt": {err: errors.New("can't be opened")},
				},
			},
			client: mockUploader{},
			path:   "foo.txt",
			entry: mockFileInfo{
				name:    "foo.txt",
				size:    12,
				mode:    0,
				modTime: time.Time{},
			},
			want:    nil,
			wantErr: true,
		},
		{
			desc: "upload error",
			fsys: mockFS{
				files: map[string]mockFile{
					"foo.txt": {body: strings.NewReader("some body")},
				},
			},
			client: mockUploader{
				uploadErr: errors.New("failed to upload"),
			},
			path: "foo.txt",
			entry: mockFileInfo{
				name:    "foo.txt",
				size:    12,
				mode:    0,
				modTime: time.Time{},
			},
			want:    nil,
			wantErr: true,
		},
		{
			desc: "successful upload",
			fsys: mockFS{
				files: map[string]mockFile{
					"foo.txt": {body: strings.NewReader("some body")},
				},
			},
			client: mockUploader{},
			path:   "foo.txt",
			entry: mockFileInfo{
				name:    "foo.txt",
				size:    12,
				mode:    0,
				modTime: time.Time{},
			},
			want: &uploadObject{
				Body:        strings.NewReader("some body"),
				Path:        "foo.txt",
				ContentType: "text/plain; charset=utf-8",
			},
		},
		{
			desc: "successful javascript upload",
			fsys: mockFS{
				files: map[string]mockFile{
					"app/index.js": {body: strings.NewReader("let foo = 'bar';")},
				},
			},
			client: mockUploader{},
			path:   "app/index.js",
			entry: mockFileInfo{
				name:    "app/index.js",
				size:    12,
				mode:    0,
				modTime: time.Time{},
			},
			want: &uploadObject{
				Body:        strings.NewReader("let foo = 'bar';"),
				Path:        "app/index.js",
				ContentType: "application/javascript",
			},
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			uploadFunc := createUploadFunc(&tC.fsys, &tC.client)

			err := uploadFunc(tC.path, tC.entry, tC.walkErr)
			if (err == nil) == tC.wantErr {
				t.Errorf("Expected error presence %v; got error %v", tC.wantErr, err)
			}

			if (tC.client.uploadedObject == nil) != (tC.want == nil) {
				t.Fatalf("Wanted uploaded object %v; got %v", tC.want, tC.client.uploadedObject)
			}

			if tC.want == nil {
				return
			}

			if tC.client.uploadedObject.Path != tC.want.Path {
				t.Fatalf("Expected upload to path %q; got %q", tC.want.Path, tC.client.uploadedObject.Path)
			}

			if tC.client.uploadedObject.ContentType != tC.want.ContentType {
				t.Fatalf("Expected content type %q; got %q", tC.want.ContentType, tC.client.uploadedObject.ContentType)
			}

			wantBody, err := ioutil.ReadAll(tC.want.Body)
			if err != nil {
				t.Fatalf("Could not read wanted body: %v", err)
			}

			gotBody, err := ioutil.ReadAll(tC.client.uploadedObject.Body)
			if err != nil {
				t.Fatalf("Could not read uploaded body: %v", err)
			}

			wantBodyStr := string(wantBody)
			gotBodyStr := string(gotBody)

			if string(wantBodyStr) != string(gotBodyStr) {
				t.Errorf("Expected body %q; got %q", wantBodyStr, gotBodyStr)
			}
		})
	}
}

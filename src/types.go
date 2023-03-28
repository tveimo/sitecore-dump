package main

import (
	"bytes"
	"io"
	"time"
)

type Field struct {
	Name  string `json:"Name"`
	Type  string `json:"Type"`
	Value string `json:"Value"`
}

type Item struct {
	Category     string           `json:"Category"`
	Database     string           `json:"Database"`
	DisplayName  string           `json:"DisplayName"`
	HasChildren  bool             `json:"HasChildren"`
	Icon         string           `json:"Icon"`
	ID           string           `json:"ID"`
	Language     string           `json:"Language"`
	LongID       string           `json:"LongID"`
	MediaUrl     string           `json:"MediaUrl"`
	Name         string           `json:"Name"`
	Path         string           `json:"Path"`
	Template     string           `json:"Template"`
	TemplateId   string           `json:"TemplateId"`
	TemplateName string           `json:"TemplateName"`
	Url          string           `json:"Url"`
	Version      int              `json:"Version"`
	Fields       map[string]Field `json:"Fields"`
	Children2    []*Item          `json:"Children"`
}

type SitecoreResults struct {
	StatusCode int `json:"statusCode"`
	Error      struct {
		Message string `json:"message"`
	} `json:"error"`
	Result struct {
		TotalCount  int    `json:"totalCount"`
		ResultCount int    `json:"resultCount"`
		Items       []Item `json:"items"`
	} `json:"result"`
}

type reusableReader struct {
	io.Reader
	readBuf *bytes.Buffer
	backBuf *bytes.Buffer
}

func ReusableReader(r io.Reader) io.Reader {
	readBuf := bytes.Buffer{}
	readBuf.ReadFrom(r) // error handling ignored for brevity
	backBuf := bytes.Buffer{}

	return reusableReader{
		io.TeeReader(&readBuf, &backBuf),
		&readBuf,
		&backBuf,
	}
}

func (r reusableReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		r.reset()
	}
	return n, err
}

func (r reusableReader) reset() {
	io.Copy(r.readBuf, r.backBuf) // nolint: errcheck
}

type AssetFolder struct {
	Name         string        `json:"name"`
	ID           string        `json:"id"`
	ParentID     string        `json:"parent_id""`
	ExternalID   string        `json:"external_id"`
	LastModified time.Time     `json:"last_modified"`
	Path         string        `json:"path"`
	Folders      []AssetFolder `json:"folders"`
}

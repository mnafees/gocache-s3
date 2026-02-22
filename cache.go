package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

type cache struct {
	client *s3.Client
	bucket string
	prefix string
	tmpDir string
}

func (c *cache) s3Key(actionID []byte) string {
	k := hex.EncodeToString(actionID)
	if c.prefix != "" {
		return c.prefix + "/" + k
	}
	return k
}

func (c *cache) diskPath(actionID []byte) string {
	return filepath.Join(c.tmpDir, hex.EncodeToString(actionID))
}

func (c *cache) get(ctx context.Context, req request) response {
	resp := response{ID: req.ID}

	out, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &c.bucket,
		Key:    aws.String(c.s3Key(req.ActionID)),
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			code := apiErr.ErrorCode()
			if code == "NoSuchKey" || code == "NotFound" {
				resp.Miss = true
				return resp
			}
		}
		resp.Err = err.Error()
		return resp
	}
	defer out.Body.Close()

	outputIDHex, ok := out.Metadata["outputid"]
	if !ok {
		resp.Miss = true
		return resp
	}

	outputID, err := hex.DecodeString(outputIDHex)
	if err != nil {
		resp.Err = fmt.Sprintf("decode output id metadata: %v", err)
		return resp
	}

	path := c.diskPath(req.ActionID)
	f, err := os.Create(path)
	if err != nil {
		resp.Err = err.Error()
		return resp
	}

	n, copyErr := io.Copy(f, out.Body)
	closeErr := f.Close()
	if copyErr != nil {
		resp.Err = copyErr.Error()
		return resp
	}
	if closeErr != nil {
		resp.Err = closeErr.Error()
		return resp
	}

	resp.OutputID = outputID
	resp.Size = n
	resp.DiskPath = path

	if ts, ok := out.Metadata["time"]; ok {
		if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			resp.Time = &t
		}
	}

	return resp
}

func (c *cache) put(ctx context.Context, req request) response {
	resp := response{ID: req.ID}
	now := time.Now()

	path := c.diskPath(req.ActionID)
	if err := os.WriteFile(path, req.body, 0o644); err != nil {
		resp.Err = err.Error()
		return resp
	}

	_, err := c.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &c.bucket,
		Key:    aws.String(c.s3Key(req.ActionID)),
		Body:   bytes.NewReader(req.body),
		Metadata: map[string]string{
			"outputid": hex.EncodeToString(req.OutputID),
			"time":     now.Format(time.RFC3339Nano),
		},
	})
	if err != nil {
		resp.Err = err.Error()
		return resp
	}

	resp.DiskPath = path
	return resp
}

func (c *cache) close() {
	os.RemoveAll(c.tmpDir)
}

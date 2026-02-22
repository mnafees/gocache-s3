package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type cmd string

const (
	cmdGet   = cmd("get")
	cmdPut   = cmd("put")
	cmdClose = cmd("close")
)

type request struct {
	ID       int64  `json:"ID"`
	Command  cmd    `json:"Command"`
	ActionID []byte `json:"ActionID,omitempty"`
	OutputID []byte `json:"OutputID,omitempty"`
	BodySize int64  `json:"BodySize,omitempty"`
	body     []byte
}

type response struct {
	ID            int64      `json:"ID"`
	Err           string     `json:"Err,omitempty"`
	KnownCommands []cmd     `json:"KnownCommands,omitempty"`
	Miss          bool       `json:"Miss,omitempty"`
	OutputID      []byte     `json:"OutputID,omitempty"`
	Size          int64      `json:"Size,omitempty"`
	Time          *time.Time `json:"Time,omitempty"`
	DiskPath      string     `json:"DiskPath,omitempty"`
}

func main() {
	log.SetPrefix("gocache-s3: ")
	log.SetFlags(0)

	bucket := flag.String("bucket", os.Getenv("GOCACHE_S3_BUCKET"), "S3 bucket name")
	prefix := flag.String("prefix", os.Getenv("GOCACHE_S3_PREFIX"), "S3 key prefix")
	pathStyle := flag.Bool("path-style", os.Getenv("GOCACHE_S3_PATH_STYLE") == "true" || os.Getenv("GOCACHE_S3_PATH_STYLE") == "1", "use path-style S3 addressing")
	flag.Parse()

	if *bucket == "" {
		log.Fatal("bucket is required: set -bucket flag or GOCACHE_S3_BUCKET env var")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("load AWS config: %v", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = *pathStyle
	})

	tmpDir, err := os.MkdirTemp("", "gocache-s3-*")
	if err != nil {
		log.Fatalf("create temp dir: %v", err)
	}

	c := &cache{
		client: client,
		bucket: *bucket,
		prefix: *prefix,
		tmpDir: tmpDir,
	}
	defer c.close()

	enc := json.NewEncoder(os.Stdout)
	var mu sync.Mutex
	send := func(r response) {
		mu.Lock()
		defer mu.Unlock()
		enc.Encode(r)
	}

	send(response{
		ID:            0,
		KnownCommands: []cmd{cmdGet, cmdPut, cmdClose},
	})

	var wg sync.WaitGroup
	reader := bufio.NewReaderSize(os.Stdin, 1<<20)

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("read stdin: %v", err)
		}

		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			log.Fatalf("unmarshal request: %v", err)
		}

		if req.BodySize > 0 {
			bodyLine, err := reader.ReadBytes('\n')
			if err != nil {
				log.Fatalf("read body: %v", err)
			}
			if err := json.Unmarshal(bodyLine, &req.body); err != nil {
				log.Fatalf("unmarshal body: %v", err)
			}
		}

		switch req.Command {
		case cmdGet:
			wg.Add(1)
			go func() {
				defer wg.Done()
				send(c.get(ctx, req))
			}()
		case cmdPut:
			wg.Add(1)
			go func() {
				defer wg.Done()
				send(c.put(ctx, req))
			}()
		case cmdClose:
			wg.Wait()
			send(response{ID: req.ID})
			return
		}
	}

	wg.Wait()
}

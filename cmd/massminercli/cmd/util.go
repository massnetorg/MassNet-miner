package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/massnetorg/mass-core/logging"
)

// server is the interface for http/rpc server.
type server interface {
	call(ctx context.Context, u *url.URL, method Method, bodyReader io.Reader) (io.ReadCloser, error)
}

type Method uint32

var methodRef = map[Method]string{GET: "GET", POST: "POST", PUT: "PUT", DELETE: "DELETE"}

func (m Method) String() string {
	if method, exists := methodRef[m]; exists {
		return method
	}
	return "INVALID"
}

const (
	// GET represents GET method in http
	GET Method = iota
	// POST represents POST method in http
	POST
	// PUT represents PUT method in http
	PUT
	// DELETE represents DELETE method in http
	DELETE
)

// httpServer is the http implement for server
type httpServer struct{}

// newHttpServer provides a new httpServer
func newHttpServer() *httpServer {
	return &httpServer{}
}

func (hs *httpServer) call(ctx context.Context, u *url.URL, method Method, bodyReader io.Reader) (io.ReadCloser, error) {
	req, err := http.NewRequest(method.String(), u.String(), bodyReader)
	if err != nil {
		return nil, err
	}

	cli := http.DefaultClient

	resp, err := cli.Do(req.WithContext(ctx))
	if err != nil && ctx.Err() != nil { // check if it timed out
		return nil, ctx.Err()
	} else if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logging.CPrint(logging.ERROR, "fail on request with code", logging.LogFormat{"code": resp.StatusCode})
	}

	return resp.Body, nil
}

// Client is the top level for http/rpc server
type Client struct {
	url string
	s   server
}

// HTTPClient creates a client based on http
func HTTPClient() *Client {
	return &Client{url: config.APIURL, s: newHttpServer()}
}

// CallRaw calls a remote node, specified by the path.
// It returns the raw response body
func (c *Client) CallRaw(ctx context.Context, path string, method Method, request interface{}) (io.ReadCloser, error) {
	u, err := url.Parse(c.url)
	if err != nil {
		return nil, err
	}
	u.Path = path

	var bodyReader io.Reader
	if request != nil {
		var jsonBody bytes.Buffer
		m := jsonpb.Marshaler{EmitDefaults: false}
		m.Marshal(&jsonBody, request.(proto.Message))
		bodyReader = &jsonBody
	}

	return c.s.call(ctx, u, method, bodyReader)
}

// Call calls a remote node, specified by the path.
func (c *Client) Call(ctx context.Context, path string, method Method, request, response interface{}) error {
	r, err := c.CallRaw(ctx, path, method, request)
	if err != nil {
		return err
	}
	defer r.Close()

	u := jsonpb.Unmarshaler{AllowUnknownFields: true}
	err = u.Unmarshal(r, response.(proto.Message))

	return err
}

// ClientCall selects a client type and execute calling
func ClientCall(path string, method Method, request, response interface{}) {
	client := HTTPClient()
	if err := client.Call(context.Background(), path, method, request, response); err != nil {
		logging.CPrint(logging.FATAL, "fail on client call", logging.LogFormat{"err": err})
	}
}

func printJSON(data interface{}) {
	m := jsonpb.Marshaler{EmitDefaults: true, Indent: "  "}

	str, err := m.MarshalToString(data.(proto.Message))
	if err != nil {
		logging.CPrint(logging.FATAL, "fail to marshal json", logging.LogFormat{"err": err, "data_type": reflect.TypeOf(data)})
	}

	fmt.Println(str)
}

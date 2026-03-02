package spotify

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
)

// DebugTransport is an http.RoundTripper that logs request/response details to Output.
// Inject it via context when creating a Client:
//
//	ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{
//	    Transport: &spotify.DebugTransport{Output: logger.DebugWriter(zlog.Logger)},
//	})
//	client, _ := spotify.New(ctx, cfg)
type DebugTransport struct {
	Base   http.RoundTripper // nil means http.DefaultTransport
	Output io.Writer
}

func (t *DebugTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	reqDump, _ := httputil.DumpRequestOut(req, true)
	fmt.Fprintf(t.Output, "→ %s %s\n%s\n", req.Method, req.URL, reqDump)

	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}

	resp, err := base.RoundTrip(req)
	if err != nil {
		fmt.Fprintf(t.Output, "← error: %v\n", err)
		return nil, err
	}

	respDump, _ := httputil.DumpResponse(resp, true)
	fmt.Fprintf(t.Output, "← %d\n%s\n", resp.StatusCode, respDump)

	return resp, nil
}

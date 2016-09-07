package langp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"sourcegraph.com/sourcegraph/sourcegraph/pkg/conf/feature"
	"sourcegraph.com/sourcegraph/sourcegraph/pkg/inventory/filelang"

	opentracing "github.com/opentracing/opentracing-go"
)

// Prefix for environment variables referring to language processor configuration
const envLanguageProcessorPrefix = "SG_LANGUAGE_PROCESSOR_"

// Maps real language name to canonical one, that can be used in environment variable names.
// For example C++ => CPP
var languageNameMap = map[string]string{
	"C++":         "CPP",
	"Objective-C": "OBJECTIVEC",
}

// DefaultClient is the default language processor client.
var DefaultClient *Client

func init() {
	if !feature.Features.Universe {
		return
	}

	endpoints := make(map[string]string)
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		lang := lpEnvLanguage(parts[0])
		if lang != "" {
			endpoints[lang] = parts[1]
		}
	}
	var err error
	DefaultClient, err = NewClient(endpoints)
	if err != nil {
		log.Fatal(err)
	}
}

// lpEnvLanguage tries to extract language name from environment variable name
// which is supposed to be in form PREFIX_LANG
func lpEnvLanguage(key string) string {
	if !strings.HasPrefix(key, envLanguageProcessorPrefix) {
		return ""
	}
	return key[len(envLanguageProcessorPrefix):]
}

// langClient is an HTTP endpoint and client for one single language.
type langClient struct {
	// endpoint is the HTTP endpoint of the Language Processor.
	endpoint *url.URL

	// client is used for making HTTP requests.
	client *http.Client
}

// endpointTo returns a URL based on c.Endpoint with the given path suffixed.
func (c *langClient) endpointTo(p string) string {
	cpy := *c.endpoint
	cpy.Path = path.Join(cpy.Path, p)
	return cpy.String()
}

// Client represents multiple Language Processor REST API clients (i.e. for
// multiple languages) which is safe for use by multiple goroutines
// concurrently.
//
// It is responsible for invoking the proper LP (or combining results from
// multiple LPs) depending on the request / which langauge the source file is.
type Client struct {
	// clients is a map of languages to their respective clients.
	clients map[string]*langClient
}

// Prepare informs the Language Processor that it should prepare a workspace
// for the specified repo / commit. It is sent prior to an actual user request
// (e.g. as soon as we have access to their repos) in hopes of having
// preparation completed already when a user makes their first request.
func (c *Client) Prepare(ctx context.Context, r *RepoRev) error {
	// Ask each LP to prepare the workspace.
	for _, lc := range c.clients {
		if err := c.do(ctx, lc, "prepare", r, nil); err != nil {
			return err
		}
	}
	return nil
}

// DefSpecToPosition returns the position of the given DefSpec.
func (c *Client) DefSpecToPosition(ctx context.Context, k *DefSpec) (*Position, error) {
	cl, err := c.clientForUnitType(k.UnitType)
	if err != nil {
		return nil, err
	}
	var result Position
	err = c.do(ctx, cl, "defspec-to-position", k, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// Definition resolves the specified position, effectively returning where the
// given definition is defined. For example, this is used for go to definition.
func (c *Client) Definition(ctx context.Context, p *Position) (*Range, error) {
	cl, err := c.clientForFile(p.File)
	if err != nil {
		return nil, err
	}
	var result Range
	err = c.do(ctx, cl, "definition", p, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// Hover returns hover-over information about the def/ref/etc at the given
// position.
func (c *Client) Hover(ctx context.Context, p *Position) (*Hover, error) {
	cl, err := c.clientForFile(p.File)
	if err != nil {
		return nil, err
	}
	var result Hover
	err = c.do(ctx, cl, "hover", p, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// LocalRefs resolves references to repository-local definitions.
func (c *Client) LocalRefs(ctx context.Context, p *Position) (*RefLocations, error) {
	cl, err := c.clientForFile(p.File)
	if err != nil {
		return nil, err
	}
	var result RefLocations
	err = c.do(ctx, cl, "local-refs", p, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// DefSpecRefs resolves references to repository definitions.
func (c *Client) DefSpecRefs(ctx context.Context, k *DefSpec) (*RefLocations, error) {
	var result RefLocations
	for _, cl := range c.clients {
		var v RefLocations
		err := c.do(ctx, cl, "defspec-refs", k, &v)
		if err != nil {
			return nil, err
		}
		result.Refs = append(result.Refs, r.Refs...)
	}
	return &result, nil
}

// ExternalRefs resolves references to repository-external definitions.
func (c *Client) ExternalRefs(ctx context.Context, r *RepoRev) (*ExternalRefs, error) {
	var result ExternalRefs
	for _, cl := range c.clients {
		var v ExternalRefs
		err := c.do(ctx, cl, "external-refs", r, &v)
		if err != nil {
			return nil, err
		}
		result.Defs = append(result.Defs, v.Defs...)
	}
	return &result, nil
}

// ExportedSymbols lists repository-local definitions which are exported.
func (c *Client) ExportedSymbols(ctx context.Context, r *RepoRev) (*ExportedSymbols, error) {
	var result ExportedSymbols
	for _, cl := range c.clients {
		var v ExportedSymbols
		err := c.do(ctx, cl, "exported-symbols", r, &v)
		if err != nil {
			return nil, err
		}
		result.Symbols = append(result.Symbols, v.Symbols...)
	}
	return &result, nil
}

// clientForFile finds the client related to the file extension for filename.
func (c *Client) clientForFile(filename string) (*langClient, error) {
	candidates := filelang.Langs.ByFilename(filename)
	for _, candidate := range candidates {
		normalized, ok := languageNameMap[candidate.Name]
		if !ok {
			normalized = candidate.Name
		}
		normalized = strings.ToUpper(normalized)
		client, ok := c.clients[normalized]
		if ok {
			return client, nil
		}
	}
	return nil, fmt.Errorf("langp.Client: no client registered for extension %q (did you set SG_LANGUAGE_PROCESSOR_<lang> ?)", filepath.Ext(filename))
}

// clientForUnitType finds the client related to the unit type.
//
// TODO(slimsag): language-specific, find a generic way.
func (c *Client) clientForUnitType(unitType string) (*langClient, error) {
	var lang string
	switch unitType {
	case "GoPackage":
		lang = "GO"
	case "JavaArtifact":
		lang = "JAVA"
	case "JSModule":
		lang = "JAVASCRIPT"
	}
	client, ok := c.clients[lang]
	if !ok {
		return nil, fmt.Errorf("langp.Client: no client registered for defkey %q (did you set SG_LANGUAGE_PROCESSOR_<lang> ?)", unitType)
	}
	return client, nil
}

func (c *Client) do(ctx context.Context, cl *langClient, endpoint string, body, results interface{}) error {
	// TODO: maybe consider retrying upon first request failure to prevent
	// such errors from ending up on the frontend for reliability purposes.
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", cl.endpointTo(endpoint), bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("%s (body '%s')", err, string(data))
	}

	req.Header.Add("Content-Type", "application/json")

	operationName := fmt.Sprintf("LP Client: POST %s", cl.endpointTo(endpoint))
	var span opentracing.Span
	span, ctx = opentracing.StartSpanFromContext(ctx, operationName)
	span.LogEventWithPayload("request body", body)
	defer span.Finish()

	if err := opentracing.GlobalTracer().Inject(span.Context(), opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(req.Header)); err != nil {
		return fmt.Errorf("%s (body '%s')", err, string(data))
	}

	resp, err := cl.client.Do(req)
	if err != nil {
		return fmt.Errorf("%s (body '%s')", err, string(data))
	}
	defer resp.Body.Close()

	// 1 KB is a good, safe choice for medium-to-high throughput traces.
	saver := &prefixSuffixSaver{N: 1 * 1024}
	tee := io.TeeReader(resp.Body, saver)
	defer func() {
		span.LogEventWithPayload("response - "+resp.Status, string(saver.Bytes()))
	}()

	if resp.StatusCode != http.StatusOK {
		var errResp Error
		if err := json.NewDecoder(tee).Decode(&errResp); err != nil {
			return fmt.Errorf("error parsing language processor error (status code %v): %v", resp.StatusCode, err)
		}
		return &errResp
	}
	if results == nil {
		return nil
	}
	return json.NewDecoder(tee).Decode(results)
}

// NewClient returns a new client with the default options connecting the given
// languages to their respective Language Processor endpoint.
//
// An error is returned only if parsing one of the endpoint URLs fails.
func NewClient(endpoints map[string]string) (*Client, error) {
	c := &Client{
		clients: make(map[string]*langClient),
	}
	for lang, endpoint := range endpoints {
		u, err := url.Parse(endpoint)
		if err != nil {
			return nil, err
		}
		if u.Scheme == "" {
			return nil, fmt.Errorf("must specify endpoint scheme")
		}
		if u.Host == "" {
			return nil, fmt.Errorf("must specify endpoint host")
		}
		c.clients[lang] = &langClient{
			endpoint: u,
			client: &http.Client{
				// TODO(slimsag): Once we have proper async operations we should
				// lower this timeout to respect those numbers. Until then, some
				// operations (listing all refs, cloning workspaces, etc) can take
				// quite a while and we don't want to abort the request.
				Timeout: 60 * time.Second,
			},
		}
	}
	return c, nil
}

package sui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type SleepFunc func(context.Context, time.Duration) error

type RetryConfig struct {
	Retries   int
	BaseDelay time.Duration
	Jitter    time.Duration
	Sleep     SleepFunc
	OnRetry   func(reason string, attempt int, delay time.Duration)
}

type GraphQLClient struct {
	Endpoint      string
	HTTPClient    HTTPDoer
	Retry         RetryConfig
	AllowInsecure bool
}

type GraphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type EventsQuery struct {
	PackageID        string
	ModuleName       string
	EventType        string
	AfterCheckpoint  *uint64
	BeforeCheckpoint *uint64
	After            string
	First            int
}

type PackageModulesQuery struct {
	PackageID string
	After     string
	First     int
}

type ObjectsQuery struct {
	Type  string
	After string
	First int
}

type EventsPage struct {
	Nodes       []MoveEventNode
	HasNextPage bool
	EndCursor   string
}

type ObjectsPage struct {
	Nodes       []MoveObjectNode
	HasNextPage bool
	EndCursor   string
}

type PackageModulesPage struct {
	Modules     []string
	HasNextPage bool
	EndCursor   string
}

type MoveEventNode struct {
	Contents          *MoveEventContents     `json:"contents,omitempty"`
	Sender            *AddressNode           `json:"sender,omitempty"`
	SequenceNumber    int64                  `json:"sequenceNumber"`
	Timestamp         string                 `json:"timestamp,omitempty"`
	Transaction       *TransactionNode       `json:"transaction,omitempty"`
	TransactionModule *TransactionModuleNode `json:"transactionModule,omitempty"`
}

type MoveEventContents struct {
	JSON any       `json:"json,omitempty"`
	Type *TypeNode `json:"type,omitempty"`
}

type MoveObjectNode struct {
	Address      string          `json:"address,omitempty"`
	Digest       string          `json:"digest,omitempty"`
	Version      GraphQLScalar   `json:"version,omitempty"`
	AsMoveObject *MoveObjectData `json:"asMoveObject,omitempty"`
}

type MoveObjectData struct {
	Contents *MoveEventContents `json:"contents,omitempty"`
}

type GraphQLScalar string

func (s *GraphQLScalar) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*s = ""
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err == nil {
		*s = GraphQLScalar(value)
		return nil
	}
	*s = GraphQLScalar(string(data))
	return nil
}

func (s GraphQLScalar) String() string {
	return string(s)
}

type TypeNode struct {
	Repr string `json:"repr,omitempty"`
}

type AddressNode struct {
	Address string `json:"address,omitempty"`
}

type TransactionNode struct {
	Digest string `json:"digest,omitempty"`
}

type TransactionModuleNode struct {
	Name    string       `json:"name,omitempty"`
	Package *AddressNode `json:"package,omitempty"`
}

func BuildEventsRequest(query EventsQuery) (GraphQLRequest, error) {
	first := query.First
	if first <= 0 {
		first = 50
	}
	variables := map[string]any{
		"first": first,
	}
	var definitions []string
	var filters []string
	if query.EventType != "" {
		if _, err := parseMoveType(query.EventType); err != nil {
			return GraphQLRequest{}, err
		}
		definitions = append(definitions, "$type: String!")
		filters = append(filters, "type: $type")
		variables["type"] = query.EventType
	} else {
		if !isSuiID(query.PackageID) {
			return GraphQLRequest{}, fmt.Errorf("malformed package ID %s", query.PackageID)
		}
		module := query.PackageID
		if query.ModuleName != "" {
			if !isMoveIdentifier(query.ModuleName) {
				return GraphQLRequest{}, fmt.Errorf("invalid Move module %s", query.ModuleName)
			}
			module += "::" + query.ModuleName
		}
		definitions = append(definitions, "$module: String!")
		filters = append(filters, "module: $module")
		variables["module"] = module
	}
	if query.AfterCheckpoint != nil {
		definitions = append(definitions, "$afterCheckpoint: UInt53")
		filters = append(filters, "afterCheckpoint: $afterCheckpoint")
		variables["afterCheckpoint"] = *query.AfterCheckpoint
	}
	if query.BeforeCheckpoint != nil {
		definitions = append(definitions, "$beforeCheckpoint: UInt53")
		filters = append(filters, "beforeCheckpoint: $beforeCheckpoint")
		variables["beforeCheckpoint"] = *query.BeforeCheckpoint
	}
	definitions = append(definitions, "$first: Int", "$after: String")
	if query.After != "" {
		variables["after"] = query.After
	}
	return GraphQLRequest{
		Query: fmt.Sprintf(`query Events(%s) {
  events(filter: { %s }, first: $first, after: $after) {
    pageInfo { hasNextPage endCursor }
    nodes {
      sequenceNumber
      timestamp
      sender { address }
      transaction { digest }
      transactionModule { name package { address } }
      contents { type { repr } json }
    }
  }
}`, strings.Join(definitions, ", "), strings.Join(filters, ", ")),
		Variables: variables,
	}, nil
}

func BuildPackageModulesRequest(query PackageModulesQuery) (GraphQLRequest, error) {
	if !isSuiID(query.PackageID) {
		return GraphQLRequest{}, fmt.Errorf("malformed package ID %s", query.PackageID)
	}
	first := query.First
	if first <= 0 {
		first = 50
	}
	variables := map[string]any{
		"address": query.PackageID,
		"first":   first,
	}
	if query.After != "" {
		variables["after"] = query.After
	}
	return GraphQLRequest{
		Query: `query PackageModules($address: SuiAddress!, $first: Int, $after: String) {
  package(address: $address) {
    modules(first: $first, after: $after) {
      pageInfo { hasNextPage endCursor }
      nodes { name }
    }
  }
}`,
		Variables: variables,
	}, nil
}

func BuildObjectsRequest(query ObjectsQuery) (GraphQLRequest, error) {
	if _, err := parseMoveType(query.Type); err != nil {
		return GraphQLRequest{}, err
	}
	first := query.First
	if first <= 0 {
		first = 50
	}
	variables := map[string]any{
		"first": first,
		"type":  query.Type,
	}
	if query.After != "" {
		variables["after"] = query.After
	}
	return GraphQLRequest{
		Query: `query ObjectsByType($type: String!, $first: Int, $after: String) {
  objects(filter: { type: $type }, first: $first, after: $after) {
    pageInfo { hasNextPage endCursor }
    nodes {
      address
      digest
      version
      asMoveObject {
        contents { type { repr } json }
      }
    }
  }
}`,
		Variables: variables,
	}, nil
}

func (c GraphQLClient) FetchEvents(ctx context.Context, query EventsQuery) (EventsPage, error) {
	request, err := BuildEventsRequest(query)
	if err != nil {
		return EventsPage{}, err
	}
	var payload struct {
		Events struct {
			Nodes    []MoveEventNode `json:"nodes"`
			PageInfo pageInfo        `json:"pageInfo"`
		} `json:"events"`
	}
	if err := c.post(ctx, request, &payload); err != nil {
		return EventsPage{}, err
	}
	return EventsPage{
		Nodes:       payload.Events.Nodes,
		HasNextPage: payload.Events.PageInfo.HasNextPage,
		EndCursor:   payload.Events.PageInfo.EndCursor,
	}, nil
}

func (c GraphQLClient) FetchObjects(ctx context.Context, query ObjectsQuery) (ObjectsPage, error) {
	request, err := BuildObjectsRequest(query)
	if err != nil {
		return ObjectsPage{}, err
	}
	var payload struct {
		Objects struct {
			Nodes    []MoveObjectNode `json:"nodes"`
			PageInfo pageInfo         `json:"pageInfo"`
		} `json:"objects"`
	}
	if err := c.post(ctx, request, &payload); err != nil {
		return ObjectsPage{}, err
	}
	return ObjectsPage{
		Nodes:       payload.Objects.Nodes,
		HasNextPage: payload.Objects.PageInfo.HasNextPage,
		EndCursor:   payload.Objects.PageInfo.EndCursor,
	}, nil
}

func (c GraphQLClient) FetchPackageModules(ctx context.Context, query PackageModulesQuery) (PackageModulesPage, error) {
	request, err := BuildPackageModulesRequest(query)
	if err != nil {
		return PackageModulesPage{}, err
	}
	var payload struct {
		Package *struct {
			Modules struct {
				Nodes []struct {
					Name string `json:"name"`
				} `json:"nodes"`
				PageInfo pageInfo `json:"pageInfo"`
			} `json:"modules"`
		} `json:"package"`
	}
	if err := c.post(ctx, request, &payload); err != nil {
		return PackageModulesPage{}, err
	}
	if payload.Package == nil {
		return PackageModulesPage{}, fmt.Errorf("sui GraphQL did not return package %s", query.PackageID)
	}
	modules := make([]string, 0, len(payload.Package.Modules.Nodes))
	for _, node := range payload.Package.Modules.Nodes {
		if node.Name != "" {
			modules = append(modules, node.Name)
		}
	}
	return PackageModulesPage{
		Modules:     modules,
		HasNextPage: payload.Package.Modules.PageInfo.HasNextPage,
		EndCursor:   payload.Package.Modules.PageInfo.EndCursor,
	}, nil
}

func (c GraphQLClient) post(ctx context.Context, request GraphQLRequest, target any) error {
	endpoint, err := url.Parse(c.Endpoint)
	if err != nil {
		return err
	}
	if endpoint.Scheme != "https" && !(c.AllowInsecure && endpoint.Scheme == "http") {
		return fmt.Errorf("sui GraphQL endpoint must use HTTPS: %s", c.Endpoint)
	}
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	retries := c.Retry.Retries
	if retries < 0 {
		retries = 0
	}
	sleep := c.Retry.Sleep
	if sleep == nil {
		sleep = sleepContext
	}
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			lastErr = err
			if attempt < retries {
				delay := retryDelay(c.Retry, attempt, 0)
				c.Retry.OnRetryMaybe(fmt.Sprintf("network error: %v", err), attempt+1, delay)
				if err := sleep(ctx, delay); err != nil {
					return err
				}
				continue
			}
			break
		}
		err = decodeGraphQLResponse(res, target)
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt < retries && isRetryableError(err) {
			delay := retryDelay(c.Retry, attempt, retryAfter(res.Header))
			c.Retry.OnRetryMaybe(err.Error(), attempt+1, delay)
			if err := sleep(ctx, delay); err != nil {
				return err
			}
			continue
		}
		break
	}
	return lastErr
}

func decodeGraphQLResponse(res *http.Response, target any) error {
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500 {
		return retryableHTTPError{status: res.StatusCode}
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return fmt.Errorf("sui GraphQL request failed with HTTP %d", res.StatusCode)
	}
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return err
	}
	if len(envelope.Errors) > 0 {
		messages := make([]string, 0, len(envelope.Errors))
		retryable := false
		for _, item := range envelope.Errors {
			messages = append(messages, item.Message)
			if isRetryableGraphQLMessage(item.Message) {
				retryable = true
			}
		}
		err := fmt.Errorf("sui GraphQL returned errors: %s", strings.Join(messages, "; "))
		if retryable {
			return retryableGraphQLError{err: err}
		}
		return err
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return errors.New("sui GraphQL response did not include data")
	}
	return json.Unmarshal(envelope.Data, target)
}

type pageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

type retryableHTTPError struct {
	status int
}

func (e retryableHTTPError) Error() string {
	return fmt.Sprintf("sui GraphQL request failed with HTTP %d", e.status)
}

type retryableGraphQLError struct {
	err error
}

func (e retryableGraphQLError) Error() string {
	return e.err.Error()
}

func isRetryableError(err error) bool {
	var httpErr retryableHTTPError
	var graphqlErr retryableGraphQLError
	return errors.As(err, &httpErr) || errors.As(err, &graphqlErr)
}

func isRetryableGraphQLMessage(message string) bool {
	normalised := strings.ToLower(message)
	return strings.Contains(normalised, "failed to load transaction events") ||
		strings.Contains(normalised, "timed out") ||
		strings.Contains(normalised, "timeout") ||
		strings.Contains(normalised, "temporarily unavailable") ||
		strings.Contains(normalised, "resource_exhausted")
}

func retryAfter(headers http.Header) time.Duration {
	value := headers.Get("Retry-After")
	if value == "" {
		return 0
	}
	if seconds, err := time.ParseDuration(value + "s"); err == nil {
		return seconds
	}
	if when, err := http.ParseTime(value); err == nil {
		return time.Until(when)
	}
	return 0
}

func retryDelay(config RetryConfig, attempt int, retryAfterDelay time.Duration) time.Duration {
	if retryAfterDelay > 0 {
		return retryAfterDelay
	}
	base := config.BaseDelay
	if base <= 0 {
		base = 750 * time.Millisecond
	}
	delay := base * time.Duration(1<<attempt)
	if config.Jitter > 0 {
		delay += time.Duration(rand.Int63n(int64(config.Jitter)))
	}
	return delay
}

func (config RetryConfig) OnRetryMaybe(reason string, attempt int, delay time.Duration) {
	if config.OnRetry != nil {
		config.OnRetry(reason, attempt, delay)
	}
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isMoveIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		if i == 0 {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '_' {
				continue
			}
			return false
		}
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return true
}

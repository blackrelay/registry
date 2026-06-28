package sui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const testPackageID = "0x28b497559d65ab320d9da4613bf2498d5946b2c0ae3597ccfda3072ce127448c"

func TestBuildEventsRequestUsesPackageModuleFilter(t *testing.T) {
	request, err := BuildEventsRequest(EventsQuery{PackageID: testPackageID, ModuleName: "character", After: "cursor", First: 25})
	if err != nil {
		t.Fatal(err)
	}
	if request.Variables["module"] != testPackageID+"::character" {
		t.Fatalf("unexpected module variable %#v", request.Variables["module"])
	}
	if request.Variables["after"] != "cursor" {
		t.Fatalf("unexpected after variable %#v", request.Variables["after"])
	}
	if request.Variables["first"] != 25 {
		t.Fatalf("unexpected first variable %#v", request.Variables["first"])
	}
}

func TestBuildEventsRequestUsesTypeAndCheckpointRangeFilter(t *testing.T) {
	eventType := testPackageID + "::fuel::FuelEvent"
	afterCheckpoint := uint64(99)
	beforeCheckpoint := uint64(201)
	request, err := BuildEventsRequest(EventsQuery{
		EventType:        eventType,
		AfterCheckpoint:  &afterCheckpoint,
		BeforeCheckpoint: &beforeCheckpoint,
		After:            "cursor",
		First:            50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if request.Variables["type"] != eventType {
		t.Fatalf("unexpected type variable %#v", request.Variables["type"])
	}
	if request.Variables["afterCheckpoint"] != afterCheckpoint {
		t.Fatalf("unexpected afterCheckpoint variable %#v", request.Variables["afterCheckpoint"])
	}
	if request.Variables["beforeCheckpoint"] != beforeCheckpoint {
		t.Fatalf("unexpected beforeCheckpoint variable %#v", request.Variables["beforeCheckpoint"])
	}
	if _, ok := request.Variables["module"]; ok {
		t.Fatalf("type-filtered request should not require a module variable: %#v", request.Variables)
	}
}

func TestBuildObjectsRequestUsesTypeFilter(t *testing.T) {
	objectType := testPackageID + "::character::PlayerProfile"
	request, err := BuildObjectsRequest(ObjectsQuery{Type: objectType, After: "cursor", First: 25})
	if err != nil {
		t.Fatal(err)
	}
	if request.Variables["type"] != objectType {
		t.Fatalf("unexpected type variable %#v", request.Variables["type"])
	}
	if request.Variables["after"] != "cursor" {
		t.Fatalf("unexpected after variable %#v", request.Variables["after"])
	}
	if request.Variables["first"] != 25 {
		t.Fatalf("unexpected first variable %#v", request.Variables["first"])
	}
}

func TestFetchEventsRetriesHTTPFailures(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`temporarily busy`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "data": {
		    "events": {
		      "pageInfo": {"hasNextPage": false, "endCursor": "cursor-1"},
		      "nodes": [
		        {
		          "sequenceNumber": 2,
		          "timestamp": "2026-05-27T18:12:12.879Z",
		          "transaction": {"digest": "7YCpwNPBFJrd6APq6ModDosVUcCmSqsCSB9X76zzm3ch"},
		          "transactionModule": {"name": "character", "package": {"address": "` + testPackageID + `"}},
		          "contents": {
		            "type": {"repr": "` + testPackageID + `::character::CharacterCreatedEvent"},
		            "json": {"key": {"item_id": "2112091476", "tenant": "stillness"}}
		          }
		        }
		      ]
		    }
		  }
		}`))
	}))
	defer server.Close()
	client := GraphQLClient{
		Endpoint:      server.URL,
		AllowInsecure: true,
		Retry: RetryConfig{
			Retries:   2,
			BaseDelay: time.Millisecond,
			Sleep: func(context.Context, time.Duration) error {
				return nil
			},
		},
	}
	page, err := client.FetchEvents(context.Background(), EventsQuery{PackageID: testPackageID, ModuleName: "character"})
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	if len(page.Nodes) != 1 || page.EndCursor != "cursor-1" {
		t.Fatalf("unexpected page %#v", page)
	}
}

func TestFetchObjectsDecodesMoveObjects(t *testing.T) {
	objectType := testPackageID + "::character::PlayerProfile"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "data": {
		    "objects": {
		      "pageInfo": {"hasNextPage": false, "endCursor": "cursor-1"},
		      "nodes": [
		        {
		          "address": "0xobject",
		          "digest": "AbcDigest",
		          "version": 7,
		          "asMoveObject": {
		            "contents": {
		              "type": {"repr": "` + objectType + `"},
		              "json": {"character_id": "2112091476"}
		            }
		          }
		        }
		      ]
		    }
		  }
		}`))
	}))
	defer server.Close()
	client := GraphQLClient{Endpoint: server.URL, AllowInsecure: true}
	page, err := client.FetchObjects(context.Background(), ObjectsQuery{Type: objectType})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Nodes) != 1 || page.Nodes[0].Version.String() != "7" {
		t.Fatalf("unexpected page %#v", page)
	}
}

func TestNormalizeMoveEventProducesRegistryEvent(t *testing.T) {
	node := MoveEventNode{
		SequenceNumber: 2,
		Timestamp:      "2026-05-27T18:12:12.879Z",
		Sender:         &AddressNode{Address: "0x59714bcd14f03bd20794bd3b5a2a52a0045e75e1bc9cc78aada8c56847e5731c"},
		Transaction:    &TransactionNode{Digest: "7YCpwNPBFJrd6APq6ModDosVUcCmSqsCSB9X76zzm3ch"},
		TransactionModule: &TransactionModuleNode{
			Name:    "character",
			Package: &AddressNode{Address: testPackageID},
		},
		Contents: &MoveEventContents{
			Type: &TypeNode{Repr: testPackageID + "::character::CharacterCreatedEvent"},
			JSON: map[string]any{
				"key": map[string]any{
					"item_id": "2112091476",
					"tenant":  "stillness",
				},
			},
		},
	}
	event, err := NormalizeMoveEvent(node, NormalizeOptions{Environment: "stillness", SourceID: "source:sui:sui-testnet:graphql"})
	if err != nil {
		t.Fatal(err)
	}
	if event.ID != "event:7YCpwNPBFJrd6APq6ModDosVUcCmSqsCSB9X76zzm3ch:2" {
		t.Fatalf("unexpected event id %s", event.ID)
	}
	if event.Kind != "character.created" {
		t.Fatalf("unexpected event kind %s", event.Kind)
	}
	if event.PackageID != testPackageID || event.Module != "character" {
		t.Fatalf("unexpected package/module %#v", event)
	}
	if event.OccurredAt.Format(time.RFC3339Nano) != "2026-05-27T18:12:12.879Z" {
		t.Fatalf("unexpected occurredAt %s", event.OccurredAt)
	}
	if event.Cycle != nil {
		t.Fatalf("pre-Cycle-6 timestamp should not be cycle-labelled, got %#v", event.Cycle)
	}
	encoded, err := json.Marshal(event.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(encoded) {
		t.Fatal("payload is not valid JSON")
	}
}

func TestNormalizeMoveObjectProducesRegistryObject(t *testing.T) {
	node := MoveObjectNode{
		Address: "0x59714bcd14f03bd20794bd3b5a2a52a0045e75e1bc9cc78aada8c56847e5731c",
		Digest:  "AbcDigest",
		Version: GraphQLScalar("7"),
		AsMoveObject: &MoveObjectData{Contents: &MoveEventContents{
			Type: &TypeNode{Repr: testPackageID + "::character::PlayerProfile"},
			JSON: map[string]any{
				"character_id": "2112091476",
			},
		}},
	}
	object, err := NormalizeMoveObject(node, NormalizeOptions{Environment: "stillness", SourceID: "source:sui:sui-testnet:graphql:objects"})
	if err != nil {
		t.Fatal(err)
	}
	if object.ID != "object:0x59714bcd14f03bd20794bd3b5a2a52a0045e75e1bc9cc78aada8c56847e5731c:7" {
		t.Fatalf("unexpected object id %s", object.ID)
	}
	if object.PackageID != testPackageID || object.Module != "character" || object.TypeName != "PlayerProfile" {
		t.Fatalf("unexpected object type %#v", object)
	}
	encoded, err := json.Marshal(object.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(encoded) {
		t.Fatal("payload is not valid JSON")
	}
}

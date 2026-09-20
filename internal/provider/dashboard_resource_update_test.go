package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	testDashcardsA = `[{"card_id":1,"row":0,"col":0,"size_x":4,"size_y":4,"dashboard_tab_id":1}]`
	testDashcardsB = `[{"card_id":1,"row":0,"col":4,"size_x":4,"size_y":4,"dashboard_tab_id":1}]`
	testTabs       = `[{"id":1,"name":"Main"}]`
)

func testDashboardModel(name string, cards string, tabs types.String) DashboardResourceModel {
	return DashboardResourceModel{
		Id:               types.Int64Value(246),
		Name:             types.StringValue(name),
		AutoApplyFilters: types.BoolValue(true),
		ParametersJson:   types.StringValue(`[]`),
		CardsJson:        types.StringValue(cards),
		TabsJson:         tabs,
	}
}

func assertPayloadKeys(t *testing.T, payload map[string]any, present []string, absent []string) {
	t.Helper()
	for _, k := range present {
		if _, ok := payload[k]; !ok {
			t.Errorf("expected %q in the update payload, got keys %v", k, payloadKeys(payload))
		}
	}
	for _, k := range absent {
		if _, ok := payload[k]; ok {
			t.Errorf("did not expect %q in the update payload, got keys %v", k, payloadKeys(payload))
		}
	}
}

func payloadKeys(payload map[string]any) []string {
	keys := make([]string, 0, len(payload))
	for k := range payload {
		keys = append(keys, k)
	}
	return keys
}

// A creation has no prior state: the whole content is sent.
func TestMakeUpdatePayloadSendsContentOnCreate(t *testing.T) {
	data := testDashboardModel("Board", testDashcardsA, types.StringValue(testTabs))

	payload, diags := makeUpdatePayload(data, nil)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	assertPayloadKeys(t, payload, []string{"name", "description", "collection_id", "parameters", "dashcards", "tabs"}, nil)
}

// A rename that leaves the content untouched sends only the dashboard's own properties, so the existing dashcards
// and tabs keep their IDs.
func TestMakeUpdatePayloadOmitsUnchangedContent(t *testing.T) {
	prior := testDashboardModel("CS Investigation", testDashcardsA, types.StringValue(testTabs))
	planned := testDashboardModel("[SA] CS Investigation", testDashcardsA, types.StringValue(testTabs))

	payload, diags := makeUpdatePayload(planned, &prior)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	assertPayloadKeys(t, payload, []string{"name", "description", "cache_ttl", "auto_apply_filters", "collection_id", "collection_position"}, []string{"parameters", "dashcards", "tabs"})
	if name, ok := payload["name"].(*string); !ok || name == nil || *name != "[SA] CS Investigation" {
		t.Errorf("expected the new name in the payload, got %#v", payload["name"])
	}
}

// Any content change still replaces the whole content, as before.
func TestMakeUpdatePayloadSendsChangedContent(t *testing.T) {
	prior := testDashboardModel("Board", testDashcardsA, types.StringValue(testTabs))
	planned := testDashboardModel("Board", testDashcardsB, types.StringValue(testTabs))

	payload, diags := makeUpdatePayload(planned, &prior)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	assertPayloadKeys(t, payload, []string{"name", "parameters", "dashcards", "tabs"}, nil)
}

// Tabs are compared like the other content: null against null is unchanged, null against a value is a change.
func TestMakeUpdatePayloadComparesNullTabs(t *testing.T) {
	prior := testDashboardModel("Board", testDashcardsA, types.StringNull())

	unchanged := testDashboardModel("Renamed", testDashcardsA, types.StringNull())
	payload, diags := makeUpdatePayload(unchanged, &prior)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	assertPayloadKeys(t, payload, []string{"name"}, []string{"parameters", "dashcards", "tabs"})

	withTabs := testDashboardModel("Renamed", testDashcardsA, types.StringValue(testTabs))
	payload, diags = makeUpdatePayload(withTabs, &prior)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	assertPayloadKeys(t, payload, []string{"name", "parameters", "dashcards", "tabs"}, nil)
}

// A formatting-only difference in the JSON is not recognised as "unchanged" and falls back to the full update —
// the safe direction.
func TestMakeUpdatePayloadFallsBackOnReformattedContent(t *testing.T) {
	prior := testDashboardModel("Board", testDashcardsA, types.StringValue(testTabs))
	planned := testDashboardModel("Board", `[ {"card_id": 1, "row": 0, "col": 0, "size_x": 4, "size_y": 4, "dashboard_tab_id": 1} ]`, types.StringValue(testTabs))

	payload, diags := makeUpdatePayload(planned, &prior)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	assertPayloadKeys(t, payload, []string{"name", "parameters", "dashcards", "tabs"}, nil)
}

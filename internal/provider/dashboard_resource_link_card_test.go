package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// A dashboard whose first dashcard is a link card to dashboard 38, as `cards_json` spells it (captured from a GET,
// so it carries the hydrated attributes as they were at capture time), plus a regular card.
const testLinkCardsJson = `[{"card_id":null,"col":0,"parameter_mappings":[],"row":0,"series":[],"size_x":8,"size_y":1,"visualization_settings":{"link":{"entity":{"collection_id":424,"db_id":null,"description":null,"display":null,"id":38,"model":"dashboard","name":"Service Fee Detail Level Receipt "}},"virtual_card":{"archived":false,"dataset_query":{},"display":"link","name":null,"visualization_settings":{}}}},{"card_id":942,"col":0,"parameter_mappings":[],"row":1,"series":[],"size_x":24,"size_y":8,"visualization_settings":{}}]`

// The same dashboard as the Metabase API returns it after dashboard 38 was renamed: the link card's entity is
// re-hydrated with the new name (the dashcards also carry attributes the provider does not manage).
const testLinkCardsResponse = `{"id":38,"name":"[EG] Service Fee Detail Level Receipt","dashcards":[{"id":901,"dashboard_id":38,"card_id":942,"col":0,"parameter_mappings":[],"row":1,"series":[],"size_x":24,"size_y":8,"visualization_settings":{},"card":{"id":942}},{"id":900,"dashboard_id":38,"card_id":null,"col":0,"parameter_mappings":[],"row":0,"series":[],"size_x":8,"size_y":1,"visualization_settings":{"link":{"entity":{"collection_id":424,"db_id":null,"description":null,"display":null,"id":38,"model":"dashboard","name":"[EG] Service Fee Detail Level Receipt"}},"virtual_card":{"archived":false,"dataset_query":{},"display":"link","name":null,"visualization_settings":{}}}}]}`

// Same response, but the regular card moved: a real difference that must reach the state.
const testLinkCardsResponseMoved = `{"id":38,"name":"[EG] Service Fee Detail Level Receipt","dashcards":[{"id":901,"dashboard_id":38,"card_id":942,"col":0,"parameter_mappings":[],"row":2,"series":[],"size_x":24,"size_y":8,"visualization_settings":{}},{"id":900,"dashboard_id":38,"card_id":null,"col":0,"parameter_mappings":[],"row":0,"series":[],"size_x":8,"size_y":1,"visualization_settings":{"link":{"entity":{"collection_id":424,"db_id":null,"description":null,"display":null,"id":38,"model":"dashboard","name":"[EG] Service Fee Detail Level Receipt"}},"virtual_card":{"archived":false,"dataset_query":{},"display":"link","name":null,"visualization_settings":{}}}}]}`

// Same response, but the link card now points at another dashboard: a real difference too.
const testLinkCardsResponseRetargeted = `{"id":38,"name":"[EG] Service Fee Detail Level Receipt","dashcards":[{"id":901,"dashboard_id":38,"card_id":942,"col":0,"parameter_mappings":[],"row":1,"series":[],"size_x":24,"size_y":8,"visualization_settings":{}},{"id":900,"dashboard_id":38,"card_id":null,"col":0,"parameter_mappings":[],"row":0,"series":[],"size_x":8,"size_y":1,"visualization_settings":{"link":{"entity":{"collection_id":424,"db_id":null,"description":null,"display":null,"id":66,"model":"dashboard","name":"[EG] Billing Validation (Offline,Service Fees)"}},"virtual_card":{"archived":false,"dataset_query":{},"display":"link","name":null,"visualization_settings":{}}}}]}`

func testLinkCardsModel() *DashboardResourceModel {
	return &DashboardResourceModel{
		Id:        types.Int64Value(38),
		Name:      types.StringValue("[EG] Service Fee Detail Level Receipt"),
		CardsJson: types.StringValue(testLinkCardsJson),
	}
}

// Renaming the dashboard a link card points to changes what Metabase returns for the card, not the card itself:
// the state keeps the user's `cards_json` verbatim.
func TestUpdateCardsFromRawBodyIgnoresLinkEntityHydration(t *testing.T) {
	data := testLinkCardsModel()

	diags := updateCardsFromRawBody([]byte(testLinkCardsResponse), data, map[int]int{})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if data.CardsJson.ValueString() != testLinkCardsJson {
		t.Errorf("expected cards_json to be kept verbatim, got %s", data.CardsJson.ValueString())
	}
}

// A real change elsewhere in the dashboard still reaches the state.
func TestUpdateCardsFromRawBodyKeepsRealChangesNextToLinkCards(t *testing.T) {
	data := testLinkCardsModel()

	diags := updateCardsFromRawBody([]byte(testLinkCardsResponseMoved), data, map[int]int{})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if data.CardsJson.ValueString() == testLinkCardsJson {
		t.Errorf("expected cards_json to pick up the moved card, but it was kept verbatim")
	}
}

// Pointing the link card at a different entity is a change of the card, not hydration.
func TestUpdateCardsFromRawBodyDetectsRetargetedLinkCard(t *testing.T) {
	data := testLinkCardsModel()

	diags := updateCardsFromRawBody([]byte(testLinkCardsResponseRetargeted), data, map[int]int{})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if data.CardsJson.ValueString() == testLinkCardsJson {
		t.Errorf("expected cards_json to pick up the retargeted link, but it was kept verbatim")
	}
}

// The comparison helper never modifies its input.
func TestWithoutLinkEntityHydrationLeavesInputUntouched(t *testing.T) {
	cards := []any{
		map[string]any{
			"card_id": nil,
			"visualization_settings": map[string]any{
				"link": map[string]any{
					"entity": map[string]any{"id": float64(38), "model": "dashboard", "name": "Before"},
				},
			},
		},
	}

	stripped := withoutLinkEntityHydration(cards)

	entity := stripped[0].(map[string]any)["visualization_settings"].(map[string]any)["link"].(map[string]any)["entity"].(map[string]any)
	if _, ok := entity["name"]; ok {
		t.Errorf("expected the hydrated name to be dropped from the comparison copy, got %v", entity)
	}
	if entity["id"] != float64(38) || entity["model"] != "dashboard" {
		t.Errorf("expected id and model to be kept, got %v", entity)
	}

	original := cards[0].(map[string]any)["visualization_settings"].(map[string]any)["link"].(map[string]any)["entity"].(map[string]any)
	if original["name"] != "Before" {
		t.Errorf("expected the input to be left untouched, got %v", original)
	}
}

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/flovouin/terraform-provider-metabase/metabase"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// Builds a `GetSettingResponse` as returned by the Metabase API: string settings are served as `text/plain` with the
// raw (unquoted) string, other setting types as `application/json`.
func makeSettingResponse(statusCode int, contentType string, body string) *metabase.GetSettingResponse {
	return &metabase.GetSettingResponse{
		Body: []byte(body),
		HTTPResponse: &http.Response{
			StatusCode: statusCode,
			Header:     http.Header{"Content-Type": []string{contentType}},
		},
	}
}

func TestSettingValueFromResponse(t *testing.T) {
	// A known value that is semantically equal to the API value is preserved as is.
	value, diags := settingValueFromResponse(
		makeSettingResponse(200, "application/json;charset=utf-8", `{"a": 1, "b": 2}`),
		types.StringValue(`{"b":2,"a":1}`),
	)
	if diags.HasError() {
		t.Fatalf("Unexpected error parsing setting value: %v", diags)
	}
	if value != `{"b":2,"a":1}` {
		t.Fatalf("Expected the known encoding to be preserved, got %s.", value)
	}

	// A different API value replaces the known value, encoded canonically.
	value, diags = settingValueFromResponse(
		makeSettingResponse(200, "application/json;charset=utf-8", `true`),
		types.StringValue(`false`),
	)
	if diags.HasError() {
		t.Fatalf("Unexpected error parsing setting value: %v", diags)
	}
	if value != `true` {
		t.Fatalf("Expected the API value, got %s.", value)
	}

	// A null known value (e.g. during import) yields the canonical API value.
	value, diags = settingValueFromResponse(
		makeSettingResponse(200, "application/json;charset=utf-8", `303`),
		types.StringNull(),
	)
	if diags.HasError() {
		t.Fatalf("Unexpected error parsing setting value: %v", diags)
	}
	if value != `303` {
		t.Fatalf("Expected the API value, got %s.", value)
	}

	// A string setting is served raw as text/plain, and is JSON-encoded for the state.
	value, diags = settingValueFromResponse(
		makeSettingResponse(200, "text/plain", `My Site`),
		types.StringValue(`"My Site"`),
	)
	if diags.HasError() {
		t.Fatalf("Unexpected error parsing setting value: %v", diags)
	}
	if value != `"My Site"` {
		t.Fatalf("Expected the JSON-encoded string, got %s.", value)
	}

	_, diags = settingValueFromResponse(
		makeSettingResponse(200, "application/json;charset=utf-8", `not json`),
		types.StringNull(),
	)
	if !diags.HasError() {
		t.Fatal("Expected an error when a JSON response cannot be parsed.")
	}
}

func TestSettingResponseHasValue(t *testing.T) {
	// Metabase answers 204 with an empty body when the setting is unset.
	if settingResponseHasValue(makeSettingResponse(204, "", "")) {
		t.Fatal("Expected a 204 response to have no value.")
	}
	if settingResponseHasValue(makeSettingResponse(200, "application/json;charset=utf-8", "null")) {
		t.Fatal("Expected a null body to have no value.")
	}
	if !settingResponseHasValue(makeSettingResponse(200, "application/json;charset=utf-8", "false")) {
		t.Fatal("Expected a false body to have a value.")
	}
	if !settingResponseHasValue(makeSettingResponse(200, "text/plain", "Metabase")) {
		t.Fatal("Expected a string body to have a value.")
	}
	// A string setting whose raw value happens to be "null" is still a value.
	if !settingResponseHasValue(makeSettingResponse(200, "text/plain", "null")) {
		t.Fatal("Expected a text/plain null body to have a value.")
	}
}

// Checks that the setting value stored in the Terraform state matches the value returned by the Metabase API.
func testAccCheckSettingMatchesApi(resourceName string, expectedValue any) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("Failed to find resource %s in state.", resourceName)
		}

		response, err := testAccMetabaseClient.GetSettingWithResponse(context.Background(), rs.Primary.ID)
		if err != nil {
			return err
		}
		if response.StatusCode() != 200 {
			return fmt.Errorf("Received unexpected response from the Metabase API when getting setting %s.", rs.Primary.ID)
		}

		var apiValue any
		if settingResponseIsJson(response) {
			if err := json.Unmarshal(response.Body, &apiValue); err != nil {
				return err
			}
		} else {
			apiValue = string(response.Body)
		}
		if !reflect.DeepEqual(apiValue, expectedValue) {
			return fmt.Errorf("Expected setting %s to be %v, got %v.", rs.Primary.ID, expectedValue, apiValue)
		}

		return nil
	}
}

func testAccSettingResource(name string, key string, value string) string {
	return fmt.Sprintf(`
resource "metabase_setting" "%s" {
  key   = "%s"
  value = jsonencode(%s)
}
`, name, key, value)
}

func TestAccSettingResource(t *testing.T) {
	homepageConfig := testAccSettingResource("homepage", "custom-homepage", "true")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerApiKeyConfig + homepageConfig +
					testAccSettingResource("site_name", "site-name", `"Terraform Acceptance Tests"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("metabase_setting.site_name", "id", "site-name"),
					resource.TestCheckResourceAttr("metabase_setting.site_name", "key", "site-name"),
					resource.TestCheckResourceAttr("metabase_setting.site_name", "value", `"Terraform Acceptance Tests"`),
					testAccCheckSettingMatchesApi("metabase_setting.site_name", "Terraform Acceptance Tests"),
					resource.TestCheckResourceAttr("metabase_setting.homepage", "value", "true"),
					testAccCheckSettingMatchesApi("metabase_setting.homepage", true),
				),
			},
			{
				ResourceName:      "metabase_setting.site_name",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: providerApiKeyConfig + homepageConfig +
					testAccSettingResource("site_name", "site-name", `"Terraform Acceptance Tests (updated)"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("metabase_setting.site_name", "value", `"Terraform Acceptance Tests (updated)"`),
					testAccCheckSettingMatchesApi("metabase_setting.site_name", "Terraform Acceptance Tests (updated)"),
				),
			},
		},
	})
}

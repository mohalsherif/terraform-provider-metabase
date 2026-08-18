package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestWidthFromRawDashboardBody(t *testing.T) {
	width, diags := widthFromRawDashboardBody([]byte(`{"id": 1, "width": "full"}`))
	if diags.HasError() {
		t.Fatalf("Unexpected error parsing width: %v", diags)
	}
	if width != "full" {
		t.Fatalf("Expected width to be full, got %s.", width)
	}

	_, diags = widthFromRawDashboardBody([]byte(`{"id": 1}`))
	if !diags.HasError() {
		t.Fatal("Expected an error when the width is missing from the response.")
	}

	_, diags = widthFromRawDashboardBody([]byte(`not json`))
	if !diags.HasError() {
		t.Fatal("Expected an error when the response is not JSON.")
	}
}

// Checks that the width stored in the Terraform state matches the width returned by the Metabase API.
func testAccCheckDashboardWidthMatchesApi(resourceName string, expectedWidth string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("Failed to find resource %s in state.", resourceName)
		}

		id, err := strconv.Atoi(rs.Primary.ID)
		if err != nil {
			return err
		}

		response, err := testAccMetabaseClient.GetDashboardWithResponse(context.Background(), id)
		if err != nil {
			return err
		}
		if response.StatusCode() != 200 {
			return fmt.Errorf("Received unexpected response from the Metabase API when getting dashboard.")
		}

		var rawDashboard map[string]any
		if err := json.Unmarshal(response.Body, &rawDashboard); err != nil {
			return err
		}

		width, ok := rawDashboard["width"].(string)
		if !ok {
			return fmt.Errorf("The Metabase API did not return a width for dashboard %d.", id)
		}
		if width != expectedWidth {
			return fmt.Errorf("Expected dashboard %d width to be %s, got %s.", id, expectedWidth, width)
		}

		return nil
	}
}

func testAccDashboardWidthResource(name string, width string) string {
	return fmt.Sprintf(`
resource "metabase_dashboard_width" "%s" {
  dashboard_id = metabase_dashboard.test.id
  width        = "%s"
}
`, name, width)
}

func TestAccDashboardWidthResource(t *testing.T) {
	dashboardConfig := testAccDashboardResource("test", "📐 Dashboard", "📖 Description")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerApiKeyConfig + dashboardConfig + testAccDashboardWidthResource("test", "full"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("metabase_dashboard_width.test", "id"),
					resource.TestCheckResourceAttrPair("metabase_dashboard_width.test", "dashboard_id", "metabase_dashboard.test", "id"),
					resource.TestCheckResourceAttr("metabase_dashboard_width.test", "width", "full"),
					testAccCheckDashboardWidthMatchesApi("metabase_dashboard_width.test", "full"),
					// The partial update must leave the rest of the dashboard untouched.
					testAccCheckDashboardExists("metabase_dashboard.test"),
				),
			},
			{
				ResourceName:      "metabase_dashboard_width.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: providerApiKeyConfig + dashboardConfig + testAccDashboardWidthResource("test", "fixed"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("metabase_dashboard_width.test", "width", "fixed"),
					testAccCheckDashboardWidthMatchesApi("metabase_dashboard_width.test", "fixed"),
				),
			},
		},
	})
}

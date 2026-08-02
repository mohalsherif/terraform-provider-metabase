package provider

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/flovouin/terraform-provider-metabase/metabase"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func testAccSnippetResource(name string, snippetName string, content string) string {
	return fmt.Sprintf(`
resource "metabase_snippet" "%s" {
  name    = "%s"
  content = "%s"
}
`,
		name,
		snippetName,
		content,
	)
}

// Returns the snippet with the given ID, or `nil` if it does not exist.
func getSnippetById(id int) (*metabase.NativeQuerySnippet, error) {
	response, err := testAccMetabaseClient.GetNativeQuerySnippetWithResponse(context.Background(), id)
	if err != nil {
		return nil, err
	}
	if response.StatusCode() == 404 {
		return nil, nil
	}
	if response.StatusCode() != 200 {
		return nil, fmt.Errorf("Received unexpected response from the Metabase API when getting a snippet.")
	}

	return response.JSON200, nil
}

func testAccCheckSnippetExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("Failed to find resource %s in state.", resourceName)
		}

		snippetId, err := strconv.ParseInt(rs.Primary.ID, 10, 64)
		if err != nil {
			return err
		}

		snippet, err := getSnippetById(int(snippetId))
		if err != nil {
			return err
		}
		if snippet == nil {
			return fmt.Errorf("Failed to find snippet %s in the Metabase API.", rs.Primary.ID)
		}
		if snippet.Archived {
			return fmt.Errorf("Snippet %s exists but is archived.", rs.Primary.ID)
		}

		if rs.Primary.Attributes["name"] != snippet.Name {
			return fmt.Errorf("Terraform resource and API response do not match for the snippet name.")
		}
		if rs.Primary.Attributes["content"] != snippet.Content {
			return fmt.Errorf("Terraform resource and API response do not match for the snippet content.")
		}

		return nil
	}
}

// Snippets cannot be deleted through the Metabase API, only archived. Destroy
// therefore checks for the archived state rather than absence.
func testAccCheckSnippetDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "metabase_snippet" {
			continue
		}

		snippetId, err := strconv.ParseInt(rs.Primary.ID, 10, 64)
		if err != nil {
			return err
		}

		snippet, err := getSnippetById(int(snippetId))
		if err != nil {
			return err
		}
		if snippet != nil && !snippet.Archived {
			return fmt.Errorf("Snippet %s still exists and is not archived.", rs.Primary.ID)
		}
	}

	return nil
}

func TestAccSnippetResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSnippetDestroy,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + testAccSnippetResource("test", "🧪 Valid receipts (test)", "WHERE deleted_at IS NULL"),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckSnippetExists("metabase_snippet.test"),
					resource.TestCheckResourceAttrSet("metabase_snippet.test", "id"),
					resource.TestCheckResourceAttr("metabase_snippet.test", "name", "🧪 Valid receipts (test)"),
					resource.TestCheckResourceAttr("metabase_snippet.test", "content", "WHERE deleted_at IS NULL"),
				),
			},
			{
				ResourceName: "metabase_snippet.test",
				ImportState:  true,
			},
			{
				Config: providerConfig + testAccSnippetResource("test", "🧪 Valid receipts (test, updated)", "WHERE deleted_at IS NULL AND state != 'Cancelled'"),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckSnippetExists("metabase_snippet.test"),
					resource.TestCheckResourceAttrSet("metabase_snippet.test", "id"),
					resource.TestCheckResourceAttr("metabase_snippet.test", "name", "🧪 Valid receipts (test, updated)"),
					resource.TestCheckResourceAttr("metabase_snippet.test", "content", "WHERE deleted_at IS NULL AND state != 'Cancelled'"),
				),
			},
		},
	})
}

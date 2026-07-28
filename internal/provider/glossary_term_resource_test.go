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

func testAccGlossaryTermResource(name string, term string, definition string) string {
	return fmt.Sprintf(`
resource "metabase_glossary_term" "%s" {
  term       = "%s"
  definition = "%s"
}
`,
		name,
		term,
		definition,
	)
}

// Returns the glossary term with the given ID, or `nil` if it does not exist.
func getGlossaryTermById(id int) (*metabase.GlossaryTerm, error) {
	response, err := testAccMetabaseClient.ListGlossaryTermsWithResponse(context.Background())
	if err != nil {
		return nil, err
	}
	if response.StatusCode() != 200 {
		return nil, fmt.Errorf("Received unexpected response from the Metabase API when listing glossary terms.")
	}

	for _, t := range response.JSON200.Data {
		if t.Id == id {
			return &t, nil
		}
	}

	return nil, nil
}

func testAccCheckGlossaryTermExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("Failed to find resource %s in state.", resourceName)
		}

		termId, err := strconv.ParseInt(rs.Primary.ID, 10, 64)
		if err != nil {
			return err
		}

		term, err := getGlossaryTermById(int(termId))
		if err != nil {
			return err
		}
		if term == nil {
			return fmt.Errorf("Failed to find glossary term %s in the Metabase API.", rs.Primary.ID)
		}

		if rs.Primary.Attributes["term"] != term.Term {
			return fmt.Errorf("Terraform resource and API response do not match for the glossary term.")
		}
		if rs.Primary.Attributes["definition"] != term.Definition {
			return fmt.Errorf("Terraform resource and API response do not match for the glossary term definition.")
		}

		return nil
	}
}

func testAccCheckGlossaryTermDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "metabase_glossary_term" {
			continue
		}

		termId, err := strconv.ParseInt(rs.Primary.ID, 10, 64)
		if err != nil {
			return err
		}

		term, err := getGlossaryTermById(int(termId))
		if err != nil {
			return err
		}
		if term != nil {
			return fmt.Errorf("Glossary term %s still exists.", rs.Primary.ID)
		}
	}

	return nil
}

// Checks whether the Metabase instance exposes the glossary API, which was introduced in a recent version.
func isGlossaryAvailable() bool {
	// The client is nil when the acceptance testing environment has not been set up.
	if testAccMetabaseClient == nil {
		return false
	}

	response, err := testAccMetabaseClient.ListGlossaryTermsWithResponse(context.Background())
	if err != nil {
		return false
	}

	return response.StatusCode() == 200
}

func TestAccGlossaryTermResource(t *testing.T) {
	if !isGlossaryAvailable() {
		t.Skip("Skipping glossary term test - the Metabase instance does not expose the glossary API.")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGlossaryTermDestroy,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + testAccGlossaryTermResource("test", "📖 NMV", "Net merchandise value, excluding tips."),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckGlossaryTermExists("metabase_glossary_term.test"),
					resource.TestCheckResourceAttrSet("metabase_glossary_term.test", "id"),
					resource.TestCheckResourceAttr("metabase_glossary_term.test", "term", "📖 NMV"),
					resource.TestCheckResourceAttr("metabase_glossary_term.test", "definition", "Net merchandise value, excluding tips."),
				),
			},
			{
				ResourceName: "metabase_glossary_term.test",
				ImportState:  true,
			},
			{
				Config: providerConfig + testAccGlossaryTermResource("test", "📖 NMV updated", "Net merchandise value, tips excluded."),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckGlossaryTermExists("metabase_glossary_term.test"),
					resource.TestCheckResourceAttrSet("metabase_glossary_term.test", "id"),
					resource.TestCheckResourceAttr("metabase_glossary_term.test", "term", "📖 NMV updated"),
					resource.TestCheckResourceAttr("metabase_glossary_term.test", "definition", "Net merchandise value, tips excluded."),
				),
			},
		},
	})
}

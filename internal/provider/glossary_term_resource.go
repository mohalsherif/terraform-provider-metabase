package provider

import (
	"context"

	"github.com/flovouin/terraform-provider-metabase/metabase"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensures provider defined types fully satisfy framework interfaces.
var _ resource.ResourceWithImportState = &GlossaryTermResource{}

// Creates a new glossary term resource.
func NewGlossaryTermResource() resource.Resource {
	return &GlossaryTermResource{
		MetabaseBaseResource{name: "glossary_term"},
	}
}

// A resource handling a glossary term.
type GlossaryTermResource struct {
	MetabaseBaseResource
}

// The Terraform model for a glossary term.
type GlossaryTermResourceModel struct {
	Id         types.Int64  `tfsdk:"id"`         // The ID of the glossary term.
	Term       types.String `tfsdk:"term"`       // The term being defined.
	Definition types.String `tfsdk:"definition"` // The definition of the term.
}

func (r *GlossaryTermResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A term in the Metabase glossary, along with its definition.",

		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the glossary term.",
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"term": schema.StringAttribute{
				MarkdownDescription: "The term being defined.",
				Required:            true,
			},
			"definition": schema.StringAttribute{
				MarkdownDescription: "The definition of the term.",
				Required:            true,
			},
		},
	}
}

// Updates the given `GlossaryTermResourceModel` from the `GlossaryTerm` returned by the Metabase API.
func updateModelFromGlossaryTerm(t metabase.GlossaryTerm, data *GlossaryTermResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	data.Id = types.Int64Value(int64(t.Id))
	data.Term = types.StringValue(t.Term)
	data.Definition = types.StringValue(t.Definition)

	return diags
}

func (r *GlossaryTermResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *GlossaryTermResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createResp, err := r.client.CreateGlossaryTermWithResponse(ctx, metabase.CreateGlossaryTermBody{
		Term:       data.Term.ValueString(),
		Definition: data.Definition.ValueString(),
	})

	resp.Diagnostics.Append(checkMetabaseResponse(createResp, err, []int{200}, "create glossary term")...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(updateModelFromGlossaryTerm(*createResp.JSON200, data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *GlossaryTermResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *GlossaryTermResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The Metabase API does not expose a single glossary term, only the entire list.
	listResp, err := r.client.ListGlossaryTermsWithResponse(ctx)

	resp.Diagnostics.Append(checkMetabaseResponse(listResp, err, []int{200}, "list glossary terms")...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := int(data.Id.ValueInt64())
	for _, t := range listResp.JSON200.Data {
		if t.Id != id {
			continue
		}

		resp.Diagnostics.Append(updateModelFromGlossaryTerm(t, data)...)
		if resp.Diagnostics.HasError() {
			return
		}

		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
		return
	}

	// The term is not part of the list anymore, it has been deleted.
	resp.State.RemoveResource(ctx)
}

func (r *GlossaryTermResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *GlossaryTermResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateResp, err := r.client.UpdateGlossaryTermWithResponse(ctx, int(data.Id.ValueInt64()), metabase.UpdateGlossaryTermBody{
		Term:       data.Term.ValueString(),
		Definition: data.Definition.ValueString(),
	})

	resp.Diagnostics.Append(checkMetabaseResponse(updateResp, err, []int{200}, "update glossary term")...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(updateModelFromGlossaryTerm(*updateResp.JSON200, data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *GlossaryTermResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *GlossaryTermResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteResp, err := r.client.DeleteGlossaryTermWithResponse(ctx, int(data.Id.ValueInt64()))

	// A 404 means the term has already been deleted, which is the expected end state.
	resp.Diagnostics.Append(checkMetabaseResponse(deleteResp, err, []int{204, 404}, "delete glossary term")...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *GlossaryTermResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importStatePassthroughIntegerId(ctx, req, resp)
}

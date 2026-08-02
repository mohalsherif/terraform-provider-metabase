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
var _ resource.ResourceWithImportState = &SnippetResource{}

// Creates a new snippet resource.
func NewSnippetResource() resource.Resource {
	return &SnippetResource{
		MetabaseBaseResource{name: "snippet"},
	}
}

// A resource handling a native query snippet.
type SnippetResource struct {
	MetabaseBaseResource
}

// The Terraform model for a native query snippet.
type SnippetResourceModel struct {
	Id          types.Int64  `tfsdk:"id"`          // The ID of the snippet.
	Name        types.String `tfsdk:"name"`        // The name of the snippet.
	Content     types.String `tfsdk:"content"`     // The SQL fragment.
	Description types.String `tfsdk:"description"` // The description of the snippet.
}

func (r *SnippetResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A native query snippet: a reusable SQL fragment referenced from native queries as `{{snippet: name}}`.\n\nThe Metabase API cannot delete snippets, only archive them: destroying this resource archives the snippet, and an archived snippet still holds its (instance-unique) name.",

		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the snippet.",
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the snippet, unique across the instance.",
				Required:            true,
			},
			"content": schema.StringAttribute{
				MarkdownDescription: "The SQL fragment.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the snippet.",
				Optional:            true,
			},
		},
	}
}

// Updates the given `SnippetResourceModel` from the `NativeQuerySnippet` returned by the Metabase API.
func updateModelFromNativeQuerySnippet(s metabase.NativeQuerySnippet, data *SnippetResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	data.Id = types.Int64Value(int64(s.Id))
	data.Name = types.StringValue(s.Name)
	data.Content = types.StringValue(s.Content)
	// The API normalizes a missing description to an empty string on some
	// versions; treat both null and empty as "no description" so an optional
	// unset attribute does not produce a permanent diff.
	if s.Description != nil && *s.Description != "" {
		data.Description = types.StringValue(*s.Description)
	} else {
		data.Description = types.StringNull()
	}

	return diags
}

func (r *SnippetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *SnippetResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createResp, err := r.client.CreateNativeQuerySnippetWithResponse(ctx, metabase.CreateNativeQuerySnippetBody{
		Name:        data.Name.ValueString(),
		Content:     data.Content.ValueString(),
		Description: data.Description.ValueStringPointer(),
	})

	resp.Diagnostics.Append(checkMetabaseResponse(createResp, err, []int{200}, "create snippet")...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(updateModelFromNativeQuerySnippet(*createResp.JSON200, data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SnippetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *SnippetResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	getResp, err := r.client.GetNativeQuerySnippetWithResponse(ctx, int(data.Id.ValueInt64()))

	resp.Diagnostics.Append(checkMetabaseResponse(getResp, err, []int{200, 404}, "get snippet")...)
	if resp.Diagnostics.HasError() {
		return
	}

	// An archived snippet no longer resolves in queries, which makes it the
	// closest thing to deleted the API can express.
	if getResp.StatusCode() == 404 || getResp.JSON200.Archived {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(updateModelFromNativeQuerySnippet(*getResp.JSON200, data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SnippetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *SnippetResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateResp, err := r.client.UpdateNativeQuerySnippetWithResponse(ctx, int(data.Id.ValueInt64()), metabase.UpdateNativeQuerySnippetBody{
		Name:        data.Name.ValueStringPointer(),
		Content:     data.Content.ValueStringPointer(),
		Description: data.Description.ValueStringPointer(),
	})

	resp.Diagnostics.Append(checkMetabaseResponse(updateResp, err, []int{200}, "update snippet")...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(updateModelFromNativeQuerySnippet(*updateResp.JSON200, data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SnippetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *SnippetResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The Metabase API has no delete endpoint for snippets; archiving is the
	// supported way to retire one. A 404 means it is already gone.
	archived := true
	deleteResp, err := r.client.UpdateNativeQuerySnippetWithResponse(ctx, int(data.Id.ValueInt64()), metabase.UpdateNativeQuerySnippetBody{
		Archived: &archived,
	})

	resp.Diagnostics.Append(checkMetabaseResponse(deleteResp, err, []int{200, 404}, "archive snippet")...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *SnippetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importStatePassthroughIntegerId(ctx, req, resp)
}

package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensures provider defined types fully satisfy framework interfaces.
var _ resource.ResourceWithImportState = &DashboardWidthResource{}
var _ resource.ResourceWithValidateConfig = &DashboardWidthResource{}

// The values accepted by the Metabase API for a dashboard's width.
const (
	dashboardWidthFixed = "fixed"
	dashboardWidthFull  = "full"
)

// Creates a new dashboard width resource.
func NewDashboardWidthResource() resource.Resource {
	return &DashboardWidthResource{
		MetabaseBaseResource{name: "dashboard_width"},
	}
}

// A resource managing only the `width` attribute of an existing Metabase dashboard.
//
// The width is deliberately kept out of the `metabase_dashboard` resource: updating that resource replaces all the
// dashcards in the dashboard, and dashboards whose layout is owned by the Metabase UI are usually declared with
// `lifecycle { ignore_changes = all }`, which would also ignore a width attribute. This companion resource sends a
// partial update containing only the width, leaving the rest of the dashboard untouched.
type DashboardWidthResource struct {
	MetabaseBaseResource
}

// The Terraform model for a dashboard width.
type DashboardWidthResourceModel struct {
	Id          types.Int64  `tfsdk:"id"`           // The ID of the dashboard, mirrored from `dashboard_id`.
	DashboardId types.Int64  `tfsdk:"dashboard_id"` // The ID of the dashboard whose width is managed.
	Width       types.String `tfsdk:"width"`        // The width of the dashboard (`fixed` or `full`).
}

func (r *DashboardWidthResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `The width setting of an existing Metabase dashboard.

Unlike the attributes of the ` + "`metabase_dashboard`" + ` resource, the width is managed through a partial update that touches nothing else in the dashboard — no dashcard is replaced. This makes it safe to point at dashboards whose layout is owned by the Metabase UI (e.g. ` + "`metabase_dashboard`" + ` resources declared with ` + "`lifecycle { ignore_changes = all }`" + `), as well as dashboards not managed by Terraform at all.

Declare at most one ` + "`metabase_dashboard_width`" + ` per dashboard. Deleting the resource simply stops managing the width: the dashboard keeps its current value.`,

		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the dashboard, mirrored from `dashboard_id`.",
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"dashboard_id": schema.Int64Attribute{
				MarkdownDescription: "The ID of the dashboard whose width is managed.",
				Required:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"width": schema.StringAttribute{
				MarkdownDescription: "The width of the dashboard. Either `fixed` or `full`.",
				Required:            true,
			},
		},
	}
}

func (r *DashboardWidthResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data DashboardWidthResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.Width.IsNull() || data.Width.IsUnknown() {
		return
	}

	width := data.Width.ValueString()
	if width != dashboardWidthFixed && width != dashboardWidthFull {
		resp.Diagnostics.AddAttributeError(
			path.Root("width"),
			"Invalid dashboard width.",
			fmt.Sprintf("The width must be either %q or %q, got: %q.", dashboardWidthFixed, dashboardWidthFull, width),
		)
	}
}

// Extracts the `width` attribute from a raw dashboard response returned by the Metabase API.
// The generated API client does not model the width, which is why the raw body is parsed instead.
func widthFromRawDashboardBody(body []byte) (string, diag.Diagnostics) {
	var diags diag.Diagnostics

	var jsonResponse map[string]any
	err := json.Unmarshal(body, &jsonResponse)
	if err != nil {
		diags.AddError("Unable to parse get dashboard response.", err.Error())
		return "", diags
	}

	width, ok := jsonResponse["width"].(string)
	if !ok {
		diags.AddError(
			"The Metabase API did not return a width for the dashboard.",
			"Dashboard width requires Metabase 0.50.0 or later.",
		)
		return "", diags
	}

	return width, diags
}

// Sends a partial dashboard update containing only the width, and returns the width echoed by the Metabase API.
func (r *DashboardWidthResource) updateWidth(ctx context.Context, data *DashboardWidthResourceModel, operation string) diag.Diagnostics {
	var diags diag.Diagnostics

	payload, err := json.Marshal(map[string]any{
		"width": data.Width.ValueString(),
	})
	if err != nil {
		diags.AddError("Error creating the payload for dashboard width update.", err.Error())
		return diags
	}

	dashboardId := int(data.DashboardId.ValueInt64())
	updateResp, err := r.client.UpdateDashboardWithBodyWithResponse(ctx, dashboardId, "application/json", bytes.NewReader(payload))
	diags.Append(checkMetabaseResponse(updateResp, err, []int{200}, operation)...)
	if diags.HasError() {
		return diags
	}

	width, widthDiags := widthFromRawDashboardBody(updateResp.Body)
	diags.Append(widthDiags...)
	if diags.HasError() {
		return diags
	}

	data.Id = data.DashboardId
	data.Width = types.StringValue(width)

	return diags
}

func (r *DashboardWidthResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *DashboardWidthResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.updateWidth(ctx, data, "set dashboard width")...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DashboardWidthResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *DashboardWidthResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	getResp, err := r.client.GetDashboardWithResponse(ctx, int(data.DashboardId.ValueInt64()))
	resp.Diagnostics.Append(checkMetabaseResponse(getResp, err, []int{200, 404}, "get dashboard width")...)
	if resp.Diagnostics.HasError() {
		return
	}

	if getResp.StatusCode() == 404 || getResp.JSON200.Archived {
		resp.State.RemoveResource(ctx)
		return
	}

	width, widthDiags := widthFromRawDashboardBody(getResp.Body)
	resp.Diagnostics.Append(widthDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.Id = data.DashboardId
	data.Width = types.StringValue(width)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DashboardWidthResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *DashboardWidthResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.updateWidth(ctx, data, "update dashboard width")...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DashboardWidthResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Deleting the resource only stops managing the width. The dashboard keeps its current width: there is no
	// meaningful value to restore, and the dashboard itself may be archived or deleted by then.
}

func (r *DashboardWidthResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Unable to convert ID to an integer.", req.ID)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("dashboard_id"), id)...)
}

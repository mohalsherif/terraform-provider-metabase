package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/flovouin/terraform-provider-metabase/metabase"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensures provider defined types fully satisfy framework interfaces.
var _ resource.ResourceWithImportState = &SettingResource{}
var _ resource.ResourceWithValidateConfig = &SettingResource{}

// Creates a new setting resource.
func NewSettingResource() resource.Resource {
	return &SettingResource{
		MetabaseBaseResource{name: "setting"},
	}
}

// A resource managing a single Metabase instance-wide setting (`/api/setting/:key`).
type SettingResource struct {
	MetabaseBaseResource
}

// The Terraform model for a setting.
type SettingResourceModel struct {
	Id    types.String `tfsdk:"id"`    // The key of the setting, mirrored from `key`.
	Key   types.String `tfsdk:"key"`   // The key of the setting.
	Value types.String `tfsdk:"value"` // The JSON-encoded value of the setting.
}

func (r *SettingResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `A single Metabase instance-wide setting, managed through ` + "`/api/setting/:key`" + `.

The value is the raw JSON encoding of the setting, so any setting type can be expressed: use ` + "`jsonencode(...)`" + ` (e.g. ` + "`jsonencode(true)`" + `, ` + "`jsonencode(42)`" + `, ` + "`jsonencode(\"a string\")`" + `).

The resource manages a non-default value for the setting. Metabase stores a value equal to the setting's default as "unset", so configuring the default (e.g. ` + "`false`" + ` for a boolean setting that defaults to false) fails with an explicit error — delete the resource instead. Likewise, deleting the resource resets the setting to its Metabase default (the API is sent a ` + "`null`" + ` value), and a configuration must not set the value to JSON ` + "`null`" + `.

Settings whose value is redacted by the API (e.g. secrets) cannot be managed by this resource: the redacted read would never match the configuration.`,

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The key of the setting, mirrored from `key`.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"key": schema.StringAttribute{
				MarkdownDescription: "The key of the setting, e.g. `custom-homepage-dashboard`.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"value": schema.StringAttribute{
				MarkdownDescription: "The JSON-encoded value of the setting. Use `jsonencode(...)`.",
				Required:            true,
			},
		},
	}
}

func (r *SettingResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data SettingResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.Value.IsNull() || data.Value.IsUnknown() {
		return
	}

	var value any
	if err := json.Unmarshal([]byte(data.Value.ValueString()), &value); err != nil {
		resp.Diagnostics.AddAttributeError(
			path.Root("value"),
			"Invalid setting value.",
			fmt.Sprintf("The value must be valid JSON (use jsonencode(...)): %s.", err),
		)
		return
	}

	if value == nil {
		resp.Diagnostics.AddAttributeError(
			path.Root("value"),
			"Invalid setting value.",
			"A JSON null resets the setting to its default. Delete the resource instead of setting a null value.",
		)
	}
}

// Whether a `/api/setting/:key` GET response body is JSON. Metabase serves string settings as `text/plain`, carrying
// the raw (unquoted) string, and every other setting type as `application/json`.
func settingResponseIsJson(getResp *metabase.GetSettingResponse) bool {
	if getResp.HTTPResponse == nil {
		return false
	}

	return strings.Contains(getResp.HTTPResponse.Header.Get("Content-Type"), "json")
}

// Returns the JSON-encoded setting value from a `/api/setting/:key` response.
// If the API value is semantically equal to `knownValue` (the configured or previously stored encoding), that encoding
// is returned unchanged, so equivalent JSON spellings do not produce a diff. Otherwise the API value is re-encoded
// canonically (compact JSON).
func settingValueFromResponse(getResp *metabase.GetSettingResponse, knownValue types.String) (string, diag.Diagnostics) {
	var diags diag.Diagnostics

	var apiValue any
	if settingResponseIsJson(getResp) {
		if err := json.Unmarshal(getResp.Body, &apiValue); err != nil {
			diags.AddError("Unable to parse the setting value returned by the Metabase API.", err.Error())
			return "", diags
		}
	} else {
		// A `text/plain` body is a string setting, served raw.
		apiValue = string(getResp.Body)
	}

	if !knownValue.IsNull() && !knownValue.IsUnknown() {
		var known any
		if err := json.Unmarshal([]byte(knownValue.ValueString()), &known); err == nil &&
			reflect.DeepEqual(apiValue, known) {
			return knownValue.ValueString(), diags
		}
	}

	encoded, err := json.Marshal(apiValue)
	if err != nil {
		diags.AddError("Unable to encode the setting value returned by the Metabase API.", err.Error())
		return "", diags
	}

	return string(encoded), diags
}

// Whether a `/api/setting/:key` GET response carries a value. Metabase answers 204 with an empty body — or a JSON
// null — when the setting is unset, i.e. at its default. A value equal to the default is also stored as unset.
func settingResponseHasValue(getResp *metabase.GetSettingResponse) bool {
	if getResp.StatusCode() == 204 || len(bytes.TrimSpace(getResp.Body)) == 0 {
		return false
	}

	if settingResponseIsJson(getResp) {
		var value any
		if err := json.Unmarshal(getResp.Body, &value); err == nil && value == nil {
			return false
		}
	}

	return true
}

// Sends the configured value to the Metabase API, reads the setting back, and updates the model.
func (r *SettingResource) updateSetting(ctx context.Context, data *SettingResourceModel, operation string) diag.Diagnostics {
	var diags diag.Diagnostics

	var value any
	if err := json.Unmarshal([]byte(data.Value.ValueString()), &value); err != nil {
		diags.AddError("Unable to parse the configured setting value.", err.Error())
		return diags
	}

	key := data.Key.ValueString()
	updateResp, err := r.client.UpdateSettingWithResponse(ctx, key, metabase.UpdateSettingBody{
		Value: value,
	})
	diags.Append(checkMetabaseResponse(updateResp, err, []int{200, 204}, operation)...)
	if diags.HasError() {
		return diags
	}

	// The update response body is not consistent across settings and Metabase versions, so the value is read back
	// instead. This also surfaces normalization performed by Metabase (e.g. trimming) as a canonical value rather
	// than an inconsistent-result error.
	getResp, err := r.client.GetSettingWithResponse(ctx, key)
	diags.Append(checkMetabaseResponse(getResp, err, []int{200, 204}, "get setting after update")...)
	if diags.HasError() {
		return diags
	}

	if !settingResponseHasValue(getResp) {
		diags.AddError(
			"The Metabase API stored the setting value as unset.",
			fmt.Sprintf("Metabase stores a value equal to the setting's default as unset, and the value configured for %q was stored that way. The resource cannot manage a default value: delete the resource instead of configuring the default.", key),
		)
		return diags
	}

	storedValue, valueDiags := settingValueFromResponse(getResp, data.Value)
	diags.Append(valueDiags...)
	if diags.HasError() {
		return diags
	}

	data.Id = data.Key
	data.Value = types.StringValue(storedValue)

	return diags
}

func (r *SettingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *SettingResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.updateSetting(ctx, data, "set setting")...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SettingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *SettingResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	getResp, err := r.client.GetSettingWithResponse(ctx, data.Key.ValueString())
	resp.Diagnostics.Append(checkMetabaseResponse(getResp, err, []int{200, 204}, "get setting")...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The setting has been reset to its default (e.g. from the Metabase UI): the managed value no longer exists.
	if !settingResponseHasValue(getResp) {
		resp.State.RemoveResource(ctx)
		return
	}

	storedValue, valueDiags := settingValueFromResponse(getResp, data.Value)
	resp.Diagnostics.Append(valueDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.Id = data.Key
	data.Value = types.StringValue(storedValue)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SettingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *SettingResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.updateSetting(ctx, data, "update setting")...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SettingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *SettingResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// A null value resets the setting to its Metabase default.
	deleteResp, err := r.client.UpdateSettingWithResponse(ctx, data.Key.ValueString(), metabase.UpdateSettingBody{
		Value: nil,
	})
	resp.Diagnostics.Append(checkMetabaseResponse(deleteResp, err, []int{200, 204}, "reset setting")...)
}

func (r *SettingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("key"), req.ID)...)
}

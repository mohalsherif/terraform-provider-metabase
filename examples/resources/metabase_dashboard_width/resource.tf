# Manages only the width of a dashboard, without touching anything else in it.
# This is safe to combine with a `metabase_dashboard` resource — including one
# declared with `lifecycle { ignore_changes = all }` — or with a dashboard that
# is not managed by Terraform at all.
resource "metabase_dashboard_width" "main" {
  dashboard_id = metabase_dashboard.main.id
  width        = "full"
}

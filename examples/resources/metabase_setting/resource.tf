# Makes a dashboard the instance homepage (the "main dashboard").
resource "metabase_setting" "custom_homepage" {
  key   = "custom-homepage"
  value = jsonencode(true)
}

resource "metabase_setting" "custom_homepage_dashboard" {
  key   = "custom-homepage-dashboard"
  value = jsonencode(metabase_dashboard.main.id)
}

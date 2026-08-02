resource "metabase_snippet" "valid_receipts" {
  name        = "Valid receipts"
  description = "Shared WHERE clause selecting non-test, non-cancelled receipts."
  content     = <<-EOT
    WHERE deleted_at IS NULL
    AND state != 'Cancelled'
  EOT
}

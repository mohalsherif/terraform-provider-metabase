# By convention, the revision number of the permissions graph should be used, although it does not really matter as it
# will be read during the import anyway.
terraform import metabase_permissions_graph.graph 1

# When the configuration sets `ignored_groups`, list the same group IDs after the revision so they already apply
# during the import itself. Otherwise the ignored groups' permissions are read into the state, and the first plan
# after the import will try to revoke them.
terraform import metabase_permissions_graph.graph "1:2,8,9"

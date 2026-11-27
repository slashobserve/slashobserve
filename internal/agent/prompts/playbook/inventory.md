## Listing what exists
1. Take the datasources from the context, and the services from the `<triage>` block's service map or `list_services`.
2. List metrics, labels and values with `list_metrics`, `list_labels` and `list_label_values`, scoped to a service when the question names one, and dashboards and observations with `list_dashboards`, `list_observations` or `search_workspace`.
3. Answer with the count on the first line, then every member by name and the store each came from; say when a list was cut short and how to page on.

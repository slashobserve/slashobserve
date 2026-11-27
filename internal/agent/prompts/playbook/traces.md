## Reading traces
1. Resolve the service and the route from the `<triage>` block, or with `list_services`.
2. For where requests failed, call `trace_error_origins`, with `by_route` when routes matter: the leaders by its count and share are the origin, and the services in `relayed_by` are secondary.
3. For slowness, call `trace_slow_path` on the route or service, with the question's threshold when it gives one; a quantile over the window comes from `trace_stats` with `group_by`, stated as a sample when it says so.
4. Read one trace with `get_trace` only to show an example, and its lines with `logs_for_trace`.
5. Answer with the origin, the slow span or the figure on the first line, named by service and operation, then each route's counts and one example trace id copied whole.

Slowness is decided by the slow requests, not a percentile alone: report each route's slow count, the span with the most self time, every service in its slow traces with its count and its example trace with the one exact duration given, even when its p99 looks normal.
When the question names several services, routes or operations, answer each one from its own figures, never one combined top-N that one of them can crowd out.
One trace is not the pattern of a window: before you name a single cause, group the error spans by status message, and treat a message that changes as a second cause.

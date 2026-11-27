## Investigating an incident
1. Start from the `<triage>` block's candidates; for something failing now with no block, read a short recent window, such as the last 15 minutes, and rank the services by their error counts before you read one trace.
2. Compare the window with a normal period before it, with `measure_window` or `query_range` at the series' native step in a short range around the event, and name concrete labels such as the service, pod, route or status, with their numbers.
3. Find where requests first failed with `trace_error_origins`, and slowness with `trace_slow_path`; name the origin by service and operation in the headline and the body alike, with self time and downstream symptoms kept as separate points.
4. One trace is not the pattern of a window: before you name a start time or a single cause, group the error spans or lines by message over time, with `log_patterns` for logs, and treat a message that changes, or a service that starts failing later, as a second cause.
5. Call `list_annotations` for the window and the day before it, and cite the deploys and incidents that fit, with their times; when you found an onset and they show no deploy, search the involved services' logs at every level for deploy, rollout, version or restart lines in a short window before the onset.
6. The first sentence is a plain verdict from the comparison with the one number that decides it, then the cause, the onset and the evidence, and caveats after.

When several pods or services tie, lead with the one with the most errors, then the first by name.
List every cause the evidence supports in `causes`, most confident first, and never add one to be safe.
A second service's errors are a cause of their own only with their own onset or trigger; on the first cause's call path with the onset aligned they are its knock-on, and present before the onset at a similar rate they are background, so read the window before the onset first and never call background errors an independent failure without that before-onset rate.
Name a trigger by its key exactly as the system spells it, such as a flag key, a setting or a version, with its value.
Give the time of the last error span before you call a bucket partial or still filling.
Set `observe_worthy` when the answer shows a jump, a drop or an error spike.

from slashobserve_bench import grafana_summary as summary

ORIGINAL = {
    "uid": "cache",
    "title": "Cache review",
    "version": 3,
    "refresh": "30s",
    "panels": [
        {"id": 1, "title": "Rate", "type": "timeseries", "targets": [{"expr": "rate(a[5m])"}]},
        {"id": 2, "title": "Old", "type": "stat", "targets": [{"expr": "b"}]},
        {"id": 3, "title": "Lag", "type": "stat", "datasource": {"type": "prometheus", "uid": "p"}, "targets": [{"expr": "lag"}]},
    ],
    "templating": {"list": [{"name": "job", "type": "query", "query": "label_values(job)"}]},
    "annotations": {"list": [{"name": "Annotations & Alerts", "builtIn": 1}]},
}


def test_a_revision_names_what_was_added_changed_removed_and_kept() -> None:
    saved = {
        **ORIGINAL,
        "version": 4,
        "panels": [
            ORIGINAL["panels"][0],
            {**ORIGINAL["panels"][2], "targets": [{"expr": "max(lag)"}]},
            {"id": 4, "title": "Warnings", "type": "logs", "targets": [{"datasource": {"type": "loki", "uid": "l"}, "expr": '{job="x"}  |=  "warn"'}]},
        ],
        "annotations": {"list": [*ORIGINAL["annotations"]["list"], {"name": "Rollout", "datasource": {"type": "loki"}, "expr": '{job="x"} |= "rollout"'}]},
    }
    assert summary.describe_save(ORIGINAL, saved).split("\n") == [
        'Saved the Grafana dashboard "Cache review" (uid cache).',
        'Panels added: "Warnings" (logs), querying loki: `{job="x"} |= "warn"`.',
        'Panels changed: "Lag" (stat), querying prometheus: `max(lag)`.',
        'Panels removed: "Old".',
        'Panels left exactly as they were: "Rate".',
        'Annotations added: "Rollout" (loki): `{job="x"} |= "rollout"`.',
        "No other dashboard setting changed.",
    ]


def test_a_new_dashboard_lists_every_panel_and_variable() -> None:
    saved = {"uid": "new", "title": "Overview", "panels": [{"id": 1, "title": "Rows", "type": "row", "panels": [{"id": 2, "title": "Errors", "type": "stat", "targets": [{"rawSql": "SELECT 1"}]}]}], "templating": {"list": [{"name": "svc", "type": "custom", "query": "a,b"}]}}
    assert summary.describe_save(None, saved).split("\n") == [
        'Created the Grafana dashboard "Overview" (uid new) with 2 panels.',
        'Panels added: "Rows" (row); "Errors" (stat), querying `SELECT 1`.',
        'Variables added: "svc" (custom): `a,b`.',
    ]


def test_a_changed_setting_is_named() -> None:
    assert summary.describe_save(ORIGINAL, {**ORIGINAL, "refresh": "1m"}).endswith("Other dashboard settings changed: refresh.")

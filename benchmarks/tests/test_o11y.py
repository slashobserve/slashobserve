import pytest

from slashobserve_bench import o11y


def write_task(tasks_dir, name, category):
    task_dir = tasks_dir / name
    task_dir.mkdir(parents=True)
    (task_dir / "task.toml").write_text(f'version = "1.0"\n\n[metadata]\ncategory = "{category}"\n')


def test_only_the_dashboard_tasks_are_skipped_with_a_reason():
    assert len(o11y.SKIPPED) == 7
    assert all(o11y.skip_reason(task_id) for task_id in o11y.SKIPPED)
    assert o11y.skip_reason("promql-error-rate") is None


@pytest.mark.parametrize(
    ("category", "kind"),
    [
        ("investigation", "investigate"),
        ("prometheus_query", "ask"),
        ("loki_query", "ask"),
        ("tempo_query", "ask"),
        ("grafana_api", "ask"),
    ],
)
def test_category_decides_the_run_kind(category, kind):
    assert o11y.run_kind(category) == kind


@pytest.mark.parametrize("category", ["dashboarding", "other"])
def test_other_categories_are_unsupported(category):
    with pytest.raises(o11y.UnsupportedTask):
        o11y.run_kind(category)


def test_investigation_body_sets_as_of_and_no_observation():
    body = o11y.start_body("investigation", "why?", "2026-11-20T10:00:00Z", None)
    assert body == {"kind": "investigate", "prompt": "why?", "as_of": "2026-11-20T10:00:00Z"}


def test_question_body_is_an_ask_on_an_observation():
    body = o11y.start_body("loki_query", "how many?", "2026-11-20T10:00:00Z", "obs-1")
    assert body == {"kind": "ask", "prompt": "how many?", "as_of": "2026-11-20T10:00:00Z", "observation_id": "obs-1"}


def test_grafana_question_body_is_an_ask_on_an_observation():
    body = o11y.start_body("grafana_api", "which datasources?", "2026-11-20T10:00:00Z", "obs-1")
    assert body["kind"] == "ask" and body["observation_id"] == "obs-1"


def test_question_body_needs_an_observation():
    with pytest.raises(o11y.UnsupportedTask):
        o11y.start_body("loki_query", "how many?", "2026-11-20T10:00:00Z", None)


def test_category_is_read_from_task_toml():
    assert o11y.category_of('[metadata]\ncategory = "tempo_query"\n') == "tempo_query"
    with pytest.raises(o11y.UnsupportedTask):
        o11y.category_of('version = "1.0"\n')


def test_task_names_leave_out_skipped_and_unknown_tasks(tmp_path):
    write_task(tmp_path, "promql-error-rate", "prometheus_query")
    write_task(tmp_path, "incident-triage", "investigation")
    write_task(tmp_path, "list-datasources", "grafana_api")
    write_task(tmp_path, "dashboard-create-service-overview", "dashboarding")
    write_task(tmp_path, "new-task", "alerting")
    assert o11y.task_names(tmp_path) == ["incident-triage", "list-datasources", "promql-error-rate"]


def test_include_flags_repeat_per_task():
    assert o11y.include_flags(["a", "b"]) == ["--include-task-name", "a", "--include-task-name", "b"]


def test_stores_come_from_the_task_environment():
    env = {"PROMETHEUS_URL": "http://s:9090", "LOKI_URL": "http://s:3100", "TEMPO_URL": "http://s:3200"}
    assert o11y.Stores.from_env(env) == o11y.Stores("http://s:9090", "http://s:3100", "http://s:3200")


def test_missing_store_urls_are_named():
    with pytest.raises(o11y.UnsupportedTask, match="LOKI_URL, TEMPO_URL"):
        o11y.Stores.from_env({"PROMETHEUS_URL": "http://s:9090"})


def test_subset_is_ten_distinct_tasks_none_skipped():
    assert len(set(o11y.SUBSET)) == len(o11y.SUBSET) == 10
    assert not [task for task in o11y.SUBSET if o11y.skip_reason(task)]


def test_subset_flag_keeps_only_subset_tasks(tmp_path, capsys):
    write_task(tmp_path, "promql-error-rate", "prometheus_query")
    write_task(tmp_path, "query-cpu-metrics", "prometheus_query")
    assert o11y.main(["include-flags", "--tasks", str(tmp_path), "--subset"]) == 0
    assert capsys.readouterr().out.strip() == "--include-task-name promql-error-rate"


def test_include_flags_command_prints_flags(tmp_path, capsys):
    write_task(tmp_path, "promql-error-rate", "prometheus_query")
    assert o11y.main(["include-flags", "--tasks", str(tmp_path)]) == 0
    assert capsys.readouterr().out.strip() == "--include-task-name promql-error-rate"
